package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input taps the well, which opens the picker below it.
	register(Demo{
		Name: "color-well", Group: "basic", Title: "Color Well", Summary: "A swatch that opens the colour picker in a popover.", Width: 440, Height: 640, FullWindow: true,
		Input: `{"steps":[{"op":"waitFrames","frames":2},{"op":"tap","x":388,"y":44},{"op":"waitFrames","frames":40}]}`,
	}, colorWellDemo)
}

func colorWellDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	tint := ctx.State("tint", ui.RGB(52, 199, 89))
	return ui.VStack(
		ui.HStack(
			ui.Text("Accent colour").Flex(1),
			ui.ColorWell(ctx.Read(tint), tint.Set).SupportsOpacity(false).Label("Accent colour"),
		).Align(geom.Center),
	).Padding(30)
	// snippet:end
}
