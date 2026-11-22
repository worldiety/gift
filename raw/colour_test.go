package raw

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"math"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/worldiety/gift/cms"
)

func near(a, b vec, eps float64) bool {
	for i := range a {
		if math.Abs(a[i]-b[i]) > eps {
			return false
		}
	}
	return true
}

// The working spaces map white to white: D50 is (1, 1, 1) in ProPhoto and,
// through Bradford, in Rec. 2020.
func TestTheWorkingSpacesKeepWhite(t *testing.T) {
	d50 := xyzOf(whiteD50)
	if v := xyzToProPhoto.apply(d50); !near(v, vec{1, 1, 1}, 1e-9) {
		t.Errorf("D50 in ProPhoto is %v", v)
	}
	if v := xyzToRec2020.apply(d50); !near(v, vec{1, 1, 1}, 1e-9) {
		t.Errorf("D50 in Rec. 2020 is %v", v)
	}
	// The matrix of cms for linear Rec. 2020 is the same.
	tr, err := cms.NewTransform(linearProPhoto(), cms.LinearRec2020(), cms.Options{Intent: cms.RelativeColorimetric})
	if err != nil {
		t.Fatal(err)
	}
	sh, _ := tr.Shaper()
	m := xyzToRec2020.mul(proPhotoToXYZ).f32()
	for i := range m {
		if math.Abs(float64(m[i]-sh.Matrix[i])) > 1e-4 {
			t.Fatalf("ProPhoto to Rec. 2020: %v, cms has %v", m, sh.Matrix)
		}
	}
}

func TestTemperature(t *testing.T) {
	for _, c := range []struct {
		xy cms.Chromaticity
		k  float64
	}{
		{cms.Chromaticity{X: 0.44757, Y: 0.40745}, 2856},
		{whiteD50, 5003},
		{whiteD65, 6504},
	} {
		if got := temperature(c.xy); math.Abs(got-c.k) > 15 {
			t.Errorf("%v is %.0f K, want %.0f", c.xy, got, c.k)
		}
	}
	if g := weight(2856, 2856, 6504); g != 1 {
		t.Errorf("at the first illuminant the weight is %v", g)
	}
	if g := weight(9000, 2856, 6504); g != 0 {
		t.Errorf("beyond the second the weight is %v", g)
	}
	if g := weight(4000, 2856, 6504); !(g > 0.3 && g < 0.5) {
		// Linear in mired: 1/4000 lies 0.36 of the way from 1/6504 to 1/2856.
		t.Errorf("at 4000 K the weight is %v", g)
	}
}

// a7iii is the matrix of the A7 III from the table, XYZ to camera.
func a7iii(t *testing.T) mat {
	c, ok := cameraMatrix("ILCE-7M3")
	if !ok {
		t.Fatal("no A7 III in the table")
	}
	var m mat
	for i, v := range c.matrix {
		m[i/3][i%3] = float64(v) / 10000
	}
	return m
}

// With one matrix, the camera's white balanced colours map to XYZ under
// D50 as Bradford adapts the colours the matrix saw under the white.
func TestTheColourModelOfOneMatrix(t *testing.T) {
	cm := a7iii(t)
	p := &Profile{cals: []calibration{{illuminant: 21, temperature: 6504, colour: cm}}}
	m := (&File{}).model(p)
	for _, white := range []cms.Chromaticity{whiteD65, {X: 0.40, Y: 0.39}, whiteD50} {
		n := m.neutralOf(white)
		if got := m.whiteOf(n); math.Abs(got.X-white.X)+math.Abs(got.Y-white.Y) > 1e-6 {
			t.Errorf("the neutral of %v is the white %v", white, got)
		}
		c2x, _, ok := m.cameraToXYZ(n)
		if !ok {
			t.Fatal("no matrix")
		}
		if v := c2x.apply(vec{1, 1, 1}); !near(v, xyzOf(whiteD50), 1e-9) {
			t.Errorf("white is %v", v)
		}
		// A colour under the white: the camera sees cm·v, white balance
		// divides by the neutral; back in XYZ it is v adapted to D50,
		// scaled by what the white balance made of the white.
		v := vec{0.2, 0.15, 0.08}
		cam := cm.apply(v)
		wb := vec{cam[0] / n[0], cam[1] / n[1], cam[2] / n[2]}
		y := cm.apply(xyzOf(white))
		want := adapt(white, whiteD50).apply(v)
		s := y[0] / n[0]
		got := c2x.apply(wb)
		for i := range got {
			got[i] /= s
		}
		if !near(got, want, 1e-9) {
			t.Errorf("under %v, %v comes back as %v, want %v", white, v, got, want)
		}
	}
}

