package raw

import (
	"fmt"
	"image"
	"math"
	"sync"

	"github.com/worldiety/gift/cms"
)

// Output is what [File.Develop] makes of the raw data.
type Output uint8

const (
	// OutputPicture is a picture to be shown: the colours of the camera
	// through its profile, the tone curve that makes the scene's light the
	// tones of a picture, and the colour space of [Options.Space], encoded
	// with its curve, from 0 to 1.
	OutputPicture Output = iota
	// OutputScene is the light of the scene, linear, in linear Rec. 2020
	// with the white of D65: the camera's colours through its profile and
	// the exposure, without a tone curve. The camera's white is 1, and
	// highlights above it stay above. It is the working space for what
	// is to act on the scene before its tones are made.
	OutputScene
	// OutputCamera is the light the sensor saw, linear, in the colours of
	// the camera, white balanced: no profile, no exposure, no tone curve.
	OutputCamera
)

func (o Output) String() string {
	switch o {
	case OutputPicture:
		return "picture"
	case OutputScene:
		return "scene"
	case OutputCamera:
		return "camera"
	}
	return fmt.Sprintf("Output(%d)", uint8(o))
}

// ProcessVersion is the version of how this package develops raw data into
// pictures: its demosaicing, its colours and its tones. It changes when a
// picture changes visibly, so that a cache of developed pictures can be
// keyed by it; within a version, the same file and options give the same
// picture.
const ProcessVersion = 1

// pipe turns the camera's white balanced colours of a tile into the output.
//
// The camera's colours become linear ProPhoto RGB, where the tables of a
// camera profile work, with the exposure between the hue and saturation map
// and the look table; then linear Rec. 2020, the light of the scene. A
// picture is made from that by the tone curve of this package, channel by
// channel in Rec. 2020 as cameras make their pictures; a profile that
// brings its own tone curve has it applied in ProPhoto, keeping the hue, as
// Adobe's raw converters do. Last, the colour space of the picture.
type pipe struct {
	out Output
	// toPro maps the camera's colours to linear ProPhoto RGB, with the
	// exposure gain unless a hue and saturation map comes first.
	toPro [9]float32
	// hueSat and look are the tables of the profile, nil for none, and
	// gain the exposure between them.
	hueSat, look *hsvMap
	gain         float32
	// toScene maps linear ProPhoto RGB to linear Rec. 2020.
	toScene [9]float32
	// tone is the tone curve, in Rec. 2020 channel by channel, or with
	// hue in ProPhoto; toOut the matrix from the space the curve works in
	// to the linear light of the picture's space, and enc its curves.
	tone  *toneCurve
	hue   bool
	toOut [9]float32
	enc   [3]encoder
}

// newPipe makes the pipe for opt and the white balance wb, nil for
// OutputCamera.
func (f *File) newPipe(opt Options, wb [3]float64) (*pipe, error) {
	if opt.Output == OutputCamera {
		return nil, nil
	}
	if opt.Output > OutputCamera {
		return nil, fmt.Errorf("raw: %v", opt.Output)
	}
	p := opt.Profile
	if p == nil {
		var err error
		if p, err = f.Profile(); err != nil {
			return nil, err
		}
	}
	neutral := vec{1 / wb[0], 1 / wb[1], 1 / wb[2]}
	hi := max(neutral[0], neutral[1], neutral[2])
	for c := range neutral {
		neutral[c] /= hi
	}
	m := f.model(p)
	c2x, g, ok := m.cameraToXYZ(neutral)
	if !ok {
		return nil, fmt.Errorf("%w: the colour matrices of %q cannot map the white balance %v", ErrUnsupported, p.name, wb)
	}
	stops := opt.Exposure + p.exposure
	if f.DNG != nil {
		stops += f.DNG.BaselineExposure
	}
	stops = min(max(stops, -20), 20)
	gain := math.Exp2(stops)

	k := &pipe{out: opt.Output, gain: 1}
	toPro := xyzToProPhoto.mul(c2x)
	if p.hueSatDims[0] > 0 {
		var b []float32
		if len(p.cals) > 1 {
			b = p.cals[1].hueSat
		}
		k.hueSat = blendHSV(p.hueSatDims, p.hueSatSRGB, p.cals[0].hueSat, b, g)
	}
	if k.hueSat != nil {
		k.gain = float32(gain)
	} else {
		toPro = toPro.scale(gain)
	}
	k.look = p.look
	proTo2020 := xyzToRec2020.mul(proPhotoToXYZ)
	if k.hueSat == nil && k.look == nil {
		// Without tables, ProPhoto is not needed: one matrix to
		// Rec. 2020.
		k.toPro = proTo2020.mul(toPro).f32()
		k.toScene = identity.f32()
	} else {
		k.toPro = toPro.f32()
		k.toScene = proTo2020.f32()
	}
	if opt.Output == OutputScene {
		return k, nil
	}

	src := cms.LinearRec2020()
	k.tone = defaultTone()
	if p.tone != nil {
		k.tone, k.hue = profileTone(p.tone), true
		k.toScene = identity.f32()
		if k.hueSat == nil && k.look == nil {
			k.toPro = toPro.f32()
		}
		src = linearProPhoto()
	}
	out, err := outputOf(src, opt.Space)
	if err != nil {
		return nil, err
	}
	k.toOut, k.enc = out.matrix, out.enc
	return k, nil
}

// output is how linear light of a source space becomes a colour space: a
// matrix to its linear light and its curves.
type output struct {
	matrix [9]float32
	enc    [3]encoder
}

// outputs caches the output of each pair of spaces, which takes a
// transform of cms to make.
var outputs sync.Map // [2]*cms.Profile -> *output

