package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/text"
	"github.com/worldiety/gift/render"
)

// The segmented entry fields: [TimeFieldView] and [DateFieldView].
//
// Both are a row of numeric parts with a separator between them, and every
// rule about typing, stepping, pasting and drawing is the same for the two of
// them. This file is that common half, [entryNode]; the two views only say
// how many parts there are, what their ranges are and how the parts become a
// [TimeOfDay] or a [Date].

// SegmentTypingPause is how long a part of a [TimeFieldView] or a
// [DateFieldView] waits for the next digit of a number.
//
// A "1" typed into the hour might be one o'clock or the start of "13", so the
// field shows 01 and keeps the hour. The next digit within this window makes
// 13 of it; a digit after it starts the hour afresh. A digit that cannot start
// a two digit number — a "3" in the hour — needs no window and moves on to
// the minutes at once.
//
// It is a package value and not a modifier for the reason the caret blink
// is; see [CaretBlinkInterval]. A second is what the platform time pickers
// use, within a few hundred milliseconds either way.
const SegmentTypingPause = time.Second

// segmentInset is the room on each side of a part, inside the highlight that
// marks the active one, in logical pixels.
const segmentInset = float32(3)

// Default appearance of an entry field. It is the face of a [TextFieldView]
// on purpose, so that a time next to a text in a form is the same kind of
// box; the one addition is the highlight of the active part, which is the
// accent and the colour that goes on it, like a selected segment of a
// [SegmentedControl].
var (
	defaultSegmentBackground = ColorAccent
	defaultSegmentForeground = ColorOnAccent
	defaultInvalidBorder     = Border{Width: 1, Color: ColorDanger}
)

// entryClock is where an empty part gets its first value from when it is
// stepped: the arrows on an empty hour start at the current hour. A variable
// so that a test can fix the day.
var entryClock = time.Now

type entryKind uint8

const (
	entryTime entryKind = iota
	entryDate
)

// entryNode is the retained half of a [TimeFieldView] or a [DateFieldView]:
// layouter, painter and interactor in one object, allocated once per build.
//
// The parts and the part that has the keyboard are not here. They are in
// [gift.EntryState], because this object is replaced by every rebuild and
// every digit typed is one; see there.
//
// # Whose value it is
//
// A field built with a value — [TimeFieldView.Value] or a binding — shows the
// application's value. What the user types is written into the draft and
// handed to the application at once, and the draft stays on screen for as
// long as it stands for the value the application holds. A draft the
// application did not adopt is replaced by the application's value at the next
// opportunity, which is how a form reset or a loaded record reaches the field.
// A draft with an empty part stands for no value, so a half typed time
// survives an application that keeps the zero TimeOfDay meanwhile.
//
// A field built with neither keeps its own draft and only reports it, like a
// [TextFieldView] over an editor nobody binds.
type entryNode struct {
	kind  entryKind
	parts int
	width [3]int8
	sep   string
	ph    [3]string

	controlled bool
	value      [3]int32
	valueSet   bool

	lo, hi       int64
	hasLo, hasHi bool
	invalid      bool
	step         int32

	emit   func(vals [3]int32, ok bool)
	submit func(vals [3]int32, ok bool)

	disabled bool

	fr  frameSpec
	st  styleSpec
	pad geom.Insets
	req text.Request

	fg, placeholderFG, activeBG, activeFG Color
	disabledFG, disabledBG                Color
	focusRing, invalidBorder              Border

	// The geometry of the last layout, in local space: the width of every
	// part's slot, the width of the separator, and the line band the text
	// stands in.
	slot           [3]float32
	sepW           float32
	lineTop, lineH float32
	baseline       float32
}

func bit(i int) uint8 { return 1 << i }

func (n *entryNode) allFilled() uint8 { return 1<<n.parts - 1 }

