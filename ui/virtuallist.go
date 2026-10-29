package ui

import (
	"fmt"
	"math"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/icon"
	"github.com/worldiety/gift/internal/text"
	"github.com/worldiety/gift/render"
)

var (
	virtualListType    = gift.RegisterType("ui.VirtualList")
	virtualListRowType = gift.RegisterType("ui.VirtualListRow")
)

// Metrics of a [VirtualList] row that [RowView] does not already define.
const (
	// vlistCheckSize is the edge of the symbol a selected row shows; see
	// [VirtualListView.SelectedIcon].
	vlistCheckSize = float32(18)
	// vlistPoolSlack is how many rows the pool holds beyond the visible
	// ones, so that a viewport that grows by a row does not rebuild.
	vlistPoolSlack = 4
)

// ListRow is one row of a [VirtualList], as data.
//
// It is data and not a [gift.View] because the rows of a virtual list are
// recycled during layout: a row that scrolls from item 3 to item 40 is the
// same node with a different ListRow, and no view function runs for it. That
// is what makes scrolling free of builds, and it is the reason the row cannot
// be an arbitrary view — a view can only become a node through a build. A row
// therefore looks like a [RowView] with a fixed set of parts: a leading icon,
// a title over an optional subtitle, a trailing value and a chevron.
type ListRow struct {
	// Icon is the leading symbol, drawn in [ColorAccent]. The zero Symbol is
	// no icon, and the labels start at the leading edge.
	Icon Symbol
	// Title is the first line. It is also the row's accessible name, so a
	// test finds the row with gifttest.ByText(title).
	Title string
	// Subtitle is a second, quieter line under the title.
	Subtitle string
	// Value is a trailing label, as [RowView.Value].
	Value string
	// Chevron is the trailing symbol that says "this row leads somewhere".
	Chevron Symbol
	// Selected tints the row and, with [VirtualListView.SelectedIcon], shows
	// a trailing mark. Which rows are selected is the application's model;
	// the list only draws it.
	Selected bool
	// Disabled quietens the labels and refuses taps and activation.
	Disabled bool
}

// ListState is the retained half of a [VirtualList]: the pool of recycled
// rows, the keyboard cursor and the bindings of the last layout.
//
// The application creates it once and holds it — usually with
// [gift.Context.State], which takes a pointer as readily as a number — for the
// reason [Gallery] gives for itself: a view value is rebuilt and thrown away on
// every build, and the pool and the cursor have to survive that.
//
//	list := ctx.State("files", (*ui.ListState)(nil))
//	if list.Get() == nil {
//		list.Set(ui.NewListState())
//	}
//	ui.VirtualList(list.Get(), len(files), func(i int) ui.ListRow { ... })
//
// A ListState belongs to the UI goroutine and to one mounted [VirtualList] at a
// time.
type ListState struct {
	// slots are the bindings of the pool, one per row node, and rows are the
	// retained halves of the row views. Both are bounded by the viewport.
	slots []listSlot
	rows  []*listRowNode
	// want is the pool size the last layout asked for; the next build
	// creates exactly that many row views.
	want int

	// cursor is the item the keyboard is on, or -1. reveal is the item the
	// next layout has to bring into view, or -1.
	cursor int
	reveal int

	// The declaration, refreshed by every build. stale is set by a build and
	// cleared by the layout that follows: a build means the application's
	// data may have changed under unchanged indices, so every visible row is
	// read again.
	count int
	row   func(int) ListRow
	onTap func(int)
	look  listLook
	stale bool

	// first and end are the half open interval of items bound by the last
	// layout. width and density are the inputs the bound rows were shaped
	// for; a change in either reshapes them.
	first, end int
	width      float32
	density    float32

	// focusVisible is the list's own focus ring state, copied during paint
	// so that the row the cursor is on can draw it.
	focusVisible bool

	// invalidate marks the list node for layout; see [ListState.SetCursor].
	invalidate func()

	// owner and ownerPass are the double mount guard, as for [Gallery].
	owner     gift.NodeRef
	ownerPass uint64
	haveOwner bool
}

