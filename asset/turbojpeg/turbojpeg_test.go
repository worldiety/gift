package turbojpeg

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"testing"
)

// --- without the library ----------------------------------------------------

func TestChooseScale(t *testing.T) {
	cases := []struct {
		name             string
		w, h, minW, minH int
		want             scale
	}{
		{"full size asked", 4000, 3000, 4000, 3000, scale{1, 1}},
		{"nothing asked is full size", 4000, 3000, 0, 0, scale{1, 1}},
		{"more than there is is full size", 400, 300, 1000, 1000, scale{1, 1}},
		{"exactly an eighth", 4000, 3000, 500, 375, scale{1, 8}},
		{"a pixel over an eighth", 4000, 3000, 501, 375, scale{1, 4}},
		{"exactly a quarter", 4000, 3000, 1000, 750, scale{1, 4}},
		{"over a quarter is a half, not three eighths", 4000, 3000, 1024, 768, scale{1, 2}},
		{"exactly a half", 4000, 3000, 2000, 1500, scale{1, 2}},
		{"the taller axis decides", 4000, 3000, 100, 1000, scale{1, 2}},
		{"a 256 tile of a 12 megapixel photograph", 4000, 3000, 256, 192, scale{1, 8}},
		{"TJSCALED rounds up", 4001, 3001, 501, 376, scale{1, 8}},
		{"tiny picture", 7, 5, 1, 1, scale{1, 8}},
		{"only one axis asked", 4000, 3000, 900, 0, scale{1, 1}},
		{"over a half is full size, not seven eighths", 4000, 3000, 2001, 1500, scale{1, 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := chooseScale(fallbackScales, c.w, c.h, c.minW, c.minH)
			if got != c.want {
				t.Fatalf("chooseScale(%dx%d, at least %dx%d) = %d/%d, want %d/%d",
					c.w, c.h, c.minW, c.minH, got.num, got.denom, c.want.num, c.want.denom)
			}
			// Whatever was chosen, the quality promise holds.
			if got.of(c.w) < min(c.w, c.minW) || got.of(c.h) < min(c.h, c.minH) {
				t.Fatalf("%d/%d gives %dx%d, smaller than asked", got.num, got.denom,
					got.of(c.w), got.of(c.h))
			}
		})
	}
}

func TestChooseScaleNeverEnlargesAndSurvivesAnEmptyTable(t *testing.T) {
	if got := chooseScale(nil, 100, 100, 10, 10); got != (scale{1, 1}) {
		t.Errorf("empty table: %v, want 1/1", got)
	}
	// Enlarging factors are in the library's table and must never win.
	if got := chooseScale([]scale{{2, 1}, {15, 8}}, 100, 100, 100, 100); got != (scale{1, 1}) {
		t.Errorf("only enlarging factors: %v, want 1/1", got)
	}
}

func TestSniff(t *testing.T) {
	var d Decoder
	if !d.Sniff(encode(t, testPicture(8, 8), 90)[:3]) {
		t.Error("a JPEG was not recognised")
	}
	if d.Sniff([]byte("\x89PNG\r\n\x1a\n")) || d.Sniff([]byte{0xFF, 0xD8}) {
		t.Error("not a JPEG, or too short to tell, was recognised")
	}
}

func TestParseFrame(t *testing.T) {
	f := parseFrame(encode(t, testPicture(64, 48), 80))
	if !f.ok || f.progressive || len(f.comps) != 3 || f.firstScan != 3 {
		t.Fatalf("baseline colour: %+v", f)
	}
	// image/jpeg writes 4:2:0: luma 2x2, chroma 1x1.
	if f.comps[0] != [2]int{2, 2} || f.comps[1] != [2]int{1, 1} {
		t.Errorf("sampling = %v, want 4:2:0", f.comps)
	}

	gray := image.NewGray(image.Rect(0, 0, 16, 16))
	f = parseFrame(encode(t, gray, 80))
	if !f.ok || len(f.comps) != 1 || f.firstScan != 1 {
		t.Errorf("gray: %+v", f)
	}

	if f := parseFrame(progressiveHeader()); !f.ok || !f.progressive || f.firstScan != 1 {
		t.Errorf("progressive: %+v", f)
	}
	for _, bad := range [][]byte{nil, {0xFF, 0xD8}, []byte("not a jpeg"), {0xFF, 0xD8, 0xFF, 0xD9}} {
		if f := parseFrame(bad); f.ok {
			t.Errorf("parseFrame(%q) = %+v, want not ok", bad, f)
		}
	}
}

