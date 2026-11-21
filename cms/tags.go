package cms

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unicode/utf16"
)

// Tag and type signatures, ICC specification sections 9 and 10.
const (
	tagDesc  = 'd'<<24 | 'e'<<16 | 's'<<8 | 'c'
	tagCprt  = 'c'<<24 | 'p'<<16 | 'r'<<8 | 't'
	tagWtpt  = 'w'<<24 | 't'<<16 | 'p'<<8 | 't'
	tagChad  = 'c'<<24 | 'h'<<16 | 'a'<<8 | 'd'
	tagRXYZ  = 'r'<<24 | 'X'<<16 | 'Y'<<8 | 'Z'
	tagGXYZ  = 'g'<<24 | 'X'<<16 | 'Y'<<8 | 'Z'
	tagBXYZ  = 'b'<<24 | 'X'<<16 | 'Y'<<8 | 'Z'
	tagRTRC  = 'r'<<24 | 'T'<<16 | 'R'<<8 | 'C'
	tagGTRC  = 'g'<<24 | 'T'<<16 | 'R'<<8 | 'C'
	tagBTRC  = 'b'<<24 | 'T'<<16 | 'R'<<8 | 'C'
	tagKTRC  = 'k'<<24 | 'T'<<16 | 'R'<<8 | 'C'
	tagA2B0  = 'A'<<24 | '2'<<16 | 'B'<<8 | '0'
	tagA2B1  = 'A'<<24 | '2'<<16 | 'B'<<8 | '1'
	tagA2B2  = 'A'<<24 | '2'<<16 | 'B'<<8 | '2'
	tagB2A0  = 'B'<<24 | '2'<<16 | 'A'<<8 | '0'
	tagB2A1  = 'B'<<24 | '2'<<16 | 'A'<<8 | '1'
	tagB2A2  = 'B'<<24 | '2'<<16 | 'A'<<8 | '2'
	typeXYZ  = 'X'<<24 | 'Y'<<16 | 'Z'<<8 | ' '
	typeCurv = 'c'<<24 | 'u'<<16 | 'r'<<8 | 'v'
	typePara = 'p'<<24 | 'a'<<16 | 'r'<<8 | 'a'
	typeMft1 = 'm'<<24 | 'f'<<16 | 't'<<8 | '1'
	typeMft2 = 'm'<<24 | 'f'<<16 | 't'<<8 | '2'
	typeMAB  = 'm'<<24 | 'A'<<16 | 'B'<<8 | ' '
	typeMBA  = 'm'<<24 | 'B'<<16 | 'A'<<8 | ' '
	typeDesc = 'd'<<24 | 'e'<<16 | 's'<<8 | 'c'
	typeMluc = 'm'<<24 | 'l'<<16 | 'u'<<8 | 'c'
	typeText = 't'<<24 | 'e'<<16 | 'x'<<8 | 't'
	typeSf32 = 's'<<24 | 'f'<<16 | '3'<<8 | '2'
)

var errShort = errors.New("cms: tag too short")

func u16(b []byte) uint16 { return binary.BigEndian.Uint16(b) }
func u32(b []byte) uint32 { return binary.BigEndian.Uint32(b) }

// s15f16 reads an s15Fixed16Number.
func s15f16(b []byte) float64 { return float64(int32(u32(b))) / 65536 }

func typeOf(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return u32(b)
}

// parseXYZ reads an XYZType tag with one value.
func parseXYZ(b []byte) (XYZ, error) {
	if typeOf(b) != typeXYZ || len(b) < 20 {
		return XYZ{}, fmt.Errorf("cms: not an XYZ tag")
	}
	return XYZ{s15f16(b[8:]), s15f16(b[12:]), s15f16(b[16:])}, nil
}

// parseSf32 reads an s15Fixed16ArrayType tag.
func parseSf32(b []byte) ([]float64, error) {
	if typeOf(b) != typeSf32 {
		return nil, fmt.Errorf("cms: not an sf32 tag")
	}
	b = b[8:]
	r := make([]float64, len(b)/4)
	for i := range r {
		r[i] = s15f16(b[4*i:])
	}
	return r, nil
}

