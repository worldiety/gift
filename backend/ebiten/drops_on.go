//go:build (darwin && !ios) || windows || (linux && !android)

package ebiten

import eb "github.com/hajimehoshi/ebiten/v2"

// droppedFiles is Ebitengine's own report of dropped files, on the three
// desktop systems gift drops into; see drop.go.
var droppedFiles = eb.DroppedFiles
