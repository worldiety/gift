package notify

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
// reader goroutine that matches replies to calls and hands signals on. Like
// wire.go it carries no build tag, and for the same reason; the tests drive it
// over net.Pipe against a bus that lives in the test.
//
// # What it does not do
//
// It connects to Unix domain sockets only and authenticates with EXTERNAL
// only, which is what every session bus on Linux offers. TCP buses, the
// DBUS_COOKIE_SHA1 mechanism, autolaunch and the passing of file descriptors
// are left out: no notification server needs them, and a session bus that
// cannot be reached this way is reported as "no notifications" rather than
// worked around.

const (
	busName      = "org.freedesktop.DBus"
	busPath      = objectPath("/org/freedesktop/DBus")
	busInterface = "org.freedesktop.DBus"
)

// errClosed is the error of every call after the connection was closed on
// purpose.
var errClosed = errors.New("gift/notify: the D-Bus connection is closed")

// busError is an error reply: the error name the peer sent and the message
// that came with it, if any.
type busError struct {
	name, message string
}

func (e *busError) Error() string {
	if e.message == "" {
		return "gift/notify: " + e.name
	}
	return "gift/notify: " + e.name + ": " + e.message
}

// sessionBusAddress finds the address of the session bus the way libdbus
// does, minus autolaunch: the environment variable, or the socket systemd
// places in the runtime directory.
func sessionBusAddress(getenv func(string) string, exists func(string) bool) (string, error) {
	if a := getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a, nil
	}
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		p := strings.TrimSuffix(dir, "/") + "/bus"
		if exists(p) {
			return "unix:path=" + p, nil
		}
	}
	return "", errors.New("gift/notify: no session bus, neither DBUS_SESSION_BUS_ADDRESS nor $XDG_RUNTIME_DIR/bus is set")
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
			return nil, fmt.Errorf("gift/notify: D-Bus address %q has no transport", entry)
		}
		kv := map[string]string{}
		for _, p := range strings.Split(params, ",") {
			if p == "" {
				continue
			}
			k, v, ok := strings.Cut(p, "=")
			if !ok {
				return nil, fmt.Errorf("gift/notify: D-Bus address %q has a key without a value", entry)
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
		return nil, fmt.Errorf("gift/notify: D-Bus address %q has no Unix socket this package can connect to", s)
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
			return "", fmt.Errorf("gift/notify: D-Bus address value %q ends in an escape", v)
		}
		x, err := hex.DecodeString(v[i+1 : i+3])
		if err != nil {
			return "", fmt.Errorf("gift/notify: D-Bus address value %q has a bad escape", v)
		}
		b.WriteByte(x[0])
		i += 2
	}
	return b.String(), nil
}

// conn is an authenticated connection to a bus.
//
// Calls may be made from any goroutine. The reader goroutine delivers replies
// to the call waiting for them and hands every signal to the handler, on
// itself, so a handler must not block and must not make a call on this
// connection.
type conn struct {
	nc      net.Conn
	r       *bufio.Reader
	timeout time.Duration

	// wmu serialises writes, so that two messages never interleave.
	wmu sync.Mutex

	mu      sync.Mutex
	serial  uint32
	pending map[uint32]chan *message
	err     error

	onSignal atomic.Pointer[func(*message)]

	// name is the unique name the bus assigned in Hello.
	name string
}