// listLook is everything a row paints with, resolved once per build.
type listLook struct {
	rowH       float32
	separators bool
	checkSym   Symbol
	font       *text.Font

	label, secondary, disabled, accent Color
	hover, pressed, selected, sep      Color
	ring                               Border
}

// listSlot is the binding of one pool row: the item, its data, and where its
// parts sit inside the row, all computed when it was bound.
type listSlot struct {
	bound bool
	item  int
	data  ListRow

	title, sub, value text.Request
	// Local rectangles inside the row. A zero rectangle is a part the row
	// does not have.
	iconAt, titleAt, subAt, valueAt, checkAt, chevAt geom.Rect
	// Mask sizes of the three symbols in device pixels.
	iconPx, checkPx, chevPx int
}

// NewListState returns the retained half of a new [VirtualList], with no
// cursor.
func NewListState() *ListState { return &ListState{cursor: -1, reveal: -1} }

// Cursor returns the item the keyboard cursor is on, or -1 for none.
func (s *ListState) Cursor() int { return s.cursor }

// SetCursor moves the keyboard cursor to item i and scrolls it into view. A
// negative i removes the cursor. An i past the end is clamped by the next
// layout.
func (s *ListState) SetCursor(i int) {
	s.cursor = max(i, -1)
	if i >= 0 {
		s.Reveal(i)
	}
}

// Reveal scrolls item i into view in the next layout, by the shortest
// distance: an item that is already fully visible does not move.
func (s *ListState) Reveal(i int) {
	s.reveal = i
	if s.invalidate != nil {
		s.invalidate()
	}
}

// VisibleRange returns the half open interval of items the last layout bound
// to rows. It may be wider than the viewport by the part of a row.
func (s *ListState) VisibleRange() (first, end int) { return s.first, s.end }

// PoolSize returns the number of row nodes that exist. It is bounded by the
// viewport and never by the item count, which is the whole claim of the type;
// tests read it to check that claim.
func (s *ListState) PoolSize() int { return len(s.slots) }

// --- the view ----------------------------------------------------------------

// VirtualListView is the declaration half of a virtual list. Create one with
// [VirtualList].
//
// # When to use it instead of a List
//
// A [ListView] builds, lays out and paints every row it has, which is right for
// a settings screen and wrong for a directory with ten thousand files: its
// documentation measures the cliff at about 280 rows on a Raspberry Pi 4. A
// VirtualList has nodes for the visible rows only — about a screenful, plus
// four — whatever the item count, and it is what [Gallery] is for
// pictures: a container that declares [gift.ScrollSpec.Virtual] and binds its
// pool to items during layout.
//
// # What it costs to scroll
//
// Rows have one fixed height, so finding the visible items is a division and
// placing a row is a multiplication; there is no index to search. A row that
// only moves is measured with the constraints it already has and answers from
// gift's layout cache, so a scroll by one row runs the list's own layouter and
// shapes the one row that came into view. No view function runs and
// [gift.Diagnostics.Builds] does not move.
//
// Measured with BenchmarkVirtualListScroll at 400x600, a pool of sixteen rows:
// a whole scrolling frame — wheel notch, update, paint — costs 2.3 µs and
// allocates nothing, identically at a thousand and at a hundred thousand
// items. With the ×10 derate of [ListView] that is about 23 µs on a
// Raspberry Pi 4, a fraction of a percent of the frame. A row whose title is
// shaped for the first time pays the shaper, as any new label does.
//
// # It needs a bounded height
//
// Like [ScrollView] and [GalleryView]: give it a Flex, a Frame or a MaxHeight.
//
// # Rows and data
//
// The rows are read through the function passed to [VirtualList], for the
// visible items only, when they are bound. Every build of the list reads the
// visible ones again, so a change to the application's data reaches the screen
// the way any other state change does: write the state the enclosing component
// reads, and the rebuild rebinds.
//
// The read has to be in the build. The row function runs during layout, where
// nothing subscribes, so a row that computes [ListRow.Selected] from
// [gift.State.Get] inside it does not change when that state does: a tap that
// writes the selection rebuilds nothing on that read's account, and the old
// mark stays. Read the state with [gift.Context.Read] in the build that
// declares the list and let the row function close over the value.
type VirtualListView struct {
	base
	s        *ListState
	count    int
	row      func(int) ListRow
	onTap    func(int)
	rowH     float32
	noSep    bool
	checkSym Symbol
	cfg      gift.ScrollConfig
	bar      ScrollBar
	name     string
}

