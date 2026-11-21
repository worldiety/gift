package cms

import (
	"math"
	"runtime"
	"sync"
)

// kernel is a transform baked into tables, the form pixels go through. It
// takes and gives float32 from 0 to 1, as many per colour as the spaces
// have channels.
type kernel interface {
	run(out, in []float32, n int)
}

// The sizes of the baked tables. 49 points per channel is LittleCMS's
// grid for 3 channels at high resolution: with 33, its default, the table
// of a transform into a table profile was a few 8 bit steps further from
// the exact pipeline than LittleCMS's own optimised tables; with 49 it is
// as close or closer in every case of TestAgainstLittleCMS, for 1.4 MB and
// about twice the time to bake. 49 holds every point of a grid of 9, 13,
// 17 or 25, which pre-linearization relies on. 4 channels have 17, as in
// LittleCMS. Curves are sampled at 4096 intervals.
const (
	grid3    = 49
	grid4    = 17
	curveLen = 4096
)

// bake chooses the kernel for an exact pipeline: curves, matrix and
// inverse curves stay that, as tables of their own, when that is all there
// is between two RGB spaces; anything else is sampled into a lookup table.
//
// Curves at the end of the pipeline stay out of the table and follow it as
// curve tables (LittleCMS's post-linearization). They are where a
// transform bends most, the encoding of sRGB or the output curves of a
// printer, and interpolating across a bend between grid points is what
// would put dark colours several 8 bit steps off.
//
// The input curves of a source's own table go before ours in the same way
// (pre-linearization), which lays our grid over the source's: 33 points
// hold every one of a table of 9 or 17, so its edges fall on ours instead
// of across them. The curves of a matrix source stay in the table, as
// their linear light would crowd the dark half of the input into two
// cells.
func bake(p pipeline, in, out int) kernel {
	if in == 3 && out == 3 {
		if k, ok := newMatrixShaper(p); ok {
			return k
		}
	}
	if in == 1 {
		return newLut1(p, out)
	}
	var pre, post *curves
	if len(p) > 1 {
		if c, ok := p[0].(curves); ok && !c.inverse {
			if _, ok := p[1].(*clut); ok {
				pre, p = &c, p[1:]
			}
		}
	}
	if n := len(p); n > 0 {
		if c, ok := p[n-1].(curves); ok {
			post, p = &c, p[:n-1]
		}
	}
	g := grid3
	if in == 4 {
		g = grid4
	}
	k := newClutKernel(p, in, out, g)
	if pre != nil {
		k.pre = make([][]float32, in)
		for c := range in {
			k.pre[c] = make([]float32, curveLen+1)
			for j := range curveLen + 1 {
				k.pre[c][j] = float32(pre.c[c].eval(float64(j) / curveLen))
			}
		}
	}
	if post != nil {
		k.post = make([][]float32, out)
		for c := range out {
			k.post[c] = shapeTable(post.c[c], post.inverse)
		}
	}
	return k
}

// shapeTable samples a curve, or its inverse, for shape: at the squares of
// evenly spaced values.
func shapeTable(c curve, inverse bool) []float32 {
	t := make([]float32, curveLen+1)
	for j := range t {
		u := float64(j) / curveLen
		if inverse {
			t[j] = float32(c.inverse(u * u))
		} else {
			t[j] = float32(c.eval(u * u))
		}
	}
	return t
}

// matrixShaper is curves, a 3 by 3 matrix and inverse curves: the whole of
// a transform between two matrix profiles, sRGB to Display P3 say. The
// output table is indexed by the square root of the linear value: the
// inverse curves rise steeply near black (a pure gamma infinitely so), and
// spacing the samples by the square root puts them where the curve bends,
// which keeps the error far below an 8 bit step without a power per pixel.
type matrixShaper struct {
	in  [3][]float32
	in8 [3][256]float32
	m   [9]float32
	off [3]float32
	out [3][]float32
}

