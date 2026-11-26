package ui_test

import (
	"github.com/worldiety/gift"
	"reflect"
	"testing"
	"time"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// The safe area tests of the modal presentations: the surface of a [ui.Sheet]
// reaches past the safe area to the edge of the window while its content
// stays inside, the scrim of a sheet and of a [ui.Modal] dims the system bars
// as well, and a modal dialog is centred in the safe area.

// sheetScrim is the wash of the sheets and modals here, a colour of its own
// so that the scrim's fill can be told from every other.
var sheetScrim = markerColor(7)

// sheetSafe mounts a presented sheet inside a Window, with marker 2 as the
// sheet's content under the key "sheet-content", and runs its opening to the
// end. A tap on the scrim counts in dismissed.
func sheetSafe(t *testing.T, in geom.Insets, size geom.Size, dismissed *int) *gifttest.Harness {
	t.Helper()
	h := newSafe(t, ui.Window(
		ui.Sheet(ui.Box().Key("behind"),
			ui.Box().Background(markerColor(2)).Key("sheet-content")).
			Scrim(sheetScrim).
			OnDismiss(func() { *dismissed++ }),
	), in, size)
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)
	return h
}

// deviceFills is every plain or rounded fill of colour c, where it lands on
// the device.
func deviceFills(h *gifttest.Harness, c ui.Color) []geom.Rect {
	want := render.Color(ui.ResolveColor(c))
	list := h.List()
	var out []geom.Rect
	for i, op := range h.Ops() {
		if (op.Kind == render.OpFillRect || op.Kind == render.OpFillRoundRect) && op.Color == want {
			out = append(out, list.DeviceXform(i).TransformRect(op.Bounds))
		}
	}
	return out
}

// sheetFace is the rounded surface of the open sheet.
func sheetFace(t *testing.T, h *gifttest.Harness) geom.Rect {
	t.Helper()
	want := surface()
	list := h.List()
	for i, op := range h.Ops() {
		if op.Kind == render.OpFillRoundRect && op.Color == want {
			return list.DeviceXform(i).TransformRect(op.Bounds)
		}
	}
	t.Fatal("the sheet's surface is not in the display list")
	return geom.Rect{}
}

func onlyFill(t *testing.T, what string, got []geom.Rect, want geom.Rect) {
	t.Helper()
	if len(got) != 1 || got[0] != want {
		t.Errorf("%s is %v, want one fill of %v", what, got, want)
	}
}

// TestSafeAreaSheetSurfaceReachesTheBottomOfTheWindow is a phone held upright:
// the face runs on behind the navigation bar, the content ends on top of it.
func TestSafeAreaSheetSurfaceReachesTheBottomOfTheWindow(t *testing.T) {
	var dismissed int
	h := sheetSafe(t, portraitInsets, geom.Sz(safeW, safeH), &dismissed)

	// The panel is 0.9 of the safe height, 728, on the safe bottom edge.
	panelTop := safeH - 48 - float32(655)
	content := h.Find(gifttest.ByKey("sheet-content")).Bounds()
	if want := geom.Rc(0, panelTop, safeW, safeH-48); content != want {
		t.Errorf("the sheet's content is %v, want %v: the panel, above the navigation bar", content, want)
	}
	face := sheetFace(t, h)
	if face.Min.Y != panelTop || face.Max.Y < safeH || face.Min.X != 0 || face.Max.X != safeW {
		t.Errorf("the sheet's surface is %v; it has to start at the panel's top, %v, and reach "+
			"the bottom of the window at %v across its whole width", face, panelTop, safeH)
	}
	onlyFill(t, "the scrim", deviceFills(h, sheetScrim), geom.Rc(0, 0, safeW, safeH))

	// The tap area of the scrim is still the safe rectangle's.
	h.TapAt(geom.Pt(200, 30))
	h.Settle()
	if dismissed != 1 {
		t.Errorf("a tap on the scrim just below the status bar dismissed %d times, want 1", dismissed)
	}
}

// TestSafeAreaSheetOnItsSideReachesBothSides is the phone turned, with the
// cutout on the left and the navigation bar on the right: a sheet as wide as
// the safe area grows into both, its content does not.
func TestSafeAreaSheetOnItsSideReachesBothSides(t *testing.T) {
	var dismissed int
	in := geom.Insets{Left: 32, Right: 48}
	size := geom.Sz(800, 400) // a safe width of 720, the sheet's maximum
	h := sheetSafe(t, in, size, &dismissed)

	content := h.Find(gifttest.ByKey("sheet-content")).Bounds()
	if content.Min.X != 32 || content.Max.X != 800-48 || content.Max.Y != 400 {
		t.Errorf("the sheet's content is %v, want x=32..752 down to the bottom edge", content)
	}
	face := sheetFace(t, h)
	if face.Min.X != 0 || face.Max.X != 800 || face.Min.Y != content.Min.Y {
		t.Errorf("the sheet's surface is %v, want x=0..800 from the panel's top at %v", face, content.Min.Y)
	}
	if face.Max.Y != 400+14 {
		t.Errorf("the sheet's surface ends at y=%v; with no bottom inset it hangs the rounding "+
			"strip below the window and no more, to %v", face.Max.Y, 400+14)
	}
	onlyFill(t, "the scrim", deviceFills(h, sheetScrim), geom.Rc(0, 0, 800, 400))
}

