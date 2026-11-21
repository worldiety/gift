package cms

import (
	"errors"
	"fmt"
	"sync"
)

// Profile is an ICC profile: how the colours of a device, a file or a
// colour space relate to the CIE colours of the profile connection space.
//
// A Profile is immutable and safe for concurrent use. Parsing reads the
// header and the tag table; the tags themselves are read when a transform
// needs them.
type Profile struct {
	data    []byte
	version uint32
	class   Class
	space   Space
	pcs     Space
	intent  Intent
	desc    string
	tags    map[uint32][]byte

	mu    sync.Mutex
	black map[blackKey]XYZ
}

// ParseProfile reads an ICC profile of version 2 or 4. It keeps data,
// which the caller must not modify afterwards.
//
// It accepts profiles of RGB, Gray and CMYK devices that connect through
// XYZ or Lab, which is every profile embedded in a photograph or describing
// a display or a printer. Device links, abstract profiles, named colours
// and the floating point tables of ICC v4.3 (D2Bx) are refused or ignored.
func ParseProfile(data []byte) (*Profile, error) {
	if len(data) < 132 {
		return nil, errors.New("cms: profile too short")
	}
	if string(data[36:40]) != "acsp" {
		return nil, errors.New("cms: no ICC profile")
	}
	if size := int(u32(data)); size < len(data) && size >= 132 {
		data = data[:size]
	}
	p := &Profile{
		data:    data,
		version: u32(data[8:]),
		class:   Class(u32(data[12:])),
		space:   Space(u32(data[16:])),
		pcs:     Space(u32(data[20:])),
		intent:  Intent(u32(data[64:]) & 3),
		tags:    map[uint32][]byte{},
	}
	switch p.class {
	case ClassLink, ClassAbstract, ClassNamedColor:
		return nil, fmt.Errorf("cms: %s profiles are not supported", p.class)
	}
	switch p.space {
	case SpaceRGB, SpaceGray, SpaceCMYK:
	default:
		return nil, fmt.Errorf("cms: device colour space %s is not supported", p.space)
	}
	if p.pcs != SpaceXYZ && p.pcs != SpaceLab {
		return nil, fmt.Errorf("cms: connection space %s", p.pcs)
	}
	n := int(u32(data[128:]))
	if n < 0 || n > (len(data)-132)/12 {
		return nil, errors.New("cms: tag table too long")
	}
	for i := range n {
		e := data[132+12*i:]
		sig, off, size := u32(e), int(u32(e[4:])), int(u32(e[8:]))
		if off < 0 || size < 0 || off > len(data) || size > len(data)-off {
			return nil, fmt.Errorf("cms: tag %s out of the profile", sigString(sig))
		}
		p.tags[sig] = data[off : off+size]
	}
	p.desc = parseText(p.tags[tagDesc])
	if p.space == SpaceRGB && !p.has(tagA2B0) && !p.has(tagA2B1) && !p.isMatrixShaper() {
		return nil, errors.New("cms: RGB profile has neither tables nor a matrix")
	}
	return p, nil
}

// Description is the profile's name, as its desc tag has it.
func (p *Profile) Description() string { return p.desc }

// Space is the colour space of the device side.
func (p *Profile) Space() Space { return p.space }

// PCS is the profile connection space, XYZ or Lab.
func (p *Profile) PCS() Space { return p.pcs }

// Class is the device class.
func (p *Profile) Class() Class { return p.class }

// Version is the ICC version the profile was written for, as 2.1 or 4.3.
func (p *Profile) Version() (major, minor int) {
	return int(p.version >> 24), int(p.version >> 20 & 0xf)
}

// Intent is the rendering intent the header recommends.
func (p *Profile) Intent() Intent { return p.intent }

// Bytes is the profile as it is stored, to embed it in a file. The caller
// must not modify it.
func (p *Profile) Bytes() []byte { return p.data }

// MediaWhitePoint is the white of the medium, relative to D50: the paper
// of a print, the display's white for a v4 display profile. A v2 display
// profile counts as D50 whatever it says, as LittleCMS treats it
// (cmsio1.c, _cmsReadMediaWhitePoint).
func (p *Profile) MediaWhitePoint() XYZ {
	if p.version < 0x04000000 && p.class == ClassDisplay {
		return D50
	}
	if w, err := parseXYZ(p.tags[tagWtpt]); err == nil && w.X > 0 && w.Y > 0 && w.Z > 0 {
		return w
	}
	return D50
}

