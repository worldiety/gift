package ebiten

import (
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// layerPage is a page of twenty rectangles in a layer at dx, as a slide
// between two pages emits it: the content in layer space, the offset only in
// the composite.
func layerPage(l *render.List, key render.ImageID, dx float32, col render.Color) {
	bounds := geom.Rc(0, 0, 400, 300)
	parent := l.PushXform(geom.Translate(geom.Pt(dx, 0)))
	inner := l.BeginLayer(key, bounds, parent, 0)
	for i := range 20 {
		f := float32(i) * 10
		l.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(f, f, f+8, f+8), Color: col, Clip: l.CurrentClip(), Xform: inner})
	}
	l.EndLayer()
}

func layerFrame(t *testing.T, r *Renderer, c *capture, build func(l *render.List)) (render.List, int) {
	t.Helper()
	var l render.List
	l.Reset()
	build(&l)
	c.reset()
	submit(t, r, c, &l)
	return l, c.batches
}

// TestAnUnchangedLayerIsCompositedAndNotDrawnAgain is the claim of the layer
// cache in draw calls: the first frame draws the page into its texture, every
// further frame of the slide draws one textured quad.
func TestAnUnchangedLayerIsCompositedAndNotDrawnAgain(t *testing.T) {
	r, c := sceneRenderer(t, 800, 600)
	red := render.RGB(200, 0, 0)

	_, batches := layerFrame(t, r, c, func(l *render.List) { layerPage(l, 1, 0, red) })
	if s := r.LayerStats(); s.Draws != 1 || s.Hits != 0 || s.Composites != 1 {
		t.Fatalf("first frame: %+v", s)
	}
	if want := []Material{MaterialShape, MaterialImage}; batches != 2 || c.mats[0] != want[0] || c.mats[1] != want[1] {
		t.Errorf("first frame drew %v, want the page into its texture and then the composite", c.mats)
	}

	for i := range 10 {
		dx := float32(i+1) * 37.3
		_, batches = layerFrame(t, r, c, func(l *render.List) { layerPage(l, 1, dx, red) })
		if batches != 1 || c.mats[0] != MaterialImage {
			t.Fatalf("frame %d of the slide drew %v, want one composite", i+2, c.mats)
		}
	}
	s := r.LayerStats()
	if s.Draws != 1 || s.Hits != 10 || s.Reused != 200 {
		t.Errorf("after the slide: %+v", s)
	}
	if st := r.Stats(); st.Accounted() != 11*21 {
		t.Errorf("accounted for %d operations, submitted %d", st.Accounted(), 11*21)
	}
}

// TestAChangedLayerIsDrawnAgain is the other half: there is no dirty flag, a
// different operation is a different picture.
func TestAChangedLayerIsDrawnAgain(t *testing.T) {
	r, c := sceneRenderer(t, 800, 600)
	layerFrame(t, r, c, func(l *render.List) { layerPage(l, 1, 0, render.RGB(200, 0, 0)) })
	layerFrame(t, r, c, func(l *render.List) { layerPage(l, 1, 0, render.RGB(0, 200, 0)) })
	if s := r.LayerStats(); s.Draws != 2 || s.Hits != 0 {
		t.Errorf("a changed colour was not drawn again: %+v", s)
	}

	// An index that moved because something before the layer changed is not
	// a change.
	layerFrame(t, r, c, func(l *render.List) {
		l.PushXform(geom.Scale(3, 3))
		l.PushClip(geom.Rc(0, 0, 700, 500))
		l.PopClip()
		layerPage(l, 1, 0, render.RGB(0, 200, 0))
	})
	if s := r.LayerStats(); s.Draws != 2 || s.Hits != 1 {
		t.Errorf("shifted side table indices were taken for a change: %+v", s)
	}
}

// TestLayersAreDrawnBeforeTheFrame pins the order that keeps a tile based GPU
// from writing its tiles out in the middle of the frame.
func TestLayersAreDrawnBeforeTheFrame(t *testing.T) {
	r, c := sceneRenderer(t, 800, 600)
	layerFrame(t, r, c, func(l *render.List) {
		addRect(l, geom.Rc(0, 0, 10, 10), render.RGB(1, 2, 3))
		layerPage(l, 1, 0, render.RGB(200, 0, 0))
		addRect(l, geom.Rc(20, 20, 30, 30), render.RGB(1, 2, 3))
		layerPage(l, 2, 400, render.RGB(0, 0, 200))
	})
	want := []Material{MaterialShape, MaterialShape, MaterialShape, MaterialImage, MaterialShape, MaterialImage}
	if len(c.mats) != len(want) {
		t.Fatalf("drew %v, want %v", c.mats, want)
	}
	for i := range want {
		if c.mats[i] != want[i] {
			t.Fatalf("drew %v, want both layers first and then the frame %v", c.mats, want)
		}
	}
}

