package main

import (
	"strings"
	"testing/fstest"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/filepicker"
)

func init() {
	// The input taps two pictures once the listing is there, so the
	// screenshot shows a selection and the primary action enabled.
	register(Demo{Name: "file-picker", Group: "dialogs", Width: 600, Height: 760, FullWindow: true, Input: `{"steps":[
		{"op":"waitFrames","frames":30},
		{"op":"tap","x":300,"y":471},
		{"op":"waitFrames","frames":2},
		{"op":"tap","x":300,"y":521},
		{"op":"waitFrames","frames":10}
	]}`}, filePickerDemo)
}

// files is the file system the demo browses: a few folders and files in
// memory, so the screenshot does not depend on the machine it is taken on.
var files = func() fstest.MapFS {
	day := func(d int) time.Time { return time.Date(2026, time.June, d, 10, 0, 0, 0, time.UTC) }
	file := func(size, d int) *fstest.MapFile {
		return &fstest.MapFile{Data: make([]byte, size), ModTime: day(d)}
	}
	return fstest.MapFS{
		"Photos/Lisbon/tram.jpg":         file(2_412_000, 14),
		"Photos/Lisbon/harbor.jpg":       file(3_105_000, 14),
		"Photos/Lisbon/alfama.png":       file(5_870_000, 15),
		"Photos/Lisbon/notes.txt":        file(1_240, 15),
		"Photos/Lisbon/sunset.mov":       file(48_300_000, 16),
		"Photos/Lisbon/Favorites/a.jpg":  file(1_900_000, 16),
		"Photos/Lisbon/Raw/IMG_0001.dng": file(24_000_000, 14),
		"Documents/report.pdf":           file(812_000, 3),
	}
}()

func filePickerDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// The application owns whether the picker is up and what was picked.
	picking := ctx.State("picking", true)
	picked := ctx.State("picked", "Nothing picked yet.")

	screen := ui.VStack(
		ui.Text("Album").FontSize(22),
		ui.Text(ctx.Read(picked)).Foreground(ui.ColorSecondaryLabel),
		ui.Button(ui.Text("Add photos"), func() { picking.Set(true) }).
			CornerRadius(8).MinHeight(ui.ControlHitTarget),
	).Gap(12).Padding(24)

	// Paths are fs.FS paths relative to the root of files. Pictures can be
	// picked; the other files are listed, disabled.
	picker := filepicker.New(files,
		func(paths []string) {
			picking.Set(false)
			picked.Set("Picked " + strings.Join(paths, ", "))
		},
		func() { picking.Set(false) },
	).Mode(filepicker.OpenMulti).
		Extensions(".jpg", ".png").
		Start("Photos/Lisbon").
		Key("picker")

	return ui.Sheet(screen, picker).
		Height(0.78).
		Presented(ctx.Read(picking)).
		OnDismiss(func() { picking.Set(false) })
	// snippet:end
}
