package main

import (
	"errors"
	"fmt"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/tray"
	"github.com/worldiety/gift/ui"
)

// menuBar is the icon of the tray page. It opens nothing until the page
// first shows it, and the page takes it out of the bar again when it is left.
var menuBar = tray.New(tray.Config{
	AppID:   "dev.gift.kitchensink",
	Icon:    outline.Star,
	Tooltip: "gift kitchen sink",
	Post: func(fn func()) {
		if App != nil {
			App.Post(fn)
		}
	},
})

func init() {
	own = append(own, page{
		Demo: showcase.Demo{
			Name: "tray", Group: "system", Title: "Tray",
			Summary: "An icon in the menu bar, the notification area or the status area of the panel, with a menu of its own.",
			Width:   560, Height: 460,
		},
		build:  trayPage,
		source: "cmd/example-kitchensink/tray.go",
	})
}

// trayPage puts an icon with a menu into the bar, with the tooltip of the
// field, a check mark that moves, a disabled entry and a submenu, and counts
// the clicks on the entries.
func trayPage(ctx *gift.Context) gift.View {
	tip := ui.Editor(ctx, "tooltip", "gift kitchen sink")
	status := ctx.State("status", "Nothing shown yet.")
	clicks := ctx.State("clicks", 0)
	checked := ctx.State("checked", false)

	// report turns the result of a call into the status line; an
	// application tells ErrUnsupported apart and may ignore it.
	report := func(done string, err error) {
		switch {
		case err == nil:
			status.Set(done)
		case errors.Is(err, tray.ErrUnsupported):
			status.Set("No tray here: " + err.Error())
		default:
			status.Set("Failed: " + err.Error())
		}
	}

	// count is the OnClick of an entry that only says it was clicked.
	count := func(what string) func() {
		return func() {
			clicks.Set(clicks.Get() + 1)
			status.Set("Clicked: " + what + ".")
		}
	}
	// set builds the menu from the states and replaces the one shown; a
	// click on Checked moves the check mark by setting it again.
	var set func(done string)
	set = func(done string) {
		err := menuBar.Set([]tray.Item{
			{Title: "Say hello", Symbol: outline.Heart, OnClick: count("Say hello")},
			{Title: "Checked", Checked: checked.Get(), OnClick: func() {
				checked.Set(!checked.Get())
				clicks.Set(clicks.Get() + 1)
				set("The check mark moved.")
			}},
			{Title: "Disabled", Disabled: true},
			tray.Separator(),
			{Title: "Submenu", Items: []tray.Item{
				{Title: "One", OnClick: count("One")},
				{Title: "Deeper", Items: []tray.Item{
					{Title: "Two", Symbol: outline.Star, OnClick: count("Two")},
				}},
			}},
		})
		report(done, err)
	}
	show := func() {
		if err := menuBar.SetTooltip(tip.Text()); err != nil {
			report("", err)
			return
		}
		set("Shown. Click the icon in the bar.")
	}
	ctx.OnUnmount("tray", func() { _ = menuBar.Remove() })

	labeled := func(text string, field gift.View) gift.View {
		return ui.VStack(
			ui.Text(text).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			field,
		).Gap(4).Align(geom.Alignment{X: 0, Y: 0})
	}
	return ui.VStack(
		labeled("Tooltip", ui.TextField(tip).Placeholder("May be empty").Key("tray-tooltip")),
		ui.HStack(
			ui.Button(ui.Text("Show or update"), show).Key("tray-show"),
			ui.Button(ui.Text("Remove"), func() {
				report("Removed.", menuBar.Remove())
			}).Key("tray-remove"),
		).Gap(8),
		ui.Text(ctx.Read(status)).FontSize(13).Foreground(ui.ColorSecondaryLabel).Key("tray-status"),
		ui.Text(fmt.Sprintf("Clicks in the menu: %d", ctx.Read(clicks))),
		ui.Text("The icon stays in the bar until Remove or until you leave this page. On GNOME it needs the "+
			"AppIndicator extension; a desktop without a status area says so above.").
			FontSize(12).Foreground(ui.ColorSecondaryLabel),
	).Gap(14).Frame(440, geom.Unbounded())
}
