// Package tray puts an icon into the menu bar of macOS, the notification area
// of the Windows taskbar and the status area of a Linux desktop, with a menu
// that opens when the user clicks it. It needs no cgo.
//
//	t := tray.New(tray.Config{
//		AppID:   "com.example.Photos",
//		Icon:    outline.CameraPhoto,
//		Tooltip: "Photos",
//		Post:    app.Post,
//		Logger:  logger,
//	})
//	_ = t.Set([]tray.Item{
//		{Title: "Show window", Symbol: outline.Window, OnClick: showWindow},
//		tray.Separator(),
//		{Title: "Import", Items: []tray.Item{
//			{Title: "From camera", OnClick: importCamera},
//			{Title: "From folder", OnClick: importFolder},
//		}},
//		{Title: "Notifications", Checked: on, OnClick: toggle},
//		{Title: "Quit", OnClick: quit},
//	})
//
// The error of Set can be ignored: a platform without a tray is reported once
// in the log and turns every call into nothing, which is the degradation an
// application wants from a convenience beside its window.
//
// # What a tray is here
//
// The part every supported system has in common, and nothing more; see the
// project plan, section 47, for the comparison it was taken from:
//
//   - one icon, a [ui.Symbol], which every system tints itself or which this
//     package tints to match the bar, and a tooltip;
//   - a menu that opens on a click on the icon, left or right, on every
//     system alike;
//   - entries with a title and optionally a symbol, separators, disabled
//     entries, entries with a check mark, and submenus;
//   - a click on an entry, delivered to OnClick through [Config.Post].
//
// Keyboard shortcuts in the menu, coloured icons, an action of its own for
// the left click and a badge on the icon differ too much between the systems
// to promise them.
//
// # The menu is a value
//
// [Tray.Set] replaces the whole menu. There are no handles to single entries:
// an application that wants a check mark to move builds the slice again and
// sets it, in the same way that a view is built again rather than mutated.
// Building a menu of a few dozen entries costs nothing next to the click that
// opens it.
//
// A click that arrives after a Set has replaced the menu it was made in is
// dropped, because the entry it was meant for may no longer exist or may now
// mean something else. That holds for a Set of the same entries as well: the
// menu of every Set is a new one. SetIcon and SetTooltip keep the menu, and
// a click in it still counts.
//
// A menu has at least one entry that is not a separator; Set refuses one
// without, since a click on the icon would open nothing.
//
// # Platforms
//
//   - darwin: NSStatusItem and NSMenu through the Objective-C runtime, in
//     platform_darwin.go. AppKit may only be touched on the main thread, which
//     belongs to Ebitengine; every change is dispatched to the main queue and
//     applied there, after Set has returned. Ebitengine creates NSApplication
//     when the application starts running; a Set before that is kept and
//     shown once the run loop serves the main queue, and returns an error
//     that wraps [ErrUnsupported] meanwhile. A program that never has
//     NSApplication — a command line tool, a test binary — therefore never
//     shows anything. No app bundle is needed, unlike gift/notify.
//   - windows: Shell_NotifyIconW and a popup menu, in platform_windows.go, on
//     a locked thread of its own with a hidden window and a message loop, so
//     that the menu does not stop the frames of the window while it is open.
//   - linux: a StatusNotifierItem with a com.canonical.dbusmenu menu on the
//     session bus, in platform_linux.go, which KDE Plasma, the AppIndicator
//     extension of GNOME, xfce4-panel, waybar and others show. A desktop
//     without a StatusNotifierWatcher is logged once and the icon appears
//     when a panel starts one, which covers an application that starts
//     before the panel at login; on GNOME without the extension, and on a
//     kiosk without a panel, that is never. A machine without a session bus
//     is the same case as an unsupported platform.
//   - Everything else: not supported, and it says so. Every call returns an
//     error that wraps [ErrUnsupported] and nothing is displayed.
//
// # Threads
//
// Set, SetIcon, SetTooltip and Remove may be called from any goroutine. They
// validate at once and return an error for an item no system can display;
// the change itself may be applied after they return, on the thread the
// platform requires, and a failure there is logged. A platform that is there
// but not ready keeps the state and catches up by itself, and logs the
// outage once rather than with every call. Only macOS before the run loop
// says so in the error, which wraps [ErrUnsupported]; on Windows, while
// Explorer has no taskbar, and on Linux, while no StatusNotifierWatcher is
// there, the calls return nil, because neither is known on the caller's
// goroutine without a round trip to the system.
//
// # Lifetime
//
// There is no Close. A Tray holds what its platform needs for the life of
// the process — on Windows a thread with a hidden window and a named mutex,
// on Linux a connection to the session bus and two goroutines, on macOS the
// status item's click target — and [Tray.Remove] only takes the icon out of
// the bar. Create one Tray per icon, once, at start, and change it with Set;
// a Tray created again and again, per window or per change of the settings,
// leaks those resources each time. On Windows a second Tray with the same
// AppID also gets an identity of its own and not the placement the user gave
// the first.
//
// OnClick runs on the UI executor: the click arrives on a thread of the
// system and is handed to [Config.Post], which is
// [github.com/worldiety/gift.App.Post] in an application.
//
// # Logging
//
// Section 15 of the project plan: log/slog, never slog.Default, and silence
// for a nil logger. The failure to open the platform is logged once.
package tray

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/worldiety/gift/ui"
)

