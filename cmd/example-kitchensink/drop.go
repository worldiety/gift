package main

import (
	"fmt"
	"strings"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
)

func init() {
	own = append(own, page{
		Demo: showcase.Demo{
			Name: "drop", Group: "system", Title: "Drag and Drop",
			Summary: "Files, text and web addresses dragged in from other applications.",
			Width:   560, Height: 420,
		},
		build:  dropPage,
		source: "cmd/example-kitchensink/drop.go",
	})
}

// dropPage is a drop destination for every kind, which says what this
// platform can drop, whether a drag is over it, and what arrived last.
func dropPage(ctx *gift.Context) gift.View {
	last := ctx.State("last", "Nothing dropped yet.")
	over := ctx.State("over", false)
	caps := ctx.DropCapabilities()

	what := "This platform drops nothing into the application."
	if caps.Kinds != 0 {
		what = "Drops " + caps.Kinds.String() + "; " + map[bool]string{
			true:  "the area lights up while a drag is over it.",
			false: "nothing shows before the drop.",
		}[caps.Live]
	}
	status := "No drag over the area."
	if ctx.Read(over) {
		status = "A drag the area would take is over it."
	}

	return ui.VStack(
		ui.Text(what).Foreground(ui.ColorSecondaryLabel),
		ui.DropDestination(
			ui.VStack(
				ui.Icon(outline.FileImport).Size(32).Foreground(ui.ColorAccent),
				ui.Text("Drag files, text or a link here"),
				ui.Text(status).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			).Gap(8).Align(geom.Alignment{X: 0.5, Y: 0.5}).Padding(24).Frame(480, 180).
				Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).CornerRadius(14),
		).CornerRadius(14).OnTargeted(over.Set).OnDrop(func(d ui.DropInfo) bool {
			var lines []string
			for _, it := range d.Items {
				switch it.Kind {
				case gift.DropFile:
					lines = append(lines, "File: "+it.Path)
				case gift.DropURL:
					lines = append(lines, "Address: "+it.URL)
				case gift.DropText:
					lines = append(lines, fmt.Sprintf("Text: %d characters", len([]rune(it.Text))))
				}
			}
			last.Set(strings.Join(lines, "\n"))
			return true
		}),
		ui.Text(ctx.Read(last)),
	).Gap(16).Padding(24)
}
