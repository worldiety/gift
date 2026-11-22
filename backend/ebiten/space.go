package ebiten

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/worldiety/gift/cms"
	"github.com/worldiety/gift/render"
)

// workingSpace is the colour space the renderer paints in when it is not
// sRGB: the space of the numbers in the frame, and the one the screen is
// tagged with.
//
// # Why there is one
//
// Every colour gift is given is sRGB: the theme, text, and the pictures of
// package asset unless it is told otherwise. On a display that shows more
// than sRGB, a frame of sRGB numbers cannot hold the colours of a photograph
// taken in Display P3, which is what an iPhone takes. A Mac shows P3, and its
// compositor converts from whatever space the Metal layer is tagged with, so
// gift paints in P3 there: every sRGB colour is converted on its way into
// the frame, and a picture made in P3 is drawn as it is.
//
// # What is converted, and where
//
// The colours of the display list, once per operation and per gradient,
// tint and vertex; see [workingSpace.color]. Pictures, when they are
// uploaded, unless they are in the working space already; see
// [TextureCache]. Video, in its conversion shaders. Not the glyph atlas,
// which holds coverage, nor layers and glass, which are drawn from the frame
// and are in its space already.
//
// Display P3 has the white and the curve of sRGB, so a gray is the same
// number in both and is passed through untouched: text, shadows and white
// tints come out bit for bit as before.
type workingSpace struct {
	profile *cms.Profile
	// m turns linear sRGB into the linear working space, row by row.
	m [9]float32
	// memo remembers the conversions of the last colours seen. A frame has
	// few distinct colours and draws each many times, and a conversion is
	// six powers.
	memo [256]spaceMemo
	// toSRGB converts a captured frame back for the automation interface.
	toSRGB *cms.Transform
}

type spaceMemo struct {
	in, out render.Color
	ok      bool
}

// errSpace is returned for a working space the renderer cannot paint in.
var errSpace = errors.New("gift/backend/ebiten: the working space must be sRGB or Display P3")

// newWorkingSpace returns the working space for p, nil for sRGB, which needs
// no conversion.
func newWorkingSpace(p *cms.Profile) (*workingSpace, error) {
	switch p {
	case nil, cms.SRGB():
		return nil, nil
	case cms.DisplayP3():
	default:
		return nil, fmt.Errorf("%w, not %s", errSpace, p.Description())
	}
	// The matrix is read off the colour management of package cms rather
	// than written down again: linear sRGB through to P3, whose curve is
	// the sRGB curve, undone.
	t, err := cms.NewTransform(cms.LinearSRGB(), p, cms.Options{Intent: cms.RelativeColorimetric})
	if err != nil {
		return nil, err
	}
	ws := &workingSpace{profile: p}
	for col := range 3 {
		in := []float64{0, 0, 0}
		in[col] = 1
		out := t.Color(in...)
		for row := range 3 {
			ws.m[3*row+col] = float32(srgbDecode(out[row]))
		}
	}
	if ws.toSRGB, err = cms.NewTransform(p, cms.SRGB(), cms.Options{Intent: cms.RelativeColorimetric}); err != nil {
		return nil, err
	}
	return ws, nil
}

// color converts an sRGB colour of the display list into the working space.
// The nil working space is sRGB and returns c.
func (ws *workingSpace) color(c render.Color) render.Color {
	if ws == nil || c.A <= 0 || (c.R == c.G && c.G == c.B) {
		return c
	}
	bits := math.Float32bits(c.R)*0x9E3779B1 ^ math.Float32bits(c.G)*0x85EBCA77 ^
		math.Float32bits(c.B)*0xC2B2AE3D ^ math.Float32bits(c.A)
	m := &ws.memo[(bits^bits>>16)&255]
	if m.ok && m.in == c {
		return m.out
	}
	a := float64(c.A)
	r := srgbDecode(min(1, float64(c.R)/a))
	g := srgbDecode(min(1, float64(c.G)/a))
	b := srgbDecode(min(1, float64(c.B)/a))
	out := render.Color{
		R: float32(srgbEncode(float64(ws.m[0])*r+float64(ws.m[1])*g+float64(ws.m[2])*b) * a),
		G: float32(srgbEncode(float64(ws.m[3])*r+float64(ws.m[4])*g+float64(ws.m[5])*b) * a),
		B: float32(srgbEncode(float64(ws.m[6])*r+float64(ws.m[7])*g+float64(ws.m[8])*b) * a),
		A: c.A,
	}
	*m = spaceMemo{in: c, out: out, ok: true}
	return out
}

