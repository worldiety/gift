//go:build windows

package web

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// soExclusiveAddrUse is SO_EXCLUSIVEADDRUSE, which winsock defines as the
// complement of SO_REUSEADDR.
const soExclusiveAddrUse = ^windows.SO_REUSEADDR

// exclusive sets SO_EXCLUSIVEADDRUSE, as RFC 8252 appendix B.2 asks of a
// loopback redirect server: without it, another process may bind the same
// port with SO_REUSEADDR and take the redirect.
func exclusive(_, _ string, c syscall.RawConn) error {
	var serr error
	if err := c.Control(func(fd uintptr) {
		serr = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, soExclusiveAddrUse, 1)
	}); err != nil {
		return err
	}
	return serr
}
