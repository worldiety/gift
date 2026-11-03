package typstview

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/syntax"
)

// The editing of a document, as the pages of an [Editor] do it and as a
// program does it with the same methods, from a toolbar of its own say.
//
// Every edit is an edit of the source, made so that the source stays a
// document Typst sets: typed characters Typst would read as markup are
// escaped, a deletion removes what the pages show and keeps the markup around
// it, and formatting wraps text in markup or takes the markup away again.
//
// What an edit leaves next to each other is checked where the two meet, at
// the seam: a character that its new neighbour would make markup, a slash
// after a slash or a parenthesis after a call, is escaped, strong text in
// stars that a letter now touches becomes a call, and text after a set rule
// goes to a line of its own. Strong and emphasised text is written as a call,
// #strong[…], which holds inside a word and across paragraphs, where stars
// would not.

// edited sets the source after an edit made through the document and
// publishes it.
func (d *Document) edited() {
	d.flush()
	if d.onChange != nil {
		d.onChange(d.src)
	}
}

// lineBreaks are the line breaks of Unicode, which Typst knows too, as the
// line break of the source.
var lineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n", " ", "\n", " ", "\n\n", "\u0085", "\n", "\v", "\n", "\f", "\n")

// paragraphBreaks are the blank lines between the paragraphs of a text.
var paragraphBreaks = regexp.MustCompile(`\n[ \t]*\n\s*`)

// Insert types s at the caret, replacing what the selection shows, as typing
// on the page does: characters Typst would read as markup are escaped, so
// that the page shows what was typed. A line break in s is a line break of
// the source, which Typst sets as a space, and a blank line breaks the
// paragraph as enter does.
func (d *Document) Insert(s string) {
	s = lineBreaks.Replace(s)
	if s == "" {
		return
	}
	if lo, hi := d.Selection(); lo != hi {
		d.deleteShown(lo, hi)
	}
	for i, part := range paragraphBreaks.Split(s, -1) {
		if i > 0 {
			d.breakParagraph()
		}
		if part != "" {
			d.insertText(part)
		}
	}
	d.edited()
}

// insertText types s, which holds no blank line, at the caret.
func (d *Document) insertText(s string) {
	for {
		at := d.caret
		if at > 0 && isEscaped(d.src, at) {
			// Not between a backslash and what it escapes.
			at--
		}
		esc := escape(d.src, parse(d.src), at, s)
		if d.loosen(at, at, esc) {
			continue
		}
		d.typing = true
		d.replace(at, at, esc, true)
		d.typing = false
		d.mend(at, d.caret)
		return
	}
}

