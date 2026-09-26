package ui_test

import (
	"slices"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// paintedGlyphIDs lays out and paints v and returns the glyph ids of every
// glyph operation in the display list, in drawing order, together with the
// app so that a test can read its diagnostics.
func paintedGlyphIDs(t testing.TB, v gift.View, viewport geom.Size) ([]render.GlyphID, *gift.App) {
	t.Helper()
	a := gift.New(gift.Options{Root: func(*gift.Context) gift.View { return v }})
	if err := a.Update(viewport); err != nil {
		t.Fatal(err)
	}
	l := a.Paint()
	var ids []render.GlyphID
	for _, op := range l.Ops() {
		if op.Kind != render.OpGlyphs {
			continue
		}
		for _, g := range l.Glyphs(op.Glyphs, op.GlyphCount) {
			ids = append(ids, g.ID)
		}
	}
	return ids, a
}

// glyphsOf returns the glyph ids s is drawn with on one unbounded line. The
// strings in this file contain no ligature in Roboto, so comparing the ids of
// a truncated label against the ids of the string it is expected to read is
// comparing the two strings.
func glyphsOf(t testing.TB, s string) []render.GlyphID {
	t.Helper()
	ids, _ := paintedGlyphIDs(t, ui.VStack(textOf(t, s).FontSize(16)), geom.Sz(4000, 200))
	return ids
}

// TestMaxLinesEndsInAnEllipsisWhenItCuts is the defect a touchscreen photo
// booth reported: a one line album name in a row that was too narrow for it
// read "Hochzeit Anna &" and stopped, and nobody could tell the name went on.
// It now reads "Hochzeit Anna & B…", and the cut is still reported as
// overflow, because an ellipsis is a mark and not a number.
func TestMaxLinesEndsInAnEllipsisWhenItCuts(t *testing.T) {
	const name = "Hochzeit Anna & Bernd"
	f := loadTestFont(t)
	limit := ui.MeasureForTest(f, "Hochzeit Anna & B…", 16, geom.Unbounded()).W + 1

	ids, a := paintedGlyphIDs(t, ui.VStack(textOf(t, name).FontSize(16).MaxWidth(limit).MaxLines(1)), geom.Sz(800, 200))
	if want := glyphsOf(t, "Hochzeit Anna & B…"); !slices.Equal(ids, want) {
		t.Errorf("MaxLines(1) at %v px drew %v, want the glyphs of %q %v", limit, ids, "Hochzeit Anna & B…", want)
	}
	if a.Diagnostics().OverflowNodes == 0 {
		t.Error("the truncated label reported no overflow; what the ellipsis hides must stay a number")
	}

	// The old hard cut is one modifier away, for a caller that relied on it.
	ids, _ = paintedGlyphIDs(t, ui.VStack(textOf(t, name).FontSize(16).MaxWidth(limit).MaxLines(1).
		Truncation(ui.TruncateNone)), geom.Sz(800, 200))
	if want := glyphsOf(t, "Hochzeit Anna & "); !slices.Equal(ids, want) {
		t.Errorf("TruncateNone drew %v, want the first wrapped line alone %v", ids, want)
	}
}

// TestMaxLinesDrawsNoEllipsisWhenTheTextFits is the other half: the default
// changed, and a label that fits must come out exactly as it did before.
func TestMaxLinesDrawsNoEllipsisWhenTheTextFits(t *testing.T) {
	const name = "Hochzeit Anna & Bernd"
	ids, a := paintedGlyphIDs(t, ui.VStack(textOf(t, name).FontSize(16).MaxWidth(400).MaxLines(1)), geom.Sz(800, 200))
	if want := glyphsOf(t, name); !slices.Equal(ids, want) {
		t.Errorf("a label that fits drew %v, want %v", ids, want)
	}
	if ell := glyphsOf(t, "…")[0]; slices.Contains(ids, ell) {
		t.Error("a label that fits drew an ellipsis")
	}
	if n := a.Diagnostics().OverflowNodes; n != 0 {
		t.Errorf("a label that fits reported %d overflowing nodes", n)
	}
}

// TestTruncationMiddleKeepsTheFileName: the mode a path wants, and on its own,
// without MaxLines, it means one line.
func TestTruncationMiddleKeepsTheFileName(t *testing.T) {
	const path = "/Volumes/photos/2026/hochzeit-anna-bernd/IMG_0001.jpg"
	f := loadTestFont(t)
	lineH := ui.LineHeightForTest(f, 16)

	v := textOf(t, path).FontSize(16).Frame(240, geom.Unbounded()).Truncation(ui.TruncateMiddle)
	ids, a := paintedGlyphIDs(t, ui.VStack(v), geom.Sz(800, 200))
	file := glyphsOf(t, "IMG_0001.jpg")
	if len(ids) < len(file) || !slices.Equal(ids[len(ids)-len(file):], file) {
		t.Errorf("the middle truncated path does not end in its file name: %v", ids)
	}
	if !slices.Contains(ids, glyphsOf(t, "…")[0]) {
		t.Errorf("no ellipsis in %v", ids)
	}
	if h := rootSize(t, a).H; h > lineH+1 {
		t.Errorf("the path is %v tall; Truncation without MaxLines is one line of %v", h, lineH)
	}
}