// VirtualList returns a list of count rows, each produced by row when it comes
// into view, kept in the retained state s.
//
//	ui.VirtualList(list, len(files), func(i int) ui.ListRow {
//		return ui.ListRow{Icon: outline.File, Title: files[i].Name}
//	}).OnTap(open).Flex(1)
//
// row is called during layout, and again for the one item a tap or Enter
// activates, to see whether it is [ListRow.Disabled]. It must not block, and
// must not write state: it is a lookup into data the application already has.
func VirtualList(s *ListState, count int, row func(i int) ListRow) VirtualListView {
	if s == nil {
		panic("gift/ui: VirtualList with a nil ListState; create one with ui.NewListState")
	}
	if count < 0 {
		panic(fmt.Sprintf("gift/ui: VirtualList with a negative count %d", count))
	}
	if row == nil && count > 0 {
		panic("gift/ui: VirtualList with items and a nil row function")
	}
	return VirtualListView{s: s, count: count, row: row}
}

// ViewType implements gift.View.
func (v VirtualListView) ViewType() gift.TypeID { return virtualListType }

// Build implements gift.View.
func (v VirtualListView) Build(*gift.BuildContext) gift.Element {
	s := v.s
	s.count, s.row, s.onTap = v.count, v.row, v.onTap
	s.look = listLook{
		rowH:       v.rowH,
		separators: !v.noSep,
		checkSym:   v.checkSym,
		font:       resolveFont(Font{}),
		label:      ResolveColor(ColorLabel),
		secondary:  ResolveColor(ColorSecondaryLabel),
		disabled:   ResolveColor(Fade(ColorSecondaryLabel, 0.6)),
		accent:     ResolveColor(ColorAccent),
		hover:      ResolveColor(ColorControlHover),
		pressed:    ResolveColor(ColorControlPressed),
		selected:   ResolveColor(Fade(ColorAccent, 0.14)),
		sep:        ResolveColor(ColorSeparator),
		ring:       resolveBorder(defaultButtonFocusRing),
	}
	s.stale = true

	n := &virtualListNode{s: s, fr: v.frame, st: v.style.resolved(), pad: v.pad}
	n.bar.style = v.bar.withDefaults()
	return gift.Element{
		Key:      v.key,
		Flex:     v.flex,
		Label:    v.name,
		Layouter: n,
		Painter:  n,
		Children: s.resizePool(s.want),
		// The list is the keyboard target and not its rows, for the reason
		// [GalleryView.Build] gives: a focusable row would put the focus on a
		// recycled node, and the focus would follow the storage instead of
		// the item.
		Interactor: n,
		Focusable:  true,
		Clip:       true,
		Scroll: &gift.ScrollSpec{
			Axis:    gift.ScrollVertical,
			Config:  v.cfg,
			Virtual: true,
		},
	}
}

// resizePool brings the pool to n rows and returns a fresh children slice for
// it. The slice is fresh because ownership of a children slice passes to gift;
// the nodes behind it are created once per slot and kept, as for
// [Gallery.resizePool].
func (s *ListState) resizePool(n int) []gift.View {
	n = max(n, 0)
	for len(s.rows) < n {
		s.rows = append(s.rows, &listRowNode{s: s, slot: len(s.rows)})
	}
	switch {
	case len(s.slots) < n:
		s.slots = append(s.slots, make([]listSlot, n-len(s.slots))...)
	case len(s.slots) > n:
		clear(s.slots[n:])
		s.slots = s.slots[:n]
	}
	s.want = n
	kids := make([]gift.View, n)
	for i := range n {
		kids[i] = listRowView{n: s.rows[i]}
	}
	return kids
}

// --- modifiers ---------------------------------------------------------------

