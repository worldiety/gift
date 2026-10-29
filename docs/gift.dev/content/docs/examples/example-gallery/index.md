---
title: Image Gallery
weight: 3
---

This example is a small photo browser built around [`ui.ImageGallery`](../../components/lists/image-gallery/).
It shows a virtualized gallery with the complete image pipeline behind it: pictures from files and over HTTP,
cold loading, decode errors, and a thumbnail cache on disk that survives a restart.

The window has a toolbar and the gallery. The toolbar shows the number of photos and the pipeline counters:
how many pictures were decoded, how many came from the disk cache, and how many failed. A goroutine checks four
times a second whether the counters or the selection differ from what the toolbar shows and rebuilds it only
then, so the counters keep up with the pictures as they arrive. Two buttons switch
between a masonry layout (`ui.Masonry`) and a justified layout (`ui.Justified`) at runtime.

{{< gallery-shot alt="A dark photo browser with a toolbar and a masonry grid of gradient pictures, one of them selected" >}}

The example uses its own dark palette, because the colors are part of a photo viewer's design. It looks the
same in the light and the dark theme.

## What to try

- Scroll with the mouse wheel, or drag the pictures. A fast drag flings, and the gallery keeps going.
- Click a tile to select it. Shift-click selects a range. Control-click or Command-click adds or removes
  one tile. The toolbar shows the selection count and the ID of the last picked entry, as long as that entry is
  still selected.
- After a click the gallery has the focus. Use the arrow keys, Page Up, Page Down, Home and End to move.
  Space toggles the selection of the current tile.
- Switch between **Masonry** and **Justified**.
- Look for the red tiles. With the generated samples, two entries are broken on purpose: `broken.png` exists but
  is not a picture, and `missing.jpg` does not exist. The example adds `missing.jpg` with `-dir` as well. A failed source becomes a visible tile state, not a broken frame.
- Run the example twice. The first run decodes the pictures, the second run answers almost all of them from the
  thumbnail cache. The "from disk" counter shows the cache hits.

## How it works

The program builds an `asset.Collection` of metadata and a `ui.Gallery` once, outside the view function. It
creates an `asset.Pipeline` that delivers its results through `app.Post` and keeps thumbnails in a disk cache,
and hands it to `ui.SetImagePipeline`. Every second picture is served by a small HTTP file server on the
loopback interface, so both `asset.File` and `asset.HTTP` sources are in use without a network.

The view function holds only two pieces of state: the chosen layout and the last selected ID. The pipeline
counters and the selection are not state, so the toolbar records what it showed, and the refresh goroutine posts
a comparison to the UI goroutine and calls `app.Invalidate` when they differ. There is no view
and no state per picture. Scrolling does not run the view function at all. The gallery binds a pool of tiles to
the visible entries and decodes the band just outside the viewport as prefetch (`Overscan`).

Read [Images](../../concepts/images/) for the pipeline and [Concurrency](../../concepts/concurrency/) for why
results come back through `app.Post`.

## Run it

From the repository root:

```sh
go run ./cmd/example-gallery
```

Without `-dir` the example writes 48 generated pictures and `broken.png` into `gift-gallery/samples` in the user
cache directory, and shows those. It writes a picture only if the file is not there yet. The `-cache` flag does not
move this folder.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-dir` | `$GIFT_GALLERY_DIR` | Directory of JPEG and PNG pictures. Empty generates the samples. |
| `-cache` | `gift-gallery` in the user cache directory | Persistent thumbnail cache directory. Empty disables the disk cache. |

To browse your own pictures:

```sh
go run ./cmd/example-gallery -dir ~/Pictures
```

The example passes the pipeline counters to the backend, so a build with metrics shows the decode, disk cache
and budget numbers in the measurement report:

```sh
GIFT_METRICS=1 go run -tags giftmetrics ./cmd/example-gallery
```

See [Diagnostics](../../concepts/diagnostics/) for the measurement report.

## Source

{{< example-code >}}

## Related

- [Image Gallery](../../components/lists/image-gallery/)
- [Image](../../components/basic/image/)
- [Button](../../components/basic/button/)
- [Stack](../../components/layout/stack/) and [ZStack](../../components/layout/zstack/)
- [Images](../../concepts/images/), [State](../../concepts/state/), [Input and Focus](../../concepts/input-and-focus/)
