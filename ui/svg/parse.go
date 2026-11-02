package svg

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/worldiety/gift/internal/iconsvg"
)

// Image is a parsed SVG document, ready to be drawn at any size. It is
// immutable and safe to share between goroutines and views.
type Image struct {
	w, h    float64
	view    [4]float64
	align   align
	shapes  []shape
	clips   []clipRef
	skipped []string
	cache   cache
}

// Size returns the size the document asks for, in logical pixels: its width
// and height, or those of its viewBox, or 300 by 150, as a browser takes it.
func (im *Image) Size() (w, h float64) { return im.w, im.h }

// Skipped returns what the document has that is not drawn, each name once:
// elements such as text or filter, and attributes such as an even-odd fill
// whose contours cross. An empty list means everything was drawn.
func (im *Image) Skipped() []string { return slices.Clone(im.skipped) }

// paintKind is what a fill or a stroke is.
type paintKind uint8

const (
	paintNone paintKind = iota
	paintColor
	paintGradient
)

type paint struct {
	kind  paintKind
	color rgba
	grad  *gradient
}

// gradient is a linear or radial gradient, with its stops.
type gradient struct {
	radial         bool
	x1, y1, x2, y2 float64
	cx, cy, r      float64
	fx, fy         float64
	userSpace      bool
	m              matrix
	spread         string
	stops          []stop
}

type stop struct {
	off float64
	c   rgba
}

// shape is one element to draw: its path in user units, its transform to
// the document, and how it is filled and stroked.
type shape struct {
	ops           []iconsvg.Op
	m             matrix
	fill, stroke  paint
	opacity       float64
	fillOpacity   float64
	strokeOpacity float64
	width         float64
	cap, join     string
	miter         float64
	dash          []float64
	dashOffset    float64
	clips         []int
	bbox          [4]float64
}

// clipRef is a clip path as one element uses it: its shapes in the user
// space of the element.
type clipRef struct {
	shapes []shape
}

// align is preserveAspectRatio.
type align struct {
	none  bool
	x, y  float64 // 0, 0.5 or 1
	slice bool
}

// node is an element of the document.
type node struct {
	name  string
	attrs map[string]string
	kids  []*node
}

// Parse reads an SVG document. Elements outside the subset this package
// draws are left out and reported by [Image.Skipped]; an error is returned
// only for a document that is no SVG at all or no XML.
func Parse(src []byte) (*Image, error) {
	root, err := parseXML(src)
	if err != nil {
		return nil, err
	}
	if root.name != "svg" {
		return nil, fmt.Errorf("svg: the document is a <%s>, not an <svg>", root.name)
	}
	p := &parser{img: &Image{}, ids: map[string]*node{}, clipIDs: map[clipKey]int{}}
	p.index(root)
	p.root(root)
	return p.img, nil
}

