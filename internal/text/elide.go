package text

import (
	"math"
	"unicode/utf8"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"github.com/worldiety/gift/geom"
)

// This file holds the line limit and the ellipsis of [Request.MaxLines] and
// [Request.Truncation]. It runs inside [Shaper.build], after every source
// paragraph was shaped and broken and before the ranges are materialised, so
// that it works on the temporary ranges alone and nothing points into the
// glyph array yet.
//
// # Why the last line is shaped a second time
//
// The glyphs of a wrapped line end where the line wrapper decided to break,
// and an ellipsis wants the text *after* that break: "Hochzeit Anna &" is the
// line, "Hochzeit Anna & B…" is what a reader expects to see. Those characters
// are on the next line, shaped with a different line start, or not shaped at
// all because MaxLines cut the paragraph off. So the rest of the text is
// shaped again as one unbroken run, and the elided line is cut out of that.
//
// That is a second shaping call per truncated miss and none per hit, which is
// the allocation boundary of the package documentation unchanged: a cache hit
// on a truncated paragraph is the same map lookup as on any other.
//
// # Where the cut may fall
//
// Only between clusters. A cluster is the unit harfbuzz will not split — a
// base letter and its combining marks, the glyphs of one ligature — and a cut
// inside one would leave an accent without its letter. The glyphs on either
// side keep the advances and offsets they were shaped with; the kerning pair
// that spans the cut is lost, which is a fraction of a pixel next to an
// ellipsis that is itself the loudest glyph on the line.
//
// Whitespace next to the ellipsis is dropped with it, so that the reader sees
// "Anna &…" and not "Anna & …", which reads as an ellipsis that belongs to no
// word.

// ellipsisText is what an elided line ends (or begins, or is broken) with: one
// HORIZONTAL ELLIPSIS. It is one glyph in every font that has it, and it is
// narrower than three full stops, which is the whole point on a line that has
// no room.
const ellipsisText = "…"

// ellipsisFallback is used when the font has no glyph for ellipsisText. Three
// full stops look slightly looser than the real thing, but every Latin font
// has a full stop, and a notdef box at the end of every truncated label would
// look like a rendering error rather than like a truncation.
const ellipsisFallback = "..."

// truncate applies the line limit and the truncation mode of req to the lines
// shaped so far and returns what it hid; see [Paragraph.Hidden].
//
// It is a no-op for a request that asks for neither, which is every request
// outside ui's labels, and it returns before touching anything when the text
// fits.
func (s *Shaper) truncate(e *entry, req Request, key cacheKey, size float32, m Metrics, bounded bool, maxWidth float32) geom.Size {
	if key.maxLines == 0 && key.trunc == TruncateNone {
		return geom.Size{}
	}
	total := len(s.tmpLines)
	keep := total
	if key.maxLines > 0 && keep > int(key.maxLines) {
		keep = int(key.maxLines)
	}
	dropped := total - keep
	last := keep - 1
	lr := s.tmpLines[last]
	wide := bounded && lr.width > maxWidth

	var hidden geom.Size
	hidden.H = float32(dropped) * m.LineHeight

	if key.trunc == TruncateNone || (dropped == 0 && !wide) {
		// A hard cut, or nothing to cut: drop the lines past the limit and
		// leave the last one as the line wrapper made it.
		if dropped > 0 {
			s.cutFrom(e, keep)
		}
		return hidden
	}
	if wide {
		hidden.W = lr.width - maxWidth
	}

	// The content the elided line stands for. For the tail it is the rest of
	// the source paragraph the line belongs to: a line break is the end of a
	// thought, and "a…" says "there is more" without splicing the next
	// paragraph onto this one. For the middle and the head it has to be the
	// rest of the whole text, because the part that is kept is its end.
	start := lr.byteStart
	end := len(req.Text)
	if key.trunc == TruncateTail {
		end = s.paragraphEnd(start)
	}
	// Everything from this line on is replaced by one new line.
	s.cutFrom(e, last)
	// Only the tail needs the ellipsis forced: its content stops at the end
	// of a paragraph, so dropped lines can be text the content does not
	// include. The content of the middle and the head runs to the end of the
	// text and includes every dropped line.
	forced := dropped > 0 && key.trunc == TruncateTail
	s.elide(e, req, key.trunc, size, start, end, forced, bounded, maxWidth)
	return hidden
}

