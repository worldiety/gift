// Command example-notify shows notifications of the operating system with
// gift/notify: one that is shown, updated under the same ID and withdrawn,
// and a count of the clicks on it.
//
//	go run ./cmd/example-notify                 # Linux
//	go install ./cmd/giftbundle
//	go run -exec "giftbundle -id dev.gift.example-notify" ./cmd/example-notify   # macOS
//
// On macOS the program has to run as an app bundle, which is what giftbundle
// does; without it the status line reports that notifications are
// unsupported, and that is the fallback an application gets on any system
// without them. On Linux the notification is filed under the desktop entry
// dev.gift.example-notify, which does not exist, so it carries the generic
// icon and the name "gift notify".
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/notify"
	"github.com/worldiety/gift/ui"
)

var (
	app      *gift.App
	notifier *notify.Notifier
)

func main() {
	if err := example.LoadFont(); err != nil {
		fmt.Fprintln(os.Stderr, "example-notify:", err)
		os.Exit(1)
	}
	app = gift.New(gift.Options{Root: root})
	notifier = notify.New(notify.Config{
		AppID:   "dev.gift.example-notify",
		AppName: "gift notify",
		Post:    app.Post,
	})
	cfg := backend.Config{Title: "gift notify", Width: 520, Height: 360,
		X11ClassName: "dev.gift.example-notify", X11InstanceName: "example-notify"}
	if err := backend.Run(app, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "example-notify:", err)
		os.Exit(1)
	}
}

func root(ctx *gift.Context) gift.View {
	status := ctx.State("status", "Nothing shown yet.")
	clicks := ctx.State("clicks", 0)
	version := ctx.State("version", 0)

	// report turns the result of a call into the status line. The three
	// errors an application tells apart are spelled out.
	report := func(what string, err error) {
		switch {
		case err == nil:
			status.Set(what)
		case errors.Is(err, notify.ErrUnsupported):
			status.Set("No notifications on this system: " + err.Error())
		case errors.Is(err, notify.ErrNotPermitted):
			status.Set("Not permitted: " + err.Error())
		default:
			status.Set("Failed: " + err.Error())
		}
	}
	show := func() {
		v := version.Get() + 1
		version.Set(v)
		err := notifier.Show(notify.Notification{
			ID:    "demo",
			Title: "gift notify",
			Body:  fmt.Sprintf("Version %d of the same notification. Click it.", v),
			OnClick: func() {
				clicks.Set(clicks.Get() + 1)
			},
		})
		report(fmt.Sprintf("Shown, version %d.", v), err)
	}

	pad := geom.Insets{Top: 8, Right: 16, Bottom: 8, Left: 16}
	button := func(label string, action func()) gift.View {
		return ui.Button(ui.Text(label), action).CornerRadius(10).PaddingInsets(pad).MinHeight(ui.ControlHitTarget)
	}
	return ui.Window(ui.VStack(
		ui.Text("Notifications").FontSize(24),
		ui.HStack(
			button("Ask permission", func() {
				notifier.Authorize(func(granted bool) {
					if granted {
						status.Set("Permitted.")
					} else {
						status.Set("Not permitted.")
					}
				})
			}),
			button("Show or update", show),
			button("Withdraw", func() {
				report("Withdrawn.", notifier.Withdraw("demo"))
			}),
		).Gap(8),
		ui.Text(ctx.Read(status)).FontSize(13).Foreground(ui.ColorSecondaryLabel),
		ui.Text(fmt.Sprintf("Clicks on the notification: %d", ctx.Read(clicks))).FontSize(15),
	).Gap(16).Padding(24)).Align(geom.Alignment{X: 0.5, Y: 0.5})
}
