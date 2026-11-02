package svg

import (
	"image"
	"image/draw"
	"math"

	"golang.org/x/image/vector"

	"github.com/worldiety/gift/internal/iconsvg"
)

// MaxExtent bounds the raster of a document, per axis, in pixels: a size
// from a layout that went wrong becomes a picture of 4096 pixels and not a
// request for gigabytes.
const MaxExtent = 4096

// Rasterize draws the document into a new picture of w by h pixels, the
// viewBox fitted as preserveAspectRatio says. The picture is premultiplied,
// as image.RGBA is, and transparent where nothing is drawn.
func (im *Image) Rasterize(w, h int) *image.RGBA {
	w, h = min(max(w, 1), MaxExtent), min(max(h, 1), MaxExtent)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	im.draw(dst, nil)
	return dst
}

// RasterizeTinted is [Image.Rasterize] with every paint replaced by tint,
// keeping its opacity: a monochrome icon in the colour of the text, the way
// SwiftUI's template rendering mode draws it. tint is not premultiplied.
func (im *Image) RasterizeTinted(w, h int, r, g, b, a float64) *image.RGBA {
	w, h = min(max(w, 1), MaxExtent), min(max(h, 1), MaxExtent)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	t := rgba{r, g, b, a}
	im.draw(dst, &t)
	return dst
}

// viewTransform maps the viewBox onto a picture of w by h pixels.
func (im *Image) viewTransform(w, h float64) matrix {
	vb := im.view
	if vb[2] <= 0 || vb[3] <= 0 {
		return identity
	}
	sx, sy := w/vb[2], h/vb[3]
	if !im.align.none {
		s := min(sx, sy)
		if im.align.slice {
			s = max(sx, sy)
		}
		sx, sy = s, s
	}
	tx := (w - vb[2]*sx) * im.align.x
	ty := (h - vb[3]*sy) * im.align.y
	if im.align.none {
		tx, ty = 0, 0
	}
	return matrix{sx, 0, 0, sy, tx - vb[0]*sx, ty - vb[1]*sy}
}

// canvas is one rasterisation in progress.
type canvas struct {
	dst   *image.RGBA
	z     *vector.Rasterizer
	mask  *image.Alpha
	clips map[int]*image.Alpha
	img   *Image
	view  matrix
	tint  *rgba
}

func (im *Image) draw(dst *image.RGBA, tint *rgba) {
	b := dst.Bounds()
	c := &canvas{dst: dst, z: vector.NewRasterizer(0, 0), mask: &image.Alpha{}, clips: map[int]*image.Alpha{},
		img: im, view: im.viewTransform(float64(b.Dx()), float64(b.Dy())), tint: tint}
	for i := range im.shapes {
		c.shape(&im.shapes[i])
	}
}

// shape fills and strokes s.
func (c *canvas) shape(s *shape) {
	m := c.view.mul(s.m)
	if s.fill.kind != paintNone {
		if r, ok := c.cover(func(z *vector.Rasterizer, ox, oy float64) { path(z, s.ops, m, ox, oy) }, deviceBounds(s.ops, m, 0)); ok {
			c.composite(r, s, &s.fill, s.fillOpacity*s.opacity, m)
		}
	}
	if s.stroke.kind != paintNone && s.width > 0 {
		hw := s.width * m.scale() / 2
		// A hairline still shows: a quarter of a pixel wide at the least,
		// fainter rather than thinner.
		alpha := 1.0
		if hw < 0.25 {
			alpha, hw = hw/0.25, 0.25
		}
		polys := flatten(s.ops, m)
		if len(s.dash) > 0 {
			polys = dash(polys, s.dash, s.dashOffset*m.scale(), m.scale())
		}
		reach := hw
		if s.join == "miter" || s.join == "miter-clip" || s.join == "arcs" {
			reach = hw * max(s.miter, 1)
		}
		bb := polyBounds(polys, reach+1)
		if r, ok := c.cover(func(z *vector.Rasterizer, ox, oy float64) {
			st := stamper{z: z, ox: ox, oy: oy}
			for _, p := range polys {
				st.stroke(p.pts, p.closed, hw, s.cap, s.join, s.miter)
			}
		}, bb); ok {
			c.composite(r, s, &s.stroke, s.strokeOpacity*s.opacity*alpha, m)
		}
	}
}

