package ui_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// The content area of a [ui.NavigationStack] under a safe area: the cut that
// keeps a sliding screen inside the stack reaches the edge of the window
// where the stack lies on the edge of the safe rectangle, so that a tab bar
// inside a screen paints behind the navigation bar of the phone, and stays at
// the edge of the stack everywhere else, so that a push still slides within
// it.

// nestedTabs is the nesting the cut used to spoil: a navigation stack whose
// screen is a tab bar.
func nestedTabs() gift.View {
	return ui.Window(ui.NavigationStack(nil, ui.Screen("Root",
		ui.TabBar(0, nil,
			ui.Tab("One", ui.Symbol{}, ui.Text("one")),
			ui.Tab("Two", ui.Symbol{}, ui.Text("two")),
		))))
}

// clipped returns every fill of colour c with the clip it is drawn under, in
// device space, for the ops outside any layer.
func clipped(h *gifttest.Harness, c render.Color) (fill, clip []geom.Rect) {
	list := h.List()
	for _, op := range h.Ops() {
		if (op.Kind == render.OpFillRect || op.Kind == render.OpFillRoundRect) && op.Color == c {
			fill = append(fill, op.Bounds)
			clip = append(clip, list.Clip(op.Clip))
		}
	}
	return fill, clip
}

// tabBarFill returns the fill of the tab bar's surface, the one that begins
// at the top of the tab bar, and the clip it is drawn under.
func tabBarFill(t *testing.T, h *gifttest.Harness) (bar, fill, clip geom.Rect) {
	t.Helper()
	// The navigation bar has the key "bar" as well; the tab bar is the one
	// further down.
	for _, n := range h.FindAll(gifttest.ByKey("bar")) {
		if b := n.Bounds(); b.Min.Y > bar.Min.Y {
			bar = b
		}
	}
	fs, cs := clipped(h, surface())
	for i, f := range fs {
		if f.Min.Y == bar.Min.Y {
			return bar, f, cs[i]
		}
	}
	t.Fatalf("no surface fill begins at the top of the tab bar %v; fills %v", bar, fs)
	return
}

// TestSafeAreaTabBarInsideANavigationStackReachesTheBottom is the defect: the
// tab bar of a screen fills down to y=800 and nothing cuts it there.
func TestSafeAreaTabBarInsideANavigationStackReachesTheBottom(t *testing.T) {
	h := newSafe(t, nestedTabs(), geom.Insets{Bottom: 48})

	bar, fill, clip := tabBarFill(t, h)
	if bar.Max.Y != safeH-48 {
		t.Fatalf("the tab bar is laid out at %v; it should end on the safe bottom edge", bar)
	}
	if fill.Max.Y != safeH || fill.Min.X != 0 || fill.Max.X != safeW {
		t.Fatalf("the tab bar fills %v, want it down to the bottom of the window", fill)
	}
	if clip.Max.Y < safeH {
		t.Fatalf("the tab bar fill %v is drawn under the clip %v, which cuts it at y=%v",
			fill, clip, clip.Max.Y)
	}
	// What the cut was for still holds above: the screen does not reach over
	// the navigation bar.
	nav := h.FindAll(gifttest.ByKey("bar"))[0].Bounds() // the navigation bar comes first
	if clip.Min.Y < nav.Max.Y {
		t.Fatalf("the clip of the screen is %v and reaches over the navigation bar %v", clip, nav)
	}
	// The tab bar has a content area of its own; the stack's comes first.
	content := h.FindAll(gifttest.ByKey("content").And(gifttest.ByType("ui.ZStack")))[0]
	if b := content.Bounds(); b.Max.Y != safeH-48 {
		t.Fatalf("the content area is laid out at %v; the cut must not move it", b)
	}
	h.AssertOpaque()
}