// limits is the range of part i, given the other parts. Only the day of a
// date depends on them, and only once the month is known.
func (n *entryNode) limits(i int, es gift.EntryState) (lo, hi int32) {
	if n.kind == entryTime {
		if i == 0 {
			return 0, 23
		}
		return 0, 59
	}
	switch i {
	case 0:
		if es.Filled&bit(1) != 0 && es.Values[1] >= 1 && es.Values[1] <= 12 {
			var y int32
			if es.Filled&bit(2) != 0 {
				y = es.Values[2]
			}
			return 1, daysIn(es.Values[1], y)
		}
		return 1, 31
	case 1:
		return 1, 12
	}
	return 1, 9999
}

// draftValue is the value the draft stands for: every part filled and every
// part within its range, or no value.
//
// A year that is still being typed is no value yet. "2", "20" and "202" are
// the way to 2026 and not three years anybody means, and a year left at "26"
// is 2026 once [entryNode.finish] has seen it; reporting the years on the way
// would hand the application the year 2 in between.
func (n *entryNode) draftValue(es gift.EntryState) ([3]int32, bool) {
	if es.Filled&n.allFilled() != n.allFilled() {
		return [3]int32{}, false
	}
	if n.kind == entryDate && es.Part == 2 && es.Digits > 0 && es.Digits < n.width[2] {
		return [3]int32{}, false
	}
	for i := range n.parts {
		lo, hi := n.limits(i, es)
		if v := es.Values[i]; v < lo || v > hi {
			return [3]int32{}, false
		}
	}
	return es.Values, true
}

// order maps a value onto a number that compares like the value does, for
// [TimeFieldView.Range] and [DateFieldView.Range].
func (n *entryNode) order(v [3]int32) int64 {
	if n.kind == entryTime {
		return int64(v[0])*60 + int64(v[1])
	}
	return int64(v[2])*10000 + int64(v[1])*100 + int64(v[0])
}

// effective is the draft as it is to be shown and edited: the retained one,
// or the application's value where the two disagree. It is a pure function so
// that the painter, which may not write the control state, draws the same
// thing the next event will start from.
func (n *entryNode) effective(es gift.EntryState) gift.EntryState {
	if !n.controlled {
		return es
	}
	v, ok := n.draftValue(es)
	if ok == n.valueSet && (!ok || v == n.value) {
		return es
	}
	es.Values, es.Digits = n.value, 0
	es.Filled = 0
	if n.valueSet {
		es.Filled = n.allFilled()
	}
	return es
}

// isInvalid reports whether the danger border is drawn: the application said
// so, the value is outside the range, or every part is filled and they do not
// make a value — the 31st of a month that has thirty days.
func (n *entryNode) isInvalid(es gift.EntryState) bool {
	if n.invalid {
		return true
	}
	v, ok := n.draftValue(es)
	if !ok {
		return es.Filled&n.allFilled() == n.allFilled() && es.Digits == 0
	}
	k := n.order(v)
	return n.hasLo && k < n.lo || n.hasHi && k > n.hi
}

// partText is what part i shows: its digits, padded to the width of the part,
// or its placeholder.
func (n *entryNode) partText(i int, es gift.EntryState) string {
	if es.Filled&bit(i) == 0 {
		return n.ph[i]
	}
	s := strconv.Itoa(int(es.Values[i]))
	if int(es.Part) == i && es.Digits > 0 && n.width[i] > 2 {
		// A year being typed shows what was typed, "20" and not "0020".
		return s
	}
	for len(s) < int(n.width[i]) {
		s = "0" + s
	}
	return s
}

// display is the whole field as it shows it, which is also what it reports as
// its value to automation; see [gift.ValueReporter].
func (n *entryNode) display(es gift.EntryState) string {
	var b strings.Builder
	for i := range n.parts {
		if i > 0 {
			b.WriteString(n.sep)
		}
		b.WriteString(n.partText(i, es))
	}
	return b.String()
}

// AccessibleValue implements [gift.ValueReporter]. An empty field reports "",
// and a field with some parts filled reports what it shows, placeholders and
// all.
func (n *entryNode) AccessibleValue(st gift.ControlState) string {
	es := n.effective(st.Entry)
	if es.Filled == 0 {
		return ""
	}
	return n.display(es)
}

