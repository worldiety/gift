package typstview

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/worldiety/gift/typst"
)

// index lists the glyphs of the pages that show the source, with their
// source carried over to the offsets of the source as it is now: the pages
// may have been set before the last edits, and a caret, a click or a
// deletion has to land where those edits left the text.
type index struct {
	glyphs []glyph // by page, line and x
	lines  []line  // by page and baseline
	stops  []int   // every offset a caret can stand at, ascending

	// spans is the text the pages show, as the source of each glyph, and
	// the text typed since they were set, which they will show once they
	// are set again. Deleting goes by them: a backspace right after a
	// keystroke deletes the character typed, not the one the stale page
	// shows before it.
	spans [][2]int
}

// glyph is one glyph of a page that was set from the source.
type glyph struct {
	page                   int
	x, adv, top, bot, base float32
	start, end             int // the source of the glyph, end > start
	line                   int
}

// line is the glyphs of one baseline of one page.
type line struct {
	page        int
	base        float32
	first, last int // glyphs[first:last]
}

// idx returns the index of the pages, building it after an edit or a new
// set of pages.
func (d *Document) idx() *index {
	d.poll()
	if d.index != nil {
		return d.index
	}
	ix := &index{}
	d.index = ix
	if d.pages == nil {
		return ix
	}
	for pi := range d.pages.Pages {
		p := &d.pages.Pages[pi]
		for _, it := range p.Items {
			if it.Kind != typst.ItemText {
				continue
			}
			for _, g := range p.Glyphs[it.First : it.First+it.Count] {
				if g.Source < 0 {
					continue
				}
				s, e := sourceSpan(d.compiledSource, g.Source, g.Source+g.Len)
				for _, ed := range d.edits {
					s, e, _ = carry(s, e, ed)
				}
				if e <= s || e > len(d.src) {
					continue
				}
				ix.glyphs = append(ix.glyphs, glyph{
					page: pi, x: g.X, adv: g.Advance, base: g.Y,
					top: g.Y - it.Size*0.8, bot: g.Y + it.Size*0.25,
					start: s, end: e,
				})
			}
		}
	}
	// A baseline is a line; glyphs a fraction of a point apart, a
	// superscript aside, share it.
	key := func(g *glyph) float32 { return float32(math.Round(float64(g.base) * 2)) }
	sort.SliceStable(ix.glyphs, func(i, j int) bool {
		a, b := &ix.glyphs[i], &ix.glyphs[j]
		if a.page != b.page {
			return a.page < b.page
		}
		if ka, kb := key(a), key(b); ka != kb {
			return ka < kb
		}
		return a.x < b.x
	})
	for i := range ix.glyphs {
		g := &ix.glyphs[i]
		if n := len(ix.lines); n == 0 || ix.lines[n-1].page != g.page || key(&ix.glyphs[ix.lines[n-1].first]) != key(g) {
			ix.lines = append(ix.lines, line{page: g.page, base: g.base, first: i})
		}
		g.line = len(ix.lines) - 1
		ix.lines[g.line].last = i + 1
		ix.stops = append(ix.stops, g.start, g.end)
	}
	// The text typed since the pages, carried over the edits after it.
	var typed [][2]int
	for _, ed := range d.edits {
		for i := range typed {
			var rest [2]int
			typed[i][0], typed[i][1], rest = carry(typed[i][0], typed[i][1], ed)
			if rest[0] < rest[1] {
				typed = append(typed, rest)
			}
		}
		if ed.typed && ed.ins > 0 {
			typed = append(typed, [2]int{ed.at, ed.at + ed.ins})
		}
	}
	for i := range ix.glyphs {
		ix.spans = append(ix.spans, [2]int{ix.glyphs[i].start, ix.glyphs[i].end})
	}
	for _, t := range typed {
		if t[0] < t[1] {
			ix.spans = append(ix.spans, t)
		}
	}
	sort.Slice(ix.spans, func(i, j int) bool { return ix.spans[i][0] < ix.spans[j][0] })
	sort.Ints(ix.stops)
	k := 0
	for i, s := range ix.stops {
		if i == 0 || s != ix.stops[k-1] {
			ix.stops[k] = s
			k++
		}
	}
	ix.stops = ix.stops[:k]
	return ix
}

// sourceSpan is the source s..e of a glyph, made to fit the source: Typst
// gives the length of the text the glyph shows, which for an escape, a smart
// quote or a dash is not the length of its source.
func sourceSpan(src string, s, e int) (int, int) {
	if s < 0 || s >= len(src) {
		return s, e
	}
	switch {
	case src[s] == '\\':
		return escapeSpan(src, s, e)
	case src[s] == '"' || src[s] == '\'' || src[s] == '~':
		return s, s + 1
	case strings.HasPrefix(src[s:], "---"):
		return s, s + 3
	case strings.HasPrefix(src[s:], "--"):
		return s, s + 2
	case src[s] == '-' && s+1 < len(src) && src[s+1] >= '0' && src[s+1] <= '9':
		return s, s + 1 // a minus sign
	}
	// Text never runs into markup.
	for k := s + 1; k < min(e, len(src)); k++ {
		if strings.IndexByte("\\#[]*_$`<>@~\"'\n", src[k]) >= 0 {
			return s, k
		}
	}
	return s, min(e, len(src))
}

