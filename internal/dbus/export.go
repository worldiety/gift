package dbus

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

// This file is the server side of a connection: objects that other peers on
// the bus call. A StatusNotifierItem is one — the panel reads its icon as a
// property and calls GetLayout on its menu — and so are the two interfaces
// every D-Bus object is expected to have, org.freedesktop.DBus.Properties
// and org.freedesktop.DBus.Introspectable, which are implemented here once
// for every exported object rather than by each one. Like the rest of the
// package it carries no build tag; export_test.go calls into it over the
// test bus of conn_test.go.
//
// # The model
//
// An object is a path and the interfaces on it. An [Interface] is a name and
// three tables: methods, which a Go function answers; properties, which a Go
// function reads; and signals, which are only listed, so that Introspect can
// name them — they are sent with [Conn.Emit]. The tables are values given to
// [Conn.Export] and not changed afterwards; to change what an object offers,
// export it again. What a property returns changes all the time, which is
// why a property is a function and not a value.
//
// A call is checked before the Go function sees it: the object must exist,
// the interface and the method must exist on it, and the signature of the
// arguments must be the one the method declares. Each failure is the
// standard error of the specification — UnknownObject, UnknownInterface,
// UnknownMethod, InvalidArgs — which is what a caller such as a panel logs
// when it talks to the wrong object, and much better than a timeout. A call
// without an interface, which the specification allows, is matched by the
// member name over all interfaces of the object.
//
// # Where it runs
//
// Every method and every getter runs on the reader goroutine of the
// connection, one after another, in the order the calls arrived. That order
// is what a menu needs: a panel sends "clicked" and then asks for the layout
// again, and must not see the answers the other way round. It also means
// that a method must not block and must not make a call on the same
// connection (see [Conn]); a method with a slow consequence starts it on a
// goroutine of its own and returns.
//
// # Properties
//
// Get and GetAll are answered from the getters. Set is refused for every
// property with PropertyReadOnly: neither of the users of this package has a
// property another program may write, and saying so is better than a write
// that is silently dropped. PropertiesChanged is not emitted on its own; an
// object that wants it emits it, and the StatusNotifierItem protocol has
// signals of its own (NewIcon, NewToolTip, …) instead.

// The names of the standard interfaces every exported object has.
const (
	IntrospectableInterface = "org.freedesktop.DBus.Introspectable"
	PropertiesInterface     = "org.freedesktop.DBus.Properties"
	PeerInterface           = "org.freedesktop.DBus.Peer"
)

// The standard error names of the specification that this package sends,
// and that a [Method] may return in an [*Error] of its own.
const (
	ErrorFailed           = "org.freedesktop.DBus.Error.Failed"
	ErrorUnknownObject    = "org.freedesktop.DBus.Error.UnknownObject"
	ErrorUnknownInterface = "org.freedesktop.DBus.Error.UnknownInterface"
	ErrorUnknownMethod    = "org.freedesktop.DBus.Error.UnknownMethod"
	ErrorUnknownProperty  = "org.freedesktop.DBus.Error.UnknownProperty"
	ErrorPropertyReadOnly = "org.freedesktop.DBus.Error.PropertyReadOnly"
	ErrorInvalidArgs      = "org.freedesktop.DBus.Error.InvalidArgs"
)

// The flags of the bus's RequestName and the answers it gives, from the
// specification's section "org.freedesktop.DBus.RequestName".
const (
	NameFlagAllowReplacement = 0x1
	NameFlagReplaceExisting  = 0x2
	NameFlagDoNotQueue       = 0x4

	RequestNamePrimaryOwner = 1
	RequestNameInQueue      = 2
	RequestNameExists       = 3
	RequestNameAlreadyOwner = 4
)

// Interface is one interface of an exported object.
type Interface struct {
	// Name is the interface name, such as "org.kde.StatusNotifierItem".
	Name string

	// Methods are the methods by member name.
	Methods map[string]Method

	// Properties are the read-only properties by name.
	Properties map[string]Property

	// Signals are the signatures of the signals this interface emits, by
	// member name. They are only used by Introspect.
	Signals map[string]Signature
}

