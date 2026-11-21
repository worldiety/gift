package asset

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/worldiety/gift/cms"
)

// maxProfile bounds the ICC profile a picture may carry. Real ones are a few
// hundred bytes for a display and a few hundred kilobytes for a printer; the
// bound is for the compressed iCCP chunk of a PNG, which could otherwise
// inflate to whatever its author liked.
const maxProfile = 8 << 20

// ReadProfile returns the colour profile an encoded picture declares its
// numbers to be in, or nil when it declares none, which means sRGB.
//
// It reads the head of the file and decodes no pixels:
//
//   - JPEG: an ICC profile in APP2 segments, split over several if it is
//     large. Without one, the EXIF of a camera set to Adobe RGB: those write
//     no profile but the interoperability index R03 of the DCF standard, or
//     the colour space 2 some makers use instead.
//   - PNG: the chunks cICP, iCCP, sRGB, and gAMA with cHRM, in the order of
//     precedence the third edition of the PNG specification gives them.
//     cICP values that are not understood, the HDR transfers PQ and HLG
//     among them, fall through to the next.
//   - WebP: an ICC profile in an ICCP chunk.
//
// A file that declares sRGB in any of these ways yields [cms.SRGB], or
// [cms.SGray] for a gray one. An error is a declaration that cannot be
// read, such as a profile cut short; the pipeline then shows the picture as
// sRGB, as it does one that declares nothing.
func ReadProfile(encoded []byte) (*cms.Profile, error) {
	d, err := declaredColour(encoded)
	return d.profile, err
}

// declared is what a picture says about its colours.
type declared struct {
	// profile is the declared profile, nil for none.
	profile *cms.Profile
	// stored is what the stored pixels are, gray, RGB or CMYK, as the file
	// itself says: the components of a JPEG frame, the colour type of a PNG.
	// Zero when the container does not say.
	stored cms.Space
	// from names the declaration, for the log.
	from string
}

// fits reports whether the profile can describe the stored pixels. An RGB
// profile on gray pixels does, as neutral RGB: software that embeds its
// working space in every file it writes does that to gray ones too.
func (d declared) fits() bool {
	if d.profile == nil {
		return false
	}
	s := d.profile.Space()
	switch {
	case s == d.stored:
		return true
	case s == cms.SpaceRGB:
		return d.stored == 0 || d.stored == cms.SpaceGray
	}
	return false
}

func declaredColour(b []byte) (declared, error) {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return jpegColour(b)
	case len(b) >= 8 && string(b[:8]) == pngSignature:
		return pngColour(b)
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return webpColour(b)
	}
	return declared{}, nil
}

const pngSignature = "\x89PNG\r\n\x1a\n"

// --- JPEG --------------------------------------------------------------------

// jpegSegments calls f with the marker of every segment before the first
// scan and where it lies: b[start:end] is the whole segment, marker and
// length included, and b[start+4:end] its payload. It stops early when f
// returns false, and returns where it stopped, which for a well formed file
// is the start of the first scan.
func jpegSegments(b []byte, f func(marker byte, start, end int) bool) int {
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return i
		}
		m := b[i+1]
		switch {
		case m == 0xFF: // fill byte
			i++
			continue
		case m == 0x01 || m == 0xD8 || (m >= 0xD0 && m <= 0xD7): // no length
			i += 2
			continue
		case m == 0xDA || m == 0xD9:
			return i
		}
		size := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		end := i + 2 + size
		if size < 2 || end > len(b) {
			return i
		}
		if !f(m, i, end) {
			return i
		}
		i = end
	}
	return i
}

const iccMarker = "ICC_PROFILE\x00"