// --- editing -------------------------------------------------------------------

// finish ends the typing in the active part: a number left short is brought
// into its range — a lone "0" in a month is January — and a year typed with
// two digits is put in this century. It then keeps the day within the month.
func (n *entryNode) finish(es gift.EntryState) gift.EntryState {
	i := int(es.Part)
	if es.Filled&bit(i) != 0 && es.Digits > 0 {
		if n.kind == entryDate && i == 2 && es.Digits <= 2 {
			es.Values[2] += 2000
		}
		lo, hi := n.limits(i, es)
		es.Values[i] = min(max(es.Values[i], lo), hi)
	}
	es.Digits = 0
	return n.clampDay(es)
}

// clampDay keeps the day of a date within its month once the month is known:
// the 31st of January becomes the 28th when the month is stepped to February.
func (n *entryNode) clampDay(es gift.EntryState) gift.EntryState {
	if n.kind != entryDate || es.Filled&bit(0) == 0 || es.Digits > 0 && es.Part == 0 {
		return es
	}
	if _, hi := n.limits(0, es); es.Values[0] > hi {
		es.Values[0] = hi
	}
	return es
}

// typeDigit is a digit typed into the active part; see [SegmentTypingPause]
// for the rule it follows.
func (n *entryNode) typeDigit(es gift.EntryState, d int32, now time.Duration) gift.EntryState {
	i := int(es.Part)
	_, hi := n.limits(i, es)
	if es.Digits > 0 && es.Filled&bit(i) != 0 && now-es.TypedAt < SegmentTypingPause {
		es.Values[i] = min(es.Values[i]*10+d, hi)
		es.Digits++
	} else {
		es.Values[i] = d
		es.Digits = 1
	}
	es.Filled |= bit(i)
	es.TypedAt = now
	if es.Digits >= n.width[i] || es.Values[i]*10 > hi {
		es = n.finish(es)
		if i < n.parts-1 {
			es.Part++
			es.Advanced = true
		}
	}
	return n.clampDay(es)
}

// stepBy moves part i by k steps up (positive) or down, wrapping around at the
// ends of its range. The minutes of a time move in steps of
// [TimeFieldView.Step], landing on a multiple of it first, and carry into the
// hour: 17:57 up by five minutes is 18:00, and the day wraps around at
// midnight. The day of a complete date carries into the month and the year the
// same way. A year does not wrap, because there is nothing after 9999 that
// anybody means. An empty part starts at today or at now; see [entryClock].
func (n *entryNode) stepBy(es gift.EntryState, i, k int) gift.EntryState {
	es = n.finish(es)
	lo, hi := n.limits(i, es)
	step := int32(1)
	if n.kind == entryTime && i == 1 && n.step > 1 {
		step = n.step
	}
	if es.Filled&bit(i) == 0 {
		es.Values[i] = n.initial(i, step)
		es.Filled |= bit(i)
		return n.clampDay(es)
	}
	if i == 0 && n.kind == entryDate && es.Filled == n.allFilled() {
		return n.stepDays(es, k)
	}
	carry := i == 1 && n.kind == entryTime && es.Filled&bit(0) != 0
	v := es.Values[i]
	for ; k > 0; k-- {
		v = (v/step + 1) * step
		if v > hi {
			if n.kind == entryDate && i == 2 {
				v = hi
			} else {
				v = lo
				if carry {
					es.Values[0] = (es.Values[0] + 1) % 24
				}
			}
		}
	}
	for ; k < 0; k++ {
		v = ((v+step-1)/step - 1) * step
		if v < lo {
			if n.kind == entryDate && i == 2 {
				v = lo
			} else {
				v = hi / step * step
				if carry {
					es.Values[0] = (es.Values[0] + 23) % 24
				}
			}
		}
	}
	es.Values[i] = v
	return n.clampDay(es)
}