// ParseFile reads the SVG document in the file at path.
func ParseFile(path string) (*Image, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

const svgNS = "http://www.w3.org/2000/svg"

// parseXML reads the elements of the SVG namespace into a tree. Elements of
// other namespaces, the metadata of an editor, are left out with what is in
// them.
func parseXML(src []byte) (*node, error) {
	d := xml.NewDecoder(bytes.NewReader(src))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	var stack []*node
	var root *node
	skip := 0
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("svg: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if skip > 0 || t.Name.Space != "" && t.Name.Space != svgNS {
				skip++
				continue
			}
			n := &node{name: t.Name.Local, attrs: map[string]string{}}
			var style string
			for _, a := range t.Attr {
				switch {
				case a.Name.Local == "style" && a.Name.Space == "":
					style = a.Value
				case a.Name.Space == "" || a.Name.Space == svgNS:
					n.attrs[a.Name.Local] = a.Value
				case a.Name.Local == "href":
					n.attrs["href"] = a.Value
				}
			}
			// A declaration of the style attribute wins over the attribute of
			// the same name.
			parseStyle(style, n.attrs)
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("svg: more than one root element")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.kids = append(p.kids, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if root == nil {
		return nil, errors.New("svg: no element")
	}
	return root, nil
}

// style is what an element inherits from its parent.
type style struct {
	fill, stroke  string
	fillOpacity   float64
	strokeOpacity float64
	fillRule      string
	width         float64
	cap, join     string
	miter         float64
	dash          []float64
	dashOffset    float64
	color         rgba
	hidden        bool
}

var initialStyle = style{fill: "black", stroke: "none", fillOpacity: 1, strokeOpacity: 1, fillRule: "nonzero",
	width: 1, cap: "butt", join: "miter", miter: 4, color: rgba{0, 0, 0, 1}}

type parser struct {
	img     *Image
	ids     map[string]*node
	clipIDs map[clipKey]int
	depth   int
}

type clipKey struct {
	id string
	m  matrix
}

func (p *parser) index(n *node) {
	if id := n.attrs["id"]; id != "" {
		if _, dup := p.ids[id]; !dup {
			p.ids[id] = n
		}
	}
	for _, k := range n.kids {
		p.index(k)
	}
}

func (p *parser) skip(what string) {
	if !slices.Contains(p.img.skipped, what) {
		p.img.skipped = append(p.img.skipped, what)
	}
}

// root takes the size and the viewBox of the document and draws what is in
// it.
func (p *parser) root(n *node) {
	img := p.img
	vb := numbers(n.attrs["viewBox"])
	hasVB := len(vb) == 4 && vb[2] > 0 && vb[3] > 0
	w, okW := parseLength(n.attrs["width"], 0)
	h, okH := parseLength(n.attrs["height"], 0)
	if strings.HasSuffix(strings.TrimSpace(n.attrs["width"]), "%") {
		okW = false
	}
	if strings.HasSuffix(strings.TrimSpace(n.attrs["height"]), "%") {
		okH = false
	}
	switch {
	case okW && okH:
	case hasVB && okW:
		h = w * vb[3] / vb[2]
	case hasVB && okH:
		w = h * vb[2] / vb[3]
	case hasVB:
		w, h = vb[2], vb[3]
	default:
		w, h = 300, 150
	}
	img.w, img.h = max(w, 0), max(h, 0)
	if hasVB {
		img.view = [4]float64{vb[0], vb[1], vb[2], vb[3]}
	} else {
		img.view = [4]float64{0, 0, img.w, img.h}
	}
	img.align = parseAlign(n.attrs["preserveAspectRatio"])
	p.children(n, p.inherit(initialStyle, n), identity, nil, 1)
}

func parseAlign(s string) align {
	a := align{x: 0.5, y: 0.5}
	f := strings.Fields(s)
	if len(f) == 0 {
		return a
	}
	if f[0] == "none" {
		a.none = true
		return a
	}
	if len(f[0]) == 8 {
		pos := map[string]float64{"Min": 0, "Mid": 0.5, "Max": 1}
		a.x, a.y = pos[f[0][1:4]], pos[f[0][5:8]]
	}
	a.slice = len(f) > 1 && f[1] == "slice"
	return a
}

// inherit returns the style of n: what it inherits, changed by its own
// presentation attributes.
func (p *parser) inherit(st style, n *node) style {
	a := n.attrs
	if v, ok := a["color"]; ok {
		if c, ok := parseColor(v); ok {
			st.color = c
		}
	}
	if v, ok := a["fill"]; ok && v != "inherit" {
		st.fill = v
	}
	if v, ok := a["stroke"]; ok && v != "inherit" {
		st.stroke = v
	}
	opacity := func(k string, into *float64) {
		if v, ok := a[k]; ok {
			if f, ok := parseOpacity(v); ok {
				*into = f
			}
		}
	}
	opacity("fill-opacity", &st.fillOpacity)
	opacity("stroke-opacity", &st.strokeOpacity)
	if v, ok := a["fill-rule"]; ok {
		st.fillRule = strings.TrimSpace(v)
	}
	if v, ok := a["stroke-width"]; ok {
		if f, ok := parseLength(v, 1); ok && f >= 0 {
			st.width = f
		}
	}
	if v, ok := a["stroke-linecap"]; ok {
		st.cap = strings.TrimSpace(v)
	}
	if v, ok := a["stroke-linejoin"]; ok {
		st.join = strings.TrimSpace(v)
	}
	if v, ok := a["stroke-miterlimit"]; ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f >= 1 {
			st.miter = f
		}
	}
	if v, ok := a["stroke-dasharray"]; ok {
		st.dash = nil
		if v = strings.TrimSpace(v); v != "none" {
			for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
				if l, ok := parseLength(f, 1); ok && l >= 0 {
					st.dash = append(st.dash, l)
				}
			}
		}
	}
	if v, ok := a["stroke-dashoffset"]; ok {
		if f, ok := parseLength(v, 1); ok {
			st.dashOffset = f
		}
	}
	if v, ok := a["visibility"]; ok {
		v = strings.TrimSpace(v)
		st.hidden = v == "hidden" || v == "collapse"
	}
	return st
}