func newMatrixShaper(p pipeline) (*matrixShaper, bool) {
	var fwd, inv []curve
	a := affine{m: identity3}
	i := 0
	if i < len(p) {
		if c, ok := p[i].(curves); ok && !c.inverse {
			fwd, i = c.c, i+1
		}
	}
	if i < len(p) {
		if s, ok := p[i].(affine); ok {
			a, i = s, i+1
		}
	}
	if i < len(p) {
		if c, ok := p[i].(curves); ok && c.inverse {
			inv, i = c.c, i+1
		}
	}
	if i != len(p) {
		return nil, false
	}
	k := &matrixShaper{}
	for c := range 3 {
		k.in[c] = make([]float32, curveLen+1)
		for j := range curveLen + 1 {
			x := float64(j) / curveLen
			if fwd != nil {
				k.in[c][j] = float32(fwd[c].eval(x))
			} else {
				k.in[c][j] = float32(x)
			}
		}
		if inv != nil {
			k.out[c] = shapeTable(inv[c], true)
		} else {
			k.out[c] = shapeTable(identityCurve{}, false)
		}
		for j := range 256 {
			x := float64(j) / 255
			if fwd != nil {
				k.in8[c][j] = float32(fwd[c].eval(x))
			} else {
				k.in8[c][j] = float32(x)
			}
		}
	}
	for r := range 3 {
		for c := range 3 {
			k.m[3*r+c] = float32(a.m[r][c])
		}
		k.off[r] = float32(a.off[r])
	}
	return k, true
}

// lerp32 interpolates the table t, spread evenly over 0 to 1, at x.
func lerp32(t []float32, x float32) float32 {
	if !(x > 0) {
		return t[0]
	}
	if x >= 1 {
		return t[len(t)-1]
	}
	x *= float32(len(t) - 1)
	i := int(x)
	f := x - float32(i)
	return t[i] + f*(t[i+1]-t[i])
}

// shape is the output curve of one channel at the linear value x.
func shape(t []float32, x float32) float32 {
	if !(x > 0) {
		return t[0]
	}
	if x >= 1 {
		return t[len(t)-1]
	}
	return lerp32(t, float32(math.Sqrt(float64(x))))
}

func (k *matrixShaper) linear(r, g, b float32) (float32, float32, float32) {
	m := &k.m
	return m[0]*r + m[1]*g + m[2]*b + k.off[0],
		m[3]*r + m[4]*g + m[5]*b + k.off[1],
		m[6]*r + m[7]*g + m[8]*b + k.off[2]
}

func (k *matrixShaper) run(out, in []float32, n int) {
	for p := range n {
		i := in[3*p : 3*p+3]
		x, y, z := k.linear(lerp32(k.in[0], i[0]), lerp32(k.in[1], i[1]), lerp32(k.in[2], i[2]))
		o := out[3*p : 3*p+3]
		o[0], o[1], o[2] = shape(k.out[0], x), shape(k.out[1], y), shape(k.out[2], z)
	}
}

// run8 is the fast path for 8 bit RGB and straight RGBA, which is what
// almost every photograph is: src and dst hold n pixels of ss and ds bytes.
func (k *matrixShaper) run8(dst []byte, ds int, src []byte, ss int, n int) {
	for p := range n {
		s := src[p*ss : p*ss+ss]
		x, y, z := k.linear(k.in8[0][s[0]], k.in8[1][s[1]], k.in8[2][s[2]])
		d := dst[p*ds : p*ds+ds]
		d[0] = to8(shape(k.out[0], x))
		d[1] = to8(shape(k.out[1], y))
		d[2] = to8(shape(k.out[2], z))
		if ds == 4 {
			if ss == 4 {
				d[3] = s[3]
			} else {
				d[3] = 255
			}
		}
	}
}