// parseCurve reads a curveType or parametricCurveType at the start of b
// and reports how many bytes it took, padded to four as the curves of an
// mAB or mBA follow each other.
func parseCurve(b []byte) (Curve, int, error) {
	if len(b) < 12 {
		return Curve{}, 0, errShort
	}
	switch typeOf(b) {
	case typeCurv:
		n := int(u32(b[8:]))
		if n < 0 || n > (len(b)-12)/2 {
			return Curve{}, 0, errShort
		}
		if n > 65535 {
			// More samples than a 16 bit curve has values: nothing a real
			// profile needs, and a way to make every transform slow.
			return Curve{}, 0, fmt.Errorf("cms: curve of %d samples", n)
		}
		size := 12 + 2*n
		switch n {
		case 0:
			return Linear(), pad4(size), nil
		case 1:
			return Gamma(float64(u16(b[12:])) / 256), pad4(size), nil
		}
		t := make([]uint16, n)
		for i := range t {
			t[i] = u16(b[12+2*i:])
		}
		return Table(t), pad4(size), nil
	case typePara:
		kind := int(u16(b[8:]))
		np := paramCount(kind)
		if np == 0 {
			return Curve{}, 0, fmt.Errorf("cms: parametric curve type %d", kind)
		}
		size := 12 + 4*np
		if len(b) < size {
			return Curve{}, 0, errShort
		}
		p := make([]float64, np)
		for i := range p {
			p[i] = s15f16(b[12+4*i:])
		}
		return Parametric(kind, p...), pad4(size), nil
	}
	return Curve{}, 0, fmt.Errorf("cms: %s is no curve", sigString(typeOf(b)))
}

func pad4(n int) int { return (n + 3) &^ 3 }

// parseCurves reads n curves following each other.
func parseCurves(b []byte, n int) ([]curve, error) {
	cs := make([]curve, n)
	for i := range n {
		c, size, err := parseCurve(b)
		if err != nil {
			return nil, err
		}
		if cs[i], err = c.compile(); err != nil {
			return nil, err
		}
		b = b[size:]
	}
	return cs, nil
}

// parseText reads a description, whatever of the three text types it is:
// textDescriptionType (v2), multiLocalizedUnicodeType (v4) or textType.
// Of a localized one it takes the English record, else the first.
func parseText(b []byte) string {
	switch typeOf(b) {
	case typeDesc:
		if len(b) < 12 {
			return ""
		}
		n := int(u32(b[8:]))
		if n > len(b)-12 {
			n = len(b) - 12
		}
		s := b[12 : 12+max(n, 0)]
		for len(s) > 0 && s[len(s)-1] == 0 {
			s = s[:len(s)-1]
		}
		return string(s)
	case typeText:
		s := b[8:]
		for len(s) > 0 && s[len(s)-1] == 0 {
			s = s[:len(s)-1]
		}
		return string(s)
	case typeMluc:
		if len(b) < 16 {
			return ""
		}
		n, size := int(u32(b[8:])), int(u32(b[12:]))
		best := -1
		for i := range n {
			rec := 16 + i*size
			if size < 12 || rec+12 > len(b) {
				break
			}
			if best < 0 || string(b[rec:rec+2]) == "en" {
				best = rec
				if string(b[rec:rec+2]) == "en" {
					break
				}
			}
		}
		if best < 0 {
			return ""
		}
		l, off := int(u32(b[best+4:])), int(u32(b[best+8:]))
		if off < 0 || l < 0 || off+l > len(b) {
			return ""
		}
		u := make([]uint16, l/2)
		for i := range u {
			u[i] = u16(b[off+2*i:])
		}
		return string(utf16.Decode(u))
	}
	return ""
}

