package ui_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// The keyboard avoidance tests: a [ui.Window] (or any [ui.ZStack]) holding an
// on-screen keyboard lays the rest of its children out in the space above the
// keyboard while it is showing; see [ui.Overlay.AvoidKeyboard].
//
// The window is the 800x480 panel the defect was found on. The keyboard is 260
// of its 480 pixels, which is the proportion that made the overlap hurt: more
// than half of every screen, with its bottom controls, disappeared while
// somebody typed.

const (
	avoidW = float32(800)
	avoidH = float32(480)
)

// avoidScreen is a photo booth screen in miniature: a scrolling inspector that
// takes whatever height is left, and a print button pinned below it. The last
// row of the inspector is a text field with nothing underneath it, which is
// the one field the obstruction reveal alone could never lift.
func avoidScreen(t testing.TB, ed *ui.TextEditor, printed *int) gift.View {
	t.Helper()
	font := loadTestFont(t)
	rows := make([]gift.View, 0, 13)
	for i := range 12 {
		rows = append(rows, ui.Box().Frame(geom.Unbounded(), 40).
			Background(ui.RGB(30, 30, 30)).Key(string(rune('a'+i))))
	}
	rows = append(rows, ui.TextField(ed).Font(font).FontSize(16).
		Frame(200, geom.Unbounded()).Key("field"))
	return ui.VStack(
		ui.VScroll(ui.VStack(rows...).Gap(8).Padding(8)).Key("inspector").Flex(1),
		ui.Button(ui.Text("Print").Font(font), func() { *printed++ }).
			MinHeight(ui.ControlHitTarget).Key("print"),
	).Key("screen")
}

// newAvoiding mounts view in an 800x480 harness with the kiosk switch on, and
// restores the switch afterwards; see [newKeyboard] for why the restore is not
// tidiness.
func newAvoiding(t testing.TB, view gift.View) *gifttest.Harness {
	t.Helper()
	ui.SetOnScreenKeyboard(nil, true)
	t.Cleanup(func() { ui.SetOnScreenKeyboard(nil, false) })
	return gifttest.New(t, gifttest.Options{
		View: view,
		Size: geom.Sz(avoidW, avoidH),
		Font: loadTestFont(t),
	})
}

// dismissKeyboard presses the keyboard's own dismiss key, which is how a kiosk
// user without a physical keyboard puts it away.
func dismissKeyboard(t testing.TB, h *gifttest.Harness) {
	t.Helper()
	r, ok := ui.KeyRectForTest("⌄")
	if !ok {
		t.Fatal("no dismiss key; the keyboard is not showing")
	}
	b := h.Find(gifttest.ByKey("kb")).Bounds()
	h.ClickAt(geom.Pt(b.Min.X+r.Min.X+r.Width()/2, b.Min.Y+r.Min.Y+r.Height()/2))
	h.Settle()
}

// TestTheWindowContentShrinksByTheKeyboardAndGrowsBack is the mechanism
// itself: the content's height is the window's minus
// [ui.OnScreenKeyboardHeight] while the keyboard is up, the content ends
// exactly where the keyboard begins, and both come back when it goes away.
func TestTheWindowContentShrinksByTheKeyboardAndGrowsBack(t *testing.T) {
	var printed int
	ed := ui.NewTextEditor("")
	h := newAvoiding(t, ui.Window(
		avoidScreen(t, ed, &printed),
		ui.OnScreenKeyboard().Key("kb"),
	).Align(geom.Bottom))

	screen := func() geom.Rect { return h.Find(gifttest.ByKey("screen")).Bounds() }
	if got := screen(); got.Height() != avoidH || got.Min.Y != 0 {
		t.Fatalf("before any focus the screen is %v; want the whole %v tall window", got, avoidH)
	}

	h.Find(gifttest.ByKey("field")).Focus()
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	if kb.Height() != ui.OnScreenKeyboardHeight() {
		t.Fatalf("the keyboard is %v tall, want %v; the fixture is wrong", kb.Height(), ui.OnScreenKeyboardHeight())
	}
	got := screen()
	if want := avoidH - ui.OnScreenKeyboardHeight(); got.Height() != want {
		t.Fatalf("with the keyboard up the screen is %v tall, want %v: the window height less the keyboard's",
			got.Height(), want)
	}
	if got.Min.Y != 0 || got.Max.Y != kb.Min.Y {
		t.Fatalf("the screen is %v and the keyboard starts at y=%v; the screen should end exactly there",
			got, kb.Min.Y)
	}

	dismissKeyboard(t, h)
	if b := h.Find(gifttest.ByKey("kb")).LayoutBounds(); b.Height() != 0 {
		t.Fatalf("the dismiss key left a keyboard %v tall", b.Height())
	}
	if got := screen(); got.Height() != avoidH {
		t.Fatalf("after the keyboard went away the screen is %v tall; it should have the whole %v back",
			got.Height(), avoidH)
	}
}

