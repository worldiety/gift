package ebiten

import (
	"math"
	"strings"
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// The headless half of the rounded picture: what reaches the vertex stream.
// The pixel half, which needs a window, is in roundimage_gpu_test.go.

// roundImageFixture returns a headless renderer inside a frame, with one
// resident texture of w by h pixels, and the id to draw it with. The caller
// submits and ends the frame.
func roundImageFixture(t testing.TB, w, h int) (*Renderer, *capture, render.ImageID) {
	t.Helper()
	r, c := newHeadlessRenderer(t)
	r.BeginFrame(geom.Sz(400, 400))
	hd, ok := r.Textures().Acquire(render.Pixels{Pix: make([]byte, w*h*4), W: w, H: h, Stride: w * 4})
	if !ok {
		t.Fatal("the upload was refused")
	}
	id, _ := r.Textures().Resolve(hd)
	return r, c, id
}

// TestRoundImageShaderCompiles is TestShaderCompiles for the rounded picture
// shader, and the source level guard of TestShaderUsesNoScreenSpaceDerivatives
// with it: the shader measures in device pixels baked in by the CPU side, so
// it has no reason to use a derivative, and one that crept in would compile
// here and fail only in a driver without them.
func TestRoundImageShaderCompiles(t *testing.T) {
	if _, err := eb.NewShader(RoundImageShaderSource()); err != nil {
		t.Fatalf("the rounded picture shader does not compile: %v", err)
	}
	src := shaderCode(string(RoundImageShaderSource()))
	for _, fn := range []string{"fwidth", "dfdx", "dfdy", "for ", "discard"} {
		if strings.Contains(src, fn) {
			t.Errorf("the rounded picture shader uses %q; see TestShaderUsesNoScreenSpaceDerivatives", fn)
		}
	}
}

// TestRoundImageIsFourQuadrantsWithTheFoldedDistanceField pins the vertex
// contract of roundimage.kage: four quads, one per quadrant of the padded
// bounds, none of which crosses a centre line, and in every vertex the first
// line of sdRoundBox evaluated at that vertex.
//
// That last property is the whole trick. q = |p| - half + radius is affine
// only on one side of each centre line; a quad that straddled one would be
// interpolated into a wrong shape without any vertex being wrong.
func TestRoundImageIsFourQuadrantsWithTheFoldedDistanceField(t *testing.T) {
	r, c, id := roundImageFixture(t, 16, 16)
	var l render.List
	l.Reset()
	b := geom.Rc(10, 20, 110, 70)
	l.Add(render.Op{Kind: render.OpImage, Bounds: b, Color: render.RGB(255, 255, 255), Image: id, CornerRadius: 8})
	r.Submit(&l)
	r.EndFrame()

	if len(c.mats) != 1 || c.mats[0] != MaterialRoundImage {
		t.Fatalf("materials = %v, want one round image batch", c.mats)
	}
	if len(c.verts) != 16 || len(c.idx) != 24 {
		t.Fatalf("%d vertices and %d indices, want 16 and 24: four quads", len(c.verts), len(c.idx))
	}
	cx, cy := float32(60), float32(45)
	for q := 0; q < 4; q++ {
		quad := c.verts[q*4 : q*4+4]
		left, right, above, below := false, false, false, false
		for _, v := range quad {
			left = left || v.DstX < cx
			right = right || v.DstX > cx
			above = above || v.DstY < cy
			below = below || v.DstY > cy
		}
		if left && right || above && below {
			t.Errorf("quad %d crosses a centre line: %+v", q, quad)
		}
	}
	for i, v := range c.verts {
		wantQX := abs32(v.DstX-cx) - 50 + 8
		wantQY := abs32(v.DstY-cy) - 25 + 8
		if !near32(v.Custom0, wantQX) || !near32(v.Custom1, wantQY) || v.Custom2 != 8 || v.Custom3 != 0 {
			t.Errorf("vertex %d at (%v, %v): custom = %v %v %v %v, want q = (%v, %v), radius 8, 0",
				i, v.DstX, v.DstY, v.Custom0, v.Custom1, v.Custom2, v.Custom3, wantQX, wantQY)
		}
		// The whole texture onto the bounds, extrapolated into the pad.
		wantU := (v.DstX - 10) * 16 / 100
		wantV := (v.DstY - 20) * 16 / 50
		if !near32(v.SrcX, wantU) || !near32(v.SrcY, wantV) {
			t.Errorf("vertex %d at (%v, %v): texel (%v, %v), want (%v, %v)", i, v.DstX, v.DstY, v.SrcX, v.SrcY, wantU, wantV)
		}
	}
	// The pad: one device pixel beyond the bounds, as for a rounded fill.
	minX, minY := float32(math.Inf(1)), float32(math.Inf(1))
	for _, v := range c.verts {
		minX, minY = min(minX, v.DstX), min(minY, v.DstY)
	}
	if minX != 10-aaPad || minY != 20-aaPad {
		t.Errorf("top left of the geometry = (%v, %v), want the bounds grown by %v", minX, minY, aaPad)
	}
	s := r.Stats()
	if s.ImageOps != 1 || s.RoundImageOps != 1 || s.ImageBatches != 1 {
		t.Errorf("ImageOps %d, RoundImageOps %d, ImageBatches %d; want 1, 1, 1", s.ImageOps, s.RoundImageOps, s.ImageBatches)
	}
}

// TestRoundImageRadiusIsClampedLikeAFill checks the clamp: the radius of a
// picture is limited to half its smaller edge, exactly as for a rounded fill,
// so a pill shaped picture and a pill shaped placeholder are the same pill.
func TestRoundImageRadiusIsClampedLikeAFill(t *testing.T) {
	r, c, id := roundImageFixture(t, 4, 4)
	var l render.List
	l.Reset()
	l.Add(render.Op{Kind: render.OpImage, Bounds: geom.Rc(0, 0, 100, 30), Color: render.RGB(255, 255, 255), Image: id, CornerRadius: 1000})
	r.Submit(&l)
	r.EndFrame()
	if got := c.verts[0].Custom2; got != 15 {
		t.Errorf("radius = %v, want 15, half the smaller edge", got)
	}
}

// TestSquareImageKeepsTheCheapPath: a radius of zero, or a negative one, is
// the textured quad of Ebitengine's own filter, exactly as before rounded
// pictures existed. The distance field is not paid for where it cannot change
// a pixel.
func TestSquareImageKeepsTheCheapPath(t *testing.T) {
	for _, radius := range []float32{0, -4} {
		r, c, id := roundImageFixture(t, 4, 4)
		var l render.List
		l.Reset()
		l.Add(render.Op{Kind: render.OpImage, Bounds: geom.Rc(0, 0, 40, 40), Color: render.RGB(255, 255, 255), Image: id, CornerRadius: radius})
		r.Submit(&l)
		r.EndFrame()
		if len(c.mats) != 1 || c.mats[0] != MaterialImage || len(c.verts) != 4 {
			t.Errorf("radius %v: materials %v with %d vertices, want one image batch of 4", radius, c.mats, len(c.verts))
		}
		if n := r.Stats().RoundImageOps; n != 0 {
			t.Errorf("radius %v: RoundImageOps = %d, want 0", radius, n)
		}
	}
}

// TestCoverCropsTheTextureInsteadOfClipping is render.ImageCover at the
// backend: Bounds is the tile, nothing is clipped, and the texture coordinates
// span the centred crop. A 32 by 16 texture in a square tile shows texels 8 to
// 24 horizontally and all of them vertically — on both paths.
func TestCoverCropsTheTextureInsteadOfClipping(t *testing.T) {
	for _, radius := range []float32{0, 6} {
		r, c, id := roundImageFixture(t, 32, 16)
		var l render.List
		l.Reset()
		l.Add(render.Op{Kind: render.OpImage, Fit: render.ImageCover, Bounds: geom.Rc(100, 100, 140, 140),
			Color: render.RGB(255, 255, 255), Image: id, CornerRadius: radius})
		r.Submit(&l)
		r.EndFrame()
		for i, v := range c.verts {
			wantU := 8 + (v.DstX-100)*16/40
			wantV := (v.DstY - 100) * 16 / 40
			if !near32(v.SrcX, wantU) || !near32(v.SrcY, wantV) {
				t.Errorf("radius %v, vertex %d at (%v, %v): texel (%v, %v), want (%v, %v)",
					radius, i, v.DstX, v.DstY, v.SrcX, v.SrcY, wantU, wantV)
			}
		}
		if radius == 0 {
			// The square path is exactly the bounds: the corners carry the
			// edges of the crop.
			if v := c.verts[0]; v.DstX != 100 || v.SrcX != 8 || v.SrcY != 0 {
				t.Errorf("top left vertex = %+v, want (100, 100) at texel (8, 0)", v)
			}
			if v := c.verts[2]; v.DstX != 140 || v.SrcX != 24 || v.SrcY != 16 {
				t.Errorf("bottom right vertex = %+v, want (140, 140) at texel (24, 16)", v)
			}
		}
	}
}

// TestRoundImageIsClippedPerQuadrant checks that a clip trims the geometry of
// a rounded picture the way it trims everything else, before a fragment is
// shaded, and that the attributes of the trimmed quads are still the folded
// distance field at the new corners.
func TestRoundImageIsClippedPerQuadrant(t *testing.T) {
	r, c, id := roundImageFixture(t, 16, 16)
	var l render.List
	l.Reset()
	// The left third of a picture centred on x = 60: only the two left
	// quadrants survive, and they are cut at x = 40.
	clip := l.PushClip(geom.Rc(0, 0, 40, 400))
	l.Add(render.Op{Kind: render.OpImage, Bounds: geom.Rc(10, 20, 110, 70), Color: render.RGB(255, 255, 255),
		Image: id, CornerRadius: 8, Clip: clip})
	l.PopClip()
	r.Submit(&l)
	r.EndFrame()
	if len(c.verts) != 8 {
		t.Fatalf("%d vertices, want 8: the two right quadrants lie outside the clip", len(c.verts))
	}
	for i, v := range c.verts {
		if v.DstX > 40 {
			t.Errorf("vertex %d at x = %v, outside the clip", i, v.DstX)
		}
		if want := abs32(v.DstX-60) - 50 + 8; !near32(v.Custom0, want) {
			t.Errorf("vertex %d: q.x = %v, want %v", i, v.Custom0, want)
		}
	}
	// Entirely outside: counted where every other operation is counted.
	r2, _, id2 := roundImageFixture(t, 4, 4)
	var l2 render.List
	l2.Reset()
	clip2 := l2.PushClip(geom.Rc(300, 300, 400, 400))
	l2.Add(render.Op{Kind: render.OpImage, Bounds: geom.Rc(0, 0, 50, 50), Color: render.RGB(255, 255, 255),
		Image: id2, CornerRadius: 8, Clip: clip2})
	l2.PopClip()
	r2.Submit(&l2)
	r2.EndFrame()
	s := r2.Stats()
	if s.SkippedOutsideClip != 1 || s.ImageOps != 0 || s.Accounted() != 1 {
		t.Errorf("SkippedOutsideClip %d, ImageOps %d, Accounted %d; want 1, 0, 1", s.SkippedOutsideClip, s.ImageOps, s.Accounted())
	}
}

// TestRoundImageUnderTheDeviceScale is the density of the project plan,
// section 18, on a rounded picture: at 2x the geometry, the half extents and
// the radius are all doubled, so the antialiased edge stays one device pixel
// wide rather than one logical one.
func TestRoundImageUnderTheDeviceScale(t *testing.T) {
	r, c, id := roundImageFixture(t, 8, 8)
	var l render.List
	l.Reset()
	x := l.PushXform(geom.Scale(2, 2))
	l.Add(render.Op{Kind: render.OpImage, Bounds: geom.Rc(10, 10, 60, 40), Color: render.RGB(255, 255, 255),
		Image: id, CornerRadius: 5, Xform: x})
	r.Submit(&l)
	r.EndFrame()
	cx, cy := float32(70), float32(50)
	for i, v := range c.verts {
		if v.Custom2 != 10 {
			t.Fatalf("vertex %d: radius %v, want 10 device pixels", i, v.Custom2)
		}
		if want := abs32(v.DstX-cx) - 50 + 10; !near32(v.Custom0, want) {
			t.Errorf("vertex %d: q.x = %v, want %v", i, v.Custom0, want)
		}
		if want := abs32(v.DstY-cy) - 30 + 10; !near32(v.Custom1, want) {
			t.Errorf("vertex %d: q.y = %v, want %v", i, v.Custom1, want)
		}
	}
	if v := c.verts[0]; v.DstX != 20-aaPad || v.DstY != 20-aaPad {
		t.Errorf("top left vertex at (%v, %v), want the device bounds grown by one device pixel", v.DstX, v.DstY)
	}
}

// TestRotatedRoundImageKeepsTheFieldAffinePerQuadrant covers the general
// path, which gift has no producer for. Under a rotation lengths are
// unchanged, so the folded field of a vertex can be recomputed from its
// device position through the inverse transform, and it must agree.
func TestRotatedRoundImageKeepsTheFieldAffinePerQuadrant(t *testing.T) {
	r, c, id := roundImageFixture(t, 8, 8)
	var l render.List
	l.Reset()
	m := geom.Rotate(0.4).Mul(geom.Translate(geom.Point{X: 200, Y: 100}))
	x := l.PushXform(m)
	b := geom.Rc(0, 0, 80, 40)
	l.Add(render.Op{Kind: render.OpImage, Bounds: b, Color: render.RGB(255, 255, 255), Image: id, CornerRadius: 6, Xform: x})
	r.Submit(&l)
	r.EndFrame()
	if len(c.mats) != 1 || c.mats[0] != MaterialRoundImage {
		t.Fatalf("materials = %v, want one round image batch", c.mats)
	}
	if len(c.verts) < 12 {
		t.Fatalf("%d vertices, want at least four polygons", len(c.verts))
	}
	inv, ok := m.Invert()
	if !ok {
		t.Fatal("the test transform is singular")
	}
	for i, v := range c.verts {
		p := inv.Apply(geom.Point{X: v.DstX, Y: v.DstY})
		wantQX := abs32(p.X-40) - 40 + 6
		wantQY := abs32(p.Y-20) - 20 + 6
		if math.Abs(float64(v.Custom0-wantQX)) > 1e-2 || math.Abs(float64(v.Custom1-wantQY)) > 1e-2 {
			t.Errorf("vertex %d: q = (%v, %v), want (%v, %v)", i, v.Custom0, v.Custom1, wantQX, wantQY)
		}
	}
}

// TestRoundImageBatching: a rounded picture is its own material, adjacent
// rounded pictures of one texture share a batch, and a square picture of the
// same texture between them is a batch boundary in display list order — the
// rule of TestImageOpsInterleaveWithShapesAndText, extended.
func TestRoundImageBatching(t *testing.T) {
	r, _, id := roundImageFixture(t, 8, 8)
	var l render.List
	l.Reset()
	white := render.RGB(255, 255, 255)
	l.Add(render.Op{Kind: render.OpImage, Bounds: geom.Rc(0, 0, 40, 40), Color: white, Image: id, CornerRadius: 4})
	l.Add(render.Op{Kind: render.OpImage, Bounds: geom.Rc(50, 0, 90, 40), Color: white, Image: id, CornerRadius: 4})
	l.Add(render.Op{Kind: render.OpImage, Bounds: geom.Rc(100, 0, 140, 40), Color: white, Image: id})
	l.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(0, 50, 40, 90), Color: white, CornerRadius: 4})
	var mats []Material
	r.drawFn = func(m Material, _ []eb.Vertex, _ []uint32) { mats = append(mats, m) }
	r.Submit(&l)
	r.EndFrame()
	want := []Material{MaterialRoundImage, MaterialImage, MaterialShape}
	if len(mats) != len(want) {
		t.Fatalf("materials = %v, want %v", mats, want)
	}
	for i := range want {
		if mats[i] != want[i] {
			t.Fatalf("materials = %v, want %v", mats, want)
		}
	}
	s := r.Stats()
	if s.ImageBatches != 2 || s.ImageOps != 3 || s.RoundImageOps != 2 || s.Accounted() != uint64(l.Len()) {
		t.Errorf("ImageBatches %d, ImageOps %d, RoundImageOps %d, Accounted %d; want 2, 3, 2, %d",
			s.ImageBatches, s.ImageOps, s.RoundImageOps, s.Accounted(), l.Len())
	}
	if MaterialRoundImage.String() != "round image" {
		t.Errorf("String = %q", MaterialRoundImage.String())
	}
}

