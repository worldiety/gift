package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "glass", Group: "styling", Width: 820, Height: 340}, glassDemo)
}

func glassDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// Glass shows what lies behind it, so it needs a busy backdrop: here a
	// row of colored stripes with a few dots on top.
	palette := []ui.Color{
		ui.RGB(232, 93, 74), ui.RGB(245, 166, 35), ui.RGB(248, 212, 66),
		ui.RGB(96, 186, 104), ui.RGB(64, 168, 176), ui.RGB(72, 132, 220),
		ui.RGB(124, 92, 214), ui.RGB(206, 84, 168),
	}
	var stripes, dots []gift.View
	for _, c := range palette {
		stripes = append(stripes, ui.Box().Background(c).Frame(96, 240))
		dots = append(dots, ui.Box().Background(ui.RGB(255, 255, 255)).Frame(20, 20).CornerRadius(10))
	}
	backdrop := ui.ZStack(
		ui.HStack(stripes...),
		ui.HStack(dots...).Gap(76),
	).Align(geom.Center)

	// One pane per variant. The pane is an ordinary stack with the glass
	// material as its background; the material is clipped to the corner radius.
	pane := func(title, detail string, glass ui.GlassMaterial) gift.View {
		return ui.VStack(
			ui.Text(title).FontSize(17).Foreground(ui.ColorLabel),
			ui.Text(detail).FontSize(13).Foreground(ui.ColorSecondaryLabel),
		).
			Gap(4).
			Padding(18).
			Frame(200, 150).
			Align(geom.Leading).
			Background(glass).
			Border(ui.Border{Width: 1, Color: ui.RGBA(255, 255, 255, 90)}).
			CornerRadius(20).
			Shadow(ui.Shadow{Blur: 16, OffsetY: 4, Color: ui.RGBA(0, 0, 0, 60)})
	}

	return ui.ZStack(
		backdrop,
		ui.HStack(
			// Reduced: no blur, only tint, edge highlight and refraction.
			pane("Reduced", "Sharp backdrop", ui.Glass().Quality(ui.Reduced)),
			// Full: the backdrop is blurred as well.
			pane("Full", "Blurred backdrop", ui.Glass().Quality(ui.Full)),
			// Full with a theme tint, a stronger blur and more refraction.
			pane("Tinted", "Surface tint, blur 28",
				ui.Glass().Quality(ui.Full).
					Tint(ui.Fade(ui.ColorSurface, 0.55)).
					Blur(28).
					Refraction(12)),
		).Gap(24),
	).Align(geom.Center)
	// snippet:end
}