// OnTap sets the function called with the item of a row that is tapped,
// clicked, or activated with enter or space while the cursor is on it. A
// disabled row does not call it.
func (v VirtualListView) OnTap(fn func(i int)) VirtualListView { v.onTap = fn; return v }

// RowHeight sets the height of every row. The default fits a title over a
// subtitle and is never less than [RowHeight]; a smaller value is raised to
// [RowHeight], because a row is a touch target.
func (v VirtualListView) RowHeight(h float32) VirtualListView {
	if !isFinite(h) || h < 0 {
		panic(fmt.Sprintf("gift/ui: VirtualList.RowHeight(%v) must be a finite, non negative number", h))
	}
	v.rowH = h
	return v
}

// Separators turns the hairlines between rows on, the default, or off.
func (v VirtualListView) Separators(on bool) VirtualListView { v.noSep = !on; return v }

// SelectedIcon sets the trailing mark a [ListRow.Selected] row shows, usually a
// check mark. Room for it is kept on every row, so selecting a row does not
// move its labels. The zero Symbol, the default, marks a selection by the tint
// alone; package ui cannot name a check mark itself, because the icon packages
// import it.
func (v VirtualListView) SelectedIcon(s Symbol) VirtualListView { v.checkSym = s; return v }

// Label sets the accessible name of the list as a whole.
func (v VirtualListView) Label(s string) VirtualListView { v.name = s; return v }

// Config sets the gesture constants of the scroll container.
func (v VirtualListView) Config(c gift.ScrollConfig) VirtualListView { v.cfg = c; return v }

// ScrollBar sets the indicator along the trailing edge; see [ScrollBar].
func (v VirtualListView) ScrollBar(b ScrollBar) VirtualListView { v.bar = b; return v }

// Padding sets the same padding on all four edges. It is inside the viewport,
// so the rows scroll through it, as for [GalleryView.Padding].
func (v VirtualListView) Padding(f float32) VirtualListView { v.setPadding(f); return v }

// PaddingInsets sets the padding per edge.
func (v VirtualListView) PaddingInsets(i geom.Insets) VirtualListView {
	v.setPaddingInsets(i)
	return v
}

// Frame fixes both axes. Not with an unbounded height, which leaves no
// viewport; see [VirtualListView].
func (v VirtualListView) Frame(w, h float32) VirtualListView { v.setFrame(w, h); return v }

// MinWidth raises the minimum width of the viewport.
func (v VirtualListView) MinWidth(f float32) VirtualListView { v.setMinWidth(f); return v }

// MinHeight raises the minimum height of the viewport.
func (v VirtualListView) MinHeight(f float32) VirtualListView { v.setMinHeight(f); return v }

// MaxWidth lowers the maximum width of the viewport.
func (v VirtualListView) MaxWidth(f float32) VirtualListView { v.setMaxWidth(f); return v }

// MaxHeight lowers the maximum height of the viewport.
func (v VirtualListView) MaxHeight(f float32) VirtualListView { v.setMaxHeight(f); return v }

// Background fills the viewport behind the rows. It does not scroll.
func (v VirtualListView) Background(b Background) VirtualListView {
	v.setBackgroundSpec(b)
	return v
}

// Border strokes the inside of the viewport after the rows were drawn.
func (v VirtualListView) Border(b Border) VirtualListView { v.setBorder(b); return v }

// Shadow draws a blurred copy of the viewport box behind it.
func (v VirtualListView) Shadow(s Shadow) VirtualListView { v.setShadow(s); return v }

// CornerRadius rounds the background and the border of the viewport.
func (v VirtualListView) CornerRadius(f float32) VirtualListView { v.setCornerRadius(f); return v }

// Clip is accepted only as Clip(true), which is what a viewport does anyway;
// see [GalleryView.Clip].
func (v VirtualListView) Clip(b bool) VirtualListView {
	if !b {
		panic("gift/ui: Clip(false) on a VirtualList; a viewport that does not clip would paint " +
			"its partially visible rows over its neighbours")
	}
	return v
}

// Key sets the reconciliation key of this view among its siblings.
func (v VirtualListView) Key(s string) VirtualListView { v.setKey(s); return v }

