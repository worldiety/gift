package asset

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/worldiety/gift/cms"
)

// Thumbnails are sRGB, which is what gift paints, and they are made with the
// perceptual intent, the one meant for photographs: for the matrix profiles
// of displays and cameras it is the same as relative colorimetric, and the
// tables of a printer profile compress its gamut as their maker chose.
var thumbnailOptions = cms.Options{Intent: cms.Perceptual}

// conversion turns the colours of a picture into sRGB.
type conversion struct {
	t *cms.Transform
	// gray is the sRGB colour of each of the 256 gray levels, for a gray
	// profile: a thumbnail is RGBA, and a gray level is looked up in it.
	gray []byte
}

// newConversion links p to sRGB, or returns nil when that changes nothing.
func newConversion(p *cms.Profile) (*conversion, error) {
	t, err := cms.NewTransform(p, cms.SRGB(), thumbnailOptions)
	if err != nil {
		return nil, err
	}
	if t.Identity() {
		return nil, nil
	}
	c := &conversion{t: t}
	if p.Space() == cms.SpaceRGB && unchanged8(t) {
		return nil, nil
	}
	if p.Space() == cms.SpaceGray {
		var ramp [256]byte
		for i := range ramp {
			ramp[i] = byte(i)
		}
		c.gray = make([]byte, 3*256)
		if err := t.Pixels(c.gray, cms.RGB8, ramp[:], cms.Gray8); err != nil {
			return nil, err
		}
		neutral := true
		for i := range 256 {
			v := byte(i)
			neutral = neutral && c.gray[3*i] == v && c.gray[3*i+1] == v && c.gray[3*i+2] == v
		}
		if neutral {
			return nil, nil
		}
	}
	return c, nil
}

// unchanged8 reports whether an RGB transform leaves 8 bit colours as they
// are, on a grid of every fifth level, 140 608 colours: most sRGB profiles
// in the wild differ from the one built in by less than half a step, which
// rounds away, and converting with them would cost a thumbnail time and
// change nothing.
func unchanged8(t *cms.Transform) bool {
	const step, n = 5, 255/5 + 1
	in := make([]byte, 0, 3*n*n*n)
	for r := 0; r <= 255; r += step {
		for g := 0; g <= 255; g += step {
			for b := 0; b <= 255; b += step {
				in = append(in, byte(r), byte(g), byte(b))
			}
		}
	}
	out := make([]byte, len(in))
	return t.Pixels(out, cms.RGB8, in, cms.RGB8) == nil && bytes.Equal(in, out)
}

// early reports whether a picture of the colour model m is converted before
// it is scaled rather than after: CMYK, because the scaler would read it
// with the naive formula of the image package, and 16 bit pictures, whose
// shadows 8 bits would band in a linear or wide space.
func (c *conversion) early(m color.Model) bool {
	switch m {
	case color.CMYKModel:
		return c.t.Source() == cms.SpaceCMYK
	case color.RGBA64Model, color.NRGBA64Model:
		return c.t.Source() == cms.SpaceRGB
	case color.Gray16Model:
		return c.t.Source() == cms.SpaceGray
	}
	return false
}