// stepDays moves a complete date by k days, across the ends of the months and
// the years. It stops at the first and the last day of the years a date can
// have.
func (n *entryNode) stepDays(es gift.EntryState, k int) gift.EntryState {
	d, m, y := es.Values[0], es.Values[1], es.Values[2]
	for ; k > 0; k-- {
		switch {
		case d < daysIn(m, y):
			d++
		case m < 12:
			d, m = 1, m+1
		case y < 9999:
			d, m, y = 1, 1, y+1
		}
	}
	for ; k < 0; k++ {
		switch {
		case d > 1:
			d--
		case m > 1:
			m--
			d = daysIn(m, y)
		case y > 1:
			d, m, y = 31, 12, y-1
		}
	}
	es.Values[0], es.Values[1], es.Values[2] = d, m, y
	return es
}

// initial is the first value of an empty part that is stepped.
func (n *entryNode) initial(i int, step int32) int32 {
	now := entryClock()
	if n.kind == entryTime {
		if i == 0 {
			return int32(now.Hour())
		}
		return int32(now.Minute()) / step * step
	}
	switch i {
	case 0:
		return int32(now.Day())
	case 1:
		return int32(now.Month())
	}
	return int32(now.Year())
}

// paste replaces every part with what the clipboard holds, if it holds a value
// of this field's kind.
func (n *entryNode) paste(es gift.EntryState, s string) (gift.EntryState, bool) {
	var vals [3]int32
	var ok bool
	if n.kind == entryTime {
		vals, ok, _ = parseClock(s)
	} else {
		vals, ok, _ = parseCalendar(s)
	}
	if !ok {
		return es, false
	}
	es.Values, es.Filled, es.Digits = vals, n.allFilled(), 0
	return es, true
}

// isSeparator reports whether r moves on to the next part. Both fields take
// the separators of both, so that a date typed with slashes or a time with a
// dot works without the user knowing which one the field shows.
func (n *entryNode) isSeparator(r rune) bool {
	if n.kind == entryTime {
		return r == ':' || r == '.' || r == ',' || r == 'h'
	}
	return r == '.' || r == '/' || r == '-' || r == ','
}

// --- layout --------------------------------------------------------------------

// measure is the width of s in the field's font.
func (n *entryNode) measure(s string) float32 {
	r := n.req
	r.Text = s
	return text.Default().Measure(r).W
}

// Layout sizes the field to its content: every part as wide as its widest
// digits or its placeholder, whichever is wider, so that the field does not
// change its width while it is typed into. It is one line of the font tall,
// exactly as tall as a [TextFieldView] in the same font, so the two line up in
// a form row.
//
// Unlike a text field it is not greedy on a bounded axis: a time is five
// characters wide whatever the row offers, and a field stretched across a form
// would put them at its left end with nothing after them. A caller who wants
// it wider writes Frame, MinWidth or Flex; the parts stay at the left, like
// the text of a field does.
func (n *entryNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	cc := n.fr.apply(c)
	r := n.req
	r.Text = "0"
	p := text.Default().Layout(r)

	var digit float32
	for d := range 10 {
		digit = max(digit, n.measure(string(rune('0'+d))))
	}
	w := n.pad.Horizontal()
	for i := range n.parts {
		n.slot[i] = max(digit*float32(n.width[i]), n.measure(n.ph[i])) + 2*segmentInset
		w += n.slot[i]
	}
	n.sepW = n.measure(n.sep)
	w += n.sepW * float32(n.parts-1)

	size := cc.Constrain(geom.Sz(roundf(w+0.49), p.Size.H+n.pad.Vertical()))
	n.lineH = p.Size.H
	inner := size.H - n.pad.Vertical()
	n.lineTop = n.pad.Top + clampLow(roundf((inner-n.lineH)/2))
	n.baseline = p.Metrics.FirstBaseline
	ctx.ReportOverflow(geom.Size{})
	ctx.ReportBaseline(n.lineTop + n.baseline)
	return size
}

