package typstview

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/worldiety/gift/typst"
	"github.com/worldiety/gift/ui"
)

// What a visitor does with an editor, as the methods a page and its toolbar
// call: clicks and drags put the caret and select, keys type, delete and
// break paragraphs, and the toolbar formats. Each returns what it did, for
// the report of a failure.
var monkeyOps = []struct {
	weight int
	do     func(d *Document, r *rand.Rand) string
}{
	{8, func(d *Document, r *rand.Rand) string { // click
		off := randomStop(d, r)
		d.SetSelection(off, off)
		return fmt.Sprintf("click %d", off)
	}},
	{3, func(d *Document, r *rand.Rand) string { // shift-click, drag
		off := randomStop(d, r)
		d.SetSelection(d.anchor, off)
		return fmt.Sprintf("select to %d", off)
	}},
	{2, func(d *Document, r *rand.Rand) string { // double click
		lo, hi := wordAt(d.src, randomStop(d, r))
		d.SetSelection(lo, hi)
		return fmt.Sprintf("select word %d..%d", lo, hi)
	}},
	{1, func(d *Document, r *rand.Rand) string { // triple click
		lo, hi := paragraphAt(d.src, randomStop(d, r))
		d.SetSelection(lo, hi)
		return fmt.Sprintf("select paragraph %d..%d", lo, hi)
	}},
	{1, func(d *Document, r *rand.Rand) string {
		d.selectAll()
		return "select all"
	}},
	{4, func(d *Document, r *rand.Rand) string {
		dx, dy := []int{-1, 1, 0, 0}[r.Intn(4)], 0
		if dx == 0 {
			dy = []int{-1, 1}[r.Intn(2)]
		}
		extend := r.Intn(3) == 0
		d.move(dx, dy, extend)
		return fmt.Sprintf("arrow %d,%d shift %v", dx, dy, extend)
	}},
	{14, func(d *Document, r *rand.Rand) string {
		s := string(monkeyRunes[r.Intn(len(monkeyRunes))])
		d.Insert(s)
		return fmt.Sprintf("type %q", s)
	}},
	{3, func(d *Document, r *rand.Rand) string {
		s := monkeyPastes[r.Intn(len(monkeyPastes))]
		if r.Intn(3) == 0 {
			s += monkeyPastes[r.Intn(len(monkeyPastes))]
		}
		d.Insert(s)
		return fmt.Sprintf("paste %q", s)
	}},
	{5, func(d *Document, r *rand.Rand) string { d.NewParagraph(); return "enter" }},
	{5, func(d *Document, r *rand.Rand) string { d.DeleteBackward(); return "backspace" }},
	{3, func(d *Document, r *rand.Rand) string { d.DeleteForward(); return "delete" }},
	{1, func(d *Document, r *rand.Rand) string { // cut
		if d.SelectedText() != "" {
			d.DeleteBackward()
		}
		return "cut"
	}},
	{3, func(d *Document, r *rand.Rand) string { d.ToggleStrong(); return "strong" }},
	{3, func(d *Document, r *rand.Rand) string { d.ToggleEmphasis(); return "emphasis" }},
	{2, func(d *Document, r *rand.Rand) string {
		l := r.Intn(3)
		d.SetHeading(l)
		return fmt.Sprintf("heading %d", l)
	}},
	{2, func(d *Document, r *rand.Rand) string {
		n := r.Intn(2) == 0
		d.ToggleList(n)
		return fmt.Sprintf("list numbered %v", n)
	}},
	{1, func(d *Document, r *rand.Rand) string { d.InsertTable(3, 2); return "table" }},
}

// monkeyRunes are typed one at a time: letters, and every character Typst
// gives a meaning in markup.
var monkeyRunes = []rune("aaaaeeeennssttxyzABZ0129 ....,;:!?()[]{}<>#$*_`@~=-+/\\\"'&%^|é€")

// monkeyPastes are pasted: lines, paragraphs, markup, links and the other
// line breaks of Unicode.
var monkeyPastes = []string{
	"word", "two words", "line\nbreak", "para\n\ngraph", "\n", "\n\n", "\r\n", "a\rb", "x y", "p q",
	"- item", "+ item", "= Head", "/ term: x", "1. one", "12. twelve", "// comment", "/* block */", "*/",
	"*strong*", "_emph_", "#strong[x]", "#emph[y]", "[x]", "]", "[", "(", ")", ".", ".x", "(y)", "\\", "\\\\",
	"$x$", "`raw`", "<label>", "@ref", "~", "--", "...", "https://gift.dev/a_(b)", "\t", " ", "  - x",
	"a - b", "a = b", "x // y", "\\u{41}", "#", "#table()", "#(1+2)", "\"quoted\"", "'single'",
}

func randomStop(d *Document, r *rand.Rand) int {
	if s := d.idx().stops; len(s) > 0 && r.Intn(8) > 0 {
		return s[r.Intn(len(s))]
	}
	return []int{0, len(d.src)}[r.Intn(2)]
}

