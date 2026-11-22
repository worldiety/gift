package raw

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// Profile is the colour of a camera: how the colours its sensor sees become
// those of the light it saw, as the colour model of the DNG specification
// describes them. It is one or two colour matrices, each measured under an
// illuminant and interpolated between them for the white of a photograph,
// with forward matrices that map the camera's colours straight to XYZ, and
// optionally tables that correct hue, saturation and value, a look table
// and a tone curve.
//
// A profile comes from a DNG, which carries one ([File.Profile]), from the
// table of camera matrices of this package for an ARW, or from a DNG camera
// profile, a .dcp file ([ParseProfile]).
type Profile struct {
	name string
	// camera is the camera the profile is for, UniqueCameraModel.
	camera string
	// signature is ProfileCalibrationSignature.
	signature string
	// cals are the one or two calibrations, the lower temperature first.
	cals []calibration
	// hueSat is the hue, saturation and value map, nil for none, with the
	// one of each calibration in calibration.hueSat; look the look table.
	hueSatDims [3]int
	hueSatSRGB bool
	look       *hsvMap
	// tone is ProfileToneCurve, pairs of input and output, nil for none.
	tone []float64
	// exposure is BaselineExposureOffset, in stops.
	exposure float64
}

// calibration is a profile's measurement under one illuminant.
type calibration struct {
	illuminant  int
	temperature float64
	// colour maps XYZ to the camera's colours, forward the camera's white
	// balanced colours to XYZ under D50.
	colour     mat
	forward    mat
	hasForward bool
	// hueSat is the data of the hue, saturation and value map.
	hueSat []float32
}

// Name is the name of the profile, ProfileName; for one of this package's
// table, the camera's.
func (p *Profile) Name() string { return p.name }

// Camera is the camera the profile is for, as DNG's UniqueCameraModel
// names it, such as "Sony ILCE-7M3"; empty when the profile does not say.
func (p *Profile) Camera() string { return p.camera }

// Illuminants are the illuminants the profile's matrices were measured
// under, as EXIF's LightSource numbers them: 17 standard light A, 21 D65.
func (p *Profile) Illuminants() []int {
	var l []int
	for _, c := range p.cals {
		l = append(l, c.illuminant)
	}
	return l
}

// HasTables reports whether the profile corrects colours by tables beyond
// its matrices: a hue, saturation and value map or a look table.
func (p *Profile) HasTables() bool { return p.hueSatDims[0] > 0 || p.look != nil }

// HasToneCurve reports whether the profile brings its own tone curve, which
// then replaces the one of this package.
func (p *Profile) HasToneCurve() bool { return p.tone != nil }

// The tags of a camera profile, in a DNG's first directory or a .dcp file.
const (
	tagUniqueCameraModel        = 0xc614
	tagColorMatrix1             = 0xc621
	tagColorMatrix2             = 0xc622
	tagCalibrationIlluminant1   = 0xc65a
	tagCalibrationIlluminant2   = 0xc65b
	tagProfileCalibrationSig    = 0xc6f4
	tagProfileName              = 0xc6f8
	tagProfileHueSatMapDims     = 0xc6f9
	tagProfileHueSatMapData1    = 0xc6fa
	tagProfileHueSatMapData2    = 0xc6fb
	tagProfileToneCurve         = 0xc6fc
	tagForwardMatrix1           = 0xc714
	tagForwardMatrix2           = 0xc715
	tagProfileLookTableDims     = 0xc725
	tagProfileLookTableData     = 0xc726
	tagProfileHueSatMapEncoding = 0xc7a3
	tagProfileLookTableEncoding = 0xc7a4
	tagBaselineExposureOffset   = 0xc7a5
)

// ParseProfile reads a DNG camera profile, a .dcp file: a TIFF structure
// with the magic number 0x4352 ("IIRC" or "MMCR") in place of 42, whose
// first directory holds the tags of a camera profile. Profiles of three
// illuminants, which DNG 1.6 added, are read as their first two.
//
// This package ships no profiles of others; the user's own are loaded so.
func ParseProfile(data []byte) (*Profile, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("%w: a camera profile of %d bytes", ErrFormat, len(data))
	}
	var bo binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return nil, fmt.Errorf("%w: not a camera profile", ErrFormat)
	}
	if bo.Uint16(data[2:]) != 0x4352 {
		return nil, fmt.Errorf("%w: not a camera profile", ErrFormat)
	}
	t := &tiff{r: bytes.NewReader(data), end: int64(len(data)), bo: bo}
	d, _, err := t.dir(int64(bo.Uint32(data[4:])))
	if err != nil {
		return nil, err
	}
	return readProfile(t, d)
}

