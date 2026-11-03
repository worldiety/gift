package typstview

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/typst"
	"github.com/worldiety/gift/ui"
)

var (
	editorType  = gift.RegisterType("typstview.Editor")
	viewerType  = gift.RegisterType("typstview.Viewer")
	docPageType = gift.RegisterType("typstview.docPage")
)

// maxFitZoom is the largest scale a page is fitted to its width with: a
// page on a wide window is not blown up past twice its size.
const maxFitZoom = 2

// EditorView edits a [Document] where it is set: its pages, below a
// toolbar. It is created by [Editor].
//
//	doc := typstview.Use(ctx, "letter", source)
//	typstview.Editor(doc).Flex(1)
//
// A click on a page puts the caret where it lands, a drag or a shift-click
// selects, a double click selects a word and a triple click a paragraph.
// What is typed goes into the source at the caret and the page is set
// again; see [Document.Insert] for how it is escaped. Backspace and delete
// remove what the page shows and keep the markup, enter starts a paragraph
// or the next item of a list, the arrows move the caret through the text the
// page shows, with shift they select, and the shortcuts select all, copy, cut
// and paste plain text.
//
// The toolbar makes the selection, or the word at the caret, strong or
// emphasised, turns the line of the caret into a heading, a paragraph or a
// list item, inserts a table, and says how many pages the document has, or
// what keeps it from being set.
//
// The pages are fitted to the width of the editor, at twice their size at
// most, unless [EditorView.Zoom] fixes the scale, and scroll.
type EditorView struct {
	d       *Document
	key     string
	flex    float32
	zoom    float32
	toolbar bool
}

// Editor returns the editor of d, with the toolbar.
func Editor(d *Document) EditorView { return EditorView{d: d, toolbar: true} }

// Zoom fixes the scale in logical pixels per point; zero fits the pages to
// the width.
func (v EditorView) Zoom(z float32) EditorView { v.zoom = z; return v }

// Toolbar shows or hides the toolbar, for a program that brings its own and
// calls the methods of [Document] from it.
func (v EditorView) Toolbar(b bool) EditorView { v.toolbar = b; return v }

// Key sets the reconciliation key among siblings.
func (v EditorView) Key(k string) EditorView { v.key = k; return v }

// Flex sets the flex factor in a stack.
func (v EditorView) Flex(f float32) EditorView { v.flex = f; return v }

// ViewType implements gift.View.
func (EditorView) ViewType() gift.TypeID { return editorType }

// Build implements gift.View.
func (v EditorView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Flex: v.flex, Layouter: fill{}, Children: []gift.View{
		gift.Component("typstview.editor", func(ctx *gift.Context) gift.View {
			Watch(ctx, v.d)
			pages := pageList(v.d, v.zoom, true)
			if !v.toolbar {
				return pages
			}
			return ui.VStack(toolbar(v.d), ui.Divider(), pages.Flex(1)).Gap(0)
		}),
	}}
}

// ViewerView shows the pages of a [Document], scrolling. It is created by
// [Viewer].
//
//	typstview.Viewer(doc).Flex(1)
//
// It is the editor without the toolbar and without input: the pages follow
// the source as it changes, fitted to the width as in [EditorView].
type ViewerView struct {
	d    *Document
	key  string
	flex float32
	zoom float32
}

// Viewer returns the viewer of d.
func Viewer(d *Document) ViewerView { return ViewerView{d: d} }

// Zoom fixes the scale in logical pixels per point; zero fits the pages to
// the width.
func (v ViewerView) Zoom(z float32) ViewerView { v.zoom = z; return v }

// Key sets the reconciliation key among siblings.
func (v ViewerView) Key(k string) ViewerView { v.key = k; return v }

// Flex sets the flex factor in a stack.
func (v ViewerView) Flex(f float32) ViewerView { v.flex = f; return v }

// ViewType implements gift.View.
func (ViewerView) ViewType() gift.TypeID { return viewerType }

// Build implements gift.View.
func (v ViewerView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Flex: v.flex, Layouter: fill{}, Children: []gift.View{
		gift.Component("typstview.viewer", func(ctx *gift.Context) gift.View {
			Watch(ctx, v.d)
			return pageList(v.d, v.zoom, false)
		}),
	}}
}