func to8(v float32) byte {
	if !(v > 0) {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return byte(v*255 + 0.5)
}

// lut1 is a transform from one channel, a table along it.
type lut1 struct {
	out int
	t   []float32
}

func newLut1(p pipeline, out int) *lut1 {
	k := &lut1{out: out, t: make([]float32, (curveLen+1)*out)}
	for j := range curveLen + 1 {
		v := p.apply(vec{float64(j) / curveLen})
		for o := range out {
			k.t[j*out+o] = float32(v[o])
		}
	}
	return k
}

func (k *lut1) run(out, in []float32, n int) {
	for p := range n {
		x := unit(in[p]) * curveLen
		j := int(x)
		f := x - float32(j)
		if j >= curveLen {
			j, f = curveLen-1, 1
		}
		a, b := k.t[j*k.out:], k.t[(j+1)*k.out:]
		for o := range k.out {
			out[p*k.out+o] = a[o] + f*(b[o]-a[o])
		}
	}
}

// clutKernel is a transform sampled on a grid over 3 or 4 input channels
// and interpolated tetrahedrally, the 4 channel case as two tetrahedral
// interpolations and a linear one between them.
type clutKernel struct {
	in, out, g int
	stride     [4]int
	s          []float32
	// pre are the curves before the table, per input channel, for lerp32;
	// post are the curves after it, per output channel, for shape.
	pre, post [][]float32
}

func newClutKernel(p pipeline, in, out, g int) *clutKernel {
	k := &clutKernel{in: in, out: out, g: g}
	s := out
	for i := in - 1; i >= 0; i-- {
		k.stride[i] = s
		s *= g
	}
	k.s = make([]float32, s)
	// The slices of the first channel are sampled in parallel: a table of
	// CMYK is 83521 evaluations of the exact pipeline.
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i0 := range g {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			var v vec
			v[0] = float64(i0) / float64(g-1)
			base := i0 * k.stride[0]
			var walk func(c, off int)
			walk = func(c, off int) {
				if c == in {
					r := p.apply(v)
					for o := range out {
						k.s[off+o] = float32(r[o])
					}
					return
				}
				for i := range g {
					v[c] = float64(i) / float64(g-1)
					walk(c+1, off+i*k.stride[c])
				}
			}
			walk(1, base)
		}()
	}
	wg.Wait()
	return k
}

func (k *clutKernel) run(out, in []float32, n int) {
	gm := float32(k.g - 1)
	var lin [4]float32
	for p := range n {
		i := in[p*k.in : p*k.in+k.in]
		o := out[p*k.out : p*k.out+k.out]
		if k.pre != nil {
			for c, t := range k.pre {
				lin[c] = lerp32(t, i[c])
			}
			i = lin[:k.in]
		}
		if k.in == 3 {
			k.tetrahedral(o, 0, i[0], i[1], i[2])
			k.shape(o)
			continue
		}
		x := unit(i[0]) * gm
		j := int(x)
		f := x - float32(j)
		j1 := j
		if j < k.g-1 {
			j1++
		}
		var lo, hi [4]float32
		k.tetrahedral(lo[:k.out], j*k.stride[0], i[1], i[2], i[3])
		k.tetrahedral(hi[:k.out], j1*k.stride[0], i[1], i[2], i[3])
		for c := range k.out {
			o[c] = lo[c] + f*(hi[c]-lo[c])
		}
		k.shape(o)
	}
}

func (k *clutKernel) shape(o []float32) {
	for c, t := range k.post {
		o[c] = shape(t, o[c])
	}
}

func (k *clutKernel) tetrahedral(o []float32, base int, x, y, z float32) {
	n := k.in
	sx, sy, sz := k.stride[n-3], k.stride[n-2], k.stride[n-1]
	gm := float32(k.g - 1)
	px := unit(x) * gm
	py := unit(y) * gm
	pz := unit(z) * gm
	x0, y0, z0 := int(px), int(py), int(pz)
	rx, ry, rz := px-float32(x0), py-float32(y0), pz-float32(z0)
	X0, Y0, Z0 := base+x0*sx, y0*sy, z0*sz
	X1, Y1, Z1 := X0, Y0, Z0
	if x0 < k.g-1 {
		X1 += sx
	}
	if y0 < k.g-1 {
		Y1 += sy
	}
	if z0 < k.g-1 {
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
	default:
		a1, b1 = X1+Y1+Z1, X0+Y1+Z1
		a2, b2 = X0+Y1+Z1, X0+Y0+Z1
		a3, b3 = X0+Y0+Z1, c0
	}
	s := k.s
	for c := range o {
		o[c] = s[c0+c] + (s[a1+c]-s[b1+c])*rx + (s[a2+c]-s[b2+c])*ry + (s[a3+c]-s[b3+c])*rz
	}
}
