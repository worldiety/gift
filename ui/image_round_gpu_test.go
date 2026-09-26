//go:build giftgpu

package ui_test

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// The pixel half of image_round_test.go: rounded pictures from real files,
// through the real pipeline, on a real framebuffer.

// TestSelectedGalleryTileIsRoundOnScreen is the photo booth's tile. One square
// picture fills one 100 by 100 tile with a corner radius of 10 and a selection
// border of width 3, and along the diagonal of the top left corner, whose arcs
// are centred on (10, 10):
//
//	(0, 0)  14.1 from the centre, beyond the outer arc  -> the background
//	(3, 3)   9.2, between the arcs                      -> the border
//	(6, 6)   5.7, inside the concentric inner arc       -> the picture
//
// (3, 3) is the discriminating pixel. It lies inside a *square* inner corner,
// which is what the border's inner edge looked like to the reporter, and
// outside the concentric arc of radius 7 the border actually has.
func TestSelectedGalleryTileIsRoundOnScreen(t *testing.T) {
	dir := t.TempDir()
	pic := color.RGBA{200, 40, 40, 255}
	p := filepath.Join(dir, "square.png")
	if err := os.WriteFile(p, pngOf(t, 80, 80, pic), 0o600); err != nil {
		t.Fatal(err)
	}
	src := asset.File(p)
	m := src.Metadata()
	m.Width, m.Height = 80, 80

	del := newDeliverer()
	pipe := asset.NewPipeline(asset.Config{Deliver: del.deliver, Sizes: []int{128}, Workers: 1})
	defer func() { pipe.Close(); del.drain() }()
	ui.ResetImageService()
	ui.SetImagePipeline(pipe)
	defer ui.ResetImageService()

	g := ui.NewGallery(asset.NewCollection([]asset.Metadata{m}))
	g.SetSources(func(asset.ID) asset.Source { return src })
	g.Selection().Only(m.ID)
	bg := color.RGBA{240, 240, 240, 255}
	border := color.RGBA{20, 60, 220, 255}
	h := gifttest.New(t, gifttest.Options{
		View: ui.ImageGallery(g).
			Layout(ui.Masonry().MinColumnWidth(100).Gap(0)).
			Tile(ui.TileStyle{CornerRadius: 10, Selected: ui.Border{Width: 3, Color: ui.RGB(border.R, border.G, border.B)}}).
			Background(ui.RGB(bg.R, bg.G, bg.B)).
			Frame(100, 120).Key("gallery"),
		Size: geom.Sz(100, 120),
	})
	settlePictures(t, h, g, del)
	img := h.Image()

	for _, c := range []struct {
		x, y int
		want color.RGBA
		what string
	}{
		{0, 0, bg, "beyond the outer arc"},
		{3, 3, border, "between the arcs, inside a square inner corner"},
		{6, 6, pic, "inside the inner arc"},
		{50, 50, pic, "the middle"},
		{99, 99, bg, "the opposite corner"},
		{50, 1, border, "the straight edge"},
	} {
		if got := colorAt(img, c.x, c.y); !nearRGBA(got, c.want, 8) {
			t.Errorf("%s at (%d, %d) = %v, want %v", c.what, c.x, c.y, got, c.want)
		}
	}
	// Nothing of the picture outside the rounded shape, anywhere in the
	// corner square: the reporter's "photo pixels show outside it at the
	// corners".
	for y := range 10 {
		for x := range 10 {
			if math.Hypot(9.5-float64(x), 9.5-float64(y)) < 11 {
				continue
			}
			if got := colorAt(img, x, y); !nearRGBA(got, bg, 2) {
				t.Errorf("pixel (%d, %d) outside the tile's rounded corner = %v, want the background", x, y, got)
			}
		}
	}
}

// TestImageViewCornerRadiusClipRoundsThePictureOnScreen is ui.Image's half:
// CornerRadius(16) with Clip(true) cuts the corners of the picture itself,
// and the background of the enclosing stack shows through them.
func TestImageViewCornerRadiusClipRoundsThePictureOnScreen(t *testing.T) {
	dir := t.TempDir()
	pic := color.RGBA{30, 160, 90, 255}
	p := filepath.Join(dir, "wide.png")
	if err := os.WriteFile(p, pngOf(t, 90, 60, pic), 0o600); err != nil {
		t.Fatal(err)
	}
	del := newDeliverer()
	pipe := asset.NewPipeline(asset.Config{Deliver: del.deliver, Sizes: []int{64}, Workers: 1})
	defer func() { pipe.Close(); del.drain() }()
	ui.ResetImageService()
	ui.SetImagePipeline(pipe)
	defer ui.ResetImageService()

	bg := color.RGBA{0, 0, 0, 255}
	h := gifttest.New(t, gifttest.Options{
		View: ui.VStack(
			ui.Image(asset.File(p)).Size(64).Frame(64, 64).Fit(ui.FitCover).
				CornerRadius(16).Clip(true).Placeholder(ui.RGB(255, 255, 255)),
		).Background(ui.RGB(0, 0, 0)).Frame(64, 64),
		Size: geom.Sz(64, 64),
	})
	del.waitFor(t, 1)
	del.drain()
	h.Frame()
	h.Image()
	img := h.Image()

	if got := colorAt(img, 32, 32); !nearRGBA(got, pic, 6) {
		t.Fatalf("the middle = %v, want the picture %v", got, pic)
	}
	for _, c := range [][2]int{{0, 0}, {63, 0}, {0, 63}, {63, 63}, {2, 2}} {
		if got := colorAt(img, c[0], c[1]); !nearRGBA(got, bg, 2) {
			t.Errorf("corner pixel %v = %v, want the background: the picture is not rounded", c, got)
		}
	}
	for _, c := range [][2]int{{0, 32}, {32, 0}, {63, 32}, {32, 63}} {
		if got := colorAt(img, c[0], c[1]); !nearRGBA(got, pic, 6) {
			t.Errorf("edge pixel %v = %v, want the picture: a cover crop fills the view", c, got)
		}
	}
}
