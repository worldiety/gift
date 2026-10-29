package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/ibmplexmono"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "text", Group: "basic", Width: 640, Height: 560}, textDemo)
}

func textDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	bold := ui.MustFont(ui.FontQuery{Family: inter.Family, Weight: ui.WeightBold})
	mono := ui.MustFont(ui.FontQuery{Family: ibmplexmono.Family})
	path := "/Volumes/photos/2026/Wedding Anna & Ben/IMG_0001.jpg"

	aligned := func(s string, a ui.TextAlign) gift.View {
		return ui.Text(s).Align(a).MinWidth(120).
			Padding(6).Background(ui.ColorControl).CornerRadius(6)
	}
	truncated := func(t ui.Truncation) gift.View {
		return ui.Text(path).Truncation(t).Font(mono).FontSize(13).MaxWidth(260)
	}

	return ui.VStack(
		ui.Text("Release notes").Font(bold).FontSize(24),
		ui.Text("Version 1.4 · October 2026").Foreground(ui.ColorSecondaryLabel),
		ui.Text("Text wraps at word boundaries inside the width its parent "+
			"allows. Measuring and drawing use the same shaping result, so "+
			"a label is never measured differently from how it is drawn.").
			MaxWidth(420),
		ui.Text("MaxLines(2) keeps a long description to two lines. When the "+
			"text needs more, the last visible line ends with an ellipsis "+
			"instead of running on.").
			MaxLines(2).MaxWidth(420).Foreground(ui.ColorSecondaryLabel),
		ui.HStack(
			aligned("Leading", ui.AlignLeading),
			aligned("Center", ui.AlignCenter),
			aligned("Trailing", ui.AlignTrailing),
		).Gap(8),
		ui.VStack(
			truncated(ui.TruncateTail),
			truncated(ui.TruncateMiddle),
			truncated(ui.TruncateHead),
		).Gap(4),
		ui.Text("Read the full changelog").Font(bold).Foreground(ui.ColorAccent),
	).Gap(12)
	// snippet:end
}