// cover rasterises the coverage of what add draws into c.mask, over the part
// of bb that is on the canvas, and returns that part.
func (c *canvas) cover(add func(z *vector.Rasterizer, ox, oy float64), bb [4]float64) (image.Rectangle, bool) {
	r := image.Rect(int(math.Floor(bb[0])), int(math.Floor(bb[1])), int(math.Ceil(bb[2])), int(math.Ceil(bb[3]))).
		Intersect(c.dst.Bounds())
	if r.Empty() {
		return r, false
	}
	w, h := r.Dx(), r.Dy()
	c.z.Reset(w, h)
	add(c.z, float64(-r.Min.X), float64(-r.Min.Y))
	if cap(c.mask.Pix) < w*h {
		c.mask.Pix = make([]byte, w*h)
	}
	c.mask.Pix = c.mask.Pix[:w*h]
	clear(c.mask.Pix)
	c.mask.Stride, c.mask.Rect = w, image.Rect(0, 0, w, h)
	c.z.DrawOp = draw.Src
	c.z.Draw(c.mask, c.mask.Rect, image.Opaque, image.Point{})
	return r, true
}

// path adds ops, transformed by m and shifted by ox, oy, to z.
func path(z *vector.Rasterizer, ops []iconsvg.Op, m matrix, ox, oy float64) {
	at := func(p [2]float64) (float32, float32) {
		x, y := m.apply(p[0], p[1])
		return float32(clampCoord(x + ox)), float32(clampCoord(y + oy))
	}
	open := false
	for _, op := range ops {
		switch op.Kind {
		case 'M':
			if open {
				z.ClosePath()
			}
			z.MoveTo(at(op.P[0]))
			open = true
		case 'L':
			if !open {
				continue
			}
			z.LineTo(at(op.P[0]))
		case 'C':
			if !open {
				continue
			}
			x1, y1 := at(op.P[0])
			x2, y2 := at(op.P[1])
			x, y := at(op.P[2])
			z.CubeTo(x1, y1, x2, y2, x, y)
		case 'Z':
			if open {
				z.ClosePath()
				open = false
			}
		}
	}
	if open {
		z.ClosePath()
	}
}

// clampCoord keeps a coordinate far off the canvas at a distance the
// rasteriser handles.
func clampCoord(f float64) float64 {
	if math.IsNaN(f) {
		return 0
	}
	return min(max(f, -1e6), 1e6)
}

// deviceBounds is the box of ops on the canvas, grown by pad.
func deviceBounds(ops []iconsvg.Op, m matrix, pad float64) [4]float64 {
	b := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, op := range ops {
		n := 0
		switch op.Kind {
		case 'M', 'L':
			n = 1
		case 'C':
			n = 3
		}
		for _, p := range op.P[:n] {
			x, y := m.apply(p[0], p[1])
			b[0], b[1], b[2], b[3] = min(b[0], x), min(b[1], y), max(b[2], x), max(b[3], y)
		}
	}
	return [4]float64{b[0] - pad, b[1] - pad, b[2] + pad, b[3] + pad}
}

// composite blends the coverage in c.mask over r of the canvas in the paint
// p at opacity, through the clips of s.
func (c *canvas) composite(r image.Rectangle, s *shape, p *paint, opacity float64, m matrix) {
	if opacity <= 0 {
		return
	}
	var clips []*image.Alpha
	for _, k := range s.clips {
		clips = append(clips, c.clipMask(k))
	}
	solid := p.kind == paintColor || c.tint != nil
	var col rgba
	if c.tint != nil {
		col = *c.tint
		if p.kind == paintColor {
			col[3] *= p.color[3]
		}
	} else if p.kind == paintColor {
		col = p.color
	}
	var g gradientSampler
	if !solid {
		g = newSampler(p.grad, m, s.bbox)
	}
	w := r.Dx()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		mrow := c.mask.Pix[(y-r.Min.Y)*w : (y-r.Min.Y+1)*w]
		drow := c.dst.Pix[c.dst.PixOffset(r.Min.X, y):]
		for i, cov := range mrow {
			if cov == 0 {
				continue
			}
			x := r.Min.X + i
			a := float64(cov) / 255 * opacity
			for _, cm := range clips {
				a *= float64(cm.Pix[cm.PixOffset(x, y)]) / 255
			}
			if a <= 0 {
				continue
			}
			pc := col
			if !solid {
				pc = g.at(float64(x)+0.5, float64(y)+0.5)
			}
			sa := pc[3] * a
			if sa <= 0 {
				continue
			}
			o := i * 4
			inv := 1 - sa
			for k := range 3 {
				v := pc[k]*sa*255 + float64(drow[o+k])*inv
				drow[o+k] = uint8(min(v+0.5, 255))
			}
			drow[o+3] = uint8(min(sa*255+float64(drow[o+3])*inv+0.5, 255))
		}
	}
}

