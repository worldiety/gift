package raw

import (
	"encoding/binary"
	"encoding/hex"
	"image"
	"math"
	"slices"
	"sync"
	"testing"
)

// The curve of Sony's compressed raw, as the A7 III's knots make it.
func TestSonyCurve(t *testing.T) {
	c := sonyCurve([4]uint16{8000, 10400, 12900, 14100})
	for _, p := range [][2]int{{0, 0}, {1000, 1000}, {2000, 2000}, {2001, 2002}, {2600, 3200}, {3225, 5700}, {3525, 8100}, {4094, 17204}} {
		if got := int(c[p[0]]); got != p[1] {
			t.Errorf("curve[%d] = %d, want %d", p[0], got, p[1])
		}
	}
	// Without knots the curve is the identity.
	id := sonyCurve([4]uint16{})
	if id[1234] != 1234 {
		t.Errorf("no knots: %d", id[1234])
	}
}

// The raw data of the crops decodes to what the camera's files do there;
// TestWriteFixtures checked that against the whole files and wrote these
// hashes in its log.
func TestTheMosaicIsDecoded(t *testing.T) {
	for _, c := range []struct{ file, hash string }{
		{"a7iii-crop-compressed.arw", "24220e521848a461bdd878b70b1b81cc1c02ac7402c195a3cdbd1910f25d52ec"},
		{"a7iii-crop-uncompressed.arw", "cc92a699aa12b415c1a0c0b366340dbf9ed6ed7883fcac1fa3a48205a136aef1"},
	} {
		f, _ := openFixture(t, c.file)
		if f.Mosaic.Width != cropSize || f.Mosaic.Height != cropSize || f.Mosaic.Crop != image.Rect(8, 8, 248, 248) {
			t.Errorf("%s: %+v", c.file, f.Mosaic)
		}
		m, err := f.ReadMosaic()
		if err != nil {
			t.Fatal(err)
		}
		if h := mosaicHash(m); hex.EncodeToString(h[:]) != c.hash {
			t.Errorf("%s decodes to %x", c.file, h)
		}
		lo, hi := slices.Min(m), slices.Max(m)
		if lo < 256 || hi > 17204 {
			t.Errorf("%s: values from %d to %d", c.file, lo, hi)
		}
	}
	// A file cut before its raw data has none to decode.
	f, _ := openFixture(t, "a7iii-portrait.arw")
	if _, err := f.ReadMosaic(); err == nil {
		t.Error("the raw data of a file cut short decoded")
	}
}

// synthetic is a file of w by h photosites in RGGB without margins, white
// at 1000, and its mosaic sampled from the picture px, in 0 to 1.
func synthetic(w, h int, px func(x, y int) [3]float64) (*File, []uint16) {
	f := &File{Mosaic: Mosaic{
		Width: w, Height: h, Crop: image.Rect(0, 0, w, h), CFA: "RGGB",
		SamplesPerPixel: 1, BitsPerSample: 16, White: 1000,
	}, WhiteBalance: [3]float64{1, 1, 1}}
	raw := make([]uint16, w*h)
	for y := range h {
		for x := range w {
			c := [4]int{0, 1, 1, 2}[(y&1)*2+x&1]
			raw[y*w+x] = uint16(math.Round(min(px(x, y)[c], 1) * 1000))
		}
	}
	return f, raw
}

func develop(t *testing.T, f *File, raw []uint16, opt Options) *Planes {
	t.Helper()
	d, err := newDeveloper(f, raw, opt)
	if err != nil {
		t.Fatal(err)
	}
	return collect(t, f.Bounds(opt), d)
}