// TestSafeAreaTabBarInsideANavigationStackWithBothBars is the same with a
// status bar as well, at a phone's density: the navigation bar fills the
// status bar and the tab bar the navigation bar, each only its own.
func TestSafeAreaTabBarInsideANavigationStackWithBothBars(t *testing.T) {
	for _, d := range []float64{1, 2.625} {
		h := gifttest.New(t, gifttest.Options{
			View: nestedTabs(), Size: geom.Sz(safeW, safeH), Font: loadTestFont(t),
			Theme: ui.LightTheme(), SafeArea: portraitInsets, Density: d,
		})
		_, fill, clip := tabBarFill(t, h)
		dd := h.Density() // the density in force, which the harness may have rounded
		if fill.Max.Y != safeH || clip.Max.Y < safeH*dd-0.5 {
			t.Errorf("density %v: the tab bar fills %v under the device clip %v; want it to reach y=%v",
				d, fill, clip, safeH)
		}
		var top bool
		for _, r := range fills(h, surface()) {
			top = top || r.Min.Y == 0 && r.Min.X == 0 && r.Max.X == safeW
		}
		if !top {
			t.Errorf("density %v: the navigation bar no longer reaches the top of the window", d)
		}
	}
}

// TestSafeAreaPushStillSlidesInsideTheStack: a stack beside a sidebar, in a
// window with a navigation bar at the bottom. Halfway through a push the
// covered screen has slid past the leading edge of the stack and is cut
// there, the arriving one is cut at the trailing edge of the window, and both
// reach the bottom of the window, because nothing is beside the stack there.
func TestSafeAreaPushStillSlidesInsideTheStack(t *testing.T) {
	const side = float32(100)
	var depth *gift.State[int]
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(safeW, safeH),
		SafeArea: geom.Insets{Bottom: 48},
		Root: func(ctx *gift.Context) gift.View {
			depth = ctx.State("depth", 0)
			screens := []ui.ScreenSpec{ui.Screen("Root", marker(0))}
			if ctx.Read(depth) >= 1 {
				screens = append(screens, ui.Screen("Pushed", marker(1)))
			}
			return ui.Window(ui.HStack(
				ui.Box().Frame(side, geom.Unbounded()).Key("sidebar"),
				ui.NavigationStack(func() { depth.Set(0) }, screens...).Flex(1),
			))
		},
	})
	depth.Set(1)
	h.Settle()
	h.Advance(60 * time.Millisecond)

	covered := mustMarkerRect(t, h, 0)
	if covered.Min.X >= side {
		t.Fatalf("the covered screen is at %v 60 ms into the push; it should have slid past "+
			"the edge of the stack at x=%v, or the test proves nothing", covered, side)
	}
	content := h.Find(gifttest.ByKey("content").And(gifttest.ByType("ui.ZStack"))).Bounds()
	// A sliding screen is drawn into a layer and composited; the composite
	// is what the cut applies to.
	list := h.List()
	var layers int
	for _, op := range h.Ops() {
		if op.Kind != render.OpLayer || op.Bounds != content {
			continue
		}
		layers++
		c := list.Clip(op.Clip)
		if c.Min.X < side || c.Max.X > safeW {
			t.Errorf("a sliding screen is composited under the clip %v, wider than the stack "+
				"from x=%v to x=%v; it slides over the sidebar", c, side, safeW)
		}
		if c.Min.Y < content.Min.Y {
			t.Errorf("a sliding screen is composited under the clip %v, which reaches over the "+
				"navigation bar above y=%v", c, content.Min.Y)
		}
		if c.Max.Y < safeH {
			t.Errorf("a sliding screen is composited under the clip %v, which stops short of "+
				"the bottom of the window", c)
		}
	}
	if layers != 2 {
		t.Fatalf("%d sliding screens composited over the content area %v, want the two of the push",
			layers, content)
	}
}

