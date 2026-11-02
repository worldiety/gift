//go:build unix

package web

import (
	"net"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLoopbackHasNoAddressReuse(t *testing.T) {
	ln, err := listenLoopback(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if ip := ln.Addr().(*net.TCPAddr).IP; !ip.IsLoopback() {
		t.Fatalf("listening on %v", ip)
	}
	rc, err := ln.(*net.TCPListener).SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var v int
	var gerr error
	if err := rc.Control(func(fd uintptr) { v, gerr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR) }); err != nil {
		t.Fatal(err)
	}
	if gerr != nil || v != 0 {
		t.Fatalf("SO_REUSEADDR = %d, %v", v, gerr)
	}
	// A plain listener of Go has it, which is what exclusive takes back.
	plain, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	rc, _ = plain.(*net.TCPListener).SyscallConn()
	_ = rc.Control(func(fd uintptr) { v, _ = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR) })
	if v == 0 {
		t.Log("Go no longer sets SO_REUSEADDR on this platform")
	}
}
