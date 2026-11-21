package cms

import "math"

// XYZ is a colour in CIE XYZ, with the Y of white at 1.
type XYZ struct{ X, Y, Z float64 }

// Chromaticity is a colour's position in the CIE xy diagram: its X and Z
// in proportion to X+Y+Z, without its brightness.
type Chromaticity struct{ X, Y float64 }

// XYZ is the colour of the chromaticity at the luminance Y = 1.
func (c Chromaticity) XYZ() XYZ {
	return XYZ{c.X / c.Y, 1, (1 - c.X - c.Y) / c.Y}
}

// The white points of the colour spaces built in.
var (
	// D50 is the illuminant of the profile connection space, as the ICC
	// specification and LittleCMS write it.
	D50 = XYZ{0.9642, 1.0, 0.8249}
	// WhiteD65 is daylight, the white of sRGB, Display P3, Adobe RGB and
	// Rec. 2020.
	WhiteD65 = Chromaticity{0.3127, 0.3290}
	// WhiteD50 is the white of ProPhoto RGB.
	WhiteD50 = Chromaticity{0.3457, 0.3585}
)

// The encodings of the profile connection space in the tables of a
// profile, which hold numbers from 0 to 1.
const (
	// maxXYZ is the XYZ value of the largest table entry: u1Fixed15Number,
	// where 0x8000 is 1.0.
	maxXYZ = 1 + 32767.0/32768
	// labV2 is how much larger a v2 Lab encoding is than the v4 one for
	// the same Lab value: 0xFF00 rather than 0xFFFF is L* 100.
	labV2 = 65535.0 / 65280
)

// labF is the cube root of the Lab formulas with its linear foot, as
// LittleCMS writes it.
func labF(t float64) float64 {
	const limit = (24.0 / 116) * (24.0 / 116) * (24.0 / 116)
	if t <= limit {
		return (841.0/108)*t + 16.0/116
	}
	return math.Cbrt(t)
}

func labFInverse(t float64) float64 {
	const limit = 24.0 / 116
	if t <= limit {
		return (108.0 / 841) * (t - 16.0/116)
	}
	return t * t * t
}

// labToXYZ converts L* a* b* relative to the white w to XYZ.
func labToXYZ(w XYZ, l, a, b float64) (x, y, z float64) {
	fy := (l + 16) / 116
	return labFInverse(fy+0.002*a) * w.X, labFInverse(fy) * w.Y, labFInverse(fy-0.005*b) * w.Z
}

// xyzToLab converts XYZ to L* a* b* relative to the white w.
func xyzToLab(w XYZ, x, y, z float64) (l, a, b float64) {
	fx, fy, fz := labF(x/w.X), labF(y/w.Y), labF(z/w.Z)
	return 116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)
}

// mat3 is a 3 by 3 matrix, rows first.
type mat3 [3][3]float64

var identity3 = mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

func diag(a, b, c float64) mat3 { return mat3{{a, 0, 0}, {0, b, 0}, {0, 0, c}} }

// mul is m·n: n applies first.
func (m mat3) mul(n mat3) mat3 {
	var r mat3
	for i := range 3 {
		for j := range 3 {
			r[i][j] = m[i][0]*n[0][j] + m[i][1]*n[1][j] + m[i][2]*n[2][j]
		}
	}
	return r
}

func (m mat3) apply(x, y, z float64) (float64, float64, float64) {
	return m[0][0]*x + m[0][1]*y + m[0][2]*z,
		m[1][0]*x + m[1][1]*y + m[1][2]*z,
		m[2][0]*x + m[2][1]*y + m[2][2]*z
}

func (m mat3) inverse() (mat3, bool) {
	c := mat3{
		{m[1][1]*m[2][2] - m[1][2]*m[2][1], m[0][2]*m[2][1] - m[0][1]*m[2][2], m[0][1]*m[1][2] - m[0][2]*m[1][1]},
		{m[1][2]*m[2][0] - m[1][0]*m[2][2], m[0][0]*m[2][2] - m[0][2]*m[2][0], m[0][2]*m[1][0] - m[0][0]*m[1][2]},
		{m[1][0]*m[2][1] - m[1][1]*m[2][0], m[0][1]*m[2][0] - m[0][0]*m[2][1], m[0][0]*m[1][1] - m[0][1]*m[1][0]},
	}
	det := m[0][0]*c[0][0] + m[0][1]*c[1][0] + m[0][2]*c[2][0]
	if math.Abs(det) < 1e-12 {
		return mat3{}, false
	}
	for i := range 3 {
		for j := range 3 {
			c[i][j] /= det
		}
	}
	return c, true
}

func (m mat3) isIdentity(eps float64) bool {
	for i := range 3 {
		for j := range 3 {
			if math.Abs(m[i][j]-identity3[i][j]) > eps {
				return false
			}
		}
	}
	return true
}

// bradford is the Bradford cone response matrix.
var bradford = mat3{
	{0.8951, 0.2664, -0.1614},
	{-0.7502, 1.7135, 0.0367},
	{0.0389, -0.0685, 1.0296},
}

// adaptation is the Bradford chromatic adaptation from white from to white
// to: the chad tag of a profile whose media white is from.
func adaptation(from, to XYZ) mat3 {
	inv, _ := bradford.inverse()
	sx, sy, sz := bradford.apply(from.X, from.Y, from.Z)
	dx, dy, dz := bradford.apply(to.X, to.Y, to.Z)
	return inv.mul(diag(dx/sx, dy/sy, dz/sz)).mul(bradford)
}

// rgbToXYZ is the matrix from linear RGB with the given primaries and white
// to XYZ relative to that white, as in SMPTE RP 177.
func rgbToXYZ(white Chromaticity, p [3]Chromaticity) (mat3, bool) {
	var m mat3
	for i, c := range p {
		xyz := c.XYZ()
		m[0][i], m[1][i], m[2][i] = xyz.X, xyz.Y, xyz.Z
	}
	inv, ok := m.inverse()
	if !ok {
		return mat3{}, false
	}
	w := white.XYZ()
	sr, sg, sb := inv.apply(w.X, w.Y, w.Z)
	return m.mul(diag(sr, sg, sb)), true
}

// unit clamps x to 0 to 1, with NaN as 0: a value that indexes a table
// must not be NaN, which Go's min and max pass through.
func unit[F float32 | float64](x F) F {
	if !(x > 0) {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
