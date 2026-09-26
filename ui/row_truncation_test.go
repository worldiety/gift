package ui_test

import (
	"slices"
	"testing"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// rowWidth is the width the rows in this file are laid out at: a card on a
// portrait panel, which is where the photo booth that found these defects put
// them.
const rowWidth = 360

// rowHarness lays v out as the only child of a column rowWidth wide, which
// measures it with a bounded width exactly like a card or a list does.
func rowHarness(t *testing.T, v ui.RowView) *gifttest.Harness {
	t.Helper()
	return gifttest.New(t, gifttest.Options{
		View: ui.VStack(v.Key("row")).Frame(rowWidth, geom.Unbounded()),
		Size: geom.Sz(600, 200),
		Font: loadTestFont(t),
	})
}

// glyphIDsOf returns the ids of glyphs, in order.
func glyphIDsOf(gs []render.Glyph) []render.GlyphID {
	out := make([]render.GlyphID, len(gs))
	for i, g := range gs {
		out[i] = g.ID
	}
	return out
}

// TestALongRowValueEndsInAnEllipsisInsideTheRow is the photo booth's settings
// screen: the value of a "storage location" row is a path on a NAS, and it
// ran over the edge of the card. It now ends in an ellipsis before the row's
// padding, while the short title next to it keeps every character.
func TestALongRowValueEndsInAnEllipsisInsideTheRow(t *testing.T) {
	const path = "/Volumes/photos/2026/hochzeit-anna-bernd/originals"
	h := rowHarness(t, ui.Row("Pfad").Value(path))

	row := h.Find(gifttest.ByKey("row")).Bounds()
	if row.Width() > rowWidth {
		t.Fatalf("the row is %v wide in a column of %v", row.Width(), rowWidth)
	}
	value := h.Find(gifttest.ByKey("value"))
	if vb := value.Bounds(); vb.Max.X > row.Max.X-ui.RowPadding {
		t.Errorf("the value ends at x %v, past the row's trailing padding at %v", vb.Max.X, row.Max.X-ui.RowPadding)
	}
	ids := glyphIDsOf(value.Glyphs())
	if !slices.Contains(ids, glyphsOf(t, "…")[0]) {
		t.Errorf("the cut value drew no ellipsis: %v", ids)
	}
	if got, want := glyphIDsOf(h.Find(gifttest.ByKey("title")).Glyphs()), glyphsOf(t, "Pfad"); !slices.Equal(got, want) {
		t.Errorf("the short title drew %v, want all of \"Pfad\" %v; it needs less than its half and must keep it", got, want)
	}
	// Exactly one overflowing node: the value, reporting what its ellipsis
	// hides. Before, the row's content stack and the column around it
	// overflowed as well, because the value kept its natural width.
	if n := h.Diagnostics().OverflowNodes; n != 1 {
		t.Errorf("%d nodes overflow, want exactly the cut value.\n%s", n, h.Dump())
	}
}

// TestARowThatFitsIsNotCut: the change must not touch a row with room, which
// is every row the goldens show.
func TestARowThatFitsIsNotCut(t *testing.T) {
	h := rowHarness(t, ui.Row("Wi-Fi").Subtitle("kiosk-net").Value("connected"))
	h.AssertNoOverflow()
	row := h.Find(gifttest.ByKey("row")).Bounds()
	if got := h.Find(gifttest.ByKey("value")).Bounds().Max.X; got != row.Max.X-ui.RowPadding {
		t.Errorf("the value ends at %v, want the trailing edge %v", got, row.Max.X-ui.RowPadding)
	}
	if got, want := glyphIDsOf(h.Find(gifttest.ByKey("value")).Glyphs()), glyphsOf(t, "connected"); !slices.Equal(got, want) {
		t.Errorf("the value drew %v, want %v", got, want)
	}
}

// TestTheTitleGivesWayBeforeAWideAccessoryOverflows is the third defect: a row
// with an HStack of chips as its accessory ran the chips over the edge of the
// card. The title is cut now, and the accessory keeps its width and ends at
// the row's padding.
func TestTheTitleGivesWayBeforeAWideAccessoryOverflows(t *testing.T) {
	chip := func(k string) ui.BoxView { return ui.Box().Frame(70, 24).Background(ui.RGB(9, 9, 9)).Key(k) }
	chips := ui.HStack(chip("a"), chip("b"), chip("c")).Gap(6).Key("chips")
	h := rowHarness(t, ui.Row("Hochzeit Anna & Bernd").Accessory(chips))

	row := h.Find(gifttest.ByKey("row")).Bounds()
	cb := h.Find(gifttest.ByKey("chips")).Bounds()
	if cb.Width() != 3*70+2*6 {
		t.Errorf("the accessory is %v wide; it must keep its natural %v", cb.Width(), 3*70+2*6)
	}
	if cb.Max.X > row.Max.X-ui.RowPadding {
		t.Errorf("the accessory ends at %v, past the row's trailing padding at %v", cb.Max.X, row.Max.X-ui.RowPadding)
	}
	title := h.Find(gifttest.ByKey("title"))
	if tb := title.Bounds(); tb.Max.X > cb.Min.X {
		t.Errorf("the title ends at %v, under the accessory starting at %v", tb.Max.X, cb.Min.X)
	}
	if ids := glyphIDsOf(title.Glyphs()); !slices.Contains(ids, glyphsOf(t, "…")[0]) {
		t.Errorf("the cut title drew no ellipsis: %v", ids)
	}
}

// TestAnAccessoryWiderThanTheRowStillOverflows is the limit the documentation
// of RowView states: the labels can give way, the accessory cannot, and what is
// left over is reported rather than hidden.
func TestAnAccessoryWiderThanTheRowStillOverflows(t *testing.T) {
	h := rowHarness(t, ui.Row("Title").Accessory(ui.Box().Frame(rowWidth, 24).Key("wide")))
	if h.Diagnostics().OverflowNodes == 0 {
		t.Fatal("an accessory wider than the row reported no overflow")
	}
}
