// Command example-tray puts an icon into the menu bar of macOS, the
// notification area of Windows or the status area of a Linux desktop with
// gift/tray, with a menu that uses everything the package offers: entries
// with a symbol, a check mark that moves, a disabled entry, separators, a
// submenu two levels deep and Quit.
//
//	go run ./cmd/example-tray
//
// Every click on an entry is counted, in the window and in the tooltip of the
// icon. With "Close to tray" checked, closing the window hides it instead of
// ending the program, and "Show window" in the menu brings it back; that is
// what an application that lives in the menu bar does with Ebitengine's
// window functions. Where there is no tray — GNOME without the AppIndicator
// extension, a kiosk without a panel — the window says so, and closing it
// ends the program, since nothing would be left to bring it back.
//
// The first Set happens in the first update, once the application runs: on
// macOS the menu bar belongs to the NSApplication that Ebitengine creates in
// backend.Run. A Set before Run is kept and shown then, but returns an error
// meanwhile.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/tray"
	"github.com/worldiety/gift/ui"
)

// symbols are the icons the submenu More › Icon offers.
var symbols = []struct {
	name   string
	symbol ui.Symbol
}{
	{"Bell", outline.Bell},
	{"Star", outline.Star},
	{"Heart", outline.Heart},
}

// model is what the window and the menu show. It is touched only on the UI
// executor: in the first update, in OnUpdate, in the handlers of the window
// and in OnClick, which tray hands to app.Post.
type model struct {
	started     bool   // the first Set has been made
	trayOK      bool   // the icon is in the bar, so the window may hide
	clicks      int    // clicks on entries of the menu
	closeToTray bool   // closing the window hides it
	symbol      int    // index into symbols
	status      string // the last thing that happened
	quit        bool
}

var (
	app  *gift.App
	icon *tray.Tray
	m    = model{closeToTray: true, status: "Starting."}
)

func main() {
	if err := example.LoadFont(); err != nil {
		fmt.Fprintln(os.Stderr, "example-tray:", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	app = gift.New(gift.Options{Root: root})
	icon = tray.New(tray.Config{
		AppID:   "dev.gift.example-tray",
		Icon:    symbols[m.symbol].symbol,
		Tooltip: tooltip(),
		Post:    app.Post,
		Logger:  logger,
	})

	// The window is closed by OnUpdate, not by Ebitengine, so that closing
	// it can mean "hide it" while the icon is in the bar.
	ebiten.SetWindowClosingHandled(true)
	cfg := backend.Config{Title: "gift tray", Width: 520, Height: 400, Logger: logger,
		X11ClassName: "dev.gift.example-tray", X11InstanceName: "example-tray"}
	cfg.OnUpdate = update
	err := backend.Run(app, cfg)
	_ = icon.Remove()
	if err != nil {
		fmt.Fprintln(os.Stderr, "example-tray:", err)
		os.Exit(1)
	}
}

// update runs once per update on the UI executor. The first one shows the
// icon; later ones watch for Quit and for the close button of the window.
func update() error {
	if !m.started {
		m.started = true
		err := icon.Set(menu())
		m.trayOK = err == nil
		report("The icon is in the bar. Click it.", err)
	}
	if m.quit {
		return backend.Terminate
	}
	if ebiten.IsWindowBeingClosed() {
		if !m.closeToTray || !m.trayOK {
			return backend.Terminate
		}
		hideWindow()
	}
	return nil
}

// menu is the whole menu as a value, built again from the model by every
// change: tray.Set replaces it, which is how the check mark moves.
func menu() []tray.Item {
	icons := make([]tray.Item, len(symbols))
	for i, s := range symbols {
		icons[i] = tray.Item{
			Title:   s.name,
			Symbol:  s.symbol,
			Checked: i == m.symbol,
			OnClick: counted(func() {
				m.symbol = i
				report("The icon is now "+s.name+".", icon.SetIcon(s.symbol))
				changed()
			}),
		}
	}
	return []tray.Item{
		{Title: "Show window", Symbol: outline.Window, OnClick: counted(showWindow)},
		{Title: "Count this click", Symbol: outline.Plus, OnClick: counted(func() {})},
		{Title: "Synchronise (not available)", Symbol: outline.Refresh, Disabled: true},
		tray.Separator(),
		{Title: "Close to tray", Checked: m.closeToTray, OnClick: counted(toggleCloseToTray)},
		{Title: "More", Items: []tray.Item{
			{Title: "Icon", Symbol: symbols[m.symbol].symbol, Items: icons},
			{Title: "Reset the counter", Symbol: outline.TrashBin, OnClick: func() {
				m.clicks = 0
				report("The counter is back at 0.", icon.SetTooltip(tooltip()))
			}},
		}},
		tray.Separator(),
		{Title: "Quit", Symbol: outline.ArrowRightToBracket, OnClick: func() { m.quit = true }},
	}
}

// counted wraps an OnClick so that it counts the click, in the window and
// in the tooltip, which SetTooltip changes without touching the menu.
func counted(fn func()) func() {
	return func() {
		m.clicks++
		fn()
		report(m.status, icon.SetTooltip(tooltip()))
	}
}

func tooltip() string {
	return fmt.Sprintf("gift tray: %d clicks", m.clicks)
}

// changed sets the menu again after the model changed what it shows.
func changed() {
	report(m.status, icon.Set(menu()))
}

func toggleCloseToTray() {
	m.closeToTray = !m.closeToTray
	if m.closeToTray {
		m.status = "Closing the window hides it."
	} else {
		m.status = "Closing the window ends the program."
	}
	changed()
}

func showWindow() {
	ebiten.SetWindowVisible(true)
	if ebiten.IsWindowMinimized() {
		ebiten.RestoreWindow()
	}
	m.status = "The window is back."
	app.Invalidate()
}

func hideWindow() {
	ebiten.SetWindowVisible(false)
	m.status = "Hidden. Show window in the menu brings it back."
	app.Invalidate()
}

// report puts the outcome of a call into the status line. ErrUnsupported is
// the one error an application tells apart: there is no tray, and the window
// is all there is.
func report(done string, err error) {
	switch {
	case err == nil:
		m.status = done
	case errors.Is(err, tray.ErrUnsupported):
		m.trayOK = false
		m.status = "No tray on this desktop: " + err.Error()
	default:
		m.status = "Failed: " + err.Error()
	}
	app.Invalidate()
}

func root(ctx *gift.Context) gift.View {
	pad := geom.Insets{Top: 8, Right: 16, Bottom: 8, Left: 16}
	button := func(label string, action func()) gift.View {
		return ui.Button(ui.Text(label), action).CornerRadius(10).PaddingInsets(pad).MinHeight(ui.ControlHitTarget)
	}
	hide := button("Hide to tray", hideWindow)
	if !m.trayOK {
		hide = ui.Text("Without a tray the window stays.").FontSize(13).Foreground(ui.ColorSecondaryLabel)
	}
	return ui.Window(ui.VStack(
		ui.Text("Tray").FontSize(24),
		ui.Text(fmt.Sprintf("Clicks in the menu: %d", m.clicks)).FontSize(15),
		ui.HStack(
			ui.Toggle(m.closeToTray, func(bool) { toggleCloseToTray() }),
			ui.Text("Close to tray"),
		).Gap(8).Align(geom.Alignment{X: 0, Y: 0.5}),
		hide,
		ui.Text(m.status).FontSize(13).Foreground(ui.ColorSecondaryLabel),
	).Gap(16).Padding(24)).Align(geom.Alignment{X: 0.5, Y: 0.5})
}
