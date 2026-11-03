package typstview

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// structure is what editing needs to know of a source beyond its characters:
// the content blocks in it, its strong and emphasised text, and where its
// embedded expressions end. It reads the markup that editing writes, and as
// much of the rest of Typst as it takes not to be misled by it: escapes,
// comments, raw text, math, strings and code.
type structure struct {
	groups []group  // by the start of their content
	embeds [][2]int // embedded expressions, from the hash to their end
	ends   []int    // where an embedded expression ends, ascending
	stmts  []int    // where a set, show or let statement ends, ascending
	code   [][2]int // strings, raw text, math and comments, where text is not markup
	marks  [][2]int // heading and list markers at the start of a line, with their space
}

type groupKind uint8

const (
	callGroup  groupKind = iota // the content block of a call in markup, #strong[…]
	argGroup                    // a content block among the arguments of a call, a table cell
	delimGroup                  // strong or emphasised text in *…* or _…_
)

// group is a stretch of markup inside markup: a content block, or strong or
// emphasised text between delimiters.
type group struct {
	kind       groupKind
	fn         string // the function of a call, or the delimiter
	open       int    // where its opening markup starts: the hash, the bracket or the delimiter
	start, end int    // its content
	close      int    // where its closing markup ends
}

// inline reports whether g is strong or emphasised text, which a paragraph
// break or a heading splits rather than ends.
func (g *group) inline() bool {
	return g.kind == delimGroup || g.kind == callGroup && (g.fn == "strong" || g.fn == "emph")
}

// prefix and suffix are the markup that opens and closes g.
func (g *group) prefix(src string) string { return src[g.open:g.start] }
func (g *group) suffix(src string) string { return src[g.end:g.close] }

func parse(src string) *structure {
	p := &parser{src: src, st: &structure{}}
	p.markup(0, false, true)
	st := p.st
	sort.Slice(st.groups, func(i, j int) bool { return st.groups[i].start < st.groups[j].start })
	sort.Ints(st.ends)
	sort.Ints(st.stmts)
	return st
}

type parser struct {
	src string
	i   int
	st  *structure
}

// markup reads markup until the end of the source, or until a bracket that
// closes the block it is in, or, for strong or emphasised text, until its
// delimiter or a paragraph break. It reports what it stopped at, which it
// does not consume: 0 at the end, the bracket, the delimiter, or '\n'.
func (p *parser) markup(delim byte, inBlock, lineStart bool) byte {
	src := p.src
	nest := 0
	for p.i < len(src) {
		c := src[p.i]
		switch {
		case c == '\\':
			p.i++
			if strings.HasPrefix(src[p.i:], "u{") {
				if k := strings.IndexByte(src[p.i:], '}'); k >= 0 {
					p.i += k + 1
				}
			} else if r, n := utf8.DecodeRuneInString(src[p.i:]); n > 0 && !unicode.IsSpace(r) {
				p.i += n
			}
			lineStart = false
		case c == '/' && strings.HasPrefix(src[p.i:], "//"):
			p.lineComment()
		case c == '/' && strings.HasPrefix(src[p.i:], "/*"):
			p.blockComment()
		case c == '`':
			p.raw()
			lineStart = false
		case c == '$':
			p.math()
			lineStart = false
		case c == '#':
			p.i++
			p.embedded()
			lineStart = false
		case c == '[':
			nest++
			p.i++
			lineStart = false
		case c == ']':
			if nest > 0 {
				nest--
				p.i++
				continue
			}
			if inBlock || delim != 0 {
				return ']'
			}
			p.i++
		case c == '*' || c == '_':
			if inWord(src, p.i) {
				p.i++
				lineStart = false
				continue
			}
			if delim == c {
				return c
			}
			open := p.i
			p.i++
			if p.markup(c, true, false) == c {
				p.st.groups = append(p.st.groups, group{kind: delimGroup, fn: string(c), open: open, start: open + 1, end: p.i, close: p.i + 1})
				p.i++
			}
			lineStart = false
		case c == '\n':
			if delim != 0 && parbreakAt(src, p.i) {
				return '\n'
			}
			p.i++
			lineStart = true
		case c == ' ' || c == '\t' || c == '\r':
			p.i++
		default:
			if lineStart {
				if n := markerLen(src[p.i:]); n > 0 {
					// Markers nest, "- + item", and what follows one starts
					// a line too.
					p.st.marks = append(p.st.marks, [2]int{p.i, p.i + n})
					p.i += n
					continue
				}
			}
			_, n := utf8.DecodeRuneInString(src[p.i:])
			p.i += n
			lineStart = false
		}
	}
	return 0
}

