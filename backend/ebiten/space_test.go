package ebiten

import (
	"bytes"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/cms"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

func p3(t testing.TB) *workingSpace {
	t.Helper()
	ws, err := newWorkingSpace(cms.DisplayP3())
	if err != nil || ws == nil {
		t.Fatalf("Display P3: %v, %v", ws, err)
	}
	return ws
}

// The conversion of a display list colour is the colour management of
// package cms, to the precision of a float32, premultiplied as it came.
func TestColoursAreConvertedAsPackageCMSConvertsThem(t *testing.T) {
	ws := p3(t)
	ref, err := cms.NewTransform(cms.SRGB(), cms.DisplayP3(), cms.Options{Intent: cms.RelativeColorimetric})
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		r, g, b := rng.Float64(), rng.Float64(), rng.Float64()
		a := float32(rng.IntN(256)) / 255
		if a == 0 {
			continue
		}
		in := render.Color{R: float32(r) * a, G: float32(g) * a, B: float32(b) * a, A: a}
		got := ws.color(in)
		want := ref.Color(float64(in.R/a), float64(in.G/a), float64(in.B/a))
		if got.A != a {
			t.Fatalf("alpha %v became %v", a, got.A)
		}
		// A thousandth is a quarter of an 8 bit step; package cms works
		// to 16 bits, and near black the curve multiplies that.
		for i, v := range []float32{got.R, got.G, got.B} {
			if d := math.Abs(float64(v/a) - want[i]); d > 1e-3 {
				t.Fatalf("%v: channel %d is %v, cms says %v", in, i, v/a, want[i])
			}
		}
		// Asked again, from the memo.
		if again := ws.color(in); again != got {
			t.Fatalf("%v: %v, then %v", in, got, again)
		}
	}
	// The matrix is the one published for sRGB to Display P3.
	for i, want := range [9]float64{
		0.8224621, 0.1775380, 0,
		0.0331941, 0.9668058, 0,
		0.0170827, 0.0723974, 0.9105199,
	} {
		if d := math.Abs(float64(ws.m[i]) - want); d > 1e-4 {
			t.Errorf("matrix element %d is %v, want %v", i, ws.m[i], want)
		}
	}
	// The red of sRGB is (234, 51, 35) in Display P3.
	red := ws.color(render.RGB(255, 0, 0))
	if r, g, b := red.R*255, red.G*255, red.B*255; math.Abs(float64(r-234)) > 0.5 || math.Abs(float64(g-51)) > 0.5 || math.Abs(float64(b-35)) > 0.5 {
		t.Errorf("sRGB red is %.1f %.1f %.1f in P3, want 234 51 35", r, g, b)
	}
}

// A gray is the same number in both spaces, and passes through untouched,
// so text and shadows come out bit for bit as before.
func TestGraysAndTheTransparentPassUntouched(t *testing.T) {
	ws := p3(t)
	for _, c := range []render.Color{
		render.RGB(0, 0, 0), render.RGB(255, 255, 255), render.RGBA(255, 255, 255, 30),
		render.RGBA(17, 17, 17, 200), {}, {R: 0.2, G: 0.4, B: 0.6, A: 0},
	} {
		if got := ws.color(c); got != c {
			t.Errorf("%v became %v", c, got)
		}
	}
	var srgb *workingSpace
	if c := render.RGB(255, 0, 0); srgb.color(c) != c {
		t.Error("the sRGB working space changed a colour")
	}
}

func TestOnlySRGBAndDisplayP3AreWorkingSpaces(t *testing.T) {
	for _, p := range []*cms.Profile{nil, cms.SRGB()} {
		if ws, err := newWorkingSpace(p); ws != nil || err != nil {
			t.Errorf("%v: %v, %v", p, ws, err)
		}
	}
	if _, err := newWorkingSpace(cms.AdobeRGB()); err == nil {
		t.Error("Adobe RGB accepted")
	}
	r, _ := newHeadlessRenderer(t)
	if err := r.SetWorkingSpace(cms.Rec2020()); err == nil || r.WorkingSpace() != cms.SRGB() {
		t.Errorf("Rec. 2020: %v, painting in %s", err, r.WorkingSpace().Description())
	}
}

func TestConvertingAColourAllocatesNothing(t *testing.T) {
	ws := p3(t)
	cs := []render.Color{render.RGB(255, 0, 0), render.RGBA(10, 200, 30, 90), render.RGB(1, 2, 3)}
	i := 0
	if n := testing.AllocsPerRun(1000, func() {
		_ = ws.color(cs[i%len(cs)])
		// And one the memo has not seen.
		_ = ws.color(render.Color{R: float32(i%97) / 97, G: 0.5, B: 0.25, A: 1})
		i++
	}); n != 0 {
		t.Errorf("%v allocations", n)
	}
}

