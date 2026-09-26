package text

import (
	"slices"
	"testing"

	"github.com/worldiety/gift/geom"
)

// glyphIDs returns the glyph ids of every line of p, in order.
func glyphIDs(p *Paragraph) []GlyphID {
	var out []GlyphID
	for _, ln := range p.Lines {
		for _, r := range ln.Runs {
			for _, g := range r.Glyphs {
				out = append(out, g.ID)
			}
		}
	}
	return out
}

// idsOf returns the glyph ids of text shaped on one unbounded line. The
// expectations below spell the elided line as a string and compare ids, which
// is exact for Roboto and the strings used here: none of them contains a
// ligature, so one rune is one glyph on either side.
func idsOf(t testing.TB, s *Shaper, f *Font, text string) []GlyphID {
	t.Helper()
	return glyphIDs(s.Layout(Request{Text: text, Font: f, Size: 16, MaxWidth: geom.Unbounded()}))
}

// runWidth is the sum of the glyph advances of the only line of p, which is
// the number [Line.Width] must be for the paragraph to be one shaping result
// used twice.
func runWidth(p *Paragraph) float32 {
	var w float32
	for _, r := range p.Lines[len(p.Lines)-1].Runs {
		for _, g := range r.Glyphs {
			w += g.Advance
		}
	}
	return w
}

// TestTailTruncationFillsTheLastLineAndEndsInAnEllipsis is the defect a photo
// booth found: a one line label in a row too narrow for it showed
// "Hochzeit Anna &" and stopped, with nothing to say the name went on. The
// last line now carries as much of the rest of the paragraph as fits, then the
// ellipsis, and is still no wider than the limit.
func TestTailTruncationFillsTheLastLineAndEndsInAnEllipsis(t *testing.T) {
	f := loadRoboto(t)
	s := newTestShaper()
	const text = "Hochzeit Anna & Bernd"
	// Wide enough for "Hochzeit Anna & B…" and not for the whole string.
	limit := width(t, s, f, 16, "Hochzeit Anna & B…") + 1
	if full := width(t, s, f, 16, text); full <= limit {
		t.Fatalf("fixture: the whole text (%v) fits the limit %v", full, limit)
	}

	hard := s.Layout(Request{Text: text, Font: f, Size: 16, MaxWidth: limit, MaxLines: 1})
	if got := lineTexts(Request{Text: text}, hard); !equalStrings(got, []string{"Hochzeit Anna & "}) {
		t.Fatalf("TruncateNone kept %q, want the first wrapped line alone", got)
	}
	if slices.Contains(glyphIDs(hard), idsOf(t, s, f, "…")[0]) {
		t.Error("TruncateNone drew an ellipsis; it must cut hard")
	}

	p := s.Layout(Request{Text: text, Font: f, Size: 16, MaxWidth: limit, MaxLines: 1, Truncation: TruncateTail})
	if n := p.LineCount(); n != 1 {
		t.Fatalf("%d lines, want 1", n)
	}
	if got, want := glyphIDs(p), idsOf(t, s, f, "Hochzeit Anna & B…"); !slices.Equal(got, want) {
		t.Errorf("glyphs %v, want those of %q %v", got, "Hochzeit Anna & B…", want)
	}
	if p.Size.W > limit {
		t.Errorf("the elided line is %v wide, over the limit %v", p.Size.W, limit)
	}
	if !nearly(p.Lines[0].Width, runWidth(p)) || !nearly(p.Size.W, p.Lines[0].Width) {
		t.Errorf("line width %v, size %v, glyph advances %v: measuring and drawing disagree",
			p.Lines[0].Width, p.Size.W, runWidth(p))
	}
	if p.Overflow != 0 {
		t.Errorf("Overflow %v; the elided line fits", p.Overflow)
	}
	if want := p.Metrics.LineHeight; p.Hidden != geom.Sz(0, want) {
		t.Errorf("Hidden %v, want the one dropped line %v", p.Hidden, geom.Sz(0, want))
	}
	if ln := p.Lines[0]; ln.Start != 0 || ln.End != len(text) {
		t.Errorf("the elided line covers [%d, %d), want the whole text [0, %d)", ln.Start, ln.End, len(text))
	}
}

// TestTruncationIsAbsentWhenTheTextFits pins the other half: a text that fits
// comes out exactly as it would without a truncation mode — same glyphs, same
// size, nothing hidden. An ellipsis on a label that is not cut would be a lie
// in the other direction.
func TestTruncationIsAbsentWhenTheTextFits(t *testing.T) {
	f := loadRoboto(t)
	s := newTestShaper()
	for _, mode := range []Truncation{TruncateTail, TruncateMiddle, TruncateHead} {
		for _, req := range []Request{
			{Text: "Hochzeit", MaxWidth: 200, MaxLines: 1},
			{Text: "two lines\nexactly", MaxWidth: 200, MaxLines: 2},
			{Text: "wrapped over two lines", MaxWidth: 100, MaxLines: 3},
			{Text: "no limit at all", MaxWidth: geom.Unbounded()},
		} {
			req.Font, req.Size = f, 16
			plain := req
			plain.MaxLines, plain.Truncation = 0, TruncateNone
			want := glyphIDs(s.Layout(plain))
			wantSize := s.Measure(plain)

			req.Truncation = mode
			p := s.Layout(req)
			if got := glyphIDs(p); !slices.Equal(got, want) {
				t.Errorf("mode %d, %q: glyphs %v, want the untruncated %v", mode, req.Text, got, want)
			}
			if p.Size != wantSize || p.Hidden != (geom.Size{}) {
				t.Errorf("mode %d, %q: size %v hidden %v, want %v and nothing hidden", mode, req.Text, p.Size, p.Hidden, wantSize)
			}
		}
	}
}

