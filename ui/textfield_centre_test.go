package ui_test

import (
	"math"
	"testing"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// glyphBaselines returns the distinct baselines, as device y, of every glyph
// the current frame draws.
func glyphBaselines(h *gifttest.Harness) []float32 {
	l := h.List()
	var ys []float32
	for _, op := range h.Ops() {
		if op.Kind != render.OpGlyphs {
			continue
		}
		for _, g := range l.Glyphs(op.Glyphs, op.GlyphCount) {
			if len(ys) == 0 || ys[len(ys)-1] != g.Y {
				ys = append(ys, g.Y)
			}
		}
	}
	return ys
}

// fieldPadV is the default top and bottom padding of a field, spelled out for
// the reason [fieldPadLeft] is.
const fieldPadV = 5

// TestATallFieldCentresItsLine is the defect of a form that gave every control
// the same 44 pixel touch height: a field with a 16 pixel font drew its text,
// its placeholder and its caret directly under the top padding, and read as a
// field whose text had slipped.
//
// The line is centred in the content area now, and the rest has to follow it:
// the caret and the selection are drawn in the same band as the glyphs, and a
// press still lands on the character under it — including at the top edge of
// the field, which the line no longer reaches.
func TestATallFieldCentresItsLine(t *testing.T) {
	const s = "helloworld"
	tall := func(v ui.TextFieldView) ui.TextFieldView {
		return v.Frame(380, 44).Placeholder("Your name")
	}

	// A field of the natural height, for the two numbers the test needs from
	// it: how tall the line box is, and where the baseline sits in it.
	natural := newField(t, s, nil)
	nb := natural.node().Bounds()
	nys := glyphBaselines(natural.h)
	if len(nys) != 1 {
		t.Fatalf("a one line field drew glyphs on %d baselines", len(nys))
	}
	lineH := nb.Height() - 2*fieldPadV
	ascent := nys[0] - nb.Min.Y - fieldPadV

	f := newField(t, s, tall)
	b := f.node().Bounds()
	if b.Height() != 44 {
		t.Fatalf("the field is %v tall, want the 44 of its Frame", b.Height())
	}
	offset := fieldPadV + float32(math.Round(float64((b.Height()-2*fieldPadV-lineH)/2)))
	if offset <= fieldPadV {
		t.Fatalf("fixture: a line %v tall does not leave room to centre in %v", lineH, b.Height())
	}
	top := b.Min.Y + offset
	if ys := glyphBaselines(f.h); len(ys) != 1 || ys[0] != top+ascent {
		t.Fatalf("the text is drawn on the baselines %v, want the centred %v (the line box is "+
			"%v tall inside a content area %v tall)", ys, top+ascent, lineH, b.Height()-2*fieldPadV)
	}

	// The caret spans the line's band, not the field's.
	f.node().Focus()
	f.h.Frame()
	c, ok := caretOp(f.h)
	if !ok {
		t.Fatalf("a focused field drew no caret.\n%s", f.h.Dump())
	}
	if c.Bounds.Min.Y != top || c.Bounds.Height() != lineH {
		t.Errorf("the caret spans y %v..%v, want the centred line band %v..%v",
			c.Bounds.Min.Y, c.Bounds.Max.Y, top, top+lineH)
	}

	// Hit testing is by x alone, so a press at the very top of the field —
	// above the line — still lands on the character under it.
	x := b.Min.X + fieldPadLeft + prefixWidth(t, s[:5]) + 1
	f.h.Advance(DoubleClickGap)
	f.h.ClickAt(geom.Pt(x, b.Min.Y+1))
	if got := f.ed.Caret(); got != 5 {
		t.Fatalf("a click at the top edge above offset 5 put the caret at %d", got)
	}

	// The selection highlight is drawn in the same band.
	y := b.Min.Y + b.Height()/2
	f.h.Advance(DoubleClickGap)
	f.h.PressAt(geom.Pt(b.Min.X+fieldPadLeft+1, y))
	f.h.MoveTo(geom.Pt(x, y))
	f.h.ReleaseAt(geom.Pt(x, y))
	if got := f.ed.SelectedText(); got != "hello" {
		t.Fatalf("the drag selected %q, want %q", got, "hello")
	}
	if sel, ok := selectionOp(f.h); !ok || sel.Bounds.Min.Y != top || sel.Bounds.Height() != lineH {
		t.Errorf("the selection is %v (drawn: %v), want it in the line band %v..%v",
			sel.Bounds, ok, top, top+lineH)
	}

	// And the placeholder of an empty field sits where the text would.
	empty := newField(t, "", tall)
	want := empty.node().Bounds().Min.Y + offset + ascent
	if ys := glyphBaselines(empty.h); len(ys) != 1 || ys[0] != want {
		t.Errorf("the placeholder is drawn on the baselines %v, want the centred text baseline %v", ys, want)
	}
}
