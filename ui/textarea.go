package ui

import (
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/text"
	"github.com/worldiety/gift/render"
)

var textAreaType = gift.RegisterType("ui.TextArea")

// defaultAreaWidth is how wide an area makes itself when nothing bounds it,
// for the reason [defaultFieldWidth] gives: an editor must not grow sideways
// under the typist.
const defaultAreaWidth = float32(280)

// TextAreaView edits text of several lines. It is created by [TextArea]; the
// zero value is not useful.
//
//	notes := ui.Editor(ctx, "notes", "")
//	ui.TextArea(notes).Placeholder("Notes").Lines(3, 8)
//
// It is the multi line sibling of [TextFieldView], over the same
// [TextEditor], and the counterpart of SwiftUI's TextEditor, whose name the
// model already has here. Lines wrap at the width of the area and enter
// starts a new one. The area grows with its text from the smallest number of
// lines of [TextAreaView.Lines] to the largest and scrolls its lines past
// that, under the wheel, under a finger and to keep the caret in view.
//
// Everything a field does with a single line it does with each line: click,
// drag and shift-click select, a double click selects a word and a triple
// click the paragraph, which is the text between two line breaks. Up and down
// move to the line above and below and keep to the column they started in
// across shorter lines; home and end go to the ends of the line on the
// screen, and with [gift.ShortcutModifier] to the ends of the text; page up
// and page down move by the lines in view. Tab is not a character here
// either: it moves the focus on, as in every form.
//
// # Drags
//
// A drag with a mouse selects, in whatever direction it goes. A drag with a
// finger that leaves its press more vertically than sideways scrolls instead:
// the area's own lines when there are more than fit, the form around it
// otherwise. That is the rule of [TextFieldView] with one exception, the
// mouse, which the field could not make because a touch panel arrived as a
// mouse; the backend now reads the panel as touches.
//
// # What it is not
//
// It has no undo, for the reason [TextFieldView] gives, and no rich text. A
// word longer than the width is not broken: it runs past the right edge and
// is cut there, as a long word is everywhere in gift's text.
type TextAreaView struct {
	base
	ed *TextEditor

	placeholder string
	name        string

	onChange func(string)
	onSubmit func(string)
	bind     gift.Binding[string]

	fg, placeholderFG, selection, caret Color
	hasFG, hasPlaceholderFG             bool
	hasSelection, hasCaret              bool

	font Font
	size float32

	focusRing          Border
	hasFocusRing       bool
	disabled           bool
	hasPadding         bool
	minLines, maxLines int
}

// TextArea returns an area editing ed, three lines tall at least and growing
// without limit; see [TextAreaView.Lines]. A nil editor panics, as in
// [TextField].
func TextArea(ed *TextEditor) TextAreaView {
	if ed == nil {
		panic("gift/ui: TextArea with a nil *TextEditor; create one with ui.NewTextEditor " +
			"or, inside a component, with ui.Editor(ctx, name, initial)")
	}
	return TextAreaView{ed: ed, minLines: 3}
}

// ViewType implements gift.View.
func (v TextAreaView) ViewType() gift.TypeID { return textAreaType }

// Build implements gift.View.
func (v TextAreaView) Build(*gift.BuildContext) gift.Element {
	size := v.size
	if size <= 0 {
		size = DefaultFontSize
	}
	// The binding is the outer truth, as in [TextFieldView.Build].
	if !v.bind.IsZero() {
		if s := v.bind.Get(); s != v.ed.Text() {
			v.ed.SetText(s)
		}
	}

	st := v.style.resolved()
	if !v.style.isSet(bitBackground) {
		st.background = ResolveColor(defaultFieldBackground)
	}
	if !v.style.isSet(bitBorder) {
		st.border = resolveBorder(defaultFieldBorder)
	}
	if !v.style.isSet(bitRadius) {
		st.radius = defaultFieldRadius
	}

	font := resolveFont(v.font)
	n := &textAreaNode{
		ed:          v.ed,
		fr:          v.frame,
		st:          st,
		pad:         geom.InsetsAll(8),
		placeholder: v.placeholder,
		onChange:    v.onChange,
		onSubmit:    v.onSubmit,
		bind:        v.bind,
		disabled:    v.disabled,
		minLines:    v.minLines,
		maxLines:    v.maxLines,
		req:         text.Request{Font: font, Size: size, MaxWidth: geom.Unbounded()},
		phReq:       text.Request{Text: v.placeholder, Font: font, Size: size, MaxWidth: geom.Unbounded()},
	}
	if v.hasPadding {
		n.pad = v.pad
	}
	n.fg = ResolveColor(ColorLabel)
	if v.hasFG {
		n.fg = ResolveColor(v.fg)
	}
	n.placeholderFG = ResolveColor(ColorSecondaryLabel)
	if v.hasPlaceholderFG {
		n.placeholderFG = ResolveColor(v.placeholderFG)
	}
	n.selectionBG = ResolveColor(defaultSelectionColor)
	if v.hasSelection {
		n.selectionBG = ResolveColor(v.selection)
	}
	n.caretColor = n.fg
	if v.hasCaret {
		n.caretColor = ResolveColor(v.caret)
	}
	n.disabledFG = ResolveColor(Fade(ColorLabel, 0.4))
	n.disabledBG = ResolveColor(ColorControlDisabled)
	n.indicator = ResolveColor(Fade(ColorLabel, 0.35))
	n.focusRing = resolveBorder(defaultFieldFocusRing)
	if v.hasFocusRing {
		n.focusRing = resolveBorder(v.focusRing)
	}

	name := v.name
	if name == "" {
		name = v.placeholder
	}
	return gift.Element{
		Key:        v.key,
		Flex:       v.flex,
		Layouter:   n,
		Painter:    n,
		Interactor: n,
		Label:      name,
		Focusable:  !v.disabled,
		Disabled:   v.disabled,
		Clip:       true,
	}
}