// TestNothingAnEditorDoesBreaksTheDocument starts from an empty document and
// does what a visitor can do with an editor, at random, setting the source
// after every step: whatever they do, Typst must set it without an error.
//
// For a longer search, GIFT_TYPST_MONKEY sets the number of runs, and
// GIFT_TYPST_MONKEY_RUN repeats the one run a failure names. With
// GIFT_TYPST_MONKEY_FREE the pages of the fast typing test arrive whenever
// the engine is done, mid-step too, which no run repeats exactly;
// GIFT_TYPST_MONKEY_DEBUG reports the text the pages showed before the step
// that failed, and GIFT_TYPST_MONKEY_TRACE the source after every step.
func TestNothingAnEditorDoesBreaksTheDocument(t *testing.T) {
	monkey(t, false)
}

// TestNothingFastTypingDoesBreaksTheDocument is the same with pages that lag
// behind, as they do while somebody types faster than the document is set:
// the editing goes by the pages set some steps before and the text typed
// since, and an engine of the test's own sets every source in between.
func TestNothingFastTypingDoesBreaksTheDocument(t *testing.T) {
	monkey(t, true)
}

func monkey(t *testing.T, lag bool) {
	if !typst.Available() {
		t.Skip("libgifttypst is not available")
	}
	var check *typst.Engine
	if lag {
		var err error
		if check, err = typst.New(); err != nil {
			t.Fatal(err)
		}
		defer check.Close()
		for _, f := range ui.RegisteredFonts(nil) {
			if data := f.Font.Data(); data != nil {
				_ = check.AddFont(data)
			}
		}
	}
	first, runs, steps := 0, 24, 80
	if n, err := strconv.Atoi(os.Getenv("GIFT_TYPST_MONKEY")); err == nil {
		runs = n
	}
	if n, err := strconv.Atoi(os.Getenv("GIFT_TYPST_MONKEY_RUN")); err == nil {
		first, runs = n, n+1 // one run, to see it fail again
	}
	total := 0
	for _, o := range monkeyOps {
		total += o.weight
	}
	for run := first; run < runs; run++ {
		start := []string{"", font}[run%2]
		r := rand.New(rand.NewSource(int64(run)))
		d := NewDocument(start)
		settled(t, d)
		// Held pages lag by whole steps, the same in every run; free ones
		// arrive whenever the engine is done, mid-step too.
		free := os.Getenv("GIFT_TYPST_MONKEY_FREE") != ""
		d.hold = lag && !free
		var log []string
		for step := range steps {
			pick := r.Intn(total)
			i := 0
			for pick >= monkeyOps[i].weight {
				pick -= monkeyOps[i].weight
				i++
			}
			before, caret, anchor := d.src, d.caret, d.anchor
			var spans []string
			if os.Getenv("GIFT_TYPST_MONKEY_DEBUG") != "" {
				ix := d.idx()
				for _, sp := range ix.spans {
					spans = append(spans, fmt.Sprintf("%d..%d %q", sp[0], sp[1], d.src[sp[0]:sp[1]]))
				}
				spans = append(spans, fmt.Sprintf("pagesSeq %d seq %d edits %+v compiled %q", d.pagesSeq, d.seq, d.edits, d.compiledSource))
			}
			log = append(log, monkeyOps[i].do(d, r))
			if os.Getenv("GIFT_TYPST_MONKEY_TRACE") != "" {
				t.Logf("step %d %s: caret %d..%d %q", step, log[len(log)-1], d.anchor, d.caret, d.src)
			}
			if d.caret < 0 || d.caret > len(d.src) || d.anchor < 0 || d.anchor > len(d.src) {
				t.Fatalf("run %d, step %d: caret %d, anchor %d outside %d bytes", run, step, d.caret, d.anchor, len(d.src))
			}
			var g []typst.Diagnostic
			if lag {
				g = compiled(t, check, d.src)
				if r.Intn(6) == 0 {
					d.hold = false
					settled(t, d)
					d.hold = !free
				}
			} else {
				g = settled(t, d)
			}
			if len(g) > 0 {
				d.Close()
				t.Fatalf("run %d, step %d breaks the document: %v\nsteps:\n  %s\nbefore: %q, caret %d, anchor %d\nafter:  %q\nspans before: %s",
					run, step, g, strings.Join(log, "\n  "), before, caret, anchor, d.src, strings.Join(spans, "\n  "))
			}
		}
		d.Close()
	}
}

// compiled sets src with e and returns its errors.
func compiled(t *testing.T, e *typst.Engine, src string) []typst.Diagnostic {
	t.Helper()
	err := e.SetSource(src)
	if err == nil {
		_, err = e.Compile()
	}
	var ce *typst.CompileError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ce):
		return ce.Diagnostics
	}
	t.Fatal(err)
	return nil
}

// settled waits until the document has been set and returns its errors.
func settled(t *testing.T, d *Document) []typst.Diagnostic {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for d.Busy() {
		if time.Now().After(deadline) {
			t.Fatal("the document was not set in time")
		}
		time.Sleep(200 * time.Microsecond)
	}
	if err := d.Err(); err != nil {
		t.Fatal(err)
	}
	return d.Diagnostics()
}
