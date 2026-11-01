// Package notify shows notifications of the operating system: the messages
// that appear outside the application's window, in the notification centre of
// the desktop and, if the user allows it, on the lock screen. It needs no cgo.
//
//	n := notify.New(notify.Config{
//		AppID:   "com.example.Photos", // the name of the .desktop file, without .desktop
//		AppName: "Photos",
//		Post:    app.Post,
//		Logger:  logger,
//	})
//	_ = n.Show(notify.Notification{
//		ID:      "import",
//		Title:   "Import finished",
//		Body:    "1 024 pictures",
//		OnClick: func() { showImport() },
//	})
//
// The error of Show can be ignored: a platform without notifications is
// reported once in the log and turns every Show into nothing, which is the
// degradation an application wants from a message that is a courtesy.
//
// # What a notification is here
//
// The part every supported system has in common, and nothing more; see the
// project plan, section 42, for the comparison it was taken from:
//
//   - a title and a body, both plain text;
//   - an ID, which replaces the notification shown under the same ID and is
//     what [Notifier.Withdraw] takes back;
//   - a click on it, delivered to OnClick through [Config.Post] while the
//     application runs.
//
// Buttons, images, sounds, urgency and expiry differ too much between the
// systems to promise them. The application's name and icon are not per
// notification either: the system takes them from the registration of the
// application, which on Linux is the desktop entry named by [Config.AppID].
//
// The lock screen is the user's decision on every system and none lets an
// application override it.
//
// # Platforms
//
//   - linux: the Desktop Notifications Specification over the session bus,
//     which GNOME Shell, KDE Plasma, dunst, mako, xfce4-notifyd and others
//     implement. The bus is reached by a D-Bus client of this package's own,
//     in wire.go and conn.go, because there is no cgo-free one in the module
//     graph; freedesktop.go is the notification protocol on top.
//   - darwin: UNUserNotificationCenter through the Objective-C runtime, in
//     platform_darwin.go. Only for an application that is an app bundle:
//     macOS ends a process that asks for the notification centre without
//     one, so this package checks first and reports [ErrUnsupported]
//     instead. cmd/giftbundle runs a program as a bundle during development;
//     see "macOS" below.
//   - windows: toast notifications of the Windows Runtime, called through
//     syscall and hand written COM vtables, in platform_windows.go. The
//     identity is a registry entry this package writes itself; see "Windows"
//     below.
//   - Everything else: not supported, and it says so. Show returns an error
//     that wraps [ErrUnsupported] and nothing is displayed.
//
// A Linux machine without a notification server — a kiosk with no desktop,
// a session without a bus — is the same case as an unsupported platform.
//
// # Permission
//
// macOS asks the user once whether the application may show notifications.
// The question is asked by [Notifier.Authorize], or by the first Show if the
// application never called it. A Show before the answer, or after a "no",
// returns an error that wraps [ErrNotPermitted] and shows nothing; it is not
// kept for later. An application that notifies early should therefore call
// Authorize at start, or when the user turns notifications on in its own
// settings. On Linux the question does not exist and Authorize reports
// permission at once.
//
// # macOS
//
// The system identifies the application by its bundle, so [Config.AppID] and
// [Config.AppName] are not used there; the bundle's CFBundleIdentifier and
// CFBundleName are. The bundle must be signed, an ad hoc signature suffices,
// and known to LaunchServices. A binary built by `go run` or `go test` is
// none of that, and cmd/giftbundle makes it so:
//
//	go install github.com/worldiety/gift/cmd/giftbundle@latest
//	go run -exec "giftbundle -id com.example.Photos" ./cmd/photos
//
// A click raises the application by itself on macOS. The system hands the
// click to this package on the main dispatch queue, which only the main
// thread serves: an Ebitengine application does, through NSApplication's run
// loop, but a program without a run loop on its main thread — a command line
// tool, a test binary — shows notifications and never hears of a click.
//
// # Windows
//
// Windows drops a toast without a word unless its AppUserModelID is
// registered. Before the first toast this package writes the registration,
// for the current user and without administrator rights, under
// HKCU\Software\Classes\AppUserModelId\<AppID>: DisplayName from AppName,
// IconUri from [Config.Icon], and CustomActivator, the class id through which
// a click reaches this process. [Config.AppID] is therefore required on
// Windows, and [Unregister] removes the entry again, for an uninstaller.
// Neither a package, a signature nor a Start menu shortcut is needed.
//
// A click on a toast, on the banner or later in the notification centre,
// reaches OnClick while the application runs and brings its window to the
// front. A process running as administrator cannot show toasts at all; that
// is reported as [ErrUnsupported].
//
// # The desktop entry
//
// GNOME and KDE file a notification under the application whose desktop entry
// the hint "desktop-entry" names, take its icon from there, and give the user
// a switch for it in the settings. Without an installed entry of that name
// the notification is still shown, under AppName and with a generic icon.
// The entry is the same one that backend/ebiten's X11ClassName has to match
// through StartupWMClass, so an application has one name to choose.
//
// # Threads
//
// Show and Withdraw may be called from any goroutine and block for one round
// trip to the notification server, bounded by a timeout of two seconds. That
// is the same budget gift/clipboard spends on a paste, and like a paste it is
// an event, never a frame.
//
// OnClick runs on the UI executor: the click arrives on the goroutine that
// reads the bus, and is handed to [Config.Post], which is [github.com/worldiety/gift.App.Post] in
// an application.
//
// # Logging
//
// Section 15 of the project plan: log/slog, never slog.Default, and silence
// for a nil logger. The failure to open the platform is logged once; a failed
// Show is logged each time, because each one is a message the user did not
// get.
package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// MaxBytes is the longest title and the longest body this package passes on,
// in bytes of UTF-8. Longer text is cut at a rune boundary and the cut is
// logged.
//
// 4 KiB. A notification shows two or three lines; everything beyond them is
// either cut by the server or turns the notification into a wall. The bound
// keeps a body that is accidentally a whole log file from reaching the
// server at all. It is a ceiling and not a promise: a platform with a smaller
// limit of its own cuts further.
const MaxBytes = 4 << 10