// Method is one method of an exported interface.
type Method struct {
	// In and Out are the signatures of the arguments and of the reply. A
	// call with other arguments than In is refused with InvalidArgs before
	// Call sees it.
	In, Out Signature

	// Call answers the call m with the body of the reply, of the types Out.
	// An [*Error] it returns is sent back with its name; any other error is
	// sent as org.freedesktop.DBus.Error.Failed. It runs on the reader
	// goroutine; see the comment at the top of this file.
	Call func(m *Message) ([]any, error)
}

// Property is one read-only property of an exported interface.
type Property struct {
	// Sig is the type of the property, a single complete type.
	Sig Signature

	// Get returns the current value, of the type Sig. It runs on the reader
	// goroutine.
	Get func() any
}

// Export makes the interfaces available at path, replacing whatever was
// exported there before. Properties, Introspectable and Peer are added by
// this package and must not be among them.
func (c *Conn) Export(path ObjectPath, ifaces ...*Interface) error {
	if err := validObjectPath(string(path)); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, it := range ifaces {
		if err := it.check(); err != nil {
			return err
		}
		if seen[it.Name] {
			return fmt.Errorf("gift/dbus: interface %s is exported twice at %s", it.Name, path)
		}
		seen[it.Name] = true
	}
	c.omu.Lock()
	if c.objects == nil {
		c.objects = map[ObjectPath][]*Interface{}
	}
	c.objects[path] = slices.Clone(ifaces)
	c.omu.Unlock()
	return nil
}

// Unexport removes the object at path. A later call to it is answered with
// UnknownObject.
func (c *Conn) Unexport(path ObjectPath) {
	c.omu.Lock()
	delete(c.objects, path)
	c.omu.Unlock()
}

// check refuses an interface that would make the connection send what the
// bus disconnects for, or what no caller could call.
func (it *Interface) check() error {
	if it.Name == "" || !strings.Contains(it.Name, ".") {
		return fmt.Errorf("gift/dbus: %q is not an interface name", it.Name)
	}
	switch it.Name {
	case IntrospectableInterface, PropertiesInterface, PeerInterface:
		return fmt.Errorf("gift/dbus: %s is implemented by this package and cannot be exported", it.Name)
	}
	// This validSignature is the package's without UNIX_FD, which a signature may name
	// but this package can neither send nor receive.
	validSignature := func(s string) error {
		if strings.IndexByte(s, 'h') >= 0 {
			return fmt.Errorf("gift/dbus: D-Bus signature %q has a UNIX_FD, which is not supported", s)
		}
		return validSignature(s)
	}
	for name, m := range it.Methods {
		if err := validSignature(string(m.In)); err != nil {
			return fmt.Errorf("gift/dbus: %s.%s: %w", it.Name, name, err)
		}
		if err := validSignature(string(m.Out)); err != nil {
			return fmt.Errorf("gift/dbus: %s.%s: %w", it.Name, name, err)
		}
		if m.Call == nil {
			return fmt.Errorf("gift/dbus: %s.%s has no Call", it.Name, name)
		}
	}
	for name, p := range it.Properties {
		if _, rest, err := nextType(string(p.Sig)); err != nil || rest != "" || strings.IndexByte(string(p.Sig), 'h') >= 0 {
			return fmt.Errorf("gift/dbus: property %s.%s has the type %q, which is not a single complete type", it.Name, name, p.Sig)
		}
		if p.Get == nil {
			return fmt.Errorf("gift/dbus: property %s.%s has no Get", it.Name, name)
		}
	}
	for name, sig := range it.Signals {
		if err := validSignature(string(sig)); err != nil {
			return fmt.Errorf("gift/dbus: signal %s.%s: %w", it.Name, name, err)
		}
	}
	return nil
}