// escapeSpan widens the source s..e of a glyph that Typst points at the
// backslash of an escape, such as \#, to the whole escape: deleting the
// glyph deletes both, and typing after it goes after both.
func escapeSpan(src string, s, e int) (int, int) {
	if s < 0 || s+1 >= len(src) || src[s] != '\\' || e > s+1 {
		return s, e
	}
	if strings.HasPrefix(src[s:], `\u{`) {
		if i := strings.IndexByte(src[s:], '}'); i > 0 {
			return s, s + i + 1
		}
	}
	_, n := utf8.DecodeRuneInString(src[s+1:])
	return s, s + 1 + n
}

// carry moves the source s..e of a glyph, or of typed text, over an edit.
// Text typed where it starts goes before it, text typed where it ends after
// it, and a glyph whose text was deleted comes out empty. Markup an edit puts
// inside it, the brackets that make part of typed text strong say, cuts it
// in two: s..e is the part before, rest the part after.
func carry(s, e int, ed edit) (int, int, [2]int) {
	lo, hi := ed.at, ed.at+ed.del
	var rest [2]int
	if !ed.typed && ed.ins > 0 {
		switch {
		case e <= lo:
			return s, e, rest
		case s >= lo:
			ns, ne := lo+ed.ins, lo+ed.ins
			if s >= hi {
				ns = s + ed.ins - ed.del
			}
			if e >= hi {
				ne = e + ed.ins - ed.del
			}
			return ns, max(ns, ne), rest
		}
		if e > hi {
			rest = [2]int{lo + ed.ins, e + ed.ins - ed.del}
		}
		return s, lo, rest
	}
	switch {
	case s >= hi:
		s += ed.ins - ed.del
	case s > lo:
		s = lo + ed.ins
	}
	switch {
	case e <= lo:
	case e >= hi:
		e += ed.ins - ed.del
	default:
		e = lo
	}
	return s, e, rest
}

// caret is where the caret for off is drawn: at the left edge of the glyph
// that starts there or the right edge of the one that ends there. Between
// glyphs, inside markup, or in text typed since the pages were set, it is at
// the right edge of the last glyph before off. It reports false without
// glyphs.
func (ix *index) caret(off int) (glyph, float32, bool) {
	var best *glyph
	for i := range ix.glyphs {
		g := &ix.glyphs[i]
		if g.start == off {
			return *g, g.x, true
		}
		if g.end <= off && (best == nil || g.end > best.end) {
			best = g
		}
	}
	if best == nil {
		if len(ix.glyphs) == 0 {
			return glyph{}, 0, false
		}
		return ix.glyphs[0], ix.glyphs[0].x, true
	}
	return *best, best.x + best.adv, true
}

// at is the offset a click at x, y on page lands on: in the nearest line, at
// the nearer edge of the nearest glyph.
func (ix *index) at(page int, x, y float32) (int, bool) {
	best, bestD := -1, float32(math.Inf(1))
	for li := range ix.lines {
		ln := &ix.lines[li]
		if ln.page != page {
			continue
		}
		g := &ix.glyphs[ln.first]
		var d float32
		switch {
		case y < g.top:
			d = g.top - y
		case y > g.bot:
			d = y - g.bot
		}
		if d < bestD {
			best, bestD = li, d
		}
	}
	if best < 0 {
		return 0, false
	}
	return ix.inLine(best, x), true
}

// inLine is the offset at x in line li, by the midpoint of the glyph under
// it.
func (ix *index) inLine(li int, x float32) int {
	ln := &ix.lines[li]
	for i := ln.first; i < ln.last; i++ {
		g := &ix.glyphs[i]
		if x < g.x+g.adv/2 {
			return g.start
		}
	}
	return ix.glyphs[ln.last-1].end
}

// step is the next offset a caret can stand at after off, or before it.
func (ix *index) step(off int, forward bool) int {
	i := sort.SearchInts(ix.stops, off)
	if forward {
		if i < len(ix.stops) && ix.stops[i] == off {
			i++
		}
		if i < len(ix.stops) {
			return ix.stops[i]
		}
		return off
	}
	if i > 0 {
		return ix.stops[i-1]
	}
	return off
}

// before is the shown text that ends last at or before off, cut at off,
// which is what backspace deletes from.
func (ix *index) before(off int) ([2]int, bool) {
	best, ok := [2]int{}, false
	for _, s := range ix.spans {
		if s[0] >= off {
			break
		}
		if e := min(s[1], off); !ok || e > best[1] {
			best, ok = [2]int{s[0], e}, true
		}
	}
	return best, ok
}

// after is the shown text that starts first at or after off, cut at off,
// which is what delete deletes from.
func (ix *index) after(off int) ([2]int, bool) {
	best, ok := [2]int{}, false
	for _, s := range ix.spans {
		if s[1] <= off {
			continue
		}
		if b := max(s[0], off); !ok || b < best[0] {
			best, ok = [2]int{b, s[1]}, true
		}
	}
	return best, ok
}

// covers reports whether the pages show, or will show, the byte at off.
func (ix *index) covers(off int) bool {
	for _, s := range ix.spans {
		if s[0] <= off && off < s[1] {
			return true
		}
	}
	return false
}

// shown are the stretches of lo..hi the pages show as text, or will show,
// merged where they touch, in ascending order.
func (ix *index) shown(lo, hi int) [][2]int {
	var out [][2]int
	for _, s := range ix.spans {
		a, b := max(s[0], lo), min(s[1], hi)
		if a >= b {
			continue
		}
		if n := len(out); n > 0 && a <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], b)
			continue
		}
		out = append(out, [2]int{a, b})
	}
	return out
}
