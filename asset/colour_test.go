package asset_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/cms"
)

// --- pictures with colour ----------------------------------------------------

func flatJPEG(t testing.TB, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 48, 32))
	for i := range img.Pix {
		img.Pix[i] = 0
	}
	for y := range 32 {
		for x := range 48 {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodePNG(t testing.TB, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withChunks inserts PNG chunks right after the header.
func withChunks(t testing.TB, b []byte, chunks ...[]byte) []byte {
	t.Helper()
	const ihdrEnd = 8 + 12 + 13
	out := append([]byte{}, b[:ihdrEnd]...)
	for _, c := range chunks {
		out = append(out, c...)
	}
	return append(out, b[ihdrEnd:]...)
}

func chunk(typ string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, typ...)
	out = append(out, data...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[4:]))
}

func u32s(v ...uint32) []byte {
	var b []byte
	for _, x := range v {
		b = binary.BigEndian.AppendUint32(b, x)
	}
	return b
}

// withEXIFColour splices an EXIF segment with a colour space tag and, when
// interop is not empty, an interoperability IFD with that index.
func withEXIFColour(t testing.TB, jpg []byte, colourSpace uint16, interop string) []byte {
	t.Helper()
	le := binary.LittleEndian
	var tiff []byte
	tiff = append(tiff, 'I', 'I', 42, 0)
	tiff = le.AppendUint32(tiff, 8)
	// IFD0 at 8: one entry, the EXIF IFD pointer.
	const exifAt = 8 + 2 + 12 + 4
	tiff = le.AppendUint16(tiff, 1)
	tiff = le.AppendUint16(tiff, 0x8769)
	tiff = le.AppendUint16(tiff, 4)
	tiff = le.AppendUint32(tiff, 1)
	tiff = le.AppendUint32(tiff, exifAt)
	tiff = le.AppendUint32(tiff, 0)
	// The EXIF IFD: ColorSpace and the interoperability pointer.
	const interopAt = exifAt + 2 + 2*12 + 4
	tiff = le.AppendUint16(tiff, 2)
	tiff = le.AppendUint16(tiff, 0xA001)
	tiff = le.AppendUint16(tiff, 3)
	tiff = le.AppendUint32(tiff, 1)
	tiff = le.AppendUint16(tiff, colourSpace)
	tiff = le.AppendUint16(tiff, 0)
	tiff = le.AppendUint16(tiff, 0xA005)
	tiff = le.AppendUint16(tiff, 4)
	tiff = le.AppendUint32(tiff, 1)
	if interop != "" {
		tiff = le.AppendUint32(tiff, interopAt)
	} else {
		tiff = le.AppendUint32(tiff, 0)
	}
	tiff = le.AppendUint32(tiff, 0)
	// The interoperability IFD: the index, four ASCII bytes inline.
	tiff = le.AppendUint16(tiff, 1)
	tiff = le.AppendUint16(tiff, 0x0001)
	tiff = le.AppendUint16(tiff, 2)
	tiff = le.AppendUint32(tiff, 4)
	tiff = append(tiff, (interop + "\x00\x00\x00\x00")[:4]...)
	tiff = le.AppendUint32(tiff, 0)

	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1}
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(payload)+2))
	seg = append(seg, payload...)
	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

