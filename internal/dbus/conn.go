package dbus

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// This file is a D-Bus connection: the address, the authentication, and a
// reader goroutine that matches replies to calls, hands signals on and
// passes the method calls of other peers to the objects of export.go. Like
// wire.go it carries no build tag, and for the same reason; the tests drive it
// over net.Pipe against a bus that lives in the test.
//
// # What it does not do
//
// It connects to Unix domain sockets only and authenticates with EXTERNAL
// only, which is what every session bus on Linux offers. TCP buses, the
// DBUS_COOKIE_SHA1 mechanism, autolaunch and the passing of file descriptors
// are left out: neither a notification server nor a status notifier host
// needs them, and a session bus that cannot be reached this way is reported
// by the caller as the absence of the feature — "no notifications", "no
// tray" — rather than worked around.

// The bus itself, which answers Hello, RequestName, AddMatch and the other
// methods of the specification's section "Message Bus Messages".
const (
	BusName      = "org.freedesktop.DBus"
	BusPath      = ObjectPath("/org/freedesktop/DBus")
	BusInterface = "org.freedesktop.DBus"
)

// ErrClosed is the error of every call after the connection was closed on
// purpose.
var ErrClosed = errors.New("gift/dbus: the D-Bus connection is closed")

// Error is an error reply: the error name the peer sent and the message
// that came with it, if any. A method of an exported object returns one to
// send that name back; see [Method].
type Error struct {
	Name, Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return "gift/dbus: " + e.Name
	}
	return "gift/dbus: " + e.Name + ": " + e.Message
}

// SessionBusAddress finds the address of the session bus the way libdbus
// does, minus autolaunch: the environment variable, or the socket systemd
// places in the runtime directory.
func SessionBusAddress(getenv func(string) string, exists func(string) bool) (string, error) {
	if a := getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a, nil
	}
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		p := strings.TrimSuffix(dir, "/") + "/bus"
		if exists(p) {
			return "unix:path=" + p, nil
		}
	}
	return "", errors.New("gift/dbus: no session bus, neither DBUS_SESSION_BUS_ADDRESS nor $XDG_RUNTIME_DIR/bus is set")
}

// busAddr is one connectable entry of a D-Bus address.
type busAddr struct {
	network, address string
}

// parseBusAddress returns the entries of a D-Bus address list that this
// package can connect to: unix:path= and, on Linux, unix:abstract=. The
// other transports are skipped; an address with none left is an error.
func parseBusAddress(s string) ([]busAddr, error) {
	var out []busAddr
	for _, entry := range strings.Split(s, ";") {
		if entry == "" {
			continue
		}
		transport, params, ok := strings.Cut(entry, ":")
		if !ok {
			return nil, fmt.Errorf("gift/dbus: D-Bus address %q has no transport", entry)
		}
		kv := map[string]string{}
		for _, p := range strings.Split(params, ",") {
			if p == "" {
				continue
			}
			k, v, ok := strings.Cut(p, "=")
			if !ok {
				return nil, fmt.Errorf("gift/dbus: D-Bus address %q has a key without a value", entry)
			}
			v, err := unescapeAddress(v)
			if err != nil {
				return nil, err
			}
			kv[k] = v
		}
		if transport != "unix" {
			continue
		}
		switch {
		case kv["path"] != "":
			out = append(out, busAddr{"unix", kv["path"]})
		case kv["abstract"] != "":
			// Go dials an abstract socket when the name starts with '@'.
			out = append(out, busAddr{"unix", "@" + kv["abstract"]})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("gift/dbus: D-Bus address %q has no Unix socket this package can connect to", s)
	}
	return out, nil
}

// unescapeAddress undoes the %XX escaping of D-Bus address values.
func unescapeAddress(v string) (string, error) {
	if !strings.Contains(v, "%") {
		return v, nil
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] != '%' {
			b.WriteByte(v[i])
			continue
		}
		if i+2 >= len(v) {
			return "", fmt.Errorf("gift/dbus: D-Bus address value %q ends in an escape", v)
		}
		x, err := hex.DecodeString(v[i+1 : i+3])
		if err != nil {
			return "", fmt.Errorf("gift/dbus: D-Bus address value %q has a bad escape", v)
		}
		b.WriteByte(x[0])
		i += 2
	}
	return b.String(), nil
}