// Flex makes the list take a share of the remaining main axis space of its
// parent stack, which is the usual way to give it a bounded height.
func (v VirtualListView) Flex(f float32) VirtualListView { v.setFlex(f); return v }

// --- the list node -----------------------------------------------------------

// virtualListNode is the retained half of a [VirtualListView]: the layouter
// that binds the pool, the painter of the viewport and the keyboard target.
type virtualListNode struct {
	s   *ListState
	fr  frameSpec
	st  styleSpec
	pad geom.Insets
	bar scrollBarState
}

// rowHeight is the height of every row: the declared one, or one that fits a
// title over a subtitle, and never less than the touch target.
func (s *ListState) rowHeight() float32 {
	if s.look.rowH > 0 {
		return max(s.look.rowH, RowHeight)
	}
	f := s.look.font
	h := 2*rowVPadding + f.Metrics(rowTitleSize).LineHeight + rowTextGap + f.Metrics(rowSubSize).LineHeight
	return max(h, RowHeight)
}

// Layout binds the visible items to the pool and places the rows.
//
// Item i sits in slot i mod k, k being the pool size. The visible items are a
// contiguous run no longer than the pool, so no two of them share a slot, and a
// scroll by one row rebinds exactly one slot: the one whose item left the
// viewport takes the item that entered it. Every other row keeps its binding
// and only moves.
func (n *virtualListNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	s := n.s
	s.checkSingleMount(ctx)
	s.invalidate = ctx.Invalidator()
	cc := n.fr.apply(c)

	// Greedy on both bounded axes, like a gallery; see [GalleryView].
	w, h := cc.Max.W, cc.Max.H
	if !cc.HasBoundedWidth() {
		w = cc.Min.W
	}
	if !cc.HasBoundedHeight() {
		h = cc.Min.H
	}
	size := cc.Constrain(geom.Sz(w, h))
	pad := n.pad
	rowW := max(size.W-pad.Horizontal(), 0)
	rowH := s.rowHeight()
	if rowW != s.width || ctx.Density() != s.density {
		s.width, s.density = rowW, ctx.Density()
		s.stale = true
	}
	if s.cursor >= s.count {
		s.cursor = s.count - 1
	}

	extent := float64(s.count) * float64(rowH)
	if extent > 0 {
		extent += float64(pad.Vertical())
	}
	ctx.ReportScrollContent(extent, 0)
	if i := s.reveal; i >= 0 {
		s.reveal = -1
		if i < s.count {
			top := float64(pad.Top) + float64(i)*float64(rowH)
			off := ctx.ScrollOffset()
			switch {
			case top < off:
				ctx.AnchorScroll(top - float64(pad.Top))
			case top+float64(rowH) > off+float64(size.H):
				ctx.AnchorScroll(top + float64(rowH) + float64(pad.Bottom) - float64(size.H))
			}
		}
	}
	off := ctx.ScrollOffset()
	// Origin and offset equal, so the transform gift pushes is zero and a
	// document of a few million pixels never passes through a float32; the
	// rows are placed at small local coordinates instead. See [Gallery].
	ctx.ReportScrollContent(extent, off)

	first, end := 0, 0
	if s.count > 0 && rowH > 0 {
		top := (off - float64(pad.Top)) / float64(rowH)
		bottom := (off + float64(size.H) - float64(pad.Top)) / float64(rowH)
		first = min(max(int(math.Floor(top)), 0), s.count)
		end = min(max(int(math.Ceil(bottom)), first), s.count)
	}

	// Size the pool for the next build, with the hysteresis of
	// [Gallery.bindAndPlace] so that the two directions cannot oscillate.
	need := end - first
	target := s.want
	if need > len(s.slots) || need*2+8 < len(s.slots) {
		target = need + vlistPoolSlack
	}
	if target != s.want {
		s.want = target
		ctx.RequestBuild()
	}
	k := ctx.ChildCount()
	if need > k {
		// One frame with a pool that is too small, until the build asked for
		// above has run. The rows at the bottom are missing for that frame.
		end = first + k
	}

	for j := range k {
		sl := &s.slots[j]
		i := -1
		if k > 0 {
			i = first + ((j-first%k)+k)%k
		}
		if i < 0 || i >= end {
			if sl.bound {
				*sl = listSlot{}
				ctx.SetChildLabel(j, "")
			}
			ctx.Measure(j, geom.Tight(geom.Size{}))
			ctx.Place(j, geom.Point{})
			continue
		}
		if !sl.bound || sl.item != i || s.stale {
			s.bind(sl, i, rowW, rowH, ctx.Density())
			ctx.SetChildLabel(j, sl.data.Title)
		}
		ctx.Measure(j, geom.Tight(geom.Sz(rowW, rowH)))
		y := float64(pad.Top) + float64(i)*float64(rowH) - off
		ctx.Place(j, geom.Pt(pad.Left, float32(y)))
	}
	s.first, s.end = first, end
	s.stale = false

	// A list cannot overflow: the scroll axis is a document and every label
	// is cut with an ellipsis to the width it has.
	ctx.ReportOverflow(geom.Size{})
	return size
}

