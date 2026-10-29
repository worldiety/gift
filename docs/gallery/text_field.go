package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input moves the focus to the first field and selects its last word
	// with shift and the word modifier, which is alt on macOS, where the
	// screenshots are taken. That shows the focus ring and the selection.
	register(Demo{
		Name: "text-field", Group: "basic", Width: 520, Height: 420,
		Input: `{"steps":[{"op":"waitFrames","frames":2},{"op":"focus"},{"op":"key","key":"left","mods":["shift","alt"]}]}`,
	}, textFieldDemo)
}

func textFieldDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// An editor holds the text, the caret and the selection, so they
	// survive the rebuild every keystroke causes.
	name := ui.Editor(ctx, "name", "Ada Lovelace")
	email := ui.Editor(ctx, "email", "")
	search := ui.Editor(ctx, "search", "")
	customer := ui.Editor(ctx, "customer", "C-2041")
	query := ctx.State("query", "")

	labeled := func(title string, field gift.View) gift.View {
		return ui.VStack(
			ui.Text(title).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			field,
		).Gap(4).Align(geom.Alignment{X: 0, Y: 0})
	}

	// A search box: a borderless field next to an icon, in a rounded
	// pill. Enter runs OnSubmit with the current text.
	searchBox := ui.HStack(
		ui.Icon(outline.Search).Size(16).Foreground(ui.ColorSecondaryLabel),
		ui.TextField(search).
			Placeholder("Search").
			Background(ui.ColorClear).
			Border(ui.Border{}).
			FocusRing(ui.Border{}).
			OnSubmit(query.Set).
			Flex(1),
	).Gap(2).Align(geom.Center).
		PaddingInsets(geom.Insets{Left: 10, Right: 4}).
		Background(ui.ColorControl).
		CornerRadius(14)

	return ui.VStack(
		labeled("Name", ui.TextField(name).Placeholder("Your name")),
		labeled("Email", ui.TextField(email).Placeholder("you@example.com")),
		labeled("Search", searchBox),
		labeled("Customer number", ui.TextField(customer).Disabled(true)),
	).Gap(14).Frame(320, geom.Unbounded())
	// snippet:end
}
