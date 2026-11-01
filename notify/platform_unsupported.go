//go:build (!linux && !darwin && !windows) || android

package notify

import (
	"fmt"
	"runtime"
)

// openPlatform is the platform layer of every operating system this package
// does not support: the BSDs, Android, iOS and js/wasm.
// It fails to open, the portable half logs that once, and every Show after it
// returns [ErrUnsupported] without displaying anything.
func openPlatform(openConfig) (platform, error) {
	return nil, fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}

func unregisterPlatform(Config) error { return nil }