// checkSingleMount rejects a second [VirtualListView] over the same
// [ListState], for the reason [Gallery.checkSingleMount] rejects a second view
// of one gallery: two nodes would bind one pool against two viewports.
func (s *ListState) checkSingleMount(ctx *gift.LayoutContext) {
	node, pass := ctx.Node(), ctx.Pass()
	if s.haveOwner && s.ownerPass == pass && s.owner != node {
		panic("gift/ui: two VirtualList views over one ui.ListState; a ListState owns a single " +
			"pool of rows. Create one ui.NewListState per list.")
	}
	s.owner, s.ownerPass, s.haveOwner = node, pass, true
}

// bind reads item i and lays its parts out inside a row of w by h.
//
// The arrangement is the one [RowView] has — icon, labels, flexible gap,
// value, selection mark, chevron — and so is the rule for a row that is too
// narrow: the labels and the value give way with an ellipsis, each guaranteed
// half of what is left; see [shareRow].
func (s *ListState) bind(sl *listSlot, i int, w, h, density float32) {
	d := s.row(i)
	lk := &s.look
	*sl = listSlot{bound: true, item: i, data: d}

	left, right := RowPadding, w-RowPadding
	if !d.Icon.IsZero() {
		sl.iconAt = centredSquare(left, rowIconSize, h)
		sl.iconPx = symbolPixels(rowIconSize, density)
		left += rowIconSize + rowGap
	}
	if !d.Chevron.IsZero() {
		sl.chevAt = centredSquare(right-rowChevron, rowChevron, h)
		sl.chevPx = symbolPixels(rowChevron, density)
		right -= rowChevron + rowGap
	}
	if !lk.checkSym.IsZero() {
		// Kept on every row, selected or not, so that a selection does not
		// shift the labels; see [VirtualListView.SelectedIcon].
		if d.Selected {
			sl.checkAt = centredSquare(right-vlistCheckSize, vlistCheckSize, h)
			sl.checkPx = symbolPixels(vlistCheckSize, density)
		}
		right -= vlistCheckSize + rowGap
	}
	avail := max(right-left, 0)

	line := func(s string, size float32, maxW float32) text.Request {
		return text.Request{Text: s, Font: lk.font, Size: size, MaxWidth: maxW, MaxLines: 1, Truncation: text.TruncateTail}
	}
	natural := func(r text.Request) float32 {
		if r.Text == "" {
			return 0
		}
		r.MaxWidth = geom.Unbounded()
		return text.Default().Layout(r).Size.W
	}
	sl.title = line(d.Title, rowTitleSize, avail)
	sl.sub = line(d.Subtitle, rowSubSize, avail)
	sl.value = line(d.Value, rowValueSize, avail)

	labelsW := max(natural(sl.title), natural(sl.sub))
	valueW := natural(sl.value)
	caps := [2]float32{labelsW, valueW}
	budget := avail
	if valueW > 0 && labelsW > 0 {
		budget -= rowGap
	}
	if labelsW+valueW > budget {
		caps = shareRow([2]float32{labelsW, valueW}, float32(math.Floor(float64(max(budget, 0)))))
	}
	sl.title.MaxWidth, sl.sub.MaxWidth, sl.value.MaxWidth = caps[0], caps[0], caps[1]

	titleH := lineHeight(sl.title)
	block := titleH
	if d.Subtitle != "" {
		block += rowTextGap + lineHeight(sl.sub)
	}
	y := roundf((h - block) / 2)
	sl.titleAt = geom.Rc(left, y, left+caps[0], y+titleH)
	if d.Subtitle != "" {
		sy := y + titleH + rowTextGap
		sl.subAt = geom.Rc(left, sy, left+caps[0], sy+lineHeight(sl.sub))
	}
	if d.Value != "" {
		vw := text.Default().Layout(sl.value).Size.W
		vh := lineHeight(sl.value)
		vy := roundf((h - vh) / 2)
		sl.valueAt = geom.Rc(right-vw, vy, right, vy+vh)
	}
}

