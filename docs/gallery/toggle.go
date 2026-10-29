package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "toggle", Group: "basic", Width: 480, Height: 360}, toggleDemo)
}

func toggleDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	wifi := ctx.State("wifi", true)
	bluetooth := ctx.State("bluetooth", false)
	airplane := ctx.State("airplane", true)

	row := func(title string, toggle ui.ToggleView) gift.View {
		return ui.HStack(ui.Text(title), ui.Spacer(), toggle.Label(title)).
			MinWidth(300).
			Align(geom.Alignment{X: 0, Y: 0.5})
	}

	return ui.Card(
		row("Wi-Fi", ui.Toggle(ctx.Read(wifi), wifi.Set)),
		row("Bluetooth", ui.Toggle(ctx.Read(bluetooth), bluetooth.Set)),
		row("Airplane mode", ui.Toggle(ctx.Read(airplane), airplane.Set).Tint(ui.ColorDanger)),
		row("Location (managed)", ui.Toggle(true, nil).Disabled(true)),
		row("Roaming (managed)", ui.Toggle(false, nil).Disabled(true)),
	).Gap(4)
	// snippet:end
}
