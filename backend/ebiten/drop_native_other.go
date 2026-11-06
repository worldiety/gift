//go:build !(darwin && !ios) && !(windows && (amd64 || arm64))

package ebiten

import "log/slog"

// openNativeDrops is the native drag layer of the platform, and there is
// none here: Linux keeps Ebitengine's drop of files, and the other systems
// drop nothing. See drop.go.
func openNativeDrops(func(), *slog.Logger) nativeDrops { return nil }