// dispatch answers a method call addressed to this connection. It runs on
// the reader goroutine.
func (c *Conn) dispatch(m *Message) {
	sig, body, err := c.handle(m)
	if m.Flags&FlagNoReplyExpected != 0 {
		return
	}
	c.reply(m, sig, body, err)
}

// reply sends the answer to call: a return with body, or the error. A reply
// that cannot be marshalled — a getter that returned the wrong Go type — is
// replaced by an error reply that says so, so that the caller learns of the
// defect instead of waiting for its timeout.
func (c *Conn) reply(call *Message, sig Signature, body []any, err error) {
	r := &Message{Type: TypeMethodReturn, Destination: call.Sender, ReplySerial: call.Serial, Sig: sig, Body: body}
	if err != nil {
		r = errorReply(call, err)
	}
	c.mu.Lock()
	r.Serial = c.nextSerialLocked()
	c.mu.Unlock()
	b, merr := r.Marshal()
	if merr != nil {
		// The serial is reused: the reply it was taken for is never sent.
		serial := r.Serial
		r = errorReply(call, fmt.Errorf("gift/dbus: the reply to %s.%s cannot be sent: %w", call.Interface, call.Member, merr))
		r.Serial = serial
		if b, merr = r.Marshal(); merr != nil {
			return
		}
	}
	_ = c.write(b)
}

func errorReply(call *Message, err error) *Message {
	name, text := ErrorFailed, err.Error()
	var be *Error
	if errors.As(err, &be) {
		name, text = be.Name, be.Message
	}
	if validString(text) != nil {
		text = strings.ToValidUTF8(strings.ReplaceAll(text, "\x00", ""), "?")
	}
	return &Message{Type: TypeError, Destination: call.Sender, ReplySerial: call.Serial,
		ErrorName: name, Sig: "s", Body: []any{text}}
}

func errorf(name, format string, args ...any) error {
	return &Error{Name: name, Message: fmt.Sprintf(format, args...)}
}

// handle finds what answers m and runs it.
func (c *Conn) handle(m *Message) (Signature, []any, error) {
	if m.Interface == PeerInterface {
		return handlePeer(m)
	}
	c.omu.RLock()
	ifaces, ok := c.objects[m.Path]
	children := c.childrenLocked(m.Path)
	c.omu.RUnlock()

	if !ok {
		// A path above an exported object exists in the sense of
		// Introspect, which is how a tool such as d-feet or busctl walks
		// down to the object; it has nothing else.
		if len(children) > 0 && (m.Interface == "" || m.Interface == IntrospectableInterface) && m.Member == "Introspect" {
			return "s", []any{introspect(nil, children)}, nil
		}
		if m.Interface == "" && m.Member == "Ping" {
			return handlePeer(m)
		}
		return "", nil, errorf(ErrorUnknownObject, "no object at %s", m.Path)
	}

	iface := m.Interface
	if iface == "" {
		iface = memberOwner(ifaces, m.Member)
	}
	switch iface {
	case IntrospectableInterface:
		if m.Member != "Introspect" {
			break
		}
		if m.Sig != "" {
			return "", nil, errorf(ErrorInvalidArgs, "Introspect takes no arguments, got %q", m.Sig)
		}
		return "s", []any{introspect(ifaces, children)}, nil
	case PropertiesInterface:
		return handleProperties(ifaces, m)
	case PeerInterface:
		return handlePeer(m)
	}

	var it *Interface
	for _, x := range ifaces {
		if x.Name == iface {
			it = x
		}
	}
	if it == nil {
		return "", nil, errorf(ErrorUnknownInterface, "no interface %s at %s", m.Interface, m.Path)
	}
	method, ok := it.Methods[m.Member]
	if !ok {
		return "", nil, errorf(ErrorUnknownMethod, "no method %s.%s at %s", it.Name, m.Member, m.Path)
	}
	if m.Sig != method.In {
		return "", nil, errorf(ErrorInvalidArgs, "%s.%s takes (%s), got (%s)", it.Name, m.Member, method.In, m.Sig)
	}
	body, err := method.Call(m)
	return method.Out, body, err
}

