package raw

import (
	"encoding/binary"
	"fmt"
)

// ReadMosaic decodes the sensor data: Mosaic.Width by Mosaic.Height values,
// row by row, in the units of Mosaic.Black and Mosaic.White, the margins
// beyond the crop included.
//
// It reads the whole of the data, tens of megabytes, and the slice it
// returns is two bytes a photosite.
func (f *File) ReadMosaic() ([]uint16, error) {
	m := &f.Mosaic
	if m.Width <= 0 || m.Height <= 0 || m.SamplesPerPixel != 1 || len(m.CFA) != 4 {
		return nil, fmt.Errorf("%w: no mosaic of red, green and blue to decode", ErrUnsupported)
	}
	n := m.Width * m.Height
	if n > maxPhotosites {
		return nil, fmt.Errorf("%w: %dx%d photosites", ErrFormat, m.Width, m.Height)
	}
	switch {
	case f.Format == ARW && m.Compression == SonyCompressed:
		src, err := f.t.readData(f.segs, int64(n))
		if err != nil {
			return nil, err
		}
		out := make([]uint16, n)
		curve := sonyCurve(f.sonyCurve)
		decodeSonyCompressed(out, src, m.Width, m.Height, &curve)
		return out, nil
	case f.Format == ARW && m.Compression == Uncompressed && m.BitsPerSample <= 16:
		src, err := f.t.readData(f.segs, int64(2*n))
		if err != nil {
			return nil, err
		}
		out := make([]uint16, n)
		for i := range out {
			out[i] = f.t.bo.Uint16(src[2*i:])
		}
		return out, nil
	case f.Format == DNG:
		return f.readDNGMosaic()
	}
	return nil, fmt.Errorf("%w: %v data of %d bits", ErrUnsupported, m.Compression, m.BitsPerSample)
}

// maxPhotosites bounds the sensor a file may claim, 400 megapixels; the
// largest cameras have a hundred and fifty, a multi-shot back four hundred.
const maxPhotosites = 400 << 20

// readData reads at least n bytes of the data of segs, one after the
// other, for decoders that need it in one piece.
func (t *tiff) readData(segs []segment, n int64) ([]byte, error) {
	// The segments are checked against the file before anything is
	// allocated, so that a file cannot make this allocate more than its
	// own size.
	var have int64
	for _, s := range segs {
		if s.off < t.base || s.n < 0 || s.off > t.end-s.n {
			return nil, fmt.Errorf("%w: raw data past the end of the file", ErrFormat)
		}
		have += s.n
	}
	if have < n {
		return nil, fmt.Errorf("%w: %d bytes of raw data, %d needed", ErrFormat, have, n)
	}
	out := make([]byte, 0, have)
	for _, s := range segs {
		b := out[len(out) : len(out)+int(s.n)]
		if k, err := t.r.ReadAt(b, s.off-t.base); int64(k) != s.n {
			return nil, fmt.Errorf("%w: raw data cut short: %v", ErrFormat, err)
		}
		out = out[:len(out)+int(s.n)]
	}
	return out, nil
}

// sonyCurve makes the curve Sony's compressed raw goes through, from the
// four knots of tag 0x7010 in the directory of the raw data, as dcraw does:
// the identity up to the first knot, then slopes of 2, 4, 8 and 16; the
// identity without knots. Entry 2v
// is the value, in 14 bits, of the 11-bit number v; it reaches past 16383,
// where the sensor clips.
func sonyCurve(knots [4]uint16) [4096]uint16 {
	var c [4096]uint16
	for i := range c {
		c[i] = uint16(i)
	}
	if knots == ([4]uint16{}) {
		return c
	}
	sc := [6]int{0, 0, 0, 0, 0, 4095}
	for i, k := range knots {
		sc[i+1] = int(k>>2) & 0xfff
	}
	for i := range 5 {
		for j := sc[i] + 1; j <= sc[i+1] && j < len(c); j++ {
			c[j] = c[j-1] + 1<<i
		}
	}
	return c
}

// decodeSonyCompressed decodes Sony's compressed raw, as dcraw's
// sony_arw2_load_raw: a row is a byte a photosite, in blocks of 16 bytes
// for 16 photosites of one colour, every other one of 32 columns. A block
// is 128 bits, little endian: the maximum and the minimum of the 16, 11
// bits each, the places of the maximum and the minimum, 4 bits each, and
// the 14 others as 7-bit steps above the minimum, shifted by as many bits
// as the range between maximum and minimum needs beyond 7.
func decodeSonyCompressed(dst []uint16, src []byte, w, h int, curve *[4096]uint16) {
	// at is the 16 bits at byte k of a block, the bytes past its end 0: a
	// block whose maximum and minimum are said to be at one place has a
	// step more than fits, which only a damaged file has.
	at := func(b []byte, k int) uint32 {
		var v uint32
		if k < len(b) {
			v = uint32(b[k])
		}
		if k+1 < len(b) {
			v |= uint32(b[k+1]) << 8
		}
		return v
	}
	var pix [16]uint32
	for y := range h {
		row := src[y*w : (y+1)*w]
		out := dst[y*w : (y+1)*w]
		for col, dp := 0, 0; col < w-30 && dp+16 <= len(row); dp += 16 {
			b := row[dp : dp+16]
			val := binary.LittleEndian.Uint32(b)
			hi, lo := val&0x7ff, val>>11&0x7ff
			imax, imin := int(val>>22&0xf), int(val>>26&0xf)
			sh := 0
			for sh < 4 && int(0x80<<sh) <= int(hi)-int(lo) {
				sh++
			}
			bit := 30
			for i := range 16 {
				switch i {
				case imax:
					pix[i] = hi
				case imin:
					pix[i] = lo
				default:
					v := (at(b, bit>>3)>>(bit&7)&0x7f)<<sh + lo
					pix[i] = min(v, 0x7ff)
					bit += 7
				}
			}
			for i := range 16 {
				out[col] = curve[pix[i]<<1]
				col += 2
			}
			if col&1 != 0 {
				col--
			} else {
				col -= 31
			}
		}
	}
}
