//go:build giftgpu

package ebiten

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/cms"
)

// The final copy converts into the display as package cms does: a matrix
// profile within a step of the exact conversion, a profile of tables within
// a few of package cms's own tables. Translucent pixels keep their alpha,
// and the translation of the final screen is applied.
//
// On a Mac the display profiles of macOS are tried as well, among them the
// matrices with curves of 1024 entries that macOS makes from what a display
// says of itself, where a table was four steps off.
func TestTheFinalCopyConvertsIntoTheDisplay(t *testing.T) {
	const w, h = 64, 64
	src := make([]byte, w*h*4)
	for y := range h {
		for x := range w {
			i := 4 * (y*w + x)
			src[i], src[i+1], src[i+2], src[i+3] = byte(x*4), byte(y*4), byte((x*y)%256), 255
			if y >= h-4 {
				// Half transparent, premultiplied.
				src[i], src[i+1], src[i+2], src[i+3] = byte(x*2), 60, 20, 128
			}
		}
	}
	offscreen := eb.NewImage(w, h)
	offscreen.WritePixels(src)

	profiles := map[string][]byte{}
	for _, name := range []string{"displayp3.icc", "adobergb.icc", "prophoto.icc", "rgb-mab.icc"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "cms", "testdata", "profiles", name))
		if err != nil {
			t.Fatal(err)
		}
		profiles[name] = data
	}
	if es, err := os.ReadDir("/Library/ColorSync/Profiles/Displays"); err == nil {
		for _, e := range es {
			if data, err := os.ReadFile(filepath.Join("/Library/ColorSync/Profiles/Displays", e.Name())); err == nil {
				profiles["macOS "+e.Name()] = data
			}
		}
	}

	for name, data := range profiles {
		t.Run(name, func(t *testing.T) {
			p, err := cms.ParseProfile(data)
			if err != nil {
				t.Fatal(err)
			}
			tr, err := cms.NewTransform(cms.SRGB(), p, cms.Options{Intent: cms.Perceptual})
			if err != nil {
				t.Fatal(err)
			}
			d := &displayPass{read: func() ([]byte, string, error) { return data, name, nil }}
			d.poll(time.Time{})
			d.upload()
			if d.cur == nil {
				t.Skip("the display is sRGB")
			}
			// Transparent, so that the alpha of the translucent rows is
			// what was drawn.
			screen := eb.NewImage(w+10, h+10)
			var geoM eb.GeoM
			geoM.Translate(10, 10)
			if !d.draw(screen, offscreen, geoM) {
				t.Fatal("nothing converted")
			}
			got := make([]byte, (w+10)*(h+10)*4)
			screen.ReadPixels(got)

			var want []byte
			tolerance := 1.5
			if d.cur.shaper {
				// Exact, in float64.
				want = make([]byte, len(src))
				for i := 0; i < len(src); i += 4 {
					a := float64(src[i+3]) / 255
					c := tr.Color(float64(src[i])/255/a, float64(src[i+1])/255/a, float64(src[i+2])/255/a)
					for k := range 3 {
						want[i+k] = byte(math.Round(min(max(c[k], 0), 1) * a * 255))
					}
					want[i+3] = src[i+3]
				}
			} else {
				want = make([]byte, len(src))
				if err := tr.Pixels(want, cms.RGBAPremul8, src, cms.RGBAPremul8); err != nil {
					t.Fatal(err)
				}
				tolerance = 3
			}
			worst := 0
			for y := range h {
				for x := range w {
					i, j := 4*(y*w+x), 4*((y+10)*(w+10)+x+10)
					for k := range 4 {
						worst = max(worst, abs(int(got[j+k])-int(want[i+k])))
					}
				}
			}
			if float64(worst) > tolerance {
				t.Errorf("up to %d steps off (matrix %v)", worst, d.cur.shaper)
			}
			t.Logf("matrix %v: up to %d steps off", d.cur.shaper, worst)
			if got[0] != 0 || got[3] != 0 {
				t.Errorf("the margin left by the translation is %v", got[:4])
			}
		})
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