// Every colour of the display list reaches its vertices converted: fills,
// both ends of a gradient, triangles. The headless renderer records the
// vertices.
func TestTheRendererConvertsTheColoursOfTheList(t *testing.T) {
	r, c := newHeadlessRenderer(t)
	if err := r.SetWorkingSpace(cms.DisplayP3()); err != nil {
		t.Fatal(err)
	}
	ws := r.ws
	red, blue, gray := render.RGB(255, 0, 0), render.RGB(0, 0, 255), render.RGB(128, 128, 128)

	vertex := func(v eb.Vertex) render.Color {
		return render.Color{R: v.ColorR, G: v.ColorG, B: v.ColorB, A: v.ColorA}
	}
	frame := func(build func(l *render.List)) {
		c.reset()
		var l render.List
		l.Reset()
		build(&l)
		r.BeginFrame(geom.Sz(64, 64))
		r.Submit(&l)
		r.EndFrame()
	}

	frame(func(l *render.List) {
		l.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(0, 0, 10, 10), Color: red})
	})
	if got := vertex(c.verts[0]); got != ws.color(red) {
		t.Errorf("fill %v, want %v", got, ws.color(red))
	}

	frame(func(l *render.List) {
		l.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(0, 0, 10, 10), Color: gray})
	})
	if got := vertex(c.verts[0]); got != gray {
		t.Errorf("gray %v, want it untouched", got)
	}

	frame(func(l *render.List) {
		l.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(0, 0, 10, 10), Color: red,
			Material: l.AddMaterial(render.LinearGradient(red, blue).Material())})
	})
	// The corners of the padded quad are extrapolated a little beyond the
	// ends, so near them.
	near := func(a, b render.Color) bool {
		return math.Abs(float64(a.R-b.R)) < 1e-3 && math.Abs(float64(a.G-b.G)) < 1e-3 && math.Abs(float64(a.B-b.B)) < 1e-3
	}
	var top, bottom bool
	for _, v := range c.verts {
		top = top || near(vertex(v), ws.color(red))
		bottom = bottom || near(vertex(v), ws.color(blue))
		if near(vertex(v), red) || near(vertex(v), blue) {
			t.Errorf("gradient corner %v is unconverted", vertex(v))
		}
	}
	if !top || !bottom {
		t.Errorf("gradient corners %v, want %v and %v", c.verts, ws.color(red), ws.color(blue))
	}

	frame(func(l *render.List) {
		first := l.VerticesLen()
		l.AppendVertex(render.Vertex{X: 1, Y: 1, Color: red})
		l.AppendVertex(render.Vertex{X: 9, Y: 1, Color: blue})
		l.AppendVertex(render.Vertex{X: 1, Y: 9, Color: gray})
		l.Add(render.Op{Kind: render.OpTriangles, Bounds: geom.Rc(0, 0, 10, 10), Color: render.RGB(255, 255, 255),
			Glyphs: first, GlyphCount: 3})
	})
	if len(c.verts) != 3 || vertex(c.verts[0]) != ws.color(red) || vertex(c.verts[1]) != ws.color(blue) || vertex(c.verts[2]) != gray {
		t.Errorf("triangle %v", c.verts)
	}
}

func TestPicturesAreConvertedFromTheirOwnSpace(t *testing.T) {
	pc := pixelConversions{dst: cms.DisplayP3()}
	if tr, err := pc.get(nil); tr == nil || err != nil {
		t.Errorf("sRGB into P3: %v, %v", tr, err)
	}
	if tr, err := pc.get(cms.DisplayP3()); tr != nil || err != nil {
		t.Errorf("P3 into P3: %v, %v", tr, err)
	}
	if tr, err := pc.get(cms.SGray()); tr != nil || err == nil {
		t.Errorf("gray pixels: %v, %v", tr, err)
	}
	srgb := pixelConversions{}
	if tr, err := srgb.get(cms.SRGB()); tr != nil || err != nil {
		t.Errorf("sRGB into sRGB: %v, %v", tr, err)
	}
	if tr, err := srgb.get(cms.DisplayP3()); tr == nil || err != nil {
		t.Errorf("P3 into sRGB: %v, %v", tr, err)
	}
}

// A large picture is converted on several cores, a small one on one; both
// give the same pixels, packed whatever the stride.
func TestConvertingInBandsIsConvertingAtOnce(t *testing.T) {
	tr, err := cms.NewTransform(cms.SRGB(), cms.DisplayP3(), cms.Options{})
	if err != nil {
		t.Fatal(err)
	}
	const w, h, stride = 700, 300, 700*4 + 12
	src := make([]byte, stride*h)
	for i := range src {
		src[i] = byte(i * 7919 >> 3)
		if i%stride < 4*w && i%4 == 3 {
			src[i] = byte(max(src[i], 200))
		}
	}
	big := make([]byte, 4*w*h)
	if err := convertPixels(tr, big, src, w, h, stride); err != nil {
		t.Fatal(err)
	}
	for y := range h {
		row := make([]byte, 4*w)
		if err := convertPixels(tr, row, src[y*stride:], w, 1, stride); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(row, big[y*4*w:(y+1)*4*w]) {
			t.Fatalf("row %d differs", y)
		}
	}
}

// The wide variants of the video shaders are made from the sRGB ones by
// replacing two lines; when the shaders change, this says so.
func TestWideVideoShadersCompile(t *testing.T) {
	for name, src := range map[string][]byte{"video.kage": videoConvertSrc, "videodirect.kage": videoDirectSrc} {
		wide := wideVideoSource(src)
		if !strings.Contains(string(wide), "toWorking(") || bytes.Equal(wide, src) {
			t.Errorf("%s: not converted", name)
		}
		if _, err := eb.NewShader(wide); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	vc, err := NewVideoCache()
	if err != nil {
		t.Fatal(err)
	}
	if err := vc.setSpace(p3(t)); err != nil || vc.convert == vc.base[0] || vc.direct == vc.base[1] {
		t.Errorf("wide shaders not installed: %v", err)
	}
	u := videoUniforms(render.VideoFrame{}, vc.ws)
	if _, ok := u["ToR"]; !ok {
		t.Error("no conversion matrix among the uniforms")
	}
	if err := vc.setSpace(nil); err != nil || vc.convert != vc.base[0] || vc.direct != vc.base[1] {
		t.Errorf("sRGB shaders not restored: %v", err)
	}
	if _, ok := videoUniforms(render.VideoFrame{}, nil)["ToR"]; ok {
		t.Error("a conversion matrix for sRGB")
	}
}
