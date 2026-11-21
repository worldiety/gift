package cms

import (
	"fmt"
	"math"
	"sort"
	"sync"
)

// Curve is a tone response curve: how a channel's encoded value maps to
// the linear light it stands for. A built-in or constructed profile has one
// per channel; a parsed profile has whatever its tags hold.
type Curve struct {
	// kind is the ICC parametric function type, 0 to 4, or -1 for a table.
	kind   int
	params [7]float64
	table  []uint16
}

// Gamma is the curve Y = X^g: 2.2 for Adobe RGB, 1.8 for ProPhoto.
func Gamma(g float64) Curve { return Curve{kind: 0, params: [7]float64{g}} }

// Linear is the identity curve of a linear colour space.
func Linear() Curve { return Gamma(1) }

// SRGBCurve is the curve of sRGB and Display P3: a power of 2.4 with a
// linear foot, IEC 61966-2-1.
func SRGBCurve() Curve {
	return Parametric(3, 2.4, 1/1.055, 0.055/1.055, 1/12.92, 0.04045)
}

// Rec709Curve is the curve of Rec. 709 and Rec. 2020 as cameras encode
// it: the inverse of the BT.709 OETF.
func Rec709Curve() Curve {
	return Parametric(3, 1/0.45, 1/1.099, 0.099/1.099, 1/4.5, 0.081)
}

// Parametric is one of the five parametric curves of the ICC
// specification, section 10.18, with its parameters in the order the
// specification lists them, g first:
//
//	0: Y = X^g
//	1: Y = (aX+b)^g for X >= -b/a, else 0
//	2: Y = (aX+b)^g + c for X >= -b/a, else c
//	3: Y = (aX+b)^g for X >= d, else cX
//	4: Y = (aX+b)^g + e for X >= d, else cX + f
//
// Missing parameters are 0; extra ones are ignored.
func Parametric(kind int, params ...float64) Curve {
	c := Curve{kind: kind}
	copy(c.params[:], params)
	return c
}

// Table is a sampled curve: n samples from X = 0 to 1, each a value from 0
// to 65535 standing for Y from 0 to 1, linearly interpolated.
func Table(samples []uint16) Curve {
	return Curve{kind: -1, table: append([]uint16(nil), samples...)}
}

func (c Curve) String() string {
	if c.kind < 0 {
		return fmt.Sprintf("Table(%d)", len(c.table))
	}
	return fmt.Sprintf("Parametric(%d, %v)", c.kind, c.params[:paramCount(c.kind)])
}

// paramCount is the number of parameters of a parametric curve type.
func paramCount(kind int) int {
	switch kind {
	case 0:
		return 1
	case 1:
		return 3
	case 2:
		return 4
	case 3:
		return 5
	case 4:
		return 7
	}
	return 0
}

// curve is the evaluated form of a Curve.
type curve interface {
	eval(x float64) float64
	// inverse maps a value back; it is only asked for values from 0 to 1.
	inverse(y float64) float64
}

// compile turns a Curve into something to evaluate.
func (c Curve) compile() (curve, error) {
	switch {
	case c.kind < 0:
		if len(c.table) < 2 {
			if len(c.table) == 1 {
				return newGamma(float64(c.table[0]) / 256)
			}
			return identityCurve{}, nil
		}
		return newSampled(c.table), nil
	case c.kind == 0:
		return newGamma(c.params[0])
	case c.kind <= 4:
		return newParametric(c.kind, c.params), nil
	}
	return nil, fmt.Errorf("cms: parametric curve type %d", c.kind)
}

type identityCurve struct{}

func (identityCurve) eval(x float64) float64    { return x }
func (identityCurve) inverse(y float64) float64 { return y }

type gammaCurve struct{ g, inv float64 }

func newGamma(g float64) (curve, error) {
	if g <= 0 || math.IsNaN(g) || math.IsInf(g, 0) {
		return nil, fmt.Errorf("cms: gamma %v", g)
	}
	if math.Abs(g-1) < 1.0/65535 {
		return identityCurve{}, nil
	}
	return gammaCurve{g, 1 / g}, nil
}