// With two matrices, the white found from a neutral is the white the
// neutral was made from, at any temperature; and forward matrices made for
// the profile's illuminants give what the colour matrices give there.
func TestTheColourModelOfTwoMatrices(t *testing.T) {
	cm2 := a7iii(t)
	// Under A the camera's blue is weaker and its red stronger.
	cm1 := diag(vec{1.4, 1, 0.55}).mul(cm2)
	p := &Profile{cals: []calibration{
		{illuminant: 17, temperature: 2856, colour: cm1},
		{illuminant: 21, temperature: 6504, colour: cm2},
	}}
	m := (&File{}).model(p)
	for _, k := range []float64{2500, 3200, 4300, 5500, 7500} {
		white := daylightOrPlanck(k)
		n := m.neutralOf(white)
		got := m.whiteOf(n)
		if math.Abs(got.X-white.X)+math.Abs(got.Y-white.Y) > 1e-5 {
			t.Errorf("%.0f K: the neutral of %v is the white %v", k, white, got)
		}
		c2x, _, ok := m.cameraToXYZ(n)
		if !ok || !near(c2x.apply(vec{1, 1, 1}), xyzOf(whiteD50), 1e-9) {
			t.Errorf("%.0f K: white is %v", k, c2x.apply(vec{1, 1, 1}))
		}
	}

	// Forward matrices as the colour matrices imply them at their own
	// illuminants. McCamy's temperature of A is a little above 2856 K, so
	// that a trace of the other matrix is in either.
	forward := func(cm mat, light cms.Chromaticity) mat {
		inv, _ := cm.inverse()
		n := cm.apply(xyzOf(light))
		fm := adapt(light, whiteD50).mul(inv).mul(diag(n))
		return fm.scale(1 / (fm[1][0] + fm[1][1] + fm[1][2]))
	}
	pf := &Profile{cals: []calibration{
		{illuminant: 17, temperature: 2856, colour: cm1, forward: forward(cm1, cms.Chromaticity{X: 0.44757, Y: 0.40745}), hasForward: true},
		{illuminant: 21, temperature: 6504, colour: cm2, forward: forward(cm2, whiteD65), hasForward: true},
	}}
	mf := (&File{}).model(pf)
	for _, white := range []cms.Chromaticity{{X: 0.44757, Y: 0.40745}, whiteD65} {
		a, _, _ := m.cameraToXYZ(m.neutralOf(white))
		b, _, _ := mf.cameraToXYZ(mf.neutralOf(white))
		for i := range 3 {
			if !near(a[i], b[i], 1e-3) {
				t.Errorf("under %v the forward matrix gives %v, the colour matrix %v", white, b, a)
				break
			}
		}
	}
}

// daylightOrPlanck is the chromaticity of a light of temperature k: the
// CIE daylight locus from 4000 K, Kim's cubic of the Planckian locus below.
func daylightOrPlanck(k float64) cms.Chromaticity {
	if k >= 4000 {
		x := -4.6070e9/(k*k*k) + 2.9678e6/(k*k) + 0.09911e3/k + 0.244063
		if k > 7000 {
			x = -2.0064e9/(k*k*k) + 1.9018e6/(k*k) + 0.24748e3/k + 0.237040
		}
		return cms.Chromaticity{X: x, Y: -3*x*x + 2.87*x - 0.275}
	}
	x := -0.2661239e9/(k*k*k) - 0.2343589e6/(k*k) + 0.8776956e3/k + 0.179910
	y := -1.1063814*x*x*x - 1.34811020*x*x + 2.18555832*x - 0.20219683
	if k >= 2222 {
		y = -0.9549476*x*x*x - 1.37418593*x*x + 2.09137015*x - 0.16748867
	}
	return cms.Chromaticity{X: x, Y: y}
}

