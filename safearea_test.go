package gift_test

import (
	"math"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var safeProbeType = gift.RegisterType("test.SafeProbe")

// safeProbe is a node placed at a fixed rectangle of the window that records
// what SafeAreaBleed answers while it is painted, and the safe area its view
// function and its build read.
type safeProbe struct {
	at    geom.Rect
	bleed *geom.Insets
	built *geom.Insets
}

func (p safeProbe) ViewType() gift.TypeID { return safeProbeType }

func (p safeProbe) Build(bc *gift.BuildContext) gift.Element {
	if p.built != nil {
		*p.built = bc.SafeArea()
	}
	return gift.Element{
		Layouter: placeAt{p.at},
		Children: []gift.View{bleedBox{w: p.at.Width(), h: p.at.Height(), out: p.bleed}},
		Painter:  paintChildren{},
	}
}

type placeAt struct{ r geom.Rect }

func (l placeAt) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	ctx.Measure(0, geom.Tight(l.r.Size()))
	ctx.Place(0, l.r.Min)
	return c.Max
}

var bleedBoxType = gift.RegisterType("test.BleedBox")

// bleedBox is a box that records SafeAreaBleed while it is painted.
type bleedBox struct {
	w, h float32
	out  *geom.Insets
}

func (b bleedBox) ViewType() gift.TypeID { return bleedBoxType }

func (b bleedBox) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Layouter: fixedSize{w: b.w, h: b.h}, Painter: bleedRecorder{b.out}}
}

type bleedRecorder struct{ out *geom.Insets }

func (r bleedRecorder) Paint(ctx *gift.PaintContext) { *r.out = ctx.SafeAreaBleed() }

// mustUpdateAt is mustUpdate with the viewport given.
func mustUpdateAt(t *testing.T, a *gift.App, s geom.Size) {
	t.Helper()
	if err := a.Update(s); err != nil {
		t.Fatal(err)
	}
}

// TestSafeAreaIsZeroAndSanitised: a desktop never reports one; a reported
// one is made whole, and what is not a length is nothing.
func TestSafeAreaIsZeroAndSanitised(t *testing.T) {
	a := gift.New(gift.Options{Root: func(*gift.Context) gift.View { return box{w: 10, h: 10} }})
	mustUpdateAt(t, a, geom.Sz(400, 800))
	if a.SafeArea() != (geom.Insets{}) {
		t.Fatalf("a fresh App has the safe area %+v", a.SafeArea())
	}
	if r := a.SafeRect(); r != geom.Rc(0, 0, 400, 800) {
		t.Fatalf("without a safe area the safe rectangle is %v, want the viewport", r)
	}
	inf := float32(math.Inf(1))
	nan := float32(math.NaN())
	a.SetSafeArea(geom.Insets{Top: 24.38, Right: -3, Bottom: 48.000001, Left: nan})
	if got, want := a.SafeArea(), (geom.Insets{Top: 25, Bottom: 48}); got != want {
		t.Fatalf("SafeArea = %+v, want %+v", got, want)
	}
	a.SetSafeArea(geom.Insets{Top: inf})
	if a.SafeArea() != (geom.Insets{}) {
		t.Fatalf("an infinite edge left %+v", a.SafeArea())
	}
	a.SetSafeArea(geom.Insets{Top: 500, Bottom: 500})
	mustUpdateAt(t, a, geom.Sz(400, 800))
	if r := a.SafeRect(); r.Height() != 0 || r.Width() != 400 {
		t.Fatalf("a safe area larger than the viewport gives %v, want a collapsed, uninverted rectangle", r)
	}
}

// TestSafeAreaChangeRebuildsItsReaders: a view function that reads the safe
// area sees the new one after a change, and setting the value in force
// rebuilds nothing.
func TestSafeAreaChangeRebuildsItsReaders(t *testing.T) {
	var seen []geom.Insets
	var built geom.Insets
	a := gift.New(gift.Options{Root: func(ctx *gift.Context) gift.View {
		seen = append(seen, ctx.SafeArea())
		return safeProbe{at: geom.Rc(0, 0, 10, 10), bleed: new(geom.Insets), built: &built}
	}})
	mustUpdateAt(t, a, geom.Sz(400, 800))
	a.SetSafeArea(geom.Insets{Top: 24, Bottom: 48})
	mustUpdateAt(t, a, geom.Sz(400, 800))
	if len(seen) != 2 || seen[1] != (geom.Insets{Top: 24, Bottom: 48}) {
		t.Fatalf("the view function saw %+v, want a second build with the new safe area", seen)
	}
	if built != (geom.Insets{Top: 24, Bottom: 48}) {
		t.Fatalf("the build read %+v", built)
	}
	a.SetSafeArea(geom.Insets{Top: 24, Bottom: 48})
	mustUpdateAt(t, a, geom.Sz(400, 800))
	if len(seen) != 2 {
		t.Fatalf("setting the safe area in force rebuilt the view function: %d builds", len(seen))
	}
	var nilBC *gift.BuildContext
	if nilBC.SafeArea() != (geom.Insets{}) {
		t.Fatal("a nil BuildContext reports a safe area")
	}
}

