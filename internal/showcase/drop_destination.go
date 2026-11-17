package showcase

import (
	"path/filepath"
	"strings"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	// A drag of text held over the inner area, so the screenshot shows the
	// highlight; nothing is dropped.
	register(Demo{Name: "drop-destination", Group: "basic", Title: "Drop Destination", Summary: "Takes files, text and web addresses dragged in from other applications.", Width: 520, Height: 340,
		Input: `{"steps":[{"op":"waitFrames","frames":10},{"op":"dragOver","x":260,"y":182,"kinds":["text"]},{"op":"waitFrames","frames":4}]}`},
		dropDestinationDemo)
}

func dropDestinationDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	files := ctx.State("files", "No files yet.")
	note := ctx.State("note", "Drop text here")

	// An inner destination for text inside an outer one for files: a file
	// dragged over the note goes to the outer area, text to the note.
	noteArea := ui.DropDestination(
		ui.HStack(
			ui.Icon(outline.Clipboard).Size(18).Foreground(ui.ColorSecondaryLabel),
			ui.Text(ctx.Read(note)).Foreground(ui.ColorSecondaryLabel),
		).Gap(8).Align(geom.Center).Padding(14).
			Background(ui.ColorControl).CornerRadius(10),
	).Accepts(gift.DropText).CornerRadius(10).OnDrop(func(d ui.DropInfo) bool {
		note.Set(strings.TrimSpace(d.Items[0].Text))
		return true
	})

	return ui.DropDestination(
		ui.VStack(
			ui.Icon(outline.FileImport).Size(36).Foreground(ui.ColorAccent),
			ui.Text("Drop files here").FontSize(17),
			ui.Text(ctx.Read(files)).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			noteArea,
		).Gap(10).Align(geom.Alignment{X: 0.5, Y: 0.5}).Padding(24).Frame(460, 280).
			Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).CornerRadius(16),
	).Accepts(gift.DropFile).CornerRadius(16).OnDrop(func(d ui.DropInfo) bool {
		names := make([]string, 0, len(d.Items))
		for _, p := range d.Files() {
			names = append(names, filepath.Base(p))
		}
		files.Set(strings.Join(names, ", "))
		return true
	})
	// snippet:end
}