// markerLen is the length of the heading or list marker s starts with, with
// the space after it, or 0.
func markerLen(s string) int {
	i := 0
	for i < len(s) && s[i] == '=' {
		i++
	}
	if i == 0 && len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		i = 1
	}
	if i == 0 || i >= len(s) || s[i] != ' ' {
		return 0
	}
	return i + 1
}

// embedded reads the expression after a hash in markup.
func (p *parser) embedded() {
	src := p.src
	hash := p.i - 1
	if p.i >= len(src) {
		return
	}
	switch c := src[p.i]; {
	case isIdentStart(c):
		switch name := p.ident(); name {
		case "set", "show", "let", "import", "include":
			p.statement()
			p.st.stmts = append(p.st.stmts, p.i)
		case "if", "for", "while", "context", "return", "break", "continue":
			// Rarer in a document than anywhere else; read as a statement.
			p.statement()
		default:
			p.postfix(hash, name)
		}
	case c == '(':
		p.code(')')
		p.postfix(hash, "")
	case c == '{':
		p.code('}')
		p.postfix(hash, "")
	case c == '[':
		p.content(hash, "", callGroup)
		p.postfix(hash, "")
	case c == '"':
		p.str()
		p.postfix(hash, "")
	default:
		return
	}
	p.st.embeds = append(p.st.embeds, [2]int{hash, p.i})
	p.st.ends = append(p.st.ends, p.i)
}

// postfix reads what continues an expression: arguments, content blocks and
// fields, as in #table.header(…) or #strong[…].
func (p *parser) postfix(hash int, name string) {
	src := p.src
	for p.i < len(src) {
		switch c := src[p.i]; {
		case c == '(':
			p.code(')')
		case c == '[':
			p.content(hash, name, callGroup)
		case c == '.' && p.i+1 < len(src) && isIdentStart(src[p.i+1]):
			p.i++
			name = p.ident()
		default:
			return
		}
	}
}

// statement reads a statement to the end of its line or a semicolon.
func (p *parser) statement() {
	src := p.src
	for p.i < len(src) {
		switch c := src[p.i]; {
		case c == '\n':
			return
		case c == ';':
			p.i++
			return
		default:
			p.codeToken()
		}
	}
}

// code reads code from an opening bracket to its closing one.
func (p *parser) code(closer byte) {
	p.i++
	for p.i < len(p.src) {
		if p.src[p.i] == closer {
			p.i++
			return
		}
		p.codeToken()
	}
}

// codeToken reads one token of code, or a whole bracketed part.
func (p *parser) codeToken() {
	src := p.src
	switch c := src[p.i]; {
	case c == '(':
		p.code(')')
	case c == '{':
		p.code('}')
	case c == '[':
		p.content(p.i, "", argGroup)
	case c == '"':
		p.str()
	case c == '`':
		p.raw()
	case c == '$':
		p.math()
	case c == '/' && strings.HasPrefix(src[p.i:], "//"):
		p.lineComment()
	case c == '/' && strings.HasPrefix(src[p.i:], "/*"):
		p.blockComment()
	default:
		p.i++
	}
}

// content reads a content block from its bracket.
func (p *parser) content(open int, fn string, kind groupKind) {
	p.i++
	start := p.i
	if p.markup(0, true, true) == ']' {
		p.st.groups = append(p.st.groups, group{kind: kind, fn: fn, open: open, start: start, end: p.i, close: p.i + 1})
		p.i++
	}
}

func (p *parser) ident() string {
	start := p.i
	for p.i < len(p.src) && (isIdentStart(p.src[p.i]) || p.src[p.i] >= '0' && p.src[p.i] <= '9' || p.src[p.i] == '-') {
		p.i++
	}
	return p.src[start:p.i]
}

func isIdentStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