// The analog balance and calibration of a DNG are the camera's own: the
// model takes them out before the profile's matrices.
func TestAnalogBalanceAndCalibration(t *testing.T) {
	cm := a7iii(t)
	p := &Profile{signature: "lab", cals: []calibration{{illuminant: 21, temperature: 6504, colour: cm}}}
	ab := vec{1.1, 1, 0.9}
	cc := mat{{1.05, 0, 0}, {0, 1, 0}, {0, 0, 0.97}}
	f := &File{DNG: &DNGColor{
		AnalogBalance:      ab[:],
		CameraCalibration1: []float64{cc[0][0], 0, 0, 0, cc[1][1], 0, 0, 0, cc[2][2]},
	}}
	full := diag(ab).mul(cc).mul(cm)
	m := f.model(p)
	if x := m.xyzToCamera(1); !near(x[0], full[0], 1e-12) || !near(x[2], full[2], 1e-12) {
		t.Errorf("XYZ to camera is %v, want %v", x, full)
	}
	// Measured against another profile, the calibration does not apply.
	f.DNG.CameraCalibrationSignature = "other"
	if x := f.model(p).xyzToCamera(1); !near(x[0], diag(ab).mul(cm)[0], 1e-12) {
		t.Errorf("a foreign calibration was applied: %v", x)
	}
}

// floats32 is an entry of FLOATs.
func floats32(tag uint16, v ...float32) tiffEntry {
	var d []byte
	for _, x := range v {
		d = binary.LittleEndian.AppendUint32(d, math.Float32bits(x))
	}
	return tiffEntry{tag, typeFloat, uint32(len(v)), d}
}

// matrix is an SRATIONAL entry of nine values in thousandths.
func matrix(tag uint16, m mat) tiffEntry {
	var v []int32
	for i := range 9 {
		v = append(v, int32(math.Round(m[i/3][i%3]*10000)), 10000)
	}
	return srationals(tag, v...)
}

// hsvTable is a table of the given divisions whose entries f makes.
func hsvTable(h, s, v int, f func(h, s, v int) [3]float32) []float32 {
	var d []float32
	for vv := range v {
		for hh := range h {
			for ss := range s {
				e := f(hh, ss, vv)
				d = append(d, e[:]...)
			}
		}
	}
	return d
}

// buildDCP makes a camera profile as a .dcp file holds it, its illuminants
// in the order D65, A.
func buildDCP(t testing.TB) []byte {
	cm2 := mat{{0.7374, -0.2389, -0.0551}, {-0.5435, 1.3162, 0.2519}, {-0.1006, 0.1795, 0.6552}}
	cm1 := diag(vec{1.4, 1, 0.55}).mul(cm2)
	w := &tiffWriter{b: []byte("IIRC\x00\x00\x00\x00")}
	shift := func(deg float32) []float32 {
		return hsvTable(6, 3, 1, func(h, s, v int) [3]float32 { return [3]float32{deg, 1, 1} })
	}
	look := hsvTable(4, 2, 2, func(h, s, v int) [3]float32 { return [3]float32{0, 1, 1} })
	ifd0 := w.dir(
		ascii(tagUniqueCameraModel, "Sony ILCE-7M3"),
		ascii(tagProfileName, "Test Profile"),
		ascii(tagProfileCalibrationSig, "sig"),
		matrix(tagColorMatrix1, cm2), matrix(tagColorMatrix2, cm1),
		shorts(tagCalibrationIlluminant1, 21), shorts(tagCalibrationIlluminant2, 17),
		longs(tagProfileHueSatMapDims, 6, 3, 1),
		floats32(tagProfileHueSatMapData1, shift(10)...),
		floats32(tagProfileHueSatMapData2, shift(-10)...),
		longs(tagProfileLookTableDims, 4, 2, 2),
		floats32(tagProfileLookTableData, look...),
		floats32(tagProfileToneCurve, 0, 0, 0.25, 0.3, 0.5, 0.6, 1, 1),
		srationals(tagBaselineExposureOffset, 1, 2),
	)
	binary.LittleEndian.PutUint32(w.b[4:], ifd0)
	return w.b
}

