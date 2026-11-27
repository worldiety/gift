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

// BackListener is implemented by the Android host; gobind cannot pass a func.
type BackListener interface{ CanGoBackChanged(canGoBack bool) }

// SetBackListener replaces the listener, nil removes it; any thread.
func SetBackListener(l BackListener) { backend.SetBackListener(l) }

// Back is called by the Android host when the user went back; any thread.
func Back() { backend.Back() }

// CanGoBack reports whether gift takes the next back; any thread.
func CanGoBack() bool { return backend.CanGoBack() }

// CancelBack is called when the user abandoned the gesture; any thread.
func CancelBack() { backend.CancelBack() }

// CancelTouches is called when the system took the touches over; any thread.
func CancelTouches() { backend.CancelTouches() }
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

## Back

Android has a system back: the back button, or the swipe in from the left or right edge. The user expects it to do
what the back affordance of the application does — pop a screen, close a sheet, a dialog or a menu — and to leave
the application only on its first view. gift answers it with its own route, separate from the escape key, and the
host hands the question and the action across through the functions of the bound package above. iOS has no
system back; there is nothing to do there.

### Why it is not the escape key

Escape is a key, and a node that consumes it says nothing about whether anything happened: a
[NavigationStack](../../components/navigation/navigation-stack/) on its root screen consumes escape on purpose, so
that the key does not close something the user cannot see. Back has a third party that escape does not have, the
operating system, which leaves the application when gift does not go back. And Android wants the answer *before*
the gesture: predictive back shows, while the finger is still moving, either the page underneath or the home
screen, and an application that opted in with `enableOnBackInvokedCallback` (the default from `targetSdk` 36) gets
neither `onBackPressed` nor the back key any more, only the callback it enabled beforehand. So back is a question
first, `App.CanGoBack`, which every view answers from what it was built with, without doing anything, and an action
second, `App.Back`. Escape is unchanged.

### The route

1. When the focused field has asked for a keyboard — gift's drawn [keyboard](../../components/dialogs/keyboard/) or
   the system's — back takes the focus from the field, which puts the keyboard away, as escape does. Nothing else
   happens. `App.CanGoBack` is true for as long as such a request stands. On Android the system keyboard takes back
   for itself before the activity sees it, so this step is mostly gift's drawn keyboard; see the end of this
   section.
2. Inside the active focus trap — an open modal, sheet, popover or menu, or else the whole window — the last view in
   document order that can go back now is the candidate. A nested stack comes after the stack around it, and a
   sheet, a modal, a popover or a menu presented over the content comes after the content. A view that declines
   is skipped and hides nothing, so a stack on its root screen lets back reach a stack before it or the sheet
   around it. Covered screens, hidden and disabled views and overlays on their way out are no candidates.
3. The focused view and its ancestors are asked, innermost first. The first that can go back does, unless the
   candidate of step 2 lies inside it: then the candidate does. The focus decides between views side by side — of
   two stacks next to each other, the one being worked in pops — but not against a deeper one: a tab item tapped a
   moment ago is focused and is not inside the stack of its tab, and back still pops that stack and not the one
   around the tab bar. A focus left inside a hidden view, an overlay on its way out or outside the active focus
   trap says nothing and is ignored.
4. With nothing focused, which on a phone is most of the time, or with nothing on the focused path that can go
   back, the candidate of step 2 goes back. When there is none, the operating system goes back, which on the first
   view leaves the application.

What the views of gift do:

| View | Back |
|------|------|
| [NavigationStack](../../components/navigation/navigation-stack/) | calls `onBack` above the root screen; declines on the root, so back reaches what is around the stack |
| [Sheet](../../components/dialogs/sheet/), presented | calls `OnDismiss`; without it takes back and does nothing |
| [Modal](../../components/dialogs/modal/) and [Alert](../../components/dialogs/alert/), presented | calls `OnDismiss`; without it takes back and does nothing |
| [Popover](../../components/dialogs/popover/), open | calls `OnDismiss`; without it takes back and does nothing |
| [Menu](../../components/dialogs/menu/), [ContextMenu](../../components/dialogs/context-menu/), open | closes the menu |
| [Image Viewer](../../components/media/image-viewer/) | closes the card first, then the viewer; with `Closable(false)` only closing the viewer is blocked, back then takes the action and does nothing; does nothing while it closes |
| [File Picker](../../components/dialogs/file-picker/) | answers an open replace question with Cancel, goes up one folder, then calls `onCancel`; declines in the root without `onCancel` |
| `ui.OnBack(content, action)` | calls `action` unless `Disabled` |