// TestSafeAreaZeroKeepsTheContentCut: without a safe area the content area is
// the clipped ZStack it was, for paint and for input — a child that overflows
// a screen is cut at the bottom of the stack and cannot be touched below it —
// and a safe area that comes and goes leaves the frame as it was, without
// remounting the screens.
func TestSafeAreaZeroKeepsTheContentCut(t *testing.T) {
	c := ui.RGB(200, 30, 90)
	var hits int
	// The identity of the screen's state: a remount would make a new one.
	var ids []*int
	overflow := gift.Component("overflow", func(ctx *gift.Context) gift.View {
		ids = append(ids, ctx.State("id", new(int)).Get())
		return ui.VStack(
			ui.Button(ui.Box().Frame(safeW, 1000).Background(c), func() { hits++ }).Padding(0),
		)
	})
	view := ui.Window(ui.VStack(
		ui.NavigationStack(nil, ui.Screen("Root", overflow)).Flex(1),
		ui.Box().Frame(geom.Unbounded(), 100).Key("below"),
	))
	h := newSafe(t, view, geom.Insets{})
	content := h.Find(gifttest.ByKey("content").And(gifttest.ByType("ui.ZStack"))).Bounds()
	_, cs := clipped(h, render.Color(ui.ResolveColor(c)))
	if len(cs) != 1 || cs[0] != content {
		t.Fatalf("the overflowing child is drawn under %v, want the content area %v", cs, content)
	}
	h.TapAt(geom.Pt(safeW/2, content.Max.Y+50))
	if hits != 0 {
		t.Fatalf("a tap below the stack reached the child that overflows it")
	}

	before := h.Ops()
	h.SetSafeArea(portraitInsets)
	if reflect.DeepEqual(before, h.Ops()) {
		t.Fatal("the safe area changed nothing; the test cannot tell the two paths apart")
	}
	h.SetSafeArea(geom.Insets{})
	if got := h.Ops(); !reflect.DeepEqual(before, got) {
		t.Fatalf("after the safe area went away the frame differs:\nbefore %v\nafter  %v", before, got)
	}
	for _, id := range ids {
		if id != ids[0] {
			t.Fatal("the screen was remounted; a safe area that comes and goes must keep its state")
		}
	}
	if len(ids) < 2 {
		t.Fatalf("the screen was built %d times; the safe area should have rebuilt it", len(ids))
	}
}

// TestSafeAreaNavigationContentFrameIsAllocationFree: the painter of the cut
// allocates nothing per frame.
func TestSafeAreaNavigationContentFrameIsAllocationFree(t *testing.T) {
	saved := ui.DefaultFont()
	ui.SetDefaultFont(loadTestFont(t))
	t.Cleanup(func() { ui.SetDefaultFont(saved) })
	a := gift.New(gift.Options{Root: func(*gift.Context) gift.View { return nestedTabs() }})
	step := func() {
		if err := a.Update(geom.Sz(safeW, safeH)); err != nil {
			t.Fatal(err)
		}
		a.Paint()
	}
	step()
	a.SetSafeArea(portraitInsets)
	for range 16 {
		step()
	}
	if got := testing.AllocsPerRun(100, step); got != 0 {
		t.Errorf("%v allocations per frame, want 0", got)
	}
}

// TestSafeAreaNavigationCutGrowsForPaintOnly: with a safe area the paint cut
// of a stack that ends on the safe bottom reaches the window's bottom, so a
// tab bar inside a screen can paint behind the navigation bar, while input is
// still cut at the stack: a child that overflows the screen cannot be touched
// in the strip behind the navigation bar.
func TestSafeAreaNavigationCutGrowsForPaintOnly(t *testing.T) {
	c := ui.RGB(20, 160, 90)
	var hits int
	overflow := ui.VStack(
		ui.Button(ui.Box().Frame(safeW, 2000).Background(c), func() { hits++ }).Padding(0),
	)
	h := newSafe(t, ui.Window(ui.NavigationStack(nil, ui.Screen("Root", overflow))), geom.Insets{Top: 24, Bottom: 48})
	content := h.Find(gifttest.ByKey("content").And(gifttest.ByType("ui.ZStack"))).Bounds()
	if content.Max.Y != safeH-48 {
		t.Fatalf("the content area ends at %v, want the safe bottom %v", content.Max.Y, safeH-48)
	}
	_, cs := clipped(h, render.Color(ui.ResolveColor(c)))
	if len(cs) != 1 || cs[0].Max.Y != safeH {
		t.Fatalf("the overflowing child is drawn under %v, want a cut that reaches the window's bottom %v", cs, safeH)
	}
	h.TapAt(geom.Pt(safeW/2, safeH-10))
	if hits != 0 {
		t.Fatal("a tap behind the navigation bar reached the child that overflows the stack")
	}
	h.TapAt(geom.Pt(safeW/2, content.Max.Y-10))
	if hits != 1 {
		t.Fatalf("a tap inside the stack reached the child %d times, want 1", hits)
	}
}
