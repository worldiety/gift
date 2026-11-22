package cms

import (
	"bytes"
	"crypto/md5"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func mustTransform(t testing.TB, src, dst *Profile, o Options) *Transform {
	t.Helper()
	tr, err := NewTransform(src, dst, o)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func testProfile(t testing.TB, name string) *Profile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "profiles", name+".icc"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseProfile(b)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTheBuiltInSpacesAreTheirStandards(t *testing.T) {
	// The colorants of sRGB adapted to D50, as every sRGB profile has them
	// to four places.
	_, m, err := SRGB().matrixShaper()
	if err != nil {
		t.Fatal(err)
	}
	want := mat3{{0.4361, 0.3851, 0.1431}, {0.2225, 0.7169, 0.0606}, {0.0139, 0.0971, 0.7141}}
	for r := range 3 {
		for c := range 3 {
			if math.Abs(m[r][c]-want[r][c]) > 2e-4 {
				t.Errorf("sRGB colorant [%d][%d] = %.4f, want %.4f", r, c, m[r][c], want[r][c])
			}
		}
	}
	if !mustTransform(t, SRGB(), SRGB(), Options{}).Identity() {
		t.Error("sRGB to sRGB is not the identity")
	}
	if mustTransform(t, SRGB(), DisplayP3(), Options{}).Identity() {
		t.Error("sRGB to Display P3 counts as the identity")
	}
	// Pure sRGB red on a P3 display, the colour the Mac showed too
	// saturated before the screen was tagged sRGB.
	out := make([]byte, 3)
	if err := mustTransform(t, SRGB(), DisplayP3(), Options{}).Pixels(out, RGB8, []byte{255, 0, 0}, RGB8); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, []byte{234, 51, 35}) {
		t.Errorf("sRGB red in Display P3 is %v, want [234 51 35]", out)
	}
	for _, p := range []*Profile{SRGB(), DisplayP3(), AdobeRGB(), ProPhotoRGB(), Rec2020(), LinearSRGB(), LinearRec2020(), SGray()} {
		if maj, min := p.Version(); maj != 4 || min != 3 {
			t.Errorf("%s is version %d.%d", p.Description(), maj, min)
		}
		// The profile ID is the MD5 of the profile with the ID zeroed.
		b := bytes.Clone(p.Bytes())
		id := bytes.Clone(b[84:100])
		clear(b[84:100])
		if sum := md5.Sum(b); !bytes.Equal(sum[:], id) {
			t.Errorf("%s has a wrong profile ID", p.Description())
		}
	}
}

// TestBakedTablesAgreeWithTheExactPipeline checks every kind of kernel at
// random colours, in 8 and 16 bits: curves and matrices within an 8 bit
// step, which is rounding, and interpolated tables within two and a half,
// about what LittleCMS's own tables are off by in the same places; see
// TestAgainstLittleCMS.
func TestBakedTablesAgreeWithTheExactPipeline(t *testing.T) {
	cmyk, mab := testProfile(t, "cmyk-lut16"), testProfile(t, "rgb-mab")
	for _, c := range []struct {
		name      string
		src, dst  *Profile
		kernel    string
		tolerance float64
	}{
		{"matrix", SRGB(), DisplayP3(), "*cms.matrixShaper", 1},
		{"gray", SGray(), SRGB(), "*cms.lut1", 1},
		{"table of 3", SRGB(), mab, "*cms.clutKernel", 2.5},
		{"table of 4", cmyk, SRGB(), "*cms.clutKernel", 2.5},
		{"to 4", SRGB(), cmyk, "*cms.clutKernel", 2.5},
	} {
		tr := mustTransform(t, c.src, c.dst, Options{Intent: RelativeColorimetric})
		if got := typeName(tr.k); got != c.kernel {
			t.Errorf("%s: kernel %s, want %s", c.name, got, c.kernel)
		}
		rng := rand.New(rand.NewPCG(1, 2))
		in, out := tr.in, tr.out
		const n = 2000
		src8 := make([]byte, n*in)
		for i := range src8 {
			src8[i] = byte(rng.IntN(256))
		}
		src16 := make([]byte, 2*n*in)
		for i := range src16 {
			src16[i] = byte(rng.IntN(256))
		}
		dst8, dst16 := make([]byte, n*out), make([]byte, 2*n*out)
		if in != out {
			if err := tr.Pixels(dst8, fmt8(in), src8, fmt8(in)); err == nil {
				t.Fatal("formats of the wrong channels were taken")
			}
		}
		if err := tr.Pixels(dst8, fmt8(out), src8, fmt8(in)); err != nil {
			t.Fatal(err)
		}
		if err := tr.Pixels(dst16, fmt16(out), src16, fmt16(in)); err != nil {
			t.Fatal(err)
		}
		worst8, worst16 := 0.0, 0.0
		for p := range n {
			v8, v16 := make([]float64, in), make([]float64, in)
			for i := range in {
				v8[i] = float64(src8[p*in+i]) / 255
				v16[i] = float64(uint16(src16[2*(p*in+i)])<<8|uint16(src16[2*(p*in+i)+1])) / 65535
			}
			e8, e16 := tr.Color(v8...), tr.Color(v16...)
			for o := range out {
				g8 := float64(dst8[p*out+o])
				g16 := float64(uint16(dst16[2*(p*out+o)])<<8|uint16(dst16[2*(p*out+o)+1])) / 257
				worst8 = max(worst8, math.Abs(g8-unit(e8[o])*255))
				worst16 = max(worst16, math.Abs(g16-unit(e16[o])*255))
			}
		}
		if worst8 > c.tolerance || worst16 > c.tolerance {
			t.Errorf("%s: baked tables off by %.2f (8 bit) and %.2f (16 bit) steps", c.name, worst8, worst16)
		}
	}
}

func fmt8(n int) Format {
	return map[int]Format{1: Gray8, 3: RGB8, 4: CMYK8}[n]
}

func fmt16(n int) Format {
	return map[int]Format{1: Gray16, 3: RGB16, 4: CMYK16}[n]
}

func typeName(v any) string {
	switch v.(type) {
	case *matrixShaper:
		return "*cms.matrixShaper"
	case *lut1:
		return "*cms.lut1"
	case *clutKernel:
		return "*cms.clutKernel"
	}
	return "?"
}

func TestAlphaIsCarriedAndPremultiplicationUndone(t *testing.T) {
	tr := mustTransform(t, SRGB(), DisplayP3(), Options{})
	// Straight alpha passes through, and the colour is that of the opaque
	// pixel.
	src := []byte{255, 0, 0, 77, 255, 0, 0, 255}
	got := make([]byte, 8)
	if err := tr.Pixels(got, RGBA8, src, RGBA8); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{234, 51, 35, 77, 234, 51, 35, 255}) {
		t.Errorf("straight RGBA: %v", got)
	}
	// A premultiplied pixel is converted as its colour, not as the darker
	// colour its numbers would be.
	half := []byte{128, 0, 0, 128}
	if err := tr.Pixels(got[:4], RGBAPremul8, half, RGBAPremul8); err != nil {
		t.Fatal(err)
	}
	if want := []byte{117, 26, 18, 128}; !bytes.Equal(got[:4], want) {
		t.Errorf("premultiplied RGBA: %v, want %v", got[:4], want)
	}
	// Opaque runs take the fast path, which must not reach a translucent
	// pixel among them: three blocks, the middle one with one pixel at
	// half alpha.
	run := make([]byte, 4*600)
	for i := 0; i < len(run); i += 4 {
		copy(run[i:], []byte{255, 0, 0, 255})
	}
	copy(run[4*300:], half)
	out := make([]byte, len(run))
	if err := tr.Pixels(out, RGBAPremul8, run, RGBAPremul8); err != nil {
		t.Fatal(err)
	}
	for p := range 600 {
		want := []byte{234, 51, 35, 255}
		if p == 300 {
			want = []byte{117, 26, 18, 128}
		}
		if !bytes.Equal(out[4*p:4*p+4], want) {
			t.Fatalf("pixel %d of a premultiplied run: %v, want %v", p, out[4*p:4*p+4], want)
		}
	}
	// Nothing visible stays nothing.
	if err := tr.Pixels(got[:4], RGBAPremul8, []byte{0, 0, 0, 0}, RGBAPremul8); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:4], []byte{0, 0, 0, 0}) {
		t.Errorf("transparent pixel: %v", got[:4])
	}
	// A source without alpha is opaque.
	if err := tr.Pixels(got[:4], RGBA8, []byte{0, 0, 0}, RGB8); err != nil {
		t.Fatal(err)
	}
	if got[3] != 255 {
		t.Errorf("alpha from RGB: %d", got[3])
	}
	// 16 bit alpha too.
	g16 := make([]byte, 8)
	if err := tr.Pixels(g16, RGBA16, []byte{0, 0, 0, 0, 0, 0, 0x12, 0x34}, RGBA16); err != nil {
		t.Fatal(err)
	}
	if g16[6] != 0x12 || g16[7] != 0x34 {
		t.Errorf("16 bit alpha: %x", g16[6:])
	}
}

