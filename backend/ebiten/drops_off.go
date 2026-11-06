//go:build !((darwin && !ios) || windows || (linux && !android))

package ebiten

import "io/fs"

// droppedFiles is nil where gift drops nothing. Ebitengine in a browser
// reports drops too; they are deliberately not read, because the project
// plan, section 49, names macOS, Windows, GNOME and KDE and nothing else.
var droppedFiles func() fs.FS
