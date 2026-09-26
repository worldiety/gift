package ui_test

import (
	"testing"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// Rounded pictures, as the display list sees them. The pixels are in
// image_gpu_test.go and in backend/ebiten's roundimage_gpu_test.go; what is
// asserted here is that ui asks for the right shape: the picture, its
// placeholder and a border of the same tile are one rectangle with one radius.

// TestGalleryTileKeepsItsShapeWhenThePictureArrives is the bug report of a
// photo booth, in display list terms. A tile with TileStyle.CornerRadius used
// to be a rounded placeholder that turned into a square photograph, clipped by
// an extra clip rectangle to an oversized one, with the selection border's
// rounded corners drawn over the photograph's square ones.
//
// Now the tile's picture is emitted with the tile's own rectangle and radius
// and a cover crop instead of a clip, so:
//
//   - the placeholder before and the picture after have the same bounds and
//     the same radius;
//   - the selection border is the same rectangle and radius as the picture
//     under it, so its outer edge is the picture's edge;
//   - no clip is pushed for a tile — the picture has the clip of the gallery,
//     like the border that follows it.
func TestGalleryTileKeepsItsShapeWhenThePictureArrives(t *testing.T) {
	const radius = 7
	_, items, srcs := writePictures(t, 12)
	del := newDeliverer()
	pipe := asset.NewPipeline(asset.Config{Deliver: del.deliver, Sizes: []int{64, 128}, Workers: 2})
	t.Cleanup(func() { pipe.Close(); del.drain() })
	ui.ResetImageService()
	ui.SetImagePipeline(pipe)
	t.Cleanup(ui.ResetImageService)

	g := ui.NewGallery(asset.NewCollection(items))
	g.SetSources(func(id asset.ID) asset.Source { return srcs[id] })
	sel := items[1].ID
	g.Selection().Only(sel)
	imgs := newFakeImages()
	h := gifttest.New(t, gifttest.Options{
		View: ui.ImageGallery(g).
			Layout(ui.Masonry().MinColumnWidth(60).Gap(4)).
			Tile(ui.TileStyle{
				CornerRadius: radius,
				Selected:     ui.Border{Width: 3, Color: ui.RGB(10, 20, 30)},
			}).
			Frame(240, 200).Key("gallery"),
		Size: geom.Sz(240, 200),
	})
	h.App().SetImages(imgs)

	// The selected tile before its picture: a rounded placeholder and the
	// border on top of it.
	imgs.beginFrame()
	h.Frame()
	placeholder, border := selectedTile(t, h.Ops())
	if placeholder.Kind != render.OpFillRoundRect || placeholder.CornerRadius != radius {
		t.Fatalf("placeholder = %+v, want a rounded fill of radius %d", placeholder, radius)
	}

	del.waitFor(t, len(g.Bindings(nil)))
	del.drain()
	for range 4 {
		imgs.beginFrame()
		h.Frame()
	}

	pictures := 0
	for _, op := range h.Ops() {
		if op.Kind != render.OpImage {
			continue
		}
		pictures++
		if op.Fit != render.ImageCover || op.CornerRadius != radius {
			t.Errorf("tile picture %+v: want a cover crop with the tile's radius %d", op, radius)
		}
	}
	if pictures == 0 {
		t.Fatalf("no picture reached the display list:\n%s", h.Dump())
	}

	picture, border2 := selectedTile(t, h.Ops())
	if picture.Kind != render.OpImage {
		t.Fatalf("the op under the selection border is %+v, want the picture", picture)
	}
	if picture.Bounds != placeholder.Bounds || picture.CornerRadius != placeholder.CornerRadius {
		t.Errorf("the tile changed shape when its picture arrived: placeholder %v radius %v, picture %v radius %v",
			placeholder.Bounds, placeholder.CornerRadius, picture.Bounds, picture.CornerRadius)
	}
	if picture.Bounds != border2.Bounds || picture.CornerRadius != border2.CornerRadius {
		t.Errorf("the selection border %v radius %v does not sit on the picture %v radius %v",
			border2.Bounds, border2.CornerRadius, picture.Bounds, picture.CornerRadius)
	}
	if picture.Clip != border2.Clip || border.Clip != border2.Clip {
		t.Errorf("the picture is under clip %d and its border under clip %d; a cover crop pushes no clip",
			picture.Clip, border2.Clip)
	}
}

// selectedTile returns the operation drawn directly under the selection
// border of the test gallery, and the border itself.
func selectedTile(t *testing.T, ops []render.Op) (under, border render.Op) {
	t.Helper()
	for i, op := range ops {
		if op.Kind == render.OpStrokeRoundRect && op.StrokeWidth == 3 && i > 0 {
			return ops[i-1], op
		}
	}
	t.Fatal("no selection border in the display list")
	return
}

// imageViewFixture draws one ui.Image of the first test picture, 64 by 48
// pixels, inside a fixed frame, and returns the harness, the display list of
// the frame before the picture arrived and the one after.
func imageViewFixture(t *testing.T, view func(asset.Source) ui.ImageView) (h *gifttest.Harness, before, after []render.Op) {
	t.Helper()
	_, items, srcs := writePictures(t, 1)
	del := newDeliverer()
	pipe := asset.NewPipeline(asset.Config{Deliver: del.deliver, Sizes: []int{128}, Workers: 1})
	t.Cleanup(func() { pipe.Close(); del.drain() })
	ui.ResetImageService()
	ui.SetImagePipeline(pipe)
	t.Cleanup(ui.ResetImageService)

	imgs := newFakeImages()
	h = gifttest.New(t, gifttest.Options{
		View: ui.VStack(view(srcs[items[0].ID]).Key("pic")).Frame(120, 100),
		Size: geom.Sz(120, 100),
	})
	h.App().SetImages(imgs)
	imgs.beginFrame()
	h.Frame()
	before = append(before, h.Ops()...)
	del.waitFor(t, 1)
	del.drain()
	for range 3 {
		imgs.beginFrame()
		h.Frame()
	}
	after = append(after, h.Ops()...)
	return h, before, after
}

func findOp(ops []render.Op, kind render.OpKind) (render.Op, bool) {
	for _, op := range ops {
		if op.Kind == kind {
			return op, true
		}
	}
	return render.Op{}, false
}

// TestImageViewClipRoundsThePictureAndItsPlaceholder is the ui.Image half of
// the brief: CornerRadius with Clip(true) rounds the picture, and the
// placeholder before it has the same shape. Across a padding the radius is
// the concentric one, the view's radius minus the padding, which is the rule
// of the focus ring.
func TestImageViewClipRoundsThePictureAndItsPlaceholder(t *testing.T) {
	h, before, after := imageViewFixture(t, func(src asset.Source) ui.ImageView {
		return ui.Image(src).Frame(100, 80).Padding(4).CornerRadius(12).Clip(true).
			Fit(ui.FitCover).Placeholder(ui.RGB(90, 90, 90))
	})
	b := h.Find(gifttest.ByKey("pic")).Bounds()
	inner := geom.Rc(b.Min.X+4, b.Min.Y+4, b.Max.X-4, b.Max.Y-4)

	ph, ok := findOp(before, render.OpFillRoundRect)
	if !ok || ph.Bounds != inner || ph.CornerRadius != 8 {
		t.Errorf("placeholder = %+v (found %v), want a rounded fill of %v with radius 8", ph, ok, inner)
	}
	pic, ok := findOp(after, render.OpImage)
	if !ok {
		t.Fatalf("no picture after the answer arrived:\n%s", h.Dump())
	}
	if pic.Bounds != inner || pic.CornerRadius != 8 || pic.Fit != render.ImageCover {
		t.Errorf("picture = %+v, want a cover crop of %v with radius 8, the placeholder's shape", pic, inner)
	}
}

// TestImageViewWithoutClipKeepsASquarePicture pins the other half of the
// contract on ImageView.CornerRadius: content clipping is switched on
// explicitly, so a radius alone rounds the background, the border and the
// placeholder, and the picture stays square.
func TestImageViewWithoutClipKeepsASquarePicture(t *testing.T) {
	_, before, after := imageViewFixture(t, func(src asset.Source) ui.ImageView {
		return ui.Image(src).Frame(100, 80).CornerRadius(12).Fit(ui.FitStretch).Placeholder(ui.RGB(90, 90, 90))
	})
	if ph, ok := findOp(before, render.OpFillRoundRect); !ok || ph.CornerRadius != 12 {
		t.Errorf("placeholder = %+v (found %v), want radius 12 as before", ph, ok)
	}
	pic, ok := findOp(after, render.OpImage)
	if !ok || pic.CornerRadius != 0 || pic.Fit != render.ImageStretch {
		t.Errorf("picture = %+v (found %v), want a square stretch", pic, ok)
	}
}

// TestImageViewContainRoundsTheLetterboxedPicture: a contained picture is
// rounded on the rectangle it actually occupies, so it has corners of its own
// rather than square ones floating inside a rounded frame.
func TestImageViewContainRoundsTheLetterboxedPicture(t *testing.T) {
	h, _, after := imageViewFixture(t, func(src asset.Source) ui.ImageView {
		return ui.Image(src).Frame(100, 100).CornerRadius(10).Clip(true).Fit(ui.FitContain)
	})
	b := h.Find(gifttest.ByKey("pic")).Bounds()
	pic, ok := findOp(after, render.OpImage)
	if !ok {
		t.Fatal("no picture")
	}
	// A 4:3 picture in a square: full width, letterboxed vertically.
	want := geom.Rc(b.Min.X, b.Min.Y+12.5, b.Max.X, b.Max.Y-12.5)
	if !nearRect(pic.Bounds, want) || pic.CornerRadius != 10 || pic.Fit != render.ImageStretch {
		t.Errorf("picture = %+v, want %v with radius 10 and no crop", pic, want)
	}
}

func nearRect(a, b geom.Rect) bool {
	n := func(x, y float32) bool { return x-y < 0.01 && y-x < 0.01 }
	return n(a.Min.X, b.Min.X) && n(a.Min.Y, b.Min.Y) && n(a.Max.X, b.Max.X) && n(a.Max.Y, b.Max.Y)
}
