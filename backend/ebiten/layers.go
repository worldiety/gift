package ebiten

import (
	"image"
	"math"
	"slices"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// Cached layers
//
// A [render.OpLayer] announces a subtree whose picture the renderer may keep.
// The producer emits the subtree every frame in the layer's own space; the
// renderer compares it with what it drew into the layer's texture last time
// and, when nothing differs, draws one textured quad instead of the subtree.
//
// # Two passes, and why
//
// Every layer that has to be drawn again is drawn *before* anything reaches
// the frame's target: [Renderer.prepareLayers] runs over the whole list
// first, and only then does the frame proper begin, in which every layer is a
// composite. The order is for the tile based GPUs gift runs on. Switching
// the render target in the middle of a frame makes such a GPU write its tiles
// out and read them back in when it returns, and on the VideoCore of a
// Raspberry Pi that round trip measured two and a half milliseconds – the
// very cost that makes a live glass panel expensive there. Drawing the layers
// first keeps the frame one uninterrupted pass over the screen.
//
// # A layer with glass in it is not cached
//
// A glass material shows what lies behind it, and in a texture of its own
// nothing does. Rather than draw the fallback, which would make a panel
// change its look for exactly as long as a page slides, such a layer is
// drawn through: its operations go straight into the frame, mapped from
// layer space into the frame by [throughSpace], and nothing is kept. It costs
// what the subtree cost without a layer. Layers inside it are still cached.
//
// # What counts as equal
//
// The operations with their side table entries resolved: the clip rectangle
// and the transform rather than their indices, the glyphs rather than their
// offset, the material rather than its slot. An index moves when something
// earlier in the list changes; the value it points at does not. A picture is
// compared by its texture's identity, so that a thumbnail replaced under the
// same slot is noticed although its operation is not.

// layerMaxIdle is the number of drawn frames a layer may go without being
// composited before its texture is released.
const layerMaxIdle = 60

// layerEntry is what the renderer keeps of one layer: its texture and the
// operations the texture shows.
type layerEntry struct {
	tex    *eb.Image
	tw, th int
	// w and h are the size of the picture in tex; the texture may be larger.
	w, h int

	// valid is false until a picture has been drawn, and again whenever
	// drawing it was incomplete.
	valid bool
	// ready is the frame the entry was last made current in, by a match or a
	// draw. A composite of an entry that is not ready this frame is skipped.
	ready uint64
	// used is the last frame the layer was composited in; see layerMaxIdle.
	used uint64
	// drawn is true when ready was reached by drawing rather than by a
	// match, in which case the operations were already accounted for.
	drawn bool
	// through is true for a layer with glass in it, which has no texture;
	// see the section on it above.
	through bool

	ops    []render.Op
	clips  []geom.Rect
	xforms []geom.Affine2D
	glyphs []render.Glyph
	mats   []render.Material
	images []uint64
}

// throughSpace maps the operations of a layer that is drawn through into the
// space the layer itself is drawn in: m takes layer pixels there, and clip is
// the layer's own clip there.
type throughSpace struct {
	m    geom.Affine2D
	clip geom.Rect
}

// opXform is the transform with index i, mapped out of a layer that is drawn
// through.
func (r *Renderer) opXform(l *render.List, i uint32) geom.Affine2D {
	xf := l.Xform(i)
	if r.inThrough {
		xf = xf.Mul(r.through.m)
	}
	return xf
}

// opClip is the clip with index i, mapped out of a layer that is drawn
// through. The clips of a layer are axis aligned in layer pixels, and the
// transforms gift produces keep them so.
func (r *Renderer) opClip(l *render.List, i uint32) geom.Rect {
	c := l.Clip(i)
	if r.inThrough {
		c = r.through.m.TransformRect(c).Canon().Intersect(r.through.clip)
	}
	return c
}

// throughOf is the space of the layer opened by op, in the current space.
func (r *Renderer) throughOf(l *render.List, op render.Op) throughSpace {
	s := op.LayerScale()
	back := geom.Scale(1/s, 1/s).Mul(geom.Translate(op.Bounds.Min))
	return throughSpace{m: back.Mul(r.opXform(l, op.Xform)), clip: r.opClip(l, op.Clip)}
}

// enterThrough makes the layer opened by op the space of the following
// operations and returns what to pass to leaveThrough.
func (r *Renderer) enterThrough(l *render.List, op render.Op) (throughSpace, bool) {
	prev, was := r.through, r.inThrough
	r.through, r.inThrough = r.throughOf(l, op), true
	return prev, was
}

func (r *Renderer) leaveThrough(prev throughSpace, was bool) {
	r.through, r.inThrough = prev, was
}

// needsBackdrop reports whether ops contain a material that shows what lies
// behind it.
func needsBackdrop(l *render.List, ops []render.Op) bool {
	if l.MaterialsLen() <= 1 {
		return false
	}
	for i := range ops {
		if ops[i].Kind == render.OpMaterial && l.Material(ops[i].Material).Kind == render.MaterialGlass {
			return true
		}
	}
	return false
}

// layerOp is op without its side table indices, which is the part of it that
// is compared as it is.
func layerOp(op render.Op) render.Op {
	op.Clip, op.Xform, op.Glyphs, op.Material = 0, 0, 0, 0
	return op
}

// matches reports whether ops are the operations e was drawn from.
func (e *layerEntry) matches(r *Renderer, l *render.List, ops []render.Op) bool {
	if !e.valid || len(ops) != len(e.ops) {
		return false
	}
	gi, mi, ii := 0, 0, 0
	for k := range ops {
		op := &ops[k]
		if layerOp(*op) != e.ops[k] || l.Clip(op.Clip) != e.clips[k] || l.Xform(op.Xform) != e.xforms[k] {
			return false
		}
		switch op.Kind {
		case render.OpGlyphs:
			gs := l.Glyphs(op.Glyphs, op.GlyphCount)
			if gi+len(gs) > len(e.glyphs) || !slices.Equal(gs, e.glyphs[gi:gi+len(gs)]) {
				return false
			}
			gi += len(gs)
		case render.OpMaterial:
			if mi >= len(e.mats) || l.Material(op.Material) != e.mats[mi] {
				return false
			}
			mi++
		case render.OpImage:
			if ii >= len(e.images) || r.imageStamp(op.Image) != e.images[ii] {
				return false
			}
			ii++
		}
	}
	return true
}

// record makes ops the operations e shows.
func (e *layerEntry) record(r *Renderer, l *render.List, ops []render.Op) {
	e.ops, e.clips, e.xforms = e.ops[:0], e.clips[:0], e.xforms[:0]
	e.glyphs, e.mats, e.images = e.glyphs[:0], e.mats[:0], e.images[:0]
	for k := range ops {
		op := &ops[k]
		e.ops = append(e.ops, layerOp(*op))
		e.clips = append(e.clips, l.Clip(op.Clip))
		e.xforms = append(e.xforms, l.Xform(op.Xform))
		switch op.Kind {
		case render.OpGlyphs:
			e.glyphs = append(e.glyphs, l.Glyphs(op.Glyphs, op.GlyphCount)...)
		case render.OpMaterial:
			e.mats = append(e.mats, l.Material(op.Material))
		case render.OpImage:
			e.images = append(e.images, r.imageStamp(op.Image))
		}
	}
}

// imageStamp identifies the texture behind id: its generation while it is
// resident, zero while it is not. A texture never changes its pixels while it
// is resident, so an equal stamp is an equal picture.
func (r *Renderer) imageStamp(id render.ImageID) uint64 {
	if r.textures == nil {
		return 0
	}
	i := uint32(id)
	t := r.textures
	if i == 0 || int(i) >= len(t.recs) || !t.recs[i].live {
		return 0
	}
	return uint64(t.recs[i].gen)<<32 | uint64(i) | 1<<63
}

// layerRange returns the operations of the layer opened at ops[i], clamped to
// the list.
func layerRange(ops []render.Op, i int) []render.Op {
	end := min(i+1+ops[i].LayerOps(), len(ops))
	return ops[i+1 : end]
}

// layerComposite is where and how the texture of a layer is drawn.
type layerComposite struct {
	w, h int
	clip geom.Rect
	xf   geom.Affine2D
	// dev is the destination in device pixels when the composite maps one
	// texel onto one pixel, and empty otherwise.
	dev geom.Rect
	// vis is the part of the destination that is visible.
	vis geom.Rect
}

// composite computes where the layer opened by op lands in a target of the
// given size.
func (r *Renderer) composite(l *render.List, op render.Op, size geom.Size) layerComposite {
	s := op.LayerScale()
	w, h := render.LayerSize(op.Bounds, s)
	c := layerComposite{w: w, h: h, clip: r.opClip(l, op.Clip), xf: r.opXform(l, op.Xform)}
	xf := c.xf
	if xf.B == 0 && xf.C == 0 && nearly(xf.A, s) && nearly(xf.D, s) {
		// One texel per pixel. The origin is rounded to a whole pixel, so
		// that the linear filter samples texel centres and the picture is
		// exactly the one that was drawn; a layer at rest is sharp, and one
		// in motion moves in whole pixels, which at sixty frames a second
		// nobody can tell from anything finer.
		x := float32(math.Round(float64(xf.A*op.Bounds.Min.X + xf.TX)))
		y := float32(math.Round(float64(xf.D*op.Bounds.Min.Y + xf.TY)))
		c.dev = geom.Rc(x, y, x+float32(w), y+float32(h))
		c.vis = c.dev
	} else {
		c.vis = xf.TransformRect(op.Bounds).Canon()
	}
	c.vis = c.vis.Intersect(c.clip)
	if size.W > 0 && size.H > 0 {
		c.vis = c.vis.Intersect(geom.Rc(0, 0, size.W, size.H))
	}
	return c
}

func nearly(a, b float32) bool {
	d := a - b
	return d < 1e-4 && d > -1e-4
}

// prepareLayers draws again every layer in ops whose operations changed since
// it was last drawn, and marks every other one as current. Layers that are not
// visible in a target of the given size are left alone. It runs before the
// frame draws anything; see the section on two passes above.
func (r *Renderer) prepareLayers(l *render.List, ops []render.Op, off int, size geom.Size) {
	for i := 0; i < len(ops); i++ {
		op := ops[i]
		if op.Kind != render.OpLayer {
			continue
		}
		sub := layerRange(ops, i)
		i += len(sub)

		c := r.composite(l, op, size)
		if c.vis.IsEmpty() || c.w <= 0 || c.h <= 0 {
			continue
		}
		e := r.layers[op.Image]
		if e == nil {
			if r.layers == nil {
				r.layers = make(map[render.ImageID]*layerEntry)
			}
			e = &layerEntry{}
			r.layers[op.Image] = e
		}
		if r.noLayerCache || needsBackdrop(l, sub) {
			e.through, e.ready = true, r.drawn
			if e.tex != nil {
				e.tex.Deallocate()
				e.tex, e.valid = nil, false
			}
			prev, was := r.enterThrough(l, op)
			r.prepareLayers(l, sub, off+i-len(sub)+1, size)
			r.leaveThrough(prev, was)
			continue
		}
		e.through = false
		if e.w == c.w && e.h == c.h && e.matches(r, l, sub) {
			e.ready, e.drawn = r.drawn, false
			r.layerHits++
			continue
		}
		// The layers inside first, so that this one composites them.
		r.prepareLayers(l, sub, off+i-len(sub)+1, geom.Size{W: float32(c.w), H: float32(c.h)})
		r.drawLayer(e, l, sub, off+i-len(sub)+1, c.w, c.h)
	}
}

// drawLayer draws ops into the texture of e, which is w by h pixels.
func (r *Renderer) drawLayer(e *layerEntry, l *render.List, ops []render.Op, off, w, h int) {
	if e.tex == nil || w > e.tw || h > e.th || 2*w*h < e.tw*e.th {
		if e.tex != nil {
			e.tex.Deallocate()
		}
		e.tex = eb.NewImageWithOptions(image.Rect(0, 0, w, h), &eb.NewImageOptions{Unmanaged: true})
		e.tw, e.th = w, h
		r.layerAllocs++
	} else if r.drawFn == nil {
		e.tex.Clear()
	}
	e.w, e.h = w, h

	r.flush()
	dst, size := r.dst, r.frameSize
	r.dst, r.frameSize = e.tex, geom.Size{W: float32(w), H: float32(h)}
	r.inLayer++
	r.drawOps(l, ops, off)
	r.flush()
	r.inLayer--
	r.dst, r.frameSize = dst, size

	// A picture that was not resident is recorded with its stamp of zero, so
	// its arrival is a difference by itself, and glyphs are rasterised
	// synchronously: nothing in a layer draws incompletely.
	e.record(r, l, ops)
	e.valid = true
	e.ready, e.drawn = r.drawn, true
	r.layerDraws++
}

// drawOps translates ops, compositing the layers among them. off is the
// index of ops[0] in the list.
func (r *Renderer) drawOps(l *render.List, ops []render.Op, off int) {
	for i := 0; i < len(ops); i++ {
		op := ops[i]
		if op.Kind == render.OpLayer {
			sub := layerRange(ops, i)
			r.appendLayer(l, op, sub, off+i+1)
			i += len(sub)
		} else {
			r.curOp = off + i
			r.appendOp(l, op)
		}
		if len(r.verts) >= maxBatchVertices {
			r.flush()
		}
	}
}

// appendLayer composites the texture of the layer opened by op, or draws a
// layer with glass in it through. Its n operations were accounted for when they were drawn into the texture, or are
// accounted for here as reused from it.
func (r *Renderer) appendLayer(l *render.List, op render.Op, sub []render.Op, off int) {
	n := len(sub)
	e := r.layers[op.Image]
	c := r.composite(l, op, r.frameSize)
	if e == nil || e.ready != r.drawn || c.vis.IsEmpty() {
		// Not visible, which is also why prepareLayers left it alone.
		r.skipOutsideClip += uint64(n) + 1
		return
	}
	e.used = r.drawn
	if e.through {
		prev, was := r.enterThrough(l, op)
		r.drawOps(l, sub, off)
		r.leaveThrough(prev, was)
		r.layerThrough++
		return
	}
	if !e.drawn {
		r.layerReused += uint64(n)
	}

	var wrote bool
	switch {
	case !c.dev.IsEmpty():
		r.material(MaterialImage, e.tex)
		wrote = r.appendDeviceQuad(c.dev, geom.Rc(0, 0, float32(c.w), float32(c.h)), c.clip, op.Color)
	default:
		s := op.LayerScale()
		src := geom.Rc(0, 0, op.Bounds.Width()*s, op.Bounds.Height()*s)
		r.material(MaterialImage, e.tex)
		if c.xf.B == 0 && c.xf.C == 0 && c.xf.A > 0 && c.xf.D > 0 {
			wrote = r.appendTexturedQuad(op.Bounds, src, c.clip, c.xf, op.Color)
		} else {
			wrote = r.appendTexturedQuadTransformed(op.Bounds, src, c.clip, c.xf, op.Color)
		}
	}
	if !wrote {
		r.skipOutsideClip++
		return
	}
	r.layerOps++
	r.emitted++
}

// evictLayers releases the textures of layers that were not composited for
// layerMaxIdle frames.
func (r *Renderer) evictLayers() {
	for k, e := range r.layers {
		if r.drawn-e.used <= layerMaxIdle {
			continue
		}
		if e.tex != nil {
			e.tex.Deallocate()
		}
		delete(r.layers, k)
		r.layerEvictions++
	}
}

// SetLayerCache turns the layer cache on, which is the default, or off. Off,
// every layer is drawn through as if it had glass in it, which costs exactly
// what the subtree cost before layers existed: the switch that lets a
// measurement say what the cache saves.
func (r *Renderer) SetLayerCache(on bool) { r.noLayerCache = !on }

// LayerStats are the counters of the layer cache.
type LayerStats struct {
	// Composites is the number of layers drawn as one textured quad.
	Composites uint64
	// Hits is the number of layers whose operations were unchanged, and
	// Draws the number drawn into their texture again. On a slide between
	// two pages Draws stays at the two of the first frame and Hits grows.
	Hits, Draws uint64
	// Reused is the number of operations that a hit did not translate.
	Reused uint64
	// Through is the number of layers drawn through because of the glass in
	// them; see the section on it above.
	Through uint64
	// Allocations and Evictions count the textures.
	Allocations, Evictions uint64
	// Resident is the number of layers holding a texture now.
	Resident int
}

// LayerStats returns the counters of the layer cache.
func (r *Renderer) LayerStats() LayerStats {
	return LayerStats{
		Composites:  r.layerOps,
		Hits:        r.layerHits,
		Draws:       r.layerDraws,
		Reused:      r.layerReused,
		Through:     r.layerThrough,
		Allocations: r.layerAllocs,
		Evictions:   r.layerEvictions,
		Resident:    len(r.layers),
	}
}