// Conn is an authenticated connection to a bus.
//
// Calls may be made from any goroutine. The reader goroutine delivers replies
// to the call waiting for them, hands every signal to the handler and runs
// the methods and property getters of exported objects, all on itself, so
// none of these may block or make a call on this connection: the reply it
// would wait for is read by the goroutine that is waiting. Emitting a signal
// is fine, it only writes.
type Conn struct {
	nc      net.Conn
	r       *bufio.Reader
	timeout time.Duration

	// wmu serialises writes, so that two messages never interleave.
	wmu sync.Mutex

	mu      sync.Mutex
	serial  uint32
	pending map[uint32]chan *Message
	err     error

	onSignal atomic.Pointer[func(*Message)]

	// limit is the receive limit; see [Conn.SetReceiveLimit].
	limit atomic.Int64

	// omu guards objects, the exported objects of export.go by path.
	omu     sync.RWMutex
	objects map[ObjectPath][]*Interface

	// name is the unique name the bus assigned in Hello.
	name string
}

// Dial connects to the first entry of address that accepts, authenticates
// and says Hello. timeout bounds each step and, afterwards, each call.
func Dial(address string, timeout time.Duration) (*Conn, error) {
	addrs, err := parseBusAddress(address)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, a := range addrs {
		nc, err := net.DialTimeout(a.network, a.address, timeout)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		c, err := newConn(nc, os.Getuid(), timeout)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		return c, nil
	}
	return nil, errors.Join(errs...)
}

// newConn runs the authentication and Hello over nc, which it closes on
// failure.
func newConn(nc net.Conn, uid int, timeout time.Duration) (*Conn, error) {
	r := bufio.NewReader(nc)
	_ = nc.SetDeadline(time.Now().Add(timeout))
	if err := authenticate(nc, r, uid); err != nil {
		nc.Close()
		return nil, err
	}
	_ = nc.SetDeadline(time.Time{})

	c := &Conn{nc: nc, r: r, timeout: timeout, pending: map[uint32]chan *Message{}}
	c.limit.Store(maxMessageBytes)
	go c.readLoop()
	reply, err := c.Call(BusName, BusPath, BusInterface, "Hello", "")
	if err != nil {
		c.Close()
		return nil, err
	}
	if len(reply) != 1 {
		c.Close()
		return nil, errors.New("gift/dbus: the bus answered Hello without a name")
	}
	c.name, _ = reply[0].(string)
	return c, nil
}

// authenticate runs the SASL exchange of the specification for EXTERNAL: the
// leading NUL byte, AUTH with the user id in hex, OK with the server's GUID,
// BEGIN. The bus checks the id against the credentials of the socket, so
// there is nothing secret in it.
func authenticate(w io.Writer, r *bufio.Reader, uid int) error {
	id := hex.EncodeToString([]byte(strconv.Itoa(uid)))
	if _, err := io.WriteString(w, "\x00AUTH EXTERNAL "+id+"\r\n"); err != nil {
		return fmt.Errorf("gift/dbus: D-Bus authentication: %w", err)
	}
	line, err := r.ReadSlice('\n')
	if err != nil {
		return fmt.Errorf("gift/dbus: D-Bus authentication: %w", err)
	}
	reply := strings.TrimRight(string(line), "\r\n")
	switch {
	case strings.HasPrefix(reply, "OK "):
	case strings.HasPrefix(reply, "REJECTED"):
		return fmt.Errorf("gift/dbus: the bus rejected EXTERNAL authentication (%s)", reply)
	default:
		return fmt.Errorf("gift/dbus: unexpected D-Bus authentication reply %q", reply)
	}
	if _, err := io.WriteString(w, "BEGIN\r\n"); err != nil {
		return fmt.Errorf("gift/dbus: D-Bus authentication: %w", err)
	}
	return nil
}

// UniqueName is the name the bus assigned to this connection in Hello, such
// as ":1.42".
func (c *Conn) UniqueName() string { return c.name }

// SetSignalHandler installs the function every signal is handed to. It runs
// on the reader goroutine; see [Conn]. Signals arrive only for the match
// rules the connection added with the bus's AddMatch, and those addressed
// to it.
func (c *Conn) SetSignalHandler(fn func(*Message)) {
	c.onSignal.Store(&fn)
}

// SetReceiveLimit bounds the messages the connection decodes to n bytes,
// which is at most the 128 MiB of the specification and that by default. A
// longer message is read past and not decoded; a method call among them is
// answered with an error. A connection that expects nothing but small
// messages sets it low, so that no peer on the bus can make the process
// decode one of a hundred megabytes, which takes about ten times as much
// memory as values as it does on the wire.
func (c *Conn) SetReceiveLimit(n int) {
	c.limit.Store(int64(min(max(n, 16), maxMessageBytes)))
}

