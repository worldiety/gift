package cms

import (
	"errors"
	"fmt"
)

// Format is the layout of pixels in a byte slice. The 16 bit formats are
// big-endian, as in the image package; each format names the image type
// whose Pix it reads and writes.
type Format uint8

// The pixel formats a transform reads and writes.
const (
	Gray8        Format = iota + 1 // image.Gray
	RGB8                           // three bytes per pixel
	RGBA8                          // image.NRGBA, straight alpha
	RGBAPremul8                    // image.RGBA, premultiplied alpha
	CMYK8                          // image.CMYK, 0 is no ink
	Gray16                         // image.Gray16
	RGB16                          // three big-endian words per pixel
	RGBA16                         // image.NRGBA64, straight alpha
	RGBAPremul16                   // image.RGBA64, premultiplied alpha
	CMYK16                         // four big-endian words per pixel
)

func (f Format) String() string {
	names := [...]string{"Format(0)", "Gray8", "RGB8", "RGBA8", "RGBAPremul8", "CMYK8", "Gray16", "RGB16", "RGBA16", "RGBAPremul16", "CMYK16"}
	if int(f) < len(names) {
		return names[f]
	}
	return fmt.Sprintf("Format(%d)", uint8(f))
}

// channels is the number of colour channels, alpha not counted.
func (f Format) channels() int {
	switch f {
	case Gray8, Gray16:
		return 1
	case CMYK8, CMYK16:
		return 4
	case 0:
		return 0
	}
	if f > CMYK16 {
		return 0
	}
	return 3
}

func (f Format) alpha() bool {
	return f == RGBA8 || f == RGBAPremul8 || f == RGBA16 || f == RGBAPremul16
}

func (f Format) premultiplied() bool { return f == RGBAPremul8 || f == RGBAPremul16 }

func (f Format) wide() bool { return f >= Gray16 }

// Size is the number of bytes of one pixel.
func (f Format) Size() int {
	n := f.channels()
	if f.alpha() {
		n++
	}
	if f.wide() {
		n *= 2
	}
	return n
}

// block is how many pixels Pixels converts at a time between bytes and the
// float32 of a kernel.
const block = 256

// Pixels converts the pixels of src, in format sf, into dst, in format df.
// The formats must have the channels of the transform's source and
// destination spaces, and dst must have room for as many pixels as src
// holds. Alpha is carried over, or opaque where src has none.
//
// The work is done with the baked tables, in one goroutine; split a large
// picture into bands to use more.
func (t *Transform) Pixels(dst []byte, df Format, src []byte, sf Format) error {
	if sf.channels() != t.in || df.channels() != t.out {
		return fmt.Errorf("cms: %s to %s for a transform from %s to %s", sf, df, t.src, t.dst)
	}
	if len(src)%sf.Size() != 0 {
		return errors.New("cms: source holds a partial pixel")
	}
	n := len(src) / sf.Size()
	if len(dst) < n*df.Size() {
		return errors.New("cms: destination too small")
	}
	// Two matrix profiles and 8 bit RGB on both sides have a fast path,
	// which a premultiplied block can take too where it is opaque: an
	// opaque premultiplied pixel is a straight one.
	ms, fast := t.k.(*matrixShaper)
	fast = fast && !t.identity && rgb8(sf) && rgb8(df)
	straight := !sf.premultiplied() && !df.premultiplied()
	var in, out [block * 4]float32
	var alpha [block]float32
	for start := 0; start < n; start += block {
		m := min(block, n-start)
		s := src[start*sf.Size() : (start+m)*sf.Size()]
		d := dst[start*df.Size() : (start+m)*df.Size()]
		if fast && (straight || opaque(s, sf)) {
			ms.run8(d, df.Size(), s, sf.Size(), m)
			continue
		}
		unpack(in[:m*t.in], alpha[:m], s, sf)
		if t.identity {
			copy(out[:m*t.out], in[:m*t.in])
		} else {
			t.k.run(out[:m*t.out], in[:m*t.in], m)
		}
		pack(d, df, out[:m*t.out], alpha[:m])
	}
	return nil
}

func rgb8(f Format) bool { return f == RGB8 || f == RGBA8 || f == RGBAPremul8 }

// opaque reports whether every pixel of s is opaque.
func opaque(s []byte, f Format) bool {
	if !f.alpha() {
		return true
	}
	size := f.Size()
	for i := size - 1; i < len(s); i += size {
		if s[i] != 255 {
			return false
		}
	}
	return true
}

// unpack reads pixels into colour values from 0 to 1 and alpha, the
// colour of a premultiplied pixel divided by its alpha.
func unpack(c, alpha []float32, s []byte, f Format) {
	ch := f.channels()
	size := f.Size()
	for p := range alpha {
		px := s[p*size : p*size+size]
		a := float32(1)
		if f.alpha() {
			if f.wide() {
				a = float32(uint16(px[6])<<8|uint16(px[7])) / 65535
			} else {
				a = float32(px[3]) / 255
			}
		}
		alpha[p] = a
		for i := range ch {
			var v float32
			if f.wide() {
				v = float32(uint16(px[2*i])<<8|uint16(px[2*i+1])) / 65535
			} else {
				v = float32(px[i]) / 255
			}
			if f.premultiplied() {
				if a > 0 {
					v = min(v/a, 1)
				} else {
					v = 0
				}
			}
			c[p*ch+i] = v
		}
	}
}

// pack writes colour values and alpha as pixels, premultiplying where the
// format wants it.
func pack(d []byte, f Format, c, alpha []float32) {
	ch := f.channels()
	size := f.Size()
	for p, a := range alpha {
		px := d[p*size : p*size+size]
		for i := range ch {
			v := unit(c[p*ch+i])
			if f.premultiplied() {
				v *= a
			}
			if f.wide() {
				w := uint16(v*65535 + 0.5)
				px[2*i], px[2*i+1] = byte(w>>8), byte(w)
			} else {
				px[i] = byte(v*255 + 0.5)
			}
		}
		if f.alpha() {
			if f.wide() {
				w := uint16(a*65535 + 0.5)
				px[6], px[7] = byte(w>>8), byte(w)
			} else {
				px[3] = byte(a*255 + 0.5)
			}
		}
	}
}