func TestPixelsRefusesWhatDoesNotFit(t *testing.T) {
	tr := mustTransform(t, SRGB(), SRGB(), Options{})
	if err := tr.Pixels(make([]byte, 4), CMYK8, make([]byte, 3), RGB8); err == nil {
		t.Error("CMYK out of an RGB transform")
	}
	if err := tr.Pixels(make([]byte, 3), RGB8, make([]byte, 4), RGB8); err == nil {
		t.Error("a partial pixel")
	}
	if err := tr.Pixels(make([]byte, 3), RGB8, make([]byte, 6), RGB8); err == nil {
		t.Error("a destination too small")
	}
}

func TestRGBToGrayAndBack(t *testing.T) {
	tg := mustTransform(t, SRGB(), SGray(), Options{})
	if g := tg.Color(0.5, 0.5, 0.5)[0]; math.Abs(g-0.5) > 1e-4 {
		t.Errorf("sRGB gray to sGray: %v", g)
	}
	tr := mustTransform(t, SGray(), SRGB(), Options{})
	if c := tr.Color(0.25); math.Abs(c[0]-0.25) > 1e-4 || math.Abs(c[1]-0.25) > 1e-4 || math.Abs(c[2]-0.25) > 1e-4 {
		t.Errorf("sGray to sRGB: %v", c)
	}
}

