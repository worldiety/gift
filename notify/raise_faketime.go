//go:build linux && !android && faketime

package notify

import "log/slog"

// newRaiser raises nothing under the faketime tag, whose purego has no dlopen
// to load libX11 with; see raise_x11.go.
func newRaiser(*slog.Logger) func(token string) { return nil }
