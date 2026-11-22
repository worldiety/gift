package raw

import (
	"math"
	"sort"
)

// toneCurve is a tone curve as a table: the curve at xmax·(i/(n-1))², the
// square putting the samples where the curve bends, near black.
type toneCurve struct {
	xmax float32
	// scale is (n-1)/sqrt(xmax), which turns sqrt(x) into an index.
	scale float32
	y     []float32
}

const toneSamples = 4096

// newToneCurve tabulates f from 0 to xmax; above xmax the curve is f(xmax).
func newToneCurve(f func(float64) float64, xmax float64) *toneCurve {
	t := &toneCurve{xmax: float32(xmax), y: make([]float32, toneSamples)}
	t.scale = float32((toneSamples - 1) / math.Sqrt(xmax))
	for i := range t.y {
		u := float64(i) / (toneSamples - 1)
		t.y[i] = float32(f(xmax * u * u))
	}
	return t
}

// at is the curve at x.
func (t *toneCurve) at(x float32) float32 {
	if !(x > 0) {
		return t.y[0]
	}
	if x >= t.xmax {
		return t.y[len(t.y)-1]
	}
	f := float32(math.Sqrt(float64(x))) * t.scale
	i := int(f)
	if i >= len(t.y)-1 {
		return t.y[len(t.y)-1]
	}
	w := f - float32(i)
	return t.y[i] + w*(t.y[i+1]-t.y[i])
}

// rgb applies the curve to a colour as Adobe's raw converters do, keeping
// its hue: the largest and the smallest channel through the curve, the
// middle one where it was between them. Saturation follows the curve's
// slope: it grows where the curve is steep and gives way near white, where
// the curve flattens and the channels come together.
func (t *toneCurve) rgb(r, g, b float32) (float32, float32, float32) {
	switch {
	case r >= g && g >= b:
		r, g, b = t.three(r, g, b)
	case r >= b && b >= g:
		r, b, g = t.three(r, b, g)
	case g >= r && r >= b:
		g, r, b = t.three(g, r, b)
	case g >= b && b >= r:
		g, b, r = t.three(g, b, r)
	case b >= r && r >= g:
		b, r, g = t.three(b, r, g)
	default:
		b, g, r = t.three(b, g, r)
	}
	return r, g, b
}

// three maps hi ≥ mid ≥ lo.
func (t *toneCurve) three(hi, mid, lo float32) (float32, float32, float32) {
	h, l := t.at(hi), t.at(lo)
	if hi <= lo {
		return h, h, h
	}
	return h, l + (h-l)*(mid-lo)/(hi-lo), l
}

// The default tone curve of this package, for linear light in which the
// camera's white, the level the first channel clips at, is 1: a
// log-logistic curve, an S in stops, x^c/(x^c+k^c), scaled so that it
// reaches 1 at the scene's value toneWhite and holds there.
var (
	// toneContrast is c: the slope in stops in the shadows, half of it at
	// k.
	toneContrast = 1.52
	// toneKnee is k, the scene's value the curve gives half of its height.
	toneKnee = 0.22
	// toneWhite is the scene's value the curve makes white: a little
	// above the camera's, so that highlights that were reconstructed
	// above it keep some of their shape.
	toneWhite = 3.0
)

// defaultTone is the default curve over the scene's values up to its white.
func defaultTone() *toneCurve {
	c, k, w := toneContrast, toneKnee, toneWhite
	kc := math.Pow(k, c)
	s := (math.Pow(w, c) + kc) / math.Pow(w, c)
	return newToneCurve(func(x float64) float64 {
		xc := math.Pow(x, c)
		return min(s*xc/(xc+kc), 1)
	}, w)
}

// profileTone is the tone curve of a camera profile: pairs of input and
// output from 0 to 1, interpolated by a monotone cubic (Fritsch and
// Carlson), so that it does not overshoot between its points.
func profileTone(pairs []float64) *toneCurve {
	n := len(pairs) / 2
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i], y[i] = pairs[2*i], pairs[2*i+1]
	}
	slope := func(k int) float64 { return (y[k+1] - y[k]) / (x[k+1] - x[k]) }
	tangent := make([]float64, n)
	for k := range n {
		switch {
		case k == 0:
			tangent[k] = slope(0)
		case k == n-1:
			tangent[k] = slope(n - 2)
		default:
			a, b := slope(k-1), slope(k)
			if a*b > 0 {
				tangent[k] = 2 / (1/a + 1/b)
			}
		}
	}
	return newToneCurve(func(v float64) float64 {
		i := sort.SearchFloat64s(x, v) - 1
		i = min(max(i, 0), n-2)
		h := x[i+1] - x[i]
		t := (v - x[i]) / h
		t2, t3 := t*t, t*t*t
		r := (2*t3-3*t2+1)*y[i] + (t3-2*t2+t)*h*tangent[i] +
			(-2*t3+3*t2)*y[i+1] + (t3-t2)*h*tangent[i+1]
		return min(max(r, 0), 1)
	}, 1)
}

// Gamut compression: a colour whose smallest channel lies less than
// gamutKnee of the way from its largest to zero keeps its saturation; the
// distances beyond, up to gamutLimit, are compressed into what remains to
// zero by a curve of power gamutPower, and those beyond the limit clip at
// zero. A colour at the edge of the space keeps 98 percent of its
// saturation.
const (
	gamutKnee  = 0.9
	gamutLimit = 1.3
	gamutPower = 3
)

// gamutScale makes the compression curve reach 1 at the limit.
var gamutScale = float32((gamutLimit - gamutKnee) * (1 - gamutKnee) /
	math.Pow(math.Pow(gamutLimit-gamutKnee, gamutPower)-math.Pow(1-gamutKnee, gamutPower), 1.0/gamutPower))

// compress brings a colour into the space it is to be shown in, as
// [inGamut] does, and scales a colour brighter than 1 down whole.
func compress(r, g, b float32) (float32, float32, float32) {
	r, g, b = inGamut(r, g, b)
	if a := max(r, g, b); a > 1 {
		r, g, b = r/a, g/a, b/a
	}
	return r, g, b
}

// inGamut brings a colour with a channel below zero, a colour more
// saturated than the space holds, into it: the channels move towards the
// largest, along with those that come close to it, so that the hue stays
// and the gradation of saturated colours does not end in a clipped edge.
func inGamut(r, g, b float32) (float32, float32, float32) {
	a := max(r, g, b)
	if !(a > 0) {
		return 0, 0, 0
	}
	if min(r, g, b) < (1-gamutKnee)*a {
		r = a - a*squeeze((a-r)/a)
		g = a - a*squeeze((a-g)/a)
		b = a - a*squeeze((a-b)/a)
	}
	return r, g, b
}

// squeeze compresses a distance d from the achromatic, 1 at the edge of the
// gamut, into [0, 1].
func squeeze(d float32) float32 {
	if d <= gamutKnee {
		return d
	}
	x := (d - gamutKnee) / gamutScale
	x3 := x * x * x
	y := gamutKnee + gamutScale*x/float32(math.Cbrt(float64(1+x3)))
	return min(y, 1)
}
