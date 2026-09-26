package ui

import (
	"math"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/layout"
)

var rowType = gift.RegisterType("ui.Row")

// Metrics of a list row, in logical pixels. Constants, for the reason the
// metrics of [TabBarView] are.
const (
	// RowHeight is the smallest height a row takes. It is
	// [ControlHitTarget], because a tappable row is a control and the target
	// panel is a touchscreen; a row that is taller than this because its
	// content is taller keeps the larger height.
	RowHeight = ControlHitTarget

	// RowPadding is the horizontal inset of a row's content, and therefore
	// the leading inset a separator has to match to line up with the text.
	RowPadding = float32(16)

	rowVPadding  = float32(8)
	rowGap       = float32(12)
	rowIconSize  = float32(22)
	rowChevron   = float32(16)
	rowTitleSize = float32(15)
	rowSubSize   = float32(12)
	rowValueSize = float32(14)
	rowTextGap   = float32(2)
)

// RowView is the standard row of a [ListView]: a leading icon, a title over an
// optional subtitle, and a trailing accessory. It is created by [Row]; the
// zero value is not useful.
//
//	ui.Row("Wi-Fi").Icon(outline.Globe).Value("kiosk-net").Chevron(outline.AngleRight).OnTap(open)
//	ui.Row("Dark mode").Icon(outline.Moon).Accessory(ui.Toggle(dark, setDark))
//
// # What a row is made of
//
// Leading icon, then the labels, then a flexible gap, then the value label,
// then the accessory, then the chevron — in that order, all of it optional
// except the title. The flexible gap is what pushes the trailing group to the
// edge, and it is also the one thing to know about putting a row somewhere
// other than a [ListView]: a stack measures an inflexible child with an
// unbounded main axis (the overflow model of the project plan, section 7), so
// a row in an [HStack] has no width to spread into and shrink-wraps its
// content. A [ListView] measures its rows with a tight width, which is the
// composition this type is for.
//
// # A row with no title is an accessory row
//
// The flexible gap is only inserted when the row has a title, a subtitle or a
// value, that is when there is something leading that the trailing group has
// to be pushed away from. A row with none of those gives its whole width to
// its accessory:
//
//	ui.Row("").Icon(outline.VolumeUp).Accessory(ui.Slider(v, set).Flex(1))
//
// That is not a convenience, it is the only way the composition works. A
// [Spacer] takes a share of the remainder like any other flexible child, so a
// row that had one *and* a flexible accessory would split the width between
// them and the slider would come out half as wide as the row — which is
// exactly what the settings screen of cmd/example-kitchensink did until this
// rule existed, and it showed up as a segmented control overflowing its own
// row by twelve pixels rather than as anything anybody would notice by
// looking.
//
// # The labels give way, the accessory does not
//
// A title, a subtitle and a value are each one line. When the row is too
// narrow for all of its content, those three are shortened with an ellipsis
// until it fits, and nothing else is: the icon, the accessory and the chevron
// keep their natural widths. A long value — a file path, a network name —
// therefore ends in "…" inside the row instead of running over the edge of the
// card, and so does a title next to an [HStack] of chips.
//
// The width that is left after the fixed parts is shared between the label
// group and the value so that neither is starved by the other: each is
// guaranteed half, and a side that needs less than its half gives the rest to
// the other. A short title next to a long path keeps the whole title and the
// path takes everything else; two long strings are cut to half each.
//
// Two limits, stated rather than hidden. The labels only give way when the row
// has a bounded width, which a [ListView] and a [Card] give it and an
// inflexible child of an [HStack] does not; there the row shrink-wraps its
// content as before. And an accessory that is wider than the row on its own
// still overflows: the labels shrink to a lone ellipsis, and the rest is
// reported, because an accessory is a view the caller built and this type
// cannot shorten it.
//
// What the ellipsis hides is still reported — it appears in
// [gift.Diagnostics.OverflowNodes] against the label, as for any
// [TextView.MaxLines] cut, and under the giftdebug tag it is logged — because
// the project plan, section 7, does not let content that does not fit vanish
// without a number. The row itself no longer overflows. The answer to a label
// that is always cut is still a shorter string or a [RowView.Subtitle].
//
// # Tapping
//
// Without [RowView.OnTap] a row is inert: no interactor, no focus stop, no
// pressed face. With it the row is a [ButtonView] — the same pointer capture,
// the same space and enter activation, the same "a release outside does not
// fire" — whose label happens to be the row's content, and the press is shown
// by the row's own face going to [ColorControlPressed].
//
// # An interactive accessory takes the tap, and the row does not also fire
//
// This is the property that makes a settings list work and it falls out of
// gift rather than out of a special case here: the hit test finds the deepest
// interactive node under the finger, and a [ToggleView] answers true to the
// press and the release, so nothing bubbles up to the row. Tapping the switch
// flips the switch; tapping anywhere else on the same row runs OnTap. Both
// still appear in the focus order, which is correct — a keyboard user needs to
// be able to reach the switch *and* the row's own command — and is the one
// consequence worth knowing about.
//
// # Disabled
//
// [RowView.Disabled] takes the row out of input and draws its title in
// [ColorSecondaryLabel]. The title colour is the row's to change because the
// row builds the label; a [ButtonView] handed an arbitrary label view cannot
// do the same, for the reason its documentation gives. The accessory is *not*
// disabled with it: it is a view the caller passed in, and reaching into it
// would mean this type deciding what "disabled" means for something it has
// never seen. Disable the accessory at the call site as well.
type RowView struct {
	base
	title     string
	subtitle  string
	value     string
	sym       Symbol
	chevron   Symbol
	accessory gift.View
	onTap     func()
	disabled  bool
	name      string
}