// cutFrom removes line n and every line after it from the temporary ranges,
// together with their runs and glyphs.
//
// The glyphs of a line are contiguous and in line order, so the cut is a
// reslice. The first glyph of line n is the first glyph of its first run; a
// line with no runs — an empty source paragraph — has appended no glyphs,
// so its glyphs start wherever the next run does, and with no next run at the
// end of the array.
func (s *Shaper) cutFrom(e *entry, n int) {
	if n >= len(s.tmpLines) {
		return
	}
	rs := s.tmpLines[n].runStart
	g := len(e.glyphs)
	if rs < len(s.tmpRuns) {
		g = s.tmpRuns[rs].start
	}
	e.glyphs = e.glyphs[:g]
	s.tmpRuns = s.tmpRuns[:rs]
	s.tmpLines = s.tmpLines[:n]
}

// paragraphEnd returns the end, in bytes of Request.Text and without the
// break, of the source paragraph that contains the byte offset at.
func (s *Shaper) paragraphEnd(at int) int {
	for _, p := range s.paras {
		if end := p.start + len(p.text); at >= p.start && at <= end {
			return end
		}
	}
	// Not reachable for an offset that came from a line of this request.
	return at
}

// elide shapes Request.Text[start:end] as one line, shortens it with an
// ellipsis under mode, and appends it as the last line of the temporary
// ranges.
//
// forced is true when there is text after end that no line shows — lines were
// dropped after a tail truncated paragraph — and the ellipsis must be there
// even if the content itself fits.
// Without it, content that fits is shown whole and without an ellipsis, which
// is what a middle or head truncation of a text with a mandatory break comes
// to when the width is unbounded.
func (s *Shaper) elide(e *entry, req Request, mode Truncation, size float32, start, end int, forced, bounded bool, maxWidth float32) {
	ell := s.shapeEllipsis(req.Font, size)
	var ellW float32
	for _, g := range ell {
		ellW += f26(g.Advance)
	}

	glyphs := s.shapeRun(req.Font, size, req.Text[start:end])
	n := len(glyphs)

	var total float32
	for i := range glyphs {
		total += f26(glyphs[i].Advance)
	}
	// avail is what the kept glyphs may take next to the ellipsis. It may be
	// negative: an ellipsis wider than the limit is still drawn, alone, and
	// overflows like any other word that does not fit.
	avail := float32(math.Inf(1))
	if bounded {
		avail = maxWidth - ellW
	}

	// [lo, hi) is the part that is *removed*; the line is glyphs[:lo], the
	// ellipsis, glyphs[hi:]. An empty range with no ellipsis is the content
	// unchanged.
	lo, hi, withEllipsis := n, n, true
	switch {
	case !forced && (!bounded || total <= maxWidth):
		withEllipsis = false
	case forced && total <= avail:
		// Everything fits next to the ellipsis: a paragraph that ends on
		// this line, followed by paragraphs that were dropped.
		lo = trimBlankBefore(glyphs, n)
	case mode == TruncateHead:
		lo = 0
		hi = trimBlankAfter(glyphs, suffixFrom(glyphs, 0, total, avail))
	case mode == TruncateMiddle:
		lo = prefixTo(glyphs, avail/2)
		var kept float32
		for i := range lo {
			kept += f26(glyphs[i].Advance)
		}
		hi = trimBlankAfter(glyphs, suffixFrom(glyphs, lo, total, avail-kept))
		lo = trimBlankBefore(glyphs, lo)
	default: // TruncateTail
		lo = trimBlankBefore(glyphs, prefixTo(glyphs, avail))
	}

	// The cluster the ellipsis stands for: the first byte it replaces.
	cut := end
	if lo < n {
		cut = start + s.byteAt(glyphs[lo].TextIndex())
	}

	runStart := len(s.tmpRuns)
	glyphStart := len(e.glyphs)
	var pen float32
	put := func(g *shaping.Glyph, cluster int) {
		adv := f26(g.Advance)
		e.glyphs = append(e.glyphs, Glyph{
			ID:      GlyphID(g.GlyphID),
			X:       round(pen + f26(g.XOffset)),
			Y:       round(-f26(g.YOffset)),
			Advance: adv,
			Cluster: int32(cluster),
		})
		pen += adv
	}
	for i := range lo {
		put(&glyphs[i], start+s.byteAt(glyphs[i].TextIndex()))
	}
	if withEllipsis {
		for i := range ell {
			put(&ell[i], cut)
		}
	}
	for i := hi; i < n; i++ {
		put(&glyphs[i], start+s.byteAt(glyphs[i].TextIndex()))
	}

	s.tmpRuns = append(s.tmpRuns, runRange{start: glyphStart, end: len(e.glyphs), advance: pen})
	s.tmpLines = append(s.tmpLines, lineRange{
		runStart: runStart,
		runEnd:   len(s.tmpRuns),
		width:    pen,
		// No trailing whitespace survives next to an ellipsis, and a line
		// without one ends in whatever the content ended in, which the
		// caret mapping of a label never asks about.
		advance:   pen,
		byteStart: start,
		byteEnd:   len(req.Text),
	})
}

