package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/filedialog"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/filepicker"
)

func init() {
	own = append(own, page{
		Demo: showcase.Demo{
			Name: "file-dialog", Group: "system", Title: "File dialog",
			Summary: "The system's own open, folder and save dialog, or the in-app picker where there is none.",
			Width:   560, Height: 460,
		},
		build:  fileDialogPage,
		source: "cmd/example-kitchensink/filedialog.go",
	})
}

// fileDialogPage opens the system's file dialog, the in-app picker, or the
// first with the second as its fallback, in the mode chosen, under the home
// directory, and shows what was picked.
func fileDialogPage(ctx *gift.Context) gift.View {
	mode := ctx.State("mode", 0)
	way := ctx.State("way", 2)
	picking := ctx.State("picking", false)
	status := ctx.State("status", "Nothing picked yet.")

	root, err := os.UserHomeDir()
	if err != nil {
		root = "."
	}
	m := filedialog.Mode(ctx.Read(mode))
	pick := func(paths []string) {
		picking.Set(false)
		status.Set("Picked: " + strings.Join(paths, ", "))
	}
	cancel := func() {
		picking.Set(false)
		status.Set("Cancelled.")
	}

	var exts []string
	if m == filedialog.Open || m == filedialog.OpenMulti {
		exts = []string{".jpg", ".jpeg", ".png", ".heic"}
	}
	picker := filepicker.New(os.DirFS(root), pick, cancel).Mode(m).Extensions(exts...).SuggestedName("untitled.txt")
	dlg := filedialog.New(pick, cancel).Mode(m).Extensions(exts...).Root(root).
		FilterName("Pictures").SuggestedName("untitled.txt")

	native := "Native dialogs are available here."
	if !filedialog.Available() {
		native = "No native dialog here; the fallback shows the in-app picker."
	}
	controls := ui.VStack(
		ui.SegmentedControl(ctx.Read(mode), []string{"Open", "Open several", "Folder", "Save"}, mode.Set).Key("file-dialog-mode"),
		ui.SegmentedControl(ctx.Read(way), []string{"Native", "In-app", "Native, else in-app"}, way.Set).Key("file-dialog-way"),
		ui.Button(ui.Text("Choose…"), func() { picking.Set(true) }).Key("file-dialog-choose"),
		ui.Text(ctx.Read(status)).FontSize(13).Foreground(ui.ColorSecondaryLabel).Key("file-dialog-status"),
		ui.Text(fmt.Sprintf("%s Paths are relative to %s.", native, root)).
			FontSize(12).Foreground(ui.ColorSecondaryLabel),
	).Gap(14).Frame(440, geom.Unbounded())

	// A sheet is a screen and fills the area it is given, so the page gives
	// it one of the size of the stage.
	presented := ctx.Read(picking)
	var v gift.View
	switch ctx.Read(way) {
	case 0:
		v = filedialog.Present(controls, dlg).Presented(presented)
	case 1:
		v = ui.Sheet(controls, picker).Presented(presented).OnDismiss(cancel)
	default:
		v = filedialog.Present(controls, dlg).Fallback(picker).Presented(presented)
	}
	return ui.ZStack(v).Frame(540, 440)
}