// textAreaNode is the retained half of a [TextAreaView]. Like
// [textFieldNode] it holds no caret, selection or scroll offset of its own;
// those live in the [TextEditor].
type textAreaNode struct {
	ed *TextEditor

	fr  frameSpec
	st  styleSpec
	pad geom.Insets

	// req shapes the document and phReq the placeholder; both wrap at the
	// width the last layout found.
	req, phReq text.Request

	placeholder string

	fg, placeholderFG, selectionBG, caretColor Color
	disabledFG, disabledBG, indicator          Color
	focusRing                                  Border

	onChange func(string)
	onSubmit func(string)
	bind     gift.Binding[string]

	disabled           bool
	minLines, maxLines int

	// inner is the content rectangle of the last layout, in local space, and
	// lines the number of lines the text had there. An edit that changes the
	// number asks for a new layout, because the area grows with its text.
	inner geom.Rect
	lines int
}

// para returns the shaped document, wrapped at the width of the last layout.
func (n *textAreaNode) para() *text.Paragraph {
	n.req.Text = n.ed.Text()
	return text.Default().Layout(n.req)
}

func lineCount(p *text.Paragraph) int { return max(len(p.Lines), 1) }

// --- layout -------------------------------------------------------------------

// Layout sizes the area: as wide as it may be, and as tall as its lines,
// between the two numbers of [TextAreaView.Lines]. A frame or a min or max
// height overrides that, and the lines scroll inside whatever height it
// gets.
func (n *textAreaNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	n.ed.checkSingleMount(ctx)
	cc := n.fr.apply(c)
	w := cc.Max.W
	if !isFinite(w) {
		w = max(defaultAreaWidth, cc.Min.W)
	}
	// The caret after the last character of the longest line stays inside.
	wrap := max(w-n.pad.Horizontal()-caretWidth, 1)
	n.req.MaxWidth, n.phReq.MaxWidth = wrap, wrap
	p := n.para()
	lh := p.Metrics.LineHeight
	n.lines = lineCount(p)
	shown := max(n.lines, n.minLines)
	if n.ed.Len() == 0 && n.placeholder != "" {
		shown = max(shown, lineCount(text.Default().Layout(n.phReq)))
	}
	if n.maxLines > 0 {
		shown = min(shown, max(n.maxLines, n.minLines))
	}
	size := cc.Constrain(geom.Sz(w, float32(shown)*lh+n.pad.Vertical()))
	n.inner = geom.Rc(n.pad.Left, n.pad.Top, size.W-n.pad.Right, size.H-n.pad.Bottom)
	n.clampScroll(p)
	// Lines past the bottom scroll, on purpose, like the text of a field;
	// see [textFieldNode.Layout].
	ctx.ReportOverflow(geom.Size{})
	ctx.ReportBaseline(n.pad.Top + p.Metrics.FirstBaseline)
	return size
}

// maxScroll is how far the lines can scroll up.
func (n *textAreaNode) maxScroll(p *text.Paragraph) float32 {
	return max(0, float32(lineCount(p))*p.Metrics.LineHeight-n.inner.Height())
}

func (n *textAreaNode) clampScroll(p *text.Paragraph) {
	n.ed.scrollY = max(0, min(n.ed.scrollY, n.maxScroll(p)))
}

// --- line geometry ---------------------------------------------------------------
//
// The paragraph says which bytes each visual line covers, a hard break
// included, and where each glyph of it starts. The functions below turn that
// into the two mappings an editor needs, from a byte offset to a line and a
// column and back.
//
// One position is two places on the screen: the offset at a soft line break
// is both the end of the upper line and the start of the lower one. It is the
// start of the lower one unless the editor's up flag says otherwise, which
// End and a click past the end of a wrapped line set. A hard break has no
// such ambiguity: its characters belong to no line, they sit between the End
// of one and the Start of the next, so the offset before them is the end of
// the upper line and the one after them the start of the lower.

func isBreakRune(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// hardAfter reports whether line i of p ends at a hard break rather than
// wrapping into the next.
func hardAfter(p *text.Paragraph, i int) bool {
	return i+1 < len(p.Lines) && p.Lines[i+1].Start > p.Lines[i].End
}

// areaLine is the index of the line that shows the offset off.
func areaLine(p *text.Paragraph, off int, up bool) int {
	last := len(p.Lines) - 1
	for i := range p.Lines {
		if i == last || off < p.Lines[i+1].Start {
			if up && i > 0 && off == p.Lines[i].Start && !hardAfter(p, i-1) {
				return i - 1
			}
			return i
		}
	}
	return max(last, 0)
}

// areaX is the x of the offset off inside line i, from the start of the line.
func areaX(p *text.Paragraph, i, off int) float32 {
	if i < 0 || i >= len(p.Lines) {
		return 0
	}
	ln := &p.Lines[i]
	for ri := range ln.Runs {
		run := &ln.Runs[ri]
		for gi := range run.Glyphs {
			if g := &run.Glyphs[gi]; int(g.Cluster) >= off {
				return g.X
			}
		}
	}
	return ln.Advance
}

// offsetInLine is the offset a click at x in line i lands on, with the
// midpoint rule of [offsetAt], and whether it is the end of a wrapped line.
func offsetInLine(p *text.Paragraph, i int, x float32) (int, bool) {
	if i < 0 || i >= len(p.Lines) {
		return 0, false
	}
	ln := &p.Lines[i]
	if x <= 0 {
		return ln.Start, false
	}
	for ri := range ln.Runs {
		run := &ln.Runs[ri]
		for gi := range run.Glyphs {
			g := &run.Glyphs[gi]
			next := ln.Advance
			if gi+1 < len(run.Glyphs) {
				next = run.Glyphs[gi+1].X
			} else if ri+1 < len(ln.Runs) && len(ln.Runs[ri+1].Glyphs) > 0 {
				next = ln.Runs[ri+1].Glyphs[0].X
			}
			if x < (g.X+next)/2 {
				return int(g.Cluster), false
			}
		}
	}
	return ln.End, i < len(p.Lines)-1 && !hardAfter(p, i)
}

// caretAt is the line and the x of the caret.
func (n *textAreaNode) caretAt(p *text.Paragraph) (int, float32) {
	if len(p.Lines) == 0 {
		return 0, 0
	}
	i := areaLine(p, n.ed.caret, n.ed.up)
	return i, areaX(p, i, n.ed.caret)
}

// revealCaret scrolls the least that puts the line of the caret in view.
func (n *textAreaNode) revealCaret() {
	p := n.para()
	lh := p.Metrics.LineHeight
	i, _ := n.caretAt(p)
	top, h := float32(i)*lh, n.inner.Height()
	switch {
	case top < n.ed.scrollY:
		n.ed.scrollY = top
	case top+lh > n.ed.scrollY+h:
		n.ed.scrollY = top + lh - h
	}
	n.clampScroll(p)
}

// --- paint ----------------------------------------------------------------------

// Paint draws the area in the order of [textFieldNode.Paint], with the lines
// clipped to the content box for the reason given there: a line scrolled
// half out is cut at the inner edge of the padding, not drawn between the
// border and the focus ring. The scroll indicator stands in the padding on
// the right, between the ring and the text, so that it covers neither.
func (n *textAreaNode) Paint(ctx *gift.PaintContext) {
	assertResolvedBorder(n.focusRing, "the focus ring of a TextArea")
	ia := ctx.Interaction()
	st := n.st
	if ia.Disabled {
		st.background = n.disabledBG
	}
	b := ctx.Bounds()
	paintBackground(ctx, st, b)
	box := n.inner.Translate(b.Min)
	ctx.PushClip(deviceRect(ctx, box))
	n.paintContent(ctx, b, ia)
	ctx.PopClip()
	n.paintIndicator(ctx, b, box)
	paintBorder(ctx, st, b)
	if ia.FocusVisible && !ia.Disabled && n.focusRing.IsVisible() {
		paintBorder(ctx, styleSpec{border: n.focusRing, radius: focusRingRadius(st.radius)}, focusRingRect(b))
	}
}

func (n *textAreaNode) paintContent(ctx *gift.PaintContext, b geom.Rect, ia gift.Interaction) {
	assertResolved(n.fg, "the foreground of a TextArea")
	assertResolved(n.placeholderFG, "the placeholder colour of a TextArea")
	assertResolved(n.selectionBG, "the selection colour of a TextArea")
	assertResolved(n.caretColor, "the caret colour of a TextArea")

	p := n.para()
	lh := p.Metrics.LineHeight
	x := roundf(b.Min.X + n.pad.Left)
	y := roundf(b.Min.Y + n.pad.Top - n.ed.scrollY)
	fg := n.fg
	if ia.Disabled {
		fg = n.disabledFG
	}

	if n.ed.Len() == 0 {
		if n.placeholder != "" && !n.placeholderFG.IsTransparent() {
			paintLines(ctx, b, text.Default().Layout(n.phReq), n.placeholderFG, x, y)
		}
	} else {
		if lo, hi := n.ed.selection(); lo != hi && ia.Focused && !n.selectionBG.IsTransparent() {
			// The width of a selected line break, so that a selection over
			// an empty line shows.
			nl := roundf(n.req.Size * 0.3)
			for i := range p.Lines {
				ln := &p.Lines[i]
				top := y + float32(i)*lh
				if top+lh < b.Min.Y || top > b.Max.Y {
					continue
				}
				// The part of the line selected, and whether the selection
				// goes on past its end, over the break or the wrap.
				a, z := max(lo, ln.Start), min(hi, ln.End)
				tail := lo <= ln.End && hi > ln.End && i < len(p.Lines)-1
				if a >= z && !tail {
					continue
				}
				x0, x1 := areaX(p, i, a), areaX(p, i, z)
				if a >= z {
					x0 = areaX(p, i, ln.End)
					x1 = x0
				}
				if tail {
					x1 = max(x1, ln.Advance) + nl
				}
				ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(x+x0, top, x+x1, top+lh), Color: n.selectionBG})
			}
		}
		if !fg.IsTransparent() {
			paintLines(ctx, b, p, fg, x, y)
		}
	}

	if ia.Focused && !ia.Disabled && !n.caretColor.IsTransparent() && n.ed.caretVisible(ctx.Now()) {
		i, cx := n.caretAt(p)
		top := y + float32(i)*lh
		cx = roundf(x + cx)
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(cx, top, cx+caretWidth, top+lh), Color: n.caretColor})
	}

}

// paintIndicator draws the scroll indicator for as long as a scroll view
// shows its own: two pixels wide, beside the content box and as tall as it,
// in the middle of the room between the box and the inner edge of the focus
// ring, or of the bounds where the padding is too thin for that.
func (n *textAreaNode) paintIndicator(ctx *gift.PaintContext, b, box geom.Rect) {
	p := n.para()
	ms := n.maxScroll(p)
	if ms <= 0 || n.ed.scrolledAt == 0 || ctx.Now()-n.ed.scrolledAt >= gift.ScrollIndicatorLinger {
		return
	}
	ring := b.Max.X - focusRingGap - n.focusRing.Width
	mid := (max(box.Max.X, min(ring, b.Max.X)) + box.Max.X) / 2
	if ring-box.Max.X < 2 {
		mid = b.Max.X - 2
	}
	th := max(24, box.Height()*n.inner.Height()/(n.inner.Height()+ms))
	ty := box.Min.Y + (box.Height()-th)*n.ed.scrollY/ms
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(mid-1, ty, mid+1, ty+th), Color: n.indicator, CornerRadius: 1})
}

// paintLines copies the glyphs of the lines of p that fall inside b into the
// display list as one operation, with the top of the paragraph at y.
func paintLines(ctx *gift.PaintContext, b geom.Rect, p *text.Paragraph, fg Color, x, y float32) {
	lh := p.Metrics.LineHeight
	first := ctx.GlyphsLen()
	for i := range p.Lines {
		ln := &p.Lines[i]
		top := y + float32(i)*lh
		if top+lh < b.Min.Y || top > b.Max.Y {
			continue
		}
		baseline := y + ln.Baseline
		for ri := range ln.Runs {
			run := &ln.Runs[ri]
			id := render.FontID(run.Font.ID())
			for gi := range run.Glyphs {
				g := &run.Glyphs[gi]
				ctx.AppendGlyph(render.Glyph{Font: id, ID: render.GlyphID(g.ID), Size: run.Size, X: x + g.X, Y: baseline + g.Y})
			}
		}
	}
	if count := ctx.GlyphsLen() - first; count > 0 {
		ctx.Add(render.Op{Kind: render.OpGlyphs, Bounds: b, Color: fg, Glyphs: first, GlyphCount: count})
	}
}

// --- input ----------------------------------------------------------------------

// AccessibleValue implements [gift.ValueReporter]: the text of the area.
func (n *textAreaNode) AccessibleValue(gift.ControlState) string { return n.ed.Text() }

// HandleEvent implements gift.Interactor, along the lines of
// [textFieldNode.HandleEvent].
func (n *textAreaNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if n.disabled && e.Kind != gift.EventFocusLost {
		return false
	}
	switch e.Kind {
	case gift.EventFocusGained:
		n.ed.restartBlink(e.Time)
		ctx.Animate(CaretBlinkWindow)
		ctx.ScrollIntoView()
		if onScreenKeyboard.Load() {
			ctx.RequestSoftKeyboard(true)
		}
		return true

	case gift.EventFocusLost:
		if onScreenKeyboard.Load() {
			ctx.RequestSoftKeyboard(false)
		}
		n.ed.drag = dragNone
		ctx.Animate(0)
		return true

	case gift.EventPointerDown:
		ctx.RequestFocus()
		n.press(ctx, e)
		return true

	case gift.EventPointerMove:
		return n.move(ctx, e)

	case gift.EventPointerUp, gift.EventPointerCancel:
		if n.ed.drag == dragNone {
			return false
		}
		if n.ed.placeOnUp && n.ed.drag == dragPending && e.Kind == gift.EventPointerUp {
			// A finger that tapped.
			off, up := n.offsetOf(ctx, e.Pos)
			n.ed.MoveCaret(off, false)
			n.ed.up = up
			n.afterCaretMove(ctx, e.Time)
		}
		n.ed.drag, n.ed.placeOnUp = dragNone, false
		return true

	case gift.EventWheel:
		step := gift.DefaultScrollWheelStep
		if e.Precise {
			step = gift.DefaultScrollPreciseWheelStep
		}
		return n.scrollBy(ctx, e.Delta.Y*step, e.Time)

	case gift.EventKeyDown:
		return n.key(ctx, e)

	case gift.EventRune:
		// A control character is a key that has its own branch, enter
		// above all; a platform that also reports it as a character would
		// otherwise break the line twice.
		if e.Mods.Has(gift.ShortcutModifier) || unicode.IsControl(e.Rune) {
			return false
		}
		n.ed.InsertRune(e.Rune)
		n.afterEdit(ctx, e.Time)
		return true
	}
	return false
}

// scrollBy moves the lines down by dy, towards the start of the text, and
// reports whether they moved; an area that is at its end lets the wheel go
// on to the form around it.
func (n *textAreaNode) scrollBy(ctx *gift.EventContext, dy float32, now time.Duration) bool {
	p := n.para()
	before := n.ed.scrollY
	n.ed.scrollY -= dy
	n.clampScroll(p)
	if n.ed.scrollY == before {
		return false
	}
	n.ed.scrolledAt = now
	ctx.Animate(gift.ScrollIndicatorLinger)
	ctx.Repaint()
	return true
}

func (n *textAreaNode) move(ctx *gift.EventContext, e gift.Event) bool {
	ed := n.ed
	switch ed.drag {
	case dragNone:
		return false
	case dragScroll:
		dy := e.Pos.Y - ed.dragFrom.Y
		ed.dragFrom = e.Pos
		n.scrollBy(ctx, dy, e.Time)
		return true
	case dragPending:
		if !e.Dragged {
			return false
		}
		dx, dy := absf(e.Pos.X-ed.dragFrom.X), absf(e.Pos.Y-ed.dragFrom.Y)
		if dy > dx && e.Device != gift.PointerMouse {
			ed.placeOnUp = false
			if n.maxScroll(n.para()) > 0 {
				ed.drag, ed.dragFrom = dragScroll, e.Pos
				ctx.StealPointer()
				return true
			}
			ed.drag = dragNone
			return false
		}
		if ed.clicks >= 2 {
			ed.drag = dragWord
		} else {
			ed.drag = dragChar
		}
		if ed.placeOnUp {
			// A finger that selects starts where it pressed.
			off, _ := n.offsetOf(ctx, ed.dragFrom)
			ed.MoveCaret(off, false)
			ed.placeOnUp = false
		}
		ctx.StealPointer()
	}
	off, up := n.offsetOf(ctx, e.Pos)
	if ed.drag == dragWord {
		ed.dragExtendWord(off)
		ed.up = false
	} else {
		ed.MoveCaret(off, true)
		ed.up = up
	}
	ed.hasGoal = false
	n.afterCaretMove(ctx, e.Time)
	return true
}

// press places the caret, or selects a word or a paragraph, by the number of
// clicks in a row; see [textFieldNode.press].
func (n *textAreaNode) press(ctx *gift.EventContext, e gift.Event) {
	ed := n.ed
	if e.Time-ed.clickAt <= DoubleClickInterval && absf(e.Pos.X-ed.clickX) <= DoubleClickSlop {
		ed.clicks++
	} else {
		ed.clicks = 1
	}
	ed.clickAt, ed.clickX = e.Time, e.Pos.X
	ed.dragFrom = e.Pos
	off, up := n.offsetOf(ctx, e.Pos)
	ed.placeOnUp = false
	switch ed.clicks {
	case 1:
		// A mouse places the caret on the press, a finger when it lifts;
		// see [TextEditor.placeOnUp].
		if e.Device == gift.PointerMouse {
			ed.MoveCaret(off, e.Mods.Has(gift.ModShift))
			ed.up = up
		} else {
			ed.placeOnUp = true
		}
		ed.drag = dragPending
	case 2:
		ed.SelectWordAt(off)
		ed.dragWordLo, ed.dragWordHi = ed.selection()
		ed.up = false
		ed.drag = dragPending
	default:
		lo, hi := paragraphAt(ed.Text(), off)
		ed.SetSelection(lo, hi)
		ed.up = false
		ed.drag = dragNone
	}
	ed.hasGoal = false
	n.afterCaretMove(ctx, e.Time)
}

// paragraphAt is the text between the hard breaks around off.
func paragraphAt(s string, off int) (int, int) {
	lo := strings.LastIndexAny(s[:off], "\n\r\v\f\u0085  ") + 1
	if lo > 0 {
		// The index is of the first byte of the break; step past all of it.
		_, size := utf8.DecodeRuneInString(s[lo-1:])
		lo += size - 1
	}
	hi := len(s)
	if i := strings.IndexAny(s[off:], "\n\r\v\f\u0085  "); i >= 0 {
		hi = off + i
	}
	return lo, hi
}

// offsetOf turns a pointer position into an offset, and whether it is the
// end of a wrapped line. A position above or below the lines lands on the
// first or the last, which is how a drag past the edge scrolls: the caret
// goes to the line beyond, and revealing it moves the lines.
func (n *textAreaNode) offsetOf(ctx *gift.EventContext, pos geom.Point) (int, bool) {
	p := n.para()
	if len(p.Lines) == 0 {
		return 0, false
	}
	b := ctx.DeviceBounds()
	x := pos.X - b.Min.X - n.pad.Left
	y := pos.Y - b.Min.Y - n.pad.Top + n.ed.scrollY
	i := int(math.Floor(float64(y / p.Metrics.LineHeight)))
	i = max(0, min(i, len(p.Lines)-1))
	return offsetInLine(p, i, x)
}

// key is the keyboard half; see [textFieldNode.key] for the modifiers.
func (n *textAreaNode) key(ctx *gift.EventContext, e gift.Event) bool {
	ed := n.ed
	extend := e.Mods.Has(gift.ModShift)
	shortcut := e.Mods.Has(gift.ShortcutModifier)
	word := e.Mods.Has(gift.WordModifier)
	keepGoal := false

	switch e.Key {
	case gift.KeyLeft:
		switch {
		case word:
			ed.MoveWordLeft(extend)
			ed.up = false
		case shortcut:
			n.lineEdge(false, extend)
		default:
			ed.MoveLeft(extend)
			ed.up = false
		}
	case gift.KeyRight:
		switch {
		case word:
			ed.MoveWordRight(extend)
			ed.up = false
		case shortcut:
			n.lineEdge(true, extend)
		default:
			ed.MoveRight(extend)
			ed.up = false
		}
	case gift.KeyUp, gift.KeyDown, gift.KeyPageUp, gift.KeyPageDown:
		k := 1
		if e.Key == gift.KeyPageUp || e.Key == gift.KeyPageDown {
			k = max(1, int(n.inner.Height()/n.para().Metrics.LineHeight)-1)
		}
		if e.Key == gift.KeyUp || e.Key == gift.KeyPageUp {
			k = -k
		}
		if shortcut && (e.Key == gift.KeyUp || e.Key == gift.KeyDown) {
			if k < 0 {
				ed.MoveHome(extend)
			} else {
				ed.MoveEnd(extend)
			}
			ed.up = false
			break
		}
		n.moveLines(k, extend)
		keepGoal = true
	case gift.KeyHome:
		if shortcut {
			ed.MoveHome(extend)
			ed.up = false
			break
		}
		n.lineEdge(false, extend)
	case gift.KeyEnd:
		if shortcut {
			ed.MoveEnd(extend)
			ed.up = false
			break
		}
		n.lineEdge(true, extend)

	case gift.KeyBackspace:
		if ed.DeleteBackward() {
			n.afterEdit(ctx, e.Time)
		}
		return true
	case gift.KeyDelete:
		if ed.DeleteForward() {
			n.afterEdit(ctx, e.Time)
		}
		return true
	case gift.KeyEnter:
		if shortcut && n.onSubmit != nil {
			n.onSubmit(ed.Text())
			return true
		}
		ed.Insert("\n")
		n.afterEdit(ctx, e.Time)
		return true

	case gift.KeyA:
		if !shortcut {
			return false
		}
		ed.SelectAll()
		ed.up = false
	case gift.KeyC:
		if !shortcut {
			return false
		}
		copySelection(ed)
		return true
	case gift.KeyX:
		if !shortcut {
			return false
		}
		if copySelection(ed) && ed.DeleteSelection() {
			n.afterEdit(ctx, e.Time)
		}
		return true
	case gift.KeyV:
		if !shortcut {
			return false
		}
		if s, ok := CurrentClipboard().Text(); ok {
			if s = multiLine(s); s != "" && ed.Insert(s) {
				n.afterEdit(ctx, e.Time)
			}
		}
		return true

	default:
		return false
	}
	if !keepGoal {
		ed.hasGoal = false
	}
	n.afterCaretMove(ctx, e.Time)
	return true
}

// lineEdge moves the caret to the start or the end of the line it is shown
// on. The end of a wrapped line is the offset the next line starts at, shown
// at the end of this one.
func (n *textAreaNode) lineEdge(end, extend bool) {
	ed := n.ed
	p := n.para()
	if len(p.Lines) == 0 {
		return
	}
	i := areaLine(p, ed.caret, ed.up)
	ln := &p.Lines[i]
	if !end {
		ed.MoveCaret(ln.Start, extend)
		ed.up = false
		return
	}
	ed.MoveCaret(ln.End, extend)
	ed.up = i < len(p.Lines)-1 && !hardAfter(p, i)
}

// moveLines moves the caret k lines down, or up for a negative k, to the
// column it has kept since the first of a run of such moves. Past the first
// or the last line it goes to the start or the end of the text.
func (n *textAreaNode) moveLines(k int, extend bool) {
	ed := n.ed
	p := n.para()
	s := ed.Text()
	if len(p.Lines) == 0 {
		return
	}
	i := areaLine(p, ed.caret, ed.up)
	if !ed.hasGoal {
		ed.goalX, ed.hasGoal = areaX(p, i, ed.caret), true
	}
	switch j := i + k; {
	case j < 0:
		ed.MoveCaret(0, extend)
		ed.up = false
	case j >= len(p.Lines):
		ed.MoveCaret(len(s), extend)
		ed.up = false
	default:
		off, up := offsetInLine(p, j, ed.goalX)
		ed.MoveCaret(off, extend)
		ed.up = up
	}
}

