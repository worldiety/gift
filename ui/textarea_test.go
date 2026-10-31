package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// areaFixture is one text area alone in the window, focused.
type areaFixture struct {
	h      *gifttest.Harness
	ed     *ui.TextEditor
	change []string
}

func newArea(t *testing.T, initial string, decorate func(ui.TextAreaView) ui.TextAreaView) *areaFixture {
	t.Helper()
	f := &areaFixture{ed: ui.NewTextEditor(initial)}
	v := ui.TextArea(f.ed).
		Font(loadTestFont(t)).
		FontSize(16).
		Frame(240, geom.Unbounded()).
		OnChange(func(s string) { f.change = append(f.change, s) }).
		Key("area")
	if decorate != nil {
		v = decorate(v)
	}
	f.h = gifttest.New(t, gifttest.Options{
		View: ui.ZStack(ui.Box(), ui.VStack(v, ui.Box().Frame(10, 10).Key("below")).Padding(20)),
		Size: geom.Sz(400, 400),
		Font: loadTestFont(t),
	})
	f.node().Focus()
	return f
}

func (f *areaFixture) node() gifttest.Node { return f.h.Find(gifttest.ByKey("area")) }

// caret is the caret of the frame; it is as tall as a line.
func (f *areaFixture) caret(t *testing.T) geom.Rect {
	t.Helper()
	op, ok := caretOp(f.h)
	if !ok {
		t.Fatal("no caret drawn")
	}
	return op.Bounds
}

func TestAnAreaGrowsWithItsLinesAndScrollsPastItsMaximum(t *testing.T) {
	f := newArea(t, "", func(v ui.TextAreaView) ui.TextAreaView { return v.Lines(2, 4) })
	lh := f.caret(t).Height()
	empty := f.node().LayoutBounds().Height()
	f.h.TypeText("one")
	f.h.Key(gift.KeyEnter)
	f.h.TypeText("two")
	if got := f.node().LayoutBounds().Height(); got != empty {
		t.Fatalf("two lines are %v tall, want the two of the minimum, %v", got, empty)
	}
	f.h.Key(gift.KeyEnter)
	f.h.TypeText("three")
	if got := f.node().LayoutBounds().Height(); got != empty+lh {
		t.Fatalf("three lines are %v tall, want %v", got, empty+lh)
	}
	for _, s := range []string{"four", "five", "six"} {
		f.h.Key(gift.KeyEnter)
		f.h.TypeText(s)
	}
	b := f.node().LayoutBounds()
	if got := b.Height(); got != empty+2*lh {
		t.Fatalf("six lines are %v tall, want the four of the maximum, %v", got, empty+2*lh)
	}
	if got := f.ed.Text(); got != "one\ntwo\nthree\nfour\nfive\nsix" {
		t.Fatalf("the text is %q", got)
	}
	// The caret on the sixth line is in view: the lines scrolled.
	if c := f.caret(t); c.Max.Y > b.Max.Y || c.Min.Y < b.Min.Y {
		t.Fatalf("the caret at %v is outside the area at %v", c, b)
	}
	// The box below moved down with the area.
	if below := f.h.Find(gifttest.ByKey("below")).LayoutBounds(); below.Min.Y < b.Max.Y {
		t.Fatalf("the box below at %v overlaps the area at %v", below, b)
	}
	if got := f.change[len(f.change)-1]; got != f.ed.Text() {
		t.Fatalf("OnChange last got %q", got)
	}
}