func (c gammaCurve) eval(x float64) float64 {
	if x <= 0 {
		return 0
	}
	return math.Pow(x, c.g)
}

func (c gammaCurve) inverse(y float64) float64 {
	if y <= 0 {
		return 0
	}
	return math.Pow(y, c.inv)
}

// parametricCurve is types 1 to 4; type 0 is gammaCurve. The inverses are
// those of LittleCMS (cmsgamma.c, DefaultEvalParametricFn), which differ
// from a strict reading of the specification at the edges, so that a
// transform here and one there agree.
type parametricCurve struct {
	kind                int
	g, a, b, c, d, e, f float64
}

func newParametric(kind int, p [7]float64) curve {
	return parametricCurve{kind: kind, g: p[0], a: p[1], b: p[2], c: p[3], d: p[4], e: p[5], f: p[6]}
}

func (p parametricCurve) eval(x float64) float64 {
	switch p.kind {
	case 1: // Y = (aX+b)^g for X >= -b/a, else 0
		if p.a != 0 && x >= -p.b/p.a {
			if e := p.a*x + p.b; e > 0 {
				return math.Pow(e, p.g)
			}
		}
		return 0
	case 2: // Y = (aX+b)^g + c for X >= -b/a, else c
		if math.Abs(p.a) < 1e-4 {
			return 0
		}
		if x >= max(-p.b/p.a, 0) {
			if e := p.a*x + p.b; e > 0 {
				return math.Pow(e, p.g) + p.c
			}
			return 0
		}
		return p.c
	case 3: // Y = (aX+b)^g for X >= d, else cX
		if x >= p.d {
			if e := p.a*x + p.b; e > 0 {
				return math.Pow(e, p.g)
			}
			return 0
		}
		return p.c * x
	default: // 4: Y = (aX+b)^g + e for X >= d, else cX + f
		if x >= p.d {
			if e := p.a*x + p.b; e > 0 {
				return math.Pow(e, p.g) + p.e
			}
			return p.e
		}
		return p.c*x + p.f
	}
}

func (p parametricCurve) inverse(y float64) float64 {
	inv := func(v float64) float64 {
		if v == 0 {
			return 0
		}
		return 1 / v
	}
	switch p.kind {
	case 1: // X = (Y^(1/g) - b) / a
		return max(0, (math.Pow(max(y, 0), inv(p.g))-p.b)*inv(p.a))
	case 2: // X = ((Y-c)^(1/g) - b) / a for Y >= c, else -b/a
		if e := y - p.c; e >= 0 {
			if e == 0 {
				return 0
			}
			return (math.Pow(e, inv(p.g)) - p.b) * inv(p.a)
		}
		return -p.b * inv(p.a)
	case 3: // X = (Y^(1/g) - b) / a for Y >= (ad+b)^g, else Y/c
		if y < math.Pow(max(p.a*p.d+p.b, 0), p.g) {
			return y * inv(p.c)
		}
		return (math.Pow(y, inv(p.g)) - p.b) * inv(p.a)
	default: // 4: X = ((Y-e)^(1/g) - b) / a for Y >= (ad+b)^g + e, else (Y-f)/c
		if y < math.Pow(max(p.a*p.d+p.b, 0), p.g)+p.e {
			return (y - p.f) * inv(p.c)
		}
		if e := y - p.e; e > 0 {
			return (math.Pow(e, inv(p.g)) - p.b) * inv(p.a)
		}
		return 0
	}
}

// sampledCurve is a table curve with linear interpolation. Its inverse is
// a reversed table of 4096 entries, built as LittleCMS builds it
// (cmsReverseToneCurveEx), so that both reverse a non-monotonic or flat
// table the same way. It is built the first time it is needed, as only the
// curves of a matrix profile are ever inverted, not those of tables.
type sampledCurve struct {
	y    []float64
	once sync.Once
	rev  []float64
}

const reverseSamples = 4096