// TestABottomPinnedButtonStaysAboveTheKeyboard is the defect as the photo booth
// saw it: the print button under the inspector was behind the keyboard for as
// long as anybody typed. It has to be above the keyboard's top edge, and it
// has to be pressable there, which is the part a geometric assertion alone
// would not prove.
func TestABottomPinnedButtonStaysAboveTheKeyboard(t *testing.T) {
	var printed int
	ed := ui.NewTextEditor("")
	h := newAvoiding(t, ui.Window(
		avoidScreen(t, ed, &printed),
		ui.OnScreenKeyboard().Key("kb"),
	).Align(geom.Bottom))

	h.Find(gifttest.ByKey("field")).Focus()
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	btn := h.Find(gifttest.ByKey("print")).Bounds()
	if !(kb.Height() > 0) {
		t.Fatal("no keyboard, so there is nothing this test can show")
	}
	if btn.Height() <= 0 || btn.Max.Y > kb.Min.Y {
		t.Fatalf("the print button is %v and the keyboard starts at y=%v; the button is behind it",
			btn, kb.Min.Y)
	}
	h.Find(gifttest.ByKey("print")).Click()
	if printed != 1 {
		t.Fatalf("a click on the print button with the keyboard up printed %d times, want 1", printed)
	}
}

// TestTheLastFieldOfAFormIsRevealedAboveTheKeyboard is the case the
// obstruction reveal could not handle and the reason applications padded their
// scroll views with a keyboard sized spacer: the field is the last row, with
// no content below it to scroll up. With the viewport really shorter, the
// ordinary reveal is enough.
func TestTheLastFieldOfAFormIsRevealedAboveTheKeyboard(t *testing.T) {
	var printed int
	ed := ui.NewTextEditor("")
	h := newAvoiding(t, ui.Window(
		avoidScreen(t, ed, &printed),
		ui.OnScreenKeyboard().Key("kb"),
	).Align(geom.Bottom))

	h.Find(gifttest.ByKey("field")).Focus()
	field := h.Find(gifttest.ByKey("field"))
	fb, kb := field.Bounds(), h.Find(gifttest.ByKey("kb")).Bounds()
	if fb.Max.Y > kb.Min.Y {
		t.Fatalf("the field ends at y=%v and the keyboard starts at y=%v", fb.Max.Y, kb.Min.Y)
	}
	field.AssertVisible()

	h.TypeText("ok")
	if got := ed.Text(); got != "ok" {
		t.Fatalf("the field holds %q, want %q", got, "ok")
	}
}

