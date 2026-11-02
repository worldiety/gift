package ui

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var (
	disclosureType        = gift.RegisterType("ui.DisclosureGroup")
	disclosureBodyType    = gift.RegisterType("ui.disclosureBody")
	disclosureContentType = gift.RegisterType("ui.disclosureContent")
	disclosureChevronType = gift.RegisterType("ui.disclosureChevron")
	accordionType         = gift.RegisterType("ui.Accordion")
)

// DisclosureDuration is how long a [DisclosureGroupView] takes to open or
// close.
const DisclosureDuration = 220 * time.Millisecond

// disclosureStep is how often the group is laid out while it opens or
// closes.
const disclosureStep = 16 * time.Millisecond

// DisclosureGroupView is a row that shows or hides content below it, after
// SwiftUI's DisclosureGroup: a chevron at its trailing edge points to the
// right while the content is hidden and down while it shows. It is created by
// [DisclosureGroup]; [Accordion] keeps at most one of several open.
//
//	ui.DisclosureGroup("Advanced",
//		ui.Toggle(ctx.Read(debug), debug.Set),
//		ui.Text("Logs go to the system journal."),
//	).Icon(outline.Cog)
//
// # The label is a row
//
// The label is a [RowView] with the title, and with what [DisclosureGroupView.Icon],
// [DisclosureGroupView.Subtitle], [DisclosureGroupView.Value] and
// [DisclosureGroupView.Leading] add, so a group sits among the rows of a
// [ListView] or a [Card] as one of them, with the same hover, press, focus
// and keyboard. A tap, a click, space or enter anywhere on it opens and
// closes the group; an interactive view among the content does not.
//
// # Content
//
// The content is laid out below the label in a [VStack], leading aligned,
// eight points apart and inset by [DisclosureGroupView.ContentPadding]: by
// default as far as the title, so that a paragraph lines up with it, also
// after an icon, whose width a [DisclosureGroupView.Leading] view is taken to
// have. Rows go into a [ListView] of their own, which gives them their
// separators, and bring their own padding:
//
//	ui.DisclosureGroup("Advanced", ui.List(proxy, dns)).ContentPadding(geom.Insets{Left: 34})
//
// The content is built when the group opens and taken out of the tree when
// it has closed, as SwiftUI does: a closed group costs its label and nothing
// else, however much it holds, and the state of what it held starts over the
// next time it opens. Content whose state must outlive that keeps it in the
// component around the group.
//
// # Open or closed
//
// Without [DisclosureGroupView.Expanded] the group keeps whether it is open
// itself and starts closed. With it, the application owns the value, as with
// SwiftUI's isExpanded binding: the group shows what it is given and reports
// a tap through the callback, which is how several groups are tied together
// and how [Accordion] works.
//
// Opening and closing take [DisclosureDuration]: the content is revealed
// from the top and the chevron turns, both on gift's clock, without a build
// per frame. A group that is built open shows its content at once.
type DisclosureGroupView struct {
	row        RowView
	title      string
	content    []gift.View
	expanded   bool
	onChange   func(bool)
	controlled bool
	inset      geom.Insets
	hasInset   bool
	disabled   bool
	key        string
	flex       float32
}

// DisclosureGroup returns a group titled title that shows content below its
// label while it is open. The content slice belongs to gift from this call
// onwards, exactly like [VStack]'s.
func DisclosureGroup(title string, content ...gift.View) DisclosureGroupView {
	return DisclosureGroupView{row: Row(title), title: title, content: content}
}

// Expanded makes the application own whether the group is open: it shows
// open as given and calls onChange with the value a tap asks for.
func (v DisclosureGroupView) Expanded(open bool, onChange func(bool)) DisclosureGroupView {
	v.expanded, v.onChange, v.controlled = open, onChange, true
	return v
}

// Icon sets the leading icon of the label; see [RowView.Icon].
func (v DisclosureGroupView) Icon(s Symbol) DisclosureGroupView { v.row = v.row.Icon(s); return v }

