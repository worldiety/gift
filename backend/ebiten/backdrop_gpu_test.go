//go:build giftgpu

package ebiten

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// drawPaneScene draws a picture and one pane over it, the second time with
// the picture made translucent by one pixel so that the pane is live.
func drawPaneScene(t *testing.T, live bool) *eb.Image {
	t.Helper()
	const w, h = 160, 120
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	pix := make([]byte, w*h*4)
	for y := range h {
		for x := range w {
			i := (y*w + x) * 4
			// Smooth, so that the blur of a region and the blur of the whole
			// picture agree away from the rim.
			pix[i], pix[i+1], pix[i+2], pix[i+3] = byte(x), byte(60+y), 180, 255
		}
	}
	if live {
		pix[3] = 254
	}
	hd, ok := r.Textures().Acquire(render.Pixels{Pix: pix, W: w, H: h, Stride: w * 4})
	if !ok {
		t.Fatal("upload refused")
	}
	id, _ := r.Textures().Resolve(hd)

	dst := eb.NewImage(w, h)
	var l render.List
	l.Reset()
	l.Add(render.Op{Kind: render.OpImage, Image: id, Bounds: geom.Rc(0, 0, w, h), Color: render.Color{R: 1, G: 1, B: 1, A: 1}})
	l.Add(render.Op{Kind: render.OpMaterial, Bounds: geom.Rc(30, 30, 130, 90), CornerRadius: 12,
		Material: l.AddMaterial(render.NewGlass().Quality(render.Full).Grain(0).Material())})
	r.SetTarget(dst)
	r.BeginFrame(geom.Sz(w, h))
	r.Submit(&l)
	r.EndFrame()
	st := r.Stats()
	if live && st.GlassFullOps != 1 || !live && st.GlassStaticOps != 1 {
		t.Fatalf("live=%v: %+v", live, st)
	}
	return dst
}

// TestAStaticPaneLooksLikeALivePane: in the middle of a pane over a smooth
// picture, the picture blurred once and the region blurred every frame are
// the same colour.
func TestAStaticPaneLooksLikeALivePane(t *testing.T) {
	static, live := drawPaneScene(t, false), drawPaneScene(t, true)
	worst := 0
	for y := 50; y < 70; y++ {
		for x := 55; x < 105; x++ {
			a, b := static.At(x, y).(color.RGBA), live.At(x, y).(color.RGBA)
			for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B)} {
				worst = max(worst, d, -d)
			}
		}
	}
	t.Logf("static and live differ by up to %d in the middle of the pane", worst)
	if dir := os.Getenv("GIFT_DUMP"); dir != "" {
		for name, img := range map[string]*eb.Image{"static": static, "live": live} {
			f, err := os.Create(dir + "/pane-" + name + ".png")
			if err != nil {
				t.Fatal(err)
			}
			b := img.Bounds()
			rgba := image.NewRGBA(b)
			img.ReadPixels(rgba.Pix)
			_ = png.Encode(f, rgba)
			_ = f.Close()
		}
	}
	if worst > 6 {
		t.Errorf("in the middle of the pane, static and live differ by up to %d", worst)
	}
	// And the rim is drawn: the highlight makes it brighter than the middle.
	if c, m := static.At(31, 60).(color.RGBA), static.At(80, 60).(color.RGBA); int(c.R)+int(c.G)+int(c.B) <= int(m.R)+int(m.G)+int(m.B)-60 {
		t.Errorf("the static pane has no rim: edge %v, middle %v", c, m)
	}
}