func jpegColour(b []byte) (declared, error) {
	var (
		d      declared
		parts  [][]byte
		broken error
		exif   []byte
	)
	jpegSegments(b, func(m byte, start, end int) bool {
		seg := b[start+4 : end]
		switch {
		case m == 0xE2 && len(seg) >= 14 && string(seg[:12]) == iccMarker:
			seq, n := int(seg[12]), int(seg[13])
			switch {
			case seq == 0 || n == 0 || seq > n:
				broken = fmt.Errorf("asset: ICC profile part %d of %d", seq, n)
			case parts == nil:
				parts = make([][]byte, n)
			case len(parts) != n:
				broken = errors.New("asset: ICC profile parts disagree on their number")
			}
			if broken == nil {
				if parts[seq-1] != nil {
					broken = fmt.Errorf("asset: ICC profile part %d twice", seq)
				} else {
					parts[seq-1] = seg[14:]
				}
			}
		case m == 0xE1 && exif == nil && len(seg) > 6 && string(seg[:6]) == "Exif\x00\x00":
			exif = seg[6:]
		case m >= 0xC0 && m <= 0xCF && m != 0xC4 && m != 0xC8 && m != 0xCC:
			// A start of frame: precision, height, width, components.
			if len(seg) >= 6 {
				d.stored = componentSpace(seg[5])
			}
		}
		return true
	})
	if broken != nil {
		return d, broken
	}
	if parts != nil {
		var icc []byte
		for i, p := range parts {
			if p == nil {
				return d, fmt.Errorf("asset: ICC profile part %d of %d missing", i+1, len(parts))
			}
			icc = append(icc, p...)
		}
		p, err := cms.ParseProfile(icc)
		if err != nil {
			return d, err
		}
		d.profile, d.from = p, "ICC profile"
		return d, nil
	}
	if exif != nil && exifAdobeRGB(exif) {
		d.profile, d.from = cms.AdobeRGB(), "EXIF, Adobe RGB"
	}
	return d, nil
}

// componentSpace is what a JPEG frame of n components holds.
func componentSpace(n byte) cms.Space {
	switch n {
	case 1:
		return cms.SpaceGray
	case 3:
		return cms.SpaceRGB
	case 4:
		return cms.SpaceCMYK
	}
	return 0
}

// exifAdobeRGB reports whether the EXIF of a JPEG says Adobe RGB the way
// cameras do: the colour space tag says uncalibrated and the
// interoperability index is R03, the DCF option file; or, as some makers
// write it, the colour space tag says 2. A colour space of 1, sRGB, wins
// over everything else.
func exifAdobeRGB(tiff []byte) bool {
	t, ok := newTiff(tiff)
	if !ok {
		return false
	}
	ifd0, _ := t.ifd(t.first)
	var sub uint32
	for _, e := range ifd0 {
		if e.tag == 0x8769 {
			sub = e.u32(t)
		}
	}
	if sub == 0 {
		return false
	}
	es, _ := t.ifd(sub)
	var cs, interop uint32
	for _, e := range es {
		switch e.tag {
		case 0xA001:
			cs = e.u32(t)
		case 0xA005:
			interop = e.u32(t)
		}
	}
	switch cs {
	case 1:
		return false
	case 2:
		return true
	}
	if interop == 0 {
		return false
	}
	ies, _ := t.ifd(interop)
	for _, e := range ies {
		if e.tag == 0x0001 {
			return e.ascii(t) == "R03"
		}
	}
	return false
}

// --- PNG ---------------------------------------------------------------------

func pngColour(b []byte) (declared, error) {
	var (
		d                            declared
		cicp, iccp, srgb, gama, chrm []byte
	)
	for i := 8; i+12 <= len(b); {
		n := binary.BigEndian.Uint32(b[i:])
		if uint64(n) > uint64(len(b)-i-12) {
			break
		}
		data := b[i+8 : i+8+int(n)]
		typ := string(b[i+4 : i+8])
		if typ == "IDAT" || typ == "IEND" {
			break
		}
		switch typ {
		case "IHDR":
			if len(data) >= 13 {
				switch data[9] {
				case 0, 4:
					d.stored = cms.SpaceGray
				case 2, 3, 6:
					d.stored = cms.SpaceRGB
				}
			}
		case "cICP":
			cicp = data
		case "iCCP":
			iccp = data
		case "sRGB":
			srgb = data
		case "gAMA":
			gama = data
		case "cHRM":
			chrm = data
		}
		i += 12 + int(n)
	}
	gray := d.stored == cms.SpaceGray

	if p := cicpProfile(cicp, gray); p != nil {
		d.profile, d.from = p, "cICP"
		return d, nil
	}
	if iccp != nil {
		icc, err := inflateICCP(iccp)
		if err != nil {
			return d, err
		}
		p, err := cms.ParseProfile(icc)
		if err != nil {
			return d, err
		}
		d.profile, d.from = p, "iCCP"
		return d, nil
	}
	if srgb != nil {
		d.profile, d.from = standard(gray), "sRGB chunk"
		return d, nil
	}
	p, err := gamaProfile(gama, chrm, gray)
	if p != nil {
		d.profile, d.from = p, "gAMA and cHRM"
	}
	return d, err
}