// partAt is the part under the device space x: the one whose slot, extended
// to the middle of the separators on either side, contains it.
func (n *entryNode) partAt(ctx *gift.EventContext, x float32) int8 {
	x -= ctx.DeviceBounds().Min.X + n.pad.Left
	for i := range n.parts - 1 {
		x -= n.slot[i]
		if x < n.sepW/2 {
			return int8(i)
		}
		x -= n.sepW
	}
	return int8(n.parts - 1)
}

// --- paint ---------------------------------------------------------------------

// Paint draws the field in the order of [textFieldNode.Paint]: background,
// content, border, focus ring. An invalid field draws its border in
// [ColorDanger], and its focus ring too, because a ring in the accent over a
// red hairline hides the one thing the user has to see.
func (n *entryNode) Paint(ctx *gift.PaintContext) {
	assertResolvedBorder(n.focusRing, "the focus ring of an entry field")
	assertResolvedBorder(n.invalidBorder, "the invalid border of an entry field")

	ia := ctx.Interaction()
	es := n.effective(ctx.ControlState().Entry)
	invalid := !ia.Disabled && n.isInvalid(es)
	st := n.st
	if ia.Disabled {
		st.background = n.disabledBG
	}
	if invalid {
		st.border = n.invalidBorder
	}
	b := ctx.Bounds()
	paintBackground(ctx, st, b)

	ctx.PushClip(ctx.DeviceBounds())
	n.paintParts(ctx, b, ia, es)
	ctx.PopClip()

	paintBorder(ctx, st, b)
	if ia.FocusVisible && !ia.Disabled {
		ring := n.focusRing
		if invalid && ring.IsVisible() {
			ring.Color = n.invalidBorder.Color
		}
		if ring.IsVisible() {
			paintBorder(ctx, styleSpec{border: ring, radius: focusRingRadius(st.radius)}, focusRingRect(b))
		}
	}
}

// paintParts draws the parts, the separators and the highlight behind the
// active part. The assertions come first for the reason
// [textFieldNode.paintContent] gives.
func (n *entryNode) paintParts(ctx *gift.PaintContext, b geom.Rect, ia gift.Interaction, es gift.EntryState) {
	assertResolved(n.fg, "the foreground of an entry field")
	assertResolved(n.placeholderFG, "the placeholder colour of an entry field")
	assertResolved(n.activeBG, "the active part of an entry field")
	assertResolved(n.activeFG, "the text of the active part of an entry field")
	assertResolved(n.disabledFG, "the foreground of a disabled entry field")

	fg := n.fg
	if ia.Disabled {
		fg = n.disabledFG
	}
	sepFG := n.placeholderFG
	if es.Filled != 0 {
		sepFG = fg
	}
	top := b.Min.Y + n.lineTop
	baseline := roundf(top + n.baseline)
	bottom := min(top+n.lineH, b.Max.Y-n.pad.Bottom)
	x := b.Min.X + n.pad.Left
	active := ia.Focused && !ia.Disabled
	for i := range n.parts {
		if i > 0 {
			n.paintText(ctx, b, n.sep, sepFG, x, baseline)
			x += n.sepW
		}
		c := fg
		if es.Filled&bit(i) == 0 && !ia.Disabled {
			c = n.placeholderFG
		}
		if active && int(es.Part) == i {
			if !n.activeBG.IsTransparent() {
				ctx.Add(render.Op{
					Kind:         render.OpFillRoundRect,
					Bounds:       geom.Rc(roundf(x), top, roundf(x+n.slot[i]), bottom),
					Color:        n.activeBG,
					CornerRadius: 4,
				})
			}
			c = n.activeFG
		}
		s := n.partText(i, es)
		tx := roundf(x + (n.slot[i]-n.measure(s))/2)
		n.paintText(ctx, b, s, c, tx, baseline)
		x += n.slot[i]
	}
}

func (n *entryNode) paintText(ctx *gift.PaintContext, b geom.Rect, s string, fg Color, x, baseline float32) {
	if fg.IsTransparent() {
		return
	}
	r := n.req
	r.Text = s
	paintShapedLine(ctx, b, r, fg, x, baseline)
}

// --- input ---------------------------------------------------------------------

