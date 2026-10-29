---
title: Installation
weight: 2
---

Gift is a plain Go module. You add it with `go get`, and the Go toolchain builds it without a C compiler. This page
lists what your machine needs, how to try the examples and which optional parts you can add.

## Requirements

- **Go 1.27** or newer. The module declares `go 1.27`.
- **A graphical session with GPU support.** Gift draws through [Ebitengine](https://ebitengine.org/) and opens a
  real window. It does not run headless; for tests without a window see [Testing](../../concepts/testing/).
- **No CGO.** Gift and its Ebitengine backend build with `CGO_ENABLED=0`. Native libraries are loaded at run time
  instead of being linked at build time. This also means you can cross-compile, for example for a Raspberry Pi
  from your laptop:

```bash
GOOS=linux GOARCH=arm64 go build ./cmd/example-counter
```

### macOS

There is nothing to install besides Go.

### Linux and Raspberry Pi OS

On Linux, Ebitengine talks to the display through X11 and draws with OpenGL. A Wayland desktop works through
XWayland. The intended Raspberry Pi setup is Raspberry Pi OS 64-bit on a Pi 4 or Pi 5 with X11 or XWayland and
hardware-accelerated Mesa.

Because nothing is linked at build time, you need no `-dev` packages. At run time the program loads the X11 client
libraries (`libX11`, `libXrandr`, `libXcursor`, `libXinerama`, `libXi`, `libXrender`, `libXext`) and an OpenGL
library (`libGL` or `libEGL`). A desktop installation of Debian or Raspberry Pi OS already has them. On a minimal
installation, install them with:

```bash
sudo apt install libx11-6 libxrandr2 libxcursor1 libxinerama1 libxi6 libxrender1 libxext6 libgl1 libegl1
```

## Try the examples

The repository contains example programs in `cmd/`. Clone it and run one at a time:

```bash
git clone https://github.com/worldiety/gift.git
cd gift
go run ./cmd/example-counter      # start here: state, buttons, scrolling
go run ./cmd/example-kitchensink  # a showcase of the components
go run ./cmd/example-gallery      # generates its own sample images
go run ./cmd/example-effects -quality=full  # border, shadow and glass at full quality
```

The gallery also shows your own JPEG and PNG files: `go run ./cmd/example-gallery -dir "$HOME/Pictures"`. Each
example has its own page under [Examples](../../examples/).

## Add gift to your module

In your own module, add the dependency:

```bash
go get github.com/worldiety/gift
```

A program imports the root package `github.com/worldiety/gift`, the views in `github.com/worldiety/gift/ui` and the
backend in `github.com/worldiety/gift/backend/ebiten`, which opens the window. [Your first app](../first-app/)
walks through a complete program.

## Fonts are opt-in

The packages `gift` and `ui` embed no typeface, and gift never looks up system fonts. Every `Text` needs a font:
the one you pass with its `Font` modifier, or the default font. If a `Text` has neither, building it panics with
a message that explains the fix. Install the default font at startup, before the first frame.

Gift ships two optional font packages that embed a typeface and register it when you import them:
`font/inter` (Inter, Regular and Bold) and `font/ibmplexmono` (IBM Plex Mono, Regular and Bold, each upright and
italic). Importing one costs binary size, importing none costs nothing. Make one of its faces the default in
`main`:

```go
import (
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/ui"
)

func main() {
	ui.SetDefaultFont(ui.MustFont(ui.FontQuery{Family: inter.Family}))
	// ...
}
```

To use your own typeface, load it with `ui.LoadFont` instead. See [Text and fonts](../../concepts/text-and-fonts/).

## Optional: libjpeg-turbo

The package `asset/turbojpeg` decodes JPEG files through libjpeg-turbo. It is several times faster than Go's
`image/jpeg` for large camera photos and can decode straight to 1/2, 1/4 or 1/8 of the size when a thumbnail is
all you need. The library is loaded at run time, still without CGO.

Install the library:

{{< tabs >}}
  {{< tab name="Raspberry Pi OS / Debian" >}}
```bash
sudo apt install libturbojpeg0
```
  {{< /tab >}}
  {{< tab name="macOS" >}}
```bash
brew install jpeg-turbo
```
  {{< /tab >}}
{{< /tabs >}}

Gift does not use it on its own, even when it is installed. Register the decoder explicitly at startup:

```go
import (
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/asset/turbojpeg"
)

func main() {
	if turbojpeg.Register() {
		asset.RegisterDecoder(asset.MIMEJPEG, turbojpeg.Decoder{})
	}
	// ...
}
```

`Register` only loads the library and reports whether that worked. Without the library it returns false, and
`image/jpeg` keeps decoding your pictures. See [Images](../../concepts/images/).

## Next steps

{{< cards >}}
  {{< card link="../first-app/" title="Your first app" icon="play" subtitle="Write, run and extend a counter application." >}}
  {{< card link="../../concepts/" title="Concepts" icon="book-open" subtitle="How a gift application works." >}}
{{< /cards >}}
