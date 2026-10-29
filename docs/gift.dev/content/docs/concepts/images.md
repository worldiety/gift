---
title: Images
weight: 8
---

A view in gift never loads a picture itself. [`ui.Image`](../../components/basic/image/) and the
[image gallery](../../components/lists/image-gallery/) only say *which* picture to show and how large. One
application wide service, the `asset.Pipeline`, does the rest in the background: it reads the file or the URL,
decodes it, applies the EXIF orientation, scales it to a thumbnail and caches the result in memory and on disk.
Until a picture is ready, the view draws a placeholder. Nothing in a frame waits for I/O or a decode.

This page explains how to set up the pipeline, where pictures come from, what is cached and bounded, and what to
do when a picture fails or changes.

## How a picture reaches the screen

The package [`asset`](https://pkg.go.dev/github.com/worldiety/gift/asset) holds everything that is not drawing.
It imports nothing from `gift` or `ui`, so you can test it without a window.

| Piece | What it is |
|-------|------------|
| `asset.Source` | Where the bytes come from. `asset.File` and `asset.HTTP` are built in; you can write your own. |
| `asset.Pipeline` | The service that fetches, decodes, scales and caches. One per application. |
| `asset.Collection` | An ordered catalog of stable IDs, the model behind a gallery. |
| `ui.Image`, `ui.ImageGallery` | The views. They draw through the pipeline installed with `ui.SetImagePipeline`. |

A request for a picture goes through these stages:

1. The view asks for the picture during layout, at the size it is drawn at.
2. A worker goroutine establishes the revision of the source, a stat for a file, and checks the memory cache and
   the disk cache for a thumbnail of that revision.
3. On a miss, the worker reads the bytes, decodes them, scales the picture to a thumbnail and orients it.
4. The result is delivered to the UI goroutine through `Config.Deliver`.
5. The backend uploads the thumbnail to the GPU, a few per frame, and the view draws it instead of its placeholder.

## Set up the pipeline

Create one pipeline in `main`, wire its `Deliver` field to `app.Post` and install it with `ui.SetImagePipeline`:

```go
func main() {
	app := gift.New(gift.Options{Root: root})

	cache, err := os.UserCacheDir()
	if err != nil {
		panic(err)
	}
	pipe := asset.NewPipeline(asset.Config{
		Deliver: app.Post, // results run on the UI goroutine
		Disk:    asset.DiskCacheConfig{Dir: filepath.Join(cache, "myapp", "thumbnails")},
	})
	defer func() {
		pipe.Close()     // cancels what is in flight
		app.DrainPosts() // runs the closures Close delivered
	}()
	ui.SetImagePipeline(pipe)

	if err := backend.Run(app, backend.Config{Title: "Photos", Width: 1024, Height: 768}); err != nil {
		panic(err)
	}
}
```

Three rules matter here:

- **`Deliver` must be `app.Post`.** The pipeline runs its own worker goroutines and never touches user interface
  state. It hands every result to `Deliver` as a closure, and `app.Post` runs that closure on the UI goroutine at
  the start of the next update. A nil `Deliver` calls the closure on the worker goroutine, which is only meant for
  tests of the `asset` package.
- **Close, then drain.** `Close` stops the workers and answers every request still in flight with
  `asset.ErrClosed`. Each delivered closure releases a reference to a thumbnail, so these closures still have to
  run. The frame loop has stopped by then, so call `app.DrainPosts` once after `Close`.
- **One pipeline per application.** `ui.Image` and every gallery share it, so a picture shown twice at the same
  size is fetched, decoded and uploaded once. `ui.SetImagePipeline` is process wide. Installing a different
  pipeline drops every texture and every request state that came from the old one. Passing nil detaches the
  pipeline, and every image then draws its placeholder. That is also what a layout test without a pipeline sees.

The zero `asset.Config` is usable: every field documents the default that zero selects. An empty `Disk.Dir`
switches the disk cache off.

## Sources

`asset.File` and `asset.HTTP` turn a path or a URL into a source. Neither performs any I/O, so you can call them
inside a view function or while building a list of thousands of pictures:

```go
ui.Image(asset.File("photos/beach.jpg"))
ui.Image(asset.HTTP("https://example.com/covers/42.jpg"))
```

Every source has a stable **ID** and, once it has been checked, a **revision**. The ID says which picture it is.
The revision says which version of its bytes. Both go into the cache key, so a changed file gets new thumbnails
and an unchanged one is never decoded twice.

### Files

The ID of `asset.File` is the cleaned absolute path with a `file://` scheme. A relative path is resolved against
the working directory at the time of the call. The media type is guessed from the extension: `.jpg`, `.jpeg` and
`.jpe` are JPEG, `.png` is PNG.

Before every request the pipeline stats the file. The revision is the modification time and the size, so an
overwritten file is noticed on the next request without any configuration.

### HTTP

`asset.HTTP` fetches with a GET request through `http.DefaultClient`, or through the client you pass to
`WithClient`. The pipeline remembers the revision from the `ETag` or `Last-Modified` header and trusts it for as
long as the response allows:

- A `max-age` in `Cache-Control` sets the time, capped at 24 hours (`asset.MaxHTTPFreshness`).
- `no-store`, `no-cache` and `must-revalidate` force a revalidation every time.
- Without a `Cache-Control` header, an `Expires` header sets the time, with the same cap.
- Otherwise the revision is trusted for five minutes (`asset.DefaultHTTPFreshness`).

While the revision is fresh, the cached thumbnail is served without touching the network. Afterwards the pipeline
sends one conditional request, and a `304 Not Modified` answer costs no decode. A response with neither `ETag` nor
`Last-Modified` has no revision, and its thumbnail is not written to the disk cache.

URLs often carry credentials, so the ID of an HTTP source is redacted: it keeps the scheme, host, port and path,
drops user info and fragment, and replaces the query by a hash. Name harmless query parameters with
`WithPublicQuery` to keep them readable in logs. Send a token in a header with `WithHeader`; for `Authorization`,
`Proxy-Authorization`, `Cookie` and `X-Api-Key`, a hash of the value also separates the cache entries of different
users:

```go
asset.HTTP("https://api.example.com/v1/photos/42/thumbnail").
	WithHeader("Authorization", "Bearer "+token)

asset.HTTP("https://cdn.example.com/photo.jpg?w=800&sig=abc").
	WithPublicQuery("w") // "w" stays legible, "sig" does not
```

{{< callout type="warning" >}}
The path of a URL is never redacted. It appears in the ID, in log lines and in error messages. Never put a
credential into a path segment.
{{< /callout >}}

### Your own source

Implement `asset.Source` to show pictures from anywhere else, for example from files embedded in the binary:

```go
//go:embed art
var pictures embed.FS

// embedSource is a picture compiled into the binary.
type embedSource struct {
	name    string // path inside pictures
	version string // changes when the embedded files change, e.g. the build version
}

func (s embedSource) Metadata() asset.Metadata {
	return asset.Metadata{ID: asset.ID("embed:///" + s.name), MIMEType: asset.MIMEPNG}
}

func (s embedSource) Open(ctx context.Context) (io.ReadCloser, error) {
	return pictures.Open(s.name)
}

// Probe gives the picture a revision, which makes it eligible for the disk cache.
func (s embedSource) Probe(ctx context.Context) (asset.ProbeResult, error) {
	fi, err := fs.Stat(pictures, s.name)
	if err != nil {
		return asset.ProbeResult{}, err
	}
	return asset.ProbeResult{Revision: s.version, Size: fi.Size(), MIMEType: asset.MIMEPNG}, nil
}
```

`Metadata` must not block and must not perform I/O; the ID must be non-empty and stable across restarts. `Open`
returns a fresh reader every time, and the pipeline closes it. A source with only these two methods can be shown
but not cached on disk, because nothing tells the pipeline which version of the bytes it has. Add `Probe`
(the `asset.Prober` interface) when a revision is cheap to find out. Report the size in bytes there too if you
know it: the pipeline reserves its input budget from it before reading. A source that only learns its revision by
downloading, like HTTP, implements `asset.Fetcher` instead.

If `Metadata` already knows the width and height, report them. They must be the dimensions *after* the EXIF
orientation. The view then has the right aspect ratio from the first frame.

## Thumbnails, not originals

The pipeline never hands the original picture to the screen. It produces thumbnails whose longest edge is one of
a few fixed sizes, the **ladder** in `Config.Sizes`. The default ladder is 128, 256 and 512 pixels.

A view asks for the size it is drawn at, in device pixels. An image whose longest edge is 200 logical pixels asks
for 400 on a display with density 2 and gets the 512 rung. A cached rung within a factor of 1.25 (`Config.Hysteresis`) of the
requested size is reused, so resizing a window by a few pixels never decodes anything again. A picture smaller
than its rung is not enlarged.

The largest rung caps the resolution. With the default ladder, a picture drawn larger than 512 device pixels is
magnified and looks soft. If you show large pictures, for example full-screen on a kiosk, extend the ladder:

```go
pipe := asset.NewPipeline(asset.Config{
	Deliver: app.Post,
	Sizes:   []int{128, 256, 512, 1024, 2048},
})
```

Each rung is a separate cache entry, and a large rung costs memory: a 2048 by 1536 thumbnail takes 12 MiB. Raise
`PixelBudget` along with the ladder. `ImageView.Size` asks for a fixed number of device pixels instead of the
measured size.

The pipeline applies the EXIF orientation of a JPEG before it reports dimensions, so a portrait photo from a
phone has a portrait aspect ratio. It scales first and rotates the small result.

## Caches

A picture can be in three caches, and each has its own limit:

| Cache | Holds | Bounded by |
|-------|-------|------------|
| CPU memory | decoded thumbnails, premultiplied RGBA | `Config.PixelBudget`, 64 MiB by default |
| Disk | thumbnails in a raw format, one file per entry | `DiskCacheConfig.Budget`, 256 MiB by default |
| GPU | textures, one per picture, rung and revision | fixed in the Ebitengine backend, 128 MiB of pixel bytes |

The **disk cache** is what makes a second start fast. Its key contains the picture's ID, revision, rung, EXIF
orientation, a hash of its credentials and `asset.ProcessingVersion`, which changes whenever a gift update would produce different pixels.
A source without a revision is never written to disk. When a write would exceed the budget, the least recently
used entries are deleted. Entries are checked with a CRC, and a file system error never fails a request: a cache
that cannot be written is only slower.

The **GPU textures** are uploaded at most eight per drawn frame and at most 2 MiB per drawn frame, so a screenful
of newly decoded pictures appears over a few frames instead of stalling one. A single picture larger than 2 MiB is
still uploaded and overshoots the budget once. A texture that has not been drawn
for 600 drawn frames is released.

## Budgets and limits

Every stage of the pipeline that holds pixels or encoded bytes is bounded in bytes, not only in count, because eight
queued thumbnails and eight queued 24 megapixel decodes are very different amounts of memory. The defaults are sized for a Raspberry Pi 4:

| Field of `asset.Config` | Default | Bounds |
|-------------------------|---------|--------|
| `Workers` | min(4, `GOMAXPROCS`) | decode goroutines |
| `MaxPixels` | 64 megapixels | the declared size of one picture |
| `MaxEncodedBytes` | 64 MiB | the encoded size of one picture |
| `InputBudget` | 96 MiB | encoded bytes held by all workers |
| `DecodeBudget` | 256 MiB | decoder working memory of all workers |
| `PixelBudget` | 64 MiB | every live thumbnail, cached or on its way to the GPU |
| `QueueLimit` | 512 | requests waiting for a worker |
| `ReadyLimit` | 64 | finished results waiting for the UI goroutine |
| `Timeout` | 30 s | one request, end to end; negative disables it |

A picture whose header declares more than `MaxPixels`, or more pixels than the whole decode budget could hold, is
refused with `asset.ErrTooLarge` before a single pixel is allocated. That is what keeps a 60000 by 60000 PNG from
killing the process. When a budget is merely full, the worker waits.

When the queue is full, a new prefetch request is refused and a new visible request evicts the oldest waiting
prefetch. Nothing grows without bound and nothing is dropped silently. `pipe.Stats()` reports every budget with its
high water mark.

## Priority and prefetch

The pipeline knows two priorities. `asset.Visible` is for a picture on the screen, `asset.Prefetch` for one that
may be soon. A visible request always overtakes waiting prefetches, and a queued prefetch is promoted when its
picture becomes visible.

You rarely set this yourself:

- An image inside a hidden subtree, such as an inactive tab of a [tab bar](../../components/navigation/tab-bar/),
  is requested as prefetch. It loads, but never ahead of what the user sees.
- The gallery requests the tiles in its `Overscan` band as prefetch, so the next row is decoded before you scroll
  to it.

Requests for the same picture and rung are deduplicated: a second request joins the running one and gets its own
result.

## Errors and retries

Failures are values, never panics. A broken picture becomes a visible state of its view, not a broken frame:

- `ui.Image` keeps drawing its placeholder.
- A gallery tile is filled with the `Error` color of its `TileStyle`.

The pipeline distinguishes two kinds of failure. A failure that a retry cannot fix, such as a missing file, a
`404`, a file that is not a picture or one over the limits, puts the source into quarantine. It is not tried again
until its revision changes or you call `pipe.Forget`. A failure that a retry might fix, such as a timeout, a `503`
or a dropped connection, is retried with an exponential delay from 500 ms up to 30 s. After three such failures the
source is quarantined too. `Config.Backoff` changes these numbers.

In the pipeline, a local file that was missing recovers by itself once it appears, because it then has a new
revision. The view, however, remembers a final failure, and it also keeps a picture it has loaded once. To show a
picture again after a failure, or to show a file that changed on disk, tell both sides:

```go
func photo(ctx *gift.Context) gift.View {
	src := asset.File("/var/lib/kiosk/today.jpg")
	reloads := ctx.State("reloads", 0)
	ctx.Read(reloads) // rebuild this component when reloads changes

	return ui.VStack(
		ui.Image(src).Frame(480, 320).Fit(ui.FitCover),
		ui.Button(ui.Text("Reload"), func() {
			id := src.Metadata().ID
			if p := ui.ImagePipeline(); p != nil {
				p.Invalidate(id) // drop the cached thumbnails
				p.Forget(id)     // clear a failure or quarantine
			}
			ui.ForgetImage(id) // drop the view state and the textures
			reloads.Set(reloads.Get() + 1)
		}),
	)
}
```

`ui.ForgetImage` must run on the UI goroutine, for example in an event handler. It does not schedule a layout by
itself; the state change above rebuilds the component, and the image asks the pipeline again. `Invalidate` keeps
the disk entries, which are keyed by revision: a new revision simply writes a new entry.

## Galleries and collections

For hundreds or thousands of pictures, use the [image gallery](../../components/lists/image-gallery/) instead of
a stack of images. Its model is an `asset.Collection`: an ordered list of `asset.Metadata` with stable, unique IDs.
The catalog holds no sources and no views, only IDs and what is known about each picture. A resolver installed
with `SetSources` turns an ID into a source when a tile needs it:

```go
func newLibrary(dir string) (*ui.Gallery, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var items []asset.Metadata
	byID := make(map[asset.ID]asset.Source)
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || (ext != ".jpg" && ext != ".jpeg" && ext != ".png") {
			continue
		}
		src := asset.File(filepath.Join(dir, e.Name()))
		m := src.Metadata()
		items = append(items, m)
		byID[m.ID] = src
	}
	g := ui.NewGallery(asset.NewCollection(items))
	g.SetSources(func(id asset.ID) asset.Source { return byID[id] })
	return g, nil
}
```

Create the `ui.Gallery` once and keep it, then describe it in a view function with `ui.ImageGallery(g)`. A few
things to know:

- `asset.NewCollection` takes ownership of the slice. Do not modify it afterwards. A duplicate or empty ID panics.
- An entry with unknown dimensions is laid out with a provisional shape. When the pipeline has probed the picture,
  the gallery folds the real dimensions back into the collection in one batch per layout pass and keeps the
  scroll position on the picture you were looking at.
- The resolver runs during layout and must not block. A resolver that returns nil leaves the tile on its
  placeholder.
- Selection and keyboard cursor live in an `asset.Selection` and are expressed in IDs, so sorting with `Reorder`
  keeps the same pictures selected.

A `Collection` and a `Selection` belong to the UI goroutine and are not safe for concurrent use. Build the list of
entries on another goroutine if you like, then hand it over with `app.Post` and call `SetCollection` there. The
[gallery example](../../examples/example-gallery/) shows the whole setup with files, HTTP and deliberate failures.

## Faster JPEG decoding with libjpeg-turbo

Camera photos of 12 to 50 megapixels are slow to decode with Go's `image/jpeg`, which always decodes the whole
picture. The package [`asset/turbojpeg`](https://pkg.go.dev/github.com/worldiety/gift/asset/turbojpeg) uses
libjpeg-turbo instead. It is several times faster and decodes straight to 1/2, 1/4 or 1/8 of the stored size when
only a thumbnail is needed.

It loads the library at run time without cgo. Install `libturbojpeg0` on Debian or Raspberry Pi OS, or
`jpeg-turbo` with Homebrew on macOS, and register the decoder once at startup:

```go
if turbojpeg.Register() {
	asset.RegisterDecoder(asset.MIMEJPEG, turbojpeg.Decoder{})
}
```

`Register` only loads the library and reports whether that worked; it registers nothing by itself. gift never
switches decoders on its own, because the two decoders do not produce identical pixels and your thumbnails should
not depend on which packages happen to be installed. Without the library, `Register` returns false and
`image/jpeg` stays in charge. CMYK JPEGs are always decoded by `image/jpeg`.

## Other formats

The pipeline decodes JPEG and PNG. `asset.RegisterDecoder` adds a format process wide; call it during startup,
before the pipeline runs. A decoder that also implements `Sniff` (the `asset.Sniffer` interface) is chosen by the
first bytes of the file. Without it, a decoder is only chosen by media type, and `asset.File` only knows the media
types of JPEG and PNG. This adds WebP from `golang.org/x/image/webp`:

```go
type webpDecoder struct{}

func (webpDecoder) DecodeConfig(r io.Reader) (image.Config, error) { return webp.DecodeConfig(r) }
func (webpDecoder) Decode(r io.Reader) (image.Image, error)        { return webp.Decode(r) }
func (webpDecoder) MemoryFactor() float64                          { return 8 } // an estimate, bytes per pixel

// Sniff recognizes WebP by its header, so files with any extension work.
func (webpDecoder) Sniff(h []byte) bool {
	return len(h) >= 12 && bytes.Equal(h[0:4], []byte("RIFF")) && bytes.Equal(h[8:12], []byte("WEBP"))
}

func init() {
	asset.RegisterDecoder("image/webp", webpDecoder{})
}
```

`MemoryFactor` is the working memory per pixel of the full picture that the pipeline reserves from
`DecodeBudget` before the decode. Overestimate rather than underestimate. A decoder is called from several
goroutines at once and must be safe for concurrent use. EXIF orientation is only read from JPEG.

## Watching the pipeline

`pipe.Stats()` returns a snapshot of all counters from any goroutine: requests, deduplications, decodes, memory
and disk hits, failures, and every budget with its peak. The [gallery example](../../examples/example-gallery/)
shows decodes against disk hits in its toolbar, so a second run visibly comes from the cache.

To include the counters in the measurement report of the Ebitengine backend, pass `ui.ImagePipelineStats`:

```go
err := backend.Run(app, backend.Config{
	Title: "Photos", Width: 1024, Height: 768,
	AssetStats: ui.ImagePipelineStats,
})
```

See [Diagnostics](../diagnostics/) for the measurement report.

## API

{{< api-funcs pkg="ui" names="SetImagePipeline,ImagePipeline,ForgetImage,ImagePipelineStats" >}}

{{< api pkg="asset" type="Pipeline" >}}

{{< api pkg="asset" type="FileSource" >}}

{{< api pkg="asset" type="HTTPSource" >}}

{{< api pkg="asset" type="Collection" >}}

{{< api-funcs pkg="asset" names="RegisterDecoder,Retryable" >}}

The fields of `asset.Config`, `asset.DiskCacheConfig` and `asset.BackoffPolicy` are documented in package
[asset](https://pkg.go.dev/github.com/worldiety/gift/asset#Config).

## Related

- [Image](../../components/basic/image/): one picture, with fit, placeholder and rounded corners
- [Image gallery](../../components/lists/image-gallery/): a virtualized gallery over an `asset.Collection`
- [Concurrency](../concurrency/): `app.Post` and the UI goroutine
- [Icons](../icons/): monochrome symbols, which do not go through the pipeline
- [Kiosks](../kiosks/): running on a Raspberry Pi
- [Gallery example](../../examples/example-gallery/): a photo browser with files, HTTP and a persistent cache
