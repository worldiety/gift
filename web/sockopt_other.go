//go:build !unix && !windows

package web

import "syscall"

func exclusive(_, _ string, _ syscall.RawConn) error { return nil }