func parseOpacity(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
	if err != nil {
		return 0, false
	}
	if strings.HasSuffix(v, "%") {
		f /= 100
	}
	return math.Min(math.Max(f, 0), 1), true
}

func (p *parser) children(n *node, st style, m matrix, clips []int, opacity float64) {
	for _, k := range n.kids {
		p.element(k, st, m, clips, opacity)
	}
}

// element draws n inside a parent of style st, transform m, clips and
// opacity.
func (p *parser) element(n *node, st style, m matrix, clips []int, opacity float64) {
	a := n.attrs
	if strings.TrimSpace(a["display"]) == "none" {
		return
	}
	if p.depth > 64 {
		p.skip("deep nesting")
		return
	}
	p.depth++
	defer func() { p.depth-- }()
	if t, ok := a["transform"]; ok {
		tm, ok := parseTransform(t)
		if !ok {
			p.skip("transform")
		}
		m = m.mul(tm)
	}
	st = p.inherit(st, n)
	if v, ok := a["opacity"]; ok {
		// Group opacity is multiplied into the children, which is the same
		// where they do not overlap.
		if f, ok := parseOpacity(v); ok {
			opacity *= f
		}
	}
	if v, ok := a["clip-path"]; ok && strings.TrimSpace(v) != "none" {
		if id, ok := urlRef(v); ok {
			if k, ok := p.clip(id, m); ok {
				clips = append(slices.Clip(clips), k)
			}
		}
	}
	if _, ok := a["mask"]; ok {
		p.skip("mask")
	}
	if _, ok := a["filter"]; ok {
		p.skip("filter")
	}
	switch n.name {
	case "g", "a", "svg", "switch":
		if n.name == "svg" {
			x, _ := parseLength(a["x"], 0)
			y, _ := parseLength(a["y"], 0)
			m = m.mul(matrix{1, 0, 0, 1, x, y})
		}
		p.children(n, st, m, clips, opacity)
	case "use":
		id := strings.TrimPrefix(a["href"], "#")
		target := p.ids[id]
		if target == nil || target == n {
			return
		}
		x, _ := parseLength(a["x"], 0)
		y, _ := parseLength(a["y"], 0)
		m = m.mul(matrix{1, 0, 0, 1, x, y})
		if target.name == "symbol" {
			p.children(target, p.inherit(st, target), m, clips, opacity)
			return
		}
		p.element(target, st, m, clips, opacity)
	case "path", "rect", "circle", "ellipse", "line", "polyline", "polygon":
		if st.hidden {
			return
		}
		ops := p.geometry(n)
		if len(ops) == 0 {
			return
		}
		p.shape(ops, st, m, clips, opacity, n.name)
	case "defs", "symbol", "clipPath", "linearGradient", "radialGradient", "stop", "title", "desc",
		"metadata", "style", "script":
		if n.name == "style" {
			p.skip("style sheets")
		}
	default:
		p.skip(n.name)
	}
}

// shape adds the element with the path ops.
func (p *parser) shape(ops []iconsvg.Op, st style, m matrix, clips []int, opacity float64, kind string) {
	s := shape{ops: ops, m: m, opacity: opacity, fillOpacity: st.fillOpacity, strokeOpacity: st.strokeOpacity,
		width: st.width, cap: st.cap, join: st.join, miter: st.miter, dash: st.dash, dashOffset: st.dashOffset,
		clips: clips, bbox: bounds(ops)}
	if kind != "line" {
		s.fill = p.paint(st.fill, st)
	}
	s.stroke = p.paint(st.stroke, st)
	if s.fill.kind != paintNone && st.fillRule == "evenodd" {
		s.ops = p.evenOdd(ops, s.bbox)
	}
	if s.fill.kind == paintNone && s.stroke.kind == paintNone {
		return
	}
	p.img.shapes = append(p.img.shapes, s)
}

