package raw

import (
	"encoding/binary"
	"fmt"
)

// ljpeg is a lossless JPEG, ITU T.81 process 14, as DNG stores its tiles:
// samples of 2 to 16 bits, each predicted from its neighbours by one of
// seven predictors and the difference Huffman coded.
type ljpeg struct {
	w, h, comps, bits int
	// pred is the predictor 1 to 7, pt the point transform.
	pred, pt int
	// restart is the restart interval in pixels, 0 for none.
	restart int
	huff    [4]*huffman
	// table is the Huffman table of each component, in the order of the
	// scan.
	table [4]int
	// scan is the entropy coded data.
	scan []byte
}

// huffman is a Huffman table of a JPEG with a lookup of the codes of up
// to fastBits bits.
type huffman struct {
	fast    [1 << fastBits]uint16 // value<<8 | length, 0 for a longer code
	maxcode [18]int32
	valptr  [17]int32
	mincode [17]int32
	vals    []uint8
}

const fastBits = 10

// parseLJPEG reads the markers of a lossless JPEG up to its scan.
func parseLJPEG(b []byte) (*ljpeg, error) {
	bad := func(what string) (*ljpeg, error) {
		return nil, fmt.Errorf("%w: lossless JPEG: %s", ErrFormat, what)
	}
	if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 {
		return bad("no start of image")
	}
	j := &ljpeg{}
	var sof bool
	for i := 2; ; {
		for i < len(b) && b[i] == 0xff && i+1 < len(b) && b[i+1] == 0xff {
			i++
		}
		if i+4 > len(b) || b[i] != 0xff {
			return bad("a marker is missing")
		}
		m := b[i+1]
		n := int(binary.BigEndian.Uint16(b[i+2:]))
		if n < 2 || i+2+n > len(b) {
			return bad("a segment is cut short")
		}
		seg := b[i+4 : i+2+n]
		switch m {
		case 0xc3:
			if len(seg) < 6 {
				return bad("a short frame header")
			}
			j.bits = int(seg[0])
			j.h = int(binary.BigEndian.Uint16(seg[1:]))
			j.w = int(binary.BigEndian.Uint16(seg[3:]))
			j.comps = int(seg[5])
			if j.bits < 2 || j.bits > 16 || j.w == 0 || j.h == 0 || j.comps < 1 || j.comps > 4 || len(seg) < 6+3*j.comps {
				return bad(fmt.Sprintf("a frame of %dx%d, %d components of %d bits", j.w, j.h, j.comps, j.bits))
			}
			sof = true
		case 0xc4:
			for len(seg) > 0 {
				if len(seg) < 17 {
					return bad("a short Huffman table")
				}
				id := int(seg[0] & 0x0f)
				if seg[0]>>4 != 0 || id > 3 {
					return bad("a Huffman table not for differences")
				}
				var counts [16]int
				total := 0
				for k := range 16 {
					counts[k] = int(seg[1+k])
					total += counts[k]
				}
				if len(seg) < 17+total || total > 256 {
					return bad("a Huffman table cut short")
				}
				h, err := newHuffman(counts, seg[17:17+total])
				if err != nil {
					return nil, err
				}
				j.huff[id] = h
				seg = seg[17+total:]
			}
		case 0xdd:
			if len(seg) < 2 {
				return bad("a short restart interval")
			}
			j.restart = int(binary.BigEndian.Uint16(seg))
		case 0xda:
			if !sof || len(seg) < 1 {
				return bad("a scan before the frame")
			}
			ns := int(seg[0])
			if ns != j.comps || len(seg) < 1+2*ns+3 {
				return bad("a scan of other components than the frame's")
			}
			for k := range ns {
				t := int(seg[2+2*k] >> 4)
				if t > 3 || j.huff[t] == nil {
					return bad("a scan with a missing Huffman table")
				}
				j.table[k] = t
			}
			j.pred = int(seg[1+2*ns])
			j.pt = int(seg[3+2*ns] & 0x0f)
			if j.pred < 1 || j.pred > 7 || j.pt >= j.bits {
				return bad(fmt.Sprintf("predictor %d, point transform %d", j.pred, j.pt))
			}
			if j.restart > 0 && j.restart%j.w != 0 {
				return bad("a restart interval within a row")
			}
			j.scan = b[i+2+n:]
			return j, nil
		case 0xc0, 0xc1, 0xc2, 0xc5, 0xc6, 0xc7, 0xc9, 0xca, 0xcb, 0xcd, 0xce, 0xcf:
			return bad("not lossless")
		}
		i += 2 + n
	}
}