// Row returns a row with the given title.
func Row(title string) RowView { return RowView{title: title} }

// ViewType implements gift.View.
func (v RowView) ViewType() gift.TypeID { return rowType }

// Build implements gift.View.
func (v RowView) Build(bc *gift.BuildContext) gift.Element {
	parts, labels, value := v.parts()
	content := HStack(parts...).
		Gap(rowGap).
		Align(geom.Center).
		PaddingInsets(geom.Insets{
			Top:    rowVPadding,
			Right:  RowPadding,
			Bottom: rowVPadding,
			Left:   RowPadding,
		})

	if v.onTap == nil {
		// Inert. No interactor, no focus stop, and — because the content
		// stack has no background, border or clip — no painter either.
		return rowContent{content.MinHeight(RowHeight).Key(v.key).Flex(v.flex), labels, value}.Build(bc)
	}
	return Button(rowContent{content, labels, value}, v.onTap).
		Key(v.key).
		Flex(v.flex).
		// The button's own padding is the row's, applied above, so that the
		// pressed face reaches the edges of the row rather than stopping
		// twelve pixels short of them.
		Padding(0).
		Align(geom.Leading).
		MinHeight(RowHeight).
		// A row draws no face at rest: the list or the card behind it does.
		Background(ColorClear).
		Border(Border{}).
		CornerRadius(0).
		// Hover and press are the states a rebuild cannot deliver, because
		// neither causes one; see [gift.Interaction].
		HoverStyle(ButtonStyle{Background: ColorControlHover}).
		PressedStyle(ButtonStyle{Background: ColorControlPressed}).
		DisabledStyle(ButtonStyle{Background: ColorClear}).
		Disabled(v.disabled).
		Label(v.accessibleName()).
		Build(bc)
}

// accessibleName is the string a person would use to refer to the row's
// command. It is [RowView.Label] when one was given and the title otherwise,
// which is what makes an icon-and-title row need no Label at all.
func (v RowView) accessibleName() string {
	if v.name != "" {
		return v.name
	}
	return v.title
}