// evenOdd orients the contours of an even-odd path so that the nonzero rule
// of the rasteriser fills the same area, which is exact for contours that do
// not cross; for ones that do the path is filled by the nonzero rule.
func (p *parser) evenOdd(ops []iconsvg.Op, bb [4]float64) []iconsvg.Op {
	w, h := bb[2]-bb[0], bb[3]-bb[1]
	box := max(w, h)
	if !(box > 0) {
		return ops
	}
	moved := shift(ops, -bb[0], -bb[1])
	out, ok := iconsvg.Orient(moved, box, 48)
	if !ok {
		p.skip("fill-rule evenodd with crossing contours")
		return ops
	}
	return shift(out, bb[0], bb[1])
}

func shift(ops []iconsvg.Op, dx, dy float64) []iconsvg.Op {
	out := make([]iconsvg.Op, len(ops))
	for i, op := range ops {
		for k := range op.P {
			op.P[k][0] += dx
			op.P[k][1] += dy
		}
		out[i] = op
	}
	return out
}

// paint resolves a fill or a stroke: none, a colour, currentColor, or a
// gradient by url, with the colour after it as the fallback.
func (p *parser) paint(v string, st style) paint {
	v = strings.TrimSpace(v)
	switch v {
	case "", "none":
		return paint{}
	case "currentColor", "currentcolor":
		return paint{kind: paintColor, color: st.color}
	}
	if id, ok := urlRef(v); ok {
		if g := p.gradient(id, 0); g != nil {
			if len(g.stops) == 1 {
				return paint{kind: paintColor, color: g.stops[0].c}
			}
			if len(g.stops) > 1 {
				return paint{kind: paintGradient, grad: g}
			}
			return paint{}
		}
		if n := p.ids[id]; n != nil && n.name == "pattern" {
			p.skip("pattern")
		}
		fallback := strings.TrimSpace(v[strings.IndexByte(v, ')')+1:])
		if c, ok := parseColor(fallback); ok {
			return paint{kind: paintColor, color: c}
		}
		return paint{}
	}
	c, ok := parseColor(v)
	if !ok {
		p.skip("colour " + v)
		return paint{}
	}
	return paint{kind: paintColor, color: c}
}

// gradient resolves the gradient id, with what it takes from the gradient
// its href names.
func (p *parser) gradient(id string, depth int) *gradient {
	n := p.ids[id]
	if n == nil || n.name != "linearGradient" && n.name != "radialGradient" || depth > 8 {
		return nil
	}
	g := &gradient{radial: n.name == "radialGradient", m: identity, spread: "pad",
		x1: 0, y1: 0, x2: 1, y2: 0, cx: 0.5, cy: 0.5, r: 0.5}
	var base *gradient
	if href := strings.TrimPrefix(n.attrs["href"], "#"); href != "" {
		base = p.gradient(href, depth+1)
	}
	a := n.attrs
	units := a["gradientUnits"]
	if base != nil {
		if base.radial == g.radial {
			*g = *base
			g.stops = slices.Clone(base.stops)
		} else {
			g.userSpace, g.m, g.spread, g.stops = base.userSpace, base.m, base.spread, base.stops
		}
	}
	if units != "" {
		g.userSpace = units == "userSpaceOnUse"
	}
	if t, ok := a["gradientTransform"]; ok {
		if m, ok := parseTransform(t); ok {
			g.m = m
		}
	}
	if s, ok := a["spreadMethod"]; ok {
		g.spread = s
	}
	coord := func(k string, into *float64) {
		v, ok := a[k]
		if !ok {
			return
		}
		ref := 1.0
		if g.userSpace {
			ref = 100
		}
		if f, ok := parseLength(v, ref); ok {
			*into = f
		}
	}
	coord("x1", &g.x1)
	coord("y1", &g.y1)
	coord("x2", &g.x2)
	coord("y2", &g.y2)
	coord("cx", &g.cx)
	coord("cy", &g.cy)
	coord("r", &g.r)
	g.fx, g.fy = g.cx, g.cy
	if base != nil && base.radial && g.radial && a["fx"] == "" && a["cx"] == "" {
		g.fx, g.fy = base.fx, base.fy
	}
	coord("fx", &g.fx)
	coord("fy", &g.fy)
	var stops []stop
	for _, k := range n.kids {
		if k.name != "stop" {
			continue
		}
		off, _ := parseOpacity(k.attrs["offset"])
		c := rgba{0, 0, 0, 1}
		if v, ok := k.attrs["stop-color"]; ok {
			if cc, ok := parseColor(v); ok {
				c = cc
			}
		}
		if v, ok := k.attrs["stop-opacity"]; ok {
			if f, ok := parseOpacity(v); ok {
				c[3] *= f
			}
		}
		if len(stops) > 0 {
			off = max(off, stops[len(stops)-1].off)
		}
		stops = append(stops, stop{off, c})
	}
	if len(stops) > 0 {
		g.stops = stops
	}
	return g
}

