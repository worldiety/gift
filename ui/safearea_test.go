package ui_test

import (
	"reflect"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// The safe area tests: a [ui.Window] that is the root of the tree lays its
// content out inside the window without the safe area of
// [gift.App.SetSafeArea], keeps its page background on the whole window, and
// the bars of [ui.TabBar] and [ui.NavigationStack] paint their surface on to
// the edge of the window behind the system bars while their items stay where
// the system does not cover them.
//
// The window is a phone held upright, 400x800, with a status bar of 24 and a
// navigation bar of 48 logical pixels; the landscape test turns it on its
// side, with the cutout on the left and the navigation bar on the right.

const (
	safeW = float32(400)
	safeH = float32(800)
)

var portraitInsets = geom.Insets{Top: 24, Bottom: 48}

// newSafe mounts view in a 400x800 harness, or size if given, with the light
// theme and the insets in force from the first frame.
func newSafe(t testing.TB, view gift.View, in geom.Insets, size ...geom.Size) *gifttest.Harness {
	t.Helper()
	sz := geom.Sz(safeW, safeH)
	if len(size) > 0 {
		sz = size[0]
	}
	return gifttest.New(t, gifttest.Options{
		View:     view,
		Size:     sz,
		Font:     loadTestFont(t),
		Theme:    ui.LightTheme(),
		SafeArea: in,
	})
}

// fills returns the bounds of every plain rectangle fill of colour c in the
// frame, in order.
func fills(h *gifttest.Harness, c render.Color) []geom.Rect {
	var out []geom.Rect
	for _, op := range h.Ops() {
		if (op.Kind == render.OpFillRect || op.Kind == render.OpFillRoundRect) && op.Color == c {
			out = append(out, op.Bounds)
		}
	}
	return out
}

func surface() render.Color    { return ui.LightTheme().Color(ui.ColorSurface) }
func background() render.Color { return ui.LightTheme().Color(ui.ColorBackground) }

// TestSafeAreaWindowKeepsThePlateAndInsetsTheContent is the mechanism of
// [ui.Window]: the page background is the whole window, the content is the
// window without the status bar and the navigation bar.
func TestSafeAreaWindowKeepsThePlateAndInsetsTheContent(t *testing.T) {
	h := newSafe(t, ui.Window(ui.Box().Key("content")), portraitInsets)

	plate := fills(h, background())
	if len(plate) != 1 || plate[0] != geom.Rc(0, 0, safeW, safeH) {
		t.Fatalf("the page background is %v, want one fill of the whole window", plate)
	}
	want := geom.Rc(0, 24, safeW, safeH-48)
	if got := h.Find(gifttest.ByKey("content")).Bounds(); got != want {
		t.Fatalf("the content is %v, want %v: the window without the safe area", got, want)
	}
}

// TestSafeAreaFollowsARotation is the same window turned on its side: the
// cutout on the left, the navigation bar on the right, and a change of the
// insets between two frames moves the content without a rebuild of the app.
func TestSafeAreaFollowsARotation(t *testing.T) {
	h := newSafe(t, ui.Window(ui.Box().Key("content")), portraitInsets)
	h.SetSafeArea(geom.Insets{Left: 32, Right: 48})
	want := geom.Rc(32, 0, safeW-48, safeH)
	if got := h.Find(gifttest.ByKey("content")).Bounds(); got != want {
		t.Fatalf("after the rotation the content is %v, want %v", got, want)
	}
}

// TestSafeAreaTabBarPaintsBehindTheNavigationBar: the surface of the bar
// reaches the bottom of the window, its items do not, and a tap behind the
// navigation bar selects nothing.
func TestSafeAreaTabBarPaintsBehindTheNavigationBar(t *testing.T) {
	selected := -1
	h := newSafe(t, ui.Window(ui.TabBar(0, func(i int) { selected = i },
		ui.Tab("One", ui.Symbol{}, ui.Text("one")),
		ui.Tab("Two", ui.Symbol{}, ui.Text("two")),
	)), portraitInsets)

	safeBottom := safeH - 48
	bar := h.Find(gifttest.ByKey("bar")).Bounds()
	if bar.Max.Y != safeBottom {
		t.Fatalf("the bar is laid out at %v; it should end on the safe bottom edge y=%v", bar, safeBottom)
	}
	var reach bool
	for _, r := range fills(h, surface()) {
		if r.Min.Y == bar.Min.Y && r.Max.Y == safeH && r.Min.X == 0 && r.Max.X == safeW {
			reach = true
		}
	}
	if !reach {
		t.Fatalf("no surface fill from the top of the bar (y=%v) to the bottom of the window; fills: %v",
			bar.Min.Y, fills(h, surface()))
	}
	// The items are the buttons of the bar, and their bounds are their hit
	// areas.
	items := h.FindAll(gifttest.ByType("ui.Button"))
	if len(items) != 2 {
		t.Fatalf("%d buttons, want the two items", len(items))
	}
	for _, it := range items {
		if b := it.Bounds(); b.Max.Y > safeBottom {
			t.Errorf("an item is at %v, below the safe bottom edge", b)
		}
	}

	h.TapAt(geom.Pt(safeW*3/4, 780))
	if selected != -1 {
		t.Fatalf("a tap behind the navigation bar selected tab %d", selected)
	}
	h.TapAt(geom.Pt(safeW*3/4, bar.Min.Y+ui.TabBarHeight/2))
	if selected != 1 {
		t.Fatalf("a tap on the second item selected %d, want 1; the fixture is wrong", selected)
	}
}

// TestSafeAreaNavigationBarPaintsBehindTheStatusBar: the bar of a navigation
// stack fills the status bar, its title is below it.
func TestSafeAreaNavigationBarPaintsBehindTheStatusBar(t *testing.T) {
	h := newSafe(t, ui.Window(ui.NavigationStack(nil,
		ui.Screen("Root", ui.Text("root")))), portraitInsets)

	bar := h.Find(gifttest.ByKey("bar")).Bounds()
	if bar.Min.Y != 24 {
		t.Fatalf("the bar is laid out at %v; it should begin on the safe top edge y=24", bar)
	}
	var reach bool
	for _, r := range fills(h, surface()) {
		if r.Min.Y == 0 && r.Max.Y == bar.Max.Y && r.Min.X == 0 && r.Max.X == safeW {
			reach = true
		}
	}
	if !reach {
		t.Fatalf("no surface fill from the top of the window to the bottom of the bar; fills: %v",
			fills(h, surface()))
	}
	if b := h.Find(gifttest.ByText("Root")).Bounds(); b.Min.Y < 24 {
		t.Fatalf("the title is at %v, under the status bar", b)
	}
}

// TestSafeAreaLandscapeBarsBleedSideways: on a phone on its side the tab bar
// fills the cutout and the navigation bar beside it.
func TestSafeAreaLandscapeBarsBleedSideways(t *testing.T) {
	size := geom.Sz(800, 400)
	h := newSafe(t, ui.Window(ui.TabBar(0, nil,
		ui.Tab("One", ui.Symbol{}, ui.NavigationStack(nil, ui.Screen("Root", ui.Text("root")))),
	)), geom.Insets{Left: 32, Right: 48}, size)

	var tab, nav bool
	for _, r := range fills(h, surface()) {
		if r.Min.X == 0 && r.Max.X == size.W && r.Max.Y == size.H {
			tab = true
		}
		if r.Min.X == 0 && r.Max.X == size.W && r.Min.Y == 0 {
			nav = true
		}
	}
	if !tab || !nav {
		t.Fatalf("tab bar reaches both sides: %v, navigation bar: %v; fills: %v", tab, nav, fills(h, surface()))
	}
	if b := h.Find(gifttest.ByType("ui.Button")).Bounds(); b.Min.X < 32 || b.Max.X > size.W-48 {
		t.Fatalf("the item is at %v, outside the safe rectangle", b)
	}
}

// TestSafeAreaNestedWindowAppliesNone: a Window given less than the whole
// window is not at the edge of the screen and is not inset.
func TestSafeAreaNestedWindowAppliesNone(t *testing.T) {
	h := newSafe(t, ui.ZStack(ui.Window(ui.Box().Key("inner"))).Frame(200, 200), portraitInsets)
	if got, want := h.Find(gifttest.ByKey("inner")).Bounds(), geom.Rc(0, 0, 200, 200); got != want {
		t.Fatalf("the content of the nested window is %v, want %v", got, want)
	}
}

// TestSafeAreaIgnoresSafeAreaGivesTheWholeWindow is the opt-out.
func TestSafeAreaIgnoresSafeAreaGivesTheWholeWindow(t *testing.T) {
	h := newSafe(t, ui.Window(ui.Box().Key("content")).IgnoresSafeArea(true), portraitInsets)
	if got, want := h.Find(gifttest.ByKey("content")).Bounds(), geom.Rc(0, 0, safeW, safeH); got != want {
		t.Fatalf("the content is %v, want the whole window %v", got, want)
	}
}

// TestSafeAreaBleedIsOptInAndNeverRounded: a bar of the application's own
// bleeds where it asks to and touches the edge, does not without being
// asked, and a rounded background never does.
func TestSafeAreaBleedIsOptInAndNeverRounded(t *testing.T) {
	c := ui.RGB(10, 120, 200)
	bottom := func(b ui.BoxView) gift.View {
		return ui.Window(b.Frame(geom.Unbounded(), 60).Background(c).Key("bar")).Align(geom.Bottom)
	}
	cases := []struct {
		name string
		view gift.View
		want geom.Rect
	}{
		{"opted in", bottom(ui.Box().BleedSafeArea(ui.EdgesAll)), geom.Rc(0, 692, safeW, safeH)},
		{"not opted in", bottom(ui.Box()), geom.Rc(0, 692, safeW, 752)},
		{"rounded", bottom(ui.Box().CornerRadius(12).BleedSafeArea(ui.EdgesAll)), geom.Rc(0, 692, safeW, 752)},
		{"other edge", bottom(ui.Box().BleedSafeArea(ui.EdgesTop)), geom.Rc(0, 692, safeW, 752)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newSafe(t, tc.view, portraitInsets)
			got := fills(h, c)
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("the fill is %v, want %v", got, tc.want)
			}
			if b := h.Find(gifttest.ByKey("bar")).Bounds(); b != geom.Rc(0, 692, safeW, 752) {
				t.Fatalf("the bar is laid out at %v; a bleed must not move it", b)
			}
		})
	}
}