func TestUpAndDownKeepTheirColumn(t *testing.T) {
	f := newArea(t, "hello world\nhi\nhello world", nil)
	f.ed.SetCaret(8) // "hello wo|rld"
	f.h.Key(gift.KeyDown)
	if got := f.ed.Caret(); got != 14 {
		t.Fatalf("down onto the short line put the caret at %d, want its end, 14", got)
	}
	f.h.Key(gift.KeyDown)
	if got := f.ed.Caret(); got != 15+8 {
		t.Fatalf("down again put the caret at %d, want the old column, %d", got, 15+8)
	}
	f.h.Key(gift.KeyUp)
	f.h.Key(gift.KeyUp)
	if got := f.ed.Caret(); got != 8 {
		t.Fatalf("up twice put the caret at %d, want 8", got)
	}
	f.h.Key(gift.KeyUp)
	if got := f.ed.Caret(); got != 0 {
		t.Fatalf("up on the first line put the caret at %d, want the start", got)
	}
	f.h.Key(gift.KeyDown, gift.ShortcutModifier)
	if got := f.ed.Caret(); got != f.ed.Len() {
		t.Fatalf("the shortcut with down put the caret at %d, want the end", got)
	}
	// Shift extends across lines.
	f.ed.SetCaret(3)
	f.h.Key(gift.KeyDown, gift.ModShift)
	if lo, hi := f.ed.Selection(); lo != 3 || hi != 14 {
		t.Fatalf("shift down selected %d..%d, want 3..14", lo, hi)
	}
}

func TestHomeAndEndGoToTheEndsOfTheLineOnTheScreen(t *testing.T) {
	// Narrow enough that the sentence wraps.
	f := newArea(t, "one two three four five six seven eight nine ten", func(v ui.TextAreaView) ui.TextAreaView {
		return v.Frame(120, geom.Unbounded())
	})
	f.ed.SetCaret(1)
	f.h.Key(gift.KeyHome)
	first := f.caret(t)
	f.h.Key(gift.KeyEnd)
	end := f.caret(t)
	if end.Min.Y != first.Min.Y || end.Min.X <= first.Min.X+40 {
		t.Fatalf("end of the first line drew the caret at %v, home at %v", end, first)
	}
	wrapped := f.ed.Caret()
	if wrapped == 0 || wrapped == f.ed.Len() {
		t.Fatalf("end of a wrapped line put the caret at %d", wrapped)
	}
	// The same offset by right arrow is the start of the next line.
	f.h.Key(gift.KeyLeft)
	f.h.Key(gift.KeyRight)
	if next := f.caret(t); f.ed.Caret() != wrapped || next.Min.Y <= first.Min.Y {
		t.Fatalf("right to offset %d drew the caret at %v, want the next line", f.ed.Caret(), next)
	}
	f.h.Key(gift.KeyEnd, gift.ShortcutModifier)
	if got := f.ed.Caret(); got != f.ed.Len() {
		t.Fatalf("the shortcut with end put the caret at %d", got)
	}
}

func TestAClickLandsOnTheLineUnderItAndATripleClickSelectsTheParagraph(t *testing.T) {
	f := newArea(t, "first\nsecond line\nthird", nil)
	f.h.Key(gift.KeyHome, gift.ShortcutModifier)
	top := f.caret(t)
	lh := top.Height()
	b := f.node().Bounds()
	x := b.Min.X + 8 + ui.MeasureForTest(loadTestFont(t), "sec", 16, geom.Unbounded()).W + 1
	y := top.Min.Y + lh*1.5
	f.h.ClickAt(geom.Pt(x, y))
	if got := f.ed.Caret(); got != 6+3 {
		t.Fatalf("a click after sec put the caret at %d, want 9", got)
	}
	f.h.ClickAt(geom.Pt(x, y))
	f.h.ClickAt(geom.Pt(x, y))
	if lo, hi := f.ed.Selection(); lo != 6 || hi != 17 {
		t.Fatalf("a triple click selected %d..%d, want the second paragraph, 6..17", lo, hi)
	}
	if _, ok := selectionOp(f.h); !ok {
		t.Fatal("no selection drawn")
	}
}

