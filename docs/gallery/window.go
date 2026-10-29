package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "window", Group: "layout", Width: 640, Height: 420}, windowDemo)
}

func windowDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// The content of a screen: a title bar and two cards side by side.
	content := ui.VStack(
		ui.HStack(
			ui.Text("Inbox").FontSize(22),
			ui.Spacer(),
			ui.Icon(outline.Bell).Size(20),
		).Align(geom.Center),
		ui.HStack(
			ui.Card(
				ui.Text("Delivery at 14:00"),
				ui.Text("Gate 3").Foreground(ui.ColorSecondaryLabel),
			).Header("Today").Flex(1),
			ui.Card(
				ui.Text("Inventory report"),
				ui.Text("12 pages").Foreground(ui.ColorSecondaryLabel),
			).Header("Yesterday").Flex(1),
		).Gap(16),
	).Gap(16).Padding(20)

	// Window paints ColorBackground behind the content, so the cards sit on
	// the page and not on whatever the framebuffer held. The result is an
	// Overlay: Align places the content inside the window.
	screen := ui.Window(content).Align(geom.TopLeading)

	// Only for this picture: a real root view fills the whole window, here
	// it is framed and outlined to show it as a miniature.
	return screen.Frame(480, 240).
		CornerRadius(12).
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).
		Clip(true)
	// snippet:end
}