// MaxTitleBytes is the longest title of an entry and the longest tooltip
// this package passes on, in bytes of UTF-8. Longer text is cut at a rune
// boundary and the cut is logged. A platform with a smaller limit of its own
// cuts further; Windows keeps 127 UTF-16 units of a tooltip.
const MaxTitleBytes = 256

// MaxItems bounds the entries of one menu, counted over all its submenus and
// separators included. A menu that long is a list and belongs in the window.
const MaxItems = 1024

// MaxDepth bounds the nesting of submenus: the top level and three below it.
const MaxDepth = 4

// ErrUnsupported is the error every call returns when there is no menu bar or
// status area to put an icon into on this machine. It is wrapped, so that the
// message says what was missing; [errors.Is] finds it.
var ErrUnsupported = errors.New("gift/tray: no tray on this platform")

// Config is the identity of the application towards the system, the initial
// icon and the way back to its UI executor.
type Config struct {
	// AppID names the application to the system: on Windows the identity of
	// the icon, from which a stable GUID is derived so that the user's choice
	// to always show it survives a restart; on Linux the Id of the
	// StatusNotifierItem. Conventionally a reverse domain name such as
	// "com.example.Photos". macOS does not use it.
	AppID string

	// Icon is the symbol shown in the bar, required. It is a mask: macOS
	// draws it as a template image in the colour of the menu bar, Windows and
	// Linux get it tinted by this package to contrast with the bar.
	Icon ui.Symbol

	// Tooltip is shown when the pointer rests on the icon. It may be empty.
	Tooltip string

	// Post runs a function on the UI executor; in an application it is
	// [github.com/worldiety/gift.App.Post]. A nil Post means that clicks are
	// not delivered.
	Post func(func())

	// Logger receives the failures; nil logs nothing.
	Logger *slog.Logger
}

// Item is one entry of the menu.
type Item struct {
	// Title is the text of the entry, required, plain text on one line.
	Title string

	// Symbol is drawn before the title; the zero Symbol draws nothing.
	Symbol ui.Symbol

	// Disabled shows the entry greyed out; it cannot be clicked, and its
	// submenu cannot be opened.
	Disabled bool

	// Checked shows a check mark before the entry. An entry with a submenu
	// cannot be checked.
	Checked bool

	// OnClick runs on the UI executor when the user clicks the entry. It is
	// nil for an entry with a submenu, and may be nil for an entry that only
	// says something.
	OnClick func()

	// Items makes the entry a submenu. An entry with Items has no OnClick.
	Items []Item

	separator bool
}

// Separator returns the line between two groups of entries.
func Separator() Item { return Item{separator: true} }

// IsSeparator reports whether it is the line made by [Separator].
func (it Item) IsSeparator() bool { return it.separator }

// Tray is the icon of the application in the bar. Create it with [New].
type Tray struct {
	cfg  Config
	open func(openConfig) (platform, error)

	once sync.Once
	p    platform

	mu       sync.Mutex
	icon     ui.Symbol
	tooltip  string
	menu     []entry
	clicks   []func()
	gen      uint64
	shown    bool
	dropOnce sync.Once
	// waiting says that the last update was answered with "not now", which
	// has been logged; see [platform.update].
	waiting bool
}

// entry is an [Item] as a platform receives it: validated, capped and
// numbered.
type entry struct {
	// id is 1 to the number of clickable entries in the menu, in order, and 0
	// for separators, submenus and entries without OnClick. It is what a
	// platform hands back on a click, together with the generation of the
	// state it was made from.
	id uint32

	title     string
	symbol    ui.Symbol
	disabled  bool
	checked   bool
	separator bool
	children  []entry
}

// state is everything a platform shows. gen counts the states of one [Tray],
// starting at 1, and changes with every call that changes anything.
type state struct {
	gen     uint64
	icon    ui.Symbol
	tooltip string
	menu    []entry
}