// TestSafeAreaNarrowSheetOnItsSideKeepsItsSides: a sheet narrower than the
// safe area is centred in it and touches neither side, so it grows into
// neither.
func TestSafeAreaNarrowSheetOnItsSideKeepsItsSides(t *testing.T) {
	var dismissed int
	in := geom.Insets{Left: 32, Right: 48}
	h := sheetSafe(t, in, geom.Sz(1000, 400), &dismissed)

	content := h.Find(gifttest.ByKey("sheet-content")).Bounds()
	if content.Width() != 720 || content.Min.X != 32+(920-720)/2 {
		t.Fatalf("the sheet's content is %v, want 720 wide and centred in the safe area", content)
	}
	face := sheetFace(t, h)
	if face.Min.X != content.Min.X || face.Max.X != content.Max.X {
		t.Errorf("the surface of a sheet narrower than the safe area is x=%v..%v, want the "+
			"content's x=%v..%v", face.Min.X, face.Max.X, content.Min.X, content.Max.X)
	}
}

// TestSafeAreaSheetOutsideAWindowKeepsItsFace: a sheet that is not presented
// by a Window — here the root of the tree — is not in the safe area, and its
// surface is what it was without one.
func TestSafeAreaSheetOutsideAWindowKeepsItsFace(t *testing.T) {
	var dismissed int
	h := newSafe(t, ui.Sheet(ui.Box(), ui.Box().Background(markerColor(2)).Key("sheet-content")).
		Scrim(sheetScrim).OnDismiss(func() { dismissed++ }), portraitInsets)
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)

	face := sheetFace(t, h)
	if want := geom.Rc(0, 80, safeW, safeH+14); face != want {
		t.Errorf("the surface of a sheet outside a Window is %v, want %v", face, want)
	}
	onlyFill(t, "the scrim", deviceFills(h, sheetScrim), geom.Rc(0, 0, safeW, safeH))
}

// TestSafeAreaSheetWithoutInsetsIsUnchanged: with no safe area a sheet in a
// Window draws exactly what it draws with a safe area that is set back to
// zero, and its face is the panel and the rounding strip.
func TestSafeAreaSheetWithoutInsetsIsUnchanged(t *testing.T) {
	var dismissed int
	h := sheetSafe(t, geom.Insets{}, geom.Sz(safeW, safeH), &dismissed)
	if want := geom.Rc(0, 80, safeW, safeH+14); sheetFace(t, h) != want {
		t.Errorf("the surface without a safe area is %v, want %v", sheetFace(t, h), want)
	}
	onlyFill(t, "the scrim", deviceFills(h, sheetScrim), geom.Rc(0, 0, safeW, safeH))
	before := append([]render.Op(nil), h.Ops()...)

	h.SetSafeArea(portraitInsets)
	h.SetSafeArea(geom.Insets{})
	if after := h.Ops(); !reflect.DeepEqual(before, after) {
		t.Errorf("a sheet draws %d operations after the safe area came and went, %d before",
			len(after), len(before))
	}
}

// TestSafeAreaModalIsCentredInTheSafeAreaAndDimsTheBars: the dialog of a modal
// sits in the middle of the safe area, its scrim covers the whole window.
func TestSafeAreaModalIsCentredInTheSafeAreaAndDimsTheBars(t *testing.T) {
	h := newSafe(t, ui.Window(
		ui.Modal(ui.Box(), ui.Box().Frame(100, 100).Key("dialog-card")).Scrim(sheetScrim),
	), portraitInsets)
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)

	card := h.Find(gifttest.ByKey("dialog-card")).Bounds()
	mid := 24 + (safeH-24-48)/2
	if want := geom.Rc(150, mid-50, 250, mid+50); card != want {
		t.Errorf("the modal's dialog is %v, want %v: centred between the bars", card, want)
	}
	onlyFill(t, "the modal's scrim", deviceFills(h, sheetScrim), geom.Rc(0, 0, safeW, safeH))
}

// TestSafeAreaModalScrimOnItsSide is the same with the cutout and the
// navigation bar at the sides.
func TestSafeAreaModalScrimOnItsSide(t *testing.T) {
	h := newSafe(t, ui.Window(
		ui.Modal(ui.Box(), ui.Box().Frame(100, 100).Key("dialog-card")).Scrim(sheetScrim),
	), geom.Insets{Left: 32, Right: 48}, geom.Sz(800, 400))
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)

	card := h.Find(gifttest.ByKey("dialog-card")).Bounds()
	mid := 32 + float32(800-32-48)/2
	if want := geom.Rc(mid-50, 150, mid+50, 250); card != want {
		t.Errorf("the modal's dialog is %v, want %v", card, want)
	}
	onlyFill(t, "the modal's scrim", deviceFills(h, sheetScrim), geom.Rc(0, 0, 800, 400))
}