// readProfile reads the camera profile in the directory d.
func readProfile(t *tiff, d dir) (*Profile, error) {
	p := &Profile{
		name:      t.strOf(d, tagProfileName),
		camera:    t.strOf(d, tagUniqueCameraModel),
		signature: t.strOf(d, tagProfileCalibrationSig),
	}
	if v := t.floatsOf(d, tagBaselineExposureOffset); len(v) > 0 && !math.IsNaN(v[0]) {
		p.exposure = min(max(v[0], -10), 10)
	}
	for i, tags := range [2][4]uint16{
		{tagColorMatrix1, tagForwardMatrix1, tagCalibrationIlluminant1, tagProfileHueSatMapData1},
		{tagColorMatrix2, tagForwardMatrix2, tagCalibrationIlluminant2, tagProfileHueSatMapData2},
	} {
		cm, ok := matOf(t.floatsOf(d, tags[0]))
		if !ok {
			if i == 0 {
				return nil, fmt.Errorf("%w: a camera profile without a 3 by 3 colour matrix", ErrUnsupported)
			}
			break
		}
		if _, ok := cm.inverse(); !ok {
			return nil, fmt.Errorf("%w: a colour matrix that cannot be inverted", ErrFormat)
		}
		c := calibration{colour: cm}
		if l, ok := t.int(d, tags[2]); ok {
			c.illuminant = int(l)
		}
		c.temperature = temperatureOf(c.illuminant)
		if fm, ok := matOf(t.floatsOf(d, tags[1])); ok {
			if _, ok := fm.inverse(); ok {
				c.forward, c.hasForward = fm, true
			}
		}
		if f, ok := d[tags[3]]; ok {
			if v, err := t.floats(f); err == nil {
				c.hueSat = make([]float32, len(v))
				for k, x := range v {
					c.hueSat[k] = float32(x)
				}
			}
		}
		p.cals = append(p.cals, c)
	}
	// A second calibration under an illuminant whose temperature is not
	// known, or the same as the first, cannot be weighted: the first
	// alone is used.
	if len(p.cals) == 2 {
		a, b := p.cals[0], p.cals[1]
		switch {
		case a.temperature == 0 || b.temperature == 0 || a.temperature == b.temperature:
			p.cals = p.cals[:1]
		case a.temperature > b.temperature:
			p.cals[0], p.cals[1] = b, a
		}
	}

	if dims := t.intsOf(d, tagProfileHueSatMapDims); len(dims) == 3 {
		h, s, v := int(dims[0]), int(dims[1]), max(int(dims[2]), 1)
		ok := h >= 1 && s >= 2 && h*s*v <= 1<<20
		for _, c := range p.cals {
			ok = ok && len(c.hueSat) == 3*h*s*v
		}
		if ok {
			p.hueSatDims = [3]int{h, s, v}
			if e, _ := t.int(d, tagProfileHueSatMapEncoding); e == 1 {
				p.hueSatSRGB = true
			}
		}
	}
	if p.hueSatDims[0] == 0 {
		for i := range p.cals {
			p.cals[i].hueSat = nil
		}
	}
	if dims := t.intsOf(d, tagProfileLookTableDims); len(dims) == 3 {
		if f, ok := d[tagProfileLookTableData]; ok {
			if v, err := t.floats(f); err == nil {
				e, _ := t.int(d, tagProfileLookTableEncoding)
				p.look = newHSVMap(int(dims[0]), int(dims[1]), int(dims[2]), toFloat32(v), e == 1)
			}
		}
	}
	if v := t.floatsOf(d, tagProfileToneCurve); len(v) >= 4 && len(v)%2 == 0 {
		if monotone(v) {
			p.tone = v
		}
	}
	return p, nil
}

func toFloat32(v []float64) []float32 {
	r := make([]float32, len(v))
	for i, x := range v {
		r[i] = float32(x)
	}
	return r
}

// monotone reports whether pairs of input and output form a curve from 0 to
// 1 whose inputs rise and whose outputs do not fall.
func monotone(v []float64) bool {
	for i := 0; i < len(v); i += 2 {
		x, y := v[i], v[i+1]
		if !(x >= 0 && x <= 1 && y >= 0 && y <= 1) {
			return false
		}
		if i > 0 && (x <= v[i-2] || y < v[i-1]) {
			return false
		}
	}
	return v[0] == 0 && v[len(v)-2] == 1
}

// Profile is the camera profile of f: a DNG's own, which its first
// directory holds, or for an ARW the matrix this package has for the
// camera, which is Adobe's for D65 as dcraw tabulates it.
//
// A camera the package has no matrix for is [ErrUnsupported]; a profile of
// the user's, [ParseProfile], develops it then.
func (f *File) Profile() (*Profile, error) {
	if f.profile != nil {
		return f.profile, nil
	}
	model := strings.TrimSpace(f.Model)
	name := strings.TrimSpace(f.Make + " " + model)
	if strings.EqualFold(strings.TrimSpace(f.Make), "SONY") {
		name = "Sony " + model
		if c, ok := cameraMatrix(model); ok {
			var m mat
			for i, v := range c.matrix {
				m[i/3][i%3] = float64(v) / 10000
			}
			return &Profile{name: name, camera: name, cals: []calibration{{
				illuminant: 21, temperature: temperatureOf(21), colour: m,
			}}}, nil
		}
	}
	return nil, fmt.Errorf("%w: no colour matrix for the %s", ErrUnsupported, name)
}
