//go:build (!linux && !darwin && !windows) || android || ios

package filedialog

import (
	"fmt"
	"runtime"
)

// platformAvailable is false on every system this package does not support:
// the BSDs, Android, iOS and js/wasm.
func platformAvailable() bool { return false }

// openPlatform fails; nothing is ever shown.
func openPlatform(openConfig) (platform, error) {
	return nil, permanent(fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS))
}