// parts builds the children of the row's content stack.
//
// The capacity is exact for the largest row this type can produce, so the
// slice is one allocation whatever the combination of options. That is six and
// not five: leading icon, label group, flexible gap, value, accessory,
// chevron. The example in [RowView]'s own documentation plus an
// [RowView.Accessory] reaches all six, and at a capacity of five that slice
// grew and copied on the last append — a second allocation per row per
// rebuild, which on a two hundred row list is two hundred of them on a frame
// the rest of this package counts single allocations on.
// TestTheLargestRowBuildsItsChildrenInOneAllocation pins it.
//
// It also returns the indices of the two children that give way when the row
// is too narrow — the label group, or the title alone, and the value — or -1
// for one the row does not have; see [node.layoutRow]. They are returned from
// here rather than recomputed by the caller, because this is the one function
// that knows where it put them.
func (v RowView) parts() (out []gift.View, labels, value int8) {
	titleFG := ColorLabel
	subFG := ColorSecondaryLabel
	if v.disabled {
		// The whole label group steps back, not only the title. A quiet
		// title over a normal subtitle reads as an emphasis, not as an
		// absence.
		titleFG, subFG = ColorSecondaryLabel, Fade(ColorSecondaryLabel, 0.6)
	}

	labels, value = -1, -1
	out = make([]gift.View, 0, 6)
	if !v.sym.IsZero() {
		fg := ColorAccent
		if v.disabled {
			fg = ColorSecondaryLabel
		}
		out = append(out, Icon(v.sym).Size(rowIconSize).Foreground(fg).Key("icon"))
	}

	switch {
	case v.title == "" && v.subtitle == "":
		// An accessory row; see [RowView]. No label, and — unless there is a
		// value to push to the trailing edge — no flexible gap either.
	case v.subtitle == "":
		labels = int8(len(out))
		out = append(out, Text(v.title).
			FontSize(rowTitleSize).Foreground(titleFG).MaxLines(1).Key("title"))
	default:
		labels = int8(len(out))
		out = append(out, VStack(
			Text(v.title).FontSize(rowTitleSize).Foreground(titleFG).MaxLines(1).Key("title"),
			Text(v.subtitle).FontSize(rowSubSize).Foreground(subFG).MaxLines(1).Key("subtitle"),
		).Gap(rowTextGap).Align(geom.Leading).Key("labels"))
	}

	// The flexible gap, which is what pushes the trailing group to the edge
	// and what makes the content stack fill the width it was offered.
	//
	// It is omitted for a row with no labels, and that is not a
	// micro-optimisation: a Spacer takes a share of the remainder, so a row
	// that has one *and* a flexible accessory splits the width between them
	// and the accessory comes out half as wide as the row. A slider or a
	// segmented control in a list is exactly that composition. See [RowView].
	if v.title != "" || v.subtitle != "" || v.value != "" {
		out = append(out, Spacer().Key("gap"))
	}

	if v.value != "" {
		value = int8(len(out))
		out = append(out, Text(v.value).
			FontSize(rowValueSize).
			Foreground(ColorSecondaryLabel).
			MaxLines(1).
			Key("value"))
	}
	if v.accessory != nil {
		out = append(out, v.accessory)
	}
	if !v.chevron.IsZero() {
		out = append(out, Icon(v.chevron).
			Size(rowChevron).
			Foreground(Fade(ColorSecondaryLabel, 0.7)).
			Key("chevron"))
	}
	return out, labels, value
}

// rowContent is the content stack of a row: an [HStack] in every respect —
// same view type, so a row that gains or loses an [RowView.OnTap] reconciles
// its content like any other stack — except that its node lays out with
// [node.layoutRow] and knows which of its children may give way.
type rowContent struct {
	Stack
	labels, value int8
}

// Build implements gift.View.
func (r rowContent) Build(bc *gift.BuildContext) gift.Element {
	el := r.Stack.Build(bc)
	n := el.Layouter.(*node)
	n.kind = kindRow
	n.row = rowShrink{idx: [2]int8{r.labels, r.value}}
	return el
}

// rowShrink is the retained state of a row's content node: which children may
// give way, and by how much they do in the pass in progress.
type rowShrink struct {
	// idx are the child indices of the label group and of the value, -1 for
	// one the row does not have.
	idx [2]int8
	// caps are the widths the two are limited to, valid while capped.
	caps   [2]float32
	capped bool
}

// limit returns c narrowed to the cap of child i, or c itself for a child that
// has none or in a pass that caps nothing.
func (r *rowShrink) limit(i int, c geom.Constraints) geom.Constraints {
	if !r.capped {
		return c
	}
	for j, x := range r.idx {
		if int(x) == i {
			c.Max.W = min(c.Max.W, r.caps[j])
			c.Min.W = min(c.Min.W, c.Max.W)
		}
	}
	return c
}