// multiLine is what a paste has to survive to get into an area: every kind
// of line break becomes "\n", a tab becomes four spaces, and other control
// characters are dropped. See [singleLine] for the field.
func multiLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	for _, r := range s {
		switch {
		case isBreakRune(r):
			b.WriteByte('\n')
		case r == '\t':
			b.WriteString("    ")
		case isNotPrintable(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// afterEdit publishes the text, as [textFieldNode.afterEdit] does, and asks
// for a layout when the number of lines changed, because the area grows and
// shrinks with them.
func (n *textAreaNode) afterEdit(ctx *gift.EventContext, now time.Duration) {
	s := n.ed.Text()
	if !n.bind.IsZero() {
		n.bind.Set(s)
	}
	if n.onChange != nil {
		n.onChange(s)
	}
	n.ed.hasGoal, n.ed.up = false, false
	if lineCount(n.para()) != n.lines {
		ctx.RequestLayout()
	}
	n.afterCaretMove(ctx, now)
}

func (n *textAreaNode) afterCaretMove(ctx *gift.EventContext, now time.Duration) {
	n.ed.restartBlink(now)
	n.revealCaret()
	ctx.Animate(CaretBlinkWindow)
	ctx.Repaint()
}

// --- modifiers --------------------------------------------------------------------

// Lines sets the height of the area in lines of its font: at least min, and
// growing with the text up to max, past which the lines scroll. A max of zero
// grows without limit, which is what an area in a scrolling form wants; the
// default is three and zero. A frame or a min or max height overrides both.
func (v TextAreaView) Lines(min, max int) TextAreaView {
	if min < 1 {
		min = 1
	}
	if max < 0 {
		max = 0
	}
	v.minLines, v.maxLines = min, max
	return v
}

// Placeholder sets the text shown while the area is empty; see
// [TextFieldView.Placeholder].
func (v TextAreaView) Placeholder(s string) TextAreaView { v.placeholder = s; return v }

// Label sets the accessible name; see [TextFieldView.Label].
func (v TextAreaView) Label(s string) TextAreaView { v.name = s; return v }

// OnChange sets the function called with the text after every edit.
func (v TextAreaView) OnChange(f func(string)) TextAreaView { v.onChange = f; return v }

// OnSubmit sets the function called with the text when the user presses enter
// with [gift.ShortcutModifier], because enter alone starts a new line here.
// Without it that combination starts a new line too.
func (v TextAreaView) OnSubmit(f func(string)) TextAreaView { v.onSubmit = f; return v }

// Bind reads the text from b and writes every edit back; see
// [TextFieldView.Bind].
func (v TextAreaView) Bind(b gift.Binding[string]) TextAreaView { v.bind = b; return v }

// Disabled takes the area out of input.
func (v TextAreaView) Disabled(b bool) TextAreaView { v.disabled = b; return v }

// Font sets the font of the text.
func (v TextAreaView) Font(f Font) TextAreaView { v.font = f; return v }

// FontSize sets the size of the text, which is also what a line is measured
// in.
func (v TextAreaView) FontSize(s float32) TextAreaView {
	if !isFinite(s) || s <= 0 {
		panic("gift/ui: TextArea.FontSize must be a finite number greater than zero")
	}
	v.size = s
	return v
}

// Foreground sets the colour of the text.
func (v TextAreaView) Foreground(c Color) TextAreaView { v.fg, v.hasFG = c, true; return v }

// PlaceholderColor sets the colour of the placeholder.
func (v TextAreaView) PlaceholderColor(c Color) TextAreaView {
	v.placeholderFG, v.hasPlaceholderFG = c, true
	return v
}

// SelectionColor sets the colour behind selected text.
func (v TextAreaView) SelectionColor(c Color) TextAreaView {
	v.selection, v.hasSelection = c, true
	return v
}

// CaretColor sets the colour of the caret.
func (v TextAreaView) CaretColor(c Color) TextAreaView { v.caret, v.hasCaret = c, true; return v }

// FocusRing sets the ring drawn while the area holds a visible focus.
func (v TextAreaView) FocusRing(b Border) TextAreaView {
	v.focusRing, v.hasFocusRing = b, true
	return v
}

// Padding sets the room between the edge and the text on every side.
func (v TextAreaView) Padding(f float32) TextAreaView {
	v.setPadding(f)
	v.hasPadding = true
	return v
}

// PaddingInsets sets the room between the edge and the text per side.
func (v TextAreaView) PaddingInsets(i geom.Insets) TextAreaView {
	v.setPaddingInsets(i)
	v.hasPadding = true
	return v
}

// Frame fixes the size; see [BoxView.Frame].
func (v TextAreaView) Frame(w, h float32) TextAreaView { v.setFrame(w, h); return v }

// MinWidth sets the smallest width.
func (v TextAreaView) MinWidth(f float32) TextAreaView { v.setMinWidth(f); return v }

// MinHeight sets the smallest height.
func (v TextAreaView) MinHeight(f float32) TextAreaView { v.setMinHeight(f); return v }

// MaxWidth sets the largest width.
func (v TextAreaView) MaxWidth(f float32) TextAreaView { v.setMaxWidth(f); return v }

// MaxHeight sets the largest height, past which the lines scroll.
func (v TextAreaView) MaxHeight(f float32) TextAreaView { v.setMaxHeight(f); return v }

// Background sets the face.
func (v TextAreaView) Background(b Background) TextAreaView { v.setBackgroundSpec(b); return v }

// Border sets the outline.
func (v TextAreaView) Border(b Border) TextAreaView { v.setBorder(b); return v }

// Shadow sets the shadow.
func (v TextAreaView) Shadow(s Shadow) TextAreaView { v.setShadow(s); return v }

// CornerRadius sets the radius of the corners.
func (v TextAreaView) CornerRadius(f float32) TextAreaView { v.setCornerRadius(f); return v }

// Clip panics for false, as [TextFieldView.Clip] does: the lines scroll
// inside the area.
func (v TextAreaView) Clip(b bool) TextAreaView {
	if !b {
		panic("gift/ui: Clip(false) on a TextArea; the lines scroll inside the area, so an " +
			"area that did not clip would paint the scrolled out ones over its neighbours")
	}
	return v
}

// Key sets the reconciliation key.
func (v TextAreaView) Key(s string) TextAreaView { v.setKey(s); return v }

// Flex sets the flex factor in a stack.
func (v TextAreaView) Flex(f float32) TextAreaView { v.setFlex(f); return v }
