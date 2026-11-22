package asset

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/worldiety/gift/cms"
)

func TestTransformsAreSharedAndBounded(t *testing.T) {
	var ts transforms
	if c, err := ts.get(cms.SRGB()); c != nil || err != nil || ts.len() != 0 {
		t.Fatalf("sRGB needs no conversion and no entry: %v, %v", c, err)
	}
	// The same profile parsed twice, as two pictures carry it, from many
	// goroutines at once: one conversion.
	a, _ := cms.ParseProfile(cms.AdobeRGB().Bytes())
	b, _ := cms.ParseProfile(cms.AdobeRGB().Bytes())
	var (
		wg  sync.WaitGroup
		got [16]*conversion
	)
	for i := range got {
		wg.Go(func() {
			p := a
			if i%2 == 1 {
				p = b
			}
			c, err := ts.get(p)
			if err != nil {
				t.Error(err)
			}
			got[i] = c
		})
	}
	wg.Wait()
	for _, c := range got {
		if c == nil || c != got[0] {
			t.Fatal("the same profile gave different conversions")
		}
	}
	if ts.len() != 1 {
		t.Fatalf("%d entries for one profile", ts.len())
	}

	// More profiles than it keeps: the oldest go, the one just used stays.
	for i := range maxTransforms + 4 {
		p, err := cms.NewRGBProfile(fmt.Sprint("gamma ", i), cms.WhiteD65,
			[3]cms.Chromaticity{{X: 0.64, Y: 0.33}, {X: 0.30, Y: 0.60}, {X: 0.15, Y: 0.06}},
			cms.Gamma(1.5+float64(i)/10))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ts.get(p); err != nil {
			t.Fatal(err)
		}
		if _, err := ts.get(a); err != nil {
			t.Fatal(err)
		}
	}
	if ts.len() != maxTransforms {
		t.Errorf("%d entries, want at most %d", ts.len(), maxTransforms)
	}
	if c, _ := ts.get(a); c != got[0] {
		t.Error("the profile used all along was evicted")
	}
}

func TestGrayProfileThatChangesNothing(t *testing.T) {
	// A gray profile with the curve of sRGB is gray in sRGB, whatever its
	// bytes: nothing to convert.
	p, err := cms.NewGrayProfile("my gray", cms.WhiteD65, cms.SRGBCurve())
	if err != nil {
		t.Fatal(err)
	}
	if c, err := newConversion(p, nil); c != nil || err != nil {
		t.Errorf("sRGB gray converted: %v, %v", c, err)
	}
}

func TestProfileName(t *testing.T) {
	long := "a" + strings.Repeat("0123456789", 9)
	for in, want := range map[string]string{
		"Adobe RGB (1998)": "Adobe RGB (1998)",
		"  Display   P3  ": "Display P3",
		"":                 "ICC Profile",
		"\x01\x02":         "ICC Profile",
		"Grüne Wiese":      "Gr\xfcne Wiese",
		"天空":               "ICC Profile",
		long:               long[:79],
	} {
		if got := profileName(in); got != want {
			t.Errorf("profileName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The cost of converting: a thumbnail after scaling, and the whole picture
// for the kinds converted first, at the size of a 24 megapixel camera.
func BenchmarkConversion(b *testing.B) {
	cmyk, err := cms.ParseProfile(mustRead(b, "cmyk-lut16.icc"))
	if err != nil {
		b.Fatal(err)
	}
	for _, c := range []struct {
		name string
		p    *cms.Profile
		img  image.Image
	}{
		{"thumbnail-512-AdobeRGB", cms.AdobeRGB(), photo(512, 341)},
		{"thumbnail-512-CMYK-table", cmyk, photo(512, 341)},
		{"full-24MP-CMYK", cmyk, image.NewCMYK(image.Rect(0, 0, 6000, 4000))},
		{"full-24MP-16bit-AdobeRGB", cms.AdobeRGB(), image.NewRGBA64(image.Rect(0, 0, 6000, 4000))},
	} {
		b.Run(c.name, func(b *testing.B) {
			conv, err := newConversion(c.p, nil)
			if err != nil || conv == nil {
				b.Fatal(err)
			}
			for b.Loop() {
				if m, ok := c.img.(*image.RGBA); ok && c.p.Space() != cms.SpaceCMYK {
					conv.thumbnail(m)
				} else if m, ok := c.img.(*image.RGBA); ok {
					// A CMYK thumbnail as if it had been scaled first.
					_ = conv.t.Pixels(m.Pix, cms.RGBAPremul8, m.Pix, cms.CMYK8)
				} else if _, ok := conv.full(c.img); !ok {
					b.Fatal("not converted")
				}
			}
		})
	}
}

// photo is an opaque picture with every colour in it, as a thumbnail is.
func photo(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7919 >> 3)
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	return img
}

func mustRead(tb testing.TB, name string) []byte {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("..", "cms", "testdata", "profiles", name))
	if err != nil {
		tb.Fatal(err)
	}
	return data
}

// The sRGB profiles in the wild differ from the one built in by less than
// half an 8 bit step, which rounds away: converting with them would only
// cost time. This one is sRGB with a curve a third of a step off in the
// middle tones, too far to be the identity, not far enough to change an 8
// bit value; macOS's sRGB Profile.icc is such a one. A profile that differs
// by more is converted.
func TestNearlySRGBIsNotConverted(t *testing.T) {
	table := make([]uint16, 4096)
	for i := range table {
		v := float64(i) / 4095
		v += 0.3 / 255 * math.Sin(math.Pi*v)
		if v <= 0.04045 {
			v /= 12.92
		} else {
			v = math.Pow((v+0.055)/1.055, 2.4)
		}
		table[i] = uint16(min(1, v)*65535 + 0.5)
	}
	near, err := cms.NewRGBProfile("sRGB IEC61966-2.1", cms.WhiteD65,
		[3]cms.Chromaticity{{X: 0.64, Y: 0.33}, {X: 0.30, Y: 0.60}, {X: 0.15, Y: 0.06}},
		cms.Table(table))
	if err != nil {
		t.Fatal(err)
	}
	if tr, _ := cms.NewTransform(near, cms.SRGB(), thumbnailOptions); tr.Identity() {
		t.Fatal("the test profile is the identity already; it tests nothing")
	}
	if c, err := newConversion(near, nil); c != nil || err != nil {
		t.Errorf("nearly sRGB is converted: %v", err)
	}
	gamma, err := cms.NewRGBProfile("gamma 2.2", cms.WhiteD65,
		[3]cms.Chromaticity{{X: 0.64, Y: 0.33}, {X: 0.30, Y: 0.60}, {X: 0.15, Y: 0.06}}, cms.Gamma(2.2))
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := newConversion(gamma, nil); c == nil {
		t.Error("gamma 2.2 is not converted")
	}
}