// lutTag is a parsed lut8, lut16, mAB or mBA tag: a pipeline from its
// input encoding to its output encoding, each from 0 to 1.
type lutTag struct {
	stages pipeline
	// lut16 is set for a lut16Type, whose Lab uses the v2 encoding.
	lut16 bool
}

// parseLut reads any of the four LUT types; out is the number of channels
// the tag must produce.
func parseLut(b []byte, in, out int) (lutTag, error) {
	switch typeOf(b) {
	case typeMft1, typeMft2:
		return parseMft(b, in, out)
	case typeMAB, typeMBA:
		return parseModular(b, in, out)
	}
	return lutTag{}, fmt.Errorf("cms: %s is no LUT type", sigString(typeOf(b)))
}

// parseMft reads lut8Type and lut16Type: matrix, input curves, table,
// output curves, ICC specification 10.10 and 10.11.
func parseMft(b []byte, in, out int) (lutTag, error) {
	if len(b) < 48 {
		return lutTag{}, errShort
	}
	is16 := typeOf(b) == typeMft2
	tin, tout, g := int(b[8]), int(b[9]), int(b[10])
	if tin != in || tout != out {
		return lutTag{}, fmt.Errorf("cms: LUT of %d to %d channels where %d to %d are needed", tin, tout, in, out)
	}
	if g < 2 {
		return lutTag{}, fmt.Errorf("cms: LUT grid of %d points", g)
	}
	var m mat3
	for i := range 9 {
		m[i/3][i%3] = s15f16(b[12+4*i:])
	}
	inEntries, outEntries, width, rest := 256, 256, 1, b[48:]
	if is16 {
		if len(b) < 52 {
			return lutTag{}, errShort
		}
		inEntries, outEntries, width, rest = int(u16(b[48:])), int(u16(b[50:])), 2, b[52:]
		if inEntries < 2 || outEntries < 2 {
			return lutTag{}, fmt.Errorf("cms: LUT curves of %d and %d entries", inEntries, outEntries)
		}
	}
	read := func(n int) ([]uint16, error) {
		if len(rest) < n*width {
			return nil, errShort
		}
		t := make([]uint16, n)
		for i := range t {
			if is16 {
				t[i] = u16(rest[2*i:])
			} else {
				t[i] = uint16(rest[i]) * 257
			}
		}
		rest = rest[n*width:]
		return t, nil
	}
	var p pipeline
	// The matrix only applies to three input channels, and LittleCMS only
	// applies it when it is not the identity (cmstypes.c,
	// Type_LUT16_Read).
	if in == 3 && !m.isIdentity(1e-9) {
		p = append(p, affine{m: m})
	}
	ins := make([]curve, in)
	for i := range ins {
		t, err := read(inEntries)
		if err != nil {
			return lutTag{}, err
		}
		ins[i] = newSampled(t)
	}
	if s := newCurves(ins, false); s != nil {
		p = append(p, s)
	}
	n := out
	grid := make([]int, in)
	for i := range grid {
		grid[i] = g
		// Checked as it grows, so that it cannot overflow before the read
		// finds the tag too short.
		if n *= g; n > len(rest) {
			return lutTag{}, errShort
		}
	}
	t, err := read(n)
	if err != nil {
		return lutTag{}, err
	}
	samples := make([]float64, n)
	for i, v := range t {
		samples[i] = float64(v) / 65535
	}
	p = append(p, newCLUT(grid, out, samples))
	outs := make([]curve, out)
	for i := range outs {
		t, err := read(outEntries)
		if err != nil {
			return lutTag{}, err
		}
		outs[i] = newSampled(t)
	}
	if s := newCurves(outs, false); s != nil {
		p = append(p, s)
	}
	return lutTag{stages: p, lut16: is16}, nil
}