// memberOwner is the interface a call without an interface field means: the
// first exported interface with a method of that name, else the standard
// interface that has it, else "" and the call fails as UnknownInterface.
func memberOwner(ifaces []*Interface, member string) string {
	for _, it := range ifaces {
		if _, ok := it.Methods[member]; ok {
			return it.Name
		}
	}
	switch member {
	case "Introspect":
		return IntrospectableInterface
	case "Get", "GetAll", "Set":
		return PropertiesInterface
	case "Ping", "GetMachineId":
		return PeerInterface
	}
	return ""
}

// childrenLocked returns the names of the elements directly below path that
// lead to an exported object, sorted. c.omu is held.
func (c *Conn) childrenLocked(path ObjectPath) []string {
	prefix := string(path) + "/"
	if path == "/" {
		prefix = "/"
	}
	var out []string
	for p := range c.objects {
		rest, ok := strings.CutPrefix(string(p), prefix)
		if !ok || rest == "" {
			continue
		}
		name, _, _ := strings.Cut(rest, "/")
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

func handlePeer(m *Message) (Signature, []any, error) {
	switch m.Member {
	case "Ping":
		return "", nil, nil
	case "GetMachineId":
		for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if b, err := os.ReadFile(p); err == nil {
				if id := strings.TrimSpace(string(b)); id != "" {
					return "s", []any{id}, nil
				}
			}
		}
		return "", nil, errorf(ErrorFailed, "this machine has no machine id")
	}
	return "", nil, errorf(ErrorUnknownMethod, "no method %s.%s", PeerInterface, m.Member)
}

func handleProperties(ifaces []*Interface, m *Message) (Signature, []any, error) {
	find := func(name string) *Interface {
		for _, it := range ifaces {
			if it.Name == name {
				return it
			}
		}
		return nil
	}
	switch m.Member {
	case "Get":
		if m.Sig != "ss" {
			return "", nil, errorf(ErrorInvalidArgs, "Get takes (ss), got (%s)", m.Sig)
		}
		iname, pname := m.Body[0].(string), m.Body[1].(string)
		it := find(iname)
		if it == nil {
			return "", nil, errorf(ErrorUnknownInterface, "no interface %s at %s", iname, m.Path)
		}
		p, ok := it.Properties[pname]
		if !ok {
			return "", nil, errorf(ErrorUnknownProperty, "no property %s.%s", iname, pname)
		}
		return "v", []any{Variant{Sig: p.Sig, Value: p.Get()}}, nil
	case "GetAll":
		if m.Sig != "s" {
			return "", nil, errorf(ErrorInvalidArgs, "GetAll takes (s), got (%s)", m.Sig)
		}
		iname := m.Body[0].(string)
		it := find(iname)
		if it == nil {
			return "", nil, errorf(ErrorUnknownInterface, "no interface %s at %s", iname, m.Path)
		}
		names := make([]string, 0, len(it.Properties))
		for name := range it.Properties {
			names = append(names, name)
		}
		slices.Sort(names)
		all := make([]DictEntry, 0, len(names))
		for _, name := range names {
			p := it.Properties[name]
			all = append(all, DictEntry{Key: name, Value: Variant{Sig: p.Sig, Value: p.Get()}})
		}
		return "a{sv}", []any{all}, nil
	case "Set":
		if m.Sig != "ssv" {
			return "", nil, errorf(ErrorInvalidArgs, "Set takes (ssv), got (%s)", m.Sig)
		}
		iname, pname := m.Body[0].(string), m.Body[1].(string)
		it := find(iname)
		if it == nil {
			return "", nil, errorf(ErrorUnknownInterface, "no interface %s at %s", iname, m.Path)
		}
		if _, ok := it.Properties[pname]; !ok {
			return "", nil, errorf(ErrorUnknownProperty, "no property %s.%s", iname, pname)
		}
		return "", nil, errorf(ErrorPropertyReadOnly, "%s.%s is read-only", iname, pname)
	}
	return "", nil, errorf(ErrorUnknownMethod, "no method %s.%s", PropertiesInterface, m.Member)
}

// The standard interfaces as Introspect lists them for every object.
const standardInterfaces = `  <interface name="org.freedesktop.DBus.Peer">
    <method name="Ping"/>
    <method name="GetMachineId">
      <arg type="s" direction="out"/>
    </method>
  </interface>
  <interface name="org.freedesktop.DBus.Introspectable">
    <method name="Introspect">
      <arg type="s" direction="out"/>
    </method>
  </interface>
  <interface name="org.freedesktop.DBus.Properties">
    <method name="Get">
      <arg type="s" direction="in"/>
      <arg type="s" direction="in"/>
      <arg type="v" direction="out"/>
    </method>
    <method name="GetAll">
      <arg type="s" direction="in"/>
      <arg type="a{sv}" direction="out"/>
    </method>
    <method name="Set">
      <arg type="s" direction="in"/>
      <arg type="s" direction="in"/>
      <arg type="v" direction="in"/>
    </method>
    <signal name="PropertiesChanged">
      <arg type="s"/>
      <arg type="a{sv}"/>
      <arg type="as"/>
    </signal>
  </interface>
`

// introspect returns the XML of the specification's section "Introspection
// Data Format" for an object with ifaces and the child nodes children. An
// object without interfaces — a path above an exported one — lists only its
// children. The output is sorted, so that it is the same on every call.
func introspect(ifaces []*Interface, children []string) string {
	var b bytes.Buffer
	b.WriteString(`<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN"` + "\n" +
		` "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">` + "\n<node>\n")
	if len(ifaces) > 0 {
		b.WriteString(standardInterfaces)
	}
	args := func(indent string, sig Signature, dir string) {
		s := string(sig)
		for s != "" {
			t, rest, err := nextType(s)
			if err != nil {
				return
			}
			if dir == "" {
				fmt.Fprintf(&b, "%s<arg type=\"%s\"/>\n", indent, xmlEscape(t))
			} else {
				fmt.Fprintf(&b, "%s<arg type=\"%s\" direction=\"%s\"/>\n", indent, xmlEscape(t), dir)
			}
			s = rest
		}
	}
	for _, it := range ifaces {
		fmt.Fprintf(&b, "  <interface name=\"%s\">\n", xmlEscape(it.Name))
		for _, name := range sortedKeys(it.Methods) {
			m := it.Methods[name]
			if m.In == "" && m.Out == "" {
				fmt.Fprintf(&b, "    <method name=\"%s\"/>\n", xmlEscape(name))
				continue
			}
			fmt.Fprintf(&b, "    <method name=\"%s\">\n", xmlEscape(name))
			args("      ", m.In, "in")
			args("      ", m.Out, "out")
			b.WriteString("    </method>\n")
		}
		for _, name := range sortedKeys(it.Properties) {
			fmt.Fprintf(&b, "    <property name=\"%s\" type=\"%s\" access=\"read\"/>\n",
				xmlEscape(name), xmlEscape(string(it.Properties[name].Sig)))
		}
		for _, name := range sortedKeys(it.Signals) {
			sig := it.Signals[name]
			if sig == "" {
				fmt.Fprintf(&b, "    <signal name=\"%s\"/>\n", xmlEscape(name))
				continue
			}
			fmt.Fprintf(&b, "    <signal name=\"%s\">\n", xmlEscape(name))
			args("      ", sig, "")
			b.WriteString("    </signal>\n")
		}
		b.WriteString("  </interface>\n")
	}
	for _, name := range children {
		fmt.Fprintf(&b, "  <node name=\"%s\"/>\n", xmlEscape(name))
	}
	b.WriteString("</node>\n")
	return b.String()
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// xmlEscape escapes what may stand in an attribute. Names and signatures
// never contain any of it but "{" and "}", which need no escape; it is here
// so that the XML is well-formed whatever an interface is called.
func xmlEscape(s string) string { return xmlEscaper.Replace(s) }

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