// clip returns the index of the clip path id as used by an element whose
// transform is m.
func (p *parser) clip(id string, m matrix) (int, bool) {
	key := clipKey{id, m}
	if k, ok := p.clipIDs[key]; ok {
		return k, true
	}
	n := p.ids[id]
	if n == nil || n.name != "clipPath" {
		return 0, false
	}
	if n.attrs["clipPathUnits"] == "objectBoundingBox" {
		p.skip("clipPathUnits objectBoundingBox")
		return 0, false
	}
	var ref clipRef
	var walk func(k *node, m matrix)
	walk = func(k *node, m matrix) {
		if t, ok := k.attrs["transform"]; ok {
			if tm, ok := parseTransform(t); ok {
				m = m.mul(tm)
			}
		}
		switch k.name {
		case "use":
			if t := p.ids[strings.TrimPrefix(k.attrs["href"], "#")]; t != nil && t != k {
				x, _ := parseLength(k.attrs["x"], 0)
				y, _ := parseLength(k.attrs["y"], 0)
				walk(t, m.mul(matrix{1, 0, 0, 1, x, y}))
			}
		case "g":
			for _, kk := range k.kids {
				walk(kk, m)
			}
		default:
			if ops := p.geometry(k); len(ops) > 0 {
				st := p.inherit(initialStyle, k)
				if v, ok := k.attrs["clip-rule"]; ok && strings.TrimSpace(v) == "evenodd" {
					ops = p.evenOdd(ops, bounds(ops))
				} else if st.fillRule == "evenodd" {
					ops = p.evenOdd(ops, bounds(ops))
				}
				ref.shapes = append(ref.shapes, shape{ops: ops, m: m})
			}
		}
	}
	for _, k := range n.kids {
		walk(k, m)
	}
	p.img.clips = append(p.img.clips, ref)
	p.clipIDs[key] = len(p.img.clips) - 1
	return len(p.img.clips) - 1, true
}

// geometry returns the path of a shape element in user units, nil for one
// that has none or is not a shape.
func (p *parser) geometry(n *node) []iconsvg.Op {
	a := n.attrs
	num := func(k string) float64 {
		f, _ := parseLength(a[k], 0)
		return f
	}
	switch n.name {
	case "path":
		ops, err := iconsvg.ParsePathData(a["d"])
		if err != nil {
			// The path as far as it is well formed, as a browser draws it.
			ops = p.prefix(a["d"])
			p.skip("malformed path data")
		}
		return ops
	case "rect":
		w, h := num("width"), num("height")
		if w <= 0 || h <= 0 {
			return nil
		}
		rx, okx := parseLength(a["rx"], 0)
		ry, oky := parseLength(a["ry"], 0)
		switch {
		case okx && !oky:
			ry = rx
		case oky && !okx:
			rx = ry
		}
		return rect(num("x"), num("y"), w, h, min(max(rx, 0), w/2), min(max(ry, 0), h/2))
	case "circle":
		r := num("r")
		if r <= 0 {
			return nil
		}
		return ellipse(num("cx"), num("cy"), r, r)
	case "ellipse":
		rx, ry := num("rx"), num("ry")
		if rx <= 0 || ry <= 0 {
			return nil
		}
		return ellipse(num("cx"), num("cy"), rx, ry)
	case "line":
		return []iconsvg.Op{
			{Kind: 'M', P: [3][2]float64{{num("x1"), num("y1")}}},
			{Kind: 'L', P: [3][2]float64{{num("x2"), num("y2")}}},
		}
	case "polyline", "polygon":
		v := numbers(a["points"])
		if len(v) < 4 {
			return nil
		}
		ops := []iconsvg.Op{{Kind: 'M', P: [3][2]float64{{v[0], v[1]}}}}
		for i := 2; i+1 < len(v); i += 2 {
			ops = append(ops, iconsvg.Op{Kind: 'L', P: [3][2]float64{{v[i], v[i+1]}}})
		}
		if n.name == "polygon" {
			ops = append(ops, iconsvg.Op{Kind: 'Z'})
		}
		return ops
	}
	return nil
}

