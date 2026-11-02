package charts

import (
	"math"
	"strconv"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// valueLabel is the label of a bar: its text and the bar it labels.
type valueLabel struct {
	text  string
	bar   int
	in    bool
	color render.Color
}

// maxValueLabels is the most bars that are labelled; more are too many to
// read.
const maxValueLabels = 100

// valueLabels works out the labels of the bars of marks with value labels:
// in the segment when the bars are stacked, above or below the bar
// otherwise.
func (st *state) valueLabels() {
	st.values = st.values[:0]
	stacked := st.v.stacking == Stacked || st.v.stacking == Normalized
	n := 0
	for _, m := range st.v.marks {
		if m.spec().kind == kindBar {
			n++
		}
	}
	stacked = stacked && n > 1
	for k, b := range st.bars {
		s := st.v.marks[b.mark].spec()
		if !s.values || b.thin || b.x1 < st.lo || b.x0 > st.hi {
			continue
		}
		text := st.v.formatY(b.value)
		if st.normal {
			text = strconv.Itoa(int(math.Round((b.y1-b.y0)*100))) + " %"
		}
		st.values = append(st.values, valueLabel{text: text, bar: k, in: stacked})
		if len(st.values) > maxValueLabels {
			st.values = st.values[:0]
			return
		}
	}
}

func sameValues(a, b []valueLabel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].text != b[i].text || a[i].in != b[i].in {
			return false
		}
	}
	return true
}

// valueViews are the labels of the bars, as children.
func (st *state) valueViews() []gift.View {
	st.builtValues = append(st.builtValues[:0], st.values...)
	var kids []gift.View
	for _, l := range st.values {
		c := ui.Color(ui.ColorSecondaryLabel)
		if l.in {
			c = ui.RGB(255, 255, 255)
		}
		kids = append(kids, ui.Text(l.text).FontSize(labelSize).Foreground(c).MaxLines(1))
	}
	return kids
}

// placeValues puts the label of each bar into its segment, if it fits, or
// above a positive bar and below a negative one, leaving out a label that
// would run into the one before.
func (n *plotNode) placeValues(ctx *gift.LayoutContext, plot geom.Rect) {
	st := n.st
	p := st.plan
	loose := geom.Constraints{Max: geom.Sz(plot.Width(), plot.Height())}
	edge := float32(math.Inf(-1))
	for k, i := 0, p.values[0]; i < p.values[1]; k, i = k+1, i+1 {
		s := ctx.Measure(i, loose)
		if k >= len(st.builtValues) || st.builtValues[k].bar >= len(st.bars) {
			ctx.Place(i, hidden)
			continue
		}
		l := st.builtValues[k]
		b := st.bars[l.bar]
		x0, x1 := float32(st.plotX(b.x0)), float32(st.plotX(b.x1))
		y0, y1 := float32(st.plotY(b.y1)), float32(st.plotY(b.y0))
		cx := (x0 + x1) / 2
		var at geom.Point
		switch {
		case l.in:
			if s.W > x1-x0-2 || s.H > y1-y0-2 {
				ctx.Place(i, hidden)
				continue
			}
			at = geom.Pt(cx-s.W/2, (y0+y1)/2-s.H/2)
		case b.value >= 0:
			at = geom.Pt(cx-s.W/2, y0-s.H-1)
		default:
			at = geom.Pt(cx-s.W/2, y1+1)
		}
		if !l.in && (at.X < edge+2 || at.Y < plot.Min.Y-2 || at.Y+s.H > plot.Max.Y+2) {
			ctx.Place(i, hidden)
			continue
		}
		if !l.in {
			edge = at.X + s.W
		}
		ctx.Place(i, at)
	}
}

// noteViews are the texts of the annotations, as children.
func (st *state) noteViews(p *plan) []gift.View {
	var kids []gift.View
	for i, m := range st.v.marks {
		s := m.spec()
		if s.kind == kindNote && s.label != "" {
			p.noteOf = append(p.noteOf, i)
			kids = append(kids, ui.Text(s.label).FontSize(labelSize).Foreground(ui.ColorLabel).MaxLines(1))
		}
	}
	return kids
}

// placeNotes puts the text of each annotation beside its point, as its
// position says, and hides the ones whose point is not on the plot.
func (n *plotNode) placeNotes(ctx *gift.LayoutContext, plot geom.Rect) {
	st := n.st
	p := st.plan
	loose := geom.Constraints{Max: geom.Sz(plot.Width(), plot.Height())}
	for k, i := 0, p.notes[0]; i < p.notes[1]; k, i = k+1, i+1 {
		s := st.v.marks[p.noteOf[k]].spec()
		size := ctx.Measure(i, loose)
		x, y := float32(st.plotX(s.a)), float32(st.plotY(s.b))
		if !plot.Contains(geom.Pt(x, y)) {
			ctx.Place(i, hidden)
			continue
		}
		const off = 7
		var at geom.Point
		switch s.pos {
		case Bottom:
			at = geom.Pt(x-size.W/2, y+off)
		case Leading:
			at = geom.Pt(x-off-size.W, y-size.H/2)
		case Trailing:
			at = geom.Pt(x+off, y-size.H/2)
		default:
			at = geom.Pt(x-size.W/2, y-off-size.H)
		}
		at.X = min(max(at.X, plot.Min.X), plot.Max.X-size.W)
		at.Y = min(max(at.Y, plot.Min.Y), plot.Max.Y-size.H)
		ctx.Place(i, at)
	}
}

// paintNotes draws the dots of the annotations.
func (n *plotNode) paintNotes(ctx *gift.PaintContext, ox, oy float64, plot geom.Rect) {
	st := n.st
	ring := ui.ResolveColor(ui.ColorSurface)
	for i, m := range st.v.marks {
		s := m.spec()
		if s.kind != kindNote || s.nodot {
			continue
		}
		x, y := float32(ox+st.plotX(s.a)), float32(oy+st.plotY(s.b))
		if !plot.Contains(geom.Pt(x, y)) {
			continue
		}
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(x-5, y-5, x+5, y+5), Color: ring, CornerRadius: 5})
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(x-3.5, y-3.5, x+3.5, y+3.5), Color: st.cache[i].color, CornerRadius: 3.5})
	}
}
