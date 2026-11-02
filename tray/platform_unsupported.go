//go:build (!linux && !darwin && !windows) || android || ios

package tray

import (
	"fmt"
	"runtime"
)

// openPlatform is the platform layer of every operating system this package
// does not support: the BSDs, Android, iOS and js/wasm. It fails to open, the
// portable half logs that once, and every call after it returns
// [ErrUnsupported] without displaying anything.
func openPlatform(openConfig) (platform, error) {
	return nil, fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}