// slidingSheet mounts a presented sheet inside a Window whose presentation is
// state, runs the opening to the end, taps the scrim to close it and advances
// a quarter of the way out, so that the panel is a sliding layer.
func slidingSheet(t *testing.T, in geom.Insets, size geom.Size) *gifttest.Harness {
	t.Helper()
	root := func(ctx *gift.Context) gift.View {
		open := ctx.State("open", true)
		return ui.Window(ui.Sheet(ui.Box(), ui.Box().Background(markerColor(2)).Key("sheet-content")).
			Presented(ctx.Read(open)).Scrim(sheetScrim).OnDismiss(func() { open.Set(false) }))
	}
	h := gifttest.New(t, gifttest.Options{
		Root: root, Size: size, Font: loadTestFont(t), Theme: ui.LightTheme(), SafeArea: in,
	})
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)
	h.TapAt(geom.Pt(size.W/2, 30))
	h.Advance(ui.ControlAnimation / 4)
	return h
}

// slidingLayer is where the one cached layer of the frame — the sliding
// panel — lands on the device: the most of it that can be seen while it moves.
func slidingLayer(t *testing.T, h *gifttest.Harness) geom.Rect {
	t.Helper()
	list := h.List()
	var out []geom.Rect
	for i, op := range h.Ops() {
		if op.Kind == render.OpLayer {
			out = append(out, list.DeviceXform(i).TransformRect(op.Bounds))
		}
	}
	if len(out) != 1 {
		t.Fatalf("the frame has %d layers, want the sliding panel alone", len(out))
	}
	return out[0]
}

// TestSafeAreaSlidingSheetKeepsItsReach: a sliding node is drawn no larger
// than its bounds, so the panel's bounds have to cover the face's reach, or
// the strips behind the bars would be cut off for the whole slide and snap
// on at its end. The bounds of the panel at rest reach the bottom of the
// window, so its whole height away is below the window.
func TestSafeAreaSlidingSheetKeepsItsReach(t *testing.T) {
	t.Run("upright", func(t *testing.T) {
		h := slidingSheet(t, portraitInsets, geom.Sz(safeW, safeH))
		layer, face := slidingLayer(t, h), sheetFace(t, h)
		// The face is the panel, the navigation bar and the rounding strip,
		// which is the one part that may be cut off: it is below the window.
		if layer.Min.Y != face.Min.Y || layer.Max.Y != face.Max.Y-14 {
			t.Errorf("the sliding panel is drawn in %v, its face is %v: the strip behind the "+
				"navigation bar is cut off while it moves", layer, face)
		}
	})
	t.Run("on its side", func(t *testing.T) {
		h := slidingSheet(t, geom.Insets{Left: 32, Right: 48}, geom.Sz(800, 400))
		layer, face := slidingLayer(t, h), sheetFace(t, h)
		if layer.Min.X > face.Min.X || layer.Max.X < face.Max.X || face.Min.X != 0 || face.Max.X != 800 {
			t.Errorf("the sliding panel is drawn in %v, its face is %v; both have to span "+
				"the window, x=0..800", layer, face)
		}
	})
}

// TestSafeAreaSheetPanelReachesTheBottomOfTheWindow: the panel's own bounds,
// and not only its face, run from the top of the content to the bottom of
// the window, which is what puts it out of sight a whole height away.
func TestSafeAreaSheetPanelReachesTheBottomOfTheWindow(t *testing.T) {
	var dismissed int
	h := sheetSafe(t, portraitInsets, geom.Sz(safeW, safeH), &dismissed)
	panel := h.Find(gifttest.ByKey("panel")).LayoutBounds()
	content := h.Find(gifttest.ByKey("sheet-content")).Bounds()
	if want := geom.Rc(0, content.Min.Y, safeW, safeH); panel != want {
		t.Errorf("the panel is %v, want %v: from its content's top to the window's bottom", panel, want)
	}
}

// TestSafeAreaSheetFollowsATurnRoundOfThePhone: a phone turned through half a
// circle swaps the cutout and the navigation bar between the sides without
// changing a single constraint, and the sheet follows.
func TestSafeAreaSheetFollowsATurnRoundOfThePhone(t *testing.T) {
	var dismissed int
	size := geom.Sz(800, 400)
	h := sheetSafe(t, geom.Insets{Left: 32, Right: 48}, size, &dismissed)
	h.SetSafeArea(geom.Insets{Left: 48, Right: 32})
	content := h.Find(gifttest.ByKey("sheet-content")).Bounds()
	if content.Min.X != 48 || content.Max.X != 800-32 {
		t.Errorf("after the turn the sheet's content is %v, want x=48..768", content)
	}
	face := sheetFace(t, h)
	if face.Min.X != 0 || face.Max.X != 800 {
		t.Errorf("after the turn the sheet's surface is %v, want x=0..800", face)
	}
}