// escape makes s safe to insert into src at at.
func escape(src string, st *structure, at int, s string) string {
	switch contextAt(src, at) {
	case ui.TokenString:
		r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
		return r.Replace(s)
	case ui.TokenRaw:
		return strings.ReplaceAll(s, "`", "'")
	}
	var b strings.Builder
	prev, _ := utf8.DecodeLastRuneInString(src[:at])
	plain := at > 0 && !isEscaped(src, at-1) // prev is markup, not escaped
	start := atStart(src, st, at)            // nothing but white space since the start of a line
	num := false                             // and then nothing but digits
	if q := strings.TrimRight(src[:at], "0123456789"); len(q) < at && atStart(src, st, len(q)) {
		num = true
	}
	for _, r := range s {
		esc := true
		switch {
		case strings.ContainsRune(`\#$*_`+"`"+`@<>[]~`, r):
		// These are markup only at the start of a line, a heading, a list
		// item or a term, or doubled, a dash or a comment.
		case strings.ContainsRune(`=-+/`, r) && (start || plain && prev == r):
		// A slash after a star ends a comment, a question mark after a dash
		// is a soft hyphen, and a dot after a number at the start of a line
		// numbers it.
		case r == '/' && plain && prev == '*', r == '?' && plain && prev == '-', r == '.' && num:
		default:
			esc = false
		}
		if esc {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
		switch {
		case r == '\n':
			start, num = true, false
		case r == ' ' || r == '\t':
			num = false
		case r >= '0' && r <= '9':
			num, start = start || num, false
		default:
			start, num = false, false
		}
		prev, plain = r, !esc
	}
	return b.String()
}

func lineStartOf(src string, at int) int { return strings.LastIndexByte(src[:at], '\n') + 1 }

// contextAt is the kind of token at at: a string or raw text in which
// characters are escaped differently, or anything else.
func contextAt(src string, at int) ui.TokenKind {
	for _, t := range syntax.Typst.Highlight(src, nil) {
		if t.Start < at && at < t.End {
			return t.Kind
		}
		if t.Start >= at {
			break
		}
	}
	return ui.TokenPlain
}

// mend checks the seams at the given offsets, where an edit put two pieces
// of the source next to each other, the last first, and repairs what Typst
// would read differently there; see the top of this file.
func (d *Document) mend(points ...int) {
	saved := d.track
	d.track = append([]int(nil), points...)
	for n := 0; len(d.track) > 0 && n < 64; n++ {
		p := d.track[len(d.track)-1]
		d.track = d.track[:len(d.track)-1]
		d.mendAt(p)
	}
	d.track = saved
}

// mendAt repairs the seam at p, between what stands before p and what
// stands at it.
func (d *Document) mendAt(p int) {
	src := d.src
	if p <= 0 || p >= len(src) {
		if p == 0 && len(src) > 0 {
			if h := startHazard(src); h >= 0 && !isEscaped(src, h) {
				d.replace(h, h, `\`, false)
			}
		}
		return
	}
	st := parse(src)
	a, _ := utf8.DecodeLastRuneInString(src[:p])
	b, _ := utf8.DecodeRuneInString(src[p:])
	plain := !isEscaped(src, p-1) && !isEscaped(src, p)
	// A slash after a slash or a star starts or ends a comment, and a
	// question mark after a dash is a soft hyphen. These come first: the
	// comment they make would hide the seam.
	if (b == '/' && (a == '/' || a == '*') || b == '?' && a == '-') && plain {
		if k := contextAt(src, p-1); k != ui.TokenString && k != ui.TokenRaw {
			d.replace(p, p, `\`, false)
		}
		return
	}
	// What ends at p, read without what follows it: a set rule, which
	// wants a line of its own, or an expression, which a parenthesis, a
	// bracket or a field would continue. Anywhere else the seam has to lie
	// in markup.
	pre := parse(src[:p])
	ended := strings.IndexByte(")]}\"", src[p-1]) >= 0 && (pre.isEnd(p) || pre.isStmt(p))
	if !ended && (!st.inMarkup(p) || st.inCode(p) || contextAt(src, p) == ui.TokenString) {
		return
	}
	if ended && pre.isStmt(p) {
		if src[p] != '\n' && src[p] != ';' {
			d.replace(p, p, "\n", false)
			d.track = append(d.track, p+1) // which starts a line now
		}
		return
	}
	if isEscaped(src, p) || src[p] == '\\' {
		return
	}
	switch {
	case (b == '(' || b == '[' || b == '.') && ended:
		d.replace(p, p, `\`, false)
	case atStart(src, st, p):
		if h := startHazard(src[p:]); h >= 0 && !isEscaped(src, p+h) {
			d.replace(p+h, p+h, `\`, false)
			return
		}
	}
	// The characters of a marker that end at p, where a line starts: what
	// follows them now, a line break or a space, can make them a marker, a
	// slash before enter, or "#strong[ /" after a deletion.
	q := p
	switch c := src[p-1]; {
	case c == '=':
		for q > 0 && src[q-1] == '=' {
			q--
		}
	case c == '-' || c == '+' || c == '/':
		q--
	case c == '.':
		k := q - 1
		for k > 0 && src[k-1] >= '0' && src[k-1] <= '9' {
			k--
		}
		if k < q-1 {
			q = k
		}
	}
	if q < p && !isEscaped(src, q) && atStart(src, st, q) {
		if h := startHazard(src[q:]); h >= 0 && q+h < p && !isEscaped(src, q+h) {
			d.replace(q+h, q+h, `\`, false)
		}
	}
}

// loosen writes strong or emphasised text in stars or underscores as a call
// where replacing lo..hi by ins would put a delimiter of it between two
// letters, where Typst reads it as text, or after a slash, where it starts a
// comment. It reports whether it changed the source; the edit is then to be
// worked out again.
func (d *Document) loosen(lo, hi int, ins string) bool {
	next := d.src[:lo] + ins + d.src[hi:]
	return d.loosenTo(next, func(q int) (int, bool) {
		switch {
		case q < lo:
			return q, true
		case q >= hi:
			return q - (hi - lo) + len(ins), true
		}
		return 0, false
	})
}

// loosenTo is [Document.loosen] for an edit that makes the source next and
// moves the offset q to moved(q), or deletes it.
func (d *Document) loosenTo(next string, moved func(q int) (int, bool)) bool {
	for _, g := range parse(d.src).groups {
		if g.kind != delimGroup {
			continue
		}
		for _, q := range []int{g.open, g.end} {
			n, ok := moved(q)
			if ok && (inWord(next, n) || q == g.open && n > 0 && next[n-1] == '/' && !isEscaped(next, n-1)) {
				d.callForm(g)
				return true
			}
		}
	}
	return false
}

// callForm writes the strong or emphasised text g, in delimiters, as a call.
func (d *Document) callForm(g group) {
	fn := "strong"
	if g.fn == "_" {
		fn = "emph"
	}
	d.replace(g.end, g.close, "]", false)
	d.replace(g.open, g.start, "#"+fn+"[", false)
}

// NewParagraph starts a new paragraph at the caret, as enter does on the
// page. In a list it starts the next item, and in an item without text it
// ends the list. Strong and emphasised text around the caret is closed
// before the break and opened again after it.
func (d *Document) NewParagraph() {
	if lo, hi := d.Selection(); lo != hi {
		d.deleteShown(lo, hi)
	}
	d.breakParagraph()
	d.edited()
}

// breakParagraph is [Document.NewParagraph] without the selection.
func (d *Document) breakParagraph() {
	src, at := d.src, d.caret
	if at > 0 && isEscaped(src, at) {
		at--
	}
	st := parse(src)
	encl := st.enclosing(at)
	var closing, opening string
	for k := len(encl) - 1; k >= 0 && encl[k].inline(); k-- {
		closing += encl[k].suffix(src)
		opening = encl[k].prefix(src) + opening
	}
	brk := "\n\n"
	bs, fresh := blockStart(src, st, at)
	if m := marker(src[bs:]); fresh && (m == "- " || m == "+ ") {
		le := lineEnd(src, at)
		if c := st.container(at); c != nil && c.end < le {
			le = c.end
		}
		whole := true // the line holds every group it touches
		for _, g := range st.groups {
			if g.open < bs && bs < g.close && g.close <= le || bs <= g.open && g.open < le && g.close > le {
				whole = false
			}
		}
		if whole && len(d.shown(bs+len(m), le)) == 0 {
			// A blank line ends the list, and what is typed next is a
			// paragraph of its own.
			d.replace(bs, le, "\n", true)
			d.mend(d.caret)
			return
		}
		brk = "\n" + m
	}
	d.replace(at, at, closing+brk+opening, true)
	d.mend(at, d.caret)
}

// marker is the heading or list marker a line starts with, with the space
// after it, or empty.
func marker(line string) string { return line[:markerLen(line)] }

// DeleteBackward deletes what the selection shows, or the character the page
// shows before the caret, as backspace does on the page. At the start of a
// heading or a list item it turns the line into a plain paragraph, and at
// the start of a paragraph it joins it to the one before. At the start of a
// table cell it does nothing.
func (d *Document) DeleteBackward() {
	if lo, hi := d.Selection(); lo != hi {
		d.deleteShown(lo, hi)
		d.edited()
		return
	}
	src, at := d.src, d.caret
	st := parse(src)
	bs, fresh := blockStart(src, st, at)
	if m := marker(src[bs:]); fresh && m != "" && bs+len(m) <= at && len(d.shown(bs+len(m), at)) == 0 {
		d.replace(bs, bs+len(m), "", false)
		d.mend(bs)
		d.edited()
		return
	}
	g, ok := d.idx().before(at)
	if !ok || !plainGap(src, st, g[1], at) {
		return
	}
	lo, hi := lineBreakIn(src, st, g[1], at)
	if lo == hi {
		_, n := utf8.DecodeLastRuneInString(src[g[0]:g[1]])
		lo, hi = unitAt(src, g[1]-n)
	}
	if d.loosen(lo, hi, "") {
		d.DeleteBackward()
		return
	}
	d.replace(lo, hi, "", false)
	d.mend(lo)
	d.edited()
}

// DeleteForward deletes what the selection shows, or the character the page
// shows after the caret, as delete does on the page. At the end of a
// paragraph it joins the next one to it.
func (d *Document) DeleteForward() {
	if lo, hi := d.Selection(); lo != hi {
		d.deleteShown(lo, hi)
		d.edited()
		return
	}
	src, at := d.src, d.caret
	st := parse(src)
	g, ok := d.idx().after(at)
	if !ok || !plainGap(src, st, at, g[0]) {
		return
	}
	lo, hi := lineBreakIn(src, st, at, g[0])
	if lo == hi {
		lo, hi = unitAt(src, g[0])
	}
	if d.loosen(lo, hi, "") {
		d.DeleteForward()
		return
	}
	d.replace(lo, hi, "", false)
	d.caret, d.anchor = min(at, lo), min(at, lo)
	d.mend(lo)
	d.edited()
}

// lineBreakIn is the white space with a line break in it between lo and hi,
// the gap between two pieces of shown text, with the heading or list marker
// that follows it: deleting it joins the two lines. It is empty where the gap
// holds none.
func lineBreakIn(src string, st *structure, lo, hi int) (int, int) {
	for i := hi - 1; i >= lo; i-- {
		if src[i] != '\n' {
			continue
		}
		s, e := i, i+1
		for s > lo && isSpace(src[s-1]) {
			s--
		}
		for e < hi && isSpace(src[e]) {
			e++
		}
		for _, m := range st.marks {
			if m[0] == e && m[1] <= hi {
				e = m[1]
			}
		}
		return s, e
	}
	return hi, hi
}

// plainGap reports whether lo..hi, between the caret and the text a key
// deletes, holds nothing but white space and the brackets of strong and
// emphasised text. Anything else, the boundary of a table cell or a set
// rule, the keys leave alone.
func plainGap(src string, st *structure, lo, hi int) bool {
next:
	for k := lo; k < hi; k++ {
		if isSpace(src[k]) {
			continue
		}
		for _, g := range st.groups {
			if g.inline() && (g.open <= k && k < g.start || g.end <= k && k < g.close) {
				continue next
			}
		}
		for _, m := range st.marks {
			if m[0] <= k && k < m[1] {
				continue next
			}
		}
		return false
	}
	return true
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// deleteShown deletes the text the pages show between lo and hi, and the
// paragraph breaks between it, and keeps the markup: deleting a selection
// that starts inside strong text and ends outside it must not delete the
// bracket that closes it. A heading or list marker whose line joins the one
// before goes too.
func (d *Document) deleteShown(lo, hi int) {
	for !d.deleteShownOnce(lo, hi) {
		lo, hi = d.Selection()
	}
}

// deleteShownOnce is [Document.deleteShown], or reports false where it
// first had to write strong text in stars as a call, which moved lo..hi.
func (d *Document) deleteShownOnce(lo, hi int) bool {
	src := d.src
	lo, hi = snap(src, lo, false), snap(src, hi, true)
	st := parse(src)
	ix := d.idx()
	stretches := d.shown(lo, hi)
	if len(stretches) == 0 {
		// Nothing the pages show, only markup, which stays.
		d.caret, d.anchor = hi, hi
		d.changed()
		return true
	}
	lo, hi = min(lo, stretches[0][0]), max(hi, stretches[len(stretches)-1][1])
	gone := make([]bool, hi-lo)
	for _, r := range stretches {
		for i := r[0]; i < r[1]; i++ {
			gone[i-lo] = true
		}
	}
	shown := func(k int) bool { return k >= lo && k < hi && gone[k-lo] || ix.covers(k) }
	transparent := func(k int) bool {
		for _, g := range st.groups {
			if g.inline() && (g.open <= k && k < g.start || g.end <= k && k < g.close) {
				return true
			}
		}
		for _, m := range st.marks {
			if m[0] <= k && k < m[1] {
				return true
			}
		}
		return false
	}
	// White space with a line break in it, between shown text, is a
	// paragraph or a line break.
	for i := lo; i < hi; {
		if gone[i-lo] || !isSpace(src[i]) {
			i++
			continue
		}
		j := i
		for j < hi && !gone[j-lo] && isSpace(src[j]) {
			j++
		}
		before, after := i-1, j
		for before >= 0 && transparent(before) {
			before--
		}
		for after < len(src) && transparent(after) {
			after++
		}
		if strings.Contains(src[i:j], "\n") && before >= 0 && shown(before) && after < len(src) && shown(after) {
			for k := i; k < j; k++ {
				gone[k-lo] = true
			}
			for _, m := range st.marks {
				if m[0] == j && m[1] <= hi {
					for k := m[0]; k < m[1]; k++ {
						gone[k-lo] = true
					}
				}
			}
		}
		i = j
	}
	var kept strings.Builder
	var seams []int
	caret := lo
	moved := make([]int, hi-lo)
	for i := lo; i < hi; i++ {
		if i == stretches[0][0] {
			caret = lo + kept.Len()
		}
		moved[i-lo] = lo + kept.Len()
		if gone[i-lo] {
			if i == lo || !gone[i-1-lo] {
				seams = append(seams, lo+kept.Len())
			}
			continue
		}
		kept.WriteByte(src[i])
	}
	next := src[:lo] + kept.String() + src[hi:]
	if d.loosenTo(next, func(q int) (int, bool) {
		switch {
		case q < lo:
			return q, true
		case q >= hi:
			return q - (hi - lo) + kept.Len(), true
		}
		return moved[q-lo], !gone[q-lo]
	}) {
		return false
	}
	d.replace(lo, hi, kept.String(), false)
	d.caret, d.anchor = caret, caret
	d.mend(seams...)
	return true
}

// shown is [index.shown] with every stretch widened to whole escapes: pages
// set some edits ago may cut one in two, and half an escape is a backslash
// that escapes something else.
func (d *Document) shown(lo, hi int) [][2]int {
	src := d.src
	var out [][2]int
	for _, r := range d.idx().shown(lo, hi) {
		if u, _ := unitAt(src, r[0]); u < r[0] {
			r[0] = u
		}
		if r[1] > 0 && r[1] <= len(src) {
			if _, u := unitAt(src, r[1]-1); u > r[1] {
				r[1] = u
			}
		}
		if n := len(out); n > 0 && r[0] <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], r[1])
			continue
		}
		out = append(out, r)
	}
	return out
}

// SelectedText returns the text the selection shows, without the markup.
func (d *Document) SelectedText() string {
	lo, hi := d.Selection()
	var b strings.Builder
	for _, r := range d.shown(lo, hi) {
		if b.Len() > 0 && strings.Contains(d.src[lo:r[0]], "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(d.src[r[0]:r[1]])
		lo = r[1]
	}
	return b.String()
}

// selectAll selects the text the pages show, from its first character to its
// last, which leaves the markup before it, a set rule say, alone.
func (d *Document) selectAll() {
	sp := d.idx().spans
	if len(sp) == 0 {
		d.SetSelection(len(d.src), len(d.src))
		return
	}
	lo, hi := sp[0][0], sp[0][1]
	for _, s := range sp {
		hi = max(hi, s[1])
	}
	d.SetSelection(lo, hi)
}

// ToggleStrong makes the selection, or the word at the caret, strong, or
// plain again if it is strong already.
func (d *Document) ToggleStrong() { d.toggle("*", "strong") }

// ToggleEmphasis makes the selection, or the word at the caret, emphasised,
// or plain again if it is emphasised already.
func (d *Document) ToggleEmphasis() { d.toggle("_", "emph") }

// toggle wraps the selection in a call of fn, or takes the call, or the
// delimiters delim, away from it.
func (d *Document) toggle(delim, fn string) {
	src := d.src
	lo, hi := d.Selection()
	if lo == hi {
		lo, hi = wordAt(src, d.caret)
	}
	lo, hi = snap(src, lo, false), snap(src, hi, true)
	st := parse(src)
	if g, ok := formatted(st, lo, hi, delim, fn); ok {
		if g.kind == delimGroup {
			d.callForm(g)
			d.toggle(delim, fn)
			return
		}
		d.unformat(st, g, lo, hi)
	} else {
		d.format(st, lo, hi, fn)
	}
	d.edited()
	d.changed()
}

// formatted is the innermost strong or emphasised text, by fn or by delim,
// around lo..hi.
func formatted(st *structure, lo, hi int, delim, fn string) (group, bool) {
	var best group
	ok := false
	for _, g := range st.groups {
		if (g.kind == callGroup && g.fn == fn || g.kind == delimGroup && g.fn == delim) && g.start <= lo && hi <= g.end {
			best, ok = g, true
		}
	}
	return best, ok
}

// unformat takes the call g away from lo..hi: the text before and after the
// selection stays formatted, unless the page shows none of it. It only
// deletes and inserts the brackets around the text, so that the pages, until
// they are set again, still find their glyphs in it.
func (d *Document) unformat(st *structure, g group, lo, hi int) {
	src := d.src
	pre, suf := g.prefix(src), g.suffix(src)
	cut := func(off int) bool { // off lies inside a group within g
		for _, h := range st.groups {
			if h.start > g.start && h.close <= g.end && h.open < off && off < h.close {
				return true
			}
		}
		return false
	}
	keepLeft := lo != hi && len(d.shown(g.start, lo)) > 0 && !cut(lo)
	keepRight := lo != hi && len(d.shown(hi, g.end)) > 0 && !cut(hi)
	a, c := g.start, g.end // the selection afterwards
	if keepLeft {
		a = lo
	}
	if keepRight {
		c = hi
		d.replace(hi, hi, pre, false)
	} else {
		d.replace(g.end, g.close, "", false)
	}
	if keepLeft {
		d.replace(lo, lo, suf, false)
		a, c = a+len(suf), c+len(suf)
	} else {
		d.replace(g.open, g.start, "", false)
		a, c = a-len(pre), c-len(pre)
	}
	d.anchor, d.caret = a, c
	seams := []int{a, c}
	if keepRight {
		seams = append(seams, c+len(pre))
	}
	d.mend(seams...)
}

// format wraps what the pages show of lo..hi in a call of fn, each stretch
// of text in a call of its own, so that no call crosses the boundary of a
// paragraph, a cell or another call. With nothing selected it puts an empty
// call at the caret, to type into.
func (d *Document) format(st *structure, lo, hi int, fn string) {
	src := d.src
	pre, suf := "#"+fn+"[", "]"
	if lo == hi {
		if lo > 0 && isEscaped(src, lo) {
			lo--
		}
		d.replace(lo, lo, pre+suf, false)
		d.caret, d.anchor = lo+len(pre), lo+len(pre)
		d.mend(lo, lo+len(pre)+len(suf))
		return
	}
	var parts [][2]int
	for _, r := range d.shown(lo, hi) {
		if _, ok := formatted(st, r[0], r[1], "", fn); !ok && !st.inCode(r[0]) {
			parts = append(parts, r)
		}
	}
	if len(parts) == 0 {
		return
	}
	for k := len(parts) - 1; k >= 0; k-- {
		d.replace(parts[k][1], parts[k][1], suf, false)
		d.replace(parts[k][0], parts[k][0], pre, false)
	}
	l := len(pre) + len(suf)
	var seams []int
	for k, r := range parts {
		seams = append(seams, r[0]+k*l+len(pre), r[1]+k*l+l)
	}
	n := len(parts) - 1
	d.anchor, d.caret = parts[0][0]+len(pre), parts[n][1]+n*l+len(pre)
	d.mend(seams...)
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// wordAt is the word around off, or the empty range at off.
func wordAt(s string, off int) (int, int) {
	lo, hi := off, off
	for lo > 0 {
		r, n := utf8.DecodeLastRuneInString(s[:lo])
		if !isWordRune(r) {
			break
		}
		lo -= n
	}
	for hi < len(s) {
		r, n := utf8.DecodeRuneInString(s[hi:])
		if !isWordRune(r) {
			break
		}
		hi += n
	}
	return lo, hi
}

// SetHeading makes the line of the caret a heading of level 1 to 6, or a
// plain paragraph for level 0.
func (d *Document) SetHeading(level int) {
	level = max(0, min(level, 6))
	d.setMarker(strings.Repeat("=", level) + " "[:min(level, 1)])
}

// ToggleList makes the line of the caret an item of a bulleted list, or of
// a numbered one, or a plain paragraph if it is one already.
func (d *Document) ToggleList(numbered bool) {
	m := "- "
	if numbered {
		m = "+ "
	}
	if bs, fresh := blockStart(d.src, parse(d.src), d.caret); fresh && marker(d.src[bs:]) == m {
		m = ""
	}
	d.setMarker(m)
}

// setMarker replaces the heading or list marker of the line of the caret,
// at the start of its source line or of the table cell it is in.
func (d *Document) setMarker(m string) {
	src := d.src
	st := parse(src)
	bs, fresh := blockStart(src, st, d.caret)
	for _, g := range st.groups {
		if g.kind == delimGroup && g.open < bs && bs <= g.end {
			// A heading ends strong text in stars at its line; a call
			// goes on.
			d.callForm(g)
			d.setMarker(m)
			return
		}
	}
	if !fresh {
		// The line starts in code, after a table say: the text after it
		// gets a line of its own.
		if m != "" {
			d.replace(bs, bs, "\n"+m, false)
			d.mend(bs + 1 + len(m)) // which starts a line now
			d.edited()
		}
		return
	}
	old := marker(src[bs:])
	if old == m {
		return
	}
	d.replace(bs, bs+len(old), m, false)
	d.mend(bs + len(m))
	d.edited()
}

// InsertTable inserts a table of the given number of columns and rows below
// the paragraph of the caret, with a header row, and puts the caret into its
// first cell. In a table, below the paragraph means below the table.
func (d *Document) InsertTable(columns, rows int) {
	columns, rows = max(1, columns), max(1, rows)
	src := d.src
	st := parse(src)
	at := len(src)
	for i := d.caret; i < len(src); {
		k := strings.Index(src[i:], "\n\n")
		if k < 0 {
			break
		}
		if st.topLevel(i + k) {
			at = i + k
			break
		}
		i += k + 1
	}
	var b strings.Builder
	b.WriteString("\n\n#table(\n  columns: ")
	b.WriteString(strconv.Itoa(columns))
	b.WriteString(",\n  table.header(")
	first := 0
	for c := range columns {
		if c > 0 {
			b.WriteString(", ")
		}
		b.WriteString("[#strong[")
		if c == 0 {
			first = b.Len()
		}
		b.WriteString("Column " + strconv.Itoa(c+1))
		b.WriteString("]]")
	}
	b.WriteString("),\n")
	for range rows {
		b.WriteString("  ")
		for c := range columns {
			if c > 0 {
				b.WriteString(", ")
			}
			b.WriteString("[Text]")
		}
		b.WriteString(",\n")
	}
	b.WriteString(")")
	d.replace(at, at, b.String(), false)
	d.caret, d.anchor = at+first, at+first
	d.mend(at)
	d.edited()
}

// move moves the caret, as an arrow key does: left and right to the next
// place a caret can stand at, up and down to the line above or below, on
// the next page past the last line of one.
func (d *Document) move(dx, dy int, extend bool) {
	ix := d.idx()
	off := d.caret
	switch {
	case dx != 0:
		if lo, hi := d.Selection(); lo != hi && !extend {
			off = lo
			if dx > 0 {
				off = hi
			}
			break
		}
		off = ix.step(off, dx > 0)
		d.hasGoal = false
	case dy != 0:
		g, x, ok := ix.caret(off)
		if !ok {
			return
		}
		if !d.hasGoal {
			d.goalX, d.hasGoal = x, true
		}
		li := g.line + dy
		if li < 0 || li >= len(ix.lines) {
			return
		}
		off = ix.inLine(li, d.goalX)
	}
	d.caret = off
	if !extend {
		d.anchor = off
	}
	d.changed()
}
