package dbus

import (
	"encoding/xml"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The server side, driven from the test bus of conn_test.go: the bus sends
// method calls to the connection the way dbus-daemon forwards those of a
// panel, and reads what comes back.

// peer is a test bus that collects everything the connection sends other
// than its own calls, which it answers with an empty return.
type peer struct {
	*testBus
	c   *Conn
	got chan *Message
}

func startPeer(t *testing.T) *peer {
	t.Helper()
	p := &peer{testBus: &testBus{}, got: make(chan *Message, 64)}
	p.handle = func(m *Message) *Message {
		if m.Type == TypeMethodCall {
			return &Message{Type: TypeMethodReturn, ReplySerial: m.Serial}
		}
		p.got <- m
		return nil
	}
	c, err := startTestBus(t, p.testBus, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	p.c = c
	return p
}

// ask sends a method call from ":1.1" to the connection and returns the
// reply to it.
func (p *peer) ask(t *testing.T, path ObjectPath, iface, member string, sig Signature, args ...any) *Message {
	t.Helper()
	m := &Message{Type: TypeMethodCall, Path: path, Interface: iface, Member: member, Sender: ":1.1",
		Destination: ":1.7", Sig: sig, Body: args}
	p.send(m)
	for {
		select {
		case r := <-p.got:
			if r.ReplySerial == m.Serial {
				return r
			}
		case <-time.After(time.Second):
			t.Fatalf("%s.%s at %s was not answered", iface, member, path)
			return nil
		}
	}
}

func wantError(t *testing.T, r *Message, name string) {
	t.Helper()
	if r.Type != TypeError || r.ErrorName != name {
		t.Fatalf("reply %+v, want the error %s", r, name)
	}
}

func echoInterface(state *string) *Interface {
	return &Interface{
		Name: "org.example.Echo",
		Methods: map[string]Method{
			"Echo": {In: "s", Out: "s", Call: func(m *Message) ([]any, error) {
				return []any{m.Body[0].(string)}, nil
			}},
			"Refuse": {Call: func(*Message) ([]any, error) {
				return nil, &Error{Name: "org.example.Error.No", Message: "never"}
			}},
			"Break": {Call: func(*Message) ([]any, error) { return nil, errors.New("broken") }},
			"Wrong": {Out: "u", Call: func(*Message) ([]any, error) { return []any{"not a u"}, nil }},
		},
		Properties: map[string]Property{
			"Text":  {Sig: "s", Get: func() any { return *state }},
			"Count": {Sig: "u", Get: func() any { return uint32(len(*state)) }},
		},
		Signals: map[string]Signature{"Changed": "su"},
	}
}

func TestAnExportedMethodIsCalledAndAnswered(t *testing.T) {
	p := startPeer(t)
	state := "x"
	if err := p.c.Export("/org/example/Echo", echoInterface(&state)); err != nil {
		t.Fatal(err)
	}
	r := p.ask(t, "/org/example/Echo", "org.example.Echo", "Echo", "s", "hello")
	if r.Type != TypeMethodReturn || r.Destination != ":1.1" || r.Sig != "s" || !reflect.DeepEqual(r.Body, []any{"hello"}) {
		t.Fatalf("reply %+v", r)
	}
	// Without an interface, the member is looked up over all of them.
	r = p.ask(t, "/org/example/Echo", "", "Echo", "s", "again")
	if r.Type != TypeMethodReturn || r.Body[0] != "again" {
		t.Fatalf("reply without interface %+v", r)
	}
}

func TestACallThatCannotBeAnsweredGetsTheStandardError(t *testing.T) {
	p := startPeer(t)
	state := ""
	if err := p.c.Export("/org/example/Echo", echoInterface(&state)); err != nil {
		t.Fatal(err)
	}
	wantError(t, p.ask(t, "/nowhere", "org.example.Echo", "Echo", "s", "x"), ErrorUnknownObject)
	wantError(t, p.ask(t, "/org/example/Echo", "org.example.Other", "Echo", "s", "x"), ErrorUnknownInterface)
	wantError(t, p.ask(t, "/org/example/Echo", "org.example.Echo", "Nope", ""), ErrorUnknownMethod)
	wantError(t, p.ask(t, "/org/example/Echo", "org.example.Echo", "Echo", "u", uint32(1)), ErrorInvalidArgs)
	r := p.ask(t, "/org/example/Echo", "org.example.Echo", "Refuse", "")
	wantError(t, r, "org.example.Error.No")
	if r.Body[0] != "never" {
		t.Fatalf("error message %v", r.Body)
	}
	wantError(t, p.ask(t, "/org/example/Echo", "org.example.Echo", "Break", ""), ErrorFailed)
	// A reply of the wrong Go type is a defect of the method, and the caller
	// hears of it rather than waiting.
	r = p.ask(t, "/org/example/Echo", "org.example.Echo", "Wrong", "")
	wantError(t, r, ErrorFailed)
	if !strings.Contains(r.Body[0].(string), "cannot be sent") {
		t.Fatalf("error message %v", r.Body)
	}

	p.c.Unexport("/org/example/Echo")
	wantError(t, p.ask(t, "/org/example/Echo", "org.example.Echo", "Echo", "s", "x"), ErrorUnknownObject)
}

func TestPropertiesAreReadFromTheGetters(t *testing.T) {
	p := startPeer(t)
	state := "abc"
	if err := p.c.Export("/org/example/Echo", echoInterface(&state)); err != nil {
		t.Fatal(err)
	}
	r := p.ask(t, "/org/example/Echo", PropertiesInterface, "Get", "ss", "org.example.Echo", "Text")
	if r.Type != TypeMethodReturn || !reflect.DeepEqual(r.Body, []any{Variant{Sig: "s", Value: "abc"}}) {
		t.Fatalf("Get %+v", r)
	}
	state = "abcd"
	r = p.ask(t, "/org/example/Echo", PropertiesInterface, "GetAll", "s", "org.example.Echo")
	want := []any{[]DictEntry{
		{Key: "Count", Value: Variant{Sig: "u", Value: uint32(4)}},
		{Key: "Text", Value: Variant{Sig: "s", Value: "abcd"}},
	}}
	if r.Type != TypeMethodReturn || r.Sig != "a{sv}" || !reflect.DeepEqual(r.Body, want) {
		t.Fatalf("GetAll %+v", r)
	}
	wantError(t, p.ask(t, "/org/example/Echo", PropertiesInterface, "Get", "ss", "org.example.Echo", "Nope"), ErrorUnknownProperty)
	wantError(t, p.ask(t, "/org/example/Echo", PropertiesInterface, "Get", "ss", "org.example.Nope", "Text"), ErrorUnknownInterface)
	wantError(t, p.ask(t, "/org/example/Echo", PropertiesInterface, "Set", "ssv", "org.example.Echo", "Text",
		Variant{Sig: "s", Value: "y"}), ErrorPropertyReadOnly)
	wantError(t, p.ask(t, "/org/example/Echo", PropertiesInterface, "Get", "s", "org.example.Echo"), ErrorInvalidArgs)
}

// introspection is the subset of the XML format the test reads back.
type introspection struct {
	Interfaces []struct {
		Name    string `xml:"name,attr"`
		Methods []struct {
			Name string `xml:"name,attr"`
			Args []struct {
				Type      string `xml:"type,attr"`
				Direction string `xml:"direction,attr"`
			} `xml:"arg"`
		} `xml:"method"`
		Properties []struct {
			Name   string `xml:"name,attr"`
			Type   string `xml:"type,attr"`
			Access string `xml:"access,attr"`
		} `xml:"property"`
		Signals []struct {
			Name string `xml:"name,attr"`
		} `xml:"signal"`
	} `xml:"interface"`
	Nodes []struct {
		Name string `xml:"name,attr"`
	} `xml:"node"`
}

func TestIntrospectDescribesTheObjectAndTheWayDownToIt(t *testing.T) {
	p := startPeer(t)
	state := ""
	if err := p.c.Export("/org/example/Echo", echoInterface(&state)); err != nil {
		t.Fatal(err)
	}
	r := p.ask(t, "/org/example/Echo", IntrospectableInterface, "Introspect", "")
	var x introspection
	if err := xml.Unmarshal([]byte(r.Body[0].(string)), &x); err != nil {
		t.Fatalf("not XML: %v\n%s", err, r.Body[0])
	}
	var names []string
	for _, it := range x.Interfaces {
		names = append(names, it.Name)
	}
	if want := []string{PeerInterface, IntrospectableInterface, PropertiesInterface, "org.example.Echo"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("interfaces %v, want %v", names, want)
	}
	echo := x.Interfaces[3]
	if echo.Methods[1].Name != "Echo" || len(echo.Methods[1].Args) != 2 || echo.Methods[1].Args[0].Direction != "in" ||
		echo.Methods[1].Args[1].Direction != "out" {
		t.Fatalf("Echo %+v", echo.Methods)
	}
	if len(echo.Properties) != 2 || echo.Properties[1].Name != "Text" || echo.Properties[1].Access != "read" {
		t.Fatalf("properties %+v", echo.Properties)
	}
	if len(echo.Signals) != 1 || echo.Signals[0].Name != "Changed" {
		t.Fatalf("signals %+v", echo.Signals)
	}

	for path, child := range map[ObjectPath]string{"/": "org", "/org": "example", "/org/example": "Echo"} {
		r := p.ask(t, path, IntrospectableInterface, "Introspect", "")
		var x introspection
		if err := xml.Unmarshal([]byte(r.Body[0].(string)), &x); err != nil {
			t.Fatal(err)
		}
		if len(x.Interfaces) != 0 || len(x.Nodes) != 1 || x.Nodes[0].Name != child {
			t.Fatalf("%s: %+v", path, x)
		}
	}
}

func TestEmitSendsASignalFromThePath(t *testing.T) {
	p := startPeer(t)
	if err := p.c.Emit("/org/example/Echo", "org.example.Echo", "Changed", "su", "a", uint32(1)); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-p.got:
		if m.Type != TypeSignal || m.Path != "/org/example/Echo" || m.Interface != "org.example.Echo" ||
			m.Member != "Changed" || !reflect.DeepEqual(m.Body, []any{"a", uint32(1)}) {
			t.Fatalf("signal %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("no signal")
	}
	if err := p.c.Emit("/org/example/Echo", "org.example.Echo", "Changed", "su", "a"); err == nil {
		t.Fatal("a signal with too few values was sent")
	}
}

// TestACallWithoutReplyIsRunButNotAnswered: a panel sends Event with
// NO_REPLY_EXPECTED. The method must run, and nothing may come back — a
// reply nobody waits for is what dbus-daemon logs as a rejected message.
func TestACallWithoutReplyIsRunButNotAnswered(t *testing.T) {
	p := startPeer(t)
	ran := make(chan string, 1)
	err := p.c.Export("/o", &Interface{Name: "org.example.Ev", Methods: map[string]Method{
		"Event": {In: "s", Call: func(m *Message) ([]any, error) { ran <- m.Body[0].(string); return nil, nil }},
	}})
	if err != nil {
		t.Fatal(err)
	}
	p.send(&Message{Type: TypeMethodCall, Flags: FlagNoReplyExpected, Path: "/o", Interface: "org.example.Ev",
		Member: "Event", Sender: ":1.1", Sig: "s", Body: []any{"clicked"}})
	if got := <-ran; got != "clicked" {
		t.Fatalf("ran with %q", got)
	}
	// A ping afterwards is answered first: nothing else was in the queue.
	r := p.ask(t, "/o", PeerInterface, "Ping", "")
	if r.Type != TypeMethodReturn {
		t.Fatalf("ping %+v", r)
	}
}

func TestExportRefusesWhatCannotBeServed(t *testing.T) {
	p := startPeer(t)
	call := func(*Message) ([]any, error) { return nil, nil }
	for name, tc := range map[string]struct {
		path ObjectPath
		it   *Interface
	}{
		"bad path":           {"/a/", &Interface{Name: "a.b"}},
		"bad name":           {"/a", &Interface{Name: "nodot"}},
		"standard interface": {"/a", &Interface{Name: PropertiesInterface}},
		"bad in signature":   {"/a", &Interface{Name: "a.b", Methods: map[string]Method{"M": {In: "(", Call: call}}}},
		"no call":            {"/a", &Interface{Name: "a.b", Methods: map[string]Method{"M": {}}}},
		"two type property":  {"/a", &Interface{Name: "a.b", Properties: map[string]Property{"P": {Sig: "ss", Get: func() any { return "" }}}}},
		"no getter":          {"/a", &Interface{Name: "a.b", Properties: map[string]Property{"P": {Sig: "s"}}}},
	} {
		if err := p.c.Export(tc.path, tc.it); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if err := p.c.Export("/a", &Interface{Name: "a.b"}, &Interface{Name: "a.b"}); err == nil {
		t.Error("an interface exported twice was accepted")
	}
}

// sendRaw sends a call from ":1.1" whose signature field, after marshalling
// as sig, is patched to patched; it returns the serial.
func (p *peer) sendRaw(t *testing.T, member string, sig, patched Signature, args ...any) uint32 {
	t.Helper()
	p.serial++
	m := &Message{Type: TypeMethodCall, Serial: p.serial, Path: "/org/example/Echo", Interface: "org.example.Echo",
		Member: member, Sender: ":1.1", Destination: ":1.7", Sig: sig, Body: args}
	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	field := []byte("\x08\x01g\x00\x01" + string(sig) + "\x00")
	i := strings.Index(string(b), string(field))
	if i < 0 || len(sig) != len(patched) {
		t.Fatal("no signature field to patch")
	}
	copy(b[i+5:], patched)
	p.out <- b
	return m.Serial
}

func (p *peer) answer(t *testing.T, serial uint32) *Message {
	t.Helper()
	for {
		select {
		case r := <-p.got:
			if r.ReplySerial == serial {
				return r
			}
		case <-time.After(time.Second):
			t.Fatalf("call %d was not answered", serial)
			return nil
		}
	}
}

// TestAMessageThatCannotBeDecodedIsRefusedNotFatal: dbus-daemon routes a
// body with a UNIX_FD in its signature, which this package does not decode;
// one such call from any peer must not end the connection.
func TestAMessageThatCannotBeDecodedIsRefusedNotFatal(t *testing.T) {
	p := startPeer(t)
	state := "x"
	if err := p.c.Export("/org/example/Echo", echoInterface(&state)); err != nil {
		t.Fatal(err)
	}
	wantError(t, p.answer(t, p.sendRaw(t, "Echo", "u", "h", uint32(0))), ErrorInvalidArgs)
	r := p.ask(t, "/org/example/Echo", "org.example.Echo", "Echo", "s", "still here")
	if r.Type != TypeMethodReturn || r.Body[0] != "still here" {
		t.Fatalf("reply after the undecodable call %+v", r)
	}
}

func TestAMessageBeyondTheReceiveLimitIsRefusedNotFatal(t *testing.T) {
	p := startPeer(t)
	state := "x"
	if err := p.c.Export("/org/example/Echo", echoInterface(&state)); err != nil {
		t.Fatal(err)
	}
	p.c.SetReceiveLimit(1024)
	wantError(t, p.ask(t, "/org/example/Echo", "org.example.Echo", "Echo", "s", strings.Repeat("a", 4096)),
		ErrorInvalidArgs)
	r := p.ask(t, "/org/example/Echo", "org.example.Echo", "Echo", "s", "small")
	if r.Type != TypeMethodReturn || r.Body[0] != "small" {
		t.Fatalf("reply after the large call %+v", r)
	}
}
