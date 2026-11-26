package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// The safe area tests of the drawn keyboard: [ui.OnScreenKeyboard] in a
// [ui.Window] that fills a phone's screen sits on the safe bottom, above the
// navigation bar, and its panel is painted on to the edge of the window
// behind it — and behind the side bars of a phone on its side — while its
// keys, its size, its hit area and the obstruction a reveal aims around stay
// where layout put them.

// kbSafe mounts the avoiding screen of keyboard_avoid_test.go and a keyboard
// in a window with the insets in, size if given, focuses the field and lets
// the keyboard finish sliding in.
func kbSafe(t *testing.T, kb ui.KeyboardView, in geom.Insets, size ...geom.Size) (*gifttest.Harness, *ui.TextEditor) {
	t.Helper()
	var printed int
	ed := ui.NewTextEditor("")
	ui.SetOnScreenKeyboard(nil, true)
	t.Cleanup(func() { ui.SetOnScreenKeyboard(nil, false) })
	h := newSafe(t, ui.Window(avoidScreen(t, ed, &printed), kb.Key("kb")).Align(geom.Bottom), in, size...)
	h.Find(gifttest.ByKey("field")).Focus()
	h.Advance(time.Second)
	return h, ed
}

// kbPanel returns the device rectangle of the keyboard's panel — the surface
// fill whose top is the top of the keyboard, drawn at the top of the keyboard
// shifted by dy — and whether there is one.
func kbPanel(h *gifttest.Harness, kb geom.Rect, dy float32) (geom.Rect, bool) {
	l := h.List()
	for i, op := range h.Ops() {
		if op.Kind != render.OpFillRect && op.Kind != render.OpFillRoundRect || op.Color != surface() {
			continue
		}
		r := l.DeviceXform(i).TransformRect(op.Bounds)
		if r.Min.Y == kb.Min.Y+dy && r.Min.X <= kb.Min.X && r.Max.X >= kb.Max.X {
			return r, true
		}
	}
	return geom.Rect{}, false
}

// TestSafeAreaKeyboardPanelReachesTheBottom is the default case: the keyboard
// ends at the safe bottom, its panel at the window's, its keys above the
// navigation bar.
func TestSafeAreaKeyboardPanelReachesTheBottom(t *testing.T) {
	h, ed := kbSafe(t, ui.OnScreenKeyboard(), geom.Insets{Bottom: 48})
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	if kb.Max.Y != safeH-48 || kb.Min.X != 0 || kb.Max.X != safeW {
		t.Fatalf("the keyboard is laid out at %v, want it on the safe bottom %v", kb, safeH-48)
	}
	panel, ok := kbPanel(h, kb, 0)
	if !ok {
		t.Fatal("no keyboard panel drawn")
	}
	if want := geom.Rc(0, kb.Min.Y, safeW, safeH); panel != want {
		t.Errorf("the keyboard panel fills %v, want %v", panel, want)
	}
	// Every key face, and so every key, stays above the navigation bar.
	keys := 0
	for _, op := range h.Ops() {
		if op.Kind == render.OpFillRoundRect && op.Bounds.Min.Y >= kb.Min.Y {
			keys++
			if op.Bounds.Max.Y > safeH-48 {
				t.Errorf("a key is drawn at %v, below the safe bottom %v", op.Bounds, safeH-48)
			}
		}
	}
	if keys < 40 {
		t.Fatalf("found %d key faces, want the keyboard's", keys)
	}
	if r, ok := ui.KeyRectForTest("q"); !ok || r.Translate(kb.Min).Max.Y > safeH-48 {
		t.Errorf("the q key is at %v (found %v), want it above the safe bottom", r.Translate(kb.Min), ok)
	}

	// The obstruction is the layout bounds: the reveal put the field above
	// the keys, not above a panel reaching to the window's bottom, and the
	// keyboard's bounds are the bounds of the window without the bar.
	if f := h.Find(gifttest.ByKey("field")).Bounds(); f.Max.Y > kb.Min.Y {
		t.Errorf("the field is at %v, under the keyboard at %v", f, kb)
	}

	// A tap on the panel behind the navigation bar is not a key.
	h.TapAt(geom.Pt(safeW/2, safeH-10))
	if got := ed.Text(); got != "" {
		t.Errorf("a tap behind the navigation bar typed %q", got)
	}
}

