package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/qr"
)

func init() {
	register(Demo{Name: "qr-code", Group: "basic", Title: "QR Code",
		Summary: "A link or a code for the camera of a phone, sharp on every screen.",
		Width:   600, Height: 260}, qrCodeDemo)
}

func qrCodeDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	caption := func(s string) gift.View {
		return ui.Text(s).FontSize(13).Foreground(ui.ColorSecondaryLabel)
	}
	return ui.HStack(
		ui.VStack(
			qr.View("https://worldiety.github.io/gift/").Size(200).Label("QR code of the gift website"),
			caption("worldiety.github.io/gift"),
		).Gap(8).Align(geom.Center),
		ui.VStack(
			// Upper case letters and digits make the smaller alphanumeric code.
			qr.View("WDJB-MJHT").Size(120),
			caption("WDJB-MJHT"),
		).Gap(8).Align(geom.Center),
		ui.VStack(
			// Level H recovers 30 % of a damaged code, at the cost of size.
			qr.View("https://worldiety.github.io/gift/").Level(qr.H).Size(200),
			caption("the same link at level H"),
		).Gap(8).Align(geom.Center),
	).Gap(24).Align(geom.Center)
	// snippet:end
}
