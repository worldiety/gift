package ebiten

import (
	"image"
	"math"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// Static backdrops
//
// Live glass is expensive on a tile based GPU for a reason that has nothing
// to do with the blur: to read what lies behind a pane, everything drawn
// before it has to reach memory first, and on the VideoCore of a Raspberry Pi
// that barrier measured two and a half milliseconds per pane before a single
// blurred pixel.
//
// Most glass in an application does not need it. A panel over a wallpaper
// shows the wallpaper, and the wallpaper does not change from frame to frame.
// So before a frame is drawn, the renderer follows the list in drawing order
// and asks of every pane: is what lies behind it one opaque picture, with
// nothing drawn over that picture inside the pane since? If so, the backdrop
// is that picture, blurred once and kept, and the pane is a single draw that
// samples it – no copy, no chain, no barrier, and no screen sized scene
// target for the frame either, as long as no pane needs a live backdrop.
//
// The application does nothing for it. It is a property of the picture
// behind the pane and is decided anew every frame.
//
// # What it overlooks
//
// Shadows. A shadow under a pane is drawn before the pane and lies behind it,
// and a live pane shows it darkened through the glass; a static one does not,
// because following soft shadows would rule out almost every panel that has
// one. The difference is the few percent of darkening a shadow leaves under
// its own caster.
//
// The blur differs at the rim: a live pane blurs only its own region, a
// static one the whole picture, so near the edge it takes in what lies just
// outside. That is the better picture and it is a different one.

// maxCovers is the number of pictures followed at a time, maxDirty the number
// of rectangles drawn over one before they are merged into their bounds.
const (
	maxCovers = 4
	maxDirty  = 32
)

// cover is an opaque picture a pane may use as its backdrop.
type cover struct {
	id    render.ImageID
	stamp uint64
	tex   *eb.Image
	// src is the part of the texture shown, dev where it is shown, rounded to
	// whole pixels, and vis the part of dev that is visible.
	src      geom.Rect
	dev, vis geom.Rect
	// dirty is what has been drawn over the picture since.
	dirty []geom.Rect
}

// backdropTracker follows the pictures a pane may use, in drawing order.
type backdropTracker struct {
	covers [maxCovers]cover
	n      int
}

func (t *backdropTracker) reset() { t.n = 0 }

// drew records that r, in the space of the frame's target, was drawn over.
func (t *backdropTracker) drew(r geom.Rect) {
	if r.IsEmpty() {
		return
	}
	for i := range t.n {
		c := &t.covers[i]
		if !c.vis.Overlaps(r) {
			continue
		}
		if len(c.dirty) == maxDirty {
			u := c.dirty[0]
			for _, d := range c.dirty[1:] {
				u = u.Union(d)
			}
			c.dirty = append(c.dirty[:0], u)
		}
		c.dirty = append(c.dirty, r)
	}
}

// picture records an opaque picture drawn at c.vis. It covers what the other
// pictures showed there.
func (t *backdropTracker) picture(c cover) {
	t.drew(c.vis)
	if t.n == maxCovers {
		// The oldest is the one most likely to be buried.
		keep := t.covers[0].dirty[:0]
		copy(t.covers[:], t.covers[1:])
		t.n--
		t.covers[t.n].dirty = keep
	}
	dirty := t.covers[t.n].dirty[:0]
	t.covers[t.n] = c
	t.covers[t.n].dirty = dirty
	t.n++
}

// behind returns the picture that is the whole backdrop of region, if one is.
func (t *backdropTracker) behind(region geom.Rect) (*cover, bool) {
	for i := t.n - 1; i >= 0; i-- {
		c := &t.covers[i]
		if c.vis.Intersect(region) != region {
			continue
		}
		for _, d := range c.dirty {
			if d.Overlaps(region) {
				return nil, false
			}
		}
		return c, true
	}
	return nil, false
}

// blurKey identifies a blurred picture: the texture, the part and size it is
// shown at, and the strength of the blur.
type blurKey struct {
	id     render.ImageID
	stamp  uint64
	src    geom.Rect
	w, h   int
	levels int
}

type blurEntry struct {
	img  *eb.Image
	used uint64
}

// staticPane is the decision for one pane: the blurred picture it samples and
// where that picture lies in the target.
type staticPane struct {
	op     int
	img    *eb.Image
	origin geom.Point
}

// SetStaticBackdrops turns static backdrops on, which is the default, or off.
// Off, every pane reads a live backdrop: the switch that lets a measurement
// say what they save.
func (r *Renderer) SetStaticBackdrops(on bool) { r.noStatic = !on }

// coverOf reports whether the picture op draws is opaque and axis aligned,
// which is what makes it usable as a backdrop, and where it lands.
func (r *Renderer) coverOf(l *render.List, op render.Op) (cover, bool) {
	if r.noStatic || r.textures == nil || op.CornerRadius > 0 || op.Bounds.IsEmpty() {
		return cover{}, false
	}
	if c := op.Color; c.R != 1 || c.G != 1 || c.B != 1 || c.A != 1 {
		return cover{}, false
	}
	xf := r.opXform(l, op.Xform)
	if xf.B != 0 || xf.C != 0 || !(xf.A > 0) || !(xf.D > 0) {
		return cover{}, false
	}
	tex := r.textures.image(op.Image)
	w, h, ok := r.textures.size(op.Image)
	if !ok || tex == nil || !r.textures.opaque(op.Image) {
		return cover{}, false
	}
	dev := xf.TransformRect(op.Bounds)
	// Whole pixels, so that the blurred picture maps one texel onto one
	// pixel and a nearest sample is exact.
	x0, y0 := float32(math.Round(float64(dev.Min.X))), float32(math.Round(float64(dev.Min.Y)))
	x1, y1 := float32(math.Round(float64(dev.Max.X))), float32(math.Round(float64(dev.Max.Y)))
	if x1-x0 < 1 || y1-y0 < 1 || x1-x0 > render.MaxLayerSide || y1-y0 > render.MaxLayerSide {
		return cover{}, false
	}
	c := cover{
		id: op.Image, stamp: r.imageStamp(op.Image), tex: tex,
		src: op.Fit.Source(op.Bounds, w, h),
		dev: geom.Rc(x0, y0, x1, y1),
	}
	c.vis = c.dev.Intersect(r.opClip(l, op.Clip))
	if r.frameSize.W > 0 && r.frameSize.H > 0 {
		c.vis = c.vis.Intersect(geom.Rc(0, 0, r.frameSize.W, r.frameSize.H))
	}
	return c, !c.vis.IsEmpty()
}

// planBackdrops follows ops in drawing order, decides for every visible glass
// pane whether it has a static backdrop, blurs the pictures that need it and
// reports whether any pane needs a live one. It runs before the frame draws
// anything, like prepareLayers and for the same reason.
//
// Its answer is what decides whether the frame pays for a scene target: the
// screen sized offscreen, its clear and its blit are owed only while a pane
// that needs a live backdrop is *visible* – not merely in the list, and not
// when every visible pane has a static one. A pane scrolled out of its clip
// and a pane over the wallpaper cost the frame nothing extra.
func (r *Renderer) planBackdrops(l *render.List, ops []render.Op, off int) (live bool) {
	for i := 0; i < len(ops); i++ {
		op := ops[i]
		switch op.Kind {
		case render.OpLayer:
			sub := layerRange(ops, i)
			c := r.composite(l, op, r.frameSize)
			if !c.vis.IsEmpty() {
				if r.noLayerCache || needsBackdrop(l, sub) {
					prev, was := r.enterThrough(l, op)
					if r.planBackdrops(l, sub, off+i+1) {
						live = true
					}
					r.leaveThrough(prev, was)
				} else {
					r.backdrops.drew(c.vis)
				}
			}
			i += len(sub)
		case render.OpImage:
			if c, ok := r.coverOf(l, op); ok {
				r.backdrops.picture(c)
				continue
			}
			r.backdrops.drew(r.opDevice(l, op))
		case render.OpShadow:
			// See "What it overlooks".
		case render.OpMaterial:
			_, vis, ok := r.materialRegion(l, op)
			if !ok {
				continue
			}
			if c, ok := r.backdrops.behind(vis); ok {
				if img := r.blurred(c, l.Material(op.Material).Glass.Blur*r.materialScale(l, op)); img != nil {
					r.statics = append(r.statics, staticPane{op: off + i, img: img, origin: c.dev.Min})
					r.backdrops.drew(vis)
					continue
				}
			}
			live = true
			r.backdrops.drew(vis)
		default:
			r.backdrops.drew(r.opDevice(l, op))
		}
	}
	return live
}

// opDevice is where op may draw in the frame's target, conservatively.
func (r *Renderer) opDevice(l *render.List, op render.Op) geom.Rect {
	b := op.PaintBounds()
	if b.IsEmpty() {
		return geom.Rect{}
	}
	return r.opXform(l, op.Xform).TransformRect(b).Canon().Intersect(r.opClip(l, op.Clip))
}

// materialScale is the scale a material's lengths are multiplied by; see
// appendMaterial.
func (r *Renderer) materialScale(l *render.List, op render.Op) float32 {
	sx, sy := deviceScale(r.opXform(l, op.Xform))
	return min(sx, sy)
}

// materialRegion computes the device region of a glass op and its visible
// part exactly as appendMaterial does, and reports whether it draws.
func (r *Renderer) materialRegion(l *render.List, op render.Op) (region, vis geom.Rect, ok bool) {
	if l.Material(op.Material).Kind != render.MaterialGlass || op.Bounds.IsEmpty() {
		return geom.Rect{}, geom.Rect{}, false
	}
	clip := r.opClip(l, op.Clip)
	if clip.IsEmpty() {
		return geom.Rect{}, geom.Rect{}, false
	}
	region = r.opXform(l, op.Xform).TransformRect(op.Bounds).Canon()
	vis = region.Intersect(clip)
	if r.frameSize.W > 0 && r.frameSize.H > 0 {
		vis = vis.Intersect(geom.Rc(0, 0, r.frameSize.W, r.frameSize.H))
	}
	return region, vis, !vis.IsEmpty()
}

// blurred returns c blurred by radiusDev device pixels, drawing it the first
// time it is asked for.
func (r *Renderer) blurred(c *cover, radiusDev float32) *eb.Image {
	w, h := int(c.dev.Width()), int(c.dev.Height())
	k := blurKey{id: c.id, stamp: c.stamp, src: c.src, w: w, h: h, levels: blurLevels(radiusDev)}
	if e := r.blurs[k]; e != nil {
		e.used = r.drawn
		return e.img
	}
	if r.targets == nil {
		return nil
	}
	img := eb.NewImageWithOptions(image.Rect(0, 0, w, h), &eb.NewImageOptions{Unmanaged: true})
	// The picture at the size it is shown at, with the linear filter, so
	// that the blurred copy maps one texel onto one pixel.
	r.passVerts = r.passVerts[:0]
	r.passIdx = r.passIdx[:0]
	for _, v := range [4][4]float32{
		{0, 0, c.src.Min.X, c.src.Min.Y},
		{float32(w), 0, c.src.Max.X, c.src.Min.Y},
		{float32(w), float32(h), c.src.Max.X, c.src.Max.Y},
		{0, float32(h), c.src.Min.X, c.src.Max.Y},
	} {
		r.passVerts = append(r.passVerts, eb.Vertex{
			DstX: v[0], DstY: v[1], SrcX: v[2], SrcY: v[3],
			ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1,
		})
	}
	r.passIdx = append(r.passIdx, 0, 1, 2, 0, 2, 3)
	if r.drawFn != nil {
		r.drawFn(MaterialGlass, r.passVerts, r.passIdx)
	} else {
		img.DrawTriangles32(r.passVerts, r.passIdx, c.tex, &r.scaleOpts)
	}
	r.blurRegion(img, w, h, radiusDev)

	if r.blurs == nil {
		r.blurs = make(map[blurKey]*blurEntry)
	}
	r.blurs[k] = &blurEntry{img: img, used: r.drawn}
	r.staticBlurs++
	return img
}

// staticFor returns the decision planBackdrops made for the op at index i.
func (r *Renderer) staticFor(i int) (staticPane, bool) {
	for r.staticNext < len(r.statics) && r.statics[r.staticNext].op < i {
		r.staticNext++
	}
	if r.staticNext < len(r.statics) && r.statics[r.staticNext].op == i {
		return r.statics[r.staticNext], true
	}
	return staticPane{}, false
}

// compositeStatic draws a pane over its static backdrop.
func (r *Renderer) compositeStatic(s staticPane, vis, region geom.Rect,
	halfW, halfH, radius, scale float32, g render.GlassParams) {
	// The whole chain ran once, so the pane is always drawn at the Full
	// level's look, grain included.
	packed := packGlassStyle(g.Refraction*scale, g.Highlight, g.Grain)
	r.passVerts = r.passVerts[:0]
	r.passIdx = r.passIdx[:0]
	for _, c := range [4][2]float32{
		{vis.Min.X, vis.Min.Y}, {vis.Max.X, vis.Min.Y},
		{vis.Max.X, vis.Max.Y}, {vis.Min.X, vis.Max.Y},
	} {
		r.passVerts = append(r.passVerts, eb.Vertex{
			DstX: c[0], DstY: c[1],
			SrcX: c[0] - region.Min.X, SrcY: c[1] - region.Min.Y,
			ColorR: g.Tint.R, ColorG: g.Tint.G, ColorB: g.Tint.B, ColorA: g.Tint.A,
			Custom0: halfW, Custom1: halfH, Custom2: radius, Custom3: packed,
		})
	}
	r.passIdx = append(r.passIdx, 0, 1, 2, 0, 2, 3)
	if r.drawFn != nil {
		r.drawFn(MaterialGlass, r.passVerts, r.passIdx)
		r.countGlassBatch()
		return
	}
	if r.dst == nil {
		return
	}
	r.staticOrigin[0], r.staticOrigin[1] = s.origin.X, s.origin.Y
	r.staticOpts.Images[0] = s.img
	r.dst.DrawTrianglesShader32(r.passVerts, r.passIdx, r.staticShader, &r.staticOpts)
	r.staticOpts.Images[0] = nil
	r.countGlassBatch()
}

// evictBlurs releases blurred pictures no pane sampled for layerMaxIdle
// frames.
func (r *Renderer) evictBlurs() {
	for k, e := range r.blurs {
		if r.drawn-e.used <= layerMaxIdle {
			continue
		}
		e.img.Deallocate()
		delete(r.blurs, k)
	}
}
