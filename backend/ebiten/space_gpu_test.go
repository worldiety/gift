//go:build giftgpu

package ebiten

import (
	"image/color"
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/cms"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// p3Of is what an sRGB colour is in Display P3, by package cms.
func p3Of(t *testing.T, c color.RGBA) color.RGBA {
	t.Helper()
	tr, err := cms.NewTransform(cms.SRGB(), cms.DisplayP3(), cms.Options{})
	if err != nil {
		t.Fatal(err)
	}
	px := []byte{c.R, c.G, c.B, c.A}
	if err := tr.Pixels(px, cms.RGBAPremul8, px, cms.RGBAPremul8); err != nil {
		t.Fatal(err)
	}
	return color.RGBA{px[0], px[1], px[2], px[3]}
}

// In Display P3 the frame holds P3 numbers: a fill of sRGB red is the red P3
// has for it, a gray is the gray it was, a picture uploaded as sRGB is
// converted and one made in P3 is drawn as it is.
func TestAP3FrameHoldsP3Numbers(t *testing.T) {
	var converted uint64
	dst := drawImages(t, 64, 64, color.RGBA{0, 0, 0, 255}, func(r *Renderer, l *render.List) {
		if err := r.SetWorkingSpace(cms.DisplayP3()); err != nil {
			t.Fatal(err)
		}
		l.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(0, 0, 32, 16), Color: render.RGB(255, 0, 0)})
		l.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(32, 0, 64, 16), Color: render.RGB(90, 90, 90)})

		srgb, ok := r.Textures().Acquire(checker(16, green, green, green, green))
		if !ok {
			t.Fatal("the sRGB upload was refused")
		}
		wide := checker(16, green, green, green, green)
		wide.Space = cms.DisplayP3()
		inP3, ok := r.Textures().Acquire(wide)
		if !ok {
			t.Fatal("the P3 upload was refused")
		}
		converted = r.Textures().Stats().Converted
		for i, h := range []render.ImageHandle{srgb, inP3} {
			id, _ := r.Textures().Resolve(h)
			x := float32(32 * i)
			l.Add(render.Op{Kind: render.OpImage, Bounds: geom.Rc(x, 32, x+32, 64),
				Color: render.RGB(255, 255, 255), Image: id})
		}
	})
	if converted != 1 {
		t.Errorf("%d pictures converted, want the sRGB one", converted)
	}
	for _, c := range []struct {
		name string
		x, y int
		want color.RGBA
	}{
		{"sRGB red", 8, 8, p3Of(t, red)},
		{"gray", 40, 8, color.RGBA{90, 90, 90, 255}},
		{"sRGB picture", 8, 48, p3Of(t, green)},
		{"P3 picture", 40, 48, green},
	} {
		if got := dst.At(c.x, c.y).(color.RGBA); !near(got, c.want, 1) {
			t.Errorf("%s = %v, want %v", c.name, got, c.want)
		}
	}
	if got := dst.At(8, 8).(color.RGBA); got.G < 40 {
		t.Errorf("sRGB red came out as %v, the red of the panel", got)
	}
}

// A video picture is converted into P3 by the wide shaders, on both paths.
func TestAP3FrameConvertsVideo(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetWorkingSpace(cms.DisplayP3()); err != nil {
		t.Fatal(err)
	}
	// Full range BT.709 red: Y = 0.2126, U and V from the matrix.
	const w, h = 16, 16
	f := render.VideoFrame{Width: w, Height: h, Stride: w, Layout: render.VideoNV12, Matrix: render.BT709, FullRange: true}
	pix := make([]byte, w*(h+h/2))
	y, u, v := 0.2126, -0.2126/1.8556+0.5, 0.5+0.5
	for i := range w * h {
		pix[i] = byte(y*255 + 0.5)
	}
	for i := w * h; i < len(pix); i += 2 {
		pix[i], pix[i+1] = byte(u*255+0.5), byte(min(1, v)*255+0.5)
	}
	f.Pix = pix
	hd, ok := r.Videos().Present(render.ImageHandle{}, f)
	if !ok {
		t.Fatal("not presented")
	}
	id, _ := r.Videos().Resolve(hd)
	want := p3Of(t, red)
	for _, c := range []struct {
		name   string
		bounds geom.Rect
	}{
		{"direct", geom.Rc(0, 0, w, h)},
		{"through the texture", geom.Rc(0, 0, 2*w, 2*h)},
	} {
		dst := eb.NewImage(2*w, 2*h)
		dst.Fill(color.Black)
		var l render.List
		l.Reset()
		l.Add(render.Op{Kind: render.OpVideo, Bounds: c.bounds, Color: render.RGB(255, 255, 255), Image: id})
		r.SetTarget(dst)
		r.BeginFrame(geom.Sz(2*w, 2*h))
		r.Submit(&l)
		r.EndFrame()
		if got := dst.At(4, 4).(color.RGBA); !near(got, want, 3) {
			t.Errorf("%s: %v, want %v", c.name, got, want)
		}
	}
}
