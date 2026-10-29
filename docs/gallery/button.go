package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "button", Group: "basic", Width: 720, Height: 200}, buttonDemo)
}

func buttonDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	saves := ctx.State("saves", 0)
	pad := geom.Insets{Top: 8, Right: 16, Bottom: 8, Left: 16}

	// A filled primary button. A button with its own background keeps it on
	// hover; the pressed face is declared, and Lift adds the press feedback.
	accent := ui.ButtonStyle{Background: ui.ColorAccent, CornerRadius: 10}
	pressed := ui.ButtonStyle{Background: ui.Fade(ui.ColorAccent, 0.8), CornerRadius: 10}
	save := ui.Button(ui.Text("Save").Foreground(ui.ColorOnAccent), func() {
		saves.Set(saves.Get() + 1)
	}).Style(accent).PressedStyle(pressed).Lift(true)

	// The label is any view: an icon next to text.
	download := ui.Button(ui.HStack(
		ui.Icon(outline.Download).Size(16),
		ui.Text("Download"),
	).Gap(6).Align(geom.Center), func() {}).CornerRadius(10)

	// The default face with a red label for a destructive command.
	remove := ui.Button(ui.Text("Delete").Foreground(ui.ColorDanger), func() {}).
		CornerRadius(10)

	// An icon alone needs an accessible name.
	favorite := ui.Button(ui.Icon(outline.Heart).Size(18), func() {}).
		CornerRadius(10).Padding(10).Label("Add to favorites")

	// Disabled dims the box; the label is yours to dim.
	disabled := ui.Button(ui.Text("Disabled").Foreground(ui.ColorSecondaryLabel), func() {}).
		CornerRadius(10).Disabled(true)

	return ui.HStack(
		save.PaddingInsets(pad).MinHeight(ui.ControlHitTarget),
		download.PaddingInsets(pad).MinHeight(ui.ControlHitTarget),
		remove.PaddingInsets(pad).MinHeight(ui.ControlHitTarget),
		favorite.MinWidth(ui.ControlHitTarget).MinHeight(ui.ControlHitTarget),
		disabled.PaddingInsets(pad).MinHeight(ui.ControlHitTarget),
	).Gap(12).Align(geom.Center)
	// snippet:end
}
