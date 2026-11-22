//go:build !(linux && !android) && !windows

package ebiten

import "log/slog"

// newDisplayReader has nothing to read here: macOS and iOS convert into the
// display themselves, and the other systems Ebitengine runs on do not say.
func newDisplayReader(*slog.Logger) displayReader { return nil }
