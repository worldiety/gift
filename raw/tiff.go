package raw

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
)

// The types of TIFF field values.
const (
	typeByte      = 1
	typeASCII     = 2
	typeShort     = 3
	typeLong      = 4
	typeRational  = 5
	typeSByte     = 6
	typeUndefined = 7
	typeSShort    = 8
	typeSLong     = 9
	typeSRational = 10
	typeFloat     = 11
	typeDouble    = 12
	typeIFD       = 13
)

// typeSize is the bytes one value of each type takes; zero for a type TIFF
// does not define.
var typeSize = [...]int{0, 1, 1, 2, 4, 8, 1, 1, 2, 4, 8, 4, 8, 4}

const (
	// maxValue bounds the bytes of one field. The largest real ones are a
	// camera's MakerNote and the tables of a DNG, well under a megabyte;
	// the bound is for a count that is garbage.
	maxValue = 16 << 20
	// maxEntries bounds the entries of one directory. Sony's SR2 block has
	// a few hundred.
	maxEntries = 4096
	// maxNumbers bounds the numbers a field is read as. The longest the
	// package reads are the tables of camera profiles, three numbers for
	// each of up to some ten thousand entries, and the offsets of strips
	// and tiles, a few thousand.
	maxNumbers = 1 << 20
)

// tiff reads the directories and fields of a TIFF structure through an
// io.ReaderAt, a field at a time, so that the metadata of a raw file is read
// without its tens of megabytes of pixels.
//
// Offsets are those of the file. base is where the bytes of r begin in it:
// zero for the file itself, the offset of Sony's SR2 block for the decrypted
// copy of it, whose fields address each other by their place in the file.
// end is the offset past the last byte r holds.
type tiff struct {
	r    io.ReaderAt
	base int64
	end  int64
	bo   binary.ByteOrder
}

// field is one entry of a directory: its tag, type, count, and the four
// bytes that are the value or the offset of it.
type field struct {
	tag, typ uint16
	count    uint32
	value    [4]byte
}

// dir is a directory read: its fields by tag.
type dir map[uint16]field

// header reads the byte order and the offset of the first directory from the
// eight bytes a TIFF file begins with.
func header(b []byte) (binary.ByteOrder, int64, bool) {
	if len(b) < 8 {
		return nil, 0, false
	}
	var bo binary.ByteOrder
	switch {
	case b[0] == 'I' && b[1] == 'I':
		bo = binary.LittleEndian
	case b[0] == 'M' && b[1] == 'M':
		bo = binary.BigEndian
	default:
		return nil, 0, false
	}
	if bo.Uint16(b[2:4]) != 42 {
		return nil, 0, false
	}
	return bo, int64(bo.Uint32(b[4:8])), true
}

