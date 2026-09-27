//go:build giftgpu

package ebiten

import (
	"image/color"
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// layerScene is a card with antialiased corners, a stroke and a translucent
// fill, the kinds of pixel a composite could get wrong, drawn either directly
// or through a layer. dx places it.
func layerScene(l *render.List, layered bool, dx float32) { layerSceneGlass(l, layered, dx, false) }

func layerSceneGlass(l *render.List, layered bool, dx float32, glass bool) {
	bounds := geom.Rc(0, 0, 60, 40)
	parent := l.PushXform(geom.Translate(geom.Pt(dx, 3)))
	xf := parent
	if layered {
		xf = l.BeginLayer(1, bounds, parent, 0)
	}
	clip := l.CurrentClip()
	l.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(2, 2, 58, 38), CornerRadius: 9, Color: render.RGB(30, 90, 200), Clip: clip, Xform: xf})
	l.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: geom.Rc(2, 2, 58, 38), CornerRadius: 9, StrokeWidth: 1.5, Color: render.RGB(250, 250, 250), Clip: clip, Xform: xf})
	l.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(10, 10, 40, 20), Color: render.RGBA(255, 0, 0, 128), Clip: clip, Xform: xf})
	if glass {
		l.Add(render.Op{
			Kind: render.OpMaterial, Bounds: geom.Rc(20, 6, 56, 34), CornerRadius: 6,
			Material: l.AddMaterial(render.NewGlass().Quality(render.Full).Material()), Clip: clip, Xform: xf,
		})
	}
	if layered {
		l.EndLayer()
	}
}

func drawLayerScene(r *Renderer, dst *eb.Image, layered bool, dx float32) {
	drawLayerSceneGlass(r, dst, layered, dx, false)
}

func drawLayerSceneGlass(r *Renderer, dst *eb.Image, layered bool, dx float32, glass bool) {
	dst.Fill(color.RGBA{20, 20, 20, 255})
	var l render.List
	l.Reset()
	layerSceneGlass(&l, layered, dx, glass)
	r.SetTarget(dst)
	b := dst.Bounds()
	r.BeginFrame(geom.Sz(float32(b.Dx()), float32(b.Dy())))
	r.Submit(&l)
	r.EndFrame()
}

// TestALayerLooksLikeItsContent is the claim that makes a layer safe to put
// anywhere: through the texture, at rest and after moving, the pixels are
// the ones the content draws directly.
func TestALayerLooksLikeItsContent(t *testing.T) {
	const w, h = 128, 64
	for _, dx := range []float32{5, 41} {
		direct, err := NewRenderer()
		if err != nil {
			t.Fatal(err)
		}
		want := eb.NewImage(w, h)
		drawLayerScene(direct, want, false, dx)

		layered, err := NewRenderer()
		if err != nil {
			t.Fatal(err)
		}
		got := eb.NewImage(w, h)
		drawLayerScene(layered, got, true, 5) // drawn into the texture here
		drawLayerScene(layered, got, true, dx)
		if s := layered.LayerStats(); s.Draws != 1 {
			t.Fatalf("dx=%v: the second frame drew the layer again: %+v", dx, s)
		}

		worst := 0
		for y := range h {
			for x := range w {
				a, b := want.At(x, y).(color.RGBA), got.At(x, y).(color.RGBA)
				for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
					worst = max(worst, d, -d)
				}
			}
		}
		// One step of eight bit rounding: the composite blends a
		// premultiplied texel that was itself rounded to eight bits.
		if worst > 1 {
			t.Errorf("dx=%v: the layer differs from its content by up to %d", dx, worst)
		}
	}
}

// TestALayerWithGlassLooksLikeItsContent: drawn through, the glass sees the
// same backdrop it sees without the layer.
func TestALayerWithGlassLooksLikeItsContent(t *testing.T) {
	const w, h = 128, 64
	direct, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	want := eb.NewImage(w, h)
	drawLayerSceneGlass(direct, want, false, 17, true)

	layered, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	got := eb.NewImage(w, h)
	drawLayerSceneGlass(layered, got, true, 17, true)
	if s := layered.LayerStats(); s.Through != 1 {
		t.Fatalf("the layer was not drawn through: %+v", s)
	}
	worst := 0
	for y := range h {
		for x := range w {
			a, b := want.At(x, y).(color.RGBA), got.At(x, y).(color.RGBA)
			for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				worst = max(worst, d, -d)
			}
		}
	}
	if worst != 0 {
		t.Errorf("drawn through, the layer differs from its content by up to %d", worst)
	}
}
