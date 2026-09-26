//go:build !(darwin || freebsd || linux)

package turbojpeg

import "image"

// load always fails where purego cannot dlopen. Windows could load
// turbojpeg.dll through syscall.LoadDLL, but gift targets Raspberry Pi OS and
// macOS, and a code path nobody runs is a code path nobody tests.
func load() error { return ErrUnavailable }

func scales() []scale { return fallbackScales }

func decompress([]byte, int, int) (*image.RGBA, error) { return nil, ErrUnavailable }
