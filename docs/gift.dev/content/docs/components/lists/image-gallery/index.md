---
title: Image Gallery
---

An image gallery shows a large catalogue of pictures as a scrolling grid of tiles. Use it for a photo library,
a media browser or any screen with hundreds to hundreds of thousands of pictures. For a single picture, use
[image](../../basic/image/); for a scrolling list of rows, use the [virtual list](../virtual-list/).

The gallery is virtualized. It keeps a small pool of tiles, about as many as fit into the viewport, and rebinds
them to other entries while you scroll. No view function runs during a scroll, and the number of tiles does not
grow with the catalogue. It arranges the tiles as masonry columns or as justified rows, keeps a selection and a
keyboard cursor, and loads its thumbnails through the same image pipeline as `ui.Image`.

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

### Two halves: Gallery and GalleryView

A gallery has a retained half and a declaration half. `ui.NewGallery` returns a `*ui.Gallery`: the catalogue,
the selection, the layout index and the tile pool. Create it once and keep it, next to the
`asset.Collection` it wraps. `ui.ImageGallery(g)` returns a `GalleryView` that says how the gallery looks right
now. Build that in your view function like any other view.

The catalogue is an `asset.Collection` of `asset.Metadata`: a stable ID per entry and, if known, the pixel
dimensions. It holds no sources. `Gallery.SetSources` installs a resolver that turns an ID into an
`asset.Source`. Without a resolver, every tile shows its placeholder.

```go
sources := make(map[asset.ID]asset.Source, len(paths))
items := make([]asset.Metadata, 0, len(paths))
for _, p := range paths {
	src := asset.File(p)
	m := src.Metadata()
	sources[m.ID] = src
	items = append(items, m)
}

photos := ui.NewGallery(asset.NewCollection(items))
photos.SetSources(func(id asset.ID) asset.Source { return sources[id] })

app := gift.New(gift.Options{Root: func(ctx *gift.Context) gift.View {
	return ui.Window(library(ctx, photos))
}})
```

The collection takes ownership of the slice. A duplicate or empty ID panics. The resolver runs during layout,
once per newly bound tile, so it must not block or perform I/O; a map lookup or `asset.File` is fine.
`library` stands for your own view function, such as the one in the next snippet.

