package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input opens the spectrum, the most colourful of the three.
	register(Demo{
		Name: "color-picker", Group: "basic", Title: "Color Picker", Summary: "A colour from a grid, a spectrum or sliders, with opacity and hex code.", Width: 420, Height: 560,
		Input: `{"steps":[{"op":"waitFrames","frames":2},{"op":"tap","x":210,"y":75},{"op":"waitFrames","frames":4}]}`,
	}, colorPickerDemo)
}

func colorPickerDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	c := ctx.State("c", ui.RGBA(255, 149, 0, 230))
	return ui.Card(ui.ColorPicker(ctx.Read(c), c.Set)).Padding(16)
	// snippet:end
}
