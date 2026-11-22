package raw

import (
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/worldiety/gift/cms"
)

// lookScene is a synthetic photograph and the picture a camera made of it
// by a known look: 768 by 512 photosites of cells of 8 by 8, each its own
// colour, and a preview of 384 by 256 pixels, a cell 4 by 4 of them.
type lookScene struct {
	f     *File
	raw   []uint16
	cells [][3]float64 // the white balanced colours of the camera
	truth func(s vec) vec
}

// cameraTruth is the look of the synthetic camera: a matrix that saturates
// in the linear light of sRGB, a curve brighter than this package's, and
// black and white if gray.
func cameraTruth(gray bool) func(s vec) vec {
	from2020 := must(rgbToXYZ(whiteD65, primariesSRGBTest).inverse())
	m := mat{{1.25, -0.2, -0.05}, {-0.1, 1.2, -0.1}, {-0.05, -0.25, 1.3}}.mul(from2020.mul(rec2020ToXYZ))
	tone := defaultTone()
	return func(s vec) vec {
		u := m.apply(s)
		if gray {
			// Black and white mixes the channels before the curve.
			y := 0.25*u[0] + 0.6*u[1] + 0.15*u[2]
			u = vec{y, y, y}
		}
		var v vec
		for c := range 3 {
			v[c] = float64(tone.at(float32(1.4 * math.Max(u[c], 0))))
		}
		return v
	}
}

func newLookScene(seed uint64, gray bool) *lookScene {
	rng := rand.New(rand.NewPCG(seed, 7))
	wb := [3]float64{2.2, 1, 1.6}
	ls := &lookScene{truth: cameraTruth(gray)}
	ls.cells = make([][3]float64, 96*64)
	for i := range ls.cells {
		l := math.Exp2(-9 + 8.5*rng.Float64())
		c := [3]float64{l * (0.4 + 1.2*rng.Float64()), l * (0.4 + 1.2*rng.Float64()), l * (0.4 + 1.2*rng.Float64())}
		for k := range 3 {
			c[k] = min(c[k], 0.85)
		}
		ls.cells[i] = c
	}
	ls.f, ls.raw = synthetic(768, 512, func(x, y int) [3]float64 {
		c := ls.cells[(y/8)*96+x/8]
		return [3]float64{c[0] / wb[0], c[1] / wb[1], c[2] / wb[2]}
	})
	ls.f.Make, ls.f.Model, ls.f.WhiteBalance = "SONY", "ILCE-7M3", wb
	return ls
}

// sceneOf is the scene of the white balanced camera colour c, as the pipe
// makes it.
func (ls *lookScene) sceneOf(t *testing.T, c [3]float64) vec {
	k, err := ls.f.newPipe(Options{Output: OutputScene}, ls.f.WhiteBalance, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b := []float32{float32(c[0])}, []float32{float32(c[1])}, []float32{float32(c[2])}
	k.run(Tile{Rect: image.Rect(0, 0, 1, 1), R: r, G: g, B: b, Stride: 1})
	return vec{float64(r[0]), float64(g[0]), float64(b[0])}
}

// preview renders the cells by the truth, in sRGB of 8 bits.
func (ls *lookScene) preview(t *testing.T) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 384, 256))
	for i, c := range ls.cells {
		v := ls.truth(ls.sceneOf(t, c))
		cx, cy := i%96, i/96
		for y := cy * 4; y < cy*4+4; y++ {
			for x := cx * 4; x < cx*4+4; x++ {
				o := img.PixOffset(x, y)
				for k := range 3 {
					img.Pix[o+k] = quantize8(srgbEncode(float32(v[k])))
				}
				img.Pix[o+3] = 0xff
			}
		}
	}
	return img
}

var primariesSRGBTest = [3]cms.Chromaticity{{X: 0.64, Y: 0.33}, {X: 0.30, Y: 0.60}, {X: 0.15, Y: 0.06}}

// The look of a camera is measured against its preview: the colours of
// scenes it was not measured on come out as the camera would make them.
func TestTheLookIsMeasured(t *testing.T) {
	for _, gray := range []bool{false, true} {
		ls := newLookScene(1, gray)
		d, err := newDeveloper(ls.f, ls.raw, Options{})
		if err != nil {
			t.Fatal(err)
		}
		l := ls.f.lookOf(d, ls.preview(t), nil)
		if l == nil {
			t.Fatalf("gray %v: no look", gray)
		}
		if l.gray != gray || l.residual > 2.5 || l.residual >= l.neutral {
			t.Errorf("gray %v: measured gray %v, ΔE %.2f held out, %.2f neutral", gray, l.gray, l.residual, l.neutral)
		}
		// Colours of another scene, through the pipe as Develop has it.
		other := newLookScene(2, gray)
		k, err := ls.f.newPipe(Options{}, ls.f.WhiteBalance, l)
		if err != nil {
			t.Fatal(err)
		}
		var sum float64
		n := 0
		for _, c := range other.cells[:2000] {
			want := ls.truth(ls.sceneOf(t, c))
			r, g, b := []float32{float32(c[0])}, []float32{float32(c[1])}, []float32{float32(c[2])}
			k.run(Tile{Rect: image.Rect(0, 0, 1, 1), R: r, G: g, B: b, Stride: 1})
			got := vec{float64(srgbDecode(r[0])), float64(srgbDecode(g[0])), float64(srgbDecode(b[0]))}
			if gray && (r[0] != g[0] || g[0] != b[0]) {
				t.Fatalf("a black and white look made %v %v %v", r[0], g[0], b[0])
			}
			a, w := labD50(got), labD50(want)
			sum += math.Sqrt((a[0]-w[0])*(a[0]-w[0]) + (a[1]-w[1])*(a[1]-w[1]) + (a[2]-w[2])*(a[2]-w[2]))
			n++
		}
		if sum/float64(n) > 1.5 {
			t.Errorf("gray %v: ΔE %.2f on average from the camera's", gray, sum/float64(n))
		}
		t.Logf("gray %v: held out ΔE %.2f, neutral %.2f, through the pipe %.2f", gray, l.residual, l.neutral, sum/float64(n))
	}
}

// A preview that is not of the same picture does not fit better than the
// neutral look and is not taken.
func TestAStrangePreviewIsNotTaken(t *testing.T) {
	ls := newLookScene(3, false)
	d, err := newDeveloper(ls.f, ls.raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	img := ls.preview(t)
	// The preview of another scene.
	other := newLookScene(4, false)
	other.truth = ls.truth
	img2 := other.preview(t)
	if l := ls.f.lookOf(d, img2, nil); l != nil {
		t.Errorf("another scene's preview was taken: ΔE %.2f against %.2f neutral", l.residual, l.neutral)
	}
	if l := ls.f.lookOf(d, img, nil); l == nil {
		t.Error("its own preview was not taken")
	}
}

func TestPAV(t *testing.T) {
	y := []float64{1, 3, 2, 2, 5, 4}
	pav(y, []float64{1, 1, 1, 1, 1, 1})
	want := []float64{1, 7.0 / 3, 7.0 / 3, 7.0 / 3, 4.5, 4.5}
	for i := range y {
		if math.Abs(y[i]-want[i]) > 1e-12 {
			t.Fatalf("%v, want %v", y, want)
		}
	}
}