// Watch rebuilds the component being built whenever d changes: when a
// source has been set, and after an edit or a move of the caret. A program
// calls it in the component that shows something of the document itself,
// its diagnostics in a code editor say; [Editor] and [Viewer] watch their
// document on their own.
func Watch(ctx *gift.Context, d *Document) {
	rev := ctx.State("rev", 0)
	ctx.Read(rev)
	tok := ctx.Request()
	key := ctx.Path()
	d.watch(key, func() { tok.Post(func() { rev.Set(rev.Get() + 1) }) })
	ctx.OnUnmount(fmt.Sprintf("typstview.watch %p", d), func() { d.watch(key, nil) })
	d.poll()
}

// pageList is the pages of d in a scroll view, or a line that says why there
// are none.
func pageList(d *Document, zoom float32, editable bool) ui.ScrollView {
	var kids []gift.View
	switch {
	case d.pages != nil:
		for i := range d.pages.Pages {
			kids = append(kids, docPage{d: d, page: i, zoom: zoom, editable: editable,
				selection: ui.ResolveColor(ui.Fade(ui.ColorAccent, 0.3))})
		}
	case d.err != nil:
		kids = append(kids, ui.Text(d.err.Error()).Foreground(ui.ColorSecondaryLabel))
	default:
		kids = append(kids, ui.Text("Setting…").Foreground(ui.ColorSecondaryLabel))
	}
	return ui.VScroll(kids...).Gap(16).Padding(16).Align(geom.Alignment{X: 0.5}).
		Background(ui.ColorBackground)
}

// toolbar is the row of tools above the pages of an editor.
func toolbar(d *Document) gift.View {
	tool := func(content gift.View, label string, fn func()) gift.View {
		return ui.Button(content, func() {
			fn()
			// The button took the focus; the page of the caret takes it
			// back, so that typing goes on where it was.
			d.refocus = true
			d.changed()
		}).Background(ui.ColorClear).Border(ui.Border{}).Padding(6).Label(label)
	}
	icon := func(sym ui.Symbol, label string, fn func()) gift.View {
		return tool(ui.Icon(sym).Size(16).Foreground(ui.ColorLabel), label, fn)
	}
	word := func(s, label string, fn func()) gift.View {
		return tool(ui.Text(s).FontSize(13), label, fn)
	}
	gap := func() gift.View { return ui.Box().Frame(1, 18).Background(ui.ColorSeparator) }
	return ui.HStack(
		icon(outline.LetterBold, "Strong", d.ToggleStrong),
		icon(outline.LetterItalic, "Emphasis", d.ToggleEmphasis),
		gap(),
		word("H1", "Heading 1", func() { d.SetHeading(1) }),
		word("H2", "Heading 2", func() { d.SetHeading(2) }),
		icon(outline.Paragraph, "Paragraph", func() { d.SetHeading(0) }),
		gap(),
		icon(outline.List, "Bulleted list", func() { d.ToggleList(false) }),
		icon(outline.OrderedList, "Numbered list", func() { d.ToggleList(true) }),
		icon(outline.Grid, "Insert table", func() { d.InsertTable(3, 2) }),
		ui.Spacer(),
		status(d),
	).Gap(2).Align(geom.Center).PaddingInsets(geom.Insets{Left: 8, Right: 12, Top: 4, Bottom: 4}).
		Background(ui.ColorSurface)
}

// status says how the document was set, or the first error that keeps it
// from being set.
func status(d *Document) gift.View {
	if d.err != nil {
		return ui.Text(d.err.Error()).FontSize(12).Foreground(ui.ColorDanger)
	}
	if len(d.diags) > 0 {
		return ui.Text(describe(d.Source(), d.diags[0])).FontSize(12).Foreground(ui.ColorDanger)
	}
	if d.pages == nil {
		return ui.Text("Setting…").FontSize(12).Foreground(ui.ColorSecondaryLabel)
	}
	n := len(d.pages.Pages)
	s := fmt.Sprintf("%d pages", n)
	if n == 1 {
		s = "1 page"
	}
	return ui.Text(s).FontSize(12).Foreground(ui.ColorSecondaryLabel)
}