// TestAFieldInAModalSheetStaysAboveTheKeyboard is the modal composition
// [ui.Overlay.AvoidKeyboard] documents: the modal is inside the window, next
// to the keyboard, and so its centred sheet is centred in the space above the
// keyboard. Without that, a sheet 200 tall centred in 480 ends at y=340 and
// the keyboard starts at y=220.
func TestAFieldInAModalSheetStaysAboveTheKeyboard(t *testing.T) {
	font := loadTestFont(t)
	ed := ui.NewTextEditor("")
	sheet := ui.VStack(
		ui.Text("Name").Font(font),
		ui.Spacer(),
		ui.TextField(ed).Font(font).FontSize(16).Frame(200, geom.Unbounded()).Key("field"),
	).Padding(12).Frame(320, 200).Background(ui.ColorSurface).Key("sheet")

	h := newAvoiding(t, ui.Window(
		ui.Modal(ui.Text("behind").Font(font), sheet),
		ui.OnScreenKeyboard().Key("kb"),
	).Align(geom.Bottom))

	before := h.Find(gifttest.ByKey("sheet")).Bounds()
	if before.Max.Y <= avoidH-ui.OnScreenKeyboardHeight() {
		t.Fatalf("the sheet is %v before the keyboard, which would not reach under a keyboard; "+
			"the fixture proves nothing", before)
	}

	h.Find(gifttest.ByKey("field")).Focus()
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	sb := h.Find(gifttest.ByKey("sheet")).Bounds()
	fb := h.Find(gifttest.ByKey("field")).Bounds()
	if !(kb.Height() > 0) {
		t.Fatal("no keyboard, so there is nothing this test can show")
	}
	if sb.Max.Y > kb.Min.Y || fb.Max.Y > kb.Min.Y {
		t.Fatalf("the sheet is %v and its field %v, the keyboard starts at y=%v", sb, fb, kb.Min.Y)
	}
	if sb.Min.Y < 0 {
		t.Fatalf("the sheet starts at y=%v, above the window", sb.Min.Y)
	}
	// Centred in the band, not merely somewhere above the keyboard.
	if got, want := (sb.Min.Y+sb.Max.Y)/2, kb.Min.Y/2; got != want {
		t.Fatalf("the sheet's centre is at y=%v, want %v: the middle of the space above the keyboard",
			got, want)
	}
}

// TestAvoidKeyboardFalseKeepsTheOverlap is the opt out: the content keeps the
// whole window and the keyboard covers its lower part, which is the behaviour
// every overlay had before avoidance existed.
func TestAvoidKeyboardFalseKeepsTheOverlap(t *testing.T) {
	var printed int
	ed := ui.NewTextEditor("")
	h := newAvoiding(t, ui.Window(
		avoidScreen(t, ed, &printed),
		ui.OnScreenKeyboard().Key("kb"),
	).Align(geom.Bottom).AvoidKeyboard(false))

	h.Find(gifttest.ByKey("field")).Focus()
	if kb := h.Find(gifttest.ByKey("kb")).Bounds(); !(kb.Height() > 0) {
		t.Fatal("no keyboard; the fixture is wrong")
	}
	if got := h.Find(gifttest.ByKey("screen")).Bounds(); got.Height() != avoidH {
		t.Fatalf("with AvoidKeyboard(false) the screen is %v tall, want the whole %v", got.Height(), avoidH)
	}
}

// TestTheWindowBackgroundStaysBehindTheKeyboard. The plate of [ui.Window] is
// the one child that does not make room: it is what paints the page, and a
// plate that shrank would leave the area under a keyboard with a translucent or
// rounded panel unpainted — the defect [ui.Window] exists to prevent.
func TestTheWindowBackgroundStaysBehindTheKeyboard(t *testing.T) {
	var printed int
	ed := ui.NewTextEditor("")
	h := newAvoiding(t, ui.Window(
		avoidScreen(t, ed, &printed),
		ui.OnScreenKeyboard().Key("kb"),
	).Align(geom.Bottom))

	h.Find(gifttest.ByKey("field")).Focus()
	plate := h.First(gifttest.ByType("ui.Box")).Bounds()
	if want := geom.Rc(0, 0, avoidW, avoidH); plate != want {
		t.Fatalf("with the keyboard up the window background is %v, want the whole window %v", plate, want)
	}
}

// TestAKeyboardAlignedToTheTopPushesTheContentDown. The reserved edge follows
// the overlay's alignment, because that is where the keyboard is: a window
// without an alignment puts its keyboard at the top, and the content has to
// start below it rather than above it.
func TestAKeyboardAlignedToTheTopPushesTheContentDown(t *testing.T) {
	var printed int
	ed := ui.NewTextEditor("")
	h := newAvoiding(t, ui.Window(
		avoidScreen(t, ed, &printed),
		ui.OnScreenKeyboard().Key("kb"),
	))

	h.Find(gifttest.ByKey("field")).Focus()
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	got := h.Find(gifttest.ByKey("screen")).Bounds()
	if kb.Min.Y != 0 || !(kb.Height() > 0) {
		t.Fatalf("the keyboard is %v; the fixture expects it at the top of the window", kb)
	}
	if got.Min.Y != kb.Max.Y || got.Max.Y != avoidH {
		t.Fatalf("the screen is %v; want it to start under the keyboard at y=%v and end at the "+
			"bottom of the window", got, kb.Max.Y)
	}
}