Thumbnails come from the pipeline you install with `ui.SetImagePipeline`; see
[image](../../basic/image/#install-the-pipeline) for the wiring. The gallery requests each thumbnail at the size
of its tile in device pixels. A tile shows its placeholder until the picture arrives. Without a pipeline, every
tile stays on its placeholder.

### Give it a bounded height

Like a scroll view, a gallery needs a bounded height. Use `Flex(1)` to fill the rest of a stack, or `Frame` or
`MaxHeight`. A gallery with an unbounded height has a zero viewport, binds no tiles and stays blank.

`Padding` lies inside the viewport and scrolls with the tiles. `Background`, `Border` and `CornerRadius` style
the viewport and do not scroll. The gallery draws the same scroll bar as a scroll view; `ScrollBar` changes it.

### Layouts

`ui.Masonry()` uses columns of equal width and places each item into the currently shortest column.
`MinColumnWidth` sets the narrowest acceptable column, 160 by default, and the gallery fits as many columns as
it can.
`ui.Justified()` fills each row to the full width and gives every item in a row the same height. `RowHeight`
sets the height it aims for, 200 by default; the real height varies per row. `Gap` sets the space between
tiles for both and is zero by default. Without `Layout`, the gallery uses masonry with these defaults.

Entries without known dimensions are laid out with `ProvisionalAspect`, which defaults to 1. When the pipeline
has probed a file, the gallery folds the real dimensions back in, in batches, and keeps the viewport on the
picture it shows. `AspectClamp` bounds extreme aspect ratios, by default to between 0.2 and 5.

A `GalleryLayout` is a plain value, so you switch layouts with ordinary state. The gallery notices the change
and reflows incrementally, discarding a reflow that is still running:

```go
justified := ctx.State("justified", false)

arrangement := ui.Masonry().MinColumnWidth(220).Gap(8)
if ctx.Read(justified) {
	arrangement = ui.Justified().RowHeight(180).Gap(8)
}

return ui.VStack(
	ui.Button(ui.Text("Switch layout"), func() { justified.Set(!justified.Get()) }),
	ui.ImageGallery(photos).Layout(arrangement).Flex(1),
)
```

### Selection and keyboard

The selection is an `asset.Selection` that belongs to the `Gallery`, not to a tile. It stores stable IDs, so it
follows the picture through scrolling, recycling and reordering. Read it with `Gallery.Selection()`, for
example `Len()` or `IDs(nil)`, or change it from code with `Only`, `Set`, `Toggle` and `SetCursor`.
`Gallery.SetSelection` shares one selection object between two galleries of the same catalogue.

| Input                           | Effect                                               |
|---------------------------------|------------------------------------------------------|
| Click                           | Selects the entry alone and moves the cursor to it.  |
| Shift-click                     | Selects the range from the anchor to the entry.      |
| Control-click or Command-click  | Toggles the entry and moves the cursor to it.        |
| Arrow keys, Page Up/Down, Home, End | Move the cursor and select the entry alone.      |
| Shift with a navigation key     | Extends the selection as a range.                    |
| Control or Command with a navigation key | Moves the cursor without selecting.         |
| Space                           | Toggles the entry under the cursor.                  |
| Enter                           | Activates the entry under the cursor.                |

A click into the gallery gives it the focus, so the keys work after the first click. Navigation follows the
catalogue order: left and right move one entry, up and down move one row's worth of entries. A drag that starts
on a tile scrolls and selects nothing.

`OnSelect` receives the ID of the entry after every click on a tile, including Shift- and Control-clicks, after
a navigation key without a modifier, and after Space selects an entry. Shift or Control with a navigation key
does not call it. `OnActivate` receives the ID of the entry Enter activated; a double click is not an activation. Both pass an
`asset.ID`, never a tile or an index. A click on a tile only repaints, and no view function runs unless your
`OnSelect` changes state: the tiles read the selection when they paint.

### Tiles

`Tile` takes a `TileStyle`. `CornerRadius` rounds the placeholder, the picture and both borders alike.
`Selected` is stroked inside a selected tile, `Cursor` inside the tile with the keyboard cursor. `Error` fills
a tile whose picture failed to load, and `Provisional` a tile whose dimensions are still a guess. `Palette` is
the set of placeholder fills; the entry is picked from the stable ID, so a tile keeps its color when it is
recycled.

Unlike the other colors, `Palette` entries are not resolved against the theme. Use literal colors there, such
as `ui.RGB`, and semantic colors such as `ui.ColorAccent` for the borders.

### Prefetch and introspection

`Overscan` keeps tiles mounted for a number of logical pixels above and below the viewport. Their thumbnails are
requested as prefetch, which never delays a visible picture. It is zero by default; about one row's height
suits a gallery that is scrolled fast.

`Gallery.Reveal` scrolls an entry into view on the next layout pass, and `Gallery.Reorder` applies a new
catalogue order while keeping the viewport on the current picture. `Bindings` and `BindingOf` return
`TileBinding` values that say which entry each tile stands for, and `Stats` returns request counters. Both are
meant for tests and diagnostics.

## API

{{< api "GalleryView" >}}

{{< api "Gallery" >}}

{{< api "GalleryLayout" >}}

{{< api type="TileStyle" decl="true" >}}

{{< api type="TileBinding" decl="true" >}}

## Related

- [Image](../../basic/image/) shows one picture through the same pipeline.
- [Virtual List](../virtual-list/) virtualizes rows of arbitrary views.
- [Scroll View](../../layout/scroll-view/) for the scroll gestures and the scroll bar.
- [Images](../../../concepts/images/) explains the pipeline, the thumbnail sizes and the disk cache.
- [Input and Focus](../../../concepts/input-and-focus/) for focus and keyboard handling.
- [Gallery example](../../../examples/example-gallery/) is a complete photo browser with a layout switch.
