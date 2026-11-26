package ui

import (
	"github.com/worldiety/gift"
)

// Window wraps the root view of an application in the window background.
//
//	func screen(ctx *gift.Context) gift.View {
//		return ui.Window(ui.VStack(/* ... */))
//	}
//
//	app := gift.New(gift.Options{Root: screen})
//
// # Why an application has to say this at all
//
// Because gift draws exactly what the view tree says and nothing else. There
// is one display list, no per-widget render target and no window chrome; the
// project plan, section 6, adds that Ebitengine clears the screen every frame,
// and the colour it clears to is transparent black. A tree of cards on nothing
// therefore does not produce "cards on the default background" — it produces
// cards on whatever the framebuffer holds, which on a desktop is a composited
// hole and on a Raspberry Pi kiosk is the console that was there before.
//
// That was a real defect and it is the reason this function exists. Four demo
// screens shipped with thirty to forty per cent of their pixels at
// {0, 0, 0, 0}, and every golden image of them looked correct, because the
// test harness cleared its own canvas to a colour of its own before rendering.
// A picture of a background nobody paints is the one defect a golden cannot
// report.
//
// # Why it is here and not in gift
//
// [gift.App] has no notion of an appearance. It owns a tree, a layout and a
// display list, and a colour it painted on its own behalf would be a colour no
// view asked for — invisible to the gifttest harness and to
// every assertion in this module, all of which look at the display list. A
// background that is a view is a background the tests can see, which is
// exactly the property that was missing.
//
// So the obligation is the application's, and this function is the whole of
// it. Harness.AssertOpaque in gifttest is the gate that says it was met.
//
// # What it draws
//
// A greedy [Box] filled with [ColorBackground], with content on top of it in
// the order given, in a [ZStack]. The Box is the documented idiom of
// [BoxView]: a ZStack hands every child loose constraints, bounded wherever
// its own are, so an unframed Box takes the whole viewport whatever the
// content does. That holds as long as the Window itself is given bounded
// constraints, which it always is as the root of the tree; nested in an
// unbounded axis, the Box collapses on that axis like any other. Painting the
// ZStack's own background instead would only cover the union of the children,
// which is the bug again for any tree that does not fill the window.
//
// The result is an ordinary [Overlay], so the alignment and the modifiers of
// that type are available:
//
//	return ui.Window(body, ui.OnScreenKeyboard()).Align(geom.Bottom)
//
// # The on-screen keyboard makes room for itself
//
// With an [OnScreenKeyboard] among the content, as in the example above, the
// content is laid out in the part of the window above the keyboard while it is
// showing, and gets the whole window back when it goes away: scroll views get
// a shorter viewport, a button pinned to the bottom of a screen stays above
// the keys, and a [Modal] sheet is centred in the remaining space. The page
// background stays full size behind the keyboard. That is the default of every
// [ZStack] holding a keyboard; [Overlay.AvoidKeyboard] has the reasons and the
// way to turn it off.
//
// # The system bars
//
// A phone draws its status bar, its navigation bar and the cutout of its
// camera on top of whatever the application draws, and an application that
// draws edge to edge — every gift application on a phone does — is told by
// how much, per edge: the safe area of [gift.App.SetSafeArea]. A Window that
// is the root of the tree honours it the way it honours the keyboard. The
// page background still covers the whole window, bars included, and every
// other child — the content, a [Modal], the [OnScreenKeyboard] — is laid out
// in the window without the safe area, so that nothing anybody has to read or
// press is under a bar:
//
//	ui.Window(ui.TabBar(sel, pick, tabs...), ui.OnScreenKeyboard()).Align(geom.Bottom)
//
// is a complete edge to edge screen. The tab bar ends above the navigation
// bar and the bar of a [NavigationStack] begins below the status bar, and
// both paint their surface on to the edge of the window behind them, so the
// bars of the system lie on the bars of the application rather than on a
// strip of page. A bar of the application's own does the same with
// [Stack.BleedSafeArea]. The keyboard sits on the safe bottom edge and the
// content makes room for it inside the safe rectangle, so a phone with its
// keyboard up is short by the keyboard and the navigation bar under it, not
// by the navigation bar twice.
//
// Only a Window that is given the whole window does this. The root is; a
// Window nested in something smaller — a preview in a gallery, a framed box
// in a test — is not at the edge of the screen and applies no safe area, so
// nothing is ever padded twice. On a desktop or a kiosk the safe area is zero
// unless the application configured one for a bezel, and a Window then lays
// out exactly as it did before there was one.
//
// [Overlay.IgnoresSafeArea] gives the content the whole window, for a camera
// preview, a map or a video that should be cut by the bars rather than framed
// by them; the application then reads [gift.Context.SafeArea] and keeps its
// controls clear itself.
//
// # The two-level hierarchy this makes real
//
// [ColorBackground] is the page and [ColorSurface] is what is raised on it —
// a [Card], a [Modal] sheet, the bar of a [TabBar] or a [NavigationStack].
// Until something painted the page, every one of those sat on nothing and the
// distinction between the two roles existed only in the palette: tinting
// [ColorBackground] magenta at runtime changed zero pixels on all four screens
// of cmd/example-kitchensink.
func Window(content ...gift.View) Overlay {
	// A fresh slice, because the ownership rule of the project plan,
	// section 4, gives a container its children slice for keeps and the
	// caller's variadic array must not become it.
	children := make([]gift.View, 0, len(content)+1)
	children = append(children, Box().Background(ColorBackground))
	children = append(children, content...)
	o := ZStack(children...)
	// The plate is the one child that stays behind the keyboard when the
	// others make room for it; see [Overlay.AvoidKeyboard].
	o.plate = true
	return o
}
