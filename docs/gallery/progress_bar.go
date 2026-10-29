package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "progress-bar", Group: "basic", Width: 480, Height: 320}, progressBarDemo)
}

func progressBarDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	item := func(title, value string, bar ui.ProgressBarView) gift.View {
		return ui.VStack(
			ui.HStack(
				ui.Text(title),
				ui.Spacer(),
				ui.Text(value).Foreground(ui.ColorSecondaryLabel),
			),
			bar.Label(title),
		).Gap(8)
	}

	return ui.VStack(
		item("Downloading", "45 %", ui.ProgressBar(0.45)),
		item("Disk usage", "92 %", ui.ProgressBar(0.92).
			Tint(ui.ColorDanger).
			Frame(geom.Unbounded(), 10)),
		item("Syncing", "Waiting for server", ui.ProgressBar(0).Indeterminate()),
	).Gap(20).Frame(320, geom.Unbounded())
	// snippet:end
}
