package dbus

import (
	"bufio"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// These tests run a bus inside the test, at the other end of a net.Pipe. It
// authenticates the way dbus-daemon does, answers Hello, and hands every other
// call to the test. What it proves is the client's side of the protocol; that
// a real dbus-daemon agrees is what notify/notify_linux_test.go and
// tray/platform_linux_test.go check, on request.

// The notification server stands in for every peer the tests call; its names
// are the ones gift/notify uses, which is where these tests came from.
const (
	notifyName      = "org.freedesktop.Notifications"
	notifyPath      = ObjectPath("/org/freedesktop/Notifications")
	notifyInterface = "org.freedesktop.Notifications"
)

// testBus is the server end.
type testBus struct {
	t      *testing.T
	nc     net.Conn
	r      *bufio.Reader
	serial uint32

	// out feeds the writer goroutine. net.Pipe has no buffer, so a bus that
	// wrote on its own goroutine would deadlock against a client that writes
	// an answer while the bus writes a reply; a real socket buffers both.
	out chan []byte

	// authLine is the AUTH line the client sent, without the leading NUL.
	authLine string
	// reject makes the bus refuse the authentication.
	reject bool
	// handle answers a method call other than Hello; nil means no answer.
	handle func(m *Message) *Message
}

// startTestBus returns a connection to a test bus, authenticated as uid 1000.
func startTestBus(t *testing.T, bus *testBus, timeout time.Duration) (*Conn, error) {
	t.Helper()
	client, server := net.Pipe()
	bus.t, bus.nc, bus.r = t, server, bufio.NewReader(server)
	bus.out = make(chan []byte, 64)
	done := make(chan struct{})
	go func() {
		for b := range bus.out {
			server.Write(b)
		}
	}()
	go func() {
		defer close(done)
		defer close(bus.out)
		bus.serve()
	}()
	t.Cleanup(func() {
		server.Close()
		<-done
	})
	return newConn(client, 1000, timeout)
}

func (b *testBus) serve() {
	nul, err := b.r.ReadByte()
	if err != nil || nul != 0 {
		return
	}
	line, err := b.r.ReadString('\n')
	if err != nil {
		return
	}
	b.authLine = strings.TrimRight(line, "\r\n")
	if b.reject {
		io.WriteString(b.nc, "REJECTED EXTERNAL\r\n")
		return
	}
	io.WriteString(b.nc, "OK 0123456789abcdef0123456789abcdef\r\n")
	if line, err := b.r.ReadString('\n'); err != nil || line != "BEGIN\r\n" {
		return
	}
	for {
		m, err := ReadMessage(b.r)
		if err != nil {
			return
		}
		var reply *Message
		switch {
		case m.Type != TypeMethodCall:
			if b.handle != nil {
				b.handle(m)
			}
			continue
		case m.Member == "Hello":
			reply = &Message{Type: TypeMethodReturn, ReplySerial: m.Serial, Sig: "s", Body: []any{":1.7"}}
		case b.handle != nil:
			reply = b.handle(m)
		}
		if reply != nil {
			b.send(reply)
		}
	}
}

func (b *testBus) send(m *Message) {
	b.serial++
	m.Serial = b.serial
	buf, err := m.Marshal()
	if err != nil {
		b.t.Errorf("test bus cannot marshal %+v: %v", m, err)
		return
	}
	b.out <- buf
}

func TestAuthenticationAndHello(t *testing.T) {
	bus := &testBus{}
	c, err := startTestBus(t, bus, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if want := "AUTH EXTERNAL " + hex.EncodeToString([]byte("1000")); bus.authLine != want {
		t.Fatalf("auth line %q, want %q", bus.authLine, want)
	}
	if c.name != ":1.7" {
		t.Fatalf("unique name %q, want :1.7", c.name)
	}
}

func TestARejectedAuthenticationIsAnError(t *testing.T) {
	_, err := startTestBus(t, &testBus{reject: true}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("error %v, want a rejection", err)
	}
}

func TestAnErrorReplyBecomesABusError(t *testing.T) {
	bus := &testBus{handle: func(m *Message) *Message {
		return &Message{Type: TypeError, ReplySerial: m.Serial, ErrorName: "org.freedesktop.DBus.Error.ServiceUnknown",
			Sig: "s", Body: []any{"nobody owns it"}}
	}}
	c, err := startTestBus(t, bus, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.Call(notifyName, notifyPath, notifyInterface, "GetCapabilities", "")
	var be *Error
	if !errors.As(err, &be) || be.Name != "org.freedesktop.DBus.Error.ServiceUnknown" || be.Message != "nobody owns it" {
		t.Fatalf("error %#v", err)
	}
}

// TestACallWithoutAReplyTimesOutAndLeavesNothingBehind: the bus ignores the
// first call. The caller must get its timeout, and the next call must still
// work, which it would not if the pending slot leaked or the stream were out
// of step.
func TestACallWithoutAReplyTimesOutAndLeavesNothingBehind(t *testing.T) {
	calls := 0
	bus := &testBus{handle: func(m *Message) *Message {
		calls++
		if calls == 1 {
			return nil
		}
		return &Message{Type: TypeMethodReturn, ReplySerial: m.Serial, Sig: "u", Body: []any{uint32(42)}}
	}}
	c, err := startTestBus(t, bus, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Call(notifyName, notifyPath, notifyInterface, "Notify", ""); err == nil || !strings.Contains(err.Error(), "no reply") {
		t.Fatalf("first call: %v, want a timeout", err)
	}
	reply, err := c.Call(notifyName, notifyPath, notifyInterface, "Notify", "")
	if err != nil || len(reply) != 1 || reply[0] != uint32(42) {
		t.Fatalf("second call: %v %v", reply, err)
	}
	c.mu.Lock()
	n := len(c.pending)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d calls still pending", n)
	}
}

func TestSignalsReachTheHandler(t *testing.T) {
	bus := &testBus{}
	bus.handle = func(m *Message) *Message {
		bus.send(&Message{Type: TypeSignal, Path: notifyPath, Interface: notifyInterface, Member: "ActionInvoked",
			Sig: "us", Body: []any{uint32(5), "default"}})
		return &Message{Type: TypeMethodReturn, ReplySerial: m.Serial}
	}
	c, err := startTestBus(t, bus, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	got := make(chan *Message, 1)
	c.SetSignalHandler(func(m *Message) { got <- m })
	if _, err := c.Call(notifyName, notifyPath, notifyInterface, "Notify", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m.Member != "ActionInvoked" || m.Body[0] != uint32(5) || m.Body[1] != "default" {
			t.Fatalf("signal %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("the signal did not arrive")
	}
}

// TestALostConnectionWakesTheCaller: a bus that goes away while a call waits
// must not leave the caller waiting for the timeout, and later calls must fail
// at once.
func TestALostConnectionWakesTheCaller(t *testing.T) {
	bus := &testBus{}
	bus.handle = func(m *Message) *Message {
		bus.nc.Close()
		return nil
	}
	c, err := startTestBus(t, bus, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := c.Call(notifyName, notifyPath, notifyInterface, "Notify", ""); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("error %v, want the connection to be gone", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("the caller waited %v", d)
	}
	if _, err := c.Call(notifyName, notifyPath, notifyInterface, "Notify", ""); err == nil {
		t.Fatal("a call on a dead connection succeeded")
	}
}

// TestPingIsAnswered: a bus or a peer may ping any connection, and a
// connection that never answers looks hung to it.
func TestPingIsAnswered(t *testing.T) {
	bus := &testBus{}
	answered := make(chan *Message, 1)
	bus.handle = func(m *Message) *Message {
		switch m.Type {
		case TypeMethodCall: // the test's trigger call
			bus.send(&Message{Type: TypeMethodCall, Path: "/", Interface: "org.freedesktop.DBus.Peer", Member: "Ping",
				Sender: ":1.1", Destination: ":1.7"})
			return &Message{Type: TypeMethodReturn, ReplySerial: m.Serial}
		default:
			answered <- m
			return nil
		}
	}
	c, err := startTestBus(t, bus, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Call(notifyName, notifyPath, notifyInterface, "Trigger", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-answered:
		if m.Type != TypeMethodReturn || m.Destination != ":1.1" || m.ReplySerial == 0 {
			t.Fatalf("answer %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("Ping was not answered")
	}
}

func TestParseBusAddress(t *testing.T) {
	got, err := parseBusAddress("tcp:host=localhost,port=1;unix:path=/run/user/1000/bus,guid=abc;unix:abstract=/tmp/dbus-x%2cy")
	if err != nil {
		t.Fatal(err)
	}
	want := []busAddr{{"unix", "/run/user/1000/bus"}, {"unix", "@/tmp/dbus-x,y"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, bad := range []string{"", "tcp:host=x", "unix:path=/a%2", "unix:path=/a%zz", "nocolon"} {
		if _, err := parseBusAddress(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestSessionBusAddress(t *testing.T) {
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	exists := func(p string) bool { return p == "/run/user/1000/bus" }

	if _, err := SessionBusAddress(getenv, exists); err == nil {
		t.Error("an empty environment produced an address")
	}
	env["XDG_RUNTIME_DIR"] = "/run/user/1000/"
	if a, err := SessionBusAddress(getenv, exists); err != nil || a != "unix:path=/run/user/1000/bus" {
		t.Errorf("runtime dir: %q %v", a, err)
	}
	env["DBUS_SESSION_BUS_ADDRESS"] = "unix:abstract=/tmp/x"
	if a, _ := SessionBusAddress(getenv, exists); a != "unix:abstract=/tmp/x" {
		t.Errorf("the environment variable lost against the runtime dir: %q", a)
	}
}