// prefix returns the longest well formed beginning of the path data d,
// command by command.
func (p *parser) prefix(d string) []iconsvg.Op {
	var best []iconsvg.Op
	for i := 1; i < len(d); i++ {
		c := d[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			if c == 'e' || c == 'E' {
				continue
			}
			if ops, err := iconsvg.ParsePathData(d[:i]); err == nil {
				best = ops
			}
		}
	}
	return best
}

// kappa is the length of the control arms of a cubic that approximates a
// quarter circle of radius 1.
const kappa = 0.5522847498

func ellipse(cx, cy, rx, ry float64) []iconsvg.Op {
	kx, ky := rx*kappa, ry*kappa
	c := func(x1, y1, x2, y2, x, y float64) iconsvg.Op {
		return iconsvg.Op{Kind: 'C', P: [3][2]float64{{x1, y1}, {x2, y2}, {x, y}}}
	}
	return []iconsvg.Op{
		{Kind: 'M', P: [3][2]float64{{cx + rx, cy}}},
		c(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry),
		c(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy),
		c(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry),
		c(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy),
		{Kind: 'Z'},
	}
}

func rect(x, y, w, h, rx, ry float64) []iconsvg.Op {
	l := func(x, y float64) iconsvg.Op { return iconsvg.Op{Kind: 'L', P: [3][2]float64{{x, y}}} }
	if rx <= 0 || ry <= 0 {
		return []iconsvg.Op{{Kind: 'M', P: [3][2]float64{{x, y}}}, l(x+w, y), l(x+w, y+h), l(x, y+h), {Kind: 'Z'}}
	}
	kx, ky := rx*kappa, ry*kappa
	c := func(x1, y1, x2, y2, x, y float64) iconsvg.Op {
		return iconsvg.Op{Kind: 'C', P: [3][2]float64{{x1, y1}, {x2, y2}, {x, y}}}
	}
	r, b := x+w, y+h
	return []iconsvg.Op{
		{Kind: 'M', P: [3][2]float64{{x + rx, y}}},
		l(r-rx, y), c(r-rx+kx, y, r, y+ry-ky, r, y+ry),
		l(r, b-ry), c(r, b-ry+ky, r-rx+kx, b, r-rx, b),
		l(x+rx, b), c(x+rx-kx, b, x, b-ry+ky, x, b-ry),
		l(x, y+ry), c(x, y+ry-ky, x+rx-kx, y, x+rx, y),
		{Kind: 'Z'},
	}
}

// bounds returns the box of the points of ops, control points included:
// the box a gradient of objectBoundingBox units spans, near enough.
func bounds(ops []iconsvg.Op) [4]float64 {
	b := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, op := range ops {
		n := 0
		switch op.Kind {
		case 'M', 'L':
			n = 1
		case 'C':
			n = 3
		}
		for _, pt := range op.P[:n] {
			b[0], b[1] = min(b[0], pt[0]), min(b[1], pt[1])
			b[2], b[3] = max(b[2], pt[0]), max(b[3], pt[1])
		}
	}
	if math.IsInf(b[0], 1) {
		return [4]float64{}
	}
	return b
}