// callTimeout bounds every round trip to the notification server.
const callTimeout = 2 * time.Second

// ErrUnsupported is the error Show returns when there is no notification
// system to talk to on this machine. It is wrapped, so that the message says
// what was missing; [errors.Is] finds it.
var ErrUnsupported = errors.New("gift/notify: no notifications on this platform")

// ErrNotPermitted is the error Show returns when the system has
// notifications but the user has not allowed them for this application, or
// has not answered yet. See the package documentation, "Permission".
var ErrNotPermitted = errors.New("gift/notify: notifications are not permitted")

// Config is the identity of the application towards the notification system
// and the way back to its UI executor.
type Config struct {
	// AppID names the application to the system. On Linux it is the name of
	// its desktop entry without ".desktop", conventionally a reverse domain
	// name such as "com.example.Photos", and it may be empty; see the package
	// documentation, "The desktop entry". On Windows it is the
	// AppUserModelID, required, at most 128 characters, without spaces and
	// backslashes. macOS takes the bundle identifier instead.
	AppID string

	// AppName is the name of the application as the user knows it. GNOME and
	// KDE show it when there is no desktop entry to take the name from;
	// Windows shows it above every toast.
	AppName string

	// Icon is the path of a PNG or ICO file that Windows shows with the
	// application's toasts and in its notification settings. Other systems
	// take the icon from the bundle or the desktop entry and ignore it.
	Icon string

	// Post runs a function on the UI executor; in an application it is
	// [github.com/worldiety/gift.App.Post]. A nil Post means that clicks are not delivered.
	Post func(func())

	// Logger receives the failures; nil logs nothing.
	Logger *slog.Logger
}

// Notification is one message to the user.
type Notification struct {
	// ID identifies the notification to [Notifier.Withdraw] and to a later
	// Show, which replaces the notification instead of adding a second one.
	// Empty means a notification that is neither replaced nor withdrawn.
	ID string

	// Title is the first line, required. Body is the text below it and may
	// be empty. Both are plain text.
	Title, Body string

	// OnClick runs on the UI executor when the user clicks the
	// notification, if the system reports clicks and the application is
	// still running. A click after the application has exited does nothing.
	OnClick func()
}

// Notifier shows notifications. Create it with [New].
type Notifier struct {
	cfg  Config
	open func(openConfig) (platform, error)

	once sync.Once
	p    platform

	dropOnce sync.Once
}

// platform is the seam between the part of this package that can be tested
// anywhere and the operating system, in the shape of gift/clipboard's.
type platform interface {
	show(n Notification) error
	withdraw(id string) error

	// authorize asks for permission where the system has such a thing and
	// calls done with the answer, from any goroutine, possibly before it
	// returns.
	authorize(done func(granted bool))
}