// describe is a diagnostic with the number of its line.
func describe(src string, g typst.Diagnostic) string {
	if g.Start < 0 || g.Start > len(src) {
		return g.Message
	}
	return fmt.Sprintf("Line %d: %s", strings.Count(src[:g.Start], "\n")+1, g.Message)
}

// fill lays its one child out in the constraints it is given.
type fill struct{}

func (fill) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	if ctx.ChildCount() == 0 {
		return c.Constrain(geom.Size{})
	}
	s := ctx.Measure(0, c)
	ctx.Place(0, geom.Point{})
	return s
}

// docPage is one page of a document in an editor or a viewer.
type docPage struct {
	d         *Document
	page      int
	zoom      float32
	editable  bool
	selection ui.Color
}

func (docPage) ViewType() gift.TypeID { return docPageType }

func (v docPage) Build(*gift.BuildContext) gift.Element {
	n := &docPageNode{v: v}
	el := gift.Element{Key: fmt.Sprint(v.page), Layouter: n, Painter: n}
	if v.editable {
		el.Interactor, el.Focusable, el.Label = n, true, "Page "+fmt.Sprint(v.page+1)
	}
	return el
}

type docPageNode struct {
	v docPage
	z float32 // the scale of the last layout
}

func (n *docPageNode) pg() *typst.Page {
	d := n.v.d
	if d.pages == nil || n.v.page >= len(d.pages.Pages) {
		return nil
	}
	return &d.pages.Pages[n.v.page]
}

// Layout implements gift.Layouter: the page at its size times the scale,
// fitted to the width it may have unless the scale is fixed.
func (n *docPageNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	p := n.pg()
	if p == nil {
		return c.Constrain(geom.Size{})
	}
	n.z = n.v.zoom
	if n.z <= 0 {
		n.z = 1
		if w := c.Max.W; w > 0 && w < 1e6 {
			n.z = min(maxFitZoom, w/p.Width)
		}
	}
	if d := n.v.d; n.v.editable && d.refocus && n.caretPage() == n.v.page {
		ctx.After(time.Millisecond)
	}
	return c.Constrain(geom.Sz(p.Width*n.z, p.Height*n.z))
}

// caretPage is the page the caret is drawn on.
func (n *docPageNode) caretPage() int {
	if g, _, ok := n.v.d.idx().caret(n.v.d.caret); ok {
		return g.page
	}
	return 0
}

// Paint implements gift.Painter: the page, then the selection and the
// caret over it.
func (n *docPageNode) Paint(ctx *gift.PaintContext) {
	p := n.pg()
	if p == nil {
		return
	}
	d := n.v.d
	paintPage(ctx, p, n.z, func(at func(geom.Rect) geom.Rect) {
		if !n.v.editable {
			return
		}
		ix := d.idx()
		if lo, hi := d.Selection(); lo != hi {
			for i := range ix.glyphs {
				g := &ix.glyphs[i]
				if g.page == n.v.page && g.start >= lo && g.end <= hi {
					ctx.Add(render.Op{Kind: render.OpFillRect, Color: n.v.selection,
						Bounds: at(geom.Rc(g.x, g.top, g.x+g.adv, g.bot))})
				}
			}
		}
		if !d.hasFocus && !d.refocus {
			return
		}
		if g, x, ok := ix.caret(d.caret); ok && g.page == n.v.page {
			r := at(geom.Rc(x, g.top, x, g.bot))
			ctx.Add(render.Op{Kind: render.OpFillRect, Color: caret,
				Bounds: geom.Rc(r.Min.X-0.75, r.Min.Y, r.Min.X+0.75, r.Max.Y)})
		}
	})
}