// read reads n bytes at the file offset off.
func (t *tiff) read(off int64, n int) ([]byte, error) {
	if n < 0 || n > maxValue || off < t.base || off > t.end-int64(n) {
		return nil, fmt.Errorf("%w: %d bytes at %d lie outside the file", ErrFormat, n, off)
	}
	b := make([]byte, n)
	k, err := t.r.ReadAt(b, off-t.base)
	if k == n {
		return b, nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return nil, err
}

// dir reads the directory at off, and the offset of the next one, zero when
// there is none.
func (t *tiff) dir(off int64) (dir, int64, error) {
	h, err := t.read(off, 2)
	if err != nil {
		return nil, 0, err
	}
	n := int(t.bo.Uint16(h))
	if n == 0 || n > maxEntries {
		return nil, 0, fmt.Errorf("%w: a directory of %d entries at %d", ErrFormat, n, off)
	}
	b, err := t.read(off+2, 12*n)
	if err != nil {
		return nil, 0, err
	}
	d := make(dir, n)
	for i := range n {
		e := b[12*i:]
		f := field{tag: t.bo.Uint16(e), typ: t.bo.Uint16(e[2:]), count: t.bo.Uint32(e[4:])}
		copy(f.value[:], e[8:12])
		d[f.tag] = f
	}
	// A directory at the very end of a block may lack the pointer to the
	// next; that is the end of the chain, not an error.
	var next int64
	if p, err := t.read(off+2+12*int64(n), 4); err == nil {
		next = int64(t.bo.Uint32(p))
	}
	return d, next, nil
}

// bytes is the value of f: the bytes of the field itself when they fit in
// four, else those at the offset it holds.
func (t *tiff) bytes(f field) ([]byte, error) {
	if int(f.typ) >= len(typeSize) || typeSize[f.typ] == 0 {
		return nil, fmt.Errorf("%w: tag %#04x has type %d", ErrFormat, f.tag, f.typ)
	}
	n := int64(typeSize[f.typ]) * int64(f.count)
	if n > maxValue {
		return nil, fmt.Errorf("%w: tag %#04x has %d values", ErrFormat, f.tag, f.count)
	}
	if n <= 4 {
		return f.value[:n], nil
	}
	return t.read(int64(t.bo.Uint32(f.value[:])), int(n))
}

// offset is the single LONG or IFD value of f, which a pointer to a
// directory is; some writers make it an UNDEFINED or BYTE of four.
func (t *tiff) offset(f field) int64 {
	return int64(t.bo.Uint32(f.value[:]))
}

// ints is the value of an integer field, BYTE, SHORT, LONG and their signed
// forms, UNDEFINED read as bytes.
func (t *tiff) ints(f field) ([]int64, error) {
	if f.count > maxNumbers {
		return nil, fmt.Errorf("%w: tag %#04x has %d values", ErrFormat, f.tag, f.count)
	}
	b, err := t.bytes(f)
	if err != nil {
		return nil, err
	}
	v := make([]int64, f.count)
	for i := range v {
		switch f.typ {
		case typeByte, typeUndefined:
			v[i] = int64(b[i])
		case typeSByte:
			v[i] = int64(int8(b[i]))
		case typeShort:
			v[i] = int64(t.bo.Uint16(b[2*i:]))
		case typeSShort:
			v[i] = int64(int16(t.bo.Uint16(b[2*i:])))
		case typeLong, typeIFD:
			v[i] = int64(t.bo.Uint32(b[4*i:]))
		case typeSLong:
			v[i] = int64(int32(t.bo.Uint32(b[4*i:])))
		default:
			return nil, fmt.Errorf("%w: tag %#04x of type %d is not an integer", ErrFormat, f.tag, f.typ)
		}
	}
	return v, nil
}

// floats is the value of a numeric field of any type, rationals divided out.
// A rational over zero is zero.
func (t *tiff) floats(f field) ([]float64, error) {
	switch f.typ {
	case typeRational, typeSRational, typeFloat, typeDouble:
	default:
		is, err := t.ints(f)
		if err != nil {
			return nil, err
		}
		v := make([]float64, len(is))
		for i, x := range is {
			v[i] = float64(x)
		}
		return v, nil
	}
	if f.count > maxNumbers {
		return nil, fmt.Errorf("%w: tag %#04x has %d values", ErrFormat, f.tag, f.count)
	}
	b, err := t.bytes(f)
	if err != nil {
		return nil, err
	}
	v := make([]float64, f.count)
	for i := range v {
		switch f.typ {
		case typeRational:
			if d := t.bo.Uint32(b[8*i+4:]); d != 0 {
				v[i] = float64(t.bo.Uint32(b[8*i:])) / float64(d)
			}
		case typeSRational:
			if d := int32(t.bo.Uint32(b[8*i+4:])); d != 0 {
				v[i] = float64(int32(t.bo.Uint32(b[8*i:]))) / float64(d)
			}
		case typeFloat:
			v[i] = float64(math.Float32frombits(t.bo.Uint32(b[4*i:])))
		case typeDouble:
			v[i] = math.Float64frombits(t.bo.Uint64(b[8*i:]))
		}
	}
	return v, nil
}

// str is the value of an ASCII field, up to its first NUL, without the
// spaces cameras pad with.
func (t *tiff) str(f field) string {
	if f.typ != typeASCII && f.typ != typeByte && f.typ != typeUndefined {
		return ""
	}
	b, err := t.bytes(f)
	if err != nil {
		return ""
	}
	s := string(b)
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// The lookups below answer the value of a tag of d, or ok false when d does
// not have it or it cannot be read; a raw file is read as far as it makes
// sense, and a field that is garbage is a field that is missing.

func (t *tiff) int(d dir, tag uint16) (int64, bool) {
	f, ok := d[tag]
	if !ok {
		return 0, false
	}
	v, err := t.ints(f)
	if err != nil || len(v) == 0 {
		return 0, false
	}
	return v[0], true
}

func (t *tiff) intsOf(d dir, tag uint16) []int64 {
	f, ok := d[tag]
	if !ok {
		return nil
	}
	v, _ := t.ints(f)
	return v
}

func (t *tiff) floatsOf(d dir, tag uint16) []float64 {
	f, ok := d[tag]
	if !ok {
		return nil
	}
	v, _ := t.floats(f)
	return v
}

func (t *tiff) strOf(d dir, tag uint16) string {
	f, ok := d[tag]
	if !ok {
		return ""
	}
	return t.str(f)
}
