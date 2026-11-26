---
title: Android and iOS
weight: 15
---

A gift application runs on Android and iOS with the same views it has on the desktop. Support for both is
experimental. What differs is who owns the program: on a phone the operating system starts the process, drives the
frame loop and ends the process, so the application is a library that a small host application in Kotlin or Swift
shows. The desktop is unchanged by any of this.

## The game

`backend.Run` opens a window and runs the frame loop until it closes. A phone has no such window, and `Run` returns
an error there. `backend.NewGame` returns the game `Run` would drive, together with its options, for Ebitengine's
`mobile` package. It is called from an `init` function of the package `ebitenmobile bind` turns into an Android
library or an iOS framework:

```go
package mobile

import (
	"github.com/hajimehoshi/ebiten/v2/mobile"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/geom"

	"example.com/notes/app"
)

func init() {
	g, opts := backend.NewGame(app.New, backend.Config{})
	mobile.SetGameWithOptions(g, opts)
}

// SetSafeArea is called by the host: dp on Android, points on iOS; any thread.
func SetSafeArea(left, top, right, bottom float64) {
	backend.SetSafeArea(geom.Insets{Top: float32(top), Right: float32(right),
		Bottom: float32(bottom), Left: float32(left)})
}
```

```sh
ebitenmobile bind -target android -javapkg com.example.app -o mobile.aar ./mobile
ebitenmobile bind -target ios -iosversion 17.0 -o Mobile.xcframework ./mobile
```

`ebitenmobile` is a command of the Ebitengine module the application already requires, and it should be the same
version, since it writes the host glue (`EbitenView`, `EbitenViewController`) that this page relies on. Run inside
the application's module, this installs the version go.mod requires:

```sh
go install github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile@$(go list -m -f '{{.Version}}' github.com/hajimehoshi/ebiten/v2)
```

`-target android` builds every ABI (arm, arm64, x86, x86_64); `-target android/arm64` builds a smaller library, but
it leaves out 32-bit ARM devices and x86 and x86_64 emulators, unless the emulator image translates ARM code. Binding
for Android needs the Android SDK with an NDK (`ANDROID_HOME`, or `ANDROID_NDK_HOME`); binding for iOS needs Xcode
on macOS. The result is an `.aar` for Gradle or an `.xcframework` for Xcode.

The application lives in a package of its own, whose `New` also does the process wide setup (`ui.SetDefaultFont`,
`ui.SetTheme`), as a desktop's `main` would:

```go
package app

func New() *gift.App {
	ui.SetDefaultFont(ui.MustFont(ui.FontQuery{Family: inter.Family}))
	return gift.New(gift.Options{Root: func(*gift.Context) gift.View {
		return ui.Window(screen(), ui.OnScreenKeyboard()).Align(geom.Bottom)
	}})
}
```

The desktop's `main` calls `backend.Run(app.New(), cfg)`, the bound package
`backend.NewGame(app.New, backend.Config{})`. A bound library has no `main`, so the font has to be installed in `New`:
without a default font the first `Text` panics.

`NewGame` takes a constructor rather than an App. The goroutine that creates an App is its
[UI goroutine](../concurrency/), and on a phone `init` runs on one goroutine while Ebitengine runs the frame loop
on another. The game calls the constructor on the loop's goroutine, the first time Ebitengine calls into it.

A zero `backend.Config` is the whole backend setup on mobile: the mouse is not read, because Ebitengine reports none
there, and text fields get the system keyboard. The application still installs its font, as on a desktop. Settings
that concern a window, such as `Title`, `Width` and `Height`, have no window to go to.

## The safe area

A phone draws its status bar, its navigation bar and the cutout of its camera over the application. The host
reports how much of each edge they cover through the bound package's `SetSafeArea`, which forwards to
`backend.SetSafeArea`. It may do so from any thread and before the game exists; the report waits for the next
update. On Android the values are the insets of `systemBars()` and `displayCutout()` in dp, from
`onApplyWindowInsets` and `onLayout` of an `EbitenView` subclass shown edge to edge. On iOS they are the view's
`safeAreaInsets` in points, from `viewSafeAreaInsetsDidChange` and `viewDidLayoutSubviews`. A desktop reports
nothing and sees zero; `Config.SafeArea` sets a fixed safe area for a bezel or an overscanned screen.

What gift does with it:

- A [Window](../../components/layout/window/) that fills the screen lays its content out in the safe rectangle and
  paints its page behind the bars. `IgnoresSafeArea` turns that off for content that draws edge to edge itself; the
  view function then pads its controls by `Context.SafeArea`.
- The bars of a [TabBar](../../components/navigation/tab-bar/) and a
  [NavigationStack](../../components/navigation/navigation-stack/), a scrim and the drawn keyboard paint behind the
  system bars and keep their content inside the safe area. A sheet lays its rounded surface out to the window edge
  and keeps its content inside.
- A stack, a box or a ZStack does the same with `BleedSafeArea(ui.EdgesTop)` and the other edges. Bleeding is paint
  only: the hit area and the layout stay where they are.
- Toasts, popovers and menus are placed inside the safe rectangle.
- A view function reads the values with `Context.SafeArea`, a custom view with `BuildContext.SafeArea` or
  `LayoutContext.SafeArea`, and `App.SafeRect` gives the rectangle. A change, a rotation for example, rebuilds
  everything.

In tests, `gifttest.Options.SafeArea` and `Harness.SetSafeArea` give the harness a phone's safe area.

## The system keyboard

On Android and iOS a text field that gains the focus raises the system keyboard, with the user's layout, language,
suggestions and autocorrection. `TextField`, `SecureField`, `SearchField`, `TextArea` and `CodeEditor` are served.
While the system keyboard is up, `App.PlatformKeyboardActive` is true and the drawn
[keyboard](../../components/dialogs/keyboard/) stays down. The word the keyboard is still composing is shown as
ordinary text and replaced as it changes; gift has no underline and no candidate window. Edits arrive as
`EventTextReplace` through `App.ReplaceText`, with the edit in `EventContext.TextReplacement`, and
`Harness.ReplaceText` sends them in a test. A custom text view that implements `gift.TextInputClient` also asks for
a keyboard when it gains the focus; the documentation of `TextInputClient` has the contract.

Three limits are worth knowing:

- **Passwords.** Password fields get the system keyboard in its normal mode, because Ebitengine cannot ask for
  password mode. The keyboard may learn and suggest what is typed into them. `Config.NoNativeKeyboard` together
  with `ui.SetOnScreenKeyboard(nil, true)` gives every field gift's drawn keyboard instead.
- **Date and time.** `DateField` and `TimeField` keep the drawn keyboard, so a mobile window places
  `ui.OnScreenKeyboard()` and is aligned to the bottom, `.Align(geom.Bottom)`, as the example above does.
- **Return.** The return key arrives as `KeyEnter`, which submits a single line field and breaks the line in an
  area.

## The host

On Android the activity declares

```xml
android:configChanges="orientation|screenSize|smallestScreenSize|screenLayout|density|keyboard|keyboardHidden|navigation|uiMode"
```

so that a rotation, a fold, a theme change or a hardware keyboard that comes or goes is a relayout and not a new
activity. It sets `windowSoftInputMode="adjustNothing"`, because Ebitengine shifts the picture itself to keep the
caret above the system keyboard, and calls `suspendGame` and `resumeGame` from `onPause` and `onResume`.
`EbitenView` consumes every key, so the activity hands Back and the volume keys back to Android. Back moves the
task to the background rather than finishing the activity: Ebitengine cannot carry its GPU resources into a new
view in the same process.

On iOS the application creates exactly one subclass of the generated `MobileEbitenViewController` (the prefix is the
bound package's name) per process and reuses it when its scene reconnects, because Ebitengine attaches its Metal
layer only once. It suspends and resumes the game from the scene delegate. iOS paints in sRGB; see
[Color Management](../color-management/).

Packages that load desktop libraries (video, typst, turbojpeg) or talk to a desktop (clipboard, notify, tray,
filedialog, kiosk) do nothing useful on either platform.

## API

{{< api-funcs pkg="ebiten" names="NewGame,SetSafeArea" >}}

## Related

- [Kiosks and Raspberry Pi](../kiosks/) for the other target without a hardware keyboard.
- [Input and focus](../input-and-focus/) for the drawn keyboard and the focus model.
- [Concurrency](../concurrency/) for the UI goroutine.
- [`backend/ebiten` on pkg.go.dev](https://pkg.go.dev/github.com/worldiety/gift/backend/ebiten#NewGame) for
  `NewGame`, `SetSafeArea` and every field of `Config`.
