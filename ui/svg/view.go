package svg

import (
	"image"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

var (
	viewType   = gift.RegisterType("svg.View")
	canvasType = gift.RegisterType("svg.canvas")
)

// ImageView shows an SVG document. It is created by [View].
type ImageView struct {
	im         *Image
	w, h       float32
	tint       ui.Color
	hasTint    bool
	label, key string
	flex       float32
}

// View returns a view of im, the counterpart of SwiftUI's Image of a vector
// asset: as large as the document asks for and no larger than there is
// room, the document fitted into it as its preserveAspectRatio says. It is
// drawn at the resolution of the screen, sharp at any size; a large one is
// rasterised on a goroutine of its own, and until it is, the view shows the
// picture it had, scaled.
//
//	tiger, err := svg.ParseFile("tiger.svg")
//	svg.View(tiger).Frame(300, 300)
func View(im *Image) ImageView { return ImageView{im: im} }

// Frame sets the size; a zero width or height is the one the document's
// aspect gives it.
func (v ImageView) Frame(w, h float32) ImageView { v.w, v.h = max(w, 0), max(h, 0); return v }

// Tint draws the document in one colour, keeping its opacity, the way
// SwiftUI's template rendering mode draws an icon: in the colour of the text
// around it, say.
func (v ImageView) Tint(c ui.Color) ImageView { v.tint, v.hasTint = c, true; return v }

// Label sets the accessible name.
func (v ImageView) Label(s string) ImageView { v.label = s; return v }

// Key sets the reconciliation key.
func (v ImageView) Key(s string) ImageView { v.key = s; return v }

// Flex sets the flex factor in a stack; a flexible view takes the room it
// is given and fits the document into it.
func (v ImageView) Flex(f float32) ImageView { v.flex = f; return v }

// ViewType implements gift.View.
func (ImageView) ViewType() gift.TypeID { return viewType }

// Build implements gift.View.
func (v ImageView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Flex: v.flex, Layouter: passLayout{},
		Children: []gift.View{gift.Component("svg.view", v.body)}}
}

type passLayout struct{}

func (passLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	s := ctx.Measure(0, c)
	ctx.Place(0, geom.Point{})
	return s
}

// raster is a rasterisation: its key and, once done, its pixels.
type raster struct {
	key rasterKey
	pix *image.RGBA
}

type rasterKey struct {
	im   *Image
	w, h int
	tint render.Color
	has  bool
}

// state is what a view keeps across builds: the raster it shows, the one on
// its way, and the texture of the one it shows.
type state struct {
	tok        gift.Token
	have       raster
	want       rasterKey
	busy       bool
	tex        render.ImageHandle
	texKey     rasterKey
	invalidate func()
}

func (v ImageView) body(ctx *gift.Context) gift.View {
	mem := ctx.State("state", (*state)(nil))
	st := mem.Get()
	if st == nil {
		st = &state{tok: ctx.Request()}
		mem.Set(st)
	}
	n := &canvasNode{v: v, st: st}
	if v.hasTint {
		n.tint = ui.ResolveColor(v.tint)
	}
	return canvasView{n: n}
}

type canvasView struct{ n *canvasNode }

func (canvasView) ViewType() gift.TypeID { return canvasType }

func (c canvasView) Build(*gift.BuildContext) gift.Element {
	label := c.n.v.label
	if label == "" {
		label = "Image"
	}
	return gift.Element{Key: "canvas", Label: label, Layouter: c.n, Painter: c.n}
}

type canvasNode struct {
	v    ImageView
	st   *state
	tint render.Color
}

// syncPixels is the most pixels a view rasterises on the spot rather than
// on a goroutine: an icon, which would flash if it came a frame later.
const syncPixels = 160 * 160

var rasterKeys atomic.Uint64

