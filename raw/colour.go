package raw

import (
	"math"
	"sync"

	"github.com/worldiety/gift/cms"
)

// vec is a colour of three components.
type vec [3]float64

// mat is a 3 by 3 matrix, row by row.
type mat [3][3]float64

var identity = mat{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

// matOf makes a matrix of nine values, row by row.
func matOf(v []float64) (mat, bool) {
	if len(v) != 9 {
		return mat{}, false
	}
	var m mat
	for i := range 9 {
		if math.IsNaN(v[i]) || math.IsInf(v[i], 0) {
			return mat{}, false
		}
		m[i/3][i%3] = v[i]
	}
	return m, true
}

func diag(v vec) mat { return mat{{v[0], 0, 0}, {0, v[1], 0}, {0, 0, v[2]}} }

func (m mat) mul(n mat) mat {
	var r mat
	for i := range 3 {
		for j := range 3 {
			r[i][j] = m[i][0]*n[0][j] + m[i][1]*n[1][j] + m[i][2]*n[2][j]
		}
	}
	return r
}

func (m mat) apply(v vec) vec {
	return vec{
		m[0][0]*v[0] + m[0][1]*v[1] + m[0][2]*v[2],
		m[1][0]*v[0] + m[1][1]*v[1] + m[1][2]*v[2],
		m[2][0]*v[0] + m[2][1]*v[1] + m[2][2]*v[2],
	}
}

func (m mat) scale(s float64) mat {
	for i := range 3 {
		for j := range 3 {
			m[i][j] *= s
		}
	}
	return m
}

// blend is g of m and 1-g of n.
func (m mat) blend(n mat, g float64) mat {
	for i := range 3 {
		for j := range 3 {
			m[i][j] = g*m[i][j] + (1-g)*n[i][j]
		}
	}
	return m
}

func (m mat) inverse() (mat, bool) {
	a := m
	det := a[0][0]*(a[1][1]*a[2][2]-a[1][2]*a[2][1]) -
		a[0][1]*(a[1][0]*a[2][2]-a[1][2]*a[2][0]) +
		a[0][2]*(a[1][0]*a[2][1]-a[1][1]*a[2][0])
	if math.Abs(det) < 1e-12 || math.IsNaN(det) {
		return mat{}, false
	}
	var r mat
	r[0][0] = (a[1][1]*a[2][2] - a[1][2]*a[2][1]) / det
	r[0][1] = (a[0][2]*a[2][1] - a[0][1]*a[2][2]) / det
	r[0][2] = (a[0][1]*a[1][2] - a[0][2]*a[1][1]) / det
	r[1][0] = (a[1][2]*a[2][0] - a[1][0]*a[2][2]) / det
	r[1][1] = (a[0][0]*a[2][2] - a[0][2]*a[2][0]) / det
	r[1][2] = (a[0][2]*a[1][0] - a[0][0]*a[1][2]) / det
	r[2][0] = (a[1][0]*a[2][1] - a[1][1]*a[2][0]) / det
	r[2][1] = (a[0][1]*a[2][0] - a[0][0]*a[2][1]) / det
	r[2][2] = (a[0][0]*a[1][1] - a[0][1]*a[1][0]) / det
	return r, true
}

// f32 is m as float32, row by row, for the loops over pixels.
func (m mat) f32() [9]float32 {
	var r [9]float32
	for i := range 9 {
		r[i] = float32(m[i/3][i%3])
	}
	return r
}

// xyOf is the chromaticity of XYZ.
func xyOf(v vec) cms.Chromaticity {
	s := v[0] + v[1] + v[2]
	if s <= 0 {
		return whiteD50
	}
	return cms.Chromaticity{X: v[0] / s, Y: v[1] / s}
}

// xyzOf is the XYZ of a chromaticity at Y = 1.
func xyzOf(c cms.Chromaticity) vec {
	if c.Y <= 0 {
		return vec{}
	}
	return vec{c.X / c.Y, 1, (1 - c.X - c.Y) / c.Y}
}

// The whites and primaries the colour model and the working spaces use.
var (
	whiteD50 = cms.WhiteD50
	whiteD65 = cms.WhiteD65
	// primariesProPhoto are those of ProPhoto RGB, the space the tables of
	// a DNG camera profile and the tone curve work in.
	primariesProPhoto = [3]cms.Chromaticity{{X: 0.7347, Y: 0.2653}, {X: 0.1596, Y: 0.8404}, {X: 0.0366, Y: 0.0001}}
	primaries2020     = [3]cms.Chromaticity{{X: 0.708, Y: 0.292}, {X: 0.170, Y: 0.797}, {X: 0.131, Y: 0.046}}
)

// bradford is the Bradford cone response matrix.
var bradford = mat{
	{0.8951, 0.2664, -0.1614},
	{-0.7502, 1.7135, 0.0367},
	{0.0389, -0.0685, 1.0296},
}

// adapt is the Bradford chromatic adaptation from XYZ under white from to
// XYZ under white to.
func adapt(from, to cms.Chromaticity) mat {
	inv, _ := bradford.inverse()
	s := bradford.apply(xyzOf(from))
	d := bradford.apply(xyzOf(to))
	return inv.mul(diag(vec{d[0] / s[0], d[1] / s[1], d[2] / s[2]})).mul(bradford)
}

// rgbToXYZ is the matrix from linear RGB of the primaries p and the white w
// to XYZ under D50, adapted by Bradford as an ICC profile's colorants are.
func rgbToXYZ(w cms.Chromaticity, p [3]cms.Chromaticity) mat {
	var m mat
	for i, c := range p {
		x := xyzOf(c)
		m[0][i], m[1][i], m[2][i] = x[0], x[1], x[2]
	}
	inv, _ := m.inverse()
	s := inv.apply(xyzOf(w))
	return adapt(w, whiteD50).mul(m.mul(diag(s)))
}

// The matrices between XYZ under D50, linear ProPhoto RGB and linear
// Rec. 2020.
var (
	proPhotoToXYZ = rgbToXYZ(whiteD50, primariesProPhoto)
	xyzToProPhoto = must(proPhotoToXYZ.inverse())
	rec2020ToXYZ  = rgbToXYZ(whiteD65, primaries2020)
	xyzToRec2020  = must(rec2020ToXYZ.inverse())
)

func must(m mat, ok bool) mat {
	if !ok {
		panic("raw: a matrix of the package cannot be inverted")
	}
	return m
}

// linearProPhoto is ProPhoto RGB on linear light, the source of the
// transform to the space a picture is rendered in.
var linearProPhoto = sync.OnceValue(func() *cms.Profile {
	p, err := cms.NewRGBProfile("Linear ProPhoto RGB", whiteD50, primariesProPhoto, cms.Linear())
	if err != nil {
		panic(err)
	}
	return p
})

// temperatureOf is the correlated colour temperature of an illuminant as
// EXIF's LightSource numbers them, that by which the colour model weights
// the matrices measured under two of them; zero for one it does not know.
// The fluorescent classes are the middle of their ranges.
func temperatureOf(light int) float64 {
	switch light {
	case 3, 17: // tungsten, standard light A
		return 2856
	case 24: // ISO studio tungsten
		return 3200
	case 18: // standard light B
		return 4874
	case 19: // standard light C
		return 6774
	case 23: // D50
		return 5003
	case 1, 4, 9, 20: // daylight, flash, fine weather, D55
		return 5503
	case 10, 21: // cloudy, D65
		return 6504
	case 11, 22: // shade, D75
		return 7504
	case 12, 25, 30: // daylight fluorescent and LED, D 5700 to 7100 K
		return 6400
	case 13, 26, 31: // day white, N 4600 to 5500 K
		return 5050
	case 2, 14, 27, 32: // fluorescent, cool white, W 3800 to 4500 K
		return 4150
	case 15, 28, 33: // white, WW 3250 to 3800 K
		return 3525
	case 16, 29, 34: // warm white, L 2600 to 3250 K
		return 2925
	}
	return 0
}

// temperature is the correlated colour temperature of a chromaticity, by
// McCamy's cubic, within a few kelvin of the exact one from 2800 to 6500 K
// and close enough beyond for weighting two matrices; it is bounded to 1500
// to 50000 K.
func temperature(c cms.Chromaticity) float64 {
	n := (c.X - 0.3320) / (0.1858 - c.Y)
	t := 449*n*n*n + 3525*n*n + 6823.3*n + 5520.33
	if math.IsNaN(t) {
		return 5003
	}
	return min(max(t, 1500), 50000)
}

// weight is how much of the first of two calibrations, measured at the
// temperatures t1 and t2, the colour model takes for a white of temperature
// t: linear in the inverse temperature between them, all of the nearer one
// outside.
func weight(t, t1, t2 float64) float64 {
	if t1 <= 0 || t2 <= 0 || t1 == t2 {
		return 1
	}
	g := (1/t - 1/t2) / (1/t1 - 1/t2)
	return min(max(g, 0), 1)
}

// model is a camera profile applied to one file: its matrices, the file's
// own calibration and analog balance.
type model struct {
	p *Profile
	// cc are the camera calibrations of the file that apply to p, one for
	// each of p's calibrations; ab is the analog balance.
	cc [2]mat
	ab mat
}

func (f *File) model(p *Profile) model {
	m := model{p: p, cc: [2]mat{identity, identity}, ab: identity}
	c := f.DNG
	if c == nil {
		return m
	}
	if len(c.AnalogBalance) == 3 {
		m.ab = diag(vec{c.AnalogBalance[0], c.AnalogBalance[1], c.AnalogBalance[2]})
	}
	// The calibration of the individual camera belongs to the matrices it
	// was measured against: those of a profile with the same signature, or
	// of any when the file gives none.
	if c.CameraCalibrationSignature == "" || c.CameraCalibrationSignature == p.signature {
		for i, v := range [][]float64{c.CameraCalibration1, c.CameraCalibration2} {
			if cc, ok := matOf(v); ok {
				m.cc[i] = cc
			}
		}
	}
	return m
}

// weightAt is the weight of the first calibration for a white.
func (m model) weightAt(white cms.Chromaticity) float64 {
	if len(m.p.cals) < 2 {
		return 1
	}
	return weight(temperature(white), m.p.cals[0].temperature, m.p.cals[1].temperature)
}

// xyzToCamera is AB·CC·CM for the weight g of the first calibration.
func (m model) xyzToCamera(g float64) mat {
	cs := m.p.cals
	cm := m.cc[0].mul(cs[0].colour)
	if len(cs) > 1 {
		cm = cm.blend(m.cc[1].mul(cs[1].colour), g)
	}
	return m.ab.mul(cm)
}

// whiteOf is the chromaticity of a white given in the camera's colours, the
// neutral: found by iteration, because which matrix applies depends on the
// white itself.
func (m model) whiteOf(neutral vec) cms.Chromaticity {
	xy := whiteD50
	for range 30 {
		inv, ok := m.xyzToCamera(m.weightAt(xy)).inverse()
		if !ok {
			return whiteD50
		}
		next := xyOf(inv.apply(neutral))
		if math.Abs(next.X-xy.X)+math.Abs(next.Y-xy.Y) < 1e-7 {
			return next
		}
		// Halfway steps keep the iteration from swinging between two
		// temperatures.
		xy = cms.Chromaticity{X: (xy.X + next.X) / 2, Y: (xy.Y + next.Y) / 2}
	}
	return xy
}

// neutralOf is the camera's colours of a white, the largest 1.
func (m model) neutralOf(white cms.Chromaticity) vec {
	n := m.xyzToCamera(m.weightAt(white)).apply(xyzOf(white))
	hi := max(n[0], n[1], n[2])
	if !(hi > 0) {
		return vec{1, 1, 1}
	}
	return vec{n[0] / hi, n[1] / hi, n[2] / hi}
}

// cameraToXYZ is the matrix from the camera's colours, white balanced so
// that neutral is (1, 1, 1), to XYZ under D50, scaled so that the neutral
// has Y = 1; and the weight of the first calibration it took.
func (m model) cameraToXYZ(neutral vec) (mat, float64, bool) {
	white := m.whiteOf(neutral)
	g := m.weightAt(white)
	cs := m.p.cals
	var c2x mat
	if cs[0].hasForward && (len(cs) < 2 || cs[1].hasForward) {
		// The forward matrices map the reference camera's white balanced
		// colours to D50; the reference camera is the camera without its
		// analog balance and calibration.
		fm := cs[0].forward
		cc := m.cc[0]
		if len(cs) > 1 {
			fm = fm.blend(cs[1].forward, g)
			cc = cc.blend(m.cc[1], g)
		}
		abcc, ok := m.ab.mul(cc).inverse()
		if !ok {
			return mat{}, 0, false
		}
		ref := abcc.apply(neutral)
		if !(ref[0] > 0 && ref[1] > 0 && ref[2] > 0) {
			return mat{}, 0, false
		}
		c2x = fm.mul(diag(vec{1 / ref[0], 1 / ref[1], 1 / ref[2]})).mul(abcc)
	} else {
		inv, ok := m.xyzToCamera(g).inverse()
		if !ok {
			return mat{}, 0, false
		}
		c2x = adapt(white, whiteD50).mul(inv)
	}
	c2x = c2x.mul(diag(neutral))
	y := c2x[1][0] + c2x[1][1] + c2x[1][2]
	if !(y > 0) {
		return mat{}, 0, false
	}
	return c2x.scale(1 / y), g, true
}