// Subtitle sets a second line under the title; see [RowView.Subtitle].
func (v DisclosureGroupView) Subtitle(s string) DisclosureGroupView {
	v.row = v.row.Subtitle(s)
	return v
}

// Value sets a secondary text before the chevron, what the content amounts
// to, such as "3 networks"; see [RowView.Value].
func (v DisclosureGroupView) Value(s string) DisclosureGroupView { v.row = v.row.Value(s); return v }

// Leading sets a view in place of the leading icon; see [RowView.Leading].
func (v DisclosureGroupView) Leading(view gift.View) DisclosureGroupView {
	v.row = v.row.Leading(view)
	return v
}

// ContentPadding sets the inset of the content. The default insets it as far
// as the title on the leading side, by [RowPadding] on the trailing side and
// by 12 points below.
func (v DisclosureGroupView) ContentPadding(i geom.Insets) DisclosureGroupView {
	v.inset, v.hasInset = i, true
	return v
}

// Disabled takes the label out of input and dims it, as [RowView.Disabled]
// does; an open group stays open.
func (v DisclosureGroupView) Disabled(b bool) DisclosureGroupView { v.disabled = b; return v }

// Label sets the accessible name of the label, the title by default.
func (v DisclosureGroupView) Label(s string) DisclosureGroupView { v.row = v.row.Label(s); return v }

// Key sets the reconciliation key.
func (v DisclosureGroupView) Key(s string) DisclosureGroupView { v.key = s; return v }

// Flex sets the flex factor in a stack.
func (v DisclosureGroupView) Flex(f float32) DisclosureGroupView { v.flex = checkFlex(f); return v }

// ViewType implements gift.View.
func (DisclosureGroupView) ViewType() gift.TypeID { return disclosureType }

// Build implements gift.View.
func (v DisclosureGroupView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Flex: v.flex, Layouter: anchorLayout{},
		Children: []gift.View{gift.Component("ui.disclosure", v.body)}}
}

// disclosure is what a group keeps across builds: whether it is open, when
// it is not the application's to say, and where its opening stands. Build
// and layout share it; both run on the UI goroutine.
type disclosure struct {
	open bool
	// showing reports whether the content is in the tree: from the build
	// that opens the group to the layout in which it has closed.
	showing bool
	// The movement, as a phase from 0 closed to 1 open on the clock of
	// [gift.App.BeginInput]: where it started, when, and where it goes.
	from, target float32
	start        time.Duration
	armed        bool
}

// phase returns how far open the group is at now, eased.
func (d *disclosure) phase(now time.Duration) float32 {
	t := now - d.start
	switch {
	case !d.armed:
		return d.target
	case t <= 0:
		return d.from
	case t >= DisclosureDuration:
		return d.target
	}
	return d.from + (d.target-d.from)*easeInOut(float32(t)/float32(DisclosureDuration))
}

func (v DisclosureGroupView) body(ctx *gift.Context) gift.View {
	mem := ctx.State("disclosure", (*disclosure)(nil))
	st := mem.Get()
	if st == nil {
		st = &disclosure{}
		mem.Set(st)
	}
	changed := ctx.State("changed", 0)
	ctx.Read(changed)

	open := st.open
	if v.controlled {
		open = v.expanded
	}
	toggle := func() {
		if v.controlled {
			if v.onChange != nil {
				v.onChange(!open)
			}
			return
		}
		st.open = !st.open
		changed.Set(changed.Get() + 1)
	}
	if open {
		st.showing = true
	}

	row := v.row.Accessory(disclosureChevron{st: st, disabled: v.disabled}).Disabled(v.disabled)
	if !v.disabled {
		row = row.OnTap(toggle)
	}
	kids := []gift.View{row}
	if st.showing {
		inset := geom.Insets{Right: RowPadding, Bottom: 12, Left: RowPadding}
		if v.row.leading != nil || !v.row.sym.IsZero() {
			// As far as the title, after an icon's width.
			inset.Left += rowIconSize + rowGap
		}
		if v.hasInset {
			inset = v.inset
		}
		kids = append(kids, disclosureContent{st: st,
			child: VStack(v.content...).Gap(8).Align(geom.Leading).PaddingInsets(inset)})
	}
	return disclosureBody{st: st, open: open, kids: kids}
}