// lineHeight is the height of the one line r lays out to, zero for no text.
func lineHeight(r text.Request) float32 {
	if r.Text == "" {
		return 0
	}
	return text.Default().Layout(r).Size.H
}

// centredSquare is the square of edge e at x, centred in a row of height h.
func centredSquare(x, e, h float32) geom.Rect {
	y := roundf((h - e) / 2)
	return geom.Rc(x, y, x+e, y+e)
}

// symbolPixels is the device pixel size of the mask of a symbol drawn at the
// logical edge e, as [iconNode.Layout] computes it.
func symbolPixels(e, density float32) int {
	px := int(math.Round(float64(e * density)))
	return min(max(px, 1), icon.MaxIconExtent)
}

// Paint draws the viewport box, the rows and the indicator over them.
func (n *virtualListNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	n.s.focusVisible = ctx.Interaction().FocusVisible
	paintBackground(ctx, n.st, b)
	ctx.PaintChildren()
	paintBorder(ctx, n.st, b)
	n.bar.paint(ctx)
}

// HandleEvent owns the keyboard and delegates the gesture, as
// [galleryNode.HandleEvent] does.
func (n *virtualListNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if n.bar.handleEvent(ctx, e) {
		return true
	}
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
	case gift.EventKeyDown:
		if n.s.handleKey(ctx, e) {
			return true
		}
	}
	return gift.ScrollInteractor().HandleEvent(ctx, e)
}

// handleKey moves the cursor and activates the row under it.
func (s *ListState) handleKey(ctx *gift.EventContext, e gift.Event) bool {
	if s.count == 0 {
		return false
	}
	cur := s.cursor
	page := max(s.end-s.first-1, 1)
	target := cur
	switch e.Key {
	case gift.KeyUp:
		target = cur - 1
	case gift.KeyDown:
		target = cur + 1
	case gift.KeyPageUp:
		target = cur - page
	case gift.KeyPageDown:
		target = cur + page
	case gift.KeyHome:
		target = 0
	case gift.KeyEnd:
		target = s.count - 1
	case gift.KeyEnter, gift.KeySpace:
		if cur >= 0 {
			s.activate(cur)
		}
		return true
	default:
		return false
	}
	if cur < 0 {
		// No cursor yet: the first key puts it on the first item rather than
		// moving from a position nobody chose.
		target = 0
	}
	s.cursor = min(max(target, 0), s.count-1)
	s.reveal = s.cursor
	ctx.RequestLayout()
	return true
}

// activate calls OnTap for item i unless its row is disabled.
func (s *ListState) activate(i int) {
	if s.onTap == nil || i < 0 || i >= s.count || s.row(i).Disabled {
		return
	}
	s.onTap(i)
}

// --- the row -----------------------------------------------------------------

// listRowView is the declaration of one pool slot. Like [tileView] it carries
// no item: which item a slot stands for is decided in layout. Its label is the
// one the slot is bound to now, because a build resets what
// [gift.LayoutContext.SetChildLabel] wrote.
type listRowView struct{ n *listRowNode }

func (listRowView) ViewType() gift.TypeID { return virtualListRowType }

func (r listRowView) Build(*gift.BuildContext) gift.Element {
	var label string
	if sl := &r.n.s.slots[r.n.slot]; sl.bound {
		label = sl.data.Title
	}
	return gift.Element{
		Layouter:   r.n,
		Painter:    r.n,
		Interactor: r.n,
		Label:      label,
		// No key: a positional pool, for the reason [tileView.Build] gives.
	}
}