// TestALayerOutOfViewIsNotDrawn keeps the parked page of a slide free.
func TestALayerOutOfViewIsNotDrawn(t *testing.T) {
	r, c := sceneRenderer(t, 800, 600)
	_, batches := layerFrame(t, r, c, func(l *render.List) { layerPage(l, 1, 900, render.RGB(200, 0, 0)) })
	if s := r.LayerStats(); batches != 0 || s.Draws != 0 || s.Composites != 0 {
		t.Errorf("a layer outside the screen cost %d batches: %+v", batches, s)
	}
	if st := r.Stats(); st.Accounted() != 21 || st.SkippedOutsideClip != 21 {
		t.Errorf("an invisible layer is accounted as %+v", st)
	}
}

// TestALayerSnapsToWholePixels keeps a layer at rest sharp.
func TestALayerSnapsToWholePixels(t *testing.T) {
	r, c := sceneRenderer(t, 800, 600)
	layerFrame(t, r, c, func(l *render.List) { layerPage(l, 1, 10.4, render.RGB(200, 0, 0)) })
	if c.verts[0].DstX != 10 || c.verts[0].SrcX != 0 {
		t.Errorf("the composite starts at x=%v (texel %v), want a whole pixel", c.verts[0].DstX, c.verts[0].SrcX)
	}
}

// TestALayerWithGlassIsDrawnThrough: a texture has no backdrop, so a layer
// with glass in it is drawn into the frame, where the panel is, and nothing
// is cached.
func TestALayerWithGlassIsDrawnThrough(t *testing.T) {
	r, _ := sceneRenderer(t, 800, 600)
	type box struct {
		m          Material
		minX, minY float32
	}
	var got []box
	r.drawFn = func(m Material, verts []eb.Vertex, _ []uint32) {
		b := box{m, verts[0].DstX, verts[0].DstY}
		for _, v := range verts {
			b.minX, b.minY = min(b.minX, v.DstX), min(b.minY, v.DstY)
		}
		got = append(got, b)
	}

	var l render.List
	l.Reset()
	parent := l.PushXform(geom.Translate(geom.Pt(100, 50)))
	inner := l.BeginLayer(1, geom.Rc(0, 0, 400, 300), parent, 0)
	l.Add(render.Op{
		Kind: render.OpMaterial, Bounds: geom.Rc(10, 10, 210, 110), CornerRadius: 12,
		Material: l.AddMaterial(render.NewGlass().Quality(render.Reduced).Material()), Clip: l.CurrentClip(), Xform: inner,
	})
	l.EndLayer()
	r.BeginFrame(geom.Sz(800, 600))
	r.Submit(&l)
	r.EndFrame()

	st := r.Stats()
	if st.GlassFallbacks != 0 || st.GlassReducedOps != 1 {
		t.Errorf("the glass in a layer was not drawn live: %+v", st)
	}
	if st.Layers.Through != 1 || st.Layers.Draws != 0 || st.Layers.Allocations != 0 {
		t.Errorf("a layer with glass was cached: %+v", st.Layers)
	}
	if st.Accounted() != 2 {
		t.Errorf("accounted for %d of 2 operations", st.Accounted())
	}
	found := false
	for _, b := range got {
		if b.m == MaterialGlass && b.minX == 110 && b.minY == 60 {
			found = true
		}
	}
	if !found {
		t.Errorf("no glass pass drew at the panel's place in the frame (110, 60): %+v", got)
	}
}

// TestAnUnusedLayerIsReleased keeps a page that was left from holding its
// texture.
func TestAnUnusedLayerIsReleased(t *testing.T) {
	r, c := sceneRenderer(t, 800, 600)
	layerFrame(t, r, c, func(l *render.List) { layerPage(l, 1, 0, render.RGB(200, 0, 0)) })
	for range layerMaxIdle + 1 {
		layerFrame(t, r, c, func(l *render.List) { addRect(l, geom.Rc(0, 0, 1, 1), render.RGB(1, 1, 1)) })
	}
	if s := r.LayerStats(); s.Resident != 0 || s.Evictions != 1 {
		t.Errorf("an unused layer was kept: %+v", s)
	}
}

