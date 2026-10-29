package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input moves the focus to the first field, which asks for the
	// keyboard, and types a name into it, so the picture shows the keyboard
	// up and the form laid out above it.
	register(Demo{
		Name: "keyboard", Group: "dialogs", Width: 720, Height: 430, FullWindow: true,
		Input: `{"steps":[{"op":"waitFrames","frames":2},{"op":"focus"},{"op":"waitFrames","frames":2},{"op":"text","text":"Ada"},{"op":"waitFrames","frames":4}]}`,
	}, keyboardDemo)
}

func keyboardDemo(ctx *gift.Context) gift.View {
	// The kiosk switch belongs in main, before the App runs. The gallery
	// shares one main between all demos, so this demo turns it on here.
	ui.SetOnScreenKeyboard(nil, true)

	// snippet:begin
	// In main: ui.SetOnScreenKeyboard(nil, true). With the switch on, a text
	// field asks for the keyboard when it takes the focus.
	name := ui.Editor(ctx, "name", "")
	company := ui.Editor(ctx, "company", "")

	labeled := func(title string, field gift.View) ui.Stack {
		return ui.VStack(
			ui.Text(title).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			field,
		).Gap(4).Align(geom.Alignment{X: 0, Y: 0})
	}

	form := ui.VStack(
		ui.HStack(
			ui.Icon(outline.User).Size(22).Foreground(ui.ColorAccent),
			ui.Text("Visitor check-in").FontSize(22),
		).Gap(10).Align(geom.Center),
		ui.HStack(
			labeled("Name", ui.TextField(name).Placeholder("Your name")).Flex(1),
			labeled("Company", ui.TextField(company).Placeholder("Optional")).Flex(1),
		).Gap(16),
	).Gap(16).Padding(24)

	// The keyboard is a direct child of the window, aligned to the bottom.
	// While it shows, the form is laid out in the space above it.
	return ui.Window(form, ui.OnScreenKeyboard()).Align(geom.Bottom)
	// snippet:end
}
