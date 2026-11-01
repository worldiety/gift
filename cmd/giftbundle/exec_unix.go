//go:build unix

package main

import (
	"os"
	"syscall"
)

// execProgram replaces this process with exe, which keeps the terminal, the
// signals and the exit status the program's own.
func execProgram(exe string, args []string) error {
	return syscall.Exec(exe, append([]string{exe}, args...), os.Environ())
}