func newSampled(t []uint16) curve {
	c := &sampledCurve{y: make([]float64, len(t))}
	for i, v := range t {
		c.y[i] = float64(v) / 65535
	}
	if c.isIdentity() {
		return identityCurve{}
	}
	return c
}

func (c *sampledCurve) isIdentity() bool {
	n := float64(len(c.y) - 1)
	for i, v := range c.y {
		if math.Abs(v-float64(i)/n) > 1.0/65535 {
			return false
		}
	}
	return true
}

func (c *sampledCurve) eval(x float64) float64 { return lerpTable(c.y, x) }

func (c *sampledCurve) inverse(y float64) float64 {
	c.once.Do(func() { c.rev = reverse(c.y) })
	return lerpTable(c.rev, y)
}

// lerpTable interpolates the table t, spread evenly over 0 to 1, at x.
func lerpTable(t []float64, x float64) float64 {
	x = unit(x) * float64(len(t)-1)
	i := int(x)
	if i >= len(t)-1 {
		return t[len(t)-1]
	}
	f := x - float64(i)
	return t[i] + f*(t[i+1]-t[i])
}

// reverse samples the inverse of the table y at reverseSamples points.
func reverse(y []float64) []float64 {
	n := len(y) - 1
	ascending := y[0] <= y[n]
	find := monotonicInterval(y, ascending)
	if find == nil {
		// A table that goes up and down is searched segment by segment,
		// as LittleCMS does; a long one is resampled first, so that the
		// search stays bounded whatever a profile holds.
		if len(y) > reverseSamples+1 {
			r := make([]float64, reverseSamples+1)
			for i := range r {
				r[i] = lerpTable(y, float64(i)/reverseSamples)
			}
			y, n = r, reverseSamples
		}
		find = func(v float64) int { return interval(y, v, ascending) }
	}
	out := make([]float64, reverseSamples)
	var a, b float64
	for i := range out {
		v := float64(i) / (reverseSamples - 1)
		if j := find(v); j >= 0 {
			x1, x2 := float64(j)/float64(n), float64(j+1)/float64(n)
			y1, y2 := y[j], y[j+1]
			if y1 == y2 {
				if ascending {
					out[i] = x2
				} else {
					out[i] = x1
				}
				continue
			}
			a = (x2 - x1) / (y2 - y1)
			b = x2 - a*y2
		}
		out[i] = unit(a*v + b)
	}
	return out
}

// monotonicInterval is interval by binary search for a table that only
// rises or only falls, or nil for one that does neither.
func monotonicInterval(y []float64, ascending bool) func(v float64) int {
	n := len(y) - 1
	for i := range n {
		if (ascending && y[i+1] < y[i]) || (!ascending && y[i+1] > y[i]) {
			return nil
		}
	}
	if ascending {
		// The highest segment holding v: the last sample at most v.
		return func(v float64) int {
			k := sort.Search(len(y), func(i int) bool { return y[i] > v }) - 1
			switch {
			case k < 0:
				return -1
			case k == n:
				if y[n] == v {
					return n - 1
				}
				return -1
			}
			return k
		}
	}
	// The lowest segment holding v: just before the first sample at most v.
	return func(v float64) int {
		m := sort.Search(len(y), func(i int) bool { return y[i] <= v })
		switch {
		case m == 0:
			if y[0] == v {
				return 0
			}
			return -1
		case m > n:
			return -1
		}
		return m - 1
	}
}

// interval finds the segment of the table that holds v: from the top for
// an ascending table, from the bottom for a descending one, as LittleCMS
// does. It returns -1 where none does.
func interval(y []float64, v float64, ascending bool) int {
	in := func(i int) bool {
		lo, hi := y[i], y[i+1]
		if hi < lo {
			lo, hi = hi, lo
		}
		return v >= lo && v <= hi
	}
	if ascending {
		for i := len(y) - 2; i >= 0; i-- {
			if in(i) {
				return i
			}
		}
		return -1
	}
	for i := 0; i < len(y)-1; i++ {
		if in(i) {
			return i
		}
	}
	return -1
}