// RCD restores a picture whose colours change less than its brightness,
// as in a photograph: a flat gray exactly, and edges and fine detail far
// better than bilinear interpolation, which is up to 0.6 off at the edges
// and 0.012 on average at the fine detail.
func TestRCDRestoresAPicture(t *testing.T) {
	for _, c := range []struct {
		name        string
		px          func(x, y int) [3]float64
		mean, worst float64
	}{
		{"gray", func(x, y int) [3]float64 { return [3]float64{0.4, 0.4, 0.4} }, 0.0006, 0.0011},
		{"edges", func(x, y int) [3]float64 {
			fx, fy := float64(x)-150, float64(y)-140
			l := 0.15
			if fx*fx+fy*fy < 90*90 {
				l = 0.6
			}
			if (x/17)%2 == 0 && y < 60 {
				l = 0.8
			}
			return [3]float64{l * 1.1, l, l * 0.7}
		}, 0.0015, 0.3},
		{"fine", func(x, y int) [3]float64 {
			fx, fy := float64(x), float64(y)
			l := 0.4 + 0.3*math.Sin(fx*fx/600+fy/5)
			return [3]float64{l * 0.9, l, l * 0.8}
		}, 0.003, 0.025},
	} {
		f, raw := synthetic(300, 280, c.px)
		l := develop(t, f, raw, Options{})
		var worst, sum, n float64
		for y := 12; y < 280-12; y++ {
			for x := 12; x < 300-12; x++ {
				i := y*300 + x
				want := c.px(x, y)
				for k, v := range [3]float32{l.R[i], l.G[i], l.B[i]} {
					e := math.Abs(float64(v) - want[k])
					worst = max(worst, e)
					sum += e
					n++
				}
			}
		}
		if sum/n > c.mean || worst > c.worst {
			t.Errorf("%s: %.5f off on average, up to %.4f", c.name, sum/n, worst)
		}
		t.Logf("%s: %.5f off on average, up to %.4f", c.name, sum/n, worst)
	}
}

// The same picture comes out of one worker and of many, and half size is
// full size averaged over 2 by 2.
func TestDevelopIsTheSameInEveryWay(t *testing.T) {
	f, _ := openFixture(t, "a7iii-crop-compressed.arw")
	raw, err := f.ReadMosaic()
	if err != nil {
		t.Fatal(err)
	}
	one := develop(t, f, raw, Options{Workers: 1})
	many := develop(t, f, raw, Options{Workers: 7})
	if !slices.Equal(one.R, many.R) || !slices.Equal(one.G, many.G) || !slices.Equal(one.B, many.B) {
		t.Error("the workers develop differently")
	}
	if one.Rect != image.Rect(0, 0, 240, 240) {
		t.Errorf("%v", one.Rect)
	}
	half := develop(t, f, raw, Options{Half: true})
	if half.Rect != image.Rect(0, 0, 120, 120) {
		t.Fatalf("%v", half.Rect)
	}
	var diff, sum float64
	for y := 2; y < 118; y++ {
		for x := 2; x < 118; x++ {
			for k, pl := range [3][]float32{one.R, one.G, one.B} {
				var m float64
				for p := range 4 {
					m += float64(pl[(2*y+p>>1)*240+2*x+p&1]) / 4
				}
				h := float64([3][]float32{half.R, half.G, half.B}[k][y*120+x])
				diff += math.Abs(h - m)
				sum += m
			}
		}
	}
	if diff/sum > 0.05 {
		t.Errorf("half size is %.1f%% off full size averaged", 100*diff/sum)
	}
	t.Logf("half size is %.2f%% off full size averaged", 100*diff/sum)
}