// TestAWordWiderThanTheLimitIsElidedOnlyWhenAsked keeps the overflow model of
// TestWordLongerThanTheLimitOverflows the default and shows what a truncation
// mode changes about it: the word is cut to fit, and the amount it was cut by
// is in Hidden instead of Overflow.
func TestAWordWiderThanTheLimitIsElidedOnlyWhenAsked(t *testing.T) {
	f := loadRoboto(t)
	s := newTestShaper()
	const word = "Donaudampfschifffahrtsgesellschaft"
	const limit = 100
	honest := width(t, s, f, 16, word)

	p := s.Layout(Request{Text: word, Font: f, Size: 16, MaxWidth: limit, Truncation: TruncateTail})
	if p.Size.W > limit || p.Overflow != 0 {
		t.Errorf("the elided word is %v wide with overflow %v; the limit is %v", p.Size.W, p.Overflow, limit)
	}
	ids := glyphIDs(p)
	if ids[len(ids)-1] != idsOf(t, s, f, "…")[0] {
		t.Errorf("the elided word does not end in the ellipsis: %v", ids)
	}
	if want := honest - limit; !nearly(p.Hidden.W, want) || p.Hidden.H != 0 {
		t.Errorf("Hidden %v, want the width the word lost, %v, and no height", p.Hidden, want)
	}
}

// TestMiddleAndHeadKeepTheEndOfTheText is the case the modes exist for: a file
// path in a row, where the part that tells two entries apart is the file name
// at the end.
func TestMiddleAndHeadKeepTheEndOfTheText(t *testing.T) {
	f := loadRoboto(t)
	s := newTestShaper()
	const path = "/Volumes/photos/2026/hochzeit-anna-bernd/IMG_0001.jpg"
	const limit = 240
	ell := idsOf(t, s, f, "…")[0]

	for _, tc := range []struct {
		mode        Truncation
		start, tail string
	}{
		{TruncateMiddle, "/Volumes/", "IMG_0001.jpg"},
		{TruncateHead, "", "IMG_0001.jpg"},
	} {
		p := s.Layout(Request{Text: path, Font: f, Size: 16, MaxWidth: limit, MaxLines: 1, Truncation: tc.mode})
		if p.LineCount() != 1 || p.Size.W > limit {
			t.Fatalf("mode %d: %d lines, %v wide; want one line within %v", tc.mode, p.LineCount(), p.Size.W, limit)
		}
		ids := glyphIDs(p)
		if head := idsOf(t, s, f, tc.start); len(ids) < len(head) || !slices.Equal(ids[:len(head)], head) {
			t.Errorf("mode %d: the line does not start with %q", tc.mode, tc.start)
		}
		if tail := idsOf(t, s, f, tc.tail); len(ids) < len(tail) || !slices.Equal(ids[len(ids)-len(tail):], tail) {
			t.Errorf("mode %d: the line does not end with %q", tc.mode, tc.tail)
		}
		if !slices.Contains(ids, ell) {
			t.Errorf("mode %d: no ellipsis in %v", tc.mode, ids)
		}
		if tc.mode == TruncateHead && ids[0] != ell {
			t.Errorf("head truncation does not begin with the ellipsis: %v", ids)
		}
		if !nearly(p.Size.W, runWidth(p)) {
			t.Errorf("mode %d: size %v, glyph advances %v", tc.mode, p.Size.W, runWidth(p))
		}
	}
}

// TestTailTruncationStopsAtTheParagraph pins the choice for text with a
// mandatory break: the ellipsis follows the paragraph the last line belongs
// to, and the next paragraph is not spliced onto it.
func TestTailTruncationStopsAtTheParagraph(t *testing.T) {
	f := loadRoboto(t)
	s := newTestShaper()
	p := s.Layout(Request{Text: "first\nsecond", Font: f, Size: 16, MaxWidth: geom.Unbounded(),
		MaxLines: 1, Truncation: TruncateTail})
	if got, want := glyphIDs(p), idsOf(t, s, f, "first…"); !slices.Equal(got, want) {
		t.Errorf("glyphs %v, want those of \"first…\" %v", got, want)
	}
}

// TestNoSpaceBeforeTheEllipsis: "Anna & …" reads as an ellipsis that belongs
// to no word.
func TestNoSpaceBeforeTheEllipsis(t *testing.T) {
	f := loadRoboto(t)
	s := newTestShaper()
	// Room for "Anna &" and the ellipsis, not for the space and the next word.
	limit := width(t, s, f, 16, "Anna & …") + 1
	p := s.Layout(Request{Text: "Anna & Bernd", Font: f, Size: 16, MaxWidth: limit, MaxLines: 1, Truncation: TruncateTail})
	if got, want := glyphIDs(p), idsOf(t, s, f, "Anna &…"); !slices.Equal(got, want) {
		t.Errorf("glyphs %v, want those of \"Anna &…\" %v", got, want)
	}
}

// TestAnEllipsisThatDoesNotFitIsDrawnAlone: a limit narrower than the ellipsis
// shows the ellipsis and overflows by the rest, rather than showing nothing.
func TestAnEllipsisThatDoesNotFitIsDrawnAlone(t *testing.T) {
	f := loadRoboto(t)
	s := newTestShaper()
	p := s.Layout(Request{Text: "Hochzeit", Font: f, Size: 16, MaxWidth: 2, Truncation: TruncateTail})
	if got, want := glyphIDs(p), idsOf(t, s, f, "…"); !slices.Equal(got, want) {
		t.Errorf("glyphs %v, want the ellipsis alone %v", got, want)
	}
	if !p.IsOverflowing() {
		t.Error("an ellipsis wider than the limit must report its overflow")
	}
}