// TestSafeAreaKeyboardSitsAboveTheNavigationBar: the drawn keyboard is laid
// out on the safe bottom edge, and the content makes room for it inside the
// safe rectangle rather than for the keyboard and the bar twice.
func TestSafeAreaKeyboardSitsAboveTheNavigationBar(t *testing.T) {
	var printed int
	ed := ui.NewTextEditor("")
	ui.SetOnScreenKeyboard(nil, true)
	t.Cleanup(func() { ui.SetOnScreenKeyboard(nil, false) })
	h := newSafe(t, ui.Window(
		avoidScreen(t, ed, &printed),
		ui.OnScreenKeyboard().Key("kb"),
	).Align(geom.Bottom), portraitInsets)

	h.Find(gifttest.ByKey("field")).Focus()
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	if kb.Height() != ui.OnScreenKeyboardHeight() || kb.Max.Y != safeH-48 {
		t.Fatalf("the keyboard is %v; want %v tall, ending on the safe bottom edge y=%v",
			kb, ui.OnScreenKeyboardHeight(), safeH-48)
	}
	screen := h.Find(gifttest.ByKey("screen")).Bounds()
	if screen.Min.Y != 24 || screen.Max.Y != kb.Min.Y {
		t.Fatalf("the screen is %v with the keyboard at %v; it should run from y=24 to the keyboard", screen, kb)
	}
}