Taking back and doing nothing is what an Android dialog does that is not cancelable: back never leaves an
application from behind a dialog that demands an answer. A stack that declines on its root is what makes a stack in
a sheet work: back pops the stack, and on its root it closes the sheet. Escape does not do that; see the
[NavigationStack](../../components/navigation/navigation-stack/) page.

`ui.OnBack` is the hook for navigation of your own, a wizard or a detail pane that replaces a list:

```go
return ui.OnBack(wizard, func() { step.Set(step.Get() - 1) }).Disabled(ctx.Read(step) == 0)
```

It answers back unless `Disabled`, and passes it on then; a nil `action` is the same as `Disabled(true)`.
`Disabled` is decided in the build because Android asks before the gesture, not when back arrives. The route puts
it after the views inside it, so a navigation stack inside it pops first and the `OnBack` gets back on the stack's
root screen; an `OnBack` inside a screen of a stack gets it before the stack. Apart from back it is transparent: it
is no hit target and no stop for bubbling events, so a tap on its content goes where it would go without it.

A custom view answers back with a `gift.BackHandler`, whose documentation has the contract, in one of two ways. Its
interactor implements the interface, which is how a navigation stack or a popover that already handles escape does
it. Or the element sets `Element.Back`: then the node answers back without having an interactor, so it is no hit
target. That is what `ui.OnBack` and `Modal` use, because a wrapper that only means "back goes here" must not take
a tap that belongs to the view under it.

### The back gesture

Android's back gesture is a swipe in from the left or the right edge, and the application gets its first touch
before the system recognises the gesture and cancels the touch. To gift that touch is an ordinary press, and a
press outside an open menu, popover or swipe action closes it. Without help, the back action that follows the
gesture then finds the menu gone and goes on to what is under it: the screen under the menu pops, or the
application leaves. One gesture would do two things. This was found on the Android emulator.

So a touch that comes down within 40 logical pixels of the left or the right edge of the window records the view
that would go back at that moment, and `App.CanGoBack` stays true while that record stands. When the back action
arrives and the view that would go back is still the recorded one, which is the common case, back goes as usual.
When the touch took its reason away — the menu closed as a press outside it, or a sheet the press opened covers it
— back is spent on that and does nothing more. A touch that only moved the focus, as a press on something that is
not focusable does, takes nothing away: the recorded view still goes back. The record is kept only across a touch
the platform cancelled, which the backend reports with `App.PointerCancel`. A touch at the edge that is released
was a tap, not the gesture — on the back button of a navigation bar, which sits there, or outside a menu that it
closed — and drops the record when the finger lifts: the next back is an ordinary one. So does any other press, a
touch away from the edges or a mouse press, and a gesture the user abandons, which the backend reports with
`App.BackCancel`, so that the first view does not keep back for a menu that is gone. Every new touch at the edge
records afresh, except while the finger that made the record is still down: a further finger then, at the edge or
not, belongs to the same gesture and changes nothing. While a keyboard is requested the record is the keyboard's:
the touch may put it away itself, as a press beside a text field blurs the field, and the back that follows is then
spent on the keyboard instead of also popping the screen. When the system keyboard takes the gesture itself and goes
down, which the backend reports with `App.DismissSoftKeyboard`, no back follows, and the record goes with the
keyboard.

The edge zone is a fact of the platform, which the backend reports with `App.SetBackGestureEdge`; the Ebitengine
backend reports 40 on Android, and nothing anywhere else. On a desktop, on iOS, in gifttest and in auto no touch
records anything. A test of the Android behaviour sets it itself with `h.App().SetBackGestureEdge(40)`.

### The backend

