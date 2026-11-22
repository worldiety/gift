package raw

import (
	"bytes"
	"errors"
	"image"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/worldiety/gift/cms"
)

func openFixture(t testing.TB, name string) (*File, []byte) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Open(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return f, b
}

// What the camera was set to for each photograph, as its menus say, and
// the white balance it recorded, in its own units of 1024 for green.
func TestTheSettingsOfTheA7IIIAreRead(t *testing.T) {
	for _, c := range []struct {
		file            string
		style           string
		con, sat, sharp int
		wb              WhiteBalance
		red, blue       float64
		compression     Compression
		largePreview    bool
	}{
		{"a7iii-bw.arw", "BW", -2, 0, -3, WhiteBalanceAuto, 2652, 1496, SonyCompressed, false},
		{"a7iii-portrait.arw", "Portrait", 0, 0, 0, WhiteBalanceAuto, 2640, 1512, SonyCompressed, false},
		{"a7iii-portrait-shade.arw", "Portrait", 0, 0, 0, WhiteBalanceShade, 2904, 1344, SonyCompressed, false},
		{"a7iii-landscape-iso16000.arw", "Landscape", 0, 0, 0, WhiteBalanceAuto, 2708, 1456, SonyCompressed, false},
		{"a7iii-preview.arw", "Portrait", 0, 0, 0, WhiteBalanceShade, 2904, 1344, SonyCompressed, true},
		{"a7iii-uncompressed.arw", "Portrait", 0, 0, 0, WhiteBalanceShade, 2904, 1344, Uncompressed, false},
	} {
		t.Run(c.file, func(t *testing.T) {
			f, _ := openFixture(t, c.file)
			if f.Format != ARW || f.Make != "SONY" || f.Model != "ILCE-7M3" || f.Orientation != 1 {
				t.Errorf("%v %q %q, orientation %d", f.Format, f.Make, f.Model, f.Orientation)
			}
			s := f.Sony
			if s == nil {
				t.Fatal("no Sony settings")
			}
			if s.ModelID != 363 || s.CreativeStyle != c.style || s.Contrast != c.con || s.Saturation != c.sat || s.Sharpness != c.sharp {
				t.Errorf("model %d, style %q %d/%d/%d", s.ModelID, s.CreativeStyle, s.Contrast, s.Saturation, s.Sharpness)
			}
			if s.DRO != DROAuto || s.WhiteBalance != c.wb || s.ColorTemperature != 0 || s.WhiteBalanceShift != [2]int{} {
				t.Errorf("DRO %v, white balance %v, %d K, shift %v", s.DRO, s.WhiteBalance, s.ColorTemperature, s.WhiteBalanceShift)
			}
			if want := [3]float64{c.red / 1024, 1, c.blue / 1024}; f.WhiteBalance != want {
				t.Errorf("white balance %v, want %v", f.WhiteBalance, want)
			}

			m := f.Mosaic
			want := Mosaic{
				Width: 6048, Height: 4024, Crop: image.Rect(12, 12, 6012, 4012),
				BitsPerSample: 14, Compression: c.compression, SamplesPerPixel: 1,
				CFA: "RGGB", Black: [4]float64{512, 512, 512, 512}, White: 16383,
			}
			if m != want {
				t.Errorf("mosaic\n%+v, want\n%+v", m, want)
			}
			if len(f.segs) != 1 || f.segs[0].n != map[Compression]int64{SonyCompressed: 6048 * 4024, Uncompressed: 6048 * 4024 * 2}[c.compression] {
				t.Errorf("raw data in %v", f.segs)
			}

			// The large preview, where the fixture keeps it, and the
			// thumbnail with its black bars.
			var sizes []image.Point
			for _, p := range f.Previews {
				sizes = append(sizes, image.Pt(p.Width, p.Height))
				if !p.JPEG || p.Space != cms.SRGB() {
					t.Errorf("a preview of %dx%d: JPEG %v, %v", p.Width, p.Height, p.JPEG, p.Space)
				}
			}
			wantSizes := []image.Point{{160, 120}}
			if c.largePreview {
				wantSizes = []image.Point{{1616, 1080}, {160, 120}}
			}
			if len(sizes) != len(wantSizes) || sizes[0] != wantSizes[0] {
				t.Errorf("previews %v, want %v", sizes, wantSizes)
			}
		})
	}
}