// progressiveHeader is SOI, a progressive SOF2 for a 6000x4000 4:2:0 picture,
// and the first scan header of a DC scan over one component. Go cannot write
// progressive JPEG, and the header is all the memory estimate reads.
func progressiveHeader() []byte {
	return []byte{
		0xFF, 0xD8,
		0xFF, 0xC2, 0x00, 0x11, 0x08, 0x0F, 0xA0, 0x17, 0x70, 0x03,
		0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01,
		0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
	}
}

// The memory figure is what makes scaling pay in the budget, and it has to
// keep the progressive risk that scaling does not remove.
func TestWorkingMemory(t *testing.T) {
	const w, h = 6000, 4000
	base := parseFrame(encode(t, testPicture(w/10, h/10), 80)) // same layout, 4:2:0 baseline
	prog := parseFrame(progressiveHeader())
	eighth, full := scale{1, 8}, scale{1, 1}

	b8 := workingMemory(base, w, h, eighth)
	p8 := workingMemory(prog, w, h, eighth)
	b1 := workingMemory(base, w, h, full)
	t.Logf("24 MP 4:2:0: baseline 1/8 %.1f MB, progressive 1/8 %.1f MB, baseline 1/1 %.1f MB",
		float64(b8)/1e6, float64(p8)/1e6, float64(b1)/1e6)

	// At an eighth a baseline file needs the 750x500 output and some rows.
	if out := int64(4 * 750 * 500); b8 < out || b8 > 4*out {
		t.Errorf("baseline at 1/8 = %d bytes, want a small multiple of the %d output", b8, out)
	}
	// A progressive one keeps 3 bytes per pixel of coefficients at 4:2:0.
	if coef := int64(3 * w * h); p8 < coef {
		t.Errorf("progressive at 1/8 = %d bytes, less than its %d of coefficients", p8, coef)
	}
	if b1 < 4*w*h {
		t.Errorf("baseline full size = %d bytes, less than its RGBA output", b1)
	}
	// Unreadable headers get the worst case, never less than a real one.
	if u := workingMemory(frame{}, w, h, eighth); u < p8 {
		t.Errorf("unknown header = %d, below the progressive %d", u, p8)
	}
}

// --- with the library -------------------------------------------------------

func needLibrary(t testing.TB) {
	t.Helper()
	if !Available() {
		t.Skipf("libturbojpeg not installed: %v", load())
	}
}

func TestDecodeMatchesImageJPEG(t *testing.T) {
	needLibrary(t)
	data := encode(t, testPicture(640, 480), 90)
	want, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != want.Bounds() {
		t.Fatalf("bounds %v, want %v", got.Bounds(), want.Bounds())
	}
	if e := meanAbsError(got, want); e > 1.5 {
		t.Errorf("mean absolute error against image/jpeg = %.3f, want at most 1.5", e)
	} else {
		t.Logf("mean absolute error against image/jpeg = %.3f", e)
	}
	for i := 3; i < len(got.Pix); i += 4 {
		if got.Pix[i] != 0xFF {
			t.Fatalf("alpha %d at byte %d, want opaque", got.Pix[i], i)
		}
	}
}

func TestDecodeScaledPicksTheSmallestSufficientFactor(t *testing.T) {
	needLibrary(t)
	data := encode(t, testPicture(1000, 600), 90)
	cases := []struct{ minW, minH, wantW, wantH int }{
		{0, 0, 1000, 600},
		{500, 300, 500, 300},
		{250, 150, 250, 150},
		{251, 150, 500, 300},
		{100, 10, 125, 75},
		{1, 1, 125, 75},
		{5000, 5000, 1000, 600},
	}
	for _, c := range cases {
		img, err := DecodeScaled(data, c.minW, c.minH)
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != c.wantW || b.Dy() != c.wantH {
			t.Errorf("at least %dx%d: got %dx%d, want %dx%d",
				c.minW, c.minH, b.Dx(), b.Dy(), c.wantW, c.wantH)
		}
	}

	// The scaled picture is the same picture: compare an eighth against
	// image/jpeg's full decode shrunk by averaging 8x8 blocks, which is
	// what the DC-only IDCT computes.
	small, err := DecodeScaled(data, 125, 75)
	if err != nil {
		t.Fatal(err)
	}
	full, _ := jpeg.Decode(bytes.NewReader(data))
	if e := meanAbsError(small, boxShrink(full, 8)); e > 3 {
		t.Errorf("1/8 decode differs from a box-filtered full decode by %.2f on average", e)
	}
}