func newHuffman(counts [16]int, vals []byte) (*huffman, error) {
	h := &huffman{vals: vals}
	code, k := int32(0), int32(0)
	for l := 1; l <= 16; l++ {
		n := int32(counts[l-1])
		h.valptr[l] = k
		h.mincode[l] = code
		if n > 0 {
			code += n
			k += n
			h.maxcode[l] = code - 1
		} else {
			h.maxcode[l] = -1
		}
		if code > 1<<l {
			return nil, fmt.Errorf("%w: lossless JPEG: a Huffman table with too many codes", ErrFormat)
		}
		if l <= fastBits {
			for c := h.mincode[l]; c <= h.maxcode[l]; c++ {
				v := uint16(vals[h.valptr[l]+c-h.mincode[l]])
				shift := fastBits - l
				for r := range int32(1) << shift {
					h.fast[c<<shift|r] = v<<8 | uint16(l)
				}
			}
		}
		code <<= 1
	}
	h.maxcode[17] = 1 << 30
	return h, nil
}

// bitReader reads the entropy coded data of a scan, the 0 stuffed after
// every 0xff taken out, and stops at a marker, after which it reads zeros.
type bitReader struct {
	b      []byte
	pos    int
	acc    uint64
	n      uint
	marker bool
}

func (r *bitReader) fill() {
	for r.n <= 56 {
		var c byte
		if !r.marker && r.pos < len(r.b) {
			c = r.b[r.pos]
			if c == 0xff {
				if r.pos+1 < len(r.b) && r.b[r.pos+1] == 0 {
					r.pos += 2
				} else {
					r.marker = true
					c = 0
				}
			} else {
				r.pos++
			}
		}
		r.acc |= uint64(c) << (56 - r.n)
		r.n += 8
	}
}

// restart skips to after the next restart marker.
func (r *bitReader) restart() {
	r.acc, r.n, r.marker = 0, 0, false
	for r.pos+1 < len(r.b) {
		if r.b[r.pos] == 0xff && r.b[r.pos+1] >= 0xd0 && r.b[r.pos+1] <= 0xd7 {
			r.pos += 2
			return
		}
		r.pos++
	}
}

func (r *bitReader) bits(k uint) int {
	if k == 0 {
		return 0
	}
	if r.n < k {
		r.fill()
	}
	v := int(r.acc >> (64 - k))
	r.acc <<= k
	r.n -= k
	return v
}

// decode reads one value of table h.
func (h *huffman) decode(r *bitReader) int {
	if r.n < 16 {
		r.fill()
	}
	if e := h.fast[r.acc>>(64-fastBits)]; e != 0 {
		l := uint(e & 0xff)
		r.acc <<= l
		r.n -= l
		return int(e >> 8)
	}
	p := int32(r.acc >> 48)
	for l := 1; l <= 16; l++ {
		c := p >> (16 - l)
		if c <= h.maxcode[l] {
			r.acc <<= uint(l)
			r.n -= uint(l)
			return int(h.vals[h.valptr[l]+c-h.mincode[l]])
		}
	}
	return -1
}

// decode decodes the scan into w*h*comps samples, pixel after pixel, the
// components of a pixel together.
func (j *ljpeg) decode() ([]uint16, error) {
	out := make([]uint16, j.w*j.h*j.comps)
	r := &bitReader{b: j.scan}
	c := j.comps
	stride := j.w * c
	first := uint16(1) << (j.bits - j.pt - 1)
	rowsPerInterval := j.h
	if j.restart > 0 {
		rowsPerInterval = j.restart / j.w
	}
	for y := range j.h {
		top := y%rowsPerInterval == 0
		if top && y > 0 {
			r.restart()
		}
		row := out[y*stride : (y+1)*stride]
		var above []uint16
		if !top {
			above = out[(y-1)*stride : y*stride]
		}
		for x := range j.w {
			for k := range c {
				t := j.huff[j.table[k]].decode(r)
				var diff int
				switch {
				case t < 0 || t > 16:
					return nil, fmt.Errorf("%w: lossless JPEG: a bad code in row %d", ErrFormat, y)
				case t == 16:
					diff = 32768
				case t > 0:
					diff = r.bits(uint(t))
					if diff < 1<<(t-1) {
						diff -= 1<<t - 1
					}
				}
				i := x*c + k
				var p int
				switch {
				case top && x == 0:
					p = int(first)
				case top:
					p = int(row[i-c])
				case x == 0:
					p = int(above[i])
				default:
					ra, rb, rc := int(row[i-c]), int(above[i]), int(above[i-c])
					switch j.pred {
					case 1:
						p = ra
					case 2:
						p = rb
					case 3:
						p = rc
					case 4:
						p = ra + rb - rc
					case 5:
						p = ra + (rb-rc)>>1
					case 6:
						p = rb + (ra-rc)>>1
					case 7:
						p = (ra + rb) >> 1
					}
				}
				row[i] = uint16(p + diff)
			}
		}
	}
	if j.pt > 0 {
		for i := range out {
			out[i] <<= j.pt
		}
	}
	return out, nil
}
