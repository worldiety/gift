package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

func TestDroppedThingsAreListed(t *testing.T) {
	drops = nil
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(640, 520),
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family})})
	h.AssertExists(gifttest.ByTextContains("drops nothing"))

	dir := t.TempDir()
	f := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(f, make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file over the note still goes to the window.
	note := h.Find(gifttest.ByKey("note"))
	if !note.DropOnto(gift.DropFiles(f, dir)) {
		t.Fatal("the window did not take the files")
	}
	h.AssertExists(gifttest.ByText(f))
	h.AssertExists(gifttest.ByText("2.0 KB"))
	h.AssertExists(gifttest.ByText("folder"))

	if !note.DropOnto(gift.DropURLData("https://example.org")) {
		t.Fatal("the note did not take the address")
	}
	h.AssertExists(gifttest.ByText("https://example.org"))
	h.AssertNoOverflow()

	// The points the documentation's shot drags to, in docs/capture.
	h.DragOverAt(geom.Pt(145, 104), gift.DropText)
	note.AssertDropTargeted()
	h.DragLeave()
	h.DragOverAt(geom.Pt(320, 300), gift.DropFile)
	h.Find(gifttest.ByKey("window")).AssertDropTargeted()
	h.DragLeave()
}