// listRowNode is the retained half of one pool slot. It knows its slot and
// reads everything else out of the [ListState] at paint time.
type listRowNode struct {
	s    *ListState
	slot int
}

// Layout takes the size the list gave it; the parts were laid out when the
// slot was bound.
func (r *listRowNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(c.Min)
}

// Paint draws the row: its face, its parts, the cursor ring and the hairline
// under it.
func (r *listRowNode) Paint(ctx *gift.PaintContext) {
	s := r.s
	sl := &s.slots[r.slot]
	if !sl.bound {
		return
	}
	lk := &s.look
	b := ctx.Bounds()
	d := &sl.data
	ia := ctx.Interaction()

	face := ColorClear
	switch {
	case ia.Pressed && !d.Disabled:
		face = lk.pressed
	case ia.Hover && !d.Disabled:
		face = lk.hover
	case d.Selected:
		face = lk.selected
	}
	if !face.IsTransparent() {
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b, Color: face})
	}

	at := func(l geom.Rect) geom.Rect { return l.Translate(b.Min) }
	titleFG, subFG, iconFG := lk.label, lk.secondary, lk.accent
	if d.Disabled {
		titleFG, subFG, iconFG = lk.secondary, lk.disabled, lk.secondary
	}
	if !d.Icon.IsZero() {
		paintListSymbol(ctx, d.Icon, iconFG, sl.iconPx, at(sl.iconAt))
	}
	(&textNode{req: sl.title, fg: titleFG}).paintGlyphs(ctx, at(sl.titleAt))
	if d.Subtitle != "" {
		(&textNode{req: sl.sub, fg: subFG}).paintGlyphs(ctx, at(sl.subAt))
	}
	if d.Value != "" {
		(&textNode{req: sl.value, fg: lk.secondary}).paintGlyphs(ctx, at(sl.valueAt))
	}
	if d.Selected && !lk.checkSym.IsZero() {
		paintListSymbol(ctx, lk.checkSym, lk.accent, sl.checkPx, at(sl.checkAt))
	}
	if !d.Chevron.IsZero() {
		paintListSymbol(ctx, d.Chevron, Fade(lk.secondary, 0.7), sl.chevPx, at(sl.chevAt))
	}

	if s.focusVisible && s.cursor == sl.item && lk.ring.IsVisible() {
		paintBorder(ctx, styleSpec{border: lk.ring}, focusRingRect(b))
	}
	if lk.separators && sl.item < s.count-1 && !lk.sep.IsTransparent() {
		dev := ctx.DeviceBounds()
		top, th := snapHairline(b.Max.Y-1, 1, b.Min.Y, dev.Min.Y, s.density)
		ctx.Add(render.Op{
			Kind: render.OpFillRect,
			// From where the labels start, so that a list of rows with icons
			// has its hairlines under the text and not under the icons.
			Bounds: geom.Rc(b.Min.X+sl.titleAt.Min.X, top, b.Max.X, top+th),
			Color:  lk.sep,
		})
	}
}

// paintListSymbol draws sym tinted fg into dst, as [iconNode.Paint] does.
func paintListSymbol(ctx *gift.PaintContext, sym Symbol, fg Color, px int, dst geom.Rect) {
	if fg.IsTransparent() {
		return
	}
	if id, ok := icons.resolve(ctx.Images(), sym, px); ok {
		ctx.Add(render.Op{Kind: render.OpImage, Bounds: dst, Color: fg, Image: id})
	}
}

// HandleEvent claims the release that is a tap on this row and nothing else,
// so that a drag starting on a row scrolls the list; see [tileNode.HandleEvent].
func (r *listRowNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventPointerUp || !e.Inside || e.Dragged {
		return false
	}
	s := r.s
	sl := &s.slots[r.slot]
	if !sl.bound || sl.data.Disabled {
		return false
	}
	s.cursor = sl.item
	s.activate(sl.item)
	ctx.Repaint()
	return true
}
