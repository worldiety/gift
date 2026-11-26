package ui_test

import (
	"testing"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// The safe area tests of the top layer: a toast, the card of a popover and an
// open menu are laid out against the whole window, as everything in the top
// layer is, and are kept inside the window without the safe area of
// [gift.App.SetSafeArea] — below the status bar, above the navigation bar and
// clear of the cutout of a phone on its side — while a scrim still covers the
// whole window. The window is the 400x800 phone of safearea_test.go.

var landscapeInsets = geom.Insets{Left: 32, Right: 48}

// safeRectOf is the window of the safe area tests without the insets in.
func safeRectOf(in geom.Insets) geom.Rect { return geom.Rc(0, 0, safeW, safeH).Inset(in) }

// within fails the test when r is not inside the rectangle inside.
func within(t *testing.T, what string, r, inside geom.Rect) {
	t.Helper()
	if r.Min.X < inside.Min.X || r.Min.Y < inside.Min.Y || r.Max.X > inside.Max.X || r.Max.Y > inside.Max.Y {
		t.Fatalf("%s is %v, which is not inside the safe rectangle %v", what, r, inside)
	}
}

// toastAt mounts a toast at edge that is presented from the first frame and
// lets its entrance end.
func toastAt(t *testing.T, edge ui.ToastEdge, in geom.Insets) geom.Rect {
	t.Helper()
	h := newSafe(t, ui.Toast(ui.Box(), "Link copied").Edge(edge).Presented(true).Duration(0), in)
	h.AdvanceTicks(40, gifttest.FrameInterval)
	return card(h).LayoutBounds()
}

// TestSafeAreaToastStaysClearOfTheSystemBars: a toast at the top rests its
// margin below the status bar, one at the bottom its margin above the
// navigation bar, and a phone on its side centres it between the cutout and
// the navigation bar.
func TestSafeAreaToastStaysClearOfTheSystemBars(t *testing.T) {
	safe := safeRectOf(portraitInsets)
	top := toastAt(t, ui.ToastTop, portraitInsets)
	within(t, "the toast at the top", top, safe)
	if top.Min.Y != 24+12 {
		t.Fatalf("the toast at the top is at %v, want 12 below the status bar (y=36)", top)
	}
	bottom := toastAt(t, ui.ToastBottom, portraitInsets)
	within(t, "the toast at the bottom", bottom, safe)
	if bottom.Max.Y != safeH-48-12 {
		t.Fatalf("the toast at the bottom is at %v, want 12 above the navigation bar (y=%v)", bottom, safeH-48-12)
	}

	side := safeRectOf(landscapeInsets)
	land := toastAt(t, ui.ToastBottom, landscapeInsets)
	within(t, "the toast of a phone on its side", land, side)
	if c := (land.Min.X + land.Max.X) / 2; c != (side.Min.X+side.Max.X)/2 {
		t.Fatalf("the toast is centred on x=%v, want the centre of the safe width x=%v", c, (side.Min.X+side.Max.X)/2)
	}
	if land.Max.Y != safeH-12 {
		t.Fatalf("with no inset at the bottom the toast should end 12 above the window's edge, it is %v", land)
	}
}

// TestSafeAreaToastWithoutInsetsIsWhereItWas: no safe area, the old places.
func TestSafeAreaToastWithoutInsetsIsWhereItWas(t *testing.T) {
	top := toastAt(t, ui.ToastTop, geom.Insets{})
	bottom := toastAt(t, ui.ToastBottom, geom.Insets{})
	if top.Min.Y != 12 || bottom.Max.Y != safeH-12 || (top.Min.X+top.Max.X)/2 != safeW/2 {
		t.Fatalf("without a safe area the toasts are at %v and %v, want 12 from the edges and centred", top, bottom)
	}
}

// popoverAt mounts an anchor of 40x40 at (x, y) with a popover of a box of
// size content under it, presented from the first frame.
func popoverAt(t *testing.T, x, y float32, content geom.Size, in geom.Insets) (h *gifttest.Harness, anchor, card geom.Rect) {
	t.Helper()
	h = newSafe(t, ui.ZStack(
		ui.Popover(
			ui.Box().Frame(40, 40).Key("anchor"),
			ui.Box().Frame(content.W, content.H).Key("content"),
		).Presented(true).Padding(0).OnDismiss(func() {}),
	).Align(geom.TopLeading).PaddingInsets(geom.Insets{Left: x, Top: y}).Frame(safeW, safeH), in)
	h.AdvanceTicks(40, gifttest.FrameInterval)
	anchor = h.Find(gifttest.ByKey("anchor")).LayoutBounds()
	if anchor.Min != geom.Pt(x, y) {
		t.Fatalf("the anchor is at %v, want its corner at (%v,%v)", anchor, x, y)
	}
	return h, anchor, h.Find(gifttest.ByKey("content")).LayoutBounds()
}

// TestSafeAreaPopoverNearTheBottomFlipsAboveTheNavigationBar: a card that
// would fit between its anchor and the bottom of the glass but not between
// the anchor and the navigation bar opens above, and the same scene without a
// safe area still opens below.
func TestSafeAreaPopoverNearTheBottomFlipsAboveTheNavigationBar(t *testing.T) {
	// The anchor ends at 640; below it are 800-8-650 = 142 pixels to the
	// glass, but only 752-8-650 = 94 to the navigation bar, and the card is
	// 100 tall.
	size := geom.Sz(100, 100)
	_, anchor, content := popoverAt(t, 180, 600, size, geom.Insets{})
	if content.Min.Y < anchor.Max.Y {
		t.Fatalf("without a safe area the popover at %v should open below its anchor at %v", content, anchor)
	}
	_, anchor, content = popoverAt(t, 180, 600, size, portraitInsets)
	if content.Max.Y > anchor.Min.Y {
		t.Fatalf("the popover at %v did not open above its anchor at %v, so it reaches under the navigation bar", content, anchor)
	}
	within(t, "the popover", content, safeRectOf(portraitInsets))

	// The press that dismisses it is still the whole window: the scrim's
	// hit area did not shrink to the safe rectangle.
	dismissed := 0
	h := newSafe(t, ui.Popover(ui.Box().Frame(40, 40), ui.Box().Frame(100, 100)).
		Presented(true).OnDismiss(func() { dismissed++ }), portraitInsets)
	h.Settle()
	h.TapAt(geom.Pt(200, 10))
	if dismissed != 1 {
		t.Fatalf("a tap on the status bar dismissed the popover %d times, want once", dismissed)
	}
}

// TestSafeAreaPopoverTooTallIsClampedToTheSafeRect: a card that fits on
// neither side takes the side with more room, measured to the margin short of
// the system bar there, not of the glass.
func TestSafeAreaPopoverTooTallIsClampedToTheSafeRect(t *testing.T) {
	// The content is a scroll view of a long column, which takes the height
	// it is offered; the card is read from the frame: the rounded fill of
	// the surface colour with the panel's radius.
	h := newSafe(t, ui.ZStack(
		ui.Popover(
			ui.Box().Frame(40, 40),
			ui.VScroll(ui.Box().Frame(100, 2000)),
		).Presented(true).Padding(0),
	).Align(geom.TopLeading).PaddingInsets(geom.Insets{Left: 180, Top: 380}).Frame(safeW, safeH), portraitInsets)
	h.AdvanceTicks(40, gifttest.FrameInterval)
	var cards []geom.Rect
	for _, op := range h.Ops() {
		if op.Kind == render.OpFillRoundRect && op.Color == surface() && op.CornerRadius == 13 {
			cards = append(cards, op.Bounds)
		}
	}
	if len(cards) != 1 {
		t.Fatalf("found %d cards, want one: %v", len(cards), cards)
	}
	want := safeRectOf(portraitInsets).Inset(geom.InsetsAll(8))
	// Above the anchor are 380-10-(24+8) = 338 pixels, below it
	// (800-48-8)-(420+10) = 314: the card takes the larger side and grows
	// up to the margin below the status bar, not to the margin of the glass.
	within(t, "the clamped card", cards[0], want)
	if cards[0].Min.Y != want.Min.Y || cards[0].Max.Y != 380-10 {
		t.Fatalf("the clamped card is %v, want it from y=%v to the arrow above the anchor at y=370",
			cards[0], want.Min.Y)
	}
}

// TestSafeAreaPopoverOnItsSideStaysOutOfTheCutout: on a phone on its side an
// anchor at the right edge of the safe area gets a card that keeps the margin
// to the navigation bar on the right, and one at the left to the cutout.
func TestSafeAreaPopoverOnItsSideStaysOutOfTheCutout(t *testing.T) {
	safe := safeRectOf(landscapeInsets).Inset(geom.InsetsAll(8))
	_, _, right := popoverAt(t, safeW-48-40, 100, geom.Sz(200, 50), landscapeInsets)
	within(t, "the popover at the right", right, safe)
	_, _, left := popoverAt(t, 32, 100, geom.Sz(200, 50), landscapeInsets)
	within(t, "the popover at the left", left, safe)

	// Without the insets the same right-hand card goes on to the margin of
	// the glass.
	_, _, glass := popoverAt(t, safeW-48-40, 100, geom.Sz(200, 50), geom.Insets{})
	if glass.Max.X != safeW-8 {
		t.Fatalf("without a safe area the card at %v should end 8 short of the glass", glass)
	}
}

// menuItems is a menu tall enough not to fit below a button at the bottom.
func menuItems() []ui.MenuElement {
	return []ui.MenuElement{
		ui.MenuItem("Copy", func() {}),
		ui.MenuItem("Archive", func() {}),
		ui.MenuItem("Delete", func() {}).Destructive(),
		ui.MenuItem("Share", func() {}),
	}
}

// TestSafeAreaMenuAtTheBottomOpensAboveTheNavigationBar: a menu of a button
// laid out on the safe bottom edge by ui.Window opens above the button, every
// row inside the safe rectangle; turned on its side, a menu of a button at the
// trailing edge stays clear of the navigation bar on the right.
func TestSafeAreaMenuAtTheBottomOpensAboveTheNavigationBar(t *testing.T) {
	rows := []string{"menu-Copy", "menu-Archive", "menu-Delete", "menu-Share"}
	view := ui.Window(ui.VStack(
		ui.Spacer(),
		ui.HStack(ui.Spacer(), ui.Menu(ui.Text("More").Key("more").Padding(10), menuItems()...)),
	))
	h := newSafe(t, view, portraitInsets)
	more := h.Find(gifttest.ByKey("more"))
	if b := more.LayoutBounds(); b.Max.Y != safeH-48 {
		t.Fatalf("the button is at %v; it should sit on the safe bottom edge y=%v", b, safeH-48)
	}
	more.Tap()
	h.Settle()
	safe := safeRectOf(portraitInsets).Inset(geom.InsetsAll(8))
	for _, k := range rows {
		r := h.Find(gifttest.ByKey(k)).LayoutBounds()
		within(t, k, r, safe)
		if r.Max.Y > more.LayoutBounds().Min.Y {
			t.Fatalf("%s at %v is not above the button at %v", k, r, more.LayoutBounds())
		}
	}

	h.SetSafeArea(landscapeInsets)
	h.Settle()
	safe = safeRectOf(landscapeInsets).Inset(geom.InsetsAll(8))
	for _, k := range rows {
		within(t, k+" on its side", h.Find(gifttest.ByKey(k)).LayoutBounds(), safe)
	}
}

// TestSafeAreaContextMenuScrimCoversTheSystemBars: the dimming of a context
// menu opened by a long press covers the whole window, the status bar and the
// navigation bar included, while its rows keep to the safe rectangle.
func TestSafeAreaContextMenuScrimCoversTheSystemBars(t *testing.T) {
	view := ui.Window(ui.VStack(
		ui.Spacer(),
		ui.ContextMenu(ui.Box().Background(ui.ColorAccent).Frame(120, 60).Key("content"), menuItems()...),
	))
	h := newSafe(t, view, portraitInsets)
	h.Find(gifttest.ByKey("content")).LongPress()
	h.AdvanceTicks(40, gifttest.FrameInterval)

	window := geom.Rc(0, 0, safeW, safeH)
	var covers bool
	for _, op := range h.Ops() {
		if op.Kind != render.OpStrokeRoundRect || op.Color.A == 0 {
			continue
		}
		// The ring's outer contour is its bounds outset by half the stroke.
		outer := op.Bounds.Outset(geom.InsetsAll(op.StrokeWidth / 2))
		if outer.Min.X <= window.Min.X && outer.Min.Y <= window.Min.Y &&
			outer.Max.X >= window.Max.X && outer.Max.Y >= window.Max.Y {
			covers = true
		}
	}
	if !covers {
		t.Fatal("no scrim of the context menu covers the whole window with its system bars")
	}
	within(t, "the first row", h.Find(gifttest.ByKey("menu-Copy")).LayoutBounds(),
		safeRectOf(portraitInsets).Inset(geom.InsetsAll(8)))
}

// paintedToast is where the capsule of the toast was drawn in the last frame,
// on the device, with the transform of its entrance applied.
func paintedToast(t *testing.T, h *gifttest.Harness, rest geom.Rect) geom.Rect {
	t.Helper()
	l := h.List()
	for i, op := range l.Ops() {
		if op.Kind == render.OpFillRoundRect && op.CornerRadius == rest.Height()/2 && op.Bounds.Size() == rest.Size() {
			return l.DeviceXform(i).TransformRect(op.Bounds)
		}
	}
	t.Fatal("no capsule of the toast was painted")
	return geom.Rect{}
}

// TestSafeAreaToastEntersFromBeyondTheStatusBar: the capsule slides in from
// beyond the edge of the glass, not from the edge of the safe rectangle: in
// the frame it is presented in it lies wholly outside the window, as it does
// without a safe area, rather than under the clock or the home indicator. The
// slide moves only the paint, never the place it rests at.
func TestSafeAreaToastEntersFromBeyondTheStatusBar(t *testing.T) {
	for _, in := range []geom.Insets{{}, portraitInsets} {
		for _, edge := range []ui.ToastEdge{ui.ToastTop, ui.ToastBottom} {
			h := newSafe(t, ui.Toast(ui.Box(), "Link copied").Edge(edge).Presented(true).Duration(0), in)
			rest := card(h).LayoutBounds()
			p := paintedToast(t, h, rest)
			if edge == ui.ToastTop && (rest.Min.Y != in.Top+12 || p.Max.Y > 0) ||
				edge == ui.ToastBottom && (rest.Max.Y != safeH-in.Bottom-12 || p.Min.Y < safeH) {
				t.Fatalf("with the safe area %v the toast at edge %v starts painted at %v, resting at %v; it should start outside the window",
					in, edge, p, rest)
			}
		}
	}
}

// TestSafeAreaLargerThanTheWindowLeavesTheTopLayerWhole: a safe area wider
// than the window, which a backend may report for a frame while a phone folds,
// gives a toast and a popover no room, but no negative constraints and no
// panic.
func TestSafeAreaLargerThanTheWindowLeavesTheTopLayerWhole(t *testing.T) {
	huge := geom.Insets{Left: 300, Right: 300, Top: 500, Bottom: 500}
	h := newSafe(t, ui.ZStack(
		ui.Toast(ui.Box(), "Link copied").Presented(true).Duration(0),
		ui.Popover(ui.Box().Frame(40, 40), ui.Text("Hello")).Presented(true),
	), huge)
	h.AdvanceTicks(40, gifttest.FrameInterval)
	if b := card(h).LayoutBounds(); !(b.Width() >= 0) || !(b.Height() >= 0) {
		t.Fatalf("the toast is %v", b)
	}
}