// platform is the seam between the part of this package that can be tested
// anywhere and the operating system, in the shape of gift/notify's.
type platform interface {
	// update shows s: it puts the icon into the bar if it is not there, and
	// replaces icon, tooltip and menu otherwise. It may return before s is
	// visible, and must not block on a thread that the caller may be running
	// on — the main thread of macOS above all. An error it returns is one it
	// knows of at once; a later failure it logs. An error that wraps
	// [ErrUnsupported] means "not now": the state is kept and shown when the
	// platform becomes ready, and the portable half logs it once.
	//
	// SetIcon and SetTooltip hand on the menu slice of the last Set
	// unchanged, and every Set makes a new one. A platform may therefore
	// compare the slices by identity to learn that only the icon or the
	// tooltip changed, keep the menu the user may have open, and report a
	// click in it under the newest generation.
	update(s state) error

	// remove takes the icon out of the bar. A later update puts it back.
	remove() error
}

// sameMenu reports whether a and b are the menu of one [Tray.Set], which is
// the rule of [platform.update]: the portable half keeps the slice of a Set
// until the next one, SetIcon and SetTooltip pass it on unchanged, and every
// Set converts into a new slice, so the same backing array means the same
// entries with the same ids. Two empty menus count as the same; neither has
// anything to click.
func sameMenu(a, b []entry) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || &a[0] == &b[0]
}

// openConfig is what a platform needs from [Config], with Post replaced by
// the function that guards it.
type openConfig struct {
	appID string

	// clicked is how a platform reports a click on the entry id of the
	// state gen, from any goroutine.
	clicked func(gen uint64, id uint32)

	log *slog.Logger
}

// New returns a tray for cfg. Nothing is shown until the first [Tray.Set]:
// an icon without a menu would open nothing when clicked. It connects to the
// system lazily, on that first call, and only once: a platform that cannot be
// opened is logged and not tried again. The Tray lives as long as the
// process; see "Lifetime" in the package documentation.
func New(cfg Config) *Tray {
	return newTray(cfg, openPlatform)
}

func newTray(cfg Config, open func(openConfig) (platform, error)) *Tray {
	t := &Tray{cfg: cfg, open: open, icon: cfg.Icon}
	if err := checkText(cfg.Tooltip); err != nil {
		t.logf(slog.LevelWarn, "Config.Tooltip is dropped", "error", err)
	} else {
		t.tooltip = t.capped("tooltip", cfg.Tooltip)
	}
	return t
}

func (t *Tray) impl() platform {
	t.once.Do(func() {
		p, err := t.open(openConfig{
			appID:   t.cfg.AppID,
			clicked: t.clicked,
			log:     t.cfg.Logger,
		})
		if err != nil {
			t.logf(slog.LevelError, "tray unavailable, nothing will be shown", "error", err)
			return
		}
		t.p = p
		t.logf(slog.LevelInfo, "tray ready")
	})
	return t.p
}

// clicked is how a platform hands a click back: it posts the OnClick of the
// entry, if the click belongs to the menu that is shown now.
func (t *Tray) clicked(gen uint64, id uint32) {
	t.mu.Lock()
	var fn func()
	if gen == t.gen && id >= 1 && int(id) <= len(t.clicks) {
		fn = t.clicks[id-1]
	}
	t.mu.Unlock()
	if fn == nil {
		return
	}
	if t.cfg.Post == nil {
		t.dropOnce.Do(func() {
			t.logf(slog.LevelWarn, "a tray entry was clicked, but Config.Post is nil, so OnClick is not run")
		})
		return
	}
	t.cfg.Post(fn)
}

