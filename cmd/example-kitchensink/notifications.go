package main

import (
	"errors"
	"fmt"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/notify"
	"github.com/worldiety/gift/ui"
)

// notifier shows the notifications of the notifications page. It opens
// nothing until the page first shows one.
//
// On macOS it needs the kitchen sink to run as an app bundle:
//
//	go run -exec "giftbundle -id dev.gift.kitchensink" ./cmd/example-kitchensink -show notifications
var notifier = notify.New(notify.Config{
	AppID:   "dev.gift.kitchensink",
	AppName: "gift kitchen sink",
	Post: func(fn func()) {
		if App != nil {
			App.Post(fn)
		}
	},
})

// own are the pages of the kitchen sink that are no component of
// internal/showcase and have no page among the components of the
// documentation.
var own = []page{{
	Demo: showcase.Demo{
		Name: "notifications", Group: "system", Title: "Notifications",
		Summary: "A message of the operating system outside the window, in the notification center and on the lock screen.",
		Width:   560, Height: 460,
	},
	build:  notificationsPage,
	source: "cmd/example-kitchensink/notifications.go",
}}

// notificationsPage shows, replaces and withdraws one notification of the
// operating system, with the text of the two fields, and counts the clicks
// on it.
func notificationsPage(ctx *gift.Context) gift.View {
	title := ui.Editor(ctx, "title", "Import finished")
	body := ui.Editor(ctx, "body", "1 024 pictures from the card.")
	status := ctx.State("status", "Nothing shown yet.")
	clicks := ctx.State("clicks", 0)

	// report turns the result of a call into the status line; an
	// application tells the two errors apart and may ignore both.
	report := func(done string, err error) {
		switch {
		case err == nil:
			status.Set(done)
		case errors.Is(err, notify.ErrUnsupported):
			status.Set("No notifications here: " + err.Error())
		case errors.Is(err, notify.ErrNotPermitted):
			status.Set("Not permitted: " + err.Error())
		default:
			status.Set("Failed: " + err.Error())
		}
	}
	show := func() {
		err := notifier.Show(notify.Notification{
			ID:      "kitchensink",
			Title:   title.Text(),
			Body:    body.Text(),
			OnClick: func() { clicks.Set(clicks.Get() + 1) },
		})
		report("Shown. Showing it again replaces it.", err)
	}
	authorize := func() {
		notifier.Authorize(func(granted bool) {
			if granted {
				status.Set("Permitted.")
			} else {
				status.Set("Not permitted.")
			}
		})
	}

	labeled := func(text string, field gift.View) gift.View {
		return ui.VStack(
			ui.Text(text).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			field,
		).Gap(4).Align(geom.Alignment{X: 0, Y: 0})
	}
	return ui.VStack(
		labeled("Title", ui.TextField(title).Placeholder("Required").Key("notification-title")),
		labeled("Body", ui.TextField(body).Placeholder("Plain text").Key("notification-body")),
		ui.HStack(
			ui.Button(ui.Text("Ask permission"), authorize).Key("notification-authorize"),
			ui.Button(ui.Text("Show or replace"), show).Key("notification-show"),
			ui.Button(ui.Text("Withdraw"), func() {
				report("Withdrawn.", notifier.Withdraw("kitchensink"))
			}).Key("notification-withdraw"),
		).Gap(8),
		ui.Text(ctx.Read(status)).FontSize(13).Foreground(ui.ColorSecondaryLabel).Key("notification-status"),
		ui.Text(fmt.Sprintf("Clicks on the notification: %d", ctx.Read(clicks))),
		ui.Text("macOS shows notifications only for an app bundle: run the kitchen sink with "+
			"go run -exec \"giftbundle -id dev.gift.kitchensink\". Windows registers the application "+
			"for the current user before the first one.").
			FontSize(12).Foreground(ui.ColorSecondaryLabel),
	).Gap(14).Frame(440, geom.Unbounded())
}