// parseModular reads lutAToBType and lutBToAType, ICC specification
// 10.12 and 10.13: A curves, table, M curves, matrix, B curves, in this
// order for A to B and the reverse for B to A. Every element is optional.
func parseModular(b []byte, in, out int) (lutTag, error) {
	if len(b) < 32 {
		return lutTag{}, errShort
	}
	aToB := typeOf(b) == typeMAB
	tin, tout := int(b[8]), int(b[9])
	if tin != in || tout != out {
		return lutTag{}, fmt.Errorf("cms: LUT of %d to %d channels where %d to %d are needed", tin, tout, in, out)
	}
	offB, offMat, offM, offCLUT, offA := int(u32(b[12:])), int(u32(b[16:])), int(u32(b[20:])), int(u32(b[24:])), int(u32(b[28:]))
	at := func(off int) ([]byte, error) {
		if off < 32 || off >= len(b) {
			return nil, errShort
		}
		return b[off:], nil
	}
	// The A curves and the table are on the device side, the M curves, the
	// matrix and the B curves on the side of the connection space.
	dev, pcs := in, out
	if !aToB {
		dev, pcs = out, in
	}
	readCurves := func(off, n int) (stage, error) {
		if off == 0 {
			return nil, nil
		}
		at, err := at(off)
		if err != nil {
			return nil, err
		}
		cs, err := parseCurves(at, n)
		if err != nil {
			return nil, err
		}
		return newCurves(cs, false), nil
	}
	a, err := readCurves(offA, dev)
	if err != nil {
		return lutTag{}, err
	}
	m, err := readCurves(offM, pcs)
	if err != nil {
		return lutTag{}, err
	}
	bc, err := readCurves(offB, pcs)
	if err != nil {
		return lutTag{}, err
	}
	var table stage
	if offCLUT != 0 {
		c, err := at(offCLUT)
		if err != nil {
			return lutTag{}, err
		}
		cin, cout := dev, pcs
		if !aToB {
			cin, cout = pcs, dev
		}
		if table, err = parseCLUT(c, cin, cout); err != nil {
			return lutTag{}, err
		}
	}
	var matrix stage
	if offMat != 0 && pcs == 3 {
		mb, err := at(offMat)
		if err != nil {
			return lutTag{}, err
		}
		if len(mb) < 48 {
			return lutTag{}, errShort
		}
		var mt affine
		for i := range 9 {
			mt.m[i/3][i%3] = s15f16(mb[4*i:])
		}
		for i := range 3 {
			mt.off[i] = s15f16(mb[36+4*i:])
		}
		if !mt.isIdentity() {
			matrix = mt
		}
	}
	elems := []stage{a, table, m, matrix, bc}
	if !aToB {
		elems = []stage{bc, matrix, m, table, a}
	}
	var p pipeline
	for _, s := range elems {
		if s != nil {
			p = append(p, s)
		}
	}
	if !p.fits(in, out) {
		return lutTag{}, fmt.Errorf("cms: LUT elements do not fit %d to %d channels", in, out)
	}
	return lutTag{stages: p}, nil
}

// parseCLUT reads the table of an mAB or mBA: the grid points per input
// channel, the precision, and the samples.
func parseCLUT(b []byte, in, out int) (*clut, error) {
	if len(b) < 20 || in > 4 {
		return nil, errShort
	}
	grid := make([]int, in)
	n := out
	for i := range grid {
		grid[i] = int(b[i])
		if grid[i] < 2 {
			return nil, fmt.Errorf("cms: LUT grid of %d points", grid[i])
		}
		if n *= grid[i]; n > len(b) {
			return nil, errShort
		}
	}
	width := int(b[16])
	if width != 1 && width != 2 {
		return nil, fmt.Errorf("cms: LUT precision of %d bytes", width)
	}
	b = b[20:]
	if len(b) < n*width {
		return nil, errShort
	}
	samples := make([]float64, n)
	for i := range samples {
		if width == 2 {
			samples[i] = float64(u16(b[2*i:])) / 65535
		} else {
			samples[i] = float64(b[i]) / 255
		}
	}
	return newCLUT(grid, out, samples), nil
}

// finite reports whether every number of x is.
func finite(x ...float64) bool {
	for _, v := range x {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}