// TestSafeAreaKeyboardWithoutInsetsPaintsItsBounds: a window without a safe
// area, a desktop or a kiosk without a configured bezel, draws the panel
// exactly on the bounds.
func TestSafeAreaKeyboardWithoutInsetsPaintsItsBounds(t *testing.T) {
	h, _ := kbSafe(t, ui.OnScreenKeyboard(), geom.Insets{})
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	if kb.Max.Y != safeH {
		t.Fatalf("the keyboard is laid out at %v, want it on the bottom of the window", kb)
	}
	if panel, ok := kbPanel(h, kb, 0); !ok || panel != kb {
		t.Errorf("the keyboard panel fills %v (drawn %v), want its bounds %v", panel, ok, kb)
	}
}

// TestSafeAreaKeyboardLandscapeBleedsAtTheSides: a phone on its side, cutout
// on the left, navigation bar on the right; the keyboard spans the safe width
// and the panel the whole one.
func TestSafeAreaKeyboardLandscapeBleedsAtTheSides(t *testing.T) {
	in := geom.Insets{Left: 32, Right: 48, Bottom: 20}
	h, _ := kbSafe(t, ui.OnScreenKeyboard(), in, geom.Sz(safeH, safeW))
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	if kb.Min.X != 32 || kb.Max.X != safeH-48 || kb.Max.Y != safeW-20 {
		t.Fatalf("the keyboard is laid out at %v, want it in the safe area", kb)
	}
	panel, ok := kbPanel(h, kb, 0)
	if want := geom.Rc(0, kb.Min.Y, safeH, safeW); !ok || panel != want {
		t.Errorf("the keyboard panel fills %v (drawn %v), want %v", panel, ok, want)
	}
}

// TestSafeAreaKeyboardBleedsWhileSliding: on its way in and on its way out the
// panel is as tall as when it is in place, and moves with the keys, so the
// strip behind the navigation bar never shows the page under a keyboard.
func TestSafeAreaKeyboardBleedsWhileSliding(t *testing.T) {
	var printed int
	ed := ui.NewTextEditor("")
	ui.SetOnScreenKeyboard(nil, true)
	t.Cleanup(func() { ui.SetOnScreenKeyboard(nil, false) })
	h := newSafe(t, ui.Window(avoidScreen(t, ed, &printed), ui.OnScreenKeyboard().Key("kb")).Align(geom.Bottom),
		geom.Insets{Bottom: 48})
	h.Find(gifttest.ByKey("field")).Focus()
	h.Advance(100 * time.Millisecond)
	kb := h.Find(gifttest.ByKey("kb")).Bounds()

	sliding := func(when string) {
		t.Helper()
		l := h.List()
		for i, op := range h.Ops() {
			if op.Kind != render.OpFillRect || op.Color != surface() {
				continue
			}
			r := l.DeviceXform(i).TransformRect(op.Bounds)
			if r.Min.X == 0 && r.Max.X == safeW && r.Min.Y > kb.Min.Y && r.Min.Y < safeH {
				if got, want := r.Height(), kb.Height()+48; got != want {
					t.Errorf("%s the panel is %v tall at %v, want %v", when, got, r, want)
				}
				return
			}
		}
		t.Errorf("%s no sliding keyboard panel drawn", when)
	}
	sliding("100 ms in")

	h.Find(gifttest.ByKey("print")).Focus()
	h.Advance(100 * time.Millisecond)
	sliding("100 ms after the blur")
}

// TestSafeAreaRoundedKeyboardDoesNotBleed: a keyboard with a corner radius
// keeps its bounds, as every rounded background does.
func TestSafeAreaRoundedKeyboardDoesNotBleed(t *testing.T) {
	h, _ := kbSafe(t, ui.OnScreenKeyboard().CornerRadius(12), geom.Insets{Bottom: 48})
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	if panel, ok := kbPanel(h, kb, 0); !ok || panel != kb {
		t.Errorf("the rounded keyboard panel fills %v (drawn %v), want its bounds %v", panel, ok, kb)
	}
}