// HandleEvent implements gift.Interactor.
//
// Every event it acts on works on the effective draft and writes it back
// whole. A change of the value the draft stands for is reported once, after
// the draft was written, through the view's callbacks; see [entryNode.report].
//
// The wheel steps the part under the pointer, but only while the field has
// the focus. A form is scrolled with the same wheel, and a field that changed
// its value whenever the pointer happened to pass over it on the way down the
// page would change values nobody looked at. The first click focuses the
// field, and from then on the wheel is the field's.
func (n *entryNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if n.disabled && e.Kind != gift.EventFocusLost {
		return false
	}
	st := ctx.ControlState()
	before := n.effective(st.Entry)
	es := before
	submit := false
	// Only the event right after an advance may see it; see
	// [gift.EntryState.Advanced].
	advanced := es.Advanced
	es.Advanced = false

	switch e.Kind {
	case gift.EventFocusGained:
		es.Part, es.Digits = 0, 0
		ctx.ScrollIntoView()
		if softKeyboardWanted(ctx) {
			ctx.RequestSoftKeyboard(true)
		}

	case gift.EventFocusLost:
		ctx.RequestSoftKeyboard(false)
		es = n.finish(es)

	case gift.EventPointerDown:
		ctx.RequestFocus()
		es = n.finish(es)
		es.Part = n.partAt(ctx, e.Pos.X)

	case gift.EventPointerUp, gift.EventPointerCancel:
		return true

	case gift.EventWheel:
		if !ctx.Interaction().Focused {
			return false
		}
		es.Part = n.partAt(ctx, e.Pos.X)
		es.Wheel += e.Delta.Y
		k := int(es.Wheel)
		es.Wheel -= float32(k)
		if k != 0 {
			es = n.stepBy(es, int(es.Part), k)
		}

	case gift.EventKeyDown:
		var ok bool
		es, ok, submit = n.key(es, e)
		if !ok {
			return false
		}

	case gift.EventRune:
		if e.Mods.Has(gift.ShortcutModifier) {
			return false
		}
		switch {
		case e.Rune >= '0' && e.Rune <= '9':
			es = n.typeDigit(es, e.Rune-'0', e.Time)
		case n.isSeparator(e.Rune):
			// "9:30" types the 9, which moves on to the minutes by itself,
			// and then the colon, which must not move on a second time.
			if !advanced {
				es = n.finish(es)
				if int(es.Part) < n.parts-1 {
					es.Part++
				}
			}
		default:
			// Swallowed rather than bubbled: a letter typed into a time
			// field is a typing mistake, and an ancestor that took it as a
			// shortcut would turn the mistake into an action.
		}

	default:
		return false
	}

	st.Entry = es
	ctx.SetControlState(st)
	n.report(before, es)
	if submit && n.submit != nil {
		v, ok := n.draftValue(es)
		n.submit(v, ok)
	}
	return true
}