`backend.Back`, `backend.CanGoBack`, `backend.SetBackListener`, `backend.CancelBack` and `backend.CancelTouches`
take calls from any thread, also before the game exists, as `SetSafeArea` does. `Back` leaves a request in a
mailbox and wakes the game; the game performs one request per tick, in the input phase, and keeps itself awake while
more are waiting. One per tick, because the views answer from what they were built with: two quick presses pop two
screens, the second after the first has been built. After every update the game asks `App.CanGoBack` and passes a
changed answer to the listener, on its own goroutine; a new listener hears the current answer once. The answer
costs nothing on a frame where it does not change. `CancelTouches` makes the next tick end the touches that are
down with `App.PointerCancel` instead of a release, and `CancelBack` reaches `App.BackCancel` in the next
tick. With `Config.NoInput` requests and reports are dropped and the answer is false.

The answer can be a frame behind: when the user goes back in the moment gift stops being able to, gift declines,
the press does nothing, and the next one goes to Android.

### The Android host

The activity is a `ComponentActivity` (androidx.activity 1.8 or later) and opts in to predictive back in the
manifest:

```xml
<application android:enableOnBackInvokedCallback="true" ...>
```

It registers two `OnBackPressedCallback`s. The one added last is asked first: it is enabled exactly while gift can
go back, calls `Mobile.back()`, and `Mobile.cancelBack()` when the user abandons the gesture. Below it, a second
one, `leave`, is always enabled and moves the task to the background with `moveTaskToBack(true)`. Neither calls
`finish()`: Ebitengine cannot set its view up a second time in the same process. The application stays where it was,
with its state, and comes back as it was left.

`leave` is always enabled, on every Android version, because leaving it to the system finishes the activity. Since
Android 12 a root activity's task is documented to go to the background by itself when no callback takes back, but
on the Android 16 emulator (API 36, with `targetSdk` 35 and 36 and `enableOnBackInvokedCallback`) the system
finished the root activity instead. The next launch then creates a second Ebitengine view in the same process,
which fails ("atlas: requestToRestoreGPUResources was called unexpectedly") and ends the process. The cost of the
always enabled callback is the predictive animation back to the home screen, which Android shows only when no
callback is enabled; the animation to the page underneath, inside gift, is unaffected.

```kotlin
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import androidx.activity.ComponentActivity
import androidx.activity.OnBackPressedCallback
import com.example.app.mobile.Mobile

class MainActivity : ComponentActivity() {
    private val main = Handler(Looper.getMainLooper())

    // Asked first, because it is added last: enabled exactly while gift can go back.
    private val giftBack = object : OnBackPressedCallback(false) {
        override fun handleOnBackPressed() = Mobile.back()
        override fun handleOnBackCancelled() = Mobile.cancelBack()
    }

    // Asked when gift cannot go back; always enabled, see above. Never finish():
    // Ebitengine cannot recreate its view in this process.
    private val leave = object : OnBackPressedCallback(true) {
        override fun handleOnBackPressed() {
            moveTaskToBack(true)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main) // holds the GiftView below

        onBackPressedDispatcher.addCallback(this, leave)
        onBackPressedDispatcher.addCallback(this, giftBack)

        // Called on gift's goroutine. Reading the latest value on the main thread
        // keeps two quick changes from arriving in the wrong order.
        Mobile.setBackListener { main.post { giftBack.isEnabled = Mobile.canGoBack() } }
    }

    override fun onDestroy() {
        Mobile.setBackListener(null)
        super.onDestroy()
    }

    // onPause and onResume suspend and resume the game, see below.
}
```

`EbitenView` consumes every key, and before Android 13, or with a `targetSdk` below 36 and no opt-in, back still
arrives as a key: it has to reach the activity, whose `onBackPressed` hands it to the same callbacks. The subclass
of `EbitenView` that reports the safe area passes it on, together with the volume keys.

It also handles `MotionEvent.ACTION_CANCEL`, which Ebitengine (v2.10.5) ignores. The back gesture cancels the edge
touch the application already got, and without the cancel that touch stays pressed in Ebitengine, so the next tap
is swallowed. The subclass first calls `Mobile.cancelTouches()`, so that gift hears a cancel and not a release, and
then releases the pointers in Ebitengine; where does not matter:

```kotlin
import android.content.Context
import android.util.AttributeSet
import android.view.KeyEvent
import android.view.MotionEvent
import com.example.app.ebitenmobileview.Ebitenmobileview
import com.example.app.mobile.EbitenView
import com.example.app.mobile.Mobile

class GiftView(context: Context, attrs: AttributeSet?) : EbitenView(context, attrs) {
    override fun onKeyDown(keyCode: Int, event: KeyEvent): Boolean =
        if (passedOn(keyCode)) false else super.onKeyDown(keyCode, event)

    override fun onKeyUp(keyCode: Int, event: KeyEvent): Boolean =
        if (passedOn(keyCode)) false else super.onKeyUp(keyCode, event)

    // Ebitengine ignores ACTION_CANCEL; the back gesture sends it for the edge touch.
    override fun onTouchEvent(e: MotionEvent): Boolean {
        if (e.actionMasked == MotionEvent.ACTION_CANCEL) {
            Mobile.cancelTouches() // first: gift hears a cancel, not a release
            for (i in 0 until e.pointerCount) {
                val id = e.getPointerId(i).toLong()
                Ebitenmobileview.updateTouchesOnAndroid(MotionEvent.ACTION_UP.toLong(), id, 0.0, 0.0)
            }
            return true
        }
        return super.onTouchEvent(e)
    }

    private fun passedOn(keyCode: Int) = when (keyCode) {
        KeyEvent.KEYCODE_BACK,
        KeyEvent.KEYCODE_VOLUME_UP, KeyEvent.KEYCODE_VOLUME_DOWN, KeyEvent.KEYCODE_VOLUME_MUTE -> true
        else -> false
    }

    // onApplyWindowInsets and onLayout report the safe area, see above.
}
```

`Ebitenmobileview` is generated by `ebitenmobile bind` into the package `ebitenmobileview` under the `-javapkg`
given, next to the bound package's own class.

Back while the system keyboard is up never reaches the activity: the keyboard takes it and goes down, Ebitengine
reports the end of the text input, and gift takes the focus from the field. The next back goes to gift.

In tests, `Harness.Back` and `Harness.CanGoBack` go back as the backend does, and the automation interface has a
`back` step; see [Testing](../testing/).

## The host

On Android the activity declares

```xml
android:configChanges="orientation|screenSize|smallestScreenSize|screenLayout|density|keyboard|keyboardHidden|navigation|uiMode"
```

so that a rotation, a fold, a theme change or a hardware keyboard that comes or goes is a relayout and not a new
activity. It sets `windowSoftInputMode="adjustNothing"`, because Ebitengine shifts the picture itself to keep the
caret above the system keyboard, and calls `suspendGame` and `resumeGame` from `onPause` and `onResume`.
`EbitenView` consumes every key, so its subclass hands the back key and the volume keys back to Android, and the
activity goes back as [Back](#back) describes: gift first, then the task to the background through a callback that
is always enabled, never `finish()`, because Ebitengine cannot carry its GPU resources into a new view in the same
process. The subclass also reports a `MotionEvent.ACTION_CANCEL`, which Ebitengine ignores, and releases its
touches.

On iOS the application creates exactly one subclass of the generated `MobileEbitenViewController` (the prefix is the
bound package's name) per process and reuses it when its scene reconnects, because Ebitengine attaches its Metal
layer only once. It suspends and resumes the game from the scene delegate. iOS paints in sRGB; see
[Color Management](../color-management/).

Packages that load desktop libraries (video, typst, turbojpeg) or talk to a desktop (clipboard, notify, tray,
filedialog, kiosk) do nothing useful on either platform.

## API

{{< api-funcs pkg="ebiten" names="NewGame,SetSafeArea,Back,CanGoBack,SetBackListener,CancelBack,CancelTouches" >}}

## Related

- [Kiosks and Raspberry Pi](../kiosks/) for the other target without a hardware keyboard.
- [Input and focus](../input-and-focus/) for the drawn keyboard and the focus model.
- [Concurrency](../concurrency/) for the UI goroutine.
- [`backend/ebiten` on pkg.go.dev](https://pkg.go.dev/github.com/worldiety/gift/backend/ebiten#NewGame) for
  `NewGame`, `SetSafeArea`, `Back`, `CanGoBack`, `SetBackListener`, `CancelBack`, `CancelTouches` and every field
  of `Config`.