// dialBus connects to the first entry of address that accepts, authenticates
// and says Hello. timeout bounds each step and, afterwards, each call.
func dialBus(address string, timeout time.Duration) (*conn, error) {
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
func newConn(nc net.Conn, uid int, timeout time.Duration) (*conn, error) {
	r := bufio.NewReader(nc)
	_ = nc.SetDeadline(time.Now().Add(timeout))
	if err := authenticate(nc, r, uid); err != nil {
		nc.Close()
		return nil, err
	}
	_ = nc.SetDeadline(time.Time{})

	c := &conn{nc: nc, r: r, timeout: timeout, pending: map[uint32]chan *message{}}
	go c.readLoop()
	reply, err := c.call(busName, busPath, busInterface, "Hello", "")
	if err != nil {
		c.close()
		return nil, err
	}
	if len(reply) != 1 {
		c.close()
		return nil, errors.New("gift/notify: the bus answered Hello without a name")
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
		return fmt.Errorf("gift/notify: D-Bus authentication: %w", err)
	}
	line, err := r.ReadSlice('\n')
	if err != nil {
		return fmt.Errorf("gift/notify: D-Bus authentication: %w", err)
	}
	reply := strings.TrimRight(string(line), "\r\n")
	switch {
	case strings.HasPrefix(reply, "OK "):
	case strings.HasPrefix(reply, "REJECTED"):
		return fmt.Errorf("gift/notify: the bus rejected EXTERNAL authentication (%s)", reply)
	default:
		return fmt.Errorf("gift/notify: unexpected D-Bus authentication reply %q", reply)
	}
	if _, err := io.WriteString(w, "BEGIN\r\n"); err != nil {
		return fmt.Errorf("gift/notify: D-Bus authentication: %w", err)
	}
	return nil
}

// setSignalHandler installs the function every signal is handed to. It runs
// on the reader goroutine; see [conn].
func (c *conn) setSignalHandler(fn func(*message)) {
	c.onSignal.Store(&fn)
}

// call sends a method call and waits for its reply, at most c.timeout. An
// error reply is returned as a *busError.
func (c *conn) call(dest string, path objectPath, iface, member string, sig signature, args ...any) ([]any, error) {
	ch := make(chan *message, 1)
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return nil, err
	}
	c.serial++
	if c.serial == 0 {
		c.serial = 1
	}
	serial := c.serial
	c.pending[serial] = ch
	c.mu.Unlock()

	m := &message{typ: msgMethodCall, serial: serial, path: path, iface: iface, member: member,
		destination: dest, sig: sig, body: args}
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
		if r.typ == msgError {
			e := &busError{name: r.errorName}
			if len(r.body) > 0 {
				e.message, _ = r.body[0].(string)
			}
			return nil, e
		}
		return r.body, nil
	case <-t.C:
		c.forget(serial)
		return nil, fmt.Errorf("gift/notify: %s.%s got no reply within %v", iface, member, c.timeout)
	}
}

func (c *conn) forget(serial uint32) {
	c.mu.Lock()
	delete(c.pending, serial)
	c.mu.Unlock()
}

// send writes one message. A write that fails leaves the stream in an
// unknown state, so it ends the connection.
func (c *conn) send(m *message) error {
	b, err := m.marshal()
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.nc.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.nc.Write(b); err != nil {
		err = fmt.Errorf("gift/notify: writing to the bus: %w", err)
		c.fail(err)
		return err
	}
	return nil
}

func (c *conn) readLoop() {
	for {
		m, err := readMessage(c.r)
		if err != nil {
			c.fail(fmt.Errorf("gift/notify: the D-Bus connection is gone: %w", err))
			return
		}
		switch m.typ {
		case msgMethodReturn, msgError:
			c.mu.Lock()
			ch := c.pending[m.replySerial]
			delete(c.pending, m.replySerial)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		case msgSignal:
			if fn := c.onSignal.Load(); fn != nil {
				(*fn)(m)
			}
		case msgMethodCall:
			c.answer(m)
		}
	}
}

// answer replies to a method call addressed to this connection. The only
// one a bus or a peer has reason to make is Peer.Ping; everything else is
// refused with the standard error, so that a caller is not left waiting.
func (c *conn) answer(m *message) {
	if m.flags&flagNoReplyExpected != 0 {
		return
	}
	r := &message{destination: m.sender, replySerial: m.serial}
	if m.iface == "org.freedesktop.DBus.Peer" && m.member == "Ping" {
		r.typ = msgMethodReturn
	} else {
		r.typ = msgError
		r.errorName = "org.freedesktop.DBus.Error.UnknownMethod"
		r.sig = "s"
		r.body = []any{fmt.Sprintf("gift/notify does not implement %s.%s", m.iface, m.member)}
	}
	c.mu.Lock()
	c.serial++
	if c.serial == 0 {
		c.serial = 1
	}
	r.serial = c.serial
	c.mu.Unlock()
	_ = c.send(r)
}

// fail ends the connection with err, once, and wakes every call that waits.
func (c *conn) fail(err error) {
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

func (c *conn) close() {
	c.fail(errClosed)
}