func TestDecodeGray(t *testing.T) {
	needLibrary(t)
	g := image.NewGray(image.Rect(0, 0, 40, 30))
	for i := range g.Pix {
		g.Pix[i] = uint8(i)
	}
	img, err := Decode(encode(t, g, 95))
	if err != nil {
		t.Fatal(err)
	}
	c := img.RGBAAt(20, 15)
	if c.R != c.G || c.G != c.B || c.A != 0xFF {
		t.Errorf("gray decoded to %v, want equal channels and opaque", c)
	}
}

func TestBrokenInputFails(t *testing.T) {
	needLibrary(t)
	data := encode(t, testPicture(320, 240), 90)
	for name, in := range map[string][]byte{
		"empty":     nil,
		"garbage":   []byte("this is not a jpeg at all, not even close"),
		"truncated": data[:len(data)/2],
	} {
		if _, err := Decode(in); err == nil {
			t.Errorf("%s: decoded without an error", name)
		}
	}
}

func TestDecoderTakesTheBuffersBytes(t *testing.T) {
	needLibrary(t)
	data := encode(t, testPicture(64, 64), 90)
	var d Decoder
	img, err := d.DecodeScaled(bytes.NewBuffer(data), 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*image.RGBA); !ok || img.Bounds().Dx() != 8 {
		t.Errorf("got %T %v, want an 8x8 *image.RGBA from the library", img, img.Bounds())
	}
	// And a reader without Bytes is read to the end.
	img, err = d.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != 64 {
		t.Errorf("plain reader: %v, %v", img.Bounds(), err)
	}
	if d.MemoryFactor() != 10 {
		t.Errorf("MemoryFactor = %v with the library, want 10", d.MemoryFactor())
	}
}

// --- helpers ----------------------------------------------------------------

// testPicture is a gradient with a little texture, so that the DCT has high
// frequencies to lose and a wrong decode shows.
func testPicture(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var seed uint32 = 1
	for y := range h {
		for x := range w {
			seed = seed*1664525 + 1013904223
			n := uint8(seed >> 28)
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x*200/max(1, w-1)) + n,
				G: uint8(y*200/max(1, h-1)) + n,
				B: uint8((x+y)*100/max(1, w+h-2)) + 0x30,
				A: 0xFF,
			})
		}
	}
	return img
}

func encode(t testing.TB, img image.Image, q int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	r := image.NewRGBA(img.Bounds())
	draw.Draw(r, r.Bounds(), img, img.Bounds().Min, draw.Src)
	return r
}

func meanAbsError(a, b image.Image) float64 {
	ra, rb := toRGBA(a), toRGBA(b)
	var sum, n float64
	for i := range ra.Pix {
		if i%4 == 3 {
			continue
		}
		d := int(ra.Pix[i]) - int(rb.Pix[i])
		sum += float64(max(d, -d))
		n++
	}
	return sum / n
}

// boxShrink averages k by k blocks.
func boxShrink(img image.Image, k int) *image.RGBA {
	src := toRGBA(img)
	b := src.Bounds()
	w, h := (b.Dx()+k-1)/k, (b.Dy()+k-1)/k
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			var s [3]int
			n := 0
			for yy := y * k; yy < min(b.Dy(), y*k+k); yy++ {
				for xx := x * k; xx < min(b.Dx(), x*k+k); xx++ {
					o := src.PixOffset(xx, yy)
					s[0] += int(src.Pix[o])
					s[1] += int(src.Pix[o+1])
					s[2] += int(src.Pix[o+2])
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(s[0] / n), uint8(s[1] / n), uint8(s[2] / n), 0xFF})
		}
	}
	return dst
}
