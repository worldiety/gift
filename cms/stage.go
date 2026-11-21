package cms

// vec is a colour of up to four channels on its way through a pipeline.
type vec [4]float64

// stage is one step of a pipeline: curves, a matrix, a colour table, a
// change of encoding. Stages are immutable and safe for concurrent use.
type stage interface {
	// channels is how many channels the stage takes and gives.
	channels() (in, out int)
	apply(v vec) vec
}

// pipeline is a sequence of stages, evaluated in float64. It is the exact
// form of a transform; a Transform bakes it into tables for pixels.
type pipeline []stage

func (p pipeline) apply(v vec) vec {
	for _, s := range p {
		v = s.apply(v)
	}
	return v
}

// channels is how many channels the pipeline takes and gives, or 0, 0 for
// an empty one.
func (p pipeline) channels() (in, out int) {
	if len(p) == 0 {
		return 0, 0
	}
	in, _ = p[0].channels()
	_, out = p[len(p)-1].channels()
	return in, out
}

// fits reports whether every stage takes what the one before gives.
func (p pipeline) fits(in, out int) bool {
	n := in
	for _, s := range p {
		i, o := s.channels()
		if i != n {
			return false
		}
		n = o
	}
	return n == out
}

// optimized merges neighbouring affine stages into one and drops those that
// do nothing, which is most of the encoding changes a link of two profiles
// is made of.
func (p pipeline) optimized() pipeline {
	var r pipeline
	for _, s := range p {
		a, ok := s.(affine)
		if !ok {
			r = append(r, s)
			continue
		}
		if len(r) > 0 {
			if prev, ok := r[len(r)-1].(affine); ok {
				r[len(r)-1] = prev.then(a)
				continue
			}
		}
		r = append(r, a)
	}
	out := r[:0]
	for _, s := range r {
		if a, ok := s.(affine); ok && a.isIdentity() {
			continue
		}
		out = append(out, s)
	}
	return out
}

// curves applies a curve to each channel, or its inverse.
type curves struct {
	c       []curve
	inverse bool
}

func (s curves) channels() (int, int) { return len(s.c), len(s.c) }

func (s curves) apply(v vec) vec {
	for i, c := range s.c {
		if s.inverse {
			// The input of an inverse curve is clipped to its domain, ICC
			// specification annex F.3.
			v[i] = c.inverse(unit(v[i]))
		} else {
			v[i] = c.eval(v[i])
		}
	}
	return v
}

// newCurves is the stage for cs, or nil when every one is the identity.
func newCurves(cs []curve, inverse bool) stage {
	for _, c := range cs {
		if _, ok := c.(identityCurve); !ok {
			return curves{cs, inverse}
		}
	}
	return nil
}

// affine is a 3 by 3 matrix and an offset: XYZ from RGB, an encoding of
// the connection space, black point compensation.
type affine struct {
	m   mat3
	off [3]float64
}

func scale3(a, b, c float64) affine { return affine{m: diag(a, b, c)} }

func (affine) channels() (int, int) { return 3, 3 }

func (s affine) apply(v vec) vec {
	x, y, z := s.m.apply(v[0], v[1], v[2])
	return vec{x + s.off[0], y + s.off[1], z + s.off[2]}
}

// then is s followed by n.
func (s affine) then(n affine) affine {
	x, y, z := n.m.apply(s.off[0], s.off[1], s.off[2])
	return affine{m: n.m.mul(s.m), off: [3]float64{x + n.off[0], y + n.off[1], z + n.off[2]}}
}

func (s affine) isIdentity() bool {
	return s.m.isIdentity(1e-12) && s.off == [3]float64{}
}

// fromGray spreads one channel over the three of the connection space:
// Y times the white for XYZ, L* with neutral a* b* for Lab.
type fromGray struct{ scale, off [3]float64 }

func (fromGray) channels() (int, int) { return 1, 3 }

func (s fromGray) apply(v vec) vec {
	g := v[0]
	return vec{g*s.scale[0] + s.off[0], g*s.scale[1] + s.off[1], g*s.scale[2] + s.off[2]}
}

// toGray picks one channel out of three: Y of XYZ, L* of Lab.
type toGray struct{ w [3]float64 }

func (toGray) channels() (int, int) { return 3, 1 }

func (s toGray) apply(v vec) vec {
	return vec{s.w[0]*v[0] + s.w[1]*v[1] + s.w[2]*v[2]}
}

// labToXYZStage converts Lab to XYZ, both relative to D50.
type labToXYZStage struct{}

func (labToXYZStage) channels() (int, int) { return 3, 3 }

func (labToXYZStage) apply(v vec) vec {
	x, y, z := labToXYZ(D50, v[0], v[1], v[2])
	return vec{x, y, z}
}

// xyzToLabStage converts XYZ to Lab, both relative to D50.
type xyzToLabStage struct{}

func (xyzToLabStage) channels() (int, int) { return 3, 3 }

func (xyzToLabStage) apply(v vec) vec {
	l, a, b := xyzToLab(D50, v[0], v[1], v[2])
	return vec{l, a, b}
}

// clut is a colour lookup table: a grid over the input channels with a
// colour of the output channels at every point, interpolated in between.
// The first input channel varies slowest, ICC specification 10.12.3.
type clut struct {
	in, out   int
	grid      []int
	stride    []int
	samples   []float64
	trilinear bool
}

