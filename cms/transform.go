package cms

import (
	"errors"
	"fmt"
	"math"
)

// Options are the choices of a transform beyond its two profiles.
type Options struct {
	// Intent is the rendering intent. The zero value is Perceptual, which
	// a profile without perceptual tables treats as RelativeColorimetric.
	Intent Intent

	// BlackPointCompensation maps the darkest colour of the source to the
	// darkest of the destination, so that shadows keep their detail rather
	// than clip to black. It has no effect with AbsoluteColorimetric, and
	// for Perceptual and Saturation it is always on when the destination
	// is an ICC v4 profile, as the specification has it and LittleCMS does.
	BlackPointCompensation bool
}

// Transform converts colours from one profile's device space to
// another's. Making one evaluates the profiles' tables and bakes them into
// a lookup table, which takes a few milliseconds; using one is fast and
// safe from many goroutines at once.
type Transform struct {
	src, dst Space
	in, out  int
	exact    pipeline
	k        kernel
	identity bool
}

// NewTransform links the device space of src to that of dst through their
// profile connection spaces.
//
// The colour science follows LittleCMS 2 (cmscnvrt.c), so that a picture
// converted here and one converted by an application built on LittleCMS
// agree to the last bit or two: the choice of tables per intent, the v2 and
// v4 encodings of Lab, black point detection and compensation, and media
// white points for the absolute intent.
func NewTransform(src, dst *Profile, o Options) (*Transform, error) {
	if src == nil || dst == nil {
		return nil, errors.New("cms: transform without a profile")
	}
	if o.Intent > AbsoluteColorimetric {
		return nil, fmt.Errorf("cms: %v", o.Intent)
	}
	pl, err := link(src, dst, o.Intent, o.BlackPointCompensation)
	if err != nil {
		return nil, err
	}
	t := &Transform{src: src.space, dst: dst.space, in: src.space.Channels(), out: dst.space.Channels(), exact: pl.optimized()}
	if !t.exact.fits(t.in, t.out) && len(t.exact) > 0 {
		return nil, fmt.Errorf("cms: %s to %s does not fit", src.space, dst.space)
	}
	t.k = bake(t.exact, t.in, t.out)
	t.identity = t.isIdentity()
	return t, nil
}

// link builds the exact pipeline from src to dst: src to its connection
// space, the conversion between the two connection spaces with absolute
// adaptation or black point compensation, and on to dst.
func link(src, dst *Profile, intent Intent, compensate bool) (pipeline, error) {
	in, err := src.toPCS(intent)
	if err != nil {
		return nil, err
	}
	out, err := dst.fromPCS(intent)
	if err != nil {
		return nil, err
	}
	a := affine{m: identity3}
	switch {
	case intent == AbsoluteColorimetric:
		// The observer is fully adapted (LittleCMS's adaptation state of
		// 1): the media whites are scaled into each other.
		wi, wo := src.MediaWhitePoint(), dst.MediaWhitePoint()
		a.m = diag(wi.X/wo.X, wi.Y/wo.Y, wi.Z/wo.Z)
	default:
		if (intent == Perceptual || intent == Saturation) && dst.version >= 0x04000000 {
			compensate = true
		}
		if compensate {
			bi, bo := src.blackPoint(intent), dst.destinationBlackPoint(intent)
			if bi != bo {
				a = bpc(bi, bo)
			}
		}
	}
	pl := append(pipeline(nil), in...)
	pl = append(pl, connect(src.pcs, dst.pcs, a)...)
	return append(pl, out...), nil
}

// connect is the conversion from one connection space to another with the
// adjustment a, which is done in XYZ.
func connect(from, to Space, a affine) pipeline {
	if a.isIdentity() {
		switch {
		case from == to:
			return nil
		case from == SpaceLab:
			return pipeline{labToXYZStage{}}
		default:
			return pipeline{xyzToLabStage{}}
		}
	}
	var pl pipeline
	if from == SpaceLab {
		pl = append(pl, labToXYZStage{})
	}
	pl = append(pl, a)
	if to == SpaceLab {
		pl = append(pl, xyzToLabStage{})
	}
	return pl
}

// Source and Destination are the device spaces the transform converts
// between.
func (t *Transform) Source() Space      { return t.src }
func (t *Transform) Destination() Space { return t.dst }

// Identity reports whether the transform changes no colour by as much as
// a quarter of an 8 bit step, as one from sRGB to sRGB does, so that a
// caller can skip it.
func (t *Transform) Identity() bool { return t.identity }

// Shaper is a transform between two matrix profiles written out as what it
// is: a curve for each input channel, a 3 by 3 matrix with an offset, and an
// inverse curve for each output channel. It is for a renderer that converts
// on the GPU, where evaluating these is exact and a lookup table would
// interpolate; see [Transform.Shaper].
type Shaper struct {
	// In are the input curves, from the encoded values of the source to
	// linear light, sampled at len-1 even steps from 0 to 1.
	In [3][]float32
	// Matrix turns linear source light into linear destination light, row
	// by row, and Offset is added to the result; it is not zero only with
	// black point compensation.
	Matrix [9]float32
	Offset [3]float32
	// Out are the output curves, from linear light to the encoded values of
	// the destination, sampled at the squares of len-1 even steps: Out[c][i]
	// is the value at linear (i/(len-1))². The square puts the samples where
	// an inverse curve bends, near black.
	Out [3][]float32
}

// Shaper returns the transform as a [Shaper] when it converts between two
// matrix profiles, an RGB colour space and a display say, and false
// otherwise. The slices are the transform's own and must not be changed.
func (t *Transform) Shaper() (Shaper, bool) {
	k, ok := t.k.(*matrixShaper)
	if !ok {
		return Shaper{}, false
	}
	return Shaper{In: k.in, Matrix: k.m, Offset: k.off, Out: k.out}, true
}

// Color converts one colour exactly, in float64 and without the lookup
// table: in has a value from 0 to 1 per channel of the source space, the
// result one per channel of the destination.
func (t *Transform) Color(in ...float64) []float64 {
	var v vec
	copy(v[:t.in], in)
	v = t.exact.apply(v)
	return append([]float64(nil), v[:t.out]...)
}

func (t *Transform) isIdentity() bool {
	if t.in != t.out {
		return false
	}
	const tolerance = 0.25 / 255
	ok := true
	sample(t.in, 9, func(v vec) {
		if !ok {
			return
		}
		r := t.exact.apply(v)
		for i := range t.in {
			if math.Abs(r[i]-v[i]) > tolerance {
				ok = false
			}
		}
	})
	return ok
}

// sample calls f for every point of a grid of g points per channel over
// the unit cube of n channels.
func sample(n, g int, f func(v vec)) {
	var v vec
	var walk func(c int)
	walk = func(c int) {
		if c == n {
			f(v)
			return
		}
		for i := range g {
			v[c] = float64(i) / float64(g-1)
			walk(c + 1)
		}
	}
	walk(0)
}