// openConfig is what a platform needs from [Config], with Post replaced by
// the function that guards it.
type openConfig struct {
	appID, appName, icon string
	activated            func(func())
	log                  *slog.Logger
}

// New returns a notifier for cfg. It connects to the system lazily, on the
// first Show, and only once: a platform that cannot be opened is logged and
// not tried again, so that an application that notifies in a loop does not
// dial the bus each time.
func New(cfg Config) *Notifier {
	return newNotifier(cfg, openPlatform)
}

// Unregister removes what this package registered with the system for cfg,
// for an uninstaller: on Windows the AppUserModelID entry in the registry.
// Elsewhere there is nothing to remove and it returns nil.
func Unregister(cfg Config) error {
	return unregisterPlatform(cfg)
}

func newNotifier(cfg Config, open func(openConfig) (platform, error)) *Notifier {
	return &Notifier{cfg: cfg, open: open}
}

func (n *Notifier) impl() platform {
	n.once.Do(func() {
		p, err := n.open(openConfig{
			appID:     n.cfg.AppID,
			appName:   n.cfg.AppName,
			icon:      n.cfg.Icon,
			activated: n.activated,
			log:       n.cfg.Logger,
		})
		if err != nil {
			n.logf(slog.LevelError, "notifications unavailable, nothing will be shown", "error", err)
			return
		}
		n.p = p
		n.logf(slog.LevelInfo, "notifications ready")
	})
	return n.p
}

// activated is how a platform hands a click back: it posts OnClick to the UI
// executor.
func (n *Notifier) activated(fn func()) {
	if n.cfg.Post == nil {
		n.dropOnce.Do(func() {
			n.logf(slog.LevelWarn, "a notification was clicked, but Config.Post is nil, so OnClick is not run")
		})
		return
	}
	n.cfg.Post(fn)
}

// Show displays note, or replaces the notification shown under note.ID.
//
// The error wraps [ErrUnsupported] when there is nothing to display it on; it
// has been logged and may be ignored.
func (n *Notifier) Show(note Notification) error {
	if note.Title == "" {
		return errors.New("gift/notify: a notification needs a title")
	}
	for _, s := range []string{note.ID, note.Title, note.Body} {
		if !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
			return errors.New("gift/notify: a notification must be valid UTF-8 without NUL")
		}
	}
	note.Title = n.capped("title", note.Title)
	note.Body = n.capped("body", note.Body)

	p := n.impl()
	if p == nil {
		return fmt.Errorf("%w: the failure was logged when it was first opened", ErrUnsupported)
	}
	if err := p.show(note); err != nil {
		n.logf(slog.LevelError, "showing a notification failed", "id", note.ID, "error", err)
		return err
	}
	return nil
}

// Authorize asks the user for permission to show notifications, on a system
// that asks (macOS), and reports the answer to done through [Config.Post].
// It returns at once; the answer may take as long as the user does. On a
// system that does not ask, the answer is yes; on one without notifications
// it is no. done may be nil.
//
// Asking again after an answer does not show the question again: the system
// returns the recorded answer, which the user changes in its settings.
func (n *Notifier) Authorize(done func(granted bool)) {
	report := func(granted bool) {
		if done != nil && n.cfg.Post != nil {
			n.cfg.Post(func() { done(granted) })
		}
	}
	p := n.impl()
	if p == nil {
		report(false)
		return
	}
	p.authorize(report)
}

// Withdraw removes the notification shown under id, if it is still shown. An
// unknown id is not an error: the user may have dismissed it already.
func (n *Notifier) Withdraw(id string) error {
	if id == "" {
		return nil
	}
	p := n.impl()
	if p == nil {
		return fmt.Errorf("%w: the failure was logged when it was first opened", ErrUnsupported)
	}
	if err := p.withdraw(id); err != nil {
		n.logf(slog.LevelError, "withdrawing a notification failed", "id", id, "error", err)
		return err
	}
	return nil
}

func (n *Notifier) capped(what, s string) string {
	if len(s) <= MaxBytes {
		return s
	}
	end := MaxBytes
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	n.logf(slog.LevelWarn, "notification text is longer than notify.MaxBytes, truncating",
		"field", what, "bytes", len(s), "kept", end)
	return s[:end]
}

func (n *Notifier) logf(level slog.Level, msg string, args ...any) {
	if n.cfg.Logger == nil {
		return
	}
	n.cfg.Logger.Log(context.Background(), level, msg, args...)
}