// TestSafeAreaZeroIsThePlainOverlay pins the promise that a desktop sees no
// difference: with no safe area a Window — which always takes the window
// path of its layouter now — draws exactly what the same tree spelled as a
// plain ZStack draws, which takes the overlay algorithm of internal/layout;
// and insets that come and go leave the frame as it was.
func TestSafeAreaZeroIsThePlainOverlay(t *testing.T) {
	screen := func() gift.View {
		return ui.TabBar(0, nil,
			ui.Tab("One", ui.Symbol{}, ui.NavigationStack(nil, ui.Screen("Root", ui.Text("root")))),
			ui.Tab("Two", ui.Symbol{}, ui.Text("two")),
		)
	}
	window := newSafe(t, ui.Window(screen()).Align(geom.Bottom).Padding(3), geom.Insets{})
	plain := newSafe(t, ui.ZStack(ui.Box().Background(ui.ColorBackground), screen()).
		Align(geom.Bottom).Padding(3), geom.Insets{})

	before := window.Ops()
	if got := plain.Ops(); !reflect.DeepEqual(before, got) {
		t.Fatalf("a Window with no safe area draws differently from the plain ZStack:\nwindow %v\nplain  %v",
			before, got)
	}
	window.SetSafeArea(portraitInsets)
	if reflect.DeepEqual(before, window.Ops()) {
		t.Fatal("the safe area changed nothing; the test cannot tell the two paths apart")
	}
	window.SetSafeArea(geom.Insets{})
	if got := window.Ops(); !reflect.DeepEqual(before, got) {
		t.Fatalf("after the safe area went away the frame differs:\nbefore %v\nafter  %v", before, got)
	}
}