func (p *Profile) has(sig uint32) bool { return p.tags[sig] != nil }

// isMatrixShaper reports whether the profile has the tags of a matrix
// profile: curves and colorants for RGB, a curve for Gray.
func (p *Profile) isMatrixShaper() bool {
	switch p.space {
	case SpaceGray:
		return p.has(tagKTRC)
	case SpaceRGB:
		return p.has(tagRXYZ) && p.has(tagGXYZ) && p.has(tagBXYZ) &&
			p.has(tagRTRC) && p.has(tagGTRC) && p.has(tagBTRC)
	}
	return false
}

// IsMatrixShaper reports whether the profile is a matrix profile, curves
// and a matrix rather than tables, as sRGB and most display profiles are.
func (p *Profile) IsMatrixShaper() bool { return p.isMatrixShaper() }

// lutTagFor is the tag of a table for the intent, ICC specification
// 8.10.2, with the perceptual one standing in for a missing one, as
// LittleCMS does. Absolute colorimetric uses the relative table.
func (p *Profile) lutTagFor(intent Intent, toPCS bool) uint32 {
	sigs := [4]uint32{tagA2B0, tagA2B1, tagA2B2, tagA2B1}
	if !toPCS {
		sigs = [4]uint32{tagB2A0, tagB2A1, tagB2A2, tagB2A1}
	}
	if s := sigs[intent&3]; p.has(s) {
		return s
	}
	if p.has(sigs[0]) {
		return sigs[0]
	}
	return 0
}

// supports reports whether the profile has what an intent needs in one
// direction: a table of exactly that intent, or a matrix, as LittleCMS
// tells it (cmsIsIntentSupported). It decides black points, not which
// tables a transform uses.
func (p *Profile) supports(intent Intent, toPCS bool) bool {
	if p.isMatrixShaper() {
		return true
	}
	sigs := [4]uint32{tagA2B0, tagA2B1, tagA2B2, tagA2B1}
	if !toPCS {
		sigs = [4]uint32{tagB2A0, tagB2A1, tagB2A2, tagB2A1}
	}
	return p.has(sigs[intent&3])
}

// hasLut reports whether the profile has a table of exactly the intent in
// the direction from the connection space (cmsIsCLUT).
func (p *Profile) hasLut(intent Intent) bool {
	sigs := [4]uint32{tagB2A0, tagB2A1, tagB2A2, tagB2A1}
	return p.has(sigs[intent&3])
}

// toPCS is the pipeline from device values to the connection space, in
// XYZ with white at Y = 1 or in Lab, relative to the media white.
func (p *Profile) toPCS(intent Intent) (pipeline, error) {
	n := p.space.Channels()
	if sig := p.lutTagFor(intent, true); sig != 0 {
		lut, err := parseLut(p.tags[sig], n, 3)
		if err != nil {
			return nil, fmt.Errorf("cms: %s: %w", sigString(sig), err)
		}
		pl := append(pipeline(nil), lut.stages...)
		return append(pl, p.decodePCS(lut.lut16)), nil
	}
	switch p.space {
	case SpaceGray:
		c, err := p.curveTag(tagKTRC)
		if err != nil {
			return nil, err
		}
		pl := pipeline{}
		if s := newCurves([]curve{c}, false); s != nil {
			pl = append(pl, s)
		}
		if p.pcs == SpaceLab {
			return append(pl, fromGray{scale: [3]float64{100, 0, 0}}), nil
		}
		return append(pl, fromGray{scale: [3]float64{D50.X, D50.Y, D50.Z}}), nil
	case SpaceRGB:
		cs, m, err := p.matrixShaper()
		if err != nil {
			return nil, err
		}
		pl := pipeline{}
		if s := newCurves(cs, false); s != nil {
			pl = append(pl, s)
		}
		pl = append(pl, affine{m: m})
		if p.pcs == SpaceLab {
			pl = append(pl, xyzToLabStage{})
		}
		return pl, nil
	}
	return nil, fmt.Errorf("cms: %s profile without a table to the connection space", p.space)
}

