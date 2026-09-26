//go:build giftgpu

package ebiten

import (
	"image/color"
	"math"
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// The pixel half of the rounded picture. roundimage_test.go says which
// vertices reach the stream; these say that the stream draws what
// render.OpImage promises.

var black = color.RGBA{0, 0, 0, 255}

// solid returns an n by n payload of one colour.
func solid(n int, c color.RGBA) render.Pixels {
	return checker(n, c, c, c, c)
}

// gradient returns a w by h payload whose red channel rises left to right and
// whose green channel rises top to bottom, so that a readback can tell a
// texture coordinate that is off by a texel from one that is right, and a
// nearest fetch from a linear one.
func gradient(w, h int) render.Pixels {
	px := render.Pixels{Pix: make([]byte, w*h*4), W: w, H: h, Stride: w * 4}
	for y := range h {
		for x := range w {
			i := y*px.Stride + x*4
			px.Pix[i] = uint8(x * 255 / (w - 1))
			px.Pix[i+1] = uint8(y * 255 / (h - 1))
			px.Pix[i+2] = 64
			px.Pix[i+3] = 255
		}
	}
	return px
}

func acquire(t *testing.T, r *Renderer, px render.Pixels) render.ImageID {
	t.Helper()
	h, ok := r.Textures().Acquire(px)
	if !ok {
		t.Fatal("the upload was refused")
	}
	id, _ := r.Textures().Resolve(h)
	return id
}

// TestRoundImageCoversExactlyWhatARoundFillCovers is the promise on
// render.OpImage that makes a placeholder turn into its picture without
// changing shape: an opaque white picture with rounded corners and an opaque
// white rounded fill of the same bounds and radius produce the same pixels,
// antialiased edge included, at every pixel of the frame.
func TestRoundImageCoversExactlyWhatARoundFillCovers(t *testing.T) {
	b := geom.Rc(4.5, 6, 70, 51.25)
	const radius = 13
	pic := drawImages(t, 80, 60, black, func(r *Renderer, l *render.List) {
		l.Add(render.Op{Kind: render.OpImage, Bounds: b, Color: render.RGB(255, 255, 255),
			Image: acquire(t, r, solid(8, white)), CornerRadius: radius})
	})
	fill := drawImages(t, 80, 60, black, func(_ *Renderer, l *render.List) {
		l.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: render.RGB(255, 255, 255), CornerRadius: radius})
	})
	partial := 0
	for y := range 60 {
		for x := range 80 {
			got, want := pic.At(x, y).(color.RGBA), fill.At(x, y).(color.RGBA)
			if !near(got, want, 1) {
				t.Errorf("pixel (%d, %d): picture %v, fill %v", x, y, got, want)
			}
			if want.R > 0 && want.R < 255 {
				partial++
			}
		}
	}
	// A comparison of two blank or two square images would pass too; the
	// antialiased corners are what is being compared.
	if partial < 20 {
		t.Fatalf("only %d partially covered pixels; the comparison did not see a rounded edge", partial)
	}
}

// TestRoundImageInteriorIsTheSquarePicture checks the filter written out in
// roundimage.kage against Ebitengine's own: away from the corners, a rounded
// picture is the same picture as a square one, texel for texel, including the
// resampling of a small texture onto a large rectangle.
//
// "Away from the corners" also has to mean away from the outermost half texel
// along every edge, and that exclusion is a finding rather than a tolerance.
// Ebitengine's linear filter does not clamp its taps to the source region, so
// in that band it blends the transparent padding around the texture into the
// picture: the square picture below fades from 130 to 74 over the four pixels
// of its left edge, where the rounded one, which clamps, stays at 130. That is
// the square path's behaviour and it is left alone here; at the sizes a
// thumbnail is drawn at it is half a device pixel wide.
func TestRoundImageInteriorIsTheSquarePicture(t *testing.T) {
	b := geom.Rc(3, 5, 93, 65)
	const radius = 10
	draw := func(radius float32) *eb.Image {
		return drawImages(t, 96, 70, black, func(r *Renderer, l *render.List) {
			l.Add(render.Op{Kind: render.OpImage, Bounds: b, Color: render.RGB(255, 255, 255),
				Image: acquire(t, r, gradient(13, 7)), CornerRadius: radius})
		})
	}
	square, round := draw(0), draw(radius)
	worst := 0
	// Half a texel of a 13 by 7 texture on 90 by 60 pixels, plus one.
	const mx, my = 5, 6
	for y := 5 + my; y < 65-my; y++ {
		for x := 3 + mx; x < 93-mx; x++ {
			// Skip the corner squares, where the two are supposed to differ.
			inCornerX := x < 3+radius+1 || x >= 93-radius-1
			inCornerY := y < 5+radius+1 || y >= 65-radius-1
			if inCornerX && inCornerY {
				continue
			}
			a, c := square.At(x, y).(color.RGBA), round.At(x, y).(color.RGBA)
			for _, d := range []int{absDiff(a.R, c.R), absDiff(a.G, c.G), absDiff(a.B, c.B)} {
				worst = max(worst, d)
			}
		}
	}
	t.Logf("largest channel difference outside the corners: %d", worst)
	if worst > 2 {
		t.Errorf("the rounded picture differs from the square one by up to %d outside the corners; "+
			"the filter of roundimage.kage does not match Ebitengine's linear filter", worst)
	}
	// And the corners do differ: the outermost corner pixel of the rounded
	// picture is the background.
	if got := round.At(3, 5).(color.RGBA); !near(got, black, 2) {
		t.Errorf("corner pixel of the rounded picture = %v, want the background", got)
	}
	if got := square.At(3, 5).(color.RGBA); near(got, black, 2) {
		t.Errorf("corner pixel of the square picture = %v, want picture", got)
	}
}

