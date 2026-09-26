package render

import (
	"testing"
	"unsafe"

	"github.com/worldiety/gift/geom"
)

// TestImageFitSource pins the crop arithmetic both sides of the display list
// share. A backend computes the texture coordinates of an [ImageCover]
// operation with exactly this function, so a wrong answer here is a wrong
// picture everywhere.
func TestImageFitSource(t *testing.T) {
	for _, c := range []struct {
		name string
		fit  ImageFit
		dst  geom.Rect
		w, h int
		want geom.Rect
	}{
		// Stretch is the whole texture, whatever the aspect ratios.
		{"stretch", ImageStretch, geom.Rc(0, 0, 100, 10), 40, 30, geom.Rc(0, 0, 40, 30)},
		// A wide texture in a square keeps its full height and loses the
		// same amount on the left and the right.
		{"cover wide into square", ImageCover, geom.Rc(10, 10, 60, 60), 200, 100, geom.Rc(50, 0, 150, 100)},
		// A tall texture in a wide tile keeps its full width.
		{"cover tall into wide", ImageCover, geom.Rc(0, 0, 200, 100), 100, 400, geom.Rc(0, 175, 100, 225)},
		// Equal aspect ratios crop nothing, whatever the scale.
		{"cover same aspect", ImageCover, geom.Rc(0, 0, 32, 24), 400, 300, geom.Rc(0, 0, 400, 300)},
		// The destination's position does not matter, only its shape.
		{"cover is translation invariant", ImageCover, geom.Rc(-500, 70, -450, 120), 200, 100, geom.Rc(50, 0, 150, 100)},
		// Degenerate inputs fall back to the whole texture rather than
		// producing a NaN; the operation is skipped for its bounds anyway.
		{"cover empty dst", ImageCover, geom.Rc(0, 0, 0, 10), 20, 20, geom.Rc(0, 0, 20, 20)},
		{"cover empty texture", ImageCover, geom.Rc(0, 0, 10, 10), 0, 20, geom.Rc(0, 0, 0, 20)},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := c.fit.Source(c.dst, c.w, c.h)
			if !nearRect(got, c.want) {
				t.Errorf("Source(%v, %d, %d) = %v, want %v", c.dst, c.w, c.h, got, c.want)
			}
		})
	}
}

// TestImageCoverSourceHasTheDestinationsAspect is the property rather than the
// examples: whatever the two shapes, the crop has the aspect ratio of the
// destination and lies inside the texture, centred.
func TestImageCoverSourceHasTheDestinationsAspect(t *testing.T) {
	for _, dst := range []geom.Rect{
		geom.Rc(0, 0, 1, 1), geom.Rc(0, 0, 90, 160), geom.Rc(3, 4, 1003, 17), geom.Rc(0, 0, 0.5, 700),
	} {
		for _, tex := range [][2]int{{1, 1}, {1920, 1080}, {7, 3000}, {128, 128}} {
			src := ImageCover.Source(dst, tex[0], tex[1])
			if src.Min.X < -1e-3 || src.Min.Y < -1e-3 ||
				src.Max.X > float32(tex[0])+1e-3 || src.Max.Y > float32(tex[1])+1e-3 {
				t.Errorf("dst %v, texture %v: crop %v leaves the texture", dst, tex, src)
			}
			da := dst.Width() / dst.Height()
			sa := src.Width() / src.Height()
			if d := da/sa - 1; d > 1e-3 || d < -1e-3 {
				t.Errorf("dst %v, texture %v: crop %v has aspect %v, want %v", dst, tex, src, sa, da)
			}
			cx, cy := (src.Min.X+src.Max.X)*0.5, (src.Min.Y+src.Max.Y)*0.5
			if !near(cx, float32(tex[0])*0.5) || !near(cy, float32(tex[1])*0.5) {
				t.Errorf("dst %v, texture %v: crop %v is not centred", dst, tex, src)
			}
		}
	}
}

// TestImageOpCarriesFitAndRadiusThroughTheList is the display list half of a
// rounded, cropped picture: the two fields that say so survive the list
// unchanged, next to the clip and transform indices the list assigns, and the
// operation stays plain old data of the pinned size.
func TestImageOpCarriesFitAndRadiusThroughTheList(t *testing.T) {
	var l List
	l.Reset()
	clip := l.PushClip(geom.Rc(0, 0, 100, 100))
	l.Add(Op{
		Kind: OpImage, Fit: ImageCover, Bounds: geom.Rc(10, 10, 60, 40),
		Color: RGB(255, 255, 255), CornerRadius: 8, Image: 3, Clip: clip,
	})
	l.PopClip()
	ops := l.Ops()
	if len(ops) != 1 {
		t.Fatalf("%d ops, want 1", len(ops))
	}
	op := ops[0]
	if op.Fit != ImageCover || op.CornerRadius != 8 || op.Image != 3 || op.Clip != clip {
		t.Errorf("op = %+v; the fit, the radius, the image or the clip did not survive the list", op)
	}
	if op.PaintBounds() != op.Bounds {
		t.Errorf("PaintBounds = %v, want the bounds %v; a rounded picture paints inside them",
			op.PaintBounds(), op.Bounds)
	}
	// Fit is free: it lives in the padding after Kind. If a later field moves
	// it, TestOpIsStillPlainOldData notices the size and this notices why.
	if off := unsafe.Offsetof(Op{}.Fit); off != 1 {
		t.Errorf("offset of Fit = %d, want 1, directly after Kind in the padding before Bounds", off)
	}
	var zero Op
	if zero.Fit != ImageStretch {
		t.Error("the zero Fit must be ImageStretch, which is what every OpImage meant before Fit existed")
	}
}

func near(a, b float32) bool {
	d := a - b
	return d < 1e-3 && d > -1e-3
}

func nearRect(a, b geom.Rect) bool {
	return near(a.Min.X, b.Min.X) && near(a.Min.Y, b.Min.Y) && near(a.Max.X, b.Max.X) && near(a.Max.Y, b.Max.Y)
}
