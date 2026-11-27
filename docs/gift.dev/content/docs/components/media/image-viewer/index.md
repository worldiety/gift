---
title: Image Viewer
---

The image viewer shows the pictures of a catalogue full screen, one at a time, in the manner of Photos on iOS. It
grows out of the thumbnail that was tapped and shrinks back into it. In between the user swipes through the
pictures, zooms into them, drags one down to close the viewer and swipes up for a card of details. With a mouse,
arrows appear at the sides while it moves and fade when it rests; the keyboard pages, zooms and closes.

{{< gallery-shot alt="A portrait gradient picture full screen on black, with a close button at the top left" >}}

{{< gallery-code >}}

## Usage

The viewer is two things, like the [image gallery](../../lists/image-gallery/): a `*imageviewer.Viewer` that the
application creates once and keeps, and `imageviewer.Fullscreen(viewer)`, the view of it. The view belongs on top of
everything else, the last child of a `ZStack`. While the viewer is closed it is not there at all.

```go
viewer := imageviewer.New(gallery.Collection(), resolve)
viewer.SetOrigin(gallery)

ui.ZStack(
	ui.ImageGallery(gallery).
		OnOpen(func(id asset.ID) { viewer.Open(id) }).     // a tap on a tile
		OnActivate(func(id asset.ID) { viewer.Open(id) }), // enter on the keyboard cursor
	imageviewer.Fullscreen(viewer).Card(details),
)
```

`New` takes the same catalogue and resolver as the gallery. `Open` shows an entry, `Close`, `Next` and `Previous`
do what they say, and `OnChange` and `OnClose` tell the application what the user did.

## The hero transition

`SetOrigin` connects the viewer to where its pictures are shown small. A `*ui.Gallery` is such an origin; anything
that implements `imageviewer.Origin` is:

- `ThumbnailFrame(id)` says where the thumbnail of a picture was drawn, so that the picture can grow out of it with
  its crop and its rounded corners and shrink back into it.
- `Conceal(id)` hides that thumbnail while the picture is shown, because a picture dragged away would otherwise
  uncover a second copy of itself.
- `Reveal(id)` scrolls the thumbnail of the picture the user paged to into view behind the viewer, so that there is
  a tile to close into.

Without an origin the viewer fades in and out.

## Gestures

| Gesture                       | What it does                                                                     |
|-------------------------------|----------------------------------------------------------------------------------|
| Swipe sideways                | the previous or the next picture; a flick turns the page whatever the distance   |
| Pinch                         | zoom about the fingers, with the rubber band and the bounce of a `UIScrollView`  |
| Pan a zoomed picture          | moves it; panned on past its edge, the pages take over                           |
| Double tap                    | zooms in about the tap, and out again                                           |
| Drag down                     | the picture follows and shrinks; released past a third, or flicked, it closes   |
| Pinch below the fit           | closes                                                                           |
| Swipe up / down               | slides the sheet of `Card` over the picture and away again                       |
| Tap                           | shows and hides the close button                                                 |
| Mouse movement                | the arrows at the sides; they fade a second and a half after the mouse rests    |
| Wheel                         | zooms about the cursor; a sideways trackpad scroll turns the page               |
| ← → Home End                  | pages                                                                            |
| + − 0, space                  | zoom in, out, to the fit; space toggles                                          |
| ↑ ↓, escape                   | open and close the card; escape closes the card or the viewer                   |

The numbers behind the feel are UIKit's and are documented in
[`ui/motion`](https://pkg.go.dev/github.com/worldiety/gift/ui/motion): the rubber band
`(1 − 1/(x·0.55/d + 1))·d`, the deceleration rate of 0.998 per millisecond, and Apple's projection of where a flick
comes to rest. The 20 pixel gap between pages, the 1200 pixel per second flick and the third of the way that close a
picture dragged down are those of Apple's photo transition sample.

`Closable(false)` keeps the user from closing the viewer, for a kiosk that is nothing but the viewer;
`ShowsControls(false)` hides the buttons and keeps the gestures and keys.

Android's back button and back gesture close the card first when it is up, and then the viewer. With
`Closable(false)` only closing the viewer is blocked: back still closes the card, and on the picture it does
nothing and does not leave the application. Back also does nothing while the viewer is closing. See
[Android and iOS](../../../concepts/mobile/#back).

## The card

`Card(func(asset.ID) gift.View)` builds the content of a card that a swipe up slides in from the bottom, rebuilt
when the page turns. It is the place for details, metadata and actions. The card is a sheet over the picture: it
follows the finger, the picture underneath stays where it is, and nothing is laid out anew. A swipe down, a tap on
the picture, the down arrow or escape slide it away again.

## The pipeline

A picture is shown in up to three stages, each drawn as soon as it has arrived:

1. A preview of `imageviewer.PreviewSize` logical pixels — the size a gallery decodes its tiles at, so that a
   picture opened from a gallery is in the cache already and appears in the first frame of the transition. With
   [libjpeg-turbo](../../../concepts/images/#faster-jpeg-decoding-with-libjpeg-turbo) registered, a camera JPEG
   that is not is decoded from the DC coefficients of its blocks first: one value per 8 by 8 block, the cheapest
   picture there is.
2. The size of the screen.
3. While the picture is zoomed, a larger decode, up to `imageviewer.MaxDetail` (4096) pixels.

The neighbours of the picture on screen are decoded ahead of time and uploaded to the GPU while the viewer is at
rest, so that a swipe finds them ready.

Stages 2 and 3 come from the pipeline's ladder, so the pipeline needs rungs as large as the screen. Keep the large
rungs off the disk cache, which stores raw pixels, and give the CPU cache room for a few screens:

```go
pipe := asset.NewPipeline(asset.Config{
	Deliver:     app.Post,
	Sizes:       []int{128, 256, 512, 1024, 2048, 4096},
	PixelBudget: 256 << 20,
	Disk:        asset.DiskCacheConfig{Dir: cacheDir, MaxSize: 512},
	Workers:     2, // on a Raspberry Pi: leave two cores to the frame loop
})
```

## On a Raspberry Pi

Measured on a Raspberry Pi 5 at 1920 by 1080, with pictures of up to 12 megapixels, libjpeg-turbo, two decode
workers and `DirectToScreen`: paging and pinching draw at a median of 16.6 ms per frame, 17.4 ms at the 95th and
20.1 ms at the 99th percentile, with 1.9 ms of CPU per frame at most. A picture larger than the per frame upload
budget is uploaded in bands of rows over several frames. Creating a texture the size of the screen still costs the
Pi one frame, which is why the neighbours are prepared while nothing moves.

Pinching needs the touch screen read directly; see [Gestures](../../../concepts/gestures/#touch-screens-on-linux).

## API

- [`imageviewer.Viewer`](https://pkg.go.dev/github.com/worldiety/gift/ui/imageviewer#Viewer)
- [`imageviewer.Fullscreen`](https://pkg.go.dev/github.com/worldiety/gift/ui/imageviewer#Fullscreen)
- [`imageviewer.Origin`](https://pkg.go.dev/github.com/worldiety/gift/ui/imageviewer#Origin)
- [`ui.Gallery.ThumbnailFrame`](https://pkg.go.dev/github.com/worldiety/gift/ui#Gallery.ThumbnailFrame)
- [`ui.Picture`](https://pkg.go.dev/github.com/worldiety/gift/ui#Picture)