func TestParseProfile(t *testing.T) {
	b := buildDCP(t)
	p, err := ParseProfile(b)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "Test Profile" || p.Camera() != "Sony ILCE-7M3" || p.signature != "sig" {
		t.Errorf("profile %q for %q, signature %q", p.Name(), p.Camera(), p.signature)
	}
	if il := p.Illuminants(); len(il) != 2 || il[0] != 17 || il[1] != 21 {
		t.Errorf("illuminants %v, want the lower temperature first", il)
	}
	// The tables went with their matrices when the two were swapped.
	if p.cals[0].hueSat[0] != -10 || p.cals[1].hueSat[0] != 10 {
		t.Errorf("hue shifts %v and %v", p.cals[0].hueSat[0], p.cals[1].hueSat[0])
	}
	if !p.HasTables() || !p.HasToneCurve() || p.look == nil || p.exposure != 0.5 {
		t.Errorf("tables %v, tone curve %v, look %v, exposure %v", p.HasTables(), p.HasToneCurve(), p.look != nil, p.exposure)
	}

	for _, c := range []struct {
		name string
		b    []byte
	}{
		{"a TIFF", append([]byte("II*\x00"), b[4:]...)},
		{"short", b[:6]},
		{"cut", b[:len(b)/2]},
		{"empty", nil},
	} {
		if _, err := ParseProfile(c.b); err == nil {
			t.Errorf("%s: no error", c.name)
		}
	}

	// A tone curve that falls is not one.
	w := &tiffWriter{b: []byte("IIRC\x00\x00\x00\x00")}
	ifd0 := w.dir(matrix(tagColorMatrix1, a7iii(t)), shorts(tagCalibrationIlluminant1, 21),
		floats32(tagProfileToneCurve, 0, 0, 0.5, 0.6, 0.6, 0.5, 1, 1))
	binary.LittleEndian.PutUint32(w.b[4:], ifd0)
	if p, err := ParseProfile(w.b); err != nil || p.HasToneCurve() {
		t.Errorf("a falling tone curve: %v, %v", err, p != nil && p.HasToneCurve())
	}
}

func FuzzParseProfile(f *testing.F) {
	f.Add(buildDCP(f))
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := ParseProfile(b)
		if err != nil {
			return
		}
		fl := &File{WhiteBalance: [3]float64{2, 1, 1.5}}
		if k, err := fl.newPipe(Options{Profile: p}, fl.WhiteBalance); err == nil {
			r := []float32{0.5, 2, -1, 0}
			k.run(Tile{Rect: image.Rect(0, 0, 4, 1), R: r, G: append([]float32(nil), r...), B: append([]float32(nil), r...), Stride: 4})
			for _, v := range r {
				if !(v >= 0 && v <= 1) {
					t.Fatalf("a picture's value %v", v)
				}
			}
		}
	})
}

func hueOf(r, g, b float32) float32 {
	v, lo := max(r, g, b), min(r, g, b)
	d := v - lo
	switch {
	case d == 0:
		return 0
	case v == r:
		return float32(math.Mod(float64((g-b)/d)+6, 6))
	case v == g:
		return 2 + (b-r)/d
	}
	return 4 + (r-g)/d
}

