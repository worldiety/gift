// Command example-drop takes files, text and web addresses dragged in from
// other applications, with ui.DropDestination.
//
//	go run ./cmd/example-drop
//
// The whole window is a destination for files and folders: drag some from the
// Finder, the Explorer, Nautilus or Dolphin, and the list shows each with its
// kind, its path and its size. The note in the middle takes text and
// addresses: drag a link from a browser or a selection from a text editor
// onto it. A file dragged over the note still goes to the window, which is
// the rule for nested destinations: the innermost one that accepts what is
// dragged.
//
// The line at the top says what this platform can drop, as
// gift.DropCapabilities reports it. On macOS and Windows the window lights up
// while a drag it would take is over it, and text and addresses arrive. On
// Linux files arrive with no highlight before the drop. Elsewhere nothing
// arrives, and the window says so.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/ui"
)

// record is one dropped thing as the list shows it.
type record struct {
	kind   string
	symbol ui.Symbol
	what   string
	detail string
}

// drops is the list, newest first. It is touched on the UI goroutine only, in
// OnDrop and in the build, and a state counter tells the build it changed.
var drops []record

const maxDrops = 50

func main() {
	if err := example.LoadFont(); err != nil {
		fmt.Fprintln(os.Stderr, "example-drop:", err)
		os.Exit(1)
	}
	app := gift.New(gift.Options{Root: root})
	if err := backend.Run(app, backend.Config{Title: "gift drop", Width: 640, Height: 520}); err != nil {
		fmt.Fprintln(os.Stderr, "example-drop:", err)
		os.Exit(1)
	}
}

// capabilities says in words what the platform can drop.
func capabilities(c gift.DropCapabilities) string {
	if c.Kinds == 0 {
		return "This platform drops nothing into the application."
	}
	var kinds []string
	for _, k := range []struct {
		k gift.DropKind
		s string
	}{{gift.DropFile, "files"}, {gift.DropText, "text"}, {gift.DropURL, "addresses"}} {
		if c.Kinds&k.k != 0 {
			kinds = append(kinds, k.s)
		}
	}
	s := "Drops " + strings.Join(kinds, ", ")
	if c.Live {
		return s + ", with a highlight while dragging."
	}
	return s + ", with no highlight before the drop."
}

func size(path string) string {
	fi, err := os.Stat(path)
	switch {
	case err != nil:
		return ""
	case fi.IsDir():
		return "folder"
	case fi.Size() < 1024:
		return fmt.Sprintf("%d bytes", fi.Size())
	case fi.Size() < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(fi.Size())/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(fi.Size())/(1024*1024))
}

func add(items []gift.DropItem) {
	for _, it := range items {
		var r record
		switch it.Kind {
		case gift.DropFile:
			r = record{kind: "File", symbol: outline.File, what: it.Path, detail: size(it.Path)}
			if it.Dir {
				r.kind, r.symbol = "Folder", outline.Folder
			}
		case gift.DropURL:
			r = record{kind: "Address", symbol: outline.Link, what: it.URL}
		case gift.DropText:
			r = record{kind: "Text", symbol: outline.Clipboard, what: strings.TrimSpace(it.Text),
				detail: fmt.Sprintf("%d characters", len([]rune(it.Text)))}
		}
		drops = append([]record{r}, drops...)
	}
	if len(drops) > maxDrops {
		drops = drops[:maxDrops]
	}
}

func root(ctx *gift.Context) gift.View {
	version := ctx.State("version", 0)
	ctx.Read(version)
	bump := func() { version.Set(version.Get() + 1) }

	// A row of its own rather than a ui.Row: a path is long, and it wraps
	// rather than losing the part that tells where the file is.
	rows := make([]gift.View, 0, len(drops))
	for _, r := range drops {
		rows = append(rows, ui.HStack(
			ui.Icon(r.symbol).Size(20).Foreground(ui.ColorSecondaryLabel),
			ui.VStack(
				ui.Text(r.what),
				ui.Text(r.kind).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			).Gap(2).Flex(1),
			ui.Text(r.detail).FontSize(13).Foreground(ui.ColorSecondaryLabel),
		).Gap(12).Align(geom.Center).Padding(8))
	}
	var list gift.View = ui.Text("Nothing dropped yet. Drag files onto the window.").
		Foreground(ui.ColorSecondaryLabel)
	if len(rows) > 0 {
		list = ui.VScroll(ui.VStack(rows...).Gap(4)).Flex(1)
	}

	note := ui.DropDestination(
		ui.HStack(
			ui.Icon(outline.Clipboard).Size(18).Foreground(ui.ColorSecondaryLabel),
			ui.Text("Drop text or an address here").Foreground(ui.ColorSecondaryLabel),
		).Gap(8).Align(geom.Center).Padding(14).Background(ui.ColorControl).CornerRadius(10),
	).Key("note").Accepts(gift.DropText | gift.DropURL).CornerRadius(10).
		OnDrop(func(d ui.DropInfo) bool { add(d.Items); bump(); return true })

	return ui.DropDestination(
		ui.Window(
			ui.VStack(
				ui.HStack(
					ui.Icon(outline.FileImport).Size(28).Foreground(ui.ColorAccent),
					ui.VStack(
						ui.Text("Drop files anywhere in this window").FontSize(17),
						ui.Text(capabilities(ctx.DropCapabilities())).FontSize(13).Foreground(ui.ColorSecondaryLabel),
					).Gap(2),
				).Gap(12).Align(geom.Center),
				note,
				list,
			).Gap(16).Padding(24),
		),
	).Key("window").Accepts(gift.DropFile).
		OnDrop(func(d ui.DropInfo) bool { add(d.Items); bump(); return true })
}
