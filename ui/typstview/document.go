package typstview

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/typst"
	"github.com/worldiety/gift/ui"
)

// Document is a Typst document that a program shows with [Viewer] and edits
// with [Editor]: its source, the engine that sets it, the pages it set last,
// and a caret and a selection in the source.
//
//	doc := typstview.Use(ctx, "letter", "#set text(font: \"Inter\")\n= Dear Ann\n")
//	typstview.Editor(doc)
//
// The source is the document. A program reads and replaces it with
// [Document.Source] and [Document.SetSource], and every edit made on the
// pages is an edit of the source, which [Document.OnChange] reports.
//
// Setting happens on a goroutine of the document's own, so that a keystroke
// never waits for it: the pages shown are those of the last source that was
// set without an error, until the next one is. An edit made meanwhile is
// carried over to them, so that a caret or a deletion lands where the edit
// left the text and not where the stale pages still show it.
//
// The engine sees every font registered with gift when the document is made,
// which a document names with Typst's own text function, as in
// #set text(font: "Inter"); [Document.AddFont] adds more.
//
// A Document belongs to the UI goroutine, as a [ui.TextEditor] does; only
// the setting runs elsewhere. Close it when it is no longer needed, or let
// [Use] do that.
type Document struct {
	src            string
	caret, anchor  int
	onChange       func(string)
	seq            int    // counts the edits of the source
	edits          []edit // the edits since the source of pages
	pages          *typst.Document
	pagesSeq       int // the edit the pages were set after
	diags          []typst.Diagnostic
	err            error
	took           time.Duration
	busy           bool
	index          *index // the glyphs of pages in the offsets of src
	goalX          float32
	hasGoal        bool
	compiledSource string
	track          []int // offsets an edit in progress keeps, moved with the text like the caret
	hold           bool  // pages that were set wait until it is cleared; tests lag with it
	dirty          bool  // the source changed since it was last handed to the worker

	// What the pages of an editor share: whether one of them has the
	// focus, whether one is to take it back from a toolbar button, a drag
	// under way, and the clicks in a row that select a word or a paragraph.
	hasFocus, refocus bool
	typing            bool // the edit being made is typed text
	dragging          bool
	clicks, clickOff  int
	clickAt           time.Duration

	mu       sync.Mutex // guards the fields below, shared with the worker
	work     chan job
	next     *result
	watchers map[string]func()
	engine   *typst.Engine
	ready    chan struct{}
	closed   bool
}

// edit is one change of the source: del bytes at at replaced by ins bytes,
// typed ones if typed is set.
type edit struct {
	seq, at, del, ins int
	typed             bool
}

type job struct {
	src string
	seq int
}

type result struct {
	job
	doc  *typst.Document
	err  error
	took time.Duration
}

// NewDocument returns a document of src and starts setting it, with the
// caret at its end.
func NewDocument(src string) *Document {
	d := &Document{
		src:      src,
		caret:    len(src),
		anchor:   len(src),
		work:     make(chan job, 1),
		ready:    make(chan struct{}),
		watchers: map[string]func(){},
		busy:     true,
	}
	var fonts [][]byte
	for _, f := range ui.RegisteredFonts(nil) {
		if data := f.Font.Data(); data != nil {
			fonts = append(fonts, data)
		}
	}
	go d.run(fonts)
	d.submit()
	return d
}

// Use returns the document called name of the component being built,
// making it of src the first time, and closes it when the component is
// unmounted; see [ui.Editor] for the pattern.
func Use(ctx *gift.Context, name, src string) *Document {
	st := ctx.State(name, (*Document)(nil))
	if d := st.Get(); d != nil {
		return d
	}
	d := NewDocument(src)
	st.Set(d)
	ctx.OnUnmount(name, d.Close)
	return d
}