// TestRoundImageSubmitDoesNotAllocate is go/no-go criterion 3 of the project
// plan, section 12, for rounded pictures on both paths: the quadrant loop and
// the per vertex arithmetic live on the stack and in the reused buffers.
func TestRoundImageSubmitDoesNotAllocate(t *testing.T) {
	for _, xf := range []geom.Affine2D{geom.Translate(geom.Point{X: 3, Y: 7}), geom.Rotate(0.3)} {
		r, _, id := roundImageFixture(t, 8, 8)
		r.EndFrame()
		r.drawFn = func(Material, []eb.Vertex, []uint32) {}
		var l render.List
		l.Reset()
		clip := l.PushClip(geom.Rc(0, 0, 300, 300))
		x := l.PushXform(xf)
		for i := 0; i < 200; i++ {
			f := float32(i)
			l.Add(render.Op{Kind: render.OpImage, Fit: render.ImageCover, Bounds: geom.Rc(f, f, f+30, f+20),
				Color: render.RGB(255, 255, 255), Image: id, CornerRadius: 5, Clip: clip, Xform: x})
		}
		frame := func() {
			r.BeginFrame(geom.Sz(300, 300))
			// Keep the texture resident, as a painter would.
			r.Textures().Resolve(render.ImageHandle{ID: id, Gen: r.Textures().recs[id].gen})
			r.Submit(&l)
			r.EndFrame()
		}
		for i := 0; i < 8; i++ {
			frame()
		}
		if n := testing.AllocsPerRun(50, frame); n != 0 {
			t.Fatalf("rounded pictures under %v allocate %v times per frame, want 0", xf, n)
		}
		if r.Stats().RoundImageOps == 0 {
			t.Fatal("nothing was drawn; this measured the skip path")
		}
	}
}

func abs32(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

func near32(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }
