package ui_test

import (
	"testing"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// TestAGalleryBindsTheTilesUnderItsPadding: the tiles scroll through the
// padding – under a floating header and a bottom bar – so every item whose
// rectangle reaches into any part of the node's height is bound, the bottom
// padding included, even with no overscan to hide a short band.
func TestAGalleryBindsTheTilesUnderItsPadding(t *testing.T) {
	const pad = 150
	g := ui.NewGallery(asset.NewCollection(synth(400)))
	gifttest.New(t, gifttest.Options{
		View: ui.VStack(
			ui.ImageGallery(g).Layout(ui.Masonry().MinColumnWidth(240).Gap(8)).
				Overscan(0.001).
				PaddingInsets(geom.Insets{Top: pad, Bottom: pad}).
				Flex(1),
		),
		Size: geom.Sz(800, 600),
	})
	if !g.Ready() {
		t.Fatal("no layout")
	}
	bound := map[int]bool{}
	for _, b := range g.Bindings(nil) {
		bound[b.Item] = true
	}
	// At offset zero the node shows document y from -pad to 600-pad.
	for i := range 400 {
		_, y, _, h, ok := g.ItemRect(i)
		if !ok {
			continue
		}
		if y < 600-pad && y+h > -pad && !bound[i] {
			t.Errorf("item %d at y %.0f..%.0f is on screen but not bound", i, y, y+h)
		}
	}
}