// HandleEvent implements gift.Interactor for the page of an editor.
func (n *docPageNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	d := n.v.d
	switch e.Kind {
	case gift.EventTimer:
		if d.refocus {
			d.refocus = false
			ctx.RequestFocus()
		}
		return true
	case gift.EventFocusGained:
		d.hasFocus = true
		ctx.Repaint()
		return true
	case gift.EventFocusLost:
		d.hasFocus = false
		ctx.Repaint()
		return true
	case gift.EventPointerDown:
		ctx.RequestFocus()
		off, ok := n.offsetAt(ctx, e.Pos)
		if !ok {
			return true
		}
		// Clicks in a row on one place select a word, then a paragraph.
		if e.Time-d.clickAt < 400*time.Millisecond && off == d.clickOff {
			d.clicks++
		} else {
			d.clicks = 1
		}
		d.clickAt, d.clickOff = e.Time, off
		switch {
		case d.clicks == 2:
			lo, hi := wordAt(d.src, off)
			d.SetSelection(lo, hi)
		case d.clicks >= 3:
			lo, hi := paragraphAt(d.src, off)
			d.SetSelection(lo, hi)
		case e.Mods.Has(gift.ModShift):
			d.SetSelection(d.anchor, off)
		default:
			d.SetSelection(off, off)
		}
		d.dragging = true
		return true
	case gift.EventPointerMove:
		if !d.dragging || !ctx.Interaction().Pressed {
			return false
		}
		if off, ok := n.offsetAt(ctx, e.Pos); ok && off != d.caret {
			d.SetSelection(d.anchor, off)
		}
		return true
	case gift.EventPointerUp, gift.EventPointerCancel:
		d.dragging = false
		return true
	case gift.EventRune:
		if !ctx.Interaction().Focused {
			return false
		}
		if e.Mods.Has(gift.ShortcutModifier) {
			switch unicode.ToLower(e.Rune) {
			case 'b':
				d.ToggleStrong()
			case 'i':
				d.ToggleEmphasis()
			default:
				return false
			}
			return true
		}
		if unicode.IsControl(e.Rune) {
			return false
		}
		d.Insert(string(e.Rune))
		return true
	case gift.EventKeyDown:
		if !ctx.Interaction().Focused {
			return false
		}
		return n.key(e)
	}
	return false
}

// key is the keyboard of a page.
func (n *docPageNode) key(e gift.Event) bool {
	d := n.v.d
	extend := e.Mods.Has(gift.ModShift)
	shortcut := e.Mods.Has(gift.ShortcutModifier)
	switch e.Key {
	case gift.KeyLeft:
		d.move(-1, 0, extend)
	case gift.KeyRight:
		d.move(1, 0, extend)
	case gift.KeyUp:
		d.move(0, -1, extend)
	case gift.KeyDown:
		d.move(0, 1, extend)
	case gift.KeyBackspace:
		d.DeleteBackward()
	case gift.KeyDelete:
		d.DeleteForward()
	case gift.KeyEnter:
		d.NewParagraph()
	case gift.KeyA:
		if !shortcut {
			return false
		}
		d.selectAll()
	case gift.KeyC, gift.KeyX:
		if !shortcut {
			return false
		}
		if s := d.SelectedText(); s != "" {
			ui.CurrentClipboard().SetText(s)
			if e.Key == gift.KeyX {
				d.DeleteBackward()
			}
		}
	case gift.KeyV:
		if !shortcut {
			return false
		}
		if s, ok := ui.CurrentClipboard().Text(); ok {
			d.Insert(s)
		}
	default:
		return false
	}
	return true
}

// offsetAt is the source offset a pointer at pos means, on this page.
func (n *docPageNode) offsetAt(ctx *gift.EventContext, pos geom.Point) (int, bool) {
	p := n.pg()
	db := ctx.DeviceBounds()
	if p == nil || !(db.Width() > 0) || !(db.Height() > 0) {
		return 0, false
	}
	x := (pos.X - db.Min.X) / db.Width() * p.Width
	y := (pos.Y - db.Min.Y) / db.Height() * p.Height
	return n.v.d.idx().at(n.v.page, x, y)
}

// paragraphAt is the paragraph of the source around off: the text between
// two blank lines.
func paragraphAt(s string, off int) (int, int) {
	lo := strings.LastIndex(s[:off], "\n\n")
	if lo < 0 {
		lo = 0
	} else {
		lo += 2
	}
	hi := len(s)
	if i := strings.Index(s[off:], "\n\n"); i >= 0 {
		hi = off + i
	}
	return lo, hi
}