// full converts a whole decoded picture into an sRGB [image.RGBA] of its
// size, for the pictures [conversion.early] names. It returns false for any
// other kind.
func (c *conversion) full(img image.Image) (*image.RGBA, bool) {
	var (
		pix         []byte
		off, stride int
		f           cms.Format
		s           cms.Space
	)
	b := img.Bounds()
	switch m := img.(type) {
	case *image.CMYK:
		pix, off, stride, f, s = m.Pix, m.PixOffset(b.Min.X, b.Min.Y), m.Stride, cms.CMYK8, cms.SpaceCMYK
	case *image.RGBA64:
		pix, off, stride, f, s = m.Pix, m.PixOffset(b.Min.X, b.Min.Y), m.Stride, cms.RGBAPremul16, cms.SpaceRGB
	case *image.NRGBA64:
		pix, off, stride, f, s = m.Pix, m.PixOffset(b.Min.X, b.Min.Y), m.Stride, cms.RGBA16, cms.SpaceRGB
	case *image.Gray16:
		pix, off, stride, f, s = m.Pix, m.PixOffset(b.Min.X, b.Min.Y), m.Stride, cms.Gray16, cms.SpaceGray
	default:
		return nil, false
	}
	if s != c.t.Source() {
		return nil, false
	}
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	n := w * f.Size()
	// A picture converted whole is a large one, a 24 megapixel CMYK
	// picture takes over half a second on one core: it is converted in
	// bands of rows on as many cores as there are.
	var (
		next   atomic.Int64
		failed atomic.Bool
		wg     sync.WaitGroup
	)
	const band = 64
	for range max(1, min(runtime.GOMAXPROCS(0), h/band)) {
		wg.Go(func() {
			for {
				y0 := int(next.Add(band)) - band
				if y0 >= h {
					return
				}
				for y := y0; y < min(h, y0+band); y++ {
					src := pix[off+y*stride : off+y*stride+n]
					row := dst.Pix[y*dst.Stride : y*dst.Stride+4*w]
					if c.t.Pixels(row, cms.RGBAPremul8, src, f) != nil {
						failed.Store(true)
						return
					}
				}
			}
		})
	}
	wg.Wait()
	return dst, !failed.Load()
}

// thumbnail converts a scaled thumbnail in place. It reports false when it
// cannot, which is a CMYK profile on a picture its decoder did not hand over
// as CMYK.
func (c *conversion) thumbnail(dst *image.RGBA) bool {
	switch c.t.Source() {
	case cms.SpaceRGB:
		return c.t.Pixels(dst.Pix, cms.RGBAPremul8, dst.Pix, cms.RGBAPremul8) == nil
	case cms.SpaceGray:
		grayLookup(dst.Pix, c.gray)
		return true
	}
	return false
}

// grayLookup replaces each gray pixel of premultiplied RGBA by the colour
// lut gives its level.
func grayLookup(pix, lut []byte) {
	for i := 0; i+3 < len(pix); i += 4 {
		switch a := uint32(pix[i+3]); a {
		case 0:
		case 255:
			v := int(pix[i])
			copy(pix[i:i+3], lut[3*v:3*v+3])
		default:
			v := min(255, (uint32(pix[i])*255+a/2)/a)
			for k := range 3 {
				pix[i+k] = byte((uint32(lut[3*v+uint32(k)])*a + 127) / 255)
			}
		}
	}
}

// maxTransforms is how many conversions a pipeline keeps. A gallery of one
// camera's pictures needs one or two; a table conversion holds about 1.4 MB.
const maxTransforms = 16

// transforms caches the conversions of the profiles pictures declare, by
// the hash of the profile: a camera embeds the same one in every picture,
// and baking it takes milliseconds that a thumbnail should not pay each
// time.
type transforms struct {
	mu    sync.Mutex
	m     map[[sha256.Size]byte]*transformEntry
	order [][sha256.Size]byte // least recently used first
}

type transformEntry struct {
	once sync.Once
	c    *conversion
	err  error
}

// get returns the conversion of p to sRGB, nil when there is nothing to
// convert. Concurrent callers for the same profile bake it once.
func (ts *transforms) get(p *cms.Profile) (*conversion, error) {
	if p == cms.SRGB() || p == cms.SGray() {
		return nil, nil
	}
	key := sha256.Sum256(p.Bytes())
	ts.mu.Lock()
	if ts.m == nil {
		ts.m = make(map[[sha256.Size]byte]*transformEntry)
	}
	e, ok := ts.m[key]
	if ok {
		for i, k := range ts.order {
			if k == key {
				ts.order = append(ts.order[:i], ts.order[i+1:]...)
				break
			}
		}
	} else {
		e = &transformEntry{}
		ts.m[key] = e
		if len(ts.order) >= maxTransforms {
			delete(ts.m, ts.order[0])
			ts.order = ts.order[1:]
		}
	}
	ts.order = append(ts.order, key)
	ts.mu.Unlock()
	e.once.Do(func() { e.c, e.err = newConversion(p) })
	return e.c, e.err
}

// len is the number of conversions held, for tests.
func (ts *transforms) len() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.m)
}