// key is the keyboard half. It reports whether it handled the key and whether
// the key was enter. Tab is not handled, which is what moves the focus on.
func (n *entryNode) key(es gift.EntryState, e gift.Event) (gift.EntryState, bool, bool) {
	shortcut := e.Mods.Has(gift.ShortcutModifier)
	last := int8(n.parts - 1)
	switch e.Key {
	case gift.KeyLeft:
		es = n.finish(es)
		es.Part = max(es.Part-1, 0)
	case gift.KeyRight:
		es = n.finish(es)
		es.Part = min(es.Part+1, last)
	case gift.KeyHome:
		es = n.finish(es)
		es.Part = 0
	case gift.KeyEnd:
		es = n.finish(es)
		es.Part = last
	case gift.KeyUp, gift.KeyDown:
		k := 1
		if e.Key == gift.KeyDown {
			k = -1
		}
		i := int(es.Part)
		// Shift steps the hours from wherever the keyboard is, so that a
		// time is moved by whole hours without leaving the minutes.
		if e.Mods.Has(gift.ModShift) && n.kind == entryTime {
			i = 0
		}
		es = n.stepBy(es, i, k)
	case gift.KeyBackspace, gift.KeyDelete:
		i := int(es.Part)
		// Backspace in a part that is already empty goes back into the one
		// before and clears that, so holding it empties the field from the
		// right the way it empties a text.
		if e.Key == gift.KeyBackspace && es.Filled&bit(i) == 0 && i > 0 {
			i--
			es.Part--
		}
		es.Filled &^= bit(i)
		es.Digits = 0
	case gift.KeyEnter:
		if e.Repeat {
			return es, true, false
		}
		return n.finish(es), true, true
	case gift.KeyC, gift.KeyX:
		if !shortcut {
			return es, false, false
		}
		if v, ok := n.draftValue(es); ok {
			CurrentClipboard().SetText(n.display(gift.EntryState{Values: v, Filled: n.allFilled()}))
			if e.Key == gift.KeyX {
				es.Filled, es.Digits, es.Part = 0, 0, 0
			}
		}
	case gift.KeyV:
		if !shortcut {
			return es, false, false
		}
		if s, ok := CurrentClipboard().Text(); ok {
			es, _ = n.paste(es, s)
		}
	default:
		return es, false, false
	}
	return es, true, false
}

// report hands a changed value to the view's callbacks: once per change of
// the value the draft stands for, and not once per keystroke. Typing the first
// digit into an empty field changes the draft and not the value, which is
// still none.
func (n *entryNode) report(before, after gift.EntryState) {
	v0, ok0 := n.draftValue(before)
	v1, ok1 := n.draftValue(after)
	if ok0 == ok1 && v0 == v1 || n.emit == nil {
		return
	}
	n.emit(v1, ok1)
}

// --- parsing -------------------------------------------------------------------

// parseClock reads a time; see [ParseTimeOfDay] for what it accepts. empty
// reports a string with nothing in it.
func parseClock(s string) (vals [3]int32, ok, empty bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return vals, false, true
	}
	var h, m string
	if i := strings.IndexAny(s, ":.,h"); i >= 0 {
		h, m = s[:i], s[i+1:]
		if len(m) != 2 {
			return vals, false, false
		}
	} else {
		switch len(s) {
		case 1, 2:
			h, m = s, "0"
		case 3:
			h, m = s[:1], s[1:]
		case 4:
			h, m = s[:2], s[2:]
		default:
			return vals, false, false
		}
	}
	hv, ok1 := digits(h, 2)
	mv, ok2 := digits(m, 2)
	if !ok1 || !ok2 || hv > 23 || mv > 59 {
		return vals, false, false
	}
	return [3]int32{hv, mv}, true, false
}

// parseCalendar reads a date into the parts of a [DateFieldView], day first;
// see [ParseDate] for what it accepts.
func parseCalendar(s string) (vals [3]int32, ok, empty bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return vals, false, true
	}
	var d, m, y string
	if f := strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == '/' || r == '-' }); len(f) == 3 {
		d, m, y = f[0], f[1], f[2]
		if len(d) == 4 {
			d, y = y, d // ISO 8601: year first
		}
	} else if len(f) == 1 && (len(s) == 6 || len(s) == 8) {
		d, m, y = s[:2], s[2:4], s[4:]
	} else {
		return vals, false, false
	}
	dv, ok1 := digits(d, 2)
	mv, ok2 := digits(m, 2)
	yv, ok3 := digits(y, 4)
	if !ok1 || !ok2 || !ok3 || len(y) != 2 && len(y) != 4 {
		return vals, false, false
	}
	if len(y) == 2 {
		yv += 2000
	}
	if mv < 1 || mv > 12 || yv < 1 || dv < 1 || dv > daysIn(mv, yv) {
		return vals, false, false
	}
	return [3]int32{dv, mv, yv}, true, false
}

// digits parses one to max ASCII digits.
func digits(s string, max int) (int32, bool) {
	if s == "" || len(s) > max {
		return 0, false
	}
	var v int32
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		v = v*10 + r - '0'
	}
	return v, true
}
