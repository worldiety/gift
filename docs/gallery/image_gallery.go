package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "image-gallery", Group: "lists", Width: 900, Height: 460, Settle: 40}, imageGalleryDemo)
}

// The retained halves of the two galleries. A Gallery outlives every build,
// so the demo keeps it in a variable rather than in the view function.
var masonryPhotos, justifiedPhotos *ui.Gallery

func imageGalleryDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// A Gallery is created once and kept, like the collection it wraps.
	// The catalogue holds metadata only; SetSources says where each
	// entry's pixels come from. pictures is a []asset.Source of
	// sample files.
	library := func() *ui.Gallery {
		items := make([]asset.Metadata, len(pictures))
		byID := make(map[asset.ID]asset.Source, len(pictures))
		for i, src := range pictures {
			items[i] = src.Metadata()
			byID[items[i].ID] = src
		}
		g := ui.NewGallery(asset.NewCollection(items))
		g.SetSources(func(id asset.ID) asset.Source { return byID[id] })
		return g
	}
	if masonryPhotos == nil {
		masonryPhotos, justifiedPhotos = library(), library()

		// The selection lives in the Gallery and follows stable IDs.
		ids := masonryPhotos.Collection()
		masonryPhotos.Selection().Only(ids.ID(1))
		masonryPhotos.Selection().Set(ids.ID(5), true)
		masonryPhotos.Selection().SetCursor(ids.ID(3), true)
	}

	bold := ui.MustFont(ui.FontQuery{Family: inter.Family, Weight: ui.WeightBold})
	tiles := ui.TileStyle{
		CornerRadius: 8,
		Palette:      []ui.Color{ui.RGB(120, 130, 150), ui.RGB(140, 128, 150)},
		Error:        ui.ColorDanger,
		Selected:     ui.Border{Width: 3, Color: ui.ColorAccent},
		Cursor:       ui.Border{Width: 2, Color: ui.ColorLabel},
	}
	panel := func(title string, gallery ui.GalleryView) gift.View {
		return ui.VStack(
			ui.Text(title).Font(bold),
			// A gallery needs a bounded height: Frame, MaxHeight or Flex.
			gallery.Tile(tiles).Padding(8).Frame(400, 340).
				Background(ui.ColorSurface).CornerRadius(12).
				Border(ui.Border{Width: 1, Color: ui.ColorSeparator}),
		).Gap(10)
	}

	return ui.HStack(
		panel("Masonry", ui.ImageGallery(masonryPhotos).
			Layout(ui.Masonry().MinColumnWidth(110).Gap(8)).
			OnSelect(func(id asset.ID) { /* show the details */ })),
		panel("Justified", ui.ImageGallery(justifiedPhotos).
			Layout(ui.Justified().RowHeight(96).Gap(8)).
			OnActivate(func(id asset.ID) { /* open the picture */ })),
	).Gap(24)
	// snippet:end
}
