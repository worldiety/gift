//go:build unix

package web

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// exclusive takes back the SO_REUSEADDR that Go sets on every listener here,
// which RFC 8252 appendix B.3 asks a loopback redirect server not to have.
func exclusive(_, _ string, c syscall.RawConn) error {
	var serr error
	if err := c.Control(func(fd uintptr) {
		serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 0)
	}); err != nil {
		return err
	}
	return serr
}