func TestHSVMap(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	ident := newHSVMap(90, 30, 1, hsvTable(90, 30, 1, func(h, s, v int) [3]float32 { return [3]float32{0, 1, 1} }), false)
	for range 1000 {
		r, g, b := rng.Float32(), rng.Float32(), rng.Float32()
		r2, g2, b2 := ident.apply(r, g, b)
		if math.Abs(float64(r2-r))+math.Abs(float64(g2-g))+math.Abs(float64(b2-b)) > 1e-5 {
			t.Fatalf("the identity maps %v %v %v to %v %v %v", r, g, b, r2, g2, b2)
		}
	}
	turn := newHSVMap(6, 2, 1, hsvTable(6, 2, 1, func(h, s, v int) [3]float32 { return [3]float32{120, 1, 1} }), false)
	if r, g, b := turn.apply(1, 0, 0); math.Abs(float64(r))+math.Abs(float64(g-1))+math.Abs(float64(b)) > 1e-5 {
		t.Errorf("red turned by 120 degrees is %v %v %v", r, g, b)
	}
	gray := newHSVMap(6, 2, 1, hsvTable(6, 2, 1, func(h, s, v int) [3]float32 { return [3]float32{0, 0, 1} }), false)
	if r, g, b := gray.apply(0.8, 0.2, 0.4); r != 0.8 || g != 0.8 || b != 0.8 {
		t.Errorf("without saturation, %v %v %v", r, g, b)
	}
	// Value scaled by 1 at black and 0.5 at 1: 0.75 at a half.
	dim := newHSVMap(6, 2, 2, hsvTable(6, 2, 2, func(h, s, v int) [3]float32 { return [3]float32{0, 1, 1 - 0.5*float32(v)} }), false)
	if r, _, _ := dim.apply(0.5, 0.25, 0.25); math.Abs(float64(r)-0.375) > 1e-6 {
		t.Errorf("value 0.5 scaled to %v", r)
	}
	// Indexed by sRGB, a half is at the encoding of a half, 0.735.
	dimS := newHSVMap(6, 2, 2, dim.data, true)
	if r, _, _ := dimS.apply(0.5, 0.25, 0.25); math.Abs(float64(r)-float64(srgbDecode(srgbEncode(0.5)*(1-0.5*srgbEncode(0.5))))) > 1e-5 {
		t.Errorf("value 0.5 by sRGB scaled to %v", r)
	}
	// A highlight above 1 is only lowered.
	if r, _, _ := ident.apply(3, 1, 1); r != 3 {
		t.Errorf("a highlight of 3 became %v", r)
	}
	if newHSVMap(6, 2, 1, make([]float32, 5), false) != nil {
		t.Error("a table of the wrong size")
	}
}

func TestToneCurves(t *testing.T) {
	c := defaultTone()
	if c.at(0) != 0 || c.at(float32(toneWhite)) != 1 || c.at(100) != 1 {
		t.Errorf("the curve is %v at 0 and %v at its white", c.at(0), c.at(float32(toneWhite)))
	}
	for x, prev := float32(0.0001), float32(0); x < 4; x *= 1.05 {
		if y := c.at(x); y < prev {
			t.Fatalf("the curve falls at %v", x)
		} else {
			prev = y
		}
	}
	// Middle gray, 3.7 stops below the camera's white, is sRGB 118, and
	// the camera's white is a little below white, as in the A7 III's
	// pictures.
	if y := srgbEncode(c.at(float32(math.Exp2(-3.7)))) * 255; math.Abs(float64(y)-118) > 4 {
		t.Errorf("middle gray is %.0f", y)
	}
	if y := srgbEncode(c.at(1)) * 255; y < 240 || y > 250 {
		t.Errorf("the camera's white is %.0f", y)
	}

	p := profileTone([]float64{0, 0, 0.25, 0.3, 0.5, 0.6, 1, 1})
	for _, pt := range [][2]float32{{0, 0}, {0.25, 0.3}, {0.5, 0.6}, {1, 1}} {
		if y := p.at(pt[0]); math.Abs(float64(y-pt[1])) > 1e-3 {
			t.Errorf("the profile's curve is %v at %v, want %v", y, pt[0], pt[1])
		}
	}
	rng := rand.New(rand.NewPCG(3, 4))
	for range 1000 {
		r, g, b := rng.Float32()*2, rng.Float32()*2, rng.Float32()*2
		r2, g2, b2 := c.rgb(r, g, b)
		if max(r2, g2, b2) < 1 && max(r2, g2, b2)-min(r2, g2, b2) > 1e-3 && math.Abs(float64(hueOf(r, g, b)-hueOf(r2, g2, b2))) > 1e-3 {
			t.Fatalf("the hue of %v %v %v is not kept: %v %v %v", r, g, b, r2, g2, b2)
		}
	}
	if r, g, b := c.rgb(0.3, 0.3, 0.3); r != g || g != b {
		t.Errorf("gray became %v %v %v", r, g, b)
	}
}

