package typstview

import (
	"strings"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/typst"
	"github.com/worldiety/gift/ui"
)

// editor shows an editor of src in a harness and waits for its pages.
func editor(t *testing.T, src string) (*gifttest.Harness, *Document) {
	t.Helper()
	if !typst.Available() {
		t.Skip("libgifttypst is not available")
	}
	d := NewDocument(font + src)
	t.Cleanup(d.Close)
	h := gifttest.New(t, gifttest.Options{
		Root: func(ctx *gift.Context) gift.View { return ui.Window(Editor(d).Flex(1)) },
		Size: geom.Sz(700, 600),
		Font: ui.MustFont(ui.FontQuery{Family: "Inter"}),
	})
	wait(t, h, d)
	return h, d
}

// wait runs frames until the document has been set and shown.
func wait(t *testing.T, h *gifttest.Harness, d *Document) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		h.AdvanceTicks(1, gifttest.FrameInterval)
		if !d.Busy() && h.Exists(gifttest.ByKey("0")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the document was not shown in time")
		}
		time.Sleep(time.Millisecond)
	}
	h.AdvanceTicks(2, gifttest.FrameInterval)
}

// point is where the glyph whose text starts at off is on the screen, at
// the given fraction of its width.
func point(t *testing.T, h *gifttest.Harness, d *Document, off int, frac float32) geom.Point {
	t.Helper()
	for _, g := range d.idx().glyphs {
		if g.start == off {
			page := h.Find(gifttest.ByKey("0")).LayoutBounds()
			p := &d.pages.Pages[g.page]
			z := page.Width() / p.Width
			return geom.Pt(page.Min.X+(g.x+g.adv*frac)*z, page.Min.Y+(g.top+g.bot)/2*z)
		}
	}
	t.Fatalf("no glyph starts at %d", off)
	return geom.Point{}
}

func TestAClickPutsTheCaretAndTypingGoesThere(t *testing.T) {
	h, d := editor(t, "= Title\n\nDear Ann and Bob,\n\nmore.")
	bob := strings.Index(d.src, "Bob")
	h.ClickAt(point(t, h, d, bob, 0.2))
	h.AdvanceTicks(2, gifttest.FrameInterval)
	if lo, hi := d.Selection(); lo != bob || hi != bob {
		t.Fatalf("the click selected %d..%d, want the caret at %d", lo, hi, bob)
	}
	h.TypeText("XY")
	h.AdvanceTicks(2, gifttest.FrameInterval)
	if got := body(d); got != "= Title\n\nDear Ann and XYBob,\n\nmore." {
		t.Fatalf("%q", got)
	}
	wait(t, h, d)

	// A double click selects the word, and strong makes it strong.
	ann := strings.Index(d.src, "Ann")
	p := point(t, h, d, ann, 0.5)
	h.ClickAt(p)
	h.ClickAt(p)
	h.AdvanceTicks(2, gifttest.FrameInterval)
	if lo, hi := d.Selection(); d.src[lo:hi] != "Ann" {
		t.Fatalf("the double click selected %q", d.src[lo:hi])
	}
	h.Find(gifttest.ByText("Strong")).Click()
	h.AdvanceTicks(4, gifttest.FrameInterval)
	if got := body(d); got != "= Title\n\nDear #strong[Ann] and XYBob,\n\nmore." {
		t.Fatalf("%q", got)
	}
	// The page took the focus back from the toolbar: the arrow collapses
	// the selection to its end, and typing goes on there.
	h.Key(gift.KeyRight)
	h.TypeText("!")
	if got := body(d); !strings.Contains(got, "#strong[Ann!]") {
		t.Fatalf("typing after the toolbar: %q", got)
	}
}