// run is the worker: it makes the engine, then sets every source it is
// handed, and hands each result back by waking the views that watch.
func (d *Document) run(fonts [][]byte) {
	e, err := typst.New()
	if err == nil {
		for _, f := range fonts {
			// A face gift reads but Typst does not, a collection say, is
			// left out rather than failing the document.
			_ = e.AddFont(f)
		}
	}
	d.mu.Lock()
	d.engine = e
	d.mu.Unlock()
	close(d.ready)
	for j := range d.work {
		r := &result{job: j, err: err}
		if err == nil {
			t0 := time.Now()
			if r.err = e.SetSource(j.src); r.err == nil {
				r.doc, r.err = e.Compile()
			}
			r.took = time.Since(t0)
		}
		d.mu.Lock()
		d.next = r
		wake := make([]func(), 0, len(d.watchers))
		for _, w := range d.watchers {
			wake = append(wake, w)
		}
		d.mu.Unlock()
		for _, w := range wake {
			w()
		}
	}
	if e != nil {
		e.Close()
	}
}

// submit hands the source to the worker, replacing one still waiting.
func (d *Document) submit() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	select {
	case <-d.work:
	default:
	}
	d.work <- job{src: d.src, seq: d.seq}
	d.busy = true
}

// poll takes the result the worker left, if there is one, and reports
// whether there was.
func (d *Document) poll() bool {
	if d.hold {
		return false
	}
	d.mu.Lock()
	r := d.next
	d.next = nil
	d.mu.Unlock()
	if r == nil {
		return false
	}
	d.busy = r.seq != d.seq
	d.took = r.took
	var ce *typst.CompileError
	switch {
	case r.err == nil:
		d.pages, d.pagesSeq, d.diags, d.err = r.doc, r.seq, nil, nil
		d.compiledSource = r.src
		d.index = nil
	case errors.As(r.err, &ce):
		d.diags, d.err = ce.Diagnostics, nil
	default:
		d.err = r.err
	}
	// Edits the pages already show are no longer carried over.
	k := 0
	for _, e := range d.edits {
		if e.seq > d.pagesSeq {
			d.edits[k] = e
			k++
		}
	}
	d.edits = d.edits[:k]
	return true
}

// watch registers wake under key, to be called from any goroutine when the
// document changed; nil removes it.
func (d *Document) watch(key string, wake func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if wake == nil {
		delete(d.watchers, key)
	} else {
		d.watchers[key] = wake
	}
}

// changed wakes the views after a change made on the UI goroutine.
func (d *Document) changed() {
	d.mu.Lock()
	wake := make([]func(), 0, len(d.watchers))
	for _, w := range d.watchers {
		wake = append(wake, w)
	}
	d.mu.Unlock()
	for _, w := range wake {
		w()
	}
}

// Close stops the setting and releases the engine. The document keeps its
// source and its last pages.
func (d *Document) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.work)
	}
}

// Source returns the source.
func (d *Document) Source() string { return d.src }

// SetSource replaces the source and sets it. The caret and the selection
// keep their place in the text around the change.
func (d *Document) SetSource(src string) {
	if src == d.src {
		return
	}
	// The change is the part between the longest common prefix and suffix,
	// which keeps the caret of an edit made elsewhere, in a code editor say,
	// where it was.
	p := 0
	for p < len(src) && p < len(d.src) && src[p] == d.src[p] {
		p++
	}
	s := 0
	for s < len(src)-p && s < len(d.src)-p && src[len(src)-1-s] == d.src[len(d.src)-1-s] {
		s++
	}
	d.replace(p, len(d.src)-s, src[p:len(src)-s], false)
	d.flush()
}

// OnChange sets the function called with the source after every edit made
// through the document, on the pages or by its methods, but not after
// [Document.SetSource].
func (d *Document) OnChange(f func(src string)) { d.onChange = f }

// AddFont makes the faces of a TrueType or OpenType file available to the
// document, as [typst.Engine.AddFont] does, and sets the source again.
func (d *Document) AddFont(data []byte) error {
	<-d.ready
	d.mu.Lock()
	e := d.engine
	d.mu.Unlock()
	if e == nil {
		return typst.ErrUnavailable
	}
	if err := e.AddFont(data); err != nil {
		return err
	}
	d.submit()
	return nil
}

