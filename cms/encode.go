package cms

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"math"
	"unicode/utf16"
)

// NewRGBProfile makes an ICC v4 display profile of an RGB space from its
// primaries, its white and the curve all three channels share. The
// profile's Bytes can be embedded in a file like any other.
//
// The colorants are adapted from white to D50 with the Bradford transform
// and the adaptation is stored in a chad tag, as the specification wants
// and LittleCMS does it.
func NewRGBProfile(description string, white Chromaticity, primaries [3]Chromaticity, trc Curve) (*Profile, error) {
	if !finite(white.X, white.Y, primaries[0].X, primaries[0].Y, primaries[1].X, primaries[1].Y, primaries[2].X, primaries[2].Y) || white.Y <= 0 {
		return nil, errors.New("cms: primaries or white out of range")
	}
	m, ok := rgbToXYZ(white, primaries)
	if !ok {
		return nil, errors.New("cms: primaries do not span a space")
	}
	chad := adaptation(white.XYZ(), D50)
	m = chad.mul(m)
	c, err := curveTag(trc)
	if err != nil {
		return nil, err
	}
	tags := []tag{
		{tagDesc, mluc(description)},
		{tagCprt, mluc("No copyright, use freely")},
		{tagWtpt, xyzTag(D50)},
		{tagChad, sf32Tag(chad)},
		{tagRXYZ, xyzTag(XYZ{m[0][0], m[1][0], m[2][0]})},
		{tagGXYZ, xyzTag(XYZ{m[0][1], m[1][1], m[2][1]})},
		{tagBXYZ, xyzTag(XYZ{m[0][2], m[1][2], m[2][2]})},
		{tagRTRC, c}, {tagGTRC, c}, {tagBTRC, c},
	}
	return ParseProfile(writeProfile(ClassDisplay, SpaceRGB, tags))
}

// NewGrayProfile makes an ICC v4 display profile of a gray space from its
// white and its curve.
func NewGrayProfile(description string, white Chromaticity, trc Curve) (*Profile, error) {
	if !finite(white.X, white.Y) || white.Y <= 0 {
		return nil, errors.New("cms: white out of range")
	}
	c, err := curveTag(trc)
	if err != nil {
		return nil, err
	}
	tags := []tag{
		{tagDesc, mluc(description)},
		{tagCprt, mluc("No copyright, use freely")},
		{tagWtpt, xyzTag(D50)},
		{tagChad, sf32Tag(adaptation(white.XYZ(), D50))},
		{tagKTRC, c},
	}
	return ParseProfile(writeProfile(ClassDisplay, SpaceGray, tags))
}

type tag struct {
	sig  uint32
	data []byte
}

// writeProfile lays out an ICC v4.3 profile that connects through XYZ.
func writeProfile(class Class, space Space, tags []tag) []byte {
	return writeProfileVersion(0x04300000, class, space, SpaceXYZ, tags)
}

// writeProfileVersion lays out a profile: header, tag table, and the tags,
// each on a four byte boundary and the same data only once. The profile ID
// is the MD5 sum the specification defines, section 7.2.18.
func writeProfileVersion(version uint32, class Class, space, pcs Space, tags []tag) []byte {
	b := make([]byte, 132+12*len(tags))
	be := binary.BigEndian
	be.PutUint32(b[8:], version)
	be.PutUint32(b[12:], uint32(class))
	be.PutUint32(b[16:], uint32(space))
	be.PutUint32(b[20:], uint32(pcs))
	// A fixed date, so that a profile made twice is the same bytes.
	for i, v := range []uint16{2026, 1, 1, 0, 0, 0} {
		be.PutUint16(b[24+2*i:], v)
	}
	copy(b[36:], "acsp")
	copy(b[68:], xyzTag(D50)[8:])
	be.PutUint32(b[128:], uint32(len(tags)))
	written := map[string]uint32{}
	for i, t := range tags {
		off, ok := written[string(t.data)]
		if !ok {
			for len(b)%4 != 0 {
				b = append(b, 0)
			}
			off = uint32(len(b))
			written[string(t.data)] = off
			b = append(b, t.data...)
		}
		e := b[132+12*i:]
		be.PutUint32(e, t.sig)
		be.PutUint32(e[4:], off)
		be.PutUint32(e[8:], uint32(len(t.data)))
	}
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	be.PutUint32(b, uint32(len(b)))
	sum := md5.Sum(b)
	copy(b[84:], sum[:])
	return b
}

func s15f16Bytes(v float64) []byte {
	return binary.BigEndian.AppendUint32(nil, uint32(int32(math.Round(v*65536))))
}

func sigBytes(s uint32) []byte { return binary.BigEndian.AppendUint32(nil, s) }

func xyzTag(c XYZ) []byte {
	b := append(sigBytes(typeXYZ), 0, 0, 0, 0)
	for _, v := range []float64{c.X, c.Y, c.Z} {
		b = append(b, s15f16Bytes(v)...)
	}
	return b
}

func sf32Tag(m mat3) []byte {
	b := append(sigBytes(typeSf32), 0, 0, 0, 0)
	for _, r := range m {
		for _, v := range r {
			b = append(b, s15f16Bytes(v)...)
		}
	}
	return b
}

// mluc is a multiLocalizedUnicodeType with one English record.
func mluc(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := append(sigBytes(typeMluc), 0, 0, 0, 0)
	b = binary.BigEndian.AppendUint32(b, 1)
	b = binary.BigEndian.AppendUint32(b, 12)
	b = append(b, "enUS"...)
	b = binary.BigEndian.AppendUint32(b, uint32(2*len(u)))
	b = binary.BigEndian.AppendUint32(b, 28)
	for _, c := range u {
		b = binary.BigEndian.AppendUint16(b, c)
	}
	return b
}

// curveTag encodes a curve as parametricCurveType, or curveType for a
// table.
func curveTag(c Curve) ([]byte, error) {
	if _, err := c.compile(); err != nil {
		return nil, err
	}
	if c.kind < 0 {
		b := append(sigBytes(typeCurv), 0, 0, 0, 0)
		b = binary.BigEndian.AppendUint32(b, uint32(len(c.table)))
		for _, v := range c.table {
			b = binary.BigEndian.AppendUint16(b, v)
		}
		return b, nil
	}
	b := append(sigBytes(typePara), 0, 0, 0, 0)
	b = binary.BigEndian.AppendUint16(b, uint16(c.kind))
	b = append(b, 0, 0)
	for _, v := range c.params[:paramCount(c.kind)] {
		b = append(b, s15f16Bytes(v)...)
	}
	return b, nil
}