func TestAPasteKeepsItsLinesButNotItsCarriageReturns(t *testing.T) {
	cb := &ui.MemoryClipboard{}
	ui.SetClipboard(cb)
	t.Cleanup(func() { ui.SetClipboard(nil) })
	cb.SetText("a\r\nb\tc\x00")
	f := newArea(t, "", nil)
	f.h.Key(gift.KeyV, gift.ShortcutModifier)
	if got := f.ed.Text(); got != "a\nb    c" {
		t.Fatalf("paste produced %q", got)
	}
	f.h.Key(gift.KeyA, gift.ShortcutModifier)
	f.h.Key(gift.KeyC, gift.ShortcutModifier)
	if got, _ := cb.Text(); got != "a\nb    c" {
		t.Fatalf("copy put %q on the clipboard", got)
	}
}

func TestTheWheelScrollsAnAreaThatOverflowsAndOnlyThen(t *testing.T) {
	f := newArea(t, "1\n2\n3\n4\n5\n6\n7\n8", func(v ui.TextAreaView) ui.TextAreaView { return v.Lines(3, 3) })
	f.h.Key(gift.KeyHome, gift.ShortcutModifier)
	before := glyphTop(f.h)
	c := centreOf(f.node().Bounds())
	f.h.Wheel(c, geom.Pt(0, -1))
	after := glyphTop(f.h)
	if !(after < before) {
		t.Fatalf("the wheel did not scroll the lines: their top was %v and is %v", before, after)
	}
	// An area that fits lets the wheel go.
	g := newArea(t, "short", nil)
	top := glyphTop(g.h)
	g.h.Wheel(centreOf(g.node().Bounds()), geom.Pt(0, -1))
	if got := glyphTop(g.h); got != top {
		t.Fatalf("an area that fits scrolled from %v to %v", top, got)
	}
}

// glyphTop is the smallest glyph y of the frame.
func glyphTop(h *gifttest.Harness) float32 {
	top := float32(1e9)
	l := h.List()
	for _, op := range h.Ops() {
		if op.Kind != render.OpGlyphs {
			continue
		}
		for _, g := range l.Glyphs(op.Glyphs, op.GlyphCount) {
			top = min(top, g.Y)
		}
	}
	return top
}

func TestAFingerScrollsTheLinesAndAMouseSelectsThem(t *testing.T) {
	f := newArea(t, "1\n2\n3\n4\n5\n6\n7\n8", func(v ui.TextAreaView) ui.TextAreaView { return v.Lines(3, 3) })
	f.h.Key(gift.KeyHome, gift.ShortcutModifier)
	// The caret stays on the first line and moves with it, drawn or not.
	before := f.caret(t).Min.Y
	c := centreOf(f.node().Bounds())
	f.h.FingerDown(1, c)
	for i := 1; i <= 8; i++ {
		f.h.FingerMove(1, geom.Pt(c.X, c.Y-float32(i)*5))
		f.h.AdvanceTicks(1, gifttest.FrameInterval)
	}
	f.h.FingerUp(1, geom.Pt(c.X, c.Y-40))
	// Forty pixels, less the slop the gesture needed to be one.
	if after := f.caret(t).Min.Y; !(after < before-25) {
		t.Fatalf("a finger forty pixels up moved the lines from %v to %v", before, after)
	}
	if f.ed.HasSelection() || f.ed.Caret() != 0 {
		t.Fatalf("the scrolling finger moved the caret to %d or selected", f.ed.Caret())
	}
	// A tap places the caret, when it lifts.
	f.h.Advance(time.Second)
	f.h.TapAt(c)
	if f.ed.Caret() == 0 || f.ed.HasSelection() {
		t.Fatalf("a tap left the caret at %d", f.ed.Caret())
	}
	// The same stroke with a mouse selects across the lines.
	g := newArea(t, "1\n2\n3\n4\n5\n6\n7\n8", func(v ui.TextAreaView) ui.TextAreaView { return v.Lines(8, 8) })
	b := g.node().Bounds()
	from := geom.Pt(b.Min.X+9, b.Min.Y+10)
	g.h.PressAt(from)
	g.h.MoveTo(geom.Pt(from.X+2, from.Y+20))
	g.h.MoveTo(geom.Pt(from.X+2, from.Y+60))
	g.h.ReleaseAt(geom.Pt(from.X+2, from.Y+60))
	if lo, hi := g.ed.Selection(); lo != 0 || hi < 4 {
		t.Fatalf("a mouse drag down three lines selected %d..%d", lo, hi)
	}
}