func TestGamutCompression(t *testing.T) {
	if r, g, b := compress(0.5, 0.3, 0.2); r != 0.5 || g != 0.3 || b != 0.2 {
		t.Errorf("a colour within the space became %v %v %v", r, g, b)
	}
	rng := rand.New(rand.NewPCG(5, 6))
	for range 10000 {
		r, g, b := rng.Float32()*3-1, rng.Float32()*3-1, rng.Float32()*3-1
		r2, g2, b2 := compress(r, g, b)
		if min(r2, g2, b2) < 0 || max(r2, g2, b2) > 1 {
			t.Fatalf("%v %v %v became %v %v %v", r, g, b, r2, g2, b2)
		}
		if max(r, g, b) > 0 && (r >= g) != (r2 >= g2) && r != g && r2 != g2 {
			t.Fatalf("the order of %v %v %v changed: %v %v %v", r, g, b, r2, g2, b2)
		}
	}
	// The edge of the space keeps 98 percent of its saturation.
	if _, g, _ := compress(1, 0, 0); g < 0.015 || g > 0.03 {
		t.Errorf("pure red is lifted by %v", g)
	}
}

// render runs f's pipe on raw, as Develop does on the file's mosaic.
func render(t *testing.T, f *File, raw []uint16, opt Options) *Planes {
	t.Helper()
	k, err := f.newPipe(opt, f.whiteBalance(opt))
	if err != nil {
		t.Fatal(err)
	}
	d, err := newDeveloper(f, raw, opt)
	if err != nil {
		t.Fatal(err)
	}
	r := f.Bounds(opt)
	n := r.Dx() * r.Dy()
	l := &Planes{Rect: r, R: make([]float32, n), G: make([]float32, n), B: make([]float32, n)}
	var mu sync.Mutex
	if err := d.run(func(tl Tile) {
		if k != nil {
			k.run(tl)
		}
		mu.Lock()
		defer mu.Unlock()
		for y := range tl.Rect.Dy() {
			for x := range tl.Rect.Dx() {
				i := (tl.Rect.Min.Y+y)*r.Dx() + tl.Rect.Min.X + x
				s := y*tl.Stride + x
				l.R[i], l.G[i], l.B[i] = tl.R[s], tl.G[s], tl.B[s]
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	return l
}

// A gray card stays gray through the whole pipe, under any white balance,
// and a stop of exposure doubles the light of the scene.
func TestGrayStaysGray(t *testing.T) {
	wb := [3]float64{2.5, 1, 1.5}
	f, raw := synthetic(64, 64, func(x, y int) [3]float64 {
		return [3]float64{0.2 / wb[0], 0.2, 0.2 / wb[2]}
	})
	f.Make, f.Model, f.WhiteBalance = "SONY", "ILCE-7M3", wb
	for _, o := range []Options{{Half: true}, {}, {Space: cms.DisplayP3()}, {Space: cms.Rec2020()}} {
		l := render(t, f, raw, o)
		i := 32*l.Rect.Dx()/2 + 16
		if math.Abs(float64(l.R[i]-l.G[i]))+math.Abs(float64(l.B[i]-l.G[i])) > 2e-3 {
			t.Errorf("%+v: gray is %v %v %v", o, l.R[i], l.G[i], l.B[i])
		}
	}
	a := render(t, f, raw, Options{Half: true, Output: OutputScene})
	b := render(t, f, raw, Options{Half: true, Output: OutputScene, Exposure: 1})
	// The mosaic is of whole numbers to 1000: blue is 133/1000 of 1.5.
	if math.Abs(float64(a.R[100]-0.2)) > 1e-3 || math.Abs(float64(b.G[100]-2*a.G[100])) > 1e-5 || math.Abs(float64(a.R[100]-a.B[100])) > 2e-3 {
		t.Errorf("the scene is %v %v %v, a stop brighter %v", a.R[100], a.G[100], a.B[100], b.G[100])
	}
}

// labD65 is CIELAB of linear sRGB, under D65.
func labD65(r, g, b float64) [3]float64 {
	x := 0.4124*r + 0.3576*g + 0.1805*b
	y := 0.2126*r + 0.7152*g + 0.0722*b
	z := 0.0193*r + 0.1192*g + 0.9505*b
	f := func(t float64) float64 {
		if t > 216.0/24389 {
			return math.Cbrt(t)
		}
		return (24389.0/27*t + 16) / 116
	}
	fx, fy, fz := f(x/0.95047), f(y), f(z/1.08883)
	return [3]float64{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

// The colours of the crops, developed, are those of the camera's own
// pictures of them: CIELAB of each of 3 by 3 squares of 80 by 80 pixels,
// measured in the A7 III's JPEGs, are met within a ΔE of 7, 4.5 on average.
// The camera's style differs, Portrait for both; this is the colour of a
// neutral rendering against the camera's.
func TestTheCameraColoursAreMet(t *testing.T) {
	for _, c := range []struct {
		file   string
		camera [9][3]float64
	}{
		{"a7iii-crop-compressed.arw", [9][3]float64{
			{55.7, -2.2, -10.2}, {63.6, -2.9, -14.0}, {59.8, -3.6, -11.3},
			{61.4, -2.4, -11.4}, {64.5, -2.8, -14.4}, {62.0, -3.7, -11.5},
			{56.5, -3.7, -7.6}, {57.8, -3.7, -10.0}, {56.5, -5.3, -7.1},
		}},
		// A wall lit by a warm lamp, part of it clipped.
		{"a7iii-crop-uncompressed.arw", [9][3]float64{
			{96.5, -4.3, 23.7}, {90.4, -1.1, 41.9}, {75.0, 7.2, 42.5},
			{96.4, -4.4, 24.4}, {90.7, -1.3, 41.2}, {74.8, 7.0, 42.2},
			{96.4, -4.3, 24.3}, {90.9, -1.5, 41.1}, {68.8, 9.0, 40.2},
		}},
	} {
		f, _ := openFixture(t, c.file)
		l, err := f.DevelopPlanes(Options{Half: true})
		if err != nil {
			t.Fatal(err)
		}
		var sum float64
		for cell := range 9 {
			var s [3]float64
			for y := cell / 3 * 40; y < cell/3*40+40; y++ {
				for x := cell % 3 * 40; x < cell%3*40+40; x++ {
					i := y*l.Rect.Dx() + x
					s[0] += float64(srgbDecode(l.R[i])) / 1600
					s[1] += float64(srgbDecode(l.G[i])) / 1600
					s[2] += float64(srgbDecode(l.B[i])) / 1600
				}
			}
			lab := labD65(s[0], s[1], s[2])
			w := c.camera[cell]
			de := math.Sqrt((lab[0]-w[0])*(lab[0]-w[0]) + (lab[1]-w[1])*(lab[1]-w[1]) + (lab[2]-w[2])*(lab[2]-w[2]))
			sum += de
			if de > 7 {
				t.Errorf("%s, square %d: %.1f %.1f %.1f, the camera's %v, ΔE %.1f", c.file, cell, lab[0], lab[1], lab[2], w, de)
			}
		}
		if sum/9 > 4.5 {
			t.Errorf("%s: ΔE %.1f on average", c.file, sum/9)
		}
		t.Logf("%s: ΔE %.1f on average", c.file, sum/9)
	}
}

func TestTheCameraTable(t *testing.T) {
	for model, ok := range map[string]bool{
		"ILCE-7M3": true, "ILCE-7": true, "ILCE-6000": true, "SLT-A99V": true, "DSC-RX100M5": true,
		"ILCE-7M4": false, "ILCE-7RM5": false, "ILCE-1": false, "DSC-RX100M7": false,
	} {
		if _, got := cameraMatrix(model); got != ok {
			t.Errorf("%s in the table: %v", model, got)
		}
	}
	f, _ := openFixture(t, "a7iii-portrait.arw")
	p, err := f.Profile()
	if err != nil || p.Camera() != "Sony ILCE-7M3" || p.HasTables() {
		t.Errorf("the A7 III's profile: %v, %v", p, err)
	}
	f.Model = "ILCE-7M4"
	if _, err := f.Profile(); !errors.Is(err, ErrUnsupported) {
		t.Errorf("an A7 IV: %v", err)
	}
	if _, err := f.RGBA(Options{Half: true}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("an A7 IV develops: %v", err)
	}
}

// A picture is made in the space asked for, of matrix profiles only, and
// RGBA and RGBA64 hold it.
func TestPictureSpaces(t *testing.T) {
	f, _ := openFixture(t, "a7iii-crop-compressed.arw")
	img, err := f.RGBA(Options{Half: true, Output: OutputScene})
	if err != nil {
		t.Fatal(err)
	}
	img16, err := f.RGBA64(Options{Half: true})
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 120, 120) || img16.Bounds() != img.Bounds() {
		t.Fatalf("bounds %v and %v", img.Bounds(), img16.Bounds())
	}
	for i := 0; i < len(img.Pix); i += 4 {
		j := 2 * i
		if img.Pix[i+3] != 0xff || img16.Pix[j+6] != 0xff || math.Abs(float64(img.Pix[i])-float64(img16.Pix[j])) > 1 {
			t.Fatalf("pixel %d: %v and %v", i/4, img.Pix[i:i+4], img16.Pix[j:j+8])
		}
	}
	p3, err := f.RGBA(Options{Half: true, Space: cms.DisplayP3()})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(p3.Pix, img.Pix) {
		t.Error("Display P3 is the same as sRGB")
	}
	if _, err := f.RGBA(Options{Half: true, Space: cms.SGray()}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("a gray picture: %v", err)
	}
}

// A profile of the user's develops a file: its tables and tone curve act,
// and a DNG's own profile is the one in the file.
func TestAProfileDevelops(t *testing.T) {
	p, err := ParseProfile(buildDCP(t))
	if err != nil {
		t.Fatal(err)
	}
	f, _ := openFixture(t, "a7iii-crop-compressed.arw")
	own, err := f.RGBA(Options{Half: true})
	if err != nil {
		t.Fatal(err)
	}
	with, err := f.RGBA(Options{Half: true, Profile: p})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(own.Pix, with.Pix) {
		t.Error("the profile changed nothing")
	}

	d, err := Open(bytes.NewReader(buildDNG(t)), int64(len(buildDNG(t))))
	if err != nil {
		t.Fatal(err)
	}
	dp, err := d.Profile()
	if err != nil || len(dp.cals) != 2 || dp.Camera() != "Testmaker T1" {
		t.Fatalf("the DNG's profile: %+v, %v", dp, err)
	}
	if _, err := d.RGBA(Options{Half: true}); err != nil {
		t.Errorf("the DNG develops: %v", err)
	}
}

// With Develop, the decoder develops the raw data, at half size when that
// is enough, in its space; a file without raw data shows its preview.
func TestTheDecoderDevelops(t *testing.T) {
	_, b := openFixture(t, "a7iii-crop-compressed.arw")
	d := Decoder{Format: ARW, Develop: true}
	cfg, err := d.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width != 240 || cfg.Height != 240 {
		t.Fatalf("%+v, %v", cfg, err)
	}
	for _, c := range []struct{ min, want int }{{100, 120}, {120, 120}, {121, 240}, {0, 120}} {
		img, err := d.DecodeScaled(bytes.NewBuffer(b), c.min, c.min)
		if err != nil || img.Bounds().Dx() != c.want {
			t.Errorf("at least %d: %v, %v", c.min, img.Bounds(), err)
		}
	}
	if img, err := d.Decode(bytes.NewBuffer(b)); err != nil || img.Bounds().Dx() != 240 {
		t.Errorf("whole: %v, %v", img.Bounds(), err)
	}
	if m := d.ScaledMemory(b, 240, 240, 100, 100); m < 2*256*256 {
		t.Errorf("memory %d", m)
	}
	if p, err := d.ReadProfile(b); err != nil || p != nil {
		t.Errorf("profile %v, %v", p, err)
	}
	p3 := Decoder{Format: ARW, Develop: true, Space: cms.DisplayP3()}
	if p, err := p3.ReadProfile(b); err != nil || p != cms.DisplayP3() {
		t.Errorf("profile %v, %v", p, err)
	}
	if v, w, x := (Decoder{}).DecoderVersion(), d.DecoderVersion(), p3.DecoderVersion(); v != "" || w != "raw1" || len(x) <= len(w) || x[:4] != "raw1" {
		t.Errorf("versions %q, %q, %q", v, w, x)
	}

	// Cut before its raw data, a file is its preview.
	_, cut := openFixture(t, "a7iii-preview.arw")
	if cfg, err := d.DecodeConfig(bytes.NewReader(cut)); err != nil || cfg.Width != 1616 {
		t.Errorf("a file cut short: %+v, %v", cfg, err)
	}
}