// disclosureBody lays out the label and, below it, as much of the content as
// the group is open.
type disclosureBody struct {
	st   *disclosure
	open bool
	kids []gift.View
}

func (disclosureBody) ViewType() gift.TypeID { return disclosureBodyType }

func (b disclosureBody) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Layouter: disclosureBodyNode{st: b.st, open: b.open}, Children: b.kids}
}

type disclosureBodyNode struct {
	st   *disclosure
	open bool
}

func (n disclosureBodyNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	st, now := n.st, ctx.Now()
	want := float32(0)
	if n.open {
		want = 1
	}
	switch {
	case !st.armed:
		st.armed, st.from, st.target = true, want, want
	case st.target != want:
		st.from, st.start, st.target = st.phase(now), now, want
	}
	// The width is the room's when it is bounded, so that the label's face
	// and the content reach across it, as a row's do in a list.
	inf := float32(math.Inf(1))
	wc := geom.Constraints{Min: geom.Sz(c.Min.W, 0), Max: geom.Sz(c.Max.W, inf)}
	label := ctx.Measure(0, wc)
	ctx.Place(0, geom.Point{})
	size := label
	if ctx.ChildCount() > 1 {
		body := ctx.Measure(1, geom.Constraints{Min: geom.Sz(max(c.Min.W, label.W), 0), Max: geom.Sz(c.Max.W, inf)})
		ctx.Place(1, geom.Pt(0, label.H))
		size = geom.Sz(max(label.W, body.W), label.H+body.H)
	}
	return c.Constrain(size)
}

// disclosureContent shows the top of the content, as far as the group is
// open, and clips the rest away from paint and input alike.
type disclosureContent struct {
	st    *disclosure
	child gift.View
}

func (disclosureContent) ViewType() gift.TypeID { return disclosureContentType }

func (d disclosureContent) Build(*gift.BuildContext) gift.Element {
	n := &disclosureContentNode{st: d.st}
	return gift.Element{Key: "content", Layouter: n, Interactor: n, Clip: true, Children: []gift.View{d.child}}
}

type disclosureContentNode struct{ st *disclosure }

func (n *disclosureContentNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	st, now := n.st, ctx.Now()
	full := ctx.Measure(0, geom.Constraints{Min: geom.Sz(c.Min.W, 0), Max: geom.Sz(c.Max.W, float32(math.Inf(1)))})
	ctx.Place(0, geom.Point{})
	p := st.phase(now)
	if p != st.target {
		// The timer fires when the clock has moved, so a frame drawn again
		// at the same moment lays nothing out.
		ctx.After(disclosureStep)
		ctx.Animate(disclosureStep)
	} else if p == 0 && st.showing {
		// Closed: the content leaves the tree with the next build.
		st.showing = false
		ctx.RequestBuild()
	}
	return geom.Sz(full.W, full.H*p)
}

// HandleEvent takes the timer that moves the opening on.
func (n *disclosureContentNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventTimer {
		return false
	}
	ctx.RequestLayout()
	ctx.Repaint()
	return true
}

// disclosureChevron is the chevron of the label, turned as far as the group
// is open: to the right closed, down open.
type disclosureChevron struct {
	st       *disclosure
	disabled bool
}

func (disclosureChevron) ViewType() gift.TypeID { return disclosureChevronType }

func (d disclosureChevron) Build(*gift.BuildContext) gift.Element {
	col := ColorAccent
	if d.disabled {
		col = ColorSecondaryLabel
	}
	n := &disclosureChevronNode{st: d.st, col: ResolveColor(col)}
	return gift.Element{Layouter: n, Painter: n}
}