func testProfile(t testing.TB, name string) *cms.Profile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "cms", "testdata", "profiles", name))
	if err != nil {
		t.Fatal(err)
	}
	p, err := cms.ParseProfile(b)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func mustEmbed(t testing.TB, b []byte, p *cms.Profile) []byte {
	t.Helper()
	out, err := asset.EmbedProfile(b, p)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func appSegments(b []byte, marker byte) int {
	n := 0
	for i := 2; i+4 <= len(b) && b[i] == 0xFF && b[i+1] != 0xDA; {
		if b[i+1] == marker {
			n++
		}
		i += 2 + int(binary.BigEndian.Uint16(b[i+2:]))
	}
	return n
}

// --- reading and writing profiles ---------------------------------------------

func TestEmbedAndReadJPEG(t *testing.T) {
	jpg := flatJPEG(t, color.RGBA{100, 150, 200, 255})
	if p, err := asset.ReadProfile(jpg); p != nil || err != nil {
		t.Fatalf("untagged JPEG: %v, %v", p, err)
	}
	tagged := mustEmbed(t, jpg, cms.AdobeRGB())
	p, err := asset.ReadProfile(tagged)
	if err != nil || p == nil || !bytes.Equal(p.Bytes(), cms.AdobeRGB().Bytes()) {
		t.Fatalf("read back: %v, %v", p, err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(tagged)); err != nil {
		t.Fatalf("tagged JPEG no longer decodes: %v", err)
	}
	// The ICC segments come after the EXIF segment, which stays first.
	withExif := mustEmbed(t, withEXIFColour(t, jpg, 1, ""), cms.AdobeRGB())
	next := 4 + int(binary.BigEndian.Uint16(withExif[4:]))
	if withExif[3] != 0xE1 || withExif[next+1] != 0xE2 {
		t.Errorf("segments %02x, %02x, want EXIF APP1, then ICC APP2", withExif[3], withExif[next+1])
	}

	// Embedding again replaces the profile.
	again := mustEmbed(t, tagged, cms.DisplayP3())
	if n := appSegments(again, 0xE2); n != 1 {
		t.Errorf("%d APP2 segments after embedding twice, want 1", n)
	}
	if p, _ := asset.ReadProfile(again); p == nil || p.Description() != "Display P3" {
		t.Errorf("after embedding twice: %v", p)
	}

	// A profile larger than a segment is split, and read back whole.
	big := testProfile(t, "cmyk-lut16.icc")
	cmykJPEG, err := os.ReadFile(filepath.Join("testdata", "cmyk.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	split := mustEmbed(t, cmykJPEG, big)
	if n := appSegments(split, 0xE2); n != 3 {
		t.Errorf("%d bytes in %d APP2 segments, want 3", len(big.Bytes()), n)
	}
	if p, err := asset.ReadProfile(split); err != nil || !bytes.Equal(p.Bytes(), big.Bytes()) {
		t.Errorf("split profile read back: %v", err)
	}

	// A part missing is an error, not a profile.
	var cut []byte
	cut = append(cut, split[:2]...)
	seen := 0
	for i := 2; i+4 <= len(split); {
		size := int(binary.BigEndian.Uint16(split[i+2:]))
		if split[i+1] == 0xDA {
			cut = append(cut, split[i:]...)
			break
		}
		if split[i+1] == 0xE2 {
			seen++
			if seen == 2 {
				i += 2 + size
				continue
			}
		}
		cut = append(cut, split[i:i+2+size]...)
		i += 2 + size
	}
	if p, err := asset.ReadProfile(cut); err == nil || p != nil {
		t.Errorf("a profile with a part missing read as %v, %v", p, err)
	}
}

func TestEmbedRefusesWhatDoesNotFit(t *testing.T) {
	rgbJPEG := flatJPEG(t, color.RGBA{1, 2, 3, 255})
	if _, err := asset.EmbedProfile(rgbJPEG, cms.SGray()); err == nil {
		t.Error("a gray profile went into a colour JPEG")
	}
	gray := image.NewGray(image.Rect(0, 0, 8, 8))
	var gj bytes.Buffer
	if err := jpeg.Encode(&gj, gray, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := asset.EmbedProfile(gj.Bytes(), cms.AdobeRGB()); err != nil {
		t.Errorf("an RGB profile on a gray JPEG: %v", err)
	}
	if _, err := asset.EmbedProfile(encodePNG(t, gray), testProfile(t, "cmyk-lut16.icc")); err == nil {
		t.Error("a CMYK profile went into a PNG")
	}
	if _, err := asset.EmbedProfile([]byte("GIF89a......"), cms.SRGB()); err == nil {
		t.Error("EmbedProfile wrote into a GIF")
	}
}

func TestEmbedAndReadPNG(t *testing.T) {
	b := encodePNG(t, testImage(16, 8))
	tagged := mustEmbed(t, b, cms.ProPhotoRGB())
	p, err := asset.ReadProfile(tagged)
	if err != nil || p == nil || !bytes.Equal(p.Bytes(), cms.ProPhotoRGB().Bytes()) {
		t.Fatalf("read back: %v, %v", p, err)
	}
	if _, err := png.Decode(bytes.NewReader(tagged)); err != nil {
		t.Fatalf("tagged PNG no longer decodes: %v", err)
	}
	// Chunks that would contradict the profile are removed, cICP above all.
	contra := withChunks(t, b, chunk("cICP", []byte{1, 13, 0, 1}), chunk("gAMA", u32s(45455)))
	if p, _ := asset.ReadProfile(contra); p != cms.SRGB() {
		t.Fatalf("cICP 1/13 read as %v", p)
	}
	re := mustEmbed(t, contra, cms.AdobeRGB())
	if p, _ := asset.ReadProfile(re); p == nil || p.Description() != "Adobe RGB (1998)" {
		t.Errorf("after embedding over cICP: %v", p)
	}
	if bytes.Contains(re, []byte("cICP")) || bytes.Contains(re, []byte("gAMA")) {
		t.Error("cICP or gAMA left beside the profile")
	}
}

func TestPNGDeclarations(t *testing.T) {
	rgb := encodePNG(t, testImage(4, 4))
	gray := encodePNG(t, image.NewGray(image.Rect(0, 0, 4, 4)))
	iccp := func(p *cms.Profile) []byte {
		var z bytes.Buffer
		w := zlib.NewWriter(&z)
		w.Write(p.Bytes())
		w.Close()
		return chunk("iCCP", append([]byte("x\x00\x00"), z.Bytes()...))
	}
	// D65 and the primaries of Display P3, times 100000.
	p3 := chunk("cHRM", u32s(31270, 32900, 68000, 32000, 26500, 69000, 15000, 6000))
	for _, c := range []struct {
		name string
		png  []byte
		want string // description, "" for none
	}{
		{"nothing", rgb, ""},
		{"sRGB", withChunks(t, rgb, chunk("sRGB", []byte{0})), "sRGB"},
		{"gAMA 1/2.2 is sRGB", withChunks(t, rgb, chunk("gAMA", u32s(45455))), "sRGB"},
		{"gAMA 1", withChunks(t, rgb, chunk("gAMA", u32s(100000))), "gAMA and cHRM"},
		{"gAMA and cHRM of P3", withChunks(t, rgb, chunk("gAMA", u32s(45455)), p3), "gAMA and cHRM"},
		{"iCCP", withChunks(t, rgb, iccp(cms.AdobeRGB())), "Adobe RGB (1998)"},
		{"iCCP over sRGB", withChunks(t, rgb, chunk("sRGB", []byte{0}), iccp(cms.AdobeRGB())), "Adobe RGB (1998)"},
		{"cICP over iCCP", withChunks(t, rgb, iccp(cms.AdobeRGB()), chunk("cICP", []byte{12, 13, 0, 1})), "Display P3"},
		{"cICP BT.2020", withChunks(t, rgb, chunk("cICP", []byte{9, 1, 0, 1})), "Rec. 2020"},
		{"cICP DCI-P3", withChunks(t, rgb, chunk("cICP", []byte{11, 13, 0, 1})), "cICP 11/13"},
		{"cICP PQ falls through", withChunks(t, rgb, chunk("cICP", []byte{9, 16, 0, 1}), iccp(cms.AdobeRGB())), "Adobe RGB (1998)"},
		{"cICP PQ alone", withChunks(t, rgb, chunk("cICP", []byte{9, 16, 0, 1})), ""},
		{"cICP narrow range", withChunks(t, rgb, chunk("cICP", []byte{12, 13, 0, 0})), ""},
		{"gray sRGB", withChunks(t, gray, chunk("sRGB", []byte{0})), "sGray"},
		{"gray gAMA 1", withChunks(t, gray, chunk("gAMA", u32s(100000))), "gAMA"},
		{"gray cICP", withChunks(t, gray, chunk("cICP", []byte{1, 8, 0, 1})), "cICP 1/8"},
	} {
		p, err := asset.ReadProfile(c.png)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		got := ""
		if p != nil {
			got = p.Description()
		}
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}

	// gAMA 1 is linear: a middle gray of linear light is 188 in sRGB.
	p, _ := asset.ReadProfile(withChunks(t, rgb, chunk("gAMA", u32s(100000))))
	tr, err := cms.NewTransform(p, cms.SRGB(), cms.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if v := tr.Color(0.5, 0.5, 0.5); int(v[0]*255+0.5) != 188 {
		t.Errorf("linear 0.5 is %.1f in sRGB, want 188", v[0]*255)
	}

	// Broken declarations are errors.
	for name, b := range map[string][]byte{
		"gAMA 0":        withChunks(t, rgb, chunk("gAMA", u32s(0))),
		"iCCP not zlib": withChunks(t, rgb, chunk("iCCP", []byte("x\x00\x00garbage"))),
		"iCCP method 1": withChunks(t, rgb, chunk("iCCP", []byte("x\x00\x01"))),
	} {
		if p, err := asset.ReadProfile(b); err == nil {
			t.Errorf("%s: read as %v without an error", name, p)
		}
	}

	// A profile that inflates past the bound is refused, not read.
	var z bytes.Buffer
	w, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	w.Write(make([]byte, 9<<20))
	w.Close()
	bomb := withChunks(t, rgb, chunk("iCCP", append([]byte("x\x00\x00"), z.Bytes()...)))
	if _, err := asset.ReadProfile(bomb); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("a 9 MiB profile: %v", err)
	}
}

func TestEXIFAdobeRGB(t *testing.T) {
	jpg := flatJPEG(t, color.RGBA{1, 2, 3, 255})
	for _, c := range []struct {
		cs      uint16
		interop string
		adobe   bool
	}{
		{0xFFFF, "R03", true},
		{0xFFFF, "R98", false},
		{0xFFFF, "", false},
		{2, "", true},
		{1, "R03", false},
		{1, "", false},
	} {
		p, err := asset.ReadProfile(withEXIFColour(t, jpg, c.cs, c.interop))
		if err != nil {
			t.Fatal(err)
		}
		if got := p == cms.AdobeRGB(); got != c.adobe {
			t.Errorf("colour space %#x, index %q: Adobe RGB %v, want %v", c.cs, c.interop, got, c.adobe)
		}
	}
	// An ICC profile wins over the EXIF.
	both := mustEmbed(t, withEXIFColour(t, jpg, 0xFFFF, "R03"), cms.DisplayP3())
	if p, _ := asset.ReadProfile(both); p == nil || p.Description() != "Display P3" {
		t.Errorf("ICC and R03: %v", p)
	}
}

func TestWebPProfile(t *testing.T) {
	riff := func(chunks ...[]byte) []byte {
		var body []byte
		body = append(body, "WEBP"...)
		for _, c := range chunks {
			body = append(body, c...)
		}
		out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
		return append(out, body...)
	}
	ch := func(typ string, data []byte) []byte {
		out := append([]byte(typ), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
		out = append(out, data...)
		if len(data)%2 == 1 {
			out = append(out, 0)
		}
		return out
	}
	vp8x := ch("VP8X", []byte{0x20, 0, 0, 0, 3, 0, 0, 3, 0, 0})
	pic := ch("VP8L", []byte{0x2f, 1, 2, 3, 4})
	p, err := asset.ReadProfile(riff(vp8x, ch("ICCP", cms.AdobeRGB().Bytes()), pic))
	if err != nil || p == nil || p.Description() != "Adobe RGB (1998)" {
		t.Errorf("ICCP chunk: %v, %v", p, err)
	}
	if p, err := asset.ReadProfile(riff(pic)); p != nil || err != nil {
		t.Errorf("simple WebP: %v, %v", p, err)
	}
	if _, err := asset.ReadProfile(riff(vp8x, ch("ICCP", []byte("not a profile")), pic)); err == nil {
		t.Error("a broken ICCP chunk is no error")
	}
}

func FuzzReadProfile(f *testing.F) {
	jpg := flatJPEG(f, color.RGBA{1, 2, 3, 255})
	f.Add(jpg)
	f.Add(mustEmbed(f, jpg, cms.AdobeRGB()))
	f.Add(withEXIFColour(f, jpg, 0xFFFF, "R03"))
	rgb := encodePNG(f, testImage(4, 4))
	f.Add(mustEmbed(f, rgb, cms.DisplayP3()))
	f.Add(withChunks(f, rgb, chunk("cICP", []byte{9, 8, 0, 1}), chunk("gAMA", u32s(50000))))
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := asset.ReadProfile(b)
		if err == nil && p != nil {
			_, _ = cms.NewTransform(p, cms.SRGB(), cms.Options{Intent: cms.Perceptual})
		}
		if out, err := asset.EmbedProfile(b, cms.SRGB()); err == nil {
			if p, err := asset.ReadProfile(out); err == nil && p != nil && !bytes.Equal(p.Bytes(), cms.SRGB().Bytes()) {
				t.Errorf("embedded sRGB read back as %q", p.Description())
			}
		}
	})
}

// --- the pipeline ------------------------------------------------------------

// thumbOf runs one picture through a pipeline and returns a copy of its
// thumbnail's pixels and the pipeline's statistics.
func thumbOf(t *testing.T, name string, data []byte) (*image.RGBA, asset.Stats) {
	t.Helper()
	img, st, _ := thumbIn(t, nil, name, data)
	return img, st
}

// thumbIn is [thumbOf] in a pipeline that makes thumbnails in space, and
// returns the space the thumbnail says it is in.
func thumbIn(t *testing.T, space *cms.Profile, name string, data []byte) (*image.RGBA, asset.Stats, *cms.Profile) {
	t.Helper()
	path := writeFile(t, t.TempDir(), name, data)
	c := newCollector()
	cfg := baseConfig(c)
	cfg.Space = space
	p := asset.NewPipeline(cfg)
	defer p.Close()
	var (
		mu  sync.Mutex
		out *image.RGBA
		in  *cms.Profile
	)
	p.Request(asset.Request{Source: asset.File(path), Size: 64, Priority: asset.Visible,
		OnResult: func(r asset.Result) {
			if r.Image != nil {
				src := r.Image.RGBA()
				cp := image.NewRGBA(src.Rect)
				copy(cp.Pix, src.Pix)
				mu.Lock()
				out, in = cp, r.Image.Space()
				mu.Unlock()
			}
			c.onResult(r)
		}})
	res := c.waitFor(t, 1)
	if err := res[0].Err(); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	mu.Lock()
	defer mu.Unlock()
	return out, p.Stats(), in
}

// decoded is the centre pixel of a picture as the standard library decodes
// it, the numbers before any colour management.
func decoded(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func near(a, b [3]int, tol int) bool {
	for i := range a {
		if d := a[i] - b[i]; d > tol || d < -tol {
			return false
		}
	}
	return true
}

func rgbAt(img *image.RGBA, x, y int) [3]int {
	o := img.PixOffset(x, y)
	return [3]int{int(img.Pix[o]), int(img.Pix[o+1]), int(img.Pix[o+2])}
}

// convert8 converts one 8 bit colour into sRGB exactly, as the expectation.
func convert8(t *testing.T, p *cms.Profile, in ...uint8) [3]int {
	t.Helper()
	return convert8To(t, p, cms.SRGB(), in...)
}

func convert8To(t *testing.T, p, dst *cms.Profile, in ...uint8) [3]int {
	t.Helper()
	tr, err := cms.NewTransform(p, dst, cms.Options{Intent: cms.Perceptual})
	if err != nil {
		t.Fatal(err)
	}
	f := make([]float64, len(in))
	for i, v := range in {
		f[i] = float64(v) / 255
	}
	o := tr.Color(f...)
	return [3]int{int(o[0]*255 + 0.5), int(o[1]*255 + 0.5), int(o[2]*255 + 0.5)}
}

func TestPipelineConvertsAdobeRGB(t *testing.T) {
	jpg := flatJPEG(t, color.RGBA{100, 150, 200, 255})
	r, g, b, _ := decoded(t, jpg).At(24, 16).RGBA()
	raw := [3]int{int(r >> 8), int(g >> 8), int(b >> 8)}
	want := convert8(t, cms.AdobeRGB(), uint8(raw[0]), uint8(raw[1]), uint8(raw[2]))
	if near(raw, want, 4) {
		t.Fatalf("test colour %v is the same in Adobe RGB and sRGB", raw)
	}

	for name, data := range map[string][]byte{
		"icc.jpg": mustEmbed(t, jpg, cms.AdobeRGB()),
		"r03.jpg": withEXIFColour(t, jpg, 0xFFFF, "R03"),
	} {
		img, st := thumbOf(t, name, data)
		if got := rgbAt(img, 24, 16); !near(got, want, 1) {
			t.Errorf("%s: %v, want %v (unconverted %v)", name, got, want, raw)
		}
		if st.Converted != 1 {
			t.Errorf("%s: Converted = %d", name, st.Converted)
		}
	}

	// Untagged, and tagged with a profile that cannot be read, the numbers
	// are shown as they are.
	broken := mustEmbed(t, jpg, cms.AdobeRGB())
	at := bytes.Index(broken, []byte("acsp"))
	copy(broken[at:], "xxxx")
	for name, data := range map[string][]byte{"plain.jpg": jpg, "broken.jpg": broken} {
		img, st := thumbOf(t, name, data)
		if got := rgbAt(img, 24, 16); !near(got, raw, 1) || st.Converted != 0 {
			t.Errorf("%s: %v converted %d times, want %v as it is", name, got, st.Converted, raw)
		}
	}
}

func TestPipelineConvertsWithAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(img.Pix); i += 4 {
		copy(img.Pix[i:], []byte{100, 150, 200, 128})
	}
	got, _ := thumbOf(t, "alpha.png", mustEmbed(t, encodePNG(t, img), cms.AdobeRGB()))
	want := convert8(t, cms.AdobeRGB(), 100, 150, 200)
	o := got.PixOffset(4, 4)
	a := int(got.Pix[o+3])
	if a != 128 {
		t.Fatalf("alpha %d, want 128", a)
	}
	var straight [3]int
	for k := range 3 {
		straight[k] = (int(got.Pix[o+k])*255 + a/2) / a
	}
	if !near(straight, want, 2) {
		t.Errorf("%v unpremultiplied, want %v", straight, want)
	}
}

// A 16 bit picture is converted before it becomes 8 bits: in a linear space
// the deep shadows would otherwise snap to the coarse steps of 8 bit linear
// light.
func TestPipelineConvertsSixteenBitFirst(t *testing.T) {
	img := image.NewRGBA64(image.Rect(0, 0, 16, 16))
	for i := 0; i < len(img.Pix); i += 8 {
		binary.BigEndian.PutUint16(img.Pix[i:], 131) // linear 0.002
		binary.BigEndian.PutUint16(img.Pix[i+2:], 131)
		binary.BigEndian.PutUint16(img.Pix[i+4:], 131)
		binary.BigEndian.PutUint16(img.Pix[i+6:], 0xFFFF)
	}
	linear := withChunks(t, encodePNG(t, img), chunk("gAMA", u32s(100000)))
	got, st := thumbOf(t, "linear16.png", linear)
	// 12.92 * 0.002 * 255 = 6.6; through 8 bits it would be 13.
	if v := rgbAt(got, 8, 8); !near(v, [3]int{7, 7, 7}, 0) || st.Converted != 1 {
		t.Errorf("linear 0.002 is %v converted %d times, want 7", v, st.Converted)
	}
}

// A pipeline told to make its thumbnails in Display P3 converts everything
// into it: a picture that declares nothing from sRGB, an Adobe RGB one from
// Adobe RGB. One in P3 already, and a gray one, it leaves alone.
func TestPipelineMakesThumbnailsInItsSpace(t *testing.T) {
	jpg := flatJPEG(t, color.RGBA{100, 150, 200, 255})
	r, g, b, _ := decoded(t, jpg).At(24, 16).RGBA()
	raw := [3]int{int(r >> 8), int(g >> 8), int(b >> 8)}

	gray := image.NewGray(image.Rect(0, 0, 16, 16))
	for i := range gray.Pix {
		gray.Pix[i] = 90
	}
	var gj bytes.Buffer
	if err := jpeg.Encode(&gj, gray, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	gv, _, _, _ := decoded(t, gj.Bytes()).At(8, 8).RGBA()

	for _, c := range []struct {
		name      string
		data      []byte
		x, y      int
		want      [3]int
		converted uint64
	}{
		{"plain.jpg", jpg, 24, 16, convert8To(t, cms.SRGB(), cms.DisplayP3(), uint8(raw[0]), uint8(raw[1]), uint8(raw[2])), 1},
		{"adobe.jpg", mustEmbed(t, jpg, cms.AdobeRGB()), 24, 16, convert8To(t, cms.AdobeRGB(), cms.DisplayP3(), uint8(raw[0]), uint8(raw[1]), uint8(raw[2])), 1},
		{"p3.jpg", mustEmbed(t, jpg, cms.DisplayP3()), 24, 16, raw, 0},
		{"gray.jpg", gj.Bytes(), 8, 8, [3]int{int(gv >> 8), int(gv >> 8), int(gv >> 8)}, 0},
	} {
		img, st, space := thumbIn(t, cms.DisplayP3(), c.name, c.data)
		if got := rgbAt(img, c.x, c.y); !near(got, c.want, 1) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
		if st.Converted != c.converted {
			t.Errorf("%s: converted %d times, want %d", c.name, st.Converted, c.converted)
		}
		if space != cms.DisplayP3() {
			t.Errorf("%s: the thumbnail says it is %s", c.name, space.Description())
		}
	}
	if near(convert8To(t, cms.SRGB(), cms.DisplayP3(), uint8(raw[0]), uint8(raw[1]), uint8(raw[2])), raw, 4) {
		t.Fatalf("test colour %v is the same in sRGB and P3", raw)
	}

	// A space thumbnails cannot be made in is sRGB.
	_, _, space := thumbIn(t, cms.SGray(), "plain.jpg", jpg)
	if space != cms.SRGB() {
		t.Errorf("a gray space gave %s thumbnails", space.Description())
	}
}

func TestPipelineConvertsGray(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 16, 16))
	for i := range img.Pix {
		img.Pix[i] = 90
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	p := testProfile(t, "gray-xyz.icc")
	v, _, _, _ := decoded(t, buf.Bytes()).At(8, 8).RGBA()
	want := convert8(t, p, uint8(v>>8))
	got, st := thumbOf(t, "gray.jpg", mustEmbed(t, buf.Bytes(), p))
	if g := rgbAt(got, 8, 8); !near(g, want, 1) || st.Converted != 1 {
		t.Errorf("gray %d is %v converted %d times, want %v", v>>8, g, st.Converted, want)
	}
}

// A CMYK JPEG is converted through its profile, before scaling, and agrees
// with LittleCMS: cmyk.py printed these numbers.
func TestCMYKJPEGWithProfile(t *testing.T) {
	plain, err := os.ReadFile(filepath.Join("testdata", "cmyk.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded(t, plain).(*image.CMYK); !ok {
		t.Fatal("cmyk.jpg does not decode as CMYK")
	}
	got, st := thumbOf(t, "cmyk.jpg", mustEmbed(t, plain, testProfile(t, "cmyk-lut16.icc")))
	if st.Converted != 1 {
		t.Errorf("Converted = %d", st.Converted)
	}
	for i, want := range [][3]int{{255, 255, 255}, {72, 255, 255}, {191, 191, 191}, {221, 180, 127}} {
		if g := rgbAt(got, 8*i+4, 8); !near(g, want, 2) {
			t.Errorf("patch %d: %v, LittleCMS %v", i, g, want)
		}
	}
}