func (n *canvasNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	v := n.v
	if v.im == nil {
		return c.Constrain(geom.Size{})
	}
	iw, ih := v.im.Size()
	w, h := v.w, v.h
	switch {
	case w > 0 && h == 0 && iw > 0:
		h = w * float32(ih/iw)
	case h > 0 && w == 0 && ih > 0:
		w = h * float32(iw/ih)
	case w == 0 && h == 0:
		w, h = float32(iw), float32(ih)
		if v.flex > 0 && !math.IsInf(float64(c.Max.W), 1) && !math.IsInf(float64(c.Max.H), 1) {
			w, h = c.Max.W, c.Max.H
		}
	}
	// No larger than there is room, keeping the aspect of the document.
	if w > c.Max.W || h > c.Max.H {
		f := min(c.Max.W/max(w, 1e-6), c.Max.H/max(h, 1e-6))
		w, h = w*f, h*f
	}
	size := c.Constrain(geom.Sz(w, h))
	d := ctx.Density()
	key := rasterKey{im: v.im, w: int(math.Round(float64(size.W * d))), h: int(math.Round(float64(size.H * d))),
		tint: n.tint, has: v.hasTint}
	key.w, key.h = min(max(key.w, 1), MaxExtent), min(max(key.h, 1), MaxExtent)
	st := n.st
	st.invalidate = ctx.Invalidator()
	if st.have.key != key && (!st.busy || st.want != key) && size.W > 0 && size.H > 0 {
		if pix, ok := v.im.cached(key); ok {
			st.have = raster{key, pix}
		} else if key.w*key.h <= syncPixels {
			st.have = raster{key, v.im.rasterKey(key)}
		} else {
			st.want, st.busy = key, true
			tok := st.tok
			go func() {
				pix := v.im.rasterKey(key)
				tok.Post(func() {
					if st.want != key {
						return
					}
					st.have, st.busy = raster{key, pix}, false
					if st.invalidate != nil {
						st.invalidate()
					}
				})
			}()
		}
	}
	return size
}

func (n *canvasNode) Paint(ctx *gift.PaintContext) {
	st := n.st
	if st.have.pix == nil {
		return
	}
	images := ctx.Images()
	if images == nil {
		return
	}
	id, ok := images.Resolve(st.tex)
	if !ok || st.texKey != st.have.key {
		if !st.tex.IsZero() {
			images.Deallocate(st.tex)
		}
		p := st.have.pix
		h, ok := images.Acquire(render.Pixels{Pix: p.Pix, W: p.Rect.Dx(), H: p.Rect.Dy(), Stride: p.Stride,
			Key: rasterKeys.Add(1)})
		if !ok {
			// Refused by the upload budget of the frame: again in the next.
			st.tex = render.ImageHandle{}
			ctx.Animate(16 * time.Millisecond)
			return
		}
		st.tex, st.texKey = h, st.have.key
		id, ok = images.Resolve(h)
		if !ok {
			return
		}
	}
	ctx.Add(render.Op{Kind: render.OpImage, Bounds: ctx.Bounds(), Color: render.RGBA(255, 255, 255, 255), Image: id})
}

// cache keeps the last rasterisations of a document, which views of the same
// document at the same size share: a list of the same icon rasterises it
// once.
type cache struct {
	mu      sync.Mutex
	entries []raster
}

const cacheSize = 4

func (im *Image) cached(k rasterKey) (*image.RGBA, bool) {
	im.cache.mu.Lock()
	defer im.cache.mu.Unlock()
	for i, e := range im.cache.entries {
		if e.key == k {
			// The most recent first.
			copy(im.cache.entries[1:i+1], im.cache.entries[:i])
			im.cache.entries[0] = e
			return e.pix, true
		}
	}
	return nil, false
}

// rasterKey rasterises the document as k says and keeps the result.
func (im *Image) rasterKey(k rasterKey) *image.RGBA {
	var pix *image.RGBA
	if k.has {
		c := k.tint
		if c.A > 0 {
			pix = im.RasterizeTinted(k.w, k.h, float64(c.R/c.A), float64(c.G/c.A), float64(c.B/c.A), float64(c.A))
		} else {
			pix = image.NewRGBA(image.Rect(0, 0, k.w, k.h))
		}
	} else {
		pix = im.Rasterize(k.w, k.h)
	}
	im.cache.mu.Lock()
	defer im.cache.mu.Unlock()
	im.cache.entries = append([]raster{{k, pix}}, im.cache.entries...)
	if len(im.cache.entries) > cacheSize {
		im.cache.entries = im.cache.entries[:cacheSize]
	}
	return pix
}
