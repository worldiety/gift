package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/icon/solid"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "tab-bar", Group: "navigation", Width: 560, Height: 420, FullWindow: true}, tabBarDemo)
}

func tabBarDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	tab := ctx.State("tab", 0)

	// Each tab's content is a whole screen. It stays mounted while another
	// tab is selected, so its state survives switching back and forth.
	screen := func(title string, rows ...gift.View) gift.View {
		return ui.VStack(
			ui.Text(title).FontSize(22),
			ui.VStack(rows...).Gap(12),
			ui.Spacer(),
		).Gap(16).Padding(24)
	}
	entry := func(sym ui.Symbol, text, detail string) gift.View {
		return ui.Card(ui.HStack(
			ui.Icon(sym).Size(22).Foreground(ui.ColorAccent),
			ui.VStack(
				ui.Text(text),
				ui.Text(detail).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			).Gap(2),
			ui.Spacer(),
		).Gap(12).Align(geom.Center))
	}

	home := screen("Good morning",
		entry(outline.CheckCircle, "Backup finished", "1,284 files copied"),
		entry(outline.Clock, "Next appointment", "Today at 10:30"),
	)

	// The selected tab is drawn in the accent color; SelectedIcon swaps the
	// outline symbol for the solid one while the tab is selected.
	return ui.TabBar(ctx.Read(tab), tab.Set,
		ui.Tab("Home", outline.Home, home).SelectedIcon(solid.Home),
		ui.Tab("Search", outline.Search, screen("Search")).SelectedIcon(solid.Search),
		ui.Tab("Inbox", outline.Inbox, screen("Inbox")).SelectedIcon(solid.Inbox),
		ui.Tab("Settings", outline.Cog, screen("Settings")).SelectedIcon(solid.Cog),
	)
	// snippet:end
}