// The SR2 block is encrypted; decrypted, it has the presets of the white
// balance. The shade preset is the white balance of the photographs taken
// with it.
func TestTheSR2BlockIsDecrypted(t *testing.T) {
	f, _ := openFixture(t, "a7iii-portrait-shade.arw")
	names := map[string][3]float64{}
	for _, p := range f.Sony.Presets {
		names[p.Name] = p.Factors
	}
	if len(f.Sony.Presets) != len(presetNames) {
		t.Fatalf("%d presets", len(f.Sony.Presets))
	}
	if got := names["Daylight"]; got != [3]float64{2448.0 / 1024, 1, 1620.0 / 1024} {
		t.Errorf("daylight %v", got)
	}
	if names["Shade"] != f.WhiteBalance {
		t.Errorf("shade %v, the photograph %v", names["Shade"], f.WhiteBalance)
	}
	// Warmer presets for colder light: more red, less blue.
	if !(names["Tungsten"][0] < names["Daylight"][0] && names["Daylight"][0] < names["Shade"][0]) {
		t.Errorf("tungsten, daylight, shade: %v %v %v", names["Tungsten"], names["Daylight"], names["Shade"])
	}

	// The cipher is an XOR with a stream: twice is nothing.
	b := []byte("sixteen bytes of plain text, and more of it")
	c := bytes.Clone(b)
	decryptSR2(c, 0x44332211)
	if bytes.Equal(b, c) {
		t.Fatal("nothing encrypted")
	}
	decryptSR2(c, 0x44332211)
	if !bytes.Equal(b, c) {
		t.Errorf("%q", c)
	}
}

func TestTheFormatIsSniffed(t *testing.T) {
	_, b := openFixture(t, "a7iii-bw.arw")
	if f := Sniff(b[:4096]); f != ARW {
		t.Errorf("the head of an ARW is %v", f)
	}
	if f := Sniff(buildDNG(t)); f != DNG {
		t.Errorf("a DNG is %v", f)
	}
	// A TIFF of another make is not an ARW.
	other := bytes.Replace(bytes.Clone(b[:4096]), []byte("SONY"), []byte("CANO"), 1)
	for _, h := range [][]byte{nil, b[:8], other, []byte("\xff\xd8\xff\xe0 a JPEG")} {
		if f := Sniff(h); f != FormatUnknown {
			t.Errorf("%q is %v", h[:min(len(h), 8)], f)
		}
	}
	if !(Decoder{}).Sniff(b) || !(Decoder{Format: ARW}).Sniff(b) || (Decoder{Format: DNG}).Sniff(b) {
		t.Error("the decoders sniff the formats of others")
	}
}

// The decoder shows a raw file as its largest preview, in the colours the
// camera declares.
func TestTheDecoderDecodesThePreview(t *testing.T) {
	_, b := openFixture(t, "a7iii-preview.arw")
	d := Decoder{Format: ARW}
	cfg, err := d.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width != 1616 || cfg.Height != 1080 {
		t.Fatalf("%+v, %v", cfg, err)
	}
	img, err := d.Decode(bytes.NewBuffer(b))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 1616, 1080) {
		t.Errorf("%v", img.Bounds())
	}
	// A photograph, not a gray or black field: its rows differ.
	var sum [2]uint32
	for x := range 1616 {
		r0, _, _, _ := img.At(x, 300).RGBA()
		r1, _, _, _ := img.At(x, 800).RGBA()
		sum[0] += r0 >> 8
		sum[1] += r1 >> 8
	}
	if sum[0] == sum[1] {
		t.Errorf("rows 300 and 800 alike: %v", sum)
	}
	if p, err := d.ReadProfile(b); err != nil || p != cms.SRGB() {
		t.Errorf("profile %v, %v", p, err)
	}

	// Without the large preview, the thumbnail is the picture.
	_, small := openFixture(t, "a7iii-bw.arw")
	if cfg, err := d.DecodeConfig(bytes.NewReader(small)); err != nil || cfg.Width != 160 {
		t.Errorf("%+v, %v", cfg, err)
	}
	if _, err := (Decoder{Format: DNG}).Decode(bytes.NewReader(b)); !errors.Is(err, ErrFormat) {
		t.Errorf("a DNG decoder decoded an ARW: %v", err)
	}
	// A reader that is neither a Buffer nor a Reader is read whole.
	if cfg, err := d.DecodeConfig(io.MultiReader(bytes.NewReader(b))); err != nil || cfg.Width != 1616 {
		t.Errorf("%+v, %v", cfg, err)
	}
}

// A damaged file is refused or read in part, never a panic, and a file cut
// anywhere still opens as far as its directories reach.
func TestDamagedFilesAreRead(t *testing.T) {
	_, b := openFixture(t, "a7iii-portrait.arw")
	rnd := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		c := bytes.Clone(b)
		for range 1 + rnd.IntN(8) {
			c[rnd.IntN(len(c))] = byte(rnd.Uint32())
		}
		c = c[:rnd.IntN(len(c)+1)]
		if f, err := Open(bytes.NewReader(c), int64(len(c))); err == nil {
			for _, p := range f.Previews {
				_, _ = f.DecodePreview(p)
			}
		}
		_ = Sniff(c)
	}
}

func FuzzOpen(f *testing.F) {
	for _, name := range []string{"a7iii-bw.arw", "a7iii-uncompressed.arw"} {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b[:8192])
	}
	f.Add(buildDNG(f))
	f.Fuzz(func(t *testing.T, b []byte) {
		if fl, err := Open(bytes.NewReader(b), int64(len(b))); err == nil {
			for _, p := range fl.Previews {
				_, _ = fl.DecodePreview(p)
			}
		}
	})
}