// layoutRow is the stack algorithm, run a second time with the labels
// narrowed when the first run overflowed.
//
// # Why a second pass and not a flexible label
//
// The obvious composition — give the labels a [TextView.Flex] — is wrong both
// ways round. A flexible child is measured with a *tight* share of the
// remainder, so a short title would be stretched to fill the row whether it
// needed the room or not, and a value next to it would have to be flexible too
// and would then get half the row for "On". Worse, an [HStack] with an
// unbounded main axis gives every flexible child exactly zero, and a row in
// such a stack — which [RowView] documents as a composition that shrink-wraps
// — would show a lone ellipsis for its title.
//
// What a row wants is "natural width, unless there is not enough room", which
// the stack algorithm does not have and which rule 1 of the overflow model of
// the project plan, section 7, deliberately keeps out of it: there, the size of
// a child must not depend on its siblings. A row is the one place where it
// must, and it is a closed composition whose children this package built, so
// the exception lives here and not in internal/layout. The first pass is the
// ordinary one; only when it overflows are the natural widths of the labels
// read off it, the excess taken out of them, and the stack run again with
// [rowShrink.limit] narrowing exactly those two children. The fixed parts are
// measured with the same constraints both times and answer from gift's layout
// cache the second time.
//
// A row that fits costs one pass, as before. A row that is cut costs two, and
// only when it is laid out at all: an unchanged row under unchanged
// constraints is not laid out again.
func (n *node) layoutRow(ctx *gift.LayoutContext, cc geom.Constraints, k int) layout.Result {
	for i := range k {
		n.items[i].Flex = ctx.ChildFlex(i)
	}
	r := &n.row
	r.capped = false
	res := layout.Stack(n.spec, cc, k, n, n.items, n.origins)
	if res.Overflow.W <= 0 || !cc.HasBoundedWidth() {
		return res
	}

	var natural [2]float32
	some := false
	for j, i := range r.idx {
		if i >= 0 && int(i) < k {
			natural[j] = n.items[i].Size.W
			some = true
		}
	}
	if !some {
		// An accessory row: nothing that may give way, so the overflow is
		// the accessory's and is reported as it is.
		return res
	}
	// Whole pixels, rounded down: the labels come back at most as wide as
	// their caps, and a budget that was a float rounding error too generous
	// would leave the row overflowing by a millionth of a pixel and counted
	// in gift.Diagnostics for it.
	budget := float32(math.Floor(float64(clampLow(natural[0] + natural[1] - res.Overflow.W))))
	r.caps = shareRow(natural, budget)
	r.capped = true
	return layout.Stack(n.spec, cc, k, n, n.items, n.origins)
}

// shareRow divides budget between the label group and the value of a row
// whose natural widths are natural. Each is guaranteed half; a side whose
// natural width is less than its half keeps its natural width and the other
// side gets the rest. A side the row does not have has a natural width of
// zero and so gives its whole half away.
func shareRow(natural [2]float32, budget float32) [2]float32 {
	half := budget / 2
	switch {
	case natural[0] <= half:
		return [2]float32{natural[0], budget - natural[0]}
	case natural[1] <= half:
		return [2]float32{budget - natural[1], natural[1]}
	default:
		return [2]float32{half, budget - half}
	}
}

// --- modifiers -------------------------------------------------------------

// Subtitle sets a second, quieter line under the title.
func (v RowView) Subtitle(s string) RowView { v.subtitle = s; return v }

// Value sets a trailing label, for the current setting of whatever the row
// names: "kiosk-net", "60 %", "Off".
func (v RowView) Value(s string) RowView { v.value = s; return v }

// Icon sets the leading symbol, drawn in [ColorAccent]. The zero [Symbol] is
// no icon and no space taken, which is what an application that imports no
// icon package gets.
func (v RowView) Icon(s Symbol) RowView { v.sym = s; return v }

// Chevron sets the trailing symbol that says "this row leads somewhere",
// drawn faintly at the very end of the row:
//
//	ui.Row("Network").Chevron(outline.AngleRight).OnTap(push)
//
// It is a parameter rather than a built in glyph for the reason
// [NavigationStackView.BackIcon] is one: package ui cannot import an icon
// package, because the icon packages import it.
func (v RowView) Chevron(s Symbol) RowView { v.chevron = s; return v }

// Accessory sets the trailing view: a [ToggleView], a [BadgeView], anything.
// Ownership of it passes to gift like any other child.
//
// An interactive accessory takes the tap without the row also firing; see
// [RowView].
func (v RowView) Accessory(a gift.View) RowView { v.accessory = a; return v }

// OnTap makes the whole row a control that runs fn when it is activated. A nil
// fn, the default, is an inert row.
func (v RowView) OnTap(fn func()) RowView { v.onTap = fn; return v }

// Disabled takes the row out of input and draws its labels in
// [ColorSecondaryLabel]. It has no effect on a row without an
// [RowView.OnTap] — an inert row is already out of input — beyond the label
// colour, which is the honest way to show "this setting is unavailable" on a
// row that only reports a value.
func (v RowView) Disabled(b bool) RowView { v.disabled = b; return v }

// Label sets the accessible name of the row's command, replacing the title.
func (v RowView) Label(s string) RowView { v.name = s; return v }

// Key sets the reconciliation key of this view among its siblings.
func (v RowView) Key(s string) RowView { v.setKey(s); return v }

// Flex makes the row take a share of the remaining main axis space of its
// parent stack. A row in a [ListView] wants none.
func (v RowView) Flex(f float32) RowView { v.setFlex(f); return v }