// shapeEllipsis shapes the ellipsis in f at size and returns its glyphs, or the
// glyphs of [ellipsisFallback] when f has no glyph for [ellipsisText].
func (s *Shaper) shapeEllipsis(f *Font, size float32) []shaping.Glyph {
	g := s.shapeRun(f, size, ellipsisText)
	for i := range g {
		if g[i].GlyphID == 0 {
			return s.shapeRun(f, size, ellipsisFallback)
		}
	}
	return g
}

// shapeRun shapes text as one unbroken left to right run and returns the
// glyphs, with s.runeBytes mapping their rune indices back to byte offsets
// inside text.
//
// A mandatory break inside text becomes a space. It can only get here for a
// middle or head truncation, whose content spans source paragraphs; one line
// cannot show a break, and joining the two paragraphs without anything between
// them would glue the last word of one to the first word of the next.
//
// The glyph slice is freshly allocated by the shaper on every call and is not
// scratch: the ellipsis and the content are shaped by two calls and both
// results are read afterwards.
func (s *Shaper) shapeRun(f *Font, size float32, text string) []shaping.Glyph {
	s.runes = s.runes[:0]
	s.runeBytes = s.runeBytes[:0]
	for i := 0; i < len(text); {
		if n := breakLen(text, i); n > 0 {
			s.runes = append(s.runes, ' ')
			s.runeBytes = append(s.runeBytes, int32(i))
			i += n
			continue
		}
		r, n := utf8.DecodeRuneInString(text[i:])
		s.runes = append(s.runes, r)
		s.runeBytes = append(s.runeBytes, int32(i))
		i += n
	}
	s.runeBytes = append(s.runeBytes, int32(len(text)))
	if len(s.runes) == 0 {
		return nil
	}
	out := s.hb.Shape(shaping.Input{
		Text:      s.runes,
		RunStart:  0,
		RunEnd:    len(s.runes),
		Direction: di.DirectionLTR,
		Face:      f.face,
		Size:      fixed.Int26_6(math.Round(float64(size) * 64)),
		Script:    scriptOf(s.runes),
		Language:  language.DefaultLanguage(),
	})
	return out.Glyphs
}

// prefixTo returns the largest cluster boundary k such that glyphs[:k] is at
// most w wide.
func prefixTo(glyphs []shaping.Glyph, w float32) int {
	var x float32
	k := 0
	for i := range glyphs {
		if i > 0 && glyphs[i].ClusterIndex != glyphs[i-1].ClusterIndex {
			k = i
		}
		x += f26(glyphs[i].Advance)
		if x > w {
			return k
		}
	}
	return len(glyphs)
}

// suffixFrom returns the smallest cluster boundary j of at least from such
// that glyphs[j:] is at most w wide, given that all of glyphs is total wide.
func suffixFrom(glyphs []shaping.Glyph, from int, total, w float32) int {
	x := total
	for j := range from {
		x -= f26(glyphs[j].Advance)
	}
	for j := from; j < len(glyphs); j++ {
		if x <= w && (j == 0 || glyphs[j].ClusterIndex != glyphs[j-1].ClusterIndex) {
			return j
		}
		x -= f26(glyphs[j].Advance)
	}
	return len(glyphs)
}

// trimBlankBefore moves the end k of a kept prefix back over whitespace, so
// that no space stands between the last kept word and the ellipsis. A glyph
// without ink is whitespace here, the same rule [Shaper.emitLine] trims by.
func trimBlankBefore(glyphs []shaping.Glyph, k int) int {
	for k > 0 && glyphs[k-1].Width == 0 {
		k--
	}
	return k
}

// trimBlankAfter moves the start j of a kept suffix forward over whitespace.
func trimBlankAfter(glyphs []shaping.Glyph, j int) int {
	for j < len(glyphs) && glyphs[j].Width == 0 {
		j++
	}
	return j
}