// TestScrolledTextIsCutAtThePaddingNotAtTheBorder pins the clip of the
// lines: a line scrolled half out must not show between the border and the
// focus ring, which sits inside the border.
func TestScrolledTextIsCutAtThePaddingNotAtTheBorder(t *testing.T) {
	f := newArea(t, "1\n2\n3\n4\n5\n6\n7\n8", func(v ui.TextAreaView) ui.TextAreaView { return v.Lines(3, 3) })
	f.h.Wheel(centreOf(f.node().Bounds()), geom.Pt(0, -0.3))
	b := f.node().Bounds()
	l := f.h.List()
	seen := false
	for _, op := range f.h.Ops() {
		if op.Kind != render.OpGlyphs {
			continue
		}
		seen = true
		c := l.Clip(op.Clip)
		if c.Min.Y < b.Min.Y+8 || c.Max.Y > b.Max.Y-8 || c.Min.X < b.Min.X+8 || c.Max.X > b.Max.X-8 {
			t.Fatalf("the lines are clipped to %v, which reaches into the padding of the area at %v", c, b)
		}
	}
	if !seen {
		t.Fatal("no lines drawn")
	}
	// The single line field cuts its scrolled text at its side padding.
	g := newField(t, "a value much longer than the field is wide, so that it scrolls", nil)
	g.node().Focus()
	g.h.Key(gift.KeyEnd)
	fb := g.node().Bounds()
	for _, op := range g.h.Ops() {
		if op.Kind == render.OpGlyphs {
			if c := g.h.List().Clip(op.Clip); c.Min.X < fb.Min.X+fieldPadLeft || c.Max.X > fb.Max.X-fieldPadLeft {
				t.Fatalf("the field's text is clipped to %v, which reaches into its padding at %v", c, fb)
			}
		}
	}
}

func TestShortcutEnterSubmitsAnArea(t *testing.T) {
	var submitted []string
	f := newArea(t, "one", func(v ui.TextAreaView) ui.TextAreaView {
		return v.OnSubmit(func(s string) { submitted = append(submitted, s) })
	})
	f.h.Key(gift.KeyEnter, gift.ShortcutModifier)
	if len(submitted) != 1 || submitted[0] != "one" {
		t.Errorf("submitted %q", submitted)
	}
	if got := f.ed.Text(); got != "one" {
		t.Errorf("text %q after a submission", got)
	}
	f.h.Key(gift.KeyEnter)
	if len(submitted) != 1 || f.ed.Text() != "one\n" {
		t.Errorf("enter alone: submitted %q, text %q", submitted, f.ed.Text())
	}
}

func TestHoldingShortcutEnterSubmitsAnAreaOnce(t *testing.T) {
	submitted := 0
	f := newArea(t, "one", func(v ui.TextAreaView) ui.TextAreaView {
		return v.OnSubmit(func(string) { submitted++ })
	})
	f.h.KeyDown(gift.KeyEnter, gift.ShortcutModifier)
	f.h.AdvanceTicks(120, 16*time.Millisecond)
	f.h.KeyUp(gift.KeyEnter, gift.ShortcutModifier)
	if submitted != 1 {
		t.Errorf("submitted %d times", submitted)
	}
	if got := f.ed.Text(); got != "one" {
		t.Errorf("text %q", got)
	}
}

func TestShortcutEnterWithoutOnSubmitStartsALine(t *testing.T) {
	f := newArea(t, "one", nil)
	f.h.Key(gift.KeyEnter, gift.ShortcutModifier)
	if got := f.ed.Text(); got != "one\n" {
		t.Errorf("text %q", got)
	}
}
