package main

import (
	"strings"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "colors", Group: "styling", Width: 760, Height: 460}, colorsDemo)
}

func colorsDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// One swatch per semantic color. The hairline keeps the swatches that
	// match the window background visible.
	swatch := func(name string, c ui.Color) gift.View {
		return ui.VStack(
			ui.Box().Background(c).Frame(150, 48).CornerRadius(10).
				Border(ui.Border{Width: 1, Color: ui.ColorSeparator}),
			ui.Text(strings.TrimPrefix(name, "Color")).FontSize(13),
		).Gap(6)
	}

	// The palette, four swatches per row, in declaration order.
	var rows []gift.View
	var row []gift.View
	for _, nc := range ui.SemanticColors() {
		row = append(row, swatch(nc.Name, nc.Color))
		if len(row) == 4 {
			rows = append(rows, ui.HStack(row...).Gap(16))
			row = nil
		}
	}

	// Fade keeps a semantic color semantic: these follow the theme too.
	fades := ui.HStack(
		swatch("Accent", ui.ColorAccent),
		swatch("Fade(Accent, 0.6)", ui.Fade(ui.ColorAccent, 0.6)),
		swatch("Fade(Accent, 0.3)", ui.Fade(ui.ColorAccent, 0.3)),
		swatch("Fade(Accent, 0.1)", ui.Fade(ui.ColorAccent, 0.1)),
	).Gap(16)

	caption := func(s string) ui.TextView {
		return ui.Text(s).FontSize(13).Foreground(ui.ColorSecondaryLabel)
	}
	return ui.VStack(
		caption("SemanticColors()"),
		ui.VStack(rows...).Gap(16),
		caption("Fade").PaddingInsets(geom.Insets{Top: 10}),
		fades,
	).Gap(14)
	// snippet:end
}