// Set shows the icon, if it is not shown yet, with items as its menu, or
// replaces the menu of the icon that is shown.
//
// The error says which item no system can display, or that items has no
// entry other than separators, and nothing is changed;
// it wraps [ErrUnsupported] when there is no tray to show it in, which has
// been logged and may be ignored.
func (t *Tray) Set(items []Item) error {
	var clicks []func()
	n := 0
	menu, err := t.convert(items, 1, &n, &clicks)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(menu, func(e entry) bool { return !e.separator }) {
		return errors.New("gift/tray: the menu has no entry other than separators")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.icon.IsZero() {
		return errors.New("gift/tray: Config.Icon is required")
	}
	t.menu, t.clicks = menu, clicks
	return t.push()
}

// SetIcon replaces the icon in the bar. It takes effect at once if the icon
// is shown, and with the next [Tray.Set] otherwise.
func (t *Tray) SetIcon(s ui.Symbol) error {
	if s.IsZero() {
		return errors.New("gift/tray: the icon cannot be the zero Symbol")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.icon = s
	if !t.shown {
		return nil
	}
	return t.push()
}

// SetTooltip replaces the tooltip of the icon. It takes effect at once if the
// icon is shown, and with the next [Tray.Set] otherwise.
func (t *Tray) SetTooltip(s string) error {
	if err := checkText(s); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tooltip = t.capped("tooltip", s)
	if !t.shown {
		return nil
	}
	return t.push()
}

// Remove takes the icon out of the bar. A click on the menu that is still
// open, if any, is dropped. A later [Tray.Set] shows the icon again. Removing
// an icon that is not shown does nothing.
func (t *Tray) Remove() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.shown {
		return nil
	}
	p := t.impl()
	if p == nil {
		return fmt.Errorf("%w: the failure was logged when it was first opened", ErrUnsupported)
	}
	t.gen++
	t.clicks = nil
	t.shown = false
	if err := p.remove(); err != nil {
		t.logf(slog.LevelError, "removing the tray icon failed", "error", err)
		return err
	}
	return nil
}

// push hands the current state to the platform. t.mu is held.
func (t *Tray) push() error {
	p := t.impl()
	if p == nil {
		return fmt.Errorf("%w: the failure was logged when it was first opened", ErrUnsupported)
	}
	t.gen++
	t.shown = true
	err := p.update(state{gen: t.gen, icon: t.icon, tooltip: t.tooltip, menu: t.menu})
	switch {
	case err == nil:
		if t.waiting {
			t.waiting = false
			t.logf(slog.LevelInfo, "the tray icon is accepted again")
		}
	case errors.Is(err, ErrUnsupported):
		if !t.waiting {
			t.waiting = true
			t.logf(slog.LevelWarn, "the tray icon is kept until the system can show it", "error", err)
		}
	default:
		t.logf(slog.LevelError, "updating the tray icon failed", "error", err)
	}
	return err
}

// convert validates items and numbers their clickable entries, appending
// their handlers to clicks. depth is 1 for the top level; n counts the
// entries converted so far, over all levels.
func (t *Tray) convert(items []Item, depth int, n *int, clicks *[]func()) ([]entry, error) {
	if depth > MaxDepth {
		return nil, fmt.Errorf("gift/tray: submenus nest deeper than tray.MaxDepth (%d)", MaxDepth)
	}
	out := make([]entry, 0, len(items))
	for i, it := range items {
		*n++
		if *n > MaxItems {
			return nil, fmt.Errorf("gift/tray: the menu has more than tray.MaxItems (%d) entries", MaxItems)
		}
		if it.separator {
			out = append(out, entry{separator: true})
			continue
		}
		if it.Title == "" {
			return nil, fmt.Errorf("gift/tray: entry %d has no title", i)
		}
		if err := checkText(it.Title); err != nil {
			return nil, fmt.Errorf("%w (entry %d)", err, i)
		}
		e := entry{
			title:    t.capped("title", it.Title),
			symbol:   it.Symbol,
			disabled: it.Disabled,
			checked:  it.Checked,
		}
		if len(it.Items) > 0 {
			if it.OnClick != nil {
				return nil, fmt.Errorf("gift/tray: entry %q has a submenu and an OnClick", it.Title)
			}
			if it.Checked {
				return nil, fmt.Errorf("gift/tray: entry %q has a submenu and cannot be checked", it.Title)
			}
			children, err := t.convert(it.Items, depth+1, n, clicks)
			if err != nil {
				return nil, err
			}
			e.children = children
		} else if it.OnClick != nil {
			*clicks = append(*clicks, it.OnClick)
			e.id = uint32(len(*clicks))
		}
		out = append(out, e)
	}
	return out, nil
}

// checkText refuses what no system can display: invalid UTF-8, NUL, and line
// breaks, which a menu entry and a tooltip on one line cannot have.
func checkText(s string) error {
	if !utf8.ValidString(s) || strings.ContainsAny(s, "\x00\r\n") {
		return errors.New("gift/tray: text must be valid UTF-8 on one line, without NUL")
	}
	return nil
}

func (t *Tray) capped(what, s string) string {
	if len(s) <= MaxTitleBytes {
		return s
	}
	end := MaxTitleBytes
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	t.logf(slog.LevelWarn, "tray text is longer than tray.MaxTitleBytes, truncating",
		"field", what, "bytes", len(s), "kept", end)
	return s[:end]
}

func (t *Tray) logf(level slog.Level, msg string, args ...any) {
	if t.cfg.Logger == nil {
		return
	}
	t.cfg.Logger.Log(context.Background(), level, msg, args...)
}