func newCLUT(grid []int, out int, samples []float64) *clut {
	c := &clut{in: len(grid), out: out, grid: grid, stride: make([]int, len(grid)), samples: samples}
	s := out
	for i := len(grid) - 1; i >= 0; i-- {
		c.stride[i] = s
		s *= grid[i]
	}
	return c
}

func (c *clut) channels() (int, int) { return c.in, c.out }

func (c *clut) apply(v vec) vec {
	var r vec
	switch {
	case c.in == 3 && !c.trilinear:
		c.tetrahedral(c.samples, 0, v[0], v[1], v[2], &r)
	case c.in == 4:
		// Two tetrahedral interpolations in the last three channels and a
		// linear one between them in the first, as LittleCMS does it.
		p := unit(v[0]) * float64(c.grid[0]-1)
		k := int(p)
		f := p - float64(k)
		k1 := k
		if k < c.grid[0]-1 {
			k1++
		}
		var lo, hi vec
		c.tetrahedral(c.samples, k*c.stride[0], v[1], v[2], v[3], &lo)
		c.tetrahedral(c.samples, k1*c.stride[0], v[1], v[2], v[3], &hi)
		for o := range c.out {
			r[o] = lo[o] + (hi[o]-lo[o])*f
		}
	default:
		c.multilinear(v, &r)
	}
	return r
}

// tetrahedral interpolates in the cube of the last three input channels
// of the table, starting at the sample base.
func (c *clut) tetrahedral(s []float64, base int, x, y, z float64, r *vec) {
	n := len(c.grid)
	gx, gy, gz := c.grid[n-3], c.grid[n-2], c.grid[n-1]
	sx, sy, sz := c.stride[n-3], c.stride[n-2], c.stride[n-1]
	px := unit(x) * float64(gx-1)
	py := unit(y) * float64(gy-1)
	pz := unit(z) * float64(gz-1)
	x0, y0, z0 := int(px), int(py), int(pz)
	rx, ry, rz := px-float64(x0), py-float64(y0), pz-float64(z0)
	X0, Y0, Z0 := base+x0*sx, y0*sy, z0*sz
	X1, Y1, Z1 := X0, Y0, Z0
	if x0 < gx-1 {
		X1 += sx
	}
	if y0 < gy-1 {
		Y1 += sy
	}
	if z0 < gz-1 {
		Z1 += sz
	}
	c0 := X0 + Y0 + Z0
	var a1, b1, a2, b2, a3, b3 int
	switch {
	case rx >= ry && ry >= rz:
		a1, b1 = X1+Y0+Z0, c0
		a2, b2 = X1+Y1+Z0, X1+Y0+Z0
		a3, b3 = X1+Y1+Z1, X1+Y1+Z0
	case rx >= rz && rz >= ry:
		a1, b1 = X1+Y0+Z0, c0
		a2, b2 = X1+Y1+Z1, X1+Y0+Z1
		a3, b3 = X1+Y0+Z1, X1+Y0+Z0
	case rz >= rx && rx >= ry:
		a1, b1 = X1+Y0+Z1, X0+Y0+Z1
		a2, b2 = X1+Y1+Z1, X1+Y0+Z1
		a3, b3 = X0+Y0+Z1, c0
	case ry >= rx && rx >= rz:
		a1, b1 = X1+Y1+Z0, X0+Y1+Z0
		a2, b2 = X0+Y1+Z0, c0
		a3, b3 = X1+Y1+Z1, X1+Y1+Z0
	case ry >= rz && rz >= rx:
		a1, b1 = X1+Y1+Z1, X0+Y1+Z1
		a2, b2 = X0+Y1+Z0, c0
		a3, b3 = X0+Y1+Z1, X0+Y1+Z0
	default: // rz >= ry && ry >= rx
		a1, b1 = X1+Y1+Z1, X0+Y1+Z1
		a2, b2 = X0+Y1+Z1, X0+Y0+Z1
		a3, b3 = X0+Y0+Z1, c0
	}
	for o := range c.out {
		r[o] = s[c0+o] + (s[a1+o]-s[b1+o])*rx + (s[a2+o]-s[b2+o])*ry + (s[a3+o]-s[b3+o])*rz
	}
}

// multilinear interpolates between the 2^n corners of the cell around v.
func (c *clut) multilinear(v vec, r *vec) {
	var idx [4]int
	var w [4]float64
	for i := range c.in {
		p := unit(v[i]) * float64(c.grid[i]-1)
		k := int(p)
		f := p - float64(k)
		if k >= c.grid[i]-1 {
			k, f = c.grid[i]-2, 1
			if k < 0 {
				k, f = 0, 0
			}
		}
		idx[i], w[i] = k, f
	}
	for corner := range 1 << c.in {
		weight, base := 1.0, 0
		for i := range c.in {
			k := idx[i]
			if corner>>i&1 == 1 {
				weight *= w[i]
				k++
			} else {
				weight *= 1 - w[i]
			}
			base += k * c.stride[i]
		}
		if weight == 0 {
			continue
		}
		for o := range c.out {
			r[o] += c.samples[base+o] * weight
		}
	}
}