// matrix is the conversion from linear sRGB as three rows, for a shader. The
// nil working space is the identity.
func (ws *workingSpace) matrix() (r, g, b [3]float32) {
	if ws == nil {
		return [3]float32{1, 0, 0}, [3]float32{0, 1, 0}, [3]float32{0, 0, 1}
	}
	return [3]float32(ws.m[0:3]), [3]float32(ws.m[3:6]), [3]float32(ws.m[6:9])
}

// srgbDecode is the sRGB curve from encoded to linear, clamped to [0, 1].
func srgbDecode(v float64) float64 {
	switch {
	case v <= 0:
		return 0
	case v >= 1:
		return 1
	case v <= 0.04045:
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// srgbEncode is the sRGB curve from linear to encoded, clamped to [0, 1].
func srgbEncode(v float64) float64 {
	switch {
	case v <= 0:
		return 0
	case v >= 1:
		return 1
	case v <= 0.0031308:
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

// pixelConversions are the conversions of uploaded pictures into a working
// space, by the space the pictures are in. Nearly every picture is sRGB or
// in the working space already, so there are one or two.
type pixelConversions struct {
	dst *cms.Profile
	m   map[*cms.Profile]*cms.Transform
}

// get returns the conversion of pixels in src, nil for sRGB, into the working
// space, or nil when there is nothing to convert. A space the pixels cannot
// be converted from is reported once and drawn as it is.
func (pc *pixelConversions) get(src *cms.Profile) (*cms.Transform, error) {
	if src == nil {
		src = cms.SRGB()
	}
	dst := pc.dst
	if dst == nil {
		dst = cms.SRGB()
	}
	if src == dst {
		return nil, nil
	}
	if t, ok := pc.m[src]; ok {
		return t, nil
	}
	if pc.m == nil || len(pc.m) >= 8 {
		pc.m = make(map[*cms.Profile]*cms.Transform)
	}
	t, err := cms.NewTransform(src, dst, cms.Options{Intent: cms.Perceptual})
	if err == nil && (t.Source() != cms.SpaceRGB || t.Destination() != cms.SpaceRGB) {
		t, err = nil, fmt.Errorf("gift/backend/ebiten: pictures are RGB, not %s", src.Description())
	}
	if err == nil && t.Identity() {
		t = nil
	}
	pc.m[src] = t
	return t, err
}

// convertPixels converts n rows of w premultiplied RGBA pixels, src with the
// given stride, into dst, packed. A large picture is converted in bands on
// as many cores as there are: the UI goroutine waits for it, and a 2 MiB
// upload is 7 ms on one core of a Mac.
func convertPixels(t *cms.Transform, dst, src []byte, w, n, stride int) error {
	row := 4 * w
	const band = 32
	workers := min(runtime.GOMAXPROCS(0), (n+band-1)/band)
	if w*n < 1<<16 || workers < 2 {
		for y := range n {
			if err := t.Pixels(dst[y*row:(y+1)*row], cms.RGBAPremul8, src[y*stride:y*stride+row], cms.RGBAPremul8); err != nil {
				return err
			}
		}
		return nil
	}
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		first error
	)
	per := (n + workers - 1) / workers
	for y0 := 0; y0 < n; y0 += per {
		y1 := min(n, y0+per)
		wg.Go(func() {
			for y := y0; y < y1; y++ {
				if err := t.Pixels(dst[y*row:(y+1)*row], cms.RGBAPremul8, src[y*stride:y*stride+row], cms.RGBAPremul8); err != nil {
					mu.Lock()
					if first == nil {
						first = err
					}
					mu.Unlock()
					return
				}
			}
		})
	}
	wg.Wait()
	return first
}