// Call sends a method call and waits for its reply, at most the timeout
// given to [Dial]. An error reply is returned as an [*Error].
func (c *Conn) Call(dest string, path ObjectPath, iface, member string, sig Signature, args ...any) ([]any, error) {
	ch := make(chan *Message, 1)
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return nil, err
	}
	serial := c.nextSerialLocked()
	c.pending[serial] = ch
	c.mu.Unlock()

	m := &Message{Type: TypeMethodCall, Serial: serial, Path: path, Interface: iface, Member: member,
		Destination: dest, Sig: sig, Body: args}
	if err := c.send(m); err != nil {
		c.forget(serial)
		return nil, err
	}

	t := time.NewTimer(c.timeout)
	defer t.Stop()
	select {
	case r, ok := <-ch:
		if !ok {
			c.mu.Lock()
			err := c.err
			c.mu.Unlock()
			return nil, err
		}
		if r.Type == TypeError {
			e := &Error{Name: r.ErrorName}
			if len(r.Body) > 0 {
				e.Message, _ = r.Body[0].(string)
			}
			return nil, e
		}
		return r.Body, nil
	case <-t.C:
		c.forget(serial)
		return nil, fmt.Errorf("gift/dbus: %s.%s got no reply within %v", iface, member, c.timeout)
	}
}

// Emit sends a signal from the object at path. It does not wait: a signal
// has no reply, and the bus delivers it to whoever subscribed.
func (c *Conn) Emit(path ObjectPath, iface, member string, sig Signature, args ...any) error {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	serial := c.nextSerialLocked()
	c.mu.Unlock()
	return c.send(&Message{Type: TypeSignal, Serial: serial, Path: path, Interface: iface, Member: member,
		Sig: sig, Body: args})
}

// nextSerialLocked returns the serial of the next message. Zero is not a
// serial, so the counter skips it when it wraps. c.mu is held.
func (c *Conn) nextSerialLocked() uint32 {
	c.serial++
	if c.serial == 0 {
		c.serial = 1
	}
	return c.serial
}

func (c *Conn) forget(serial uint32) {
	c.mu.Lock()
	delete(c.pending, serial)
	c.mu.Unlock()
}

// send writes one message. A write that fails leaves the stream in an
// unknown state, so it ends the connection.
func (c *Conn) send(m *Message) error {
	b, err := m.Marshal()
	if err != nil {
		return err
	}
	return c.write(b)
}

// write sends one marshalled message.
func (c *Conn) write(b []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.nc.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.nc.Write(b); err != nil {
		err = fmt.Errorf("gift/dbus: writing to the bus: %w", err)
		c.fail(err)
		return err
	}
	return nil
}

func (c *Conn) readLoop() {
	for {
		m, err := readMessage(c.r, func() int { return int(c.limit.Load()) })
		if err != nil && m != nil && errors.Is(err, ErrUndecodable) {
			c.undecodable(m, err)
			continue
		}
		if err != nil {
			c.fail(fmt.Errorf("gift/dbus: the D-Bus connection is gone: %w", err))
			return
		}
		switch m.Type {
		case TypeMethodReturn, TypeError:
			c.mu.Lock()
			ch := c.pending[m.ReplySerial]
			delete(c.pending, m.ReplySerial)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		case TypeSignal:
			if fn := c.onSignal.Load(); fn != nil {
				(*fn)(m)
			}
		case TypeMethodCall:
			c.dispatch(m)
		}
	}
}

// undecodable handles a message that was read but cannot be used: a call
// is answered with an error, if it wants an answer and says who sent it, a
// reply is handed to its call as an error, so that the call does not wait
// for its timeout, and a signal is dropped. The connection goes on.
func (c *Conn) undecodable(m *Message, err error) {
	if !m.Header() {
		return
	}
	switch m.Type {
	case TypeMethodCall:
		if m.Flags&FlagNoReplyExpected == 0 {
			c.reply(m, "", nil, &Error{Name: ErrorInvalidArgs, Message: err.Error()})
		}
	case TypeMethodReturn, TypeError:
		c.mu.Lock()
		ch := c.pending[m.ReplySerial]
		delete(c.pending, m.ReplySerial)
		c.mu.Unlock()
		if ch != nil {
			ch <- &Message{Type: TypeError, ReplySerial: m.ReplySerial, ErrorName: ErrorFailed,
				Body: []any{err.Error()}}
		}
	}
}

// fail ends the connection with err, once, and wakes every call that waits.
func (c *Conn) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
		for s, ch := range c.pending {
			close(ch)
			delete(c.pending, s)
		}
	}
	c.mu.Unlock()
	c.nc.Close()
}

// Close ends the connection. Every call that waits returns [ErrClosed], and
// the bus forgets the names and the match rules of the connection, which is
// how a name requested with RequestName is given up when a program ends
// without releasing it.
func (c *Conn) Close() {
	c.fail(ErrClosed)
}