// standard is sRGB, or its gray for a gray picture.
func standard(gray bool) *cms.Profile {
	if gray {
		return cms.SGray()
	}
	return cms.SRGB()
}

// inflateICCP reads the profile of an iCCP chunk: a name of 1 to 79 bytes,
// a zero, the compression method, which must be 0, and a zlib stream.
func inflateICCP(data []byte) ([]byte, error) {
	z := bytes.IndexByte(data, 0)
	if z < 1 || z > 79 || z+2 > len(data) || data[z+1] != 0 {
		return nil, errors.New("asset: iCCP chunk malformed")
	}
	r, err := zlib.NewReader(bytes.NewReader(data[z+2:]))
	if err != nil {
		return nil, fmt.Errorf("asset: iCCP chunk: %w", err)
	}
	defer r.Close()
	icc, err := io.ReadAll(io.LimitReader(r, maxProfile+1))
	if err != nil {
		return nil, fmt.Errorf("asset: iCCP chunk: %w", err)
	}
	if len(icc) > maxProfile {
		return nil, errors.New("asset: iCCP profile too large")
	}
	return icc, nil
}

// cicpPrimaries are the colour primaries of ITU-T H.273 that a PNG may
// name: red, green, blue and white.
var cicpPrimaries = map[byte][4]cms.Chromaticity{
	1:  {xy(0.64, 0.33), xy(0.30, 0.60), xy(0.15, 0.06), cms.WhiteD65},           // BT.709, sRGB
	4:  {xy(0.67, 0.33), xy(0.21, 0.71), xy(0.14, 0.08), xy(0.310, 0.316)},       // BT.470 M, illuminant C
	5:  {xy(0.64, 0.33), xy(0.29, 0.60), xy(0.15, 0.06), cms.WhiteD65},           // BT.601 625 lines
	6:  {xy(0.630, 0.340), xy(0.310, 0.595), xy(0.155, 0.070), cms.WhiteD65},     // BT.601 525 lines
	7:  {xy(0.630, 0.340), xy(0.310, 0.595), xy(0.155, 0.070), cms.WhiteD65},     // SMPTE 240M
	9:  {xy(0.708, 0.292), xy(0.170, 0.797), xy(0.131, 0.046), cms.WhiteD65},     // BT.2020
	11: {xy(0.680, 0.320), xy(0.265, 0.690), xy(0.150, 0.060), xy(0.314, 0.351)}, // DCI-P3
	12: {xy(0.680, 0.320), xy(0.265, 0.690), xy(0.150, 0.060), cms.WhiteD65},     // Display P3
	22: {xy(0.630, 0.340), xy(0.295, 0.605), xy(0.155, 0.077), cms.WhiteD65},     // EBU 3213
}

func xy(x, y float64) cms.Chromaticity { return cms.Chromaticity{X: x, Y: y} }

// cicpTransfer is the curve of an H.273 transfer characteristic, or false
// for one this package does not convert: PQ and HLG are for HDR displays
// and would need a tone mapping to be shown on any other.
func cicpTransfer(t byte) (cms.Curve, bool) {
	switch t {
	case 1, 6, 14, 15:
		return cms.Rec709Curve(), true
	case 4:
		return cms.Gamma(2.2), true
	case 5:
		return cms.Gamma(2.8), true
	case 8:
		return cms.Linear(), true
	case 13:
		return cms.SRGBCurve(), true
	}
	return cms.Curve{}, false
}