func (p *parser) str() {
	start := p.i
	p.i++
	for p.i < len(p.src) && p.src[p.i] != '\n' {
		switch p.src[p.i] {
		case '\\':
			p.i += 2
			continue
		case '"':
			p.i++
			p.opaque(start)
			return
		}
		p.i++
	}
	p.i = min(p.i, len(p.src))
	p.opaque(start)
}

func (p *parser) raw() {
	start := p.i
	n := 0
	for p.i < len(p.src) && p.src[p.i] == '`' {
		p.i++
		n++
	}
	if n == 2 {
		p.opaque(start)
		return
	}
	if k := strings.Index(p.src[p.i:], strings.Repeat("`", n)); k >= 0 {
		p.i += k + n
	} else {
		p.i = len(p.src)
	}
	p.opaque(start)
}

func (p *parser) math() {
	start := p.i
	p.i++
	for p.i < len(p.src) {
		switch p.src[p.i] {
		case '\\':
			p.i += 2
			continue
		case '$':
			p.i++
			p.opaque(start)
			return
		}
		p.i++
	}
	p.i = len(p.src)
	p.opaque(start)
}

func (p *parser) lineComment() {
	start := p.i
	if k := strings.IndexByte(p.src[p.i:], '\n'); k >= 0 {
		p.i += k
	} else {
		p.i = len(p.src)
	}
	p.opaque(start)
}

func (p *parser) blockComment() {
	start := p.i
	depth := 0
	for p.i < len(p.src) {
		switch {
		case strings.HasPrefix(p.src[p.i:], "/*"):
			depth++
			p.i += 2
		case strings.HasPrefix(p.src[p.i:], "*/"):
			depth--
			p.i += 2
			if depth == 0 {
				p.opaque(start)
				return
			}
		default:
			p.i++
		}
	}
	p.opaque(start)
}

func (p *parser) opaque(start int) {
	p.i = min(p.i, len(p.src))
	p.st.code = append(p.st.code, [2]int{start, p.i})
}

// inWord reports whether the character at i stands between two letters or
// digits, where Typst reads a star or an underscore as text.
func inWord(src string, i int) bool {
	a, _ := utf8.DecodeLastRuneInString(src[:i])
	_, n := utf8.DecodeRuneInString(src[i:])
	b, _ := utf8.DecodeRuneInString(src[i+n:])
	return isWordRune(a) && isWordRune(b)
}

// parbreakAt reports whether the line break at i starts a paragraph break:
// white space with another line break in it.
func parbreakAt(src string, i int) bool {
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\n':
			return true
		case ' ', '\t', '\r':
		default:
			return false
		}
	}
	return false
}

// enclosing returns the groups whose content holds off, outermost first.
func (st *structure) enclosing(off int) []group {
	var out []group
	for _, g := range st.groups {
		if g.start <= off && off <= g.end {
			out = append(out, g)
		}
	}
	return out
}

// container is the innermost group around off that is not strong or
// emphasised text: the content block a paragraph lives in, or nil for the
// document itself.
func (st *structure) container(off int) *group {
	var best *group
	for i := range st.groups {
		g := &st.groups[i]
		if !g.inline() && g.start <= off && off <= g.end && (best == nil || g.start > best.start) {
			best = g
		}
	}
	return best
}

// topLevel reports whether off is in the markup of the document itself,
// outside every embedded expression and every group.
func (st *structure) topLevel(off int) bool {
	for _, e := range st.embeds {
		if e[0] <= off && off < e[1] {
			return false
		}
	}
	for _, g := range st.groups {
		if g.open <= off && off < g.close {
			return false
		}
	}
	return true
}

// inMarkup reports whether off lies in markup: outside every embedded
// expression, or in a content block of the innermost one around it.
func (st *structure) inMarkup(off int) bool {
	embed := -1
	for _, e := range st.embeds {
		if e[0] < off && off < e[1] {
			embed = max(embed, e[0])
		}
	}
	if embed < 0 {
		return true
	}
	for _, g := range st.groups {
		if g.kind != delimGroup && g.start <= off && off <= g.end && g.open >= embed {
			return true
		}
	}
	return false
}

// inCode reports whether off lies in a string, raw text, math or a comment.
func (st *structure) inCode(off int) bool {
	for _, c := range st.code {
		if c[0] < off && off < c[1] {
			return true
		}
	}
	return false
}