// clipMask returns the coverage of clip k over the whole canvas.
func (c *canvas) clipMask(k int) *image.Alpha {
	if m, ok := c.clips[k]; ok {
		return m
	}
	b := c.dst.Bounds()
	out := image.NewAlpha(b)
	tmp := image.NewAlpha(b)
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	for _, s := range c.img.clips[k].shapes {
		m := c.view.mul(s.m)
		z.Reset(b.Dx(), b.Dy())
		path(z, s.ops, m, 0, 0)
		clear(tmp.Pix)
		z.DrawOp = draw.Src
		z.Draw(tmp, b, image.Opaque, image.Point{})
		// The shapes of a clip path are a union, each taken on its own so
		// that two of opposite winding do not cancel.
		for i, v := range tmp.Pix {
			out.Pix[i] = max(out.Pix[i], v)
		}
	}
	c.clips[k] = out
	return out
}

// gradientSampler returns the colour of a gradient at a point of the
// canvas.
type gradientSampler struct {
	g   *gradient
	inv matrix
}

// newSampler prepares g for a shape whose transform to the canvas is m and
// whose box in user units is bb.
func newSampler(g *gradient, m matrix, bb [4]float64) gradientSampler {
	t := m
	if !g.userSpace {
		t = t.mul(matrix{bb[2] - bb[0], 0, 0, bb[3] - bb[1], bb[0], bb[1]})
	}
	t = t.mul(g.m)
	inv, ok := t.invert()
	if !ok {
		inv = matrix{}
	}
	return gradientSampler{g: g, inv: inv}
}

func (s gradientSampler) at(x, y float64) rgba {
	g := s.g
	gx, gy := s.inv.apply(x, y)
	var t float64
	if !g.radial {
		dx, dy := g.x2-g.x1, g.y2-g.y1
		l := dx*dx + dy*dy
		if l > 0 {
			t = ((gx-g.x1)*dx + (gy-g.y1)*dy) / l
		}
	} else {
		t = radialT(g, gx, gy)
	}
	switch g.spread {
	case "reflect":
		t = math.Abs(t)
		f := math.Mod(t, 2)
		if f > 1 {
			f = 2 - f
		}
		t = f
	case "repeat":
		t -= math.Floor(t)
	default:
		t = min(max(t, 0), 1)
	}
	return stopAt(g.stops, t)
}

// radialT is where the point lies between the focus (0) and the circle (1)
// of a radial gradient.
func radialT(g *gradient, x, y float64) float64 {
	if g.r <= 0 {
		return 1
	}
	dx, dy := g.cx-g.fx, g.cy-g.fy
	px, py := x-g.fx, y-g.fy
	if dx*dx+dy*dy < 1e-12 {
		return math.Hypot(px, py) / g.r
	}
	// The circle of t has its centre at f + t·(c − f) and the radius t·r.
	a := dx*dx + dy*dy - g.r*g.r
	b := px*dx + py*dy
	cc := px*px + py*py
	if math.Abs(a) < 1e-12 {
		if b == 0 {
			return 0
		}
		return cc / (2 * b)
	}
	disc := b*b - a*cc
	if disc < 0 {
		return 1
	}
	return (b - math.Sqrt(disc)) / a
}

// stopAt interpolates the stops at t.
func stopAt(stops []stop, t float64) rgba {
	if t <= stops[0].off {
		return stops[0].c
	}
	for i := 1; i < len(stops); i++ {
		a, b := stops[i-1], stops[i]
		if t <= b.off {
			if b.off == a.off {
				return b.c
			}
			f := (t - a.off) / (b.off - a.off)
			return rgba{a.c[0] + (b.c[0]-a.c[0])*f, a.c[1] + (b.c[1]-a.c[1])*f, a.c[2] + (b.c[2]-a.c[2])*f, a.c[3] + (b.c[3]-a.c[3])*f}
		}
	}
	return stops[len(stops)-1].c
}