// fromPCS is the pipeline from the connection space to device values.
func (p *Profile) fromPCS(intent Intent) (pipeline, error) {
	n := p.space.Channels()
	if sig := p.lutTagFor(intent, false); sig != 0 {
		lut, err := parseLut(p.tags[sig], 3, n)
		if err != nil {
			return nil, fmt.Errorf("cms: %s: %w", sigString(sig), err)
		}
		if p.pcs == SpaceLab {
			// LittleCMS interpolates the tables of Lab output profiles
			// trilinearly (cmsio1.c, _cmsReadOutputLUT).
			for i, s := range lut.stages {
				if c, ok := s.(*clut); ok {
					t := *c
					t.trilinear = true
					lut.stages[i] = &t
				}
			}
		}
		return append(pipeline{p.encodePCS(lut.lut16)}, lut.stages...), nil
	}
	switch p.space {
	case SpaceGray:
		c, err := p.curveTag(tagKTRC)
		if err != nil {
			return nil, err
		}
		pl := pipeline{toGray{w: [3]float64{0, 1, 0}}}
		if p.pcs == SpaceLab {
			pl[0] = toGray{w: [3]float64{1.0 / 100, 0, 0}}
		}
		if s := newCurves([]curve{c}, true); s != nil {
			pl = append(pl, s)
		}
		return pl, nil
	case SpaceRGB:
		cs, m, err := p.matrixShaper()
		if err != nil {
			return nil, err
		}
		inv, ok := m.inverse()
		if !ok {
			return nil, errors.New("cms: the colorant matrix cannot be inverted")
		}
		pl := pipeline{}
		if p.pcs == SpaceLab {
			pl = append(pl, labToXYZStage{})
		}
		pl = append(pl, affine{m: inv})
		if s := newCurves(cs, true); s != nil {
			pl = append(pl, s)
		}
		return pl, nil
	}
	return nil, fmt.Errorf("cms: %s profile without a table from the connection space", p.space)
}

// decodePCS turns the 0 to 1 encoding of a table's connection space side
// into XYZ or Lab.
func (p *Profile) decodePCS(lut16 bool) affine {
	if p.pcs == SpaceXYZ {
		return scale3(maxXYZ, maxXYZ, maxXYZ)
	}
	k := 1.0
	if lut16 {
		k = labV2
	}
	return affine{m: diag(100*k, 255*k, 255*k), off: [3]float64{0, -128, -128}}
}

// encodePCS is the inverse of decodePCS.
func (p *Profile) encodePCS(lut16 bool) affine {
	if p.pcs == SpaceXYZ {
		return scale3(1/maxXYZ, 1/maxXYZ, 1/maxXYZ)
	}
	k := 1.0
	if lut16 {
		k = labV2
	}
	return affine{m: diag(1/(100*k), 1/(255*k), 1/(255*k)), off: [3]float64{0, 128 / (255 * k), 128 / (255 * k)}}
}

func (p *Profile) curveTag(sig uint32) (curve, error) {
	c, _, err := parseCurve(p.tags[sig])
	if err != nil {
		return nil, fmt.Errorf("cms: %s: %w", sigString(sig), err)
	}
	return c.compile()
}

// matrixShaper reads the three curves and the colorant matrix of an RGB
// matrix profile.
func (p *Profile) matrixShaper() ([]curve, mat3, error) {
	var m mat3
	for i, sig := range []uint32{tagRXYZ, tagGXYZ, tagBXYZ} {
		c, err := parseXYZ(p.tags[sig])
		if err != nil {
			return nil, m, fmt.Errorf("cms: %s: %w", sigString(sig), err)
		}
		m[0][i], m[1][i], m[2][i] = c.X, c.Y, c.Z
	}
	cs := make([]curve, 3)
	for i, sig := range []uint32{tagRTRC, tagGTRC, tagBTRC} {
		c, err := p.curveTag(sig)
		if err != nil {
			return nil, m, err
		}
		cs[i] = c
	}
	return cs, m, nil
}