// Pages returns the pages of the last source that was set without an error,
// or nil before the first.
func (d *Document) Pages() *typst.Document {
	d.poll()
	return d.pages
}

// Diagnostics returns the errors of the last source that was set, or nil if
// it was set without one.
func (d *Document) Diagnostics() []typst.Diagnostic {
	d.poll()
	return d.diags
}

// Err returns why the document cannot be set at all, such as
// [typst.ErrUnavailable], or nil.
func (d *Document) Err() error {
	d.poll()
	return d.err
}

// Busy reports whether a source is being set, or waits to be.
func (d *Document) Busy() bool {
	d.poll()
	return d.busy
}

// Took returns how long the last source took to set.
func (d *Document) Took() time.Duration { return d.took }

// PDF waits for the source to be set and returns the pages of the last
// source that was set without an error as a PDF file.
func (d *Document) PDF() ([]byte, error) {
	<-d.ready
	d.mu.Lock()
	e := d.engine
	d.mu.Unlock()
	if e == nil {
		return nil, typst.ErrUnavailable
	}
	// The engine serialises its calls, so this waits for a compilation that
	// is under way.
	return e.PDF()
}

// Caret returns the byte offset of the caret in the source.
func (d *Document) Caret() int { return d.caret }

// Selection returns the selected range of the source, lowest offset first;
// both are the caret without a selection.
func (d *Document) Selection() (int, int) {
	return min(d.anchor, d.caret), max(d.anchor, d.caret)
}

// SetSelection selects the source from anchor to caret, which may lie either
// way round; equal offsets put the caret there without a selection.
func (d *Document) SetSelection(anchor, caret int) {
	d.anchor = max(0, min(anchor, len(d.src)))
	d.caret = max(0, min(caret, len(d.src)))
	d.hasGoal = false
	d.changed()
}

// replace changes the source and records the edit for carrying it over to
// the pages; [Document.flush] sets it. With caretAfter the caret goes after
// the inserted text; otherwise caret and anchor keep their place in the text
// around the change. It does not call OnChange: an edit made through the
// document does that once it is complete, see [Document.edited].
func (d *Document) replace(lo, hi int, s string, caretAfter bool) {
	d.seq++
	d.edits = append(d.edits, edit{seq: d.seq, at: lo, del: hi - lo, ins: len(s), typed: d.typing})
	d.src = d.src[:lo] + s + d.src[hi:]
	d.index = nil
	if caretAfter {
		d.caret, d.anchor = lo+len(s), lo+len(s)
	} else {
		d.caret, d.anchor = shiftOff(d.caret, lo, hi, len(s)), shiftOff(d.anchor, lo, hi, len(s))
	}
	for i, off := range d.track {
		d.track[i] = shiftOff(off, lo, hi, len(s))
	}
	d.hasGoal = false
	d.dirty = true
	d.changed()
}

// flush hands the source to the worker if it changed. An edit made of
// several replacements flushes once, at its end: a source half way through
// one, strong text whose closing bracket is gone and whose opening one is
// not yet, can still set, and then as text the brackets of a table cell
// stand in, which pages set from it would let a deletion take.
func (d *Document) flush() {
	if d.dirty {
		d.dirty = false
		d.submit()
	}
}

// shiftOff moves an offset over the replacement of lo..hi by ins bytes: one
// before it stays, one after it moves with the text, and one inside it goes
// to the end of the replacement.
func shiftOff(off, lo, hi, ins int) int {
	switch {
	case off <= lo:
		return off
	case off >= hi:
		return off - (hi - lo) + ins
	default:
		return lo + ins
	}
}

// lineEnd is the offset of the line break after off, or the end of s.
func lineEnd(s string, off int) int {
	if i := strings.IndexByte(s[off:], '\n'); i >= 0 {
		return off + i
	}
	return len(s)
}
