package qr

import (
	"math"
	"sync"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var viewType = gift.RegisterType("qr.View")

// CodeView shows a QR code. It is created by [View].
type CodeView struct {
	text       string
	level      Level
	size       float32
	label, key string
	flex       float32
}

// View returns a view of text as a QR code at level [M]: black modules on a
// white square with the quiet zone, in light and in dark appearance alike,
// because not every reader takes a code with its colours inverted.
//
//	qr.View("https://example.com/pay/4711").Size(240)
//
// Without a size it is four points per module, which a phone reads from an
// arm's length; never larger than there is room. The modules are snapped to
// whole pixels of the screen, so that every one is as wide as the next. A
// text too long for a QR code shows the white square without a code; [Encode]
// says why.
func View(text string) CodeView { return CodeView{text: text, level: M} }

// Level sets the error correction level, [M] by default.
func (v CodeView) Level(l Level) CodeView { v.level = l; return v }

// Size sets the length of each side, quiet zone included.
func (v CodeView) Size(s float32) CodeView { v.size = max(s, 0); return v }

// Label sets the accessible name, "QR code" by default.
func (v CodeView) Label(s string) CodeView { v.label = s; return v }

// Key sets the reconciliation key.
func (v CodeView) Key(s string) CodeView { v.key = s; return v }

// Flex sets the flex factor in a stack; a flexible code takes the room it is
// given, as a square.
func (v CodeView) Flex(f float32) CodeView { v.flex = f; return v }

// ViewType implements gift.View.
func (CodeView) ViewType() gift.TypeID { return viewType }

// Build implements gift.View.
func (v CodeView) Build(*gift.BuildContext) gift.Element {
	label := v.label
	if label == "" {
		label = "QR code"
	}
	n := &codeNode{v: v, code: cached(v.text, v.level)}
	return gift.Element{Key: v.key, Flex: v.flex, Label: label, Layouter: n, Painter: n}
}

type codeNode struct {
	v    CodeView
	code *Code
}

// modules is the number of modules on each side with the quiet zone.
func (n *codeNode) modules() int {
	if n.code == nil {
		return 21 + 2*QuietZone
	}
	return n.code.size + 2*QuietZone
}

func (n *codeNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	side := n.v.size
	if side == 0 {
		side = float32(n.modules() * 4)
		if n.v.flex > 0 && !math.IsInf(float64(c.Max.W), 1) && !math.IsInf(float64(c.Max.H), 1) {
			side = min(c.Max.W, c.Max.H)
		}
	}
	side = min(side, c.Max.W, c.Max.H)
	return c.Constrain(geom.Sz(side, side))
}

var (
	white = render.RGBA(255, 255, 255, 255)
	black = render.RGBA(0, 0, 0, 255)
)

func (n *codeNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	db := ctx.DeviceBounds()
	side := min(b.Width(), b.Height())
	if side <= 0 || db.Width() <= 0 {
		return
	}
	// Square in the middle of the bounds, in local units, and the scale
	// from local units to device pixels.
	x0, y0 := b.Min.X+(b.Width()-side)/2, b.Min.Y+(b.Height()-side)/2
	scale := db.Width() / b.Width()
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(x0, y0, x0+side, y0+side), Color: white,
		CornerRadius: min(12, side*0.04)})
	c := n.code
	if c == nil {
		return
	}
	// Whole device pixels per module where there are enough of them, so
	// that no module is a pixel wider than its neighbour; the code is then
	// centred on the device pixel grid.
	total := n.modules()
	dside := side * scale
	m := float32(math.Floor(float64(dside) / float64(total)))
	if m < 1 {
		m = dside / float32(total)
	}
	dx := db.Min.X + (x0-b.Min.X)*scale
	dy := db.Min.Y + (y0-b.Min.Y)*scale
	ox := float32(math.Round(float64(dx + (dside-m*float32(c.size))/2)))
	oy := float32(math.Round(float64(dy + (dside-m*float32(c.size))/2)))
	// Back to local units: the operations are in the node's space.
	lx := func(d float32) float32 { return b.Min.X + (d-db.Min.X)/scale }
	ly := func(d float32) float32 { return b.Min.Y + (d-db.Min.Y)/scale }
	for y := range c.size {
		top, bottom := ly(oy+float32(y)*m), ly(oy+float32(y+1)*m)
		for x := 0; x < c.size; {
			if !c.Dark(x, y) {
				x++
				continue
			}
			start := x
			for x < c.size && c.Dark(x, y) {
				x++
			}
			ctx.Add(render.Op{Kind: render.OpFillRect, Color: black,
				Bounds: geom.Rc(lx(ox+float32(start)*m), top, lx(ox+float32(x)*m), bottom)})
		}
	}
}

// encodings keeps the last codes, so that a view that is built again with
// the same text, as every view is on every change around it, does not
// encode it again.
var encodings struct {
	mu      sync.Mutex
	entries []encoding
}

type encoding struct {
	text  string
	level Level
	code  *Code
}

const encodingsKept = 8

func cached(text string, l Level) *Code {
	encodings.mu.Lock()
	defer encodings.mu.Unlock()
	for i, e := range encodings.entries {
		if e.text == text && e.level == l {
			copy(encodings.entries[1:i+1], encodings.entries[:i])
			encodings.entries[0] = e
			return e.code
		}
	}
	c, _ := Encode(text, l)
	encodings.entries = append([]encoding{{text, l, c}}, encodings.entries...)
	if len(encodings.entries) > encodingsKept {
		encodings.entries = encodings.entries[:encodingsKept]
	}
	return c
}
