package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "row", Group: "lists", Width: 520, Height: 480}, rowDemo)
}

func rowDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	dark := ctx.State("dark", true)
	opened := ctx.State("opened", "")

	return ui.Card(
		ui.List(
			// Title, value and chevron: a row that leads somewhere.
			ui.Row("Wi-Fi").
				Icon(outline.Globe).
				Value("kiosk-net").
				Chevron(outline.AngleRight).
				OnTap(func() { opened.Set("wifi") }),

			// A subtitle is a second, quieter line under the title.
			ui.Row("Account").
				Icon(outline.UserCircle).
				Subtitle("anna@example.com").
				Chevron(outline.AngleRight).
				OnTap(func() { opened.Set("account") }),

			// An interactive accessory takes its own tap.
			ui.Row("Dark mode").
				Icon(outline.Moon).
				Accessory(ui.Toggle(ctx.Read(dark), dark.Set).Label("Dark mode")),

			// An inert row that only reports a value.
			ui.Row("Language").
				Icon(outline.Language).
				Value("English"),

			// Disabled takes the row out of input and dims its labels.
			ui.Row("Remote access").
				Icon(outline.Lock).
				Subtitle("managed by your administrator").
				Chevron(outline.AngleRight).
				Disabled(true).
				OnTap(func() { opened.Set("remote") }),
		).SeparatorInsets(ui.RowPadding+22+12, 0).MinWidth(380),
	).Padding(0)
	// snippet:end
}
