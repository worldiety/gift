---
title: Photos
weight: 4
---

This example is a photo browser in the manner of Photos on iOS: a gallery of thumbnails, and a full screen
[image viewer](../../components/media/image-viewer/) that grows out of the tile you tap and shrinks back into it.
It is the program to try the gestures with, on a touch panel, a trackpad or a mouse.

{{< gallery-shot alt="A panorama full screen on black, with a sheet slid over its lower half that shows the file name and the size of the picture" >}}

## Run it

From a checkout of the [repository](https://github.com/worldiety/gift):

```bash
go run ./cmd/example-photos                       # two dozen generated pictures of up to 12 megapixels
go run ./cmd/example-photos -dir ~/Pictures       # your own, raw files of cameras (ARW, DNG) among them
go run ./cmd/example-photos -fullscreen -workers 2  # on a Raspberry Pi
```

On first run without `-dir` the example writes its sample pictures into its cache directory: landscapes, portraits
and panoramas with a fine grid that only the zoomed stage resolves. With `libturbojpeg0` installed it decodes JPEG
with libjpeg-turbo. `-stats` prints the frame rate and the decodes every five seconds, `-upload` sets the bytes
uploaded per frame, and `-open n` opens the viewer on the n-th picture at start.

## What to try

- Tap a tile. The picture grows out of it; its tile stays empty while the picture is shown.
- Swipe through the pictures, slowly and with a flick. Pinch one, pan it to its edge and on: the next page follows.
- Drag a picture down a little and let go, then drag it further or flick it: it shrinks back into its tile, which
  the gallery has scrolled into view.
- Swipe up for the sheet with the picture's file name and size; it slides over the picture and away again.
- With a mouse, move it to bring up the arrows, wheel to zoom, and use the arrow keys, plus, minus and escape.

On a Raspberry Pi with a touch display, pinching works because the backend reads the panel directly; see
[Gestures](../../concepts/gestures/#touch-screens-on-linux).

## Code

{{< example-code >}}