// TestRoundImageCoverShowsTheCentre is render.ImageCover on screen: a texture
// of three vertical stripes, red, green and blue, each as wide as the texture
// is tall, drawn into a square shows only the green one.
//
// The samples stay two pixels inside the tile. The outermost pixel column
// legitimately blends a little of the neighbouring stripe, because a linear
// filter at the edge of a crop reads the picture that continues beyond it —
// which is what a crop is.
func TestRoundImageCoverShowsTheCentre(t *testing.T) {
	px := render.Pixels{Pix: make([]byte, 30*10*4), W: 30, H: 10, Stride: 120}
	for y := range 10 {
		for x := range 30 {
			c := [3]color.RGBA{red, green, blue}[x/10]
			i := y*px.Stride + x*4
			px.Pix[i], px.Pix[i+1], px.Pix[i+2], px.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	for _, radius := range []float32{0, 8} {
		dst := drawImages(t, 48, 48, black, func(r *Renderer, l *render.List) {
			l.Add(render.Op{Kind: render.OpImage, Fit: render.ImageCover, Bounds: geom.Rc(8, 8, 40, 40),
				Color: render.RGB(255, 255, 255), Image: acquire(t, r, px), CornerRadius: radius})
		})
		for _, p := range [][2]int{{10, 24}, {24, 24}, {37, 24}, {24, 10}, {24, 37}} {
			if got := dst.At(p[0], p[1]).(color.RGBA); !near(got, green, 3) {
				t.Errorf("radius %v, pixel %v = %v, want green: only the centre stripe survives the crop", radius, p, got)
			}
		}
		if got := dst.At(6, 24).(color.RGBA); !near(got, black, 2) {
			t.Errorf("radius %v, left of the tile = %v, want the background: nothing outside Bounds", radius, got)
		}
	}
}

// TestBorderSitsOnARoundedPicture is the gallery tile of the brief, in the
// backend's terms: a rounded picture with a stroke of the same bounds and
// radius on top of it.
//
//   - Beyond the outer arc nothing of the picture shows.
//   - Between the arcs is the stroke.
//   - The inner edge of the stroke is the concentric arc of radius
//     max(0, radius-width), not a square corner — the pixel at (4, 4) inside
//     the box is inside a square inner corner and outside the concentric one.
//   - Inside the inner arc is the picture.
func TestBorderSitsOnARoundedPicture(t *testing.T) {
	const radius, width = 12, 4
	b := geom.Rc(10, 10, 70, 50)
	dst := drawImages(t, 80, 60, black, func(r *Renderer, l *render.List) {
		l.Add(render.Op{Kind: render.OpImage, Fit: render.ImageCover, Bounds: b, Color: render.RGB(255, 255, 255),
			Image: acquire(t, r, solid(8, red)), CornerRadius: radius})
		l.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: b, Color: render.RGB(255, 255, 255),
			CornerRadius: radius, StrokeWidth: width})
	})
	// Along the diagonal of the top left corner, whose arcs are centred on
	// (22, 22). Pixel centres are at +0.5.
	for _, c := range []struct {
		x    int
		want color.RGBA
		what string
	}{
		{10 + 1, black, "beyond the outer arc"},
		{10 + 4, white, "between the arcs, inside a square inner corner"},
		{10 + 7, red, "inside the inner arc"},
		{40, red, "the middle"},
	} {
		got := dst.At(c.x, c.x).(color.RGBA)
		if c.x == 40 {
			got = dst.At(40, 30).(color.RGBA)
		}
		if !near(got, c.want, 3) {
			d := math.Hypot(22-float64(c.x)-0.5, 22-float64(c.x)-0.5)
			t.Errorf("%s (pixel %d, %.1f from the arc centre) = %v, want %v", c.what, c.x, d, got, c.want)
		}
	}
	// No red anywhere outside the outer contour: scan the whole corner
	// square for pixels whose red exceeds what the stroke's antialiasing
	// can leave there.
	for y := 10; y < 22; y++ {
		for x := 10; x < 22; x++ {
			dx, dy := 22-float64(x)-0.5, 22-float64(y)-0.5
			if math.Hypot(dx, dy) < radius+1 {
				continue
			}
			if got := dst.At(x, y).(color.RGBA); got.R > 2 {
				t.Errorf("pixel (%d, %d) outside the rounded shape = %v; the picture shows through", x, y, got)
			}
		}
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