// cicpProfile is the profile of a cICP chunk, or nil when it is absent or
// names something this package does not convert. A PNG holds RGB, so the
// matrix coefficients must be 0 and the range full.
func cicpProfile(c []byte, gray bool) *cms.Profile {
	if len(c) != 4 || c[2] != 0 || c[3] != 1 {
		return nil
	}
	pr, ok := cicpPrimaries[c[0]]
	if !ok {
		return nil
	}
	tc, ok := cicpTransfer(c[1])
	if !ok {
		return nil
	}
	name := fmt.Sprintf("cICP %d/%d", c[0], c[1])
	if gray {
		if c[1] == 13 {
			return cms.SGray()
		}
		p, _ := cms.NewGrayProfile(name, pr[3], tc)
		return p
	}
	switch [2]byte{c[0], c[1]} {
	case [2]byte{1, 13}:
		return cms.SRGB()
	case [2]byte{12, 13}:
		return cms.DisplayP3()
	case [2]byte{9, 1}, [2]byte{9, 14}, [2]byte{9, 15}:
		return cms.Rec2020()
	case [2]byte{1, 8}:
		return cms.LinearSRGB()
	case [2]byte{9, 8}:
		return cms.LinearRec2020()
	}
	p, _ := cms.NewRGBProfile(name, pr[3], [3]cms.Chromaticity{pr[0], pr[1], pr[2]}, tc)
	return p
}

// gamaProfile is the profile of a gAMA chunk, with the primaries of a cHRM
// chunk where there is one.
//
// A gAMA of 1/2.2 is how most writers say sRGB, libpng among them, and it is
// taken as the sRGB curve, which it approximates everywhere but in the
// deepest shadows. Without cHRM the primaries are those of sRGB, as the
// specification says a decoder should assume.
func gamaProfile(gama, chrm []byte, gray bool) (*cms.Profile, error) {
	var (
		curve  = cms.SRGBCurve()
		isSRGB = true
	)
	if len(gama) == 4 {
		g := binary.BigEndian.Uint32(gama)
		if g == 0 {
			return nil, errors.New("asset: gAMA of 0")
		}
		gamma := 100000 / float64(g)
		if gamma < 0.2 || gamma > 5 {
			return nil, fmt.Errorf("asset: gAMA of %d", g)
		}
		if math.Abs(gamma-2.2) > 0.022 {
			curve, isSRGB = cms.Gamma(gamma), false
		}
	} else if len(chrm) != 32 {
		return nil, nil
	}
	if gray {
		if isSRGB {
			return cms.SGray(), nil
		}
		return cms.NewGrayProfile("gAMA", cms.WhiteD65, curve)
	}
	white := cms.WhiteD65
	primaries := [3]cms.Chromaticity{xy(0.64, 0.33), xy(0.30, 0.60), xy(0.15, 0.06)}
	if len(chrm) == 32 {
		var v [8]float64
		for i := range v {
			v[i] = float64(binary.BigEndian.Uint32(chrm[4*i:])) / 100000
		}
		c := [4]cms.Chromaticity{xy(v[0], v[1]), xy(v[2], v[3]), xy(v[4], v[5]), xy(v[6], v[7])}
		if !nearChromaticities(c, [4]cms.Chromaticity{cms.WhiteD65, primaries[0], primaries[1], primaries[2]}) {
			white, primaries, isSRGB = c[0], [3]cms.Chromaticity{c[1], c[2], c[3]}, false
		}
	}
	if isSRGB {
		return cms.SRGB(), nil
	}
	return cms.NewRGBProfile("gAMA and cHRM", white, primaries, curve)
}

// nearChromaticities compares the way libpng recognises sRGB in a cHRM
// chunk: to a thousandth.
func nearChromaticities(a, b [4]cms.Chromaticity) bool {
	for i := range a {
		if math.Abs(a[i].X-b[i].X) > 0.001 || math.Abs(a[i].Y-b[i].Y) > 0.001 {
			return false
		}
	}
	return true
}

// --- WebP --------------------------------------------------------------------

func webpColour(b []byte) (declared, error) {
	d := declared{stored: cms.SpaceRGB}
	for i := 12; i+8 <= len(b); {
		n := binary.LittleEndian.Uint32(b[i+4:])
		if uint64(n) > uint64(len(b)-i-8) {
			break
		}
		typ := string(b[i : i+4])
		data := b[i+8 : i+8+int(n)]
		switch typ {
		case "ICCP":
			p, err := cms.ParseProfile(data)
			if err != nil {
				return d, err
			}
			d.profile, d.from = p, "ICCP chunk"
			return d, nil
		case "VP8 ", "VP8L", "ANIM":
			// The ICCP chunk comes before the picture.
			return d, nil
		}
		i += 8 + int(n) + int(n&1)
	}
	return d, nil
}