type disclosureChevronNode struct {
	st  *disclosure
	col Color
}

func (n *disclosureChevronNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(geom.Sz(rowChevron, rowChevron))
}

func (n *disclosureChevronNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	p := n.st.phase(ctx.Now())
	a := float64(p) * math.Pi / 2
	c := geom.Pt((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2)
	paintChevron(ctx, c, geom.Pt(float32(math.Cos(a)), float32(math.Sin(a))), 4.5, n.col)
	if p != n.st.target {
		ctx.Animate(disclosureStep)
	}
}

// AccordionView is a list of disclosure groups of which at most one is open:
// opening one closes the one that was. It is created by [Accordion].
type AccordionView struct {
	groups      []DisclosureGroupView
	open        int
	onChange    func(int)
	controlled  bool
	lead, trail float32
	hasInsets   bool
	noSep       bool
	w, h        float32
	hasFrame    bool
	key         string
	flex        float32
}

// Accordion returns the groups as a [ListView] in which at most one is open,
// the accordion of the web, which SwiftUI builds from DisclosureGroups and a
// shared isExpanded. A tap on the open group closes it.
//
//	ui.Card(ui.Accordion(
//		ui.DisclosureGroup("Shipping", ui.Text(shipping)),
//		ui.DisclosureGroup("Returns", ui.Text(returns)),
//	)).Padding(0)
//
// Without [AccordionView.Open] it keeps the open group itself and starts with
// none; the [DisclosureGroupView.Expanded] of a group is replaced.
func Accordion(groups ...DisclosureGroupView) AccordionView {
	return AccordionView{groups: groups, open: -1}
}

// Open makes the application own which group is open: the one at index i,
// none for -1. onChange is called with the index a tap asks for.
func (v AccordionView) Open(i int, onChange func(int)) AccordionView {
	v.open, v.onChange, v.controlled = i, onChange, true
	return v
}

// SeparatorInsets sets the insets of the separators between the groups; see
// [ListView.SeparatorInsets].
func (v AccordionView) SeparatorInsets(leading, trailing float32) AccordionView {
	v.lead, v.trail, v.hasInsets = leading, trailing, true
	return v
}

// Separators turns the separators between the groups on or off; see
// [ListView.Separators].
func (v AccordionView) Separators(on bool) AccordionView { v.noSep = !on; return v }

// Frame sets the size of the list; see [ListView.Frame].
func (v AccordionView) Frame(w, h float32) AccordionView { v.w, v.h, v.hasFrame = w, h, true; return v }

// Key sets the reconciliation key.
func (v AccordionView) Key(s string) AccordionView { v.key = s; return v }

// Flex sets the flex factor in a stack.
func (v AccordionView) Flex(f float32) AccordionView { v.flex = checkFlex(f); return v }

// ViewType implements gift.View.
func (AccordionView) ViewType() gift.TypeID { return accordionType }

// Build implements gift.View.
func (v AccordionView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Flex: v.flex, Layouter: anchorLayout{},
		Children: []gift.View{gift.Component("ui.accordion", v.body)}}
}

func (v AccordionView) body(ctx *gift.Context) gift.View {
	own := ctx.State("open", -1)
	open := ctx.Read(own)
	set := own.Set
	if v.controlled {
		open, set = v.open, v.onChange
	}
	items := make([]gift.View, len(v.groups))
	for i, g := range v.groups {
		items[i] = g.Expanded(open == i, func(on bool) {
			if set == nil {
				return
			}
			switch {
			case on:
				set(i)
			case open == i:
				set(-1)
			}
		})
	}
	l := List(items...).Separators(!v.noSep)
	if v.hasInsets {
		l = l.SeparatorInsets(v.lead, v.trail)
	}
	if v.hasFrame {
		l = l.Frame(v.w, v.h)
	}
	return l
}