func outputOf(src, space *cms.Profile) (*output, error) {
	if space == nil {
		space = cms.SRGB()
	}
	key := [2]*cms.Profile{src, space}
	if o, ok := outputs.Load(key); ok {
		return o.(*output), nil
	}
	if space.Space() != cms.SpaceRGB {
		return nil, fmt.Errorf("%w: a picture in a space other than RGB", ErrUnsupported)
	}
	t, err := cms.NewTransform(src, space, cms.Options{Intent: cms.RelativeColorimetric})
	if err != nil {
		return nil, err
	}
	sh, ok := t.Shaper()
	if !ok {
		return nil, fmt.Errorf("%w: a picture in a space of tables rather than a matrix", ErrUnsupported)
	}
	o := &output{matrix: sh.Matrix}
	for c := range 3 {
		o.enc[c] = encoder{table: sh.Out[c], scale: float32(len(sh.Out[c]) - 1)}
	}
	v, _ := outputs.LoadOrStore(key, o)
	return v.(*output), nil
}

// encoder is the curve of a colour space, from linear light to its encoded
// values, as a table over the square root of the light.
type encoder struct {
	table []float32
	scale float32
}

func (e encoder) at(x float32) float32 {
	if !(x > 0) {
		return e.table[0]
	}
	if x >= 1 {
		return e.table[len(e.table)-1]
	}
	f := float32(math.Sqrt(float64(x))) * e.scale
	i := int(f)
	if i >= len(e.table)-1 {
		return e.table[len(e.table)-1]
	}
	w := f - float32(i)
	return e.table[i] + w*(e.table[i+1]-e.table[i])
}

func apply(m *[9]float32, r, g, b float32) (float32, float32, float32) {
	return m[0]*r + m[1]*g + m[2]*b, m[3]*r + m[4]*g + m[5]*b, m[6]*r + m[7]*g + m[8]*b
}

// run turns the pixels of the planes of t, which hold the camera's colours,
// into the output, in place.
func (k *pipe) run(t Tile) {
	w, h := t.Rect.Dx(), t.Rect.Dy()
	for y := range h {
		o := y * t.Stride
		rs, gs, bs := t.R[o:o+w], t.G[o:o+w], t.B[o:o+w]
		for i := range rs {
			r, g, b := apply(&k.toPro, rs[i], gs[i], bs[i])
			if k.hueSat != nil {
				r, g, b = k.hueSat.apply(max(r, 0), max(g, 0), max(b, 0))
				r, g, b = r*k.gain, g*k.gain, b*k.gain
			}
			if k.look != nil {
				r, g, b = k.look.apply(max(r, 0), max(g, 0), max(b, 0))
			}
			r, g, b = apply(&k.toScene, r, g, b)
			switch {
			case k.out == OutputScene:
				rs[i], gs[i], bs[i] = r, g, b
				continue
			case k.hue:
				r, g, b = k.tone.rgb(r, g, b)
			default:
				// A colour beyond Rec. 2020 has a channel below zero,
				// which the curve would clip: it is brought in first.
				if min(r, g, b) < 0 {
					r, g, b = inGamut(r, g, b)
				}
				r, g, b = k.tone.at(r), k.tone.at(g), k.tone.at(b)
			}
			r, g, b = compress(apply(&k.toOut, r, g, b))
			rs[i], gs[i], bs[i] = k.enc[0].at(r), k.enc[1].at(g), k.enc[2].at(b)
		}
	}
}

// RGBA develops f into a picture of 8 bits a channel, in the colour space
// of opt.Space, as [OutputPicture] whatever opt.Output says. It is stored
// as the sensor saw it: the file's [File.Orientation] is not applied.
func (f *File) RGBA(opt Options) (*image.RGBA, error) {
	opt.Output = OutputPicture
	img := image.NewRGBA(f.Bounds(opt))
	err := f.Develop(opt, func(t Tile) {
		for y := range t.Rect.Dy() {
			s := y * t.Stride
			row := img.Pix[img.PixOffset(t.Rect.Min.X, t.Rect.Min.Y+y):]
			for x := range t.Rect.Dx() {
				p := row[4*x : 4*x+4 : 4*x+4]
				p[0], p[1], p[2], p[3] = quantize8(t.R[s+x]), quantize8(t.G[s+x]), quantize8(t.B[s+x]), 0xff
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return img, nil
}

// RGBA64 is [File.RGBA] with 16 bits a channel.
func (f *File) RGBA64(opt Options) (*image.RGBA64, error) {
	opt.Output = OutputPicture
	img := image.NewRGBA64(f.Bounds(opt))
	err := f.Develop(opt, func(t Tile) {
		for y := range t.Rect.Dy() {
			s := y * t.Stride
			row := img.Pix[img.PixOffset(t.Rect.Min.X, t.Rect.Min.Y+y):]
			for x := range t.Rect.Dx() {
				p := row[8*x : 8*x+8 : 8*x+8]
				r, g, b := quantize16(t.R[s+x]), quantize16(t.G[s+x]), quantize16(t.B[s+x])
				p[0], p[1], p[2], p[3] = byte(r>>8), byte(r), byte(g>>8), byte(g)
				p[4], p[5], p[6], p[7] = byte(b>>8), byte(b), 0xff, 0xff
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return img, nil
}

func quantize8(v float32) uint8 {
	if !(v > 0) {
		return 0
	}
	if v >= 1 {
		return 0xff
	}
	return uint8(v*255 + 0.5)
}

func quantize16(v float32) uint16 {
	if !(v > 0) {
		return 0
	}
	if v >= 1 {
		return 0xffff
	}
	return uint16(v*65535 + 0.5)
}