func (st *structure) isEnd(off int) bool  { return has(st.ends, off) }
func (st *structure) isStmt(off int) bool { return has(st.stmts, off) }

func has(s []int, v int) bool {
	i := sort.SearchInts(s, v)
	return i < len(s) && s[i] == v
}

// isEscaped reports whether the character at i is escaped: an odd number of
// backslashes stands right before it.
func isEscaped(src string, i int) bool {
	n := 0
	for i-n > 0 && src[i-n-1] == '\\' {
		n++
	}
	return n%2 == 1
}

// unitAt is the character at i as editing treats it: with the backslash
// that escapes it, or as the whole escape that starts at i.
func unitAt(src string, i int) (int, int) {
	if k := strings.LastIndex(src[max(0, i-12):i+1], `\u{`); k >= 0 {
		if s := max(0, i-12) + k; !isEscaped(src, s) {
			if _, e := escapeSpan(src, s, s+1); e > i {
				return s, e
			}
		}
	}
	if isEscaped(src, i) {
		_, n := utf8.DecodeRuneInString(src[i:])
		return i - 1, i + n
	}
	if src[i] == '\\' && i+1 < len(src) {
		s, e := escapeSpan(src, i, i+1)
		return s, e
	}
	_, n := utf8.DecodeRuneInString(src[i:])
	return i, i + n
}

// snap moves off out of an escape it lies inside, between the backslash and
// what it escapes: to the start of the escape, or with end to its end.
func snap(src string, off int, end bool) int {
	if off <= 0 || off >= len(src) {
		return off
	}
	if u0, u1 := unitAt(src, off); u0 < off {
		if end {
			return u1
		}
		return u0
	}
	return off
}

// blockStart is the start of the line of off as markup sees it: the start of
// the source line, or the start of the content block it is in, a table cell
// say, where that comes later. Where the line starts in the code of an
// expression, a table that ends on it, it is the end of that expression,
// and fresh is false: a heading or a list marker needs a line break there.
func blockStart(src string, st *structure, off int) (bs int, fresh bool) {
	bs, fresh = lineStartOf(src, off), true
	for _, e := range st.embeds {
		if e[0] < bs && bs < e[1] && e[1] <= off {
			bs, fresh = e[1], false
		}
	}
	if g := st.container(off); g != nil && g.start > bs {
		return g.start, true
	}
	return bs, fresh
}

// atStart reports whether Typst reads what stands at off as the start of a
// line, where a heading or a list begins: after nothing but white space on
// its line, at the start of a content block, or right after a heading or
// list marker.
func atStart(src string, st *structure, off int) bool {
	q := off
	for q > 0 && (src[q-1] == ' ' || src[q-1] == '\t') {
		q--
	}
	if q == 0 || src[q-1] == '\n' {
		return true
	}
	for _, g := range st.groups {
		if g.kind != delimGroup && q <= g.start && g.start <= off {
			return true
		}
	}
	for _, m := range st.marks {
		if q <= m[1] && m[1] <= off {
			return true
		}
	}
	return false
}

// startHazard is where in s, standing at the start of a line, a backslash
// keeps Typst from reading a heading, a list or a term: before the marker,
// or before the dot of a number like "1.". It is -1 where there is none.
func startHazard(s string) int {
	if t := strings.TrimLeft(s, " \t"); len(t) < len(s) {
		if h := startHazard(t); h >= 0 {
			return h + len(s) - len(t)
		}
		return -1
	}
	if s == "" {
		return -1
	}
	spaceOrEnd := func(i int) bool {
		if i >= len(s) {
			return true
		}
		r, _ := utf8.DecodeRuneInString(s[i:])
		return unicode.IsSpace(r)
	}
	switch c := s[0]; {
	case c == '=':
		i := 0
		for i < len(s) && s[i] == '=' {
			i++
		}
		if spaceOrEnd(i) {
			return 0
		}
	case c == '-' || c == '+' || c == '/':
		if spaceOrEnd(1) {
			return 0
		}
	case c >= '0' && c <= '9':
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i < len(s) && s[i] == '.' && spaceOrEnd(i+1) {
			return i
		}
	}
	return -1
}