// Where green clipped below red, it is raised to what the pixels around
// the clipped area say; clipped, no channel is above white; and what
// clipped in all three is white, not the magenta of green clipped lowest.
func TestHighlights(t *testing.T) {
	f, _ := openFixture(t, "a7iii-crop-uncompressed.arw")
	raw, err := f.ReadMosaic()
	if err != nil {
		t.Fatal(err)
	}
	d, err := newDeveloper(f, raw, Options{Half: true})
	if err != nil {
		t.Fatal(err)
	}
	mask := d.clip
	if mask == nil {
		t.Fatal("nothing clipped")
	}
	d.clip = nil
	bare := collect(t, f.Bounds(Options{Half: true}), d)
	rec := develop(t, f, raw, Options{Half: true})
	// The pixels whose block clipped in green alone: a warm lamp, red
	// close to clipping, so that green clipped at a third of it.
	var g0, g1, n float64
	for y := range 120 {
		for x := range 120 {
			if mask[(y+4)*d.bw+x+4]&7 != 2 {
				continue
			}
			i := y*120 + x
			g0 += float64(bare.G[i])
			g1 += float64(rec.G[i])
			n++
		}
	}
	if n < 100 || g1 < 1.2*g0 || g1/n > float64(d.top) {
		t.Errorf("%v pixels clipped in green: %.2f untreated, %.2f reconstructed", n, g0/n, g1/n)
	}
	t.Logf("%v pixels clipped in green: %.2f untreated, %.2f reconstructed", n, g0/n, g1/n)

	clip := develop(t, f, raw, Options{Highlights: HighlightsClip})
	for i := range clip.G {
		if clip.R[i] > 1 || clip.G[i] > 1 || clip.B[i] > 1 {
			t.Fatalf("clipped, pixel %d is %v %v %v", i, clip.R[i], clip.G[i], clip.B[i])
		}
	}

	// A white disc that clipped in all three under a white balance that
	// lifts red and blue.
	sf, sraw := synthetic(120, 120, func(x, y int) [3]float64 {
		fx, fy := float64(x)-60, float64(y)-60
		if fx*fx+fy*fy < 30*30 {
			return [3]float64{1, 1, 1}
		}
		return [3]float64{0.2, 0.3, 0.25}
	})
	sf.WhiteBalance = [3]float64{2, 1, 1.5}
	for _, h := range []Highlights{HighlightsReconstruct, HighlightsClip} {
		l := develop(t, sf, sraw, Options{Highlights: h})
		i := 60*120 + 60
		if l.R[i] != l.G[i] || l.G[i] != l.B[i] {
			t.Errorf("highlights %d: the disc is %v %v %v", h, l.R[i], l.G[i], l.B[i])
		}
	}
}

// collect runs d into one picture of bounds r.
func collect(t *testing.T, r image.Rectangle, d *developer) *Planes {
	n := r.Dx() * r.Dy()
	l := &Planes{Rect: r, R: make([]float32, n), G: make([]float32, n), B: make([]float32, n)}
	var mu sync.Mutex
	if err := d.run(func(tl Tile) {
		mu.Lock()
		defer mu.Unlock()
		for y := range tl.Rect.Dy() {
			o := (tl.Rect.Min.Y+y)*r.Dx() + tl.Rect.Min.X
			copy(l.R[o:o+tl.Rect.Dx()], tl.R[y*tl.Stride:])
			copy(l.G[o:o+tl.Rect.Dx()], tl.G[y*tl.Stride:])
			copy(l.B[o:o+tl.Rect.Dx()], tl.B[y*tl.Stride:])
		}
	}); err != nil {
		t.Fatal(err)
	}
	return l
}

// A block that says its maximum and minimum are at one place has a step
// more than fits; it decodes, to something, without reading past itself.
// The fuzzer found it.
func TestADamagedBlockOfSonysCompressedRaw(t *testing.T) {
	src := make([]byte, 32)
	for i := range src {
		src[i] = 0xff
	}
	// maximum 2047, minimum 0, both at place 3.
	binary.LittleEndian.PutUint32(src, 0x7ff|3<<22|3<<26)
	dst := make([]uint16, 32)
	curve := sonyCurve([4]uint16{})
	decodeSonyCompressed(dst, src, 32, 1, &curve)
	if dst[6] != 4094 {
		t.Errorf("the maximum at place 3 is %d", dst[6])
	}
}