// TestSafeAreaBleedAtPhoneDensities: the safe area and the bleed are logical
// pixels at every density a phone reports, the fractional ones included, so
// the bars reach the edges of the window and not a density's worth past or
// short of them.
func TestSafeAreaBleedAtPhoneDensities(t *testing.T) {
	for _, d := range []float64{1, 2, 2.625, 3} {
		h := gifttest.New(t, gifttest.Options{
			View: ui.Window(ui.TabBar(0, nil,
				ui.Tab("One", ui.Symbol{}, ui.NavigationStack(nil, ui.Screen("Root", ui.Text("root")))))),
			Size:     geom.Sz(safeW, safeH),
			Font:     loadTestFont(t),
			Theme:    ui.LightTheme(),
			SafeArea: portraitInsets,
			Density:  d,
		})
		var top, bottom bool
		for _, r := range fills(h, surface()) {
			top = top || r.Min.Y == 0 && r.Min.X == 0 && r.Max.X == safeW
			bottom = bottom || r.Max.Y == safeH && r.Min.X == 0 && r.Max.X == safeW
		}
		if !top || !bottom {
			t.Errorf("density %v: navigation bar reaches the top: %v, tab bar the bottom: %v; fills %v",
				d, top, bottom, fills(h, surface()))
		}
	}
}

// TestSafeAreaTabBarOnTheKeyboardDoesNotBleed: with the keyboard up the tab
// bar sits on the keyboard, not on the safe bottom edge, and paints its
// bounds; what is below it is the keyboard's.
func TestSafeAreaTabBarOnTheKeyboardDoesNotBleed(t *testing.T) {
	var printed int
	ed := ui.NewTextEditor("")
	ui.SetOnScreenKeyboard(nil, true)
	t.Cleanup(func() { ui.SetOnScreenKeyboard(nil, false) })
	h := newSafe(t, ui.Window(
		ui.TabBar(0, nil, ui.Tab("One", ui.Symbol{}, avoidScreen(t, ed, &printed))),
		ui.OnScreenKeyboard().Key("kb"),
	).Align(geom.Bottom), portraitInsets)

	h.Find(gifttest.ByKey("field")).Focus()
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	var bar geom.Rect
	for _, n := range h.FindAll(gifttest.ByKey("bar")) {
		bar = n.Bounds()
	}
	if bar.Max.Y != kb.Min.Y {
		t.Fatalf("the tab bar is at %v with the keyboard at %v; it should sit on the keyboard", bar, kb)
	}
	for _, r := range fills(h, surface()) {
		if r.Min.Y == bar.Min.Y && r.Max.Y != bar.Max.Y {
			t.Fatalf("the tab bar on the keyboard fills %v, past its bounds %v", r, bar)
		}
	}
}

// TestSafeAreaFramePathIsAllocationFree: a phone screen with a safe area, in
// which both bars bleed every frame, costs nothing per frame, idle or
// relaid out; the frame path contract of the project plan, section 11.
func TestSafeAreaFramePathIsAllocationFree(t *testing.T) {
	saved := ui.DefaultFont()
	ui.SetDefaultFont(loadTestFont(t))
	t.Cleanup(func() { ui.SetDefaultFont(saved) })
	a := gift.New(gift.Options{Root: func(*gift.Context) gift.View {
		return ui.Window(ui.TabBar(0, nil,
			ui.Tab("One", ui.Symbol{}, ui.NavigationStack(nil, ui.Screen("Root", ui.Text("root"))))))
	}})
	w := safeW
	step := func(resize bool) func() {
		return func() {
			if resize {
				if w == safeW {
					w = safeW + 1
				} else {
					w = safeW
				}
			}
			if err := a.Update(geom.Sz(w, safeH)); err != nil {
				t.Fatal(err)
			}
			a.Paint()
		}
	}
	step(false)()
	a.SetSafeArea(portraitInsets)
	for _, resize := range []bool{false, true} {
		for range 16 {
			step(resize)()
		}
		if got := testing.AllocsPerRun(100, step(resize)); got != 0 {
			t.Errorf("resize %v: %v allocations per frame, want 0", resize, got)
		}
	}
}