type paintChildren struct{}

func (paintChildren) Paint(ctx *gift.PaintContext) { ctx.PaintChildren() }

// TestSafeAreaBleedReachesTheWindowEdge: a node on an edge of the safe
// rectangle may draw its background to the edge of the window; one a pixel
// away may not. The answer is in local units, at any density.
func TestSafeAreaBleedReachesTheWindowEdge(t *testing.T) {
	safe := geom.Insets{Top: 24, Right: 16, Bottom: 48, Left: 8}
	for _, density := range []float64{1, 2} {
		cases := []struct {
			name string
			at   geom.Rect
			want geom.Insets
		}{
			{"the whole safe rectangle", geom.Rc(8, 24, 384, 752), safe},
			{"a bar at the bottom", geom.Rc(8, 700, 384, 752), geom.Insets{Right: 16, Bottom: 48, Left: 8}},
			{"a bar a pixel above it", geom.Rc(8, 699, 384, 751), geom.Insets{Right: 16, Left: 8}},
			{"a box in the middle", geom.Rc(100, 100, 200, 200), geom.Insets{}},
		}
		for _, c := range cases {
			var got geom.Insets
			a := gift.New(gift.Options{Root: func(*gift.Context) gift.View { return safeProbe{at: c.at, bleed: &got} }})
			a.SetDensity(density)
			a.SetSafeArea(safe)
			mustUpdateAt(t, a, geom.Sz(400, 800))
			a.Paint()
			if got != c.want {
				t.Errorf("density %v, %s: bleed %+v, want %+v", density, c.name, got, c.want)
			}
		}
	}
	// No safe area, no bleed, wherever the node is.
	var got geom.Insets
	a := gift.New(gift.Options{Root: func(*gift.Context) gift.View { return safeProbe{at: geom.Rc(0, 0, 400, 800), bleed: &got} }})
	mustUpdateAt(t, a, geom.Sz(400, 800))
	a.Paint()
	if got != (geom.Insets{}) {
		t.Fatalf("without a safe area the bleed is %+v", got)
	}
}

var bleedClipType = gift.RegisterType("test.BleedClip")

// bleedClip is a clipping container that asks for ClipBleedsSafeArea, with
// a box that fills it.
type bleedClip struct {
	at     geom.Rect
	radius float32
	xform  *geom.Affine2D
}

func (b bleedClip) ViewType() gift.TypeID { return bleedClipType }

func (b bleedClip) Build(*gift.BuildContext) gift.Element {
	return gift.Element{
		Layouter:           placeAt{geom.Rc(0, 0, b.at.Width(), b.at.Height())},
		Children:           []gift.View{box{w: b.at.Width(), h: b.at.Height()}},
		Painter:            paintChildren{},
		Clip:               true,
		ClipRadius:         b.radius,
		ClipBleedsSafeArea: true,
		Transform:          b.xform,
	}
}

var bleedRootType = gift.RegisterType("test.BleedRoot")

type bleedRoot struct{ clip bleedClip }

func (r bleedRoot) ViewType() gift.TypeID { return bleedRootType }

func (r bleedRoot) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Layouter: placeAt{r.clip.at}, Children: []gift.View{r.clip}, Painter: paintChildren{}}
}

// TestClipBleedsSafeAreaFollowsTheBleedRule: a plain clip on the safe
// rectangle grows to the window edge; a rounded one and one under a mirror
// keep the bounds, as SafeAreaBleed answers zero there, and without the safe
// area nothing grows at all.
func TestClipBleedsSafeAreaFollowsTheBleedRule(t *testing.T) {
	safe := geom.Insets{Top: 24, Right: 16, Bottom: 48, Left: 8}
	at := geom.Rc(8, 24, 384, 752)
	mirror := geom.Affine2D{A: -1, D: 1, TX: 392}
	cases := []struct {
		name string
		clip bleedClip
		safe geom.Insets
		want geom.Rect
	}{
		{"a plain clip", bleedClip{at: at}, safe, geom.Rc(0, 0, 400, 800)},
		{"a rounded clip", bleedClip{at: at, radius: 12}, safe, at},
		{"a mirrored clip", bleedClip{at: at, xform: &mirror}, safe, at},
		{"no safe area", bleedClip{at: at}, geom.Insets{}, at},
	}
	for _, c := range cases {
		a := gift.New(gift.Options{Root: func(*gift.Context) gift.View { return bleedRoot{c.clip} }})
		a.SetSafeArea(c.safe)
		mustUpdateAt(t, a, geom.Sz(400, 800))
		list := a.Paint()
		// The fill is drawn under the clip, or, rounded, into a layer
		// whose composite is drawn under it; the widest clip of any op is
		// the clip the container pushed.
		var widest geom.Rect
		for _, op := range list.Ops() {
			if op.Clip == 0 {
				continue
			}
			r := list.Clip(op.Clip)
			if r.Width()*r.Height() > widest.Width()*widest.Height() {
				widest = r
			}
		}
		if widest != c.want {
			t.Errorf("%s: widest clip %v, want %v", c.name, widest, c.want)
		}
	}
}