func TestAConstructedProfileIsItsBytes(t *testing.T) {
	p, err := NewRGBProfile("Wide", WhiteD65, primaries2020, Gamma(2.4))
	if err != nil {
		t.Fatal(err)
	}
	if p.Description() != "Wide" || p.Space() != SpaceRGB || p.PCS() != SpaceXYZ || !p.IsMatrixShaper() {
		t.Errorf("constructed profile: %q %v %v", p.Description(), p.Space(), p.PCS())
	}
	q, err := ParseProfile(bytes.Clone(p.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !mustTransform(t, p, q, Options{}).Identity() {
		t.Error("a profile and its parsed bytes differ")
	}
	if _, err := NewRGBProfile("Flat", WhiteD65, [3]Chromaticity{{0.3, 0.3}, {0.3, 0.3}, {0.3, 0.3}}, Linear()); err == nil {
		t.Error("primaries in one point make a profile")
	}
	if _, err := NewRGBProfile("Bad", WhiteD65, primariesSRGB, Gamma(-1)); err == nil {
		t.Error("a negative gamma makes a profile")
	}
}

func BenchmarkPixelsMatrix8(b *testing.B) {
	tr := mustTransform(b, AdobeRGB(), SRGB(), Options{})
	src := make([]byte, 1<<20*3)
	for i := range src {
		src[i] = byte(i * 7)
	}
	dst := make([]byte, len(src))
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		_ = tr.Pixels(dst, RGB8, src, RGB8)
	}
}

func BenchmarkPixelsCMYK8(b *testing.B) {
	tr := mustTransform(b, testProfile(b, "cmyk-lut16"), SRGB(), Options{})
	src := make([]byte, 1<<20*4)
	for i := range src {
		src[i] = byte(i * 7)
	}
	dst := make([]byte, 1<<20*3)
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		_ = tr.Pixels(dst, RGB8, src, CMYK8)
	}
}

func BenchmarkNewTransformTable(b *testing.B) {
	p := testProfile(b, "rgb-mab")
	for b.Loop() {
		_ = mustTransform(b, SRGB(), p, Options{})
	}
}

// The parts of a transform between two matrix profiles convert as the
// transform does.
func TestShaperIsTheTransform(t *testing.T) {
	tr, err := NewTransform(SRGB(), AdobeRGB(), Options{Intent: Perceptual})
	if err != nil {
		t.Fatal(err)
	}
	s, ok := tr.Shaper()
	if !ok {
		t.Fatal("sRGB to Adobe RGB is not a shaper")
	}
	lerp := func(tab []float32, x float64) float64 {
		x = min(max(x, 0), 1) * float64(len(tab)-1)
		i := min(int(x), len(tab)-2)
		f := x - float64(i)
		return float64(tab[i]) + f*float64(tab[i+1]-tab[i])
	}
	for _, c := range [][3]float64{{1, 0, 0}, {0.2, 0.5, 0.9}, {0.01, 0.02, 0.005}, {1, 1, 1}} {
		var lin, out [3]float64
		for k := range 3 {
			lin[k] = lerp(s.In[k], c[k])
		}
		want := tr.Color(c[0], c[1], c[2])
		for r := range 3 {
			v := float64(s.Matrix[3*r])*lin[0] + float64(s.Matrix[3*r+1])*lin[1] + float64(s.Matrix[3*r+2])*lin[2] + float64(s.Offset[r])
			out[r] = lerp(s.Out[r], math.Sqrt(max(v, 0)))
			if d := math.Abs(out[r] - want[r]); d > 1e-3 {
				t.Errorf("%v: channel %d is %v, want %v", c, r, out[r], want[r])
			}
		}
	}
	lut, err := ParseProfile(rgbModular())
	if err != nil {
		t.Fatal(err)
	}
	if tr, err := NewTransform(SRGB(), lut, Options{}); err != nil {
		t.Fatal(err)
	} else if _, ok := tr.Shaper(); ok {
		t.Error("a table profile gave a shaper")
	}
}
