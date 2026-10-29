---
title: Icons
weight: 7
---

An icon in gift is a `ui.Symbol`: the outlines of one icon, parsed ahead of time and embedded in your binary. You
draw a symbol with [`ui.Icon`](../../components/basic/icon/). gift rasterizes it on the CPU at the exact number of
device pixels it covers, uploads the result once as a texture and from then on draws it as a tinted image. There is
no SVG parser at run time, no asset to load and no placeholder.

gift ships two sets of symbols, both from [Flowbite Icons](https://flowbite.com/icons/) (MIT licensed):
`icon/outline` and `icon/solid`. Your own SVGs become symbols with the generator `cmd/gift-icongen`.

## Using an icon

Import a set and name the icon. Every icon is a package level variable whose name is the SVG file name in Go
spelling, so `address-book.svg` is `outline.AddressBook`.

```go
import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/icon/solid"
	"github.com/worldiety/gift/ui"
)

func toolbar(ctx *gift.Context) gift.View {
	return ui.HStack(
		ui.Icon(outline.Search),
		ui.Icon(outline.Bell).Size(24),
		ui.Icon(solid.Heart).Foreground(ui.ColorDanger).Label("Favorite"),
	).Gap(12).Align(geom.Center)
}
```

- **Size** is the edge of the square icon in logical pixels. The default is `ui.DefaultIconSize`, 20, which sits
  well next to body text. The artwork is drawn on a 24 unit grid, so `Size(24)` shows it at its design size.
  `Size` panics on a value that is not positive and finite.
- **Foreground** is the tint. The default is `ui.ColorLabel`, so an icon follows the light and dark theme like
  text does. Use any [semantic color](../../components/styling/colors/) or a fixed one; see
  [Theming](../theming/).
- **Label** sets the accessible name. Text carries its own label, an icon does not. Give every icon that means
  something on its own a label, so that a test can find it with `gifttest.ByText` and an accessibility bridge has
  something to read.

The other modifiers (`Padding`, `Background`, `Border`, `CornerRadius`, ...) work as on every view, see
[Modifiers](../modifiers/). They style the box around the icon, not the icon itself: `CornerRadius` rounds the
background, and `Shadow` is the shadow of the box, not of the icon's outline.

Nothing has to be registered. Importing a set has no side effect, so you can import `outline` and `solid` in the
same program. If you prefer a shorter name, alias the import: `import icons "github.com/worldiety/gift/icon/outline"`.

## Outline or solid

| Package                               | Style                                         | Icons |
|---------------------------------------|-----------------------------------------------|-------|
| `github.com/worldiety/gift/icon/outline` | strokes, two units wide, round caps and joins | 282   |
| `github.com/worldiety/gift/icon/solid`   | filled shapes                                 | 239   |

Outline icons read lighter and suit toolbars and list rows. Solid icons suit a selected state and small sizes,
where a two unit stroke starts to close up. Each package exports `Count`, the number of icons it holds.

The two sets do **not** have the same names. Some basic glyphs exist only in `outline`, for example `Check`,
`Close`, `Plus`, `Minus`, `Bars`, `ChevronDown` and `AngleRight`. `solid` has brand logos (`Github`, `Visa`, ...)
that `outline` lacks. The compiler tells you when a name is missing; browse the
[outline](https://pkg.go.dev/github.com/worldiety/gift/icon/outline#pkg-variables) and
[solid](https://pkg.go.dev/github.com/worldiety/gift/icon/solid#pkg-variables) variables on pkg.go.dev.

`solid` is monochrome like every gift icon. Where the original SVG draws in a literal color, such as the lettering
of `solid.Visa`, that part is punched out of the shape and shows the background.

## Icons inside other views

`ui.Icon` is an ordinary view, so it goes anywhere a view goes. A button with an icon and a caption is a stack as its
label:

```go
func saveButton() gift.View {
	return ui.Button(ui.HStack(
		ui.Icon(outline.FloppyDisk).Size(16).Foreground(ui.ColorOnAccent),
		ui.Text("Save").Foreground(ui.ColorOnAccent),
	).Gap(6).Align(geom.Center), save)
}
```

A button whose label is only an icon has no text for a test or an accessibility bridge to find. Name the command
with `ButtonView.Label`:

```go
ui.Button(ui.Icon(outline.Cog), save).Label("Settings")
```

Several components take a `ui.Symbol` directly instead of a view. Package `ui` cannot import the icon packages,
because they import `ui`, so these components draw no icon until you hand them one:

| Where                                                                     | What it is for                               |
|---------------------------------------------------------------------------|----------------------------------------------|
| [`ui.Tab(title, sym, content)`](../../components/navigation/tab-bar/), `TabSpec.SelectedIcon` | the tab icon, and the one shown while selected |
| [`RowView.Icon`, `RowView.Chevron`](../../components/lists/row/)          | leading symbol and trailing "leads somewhere" mark |
| [`ListRow.Icon`, `ListRow.Chevron`, `VirtualListView.SelectedIcon`](../../components/lists/virtual-list/) | the same for virtual lists, plus the selection mark |
| [`NavigationStackView.BackIcon`](../../components/navigation/navigation-stack/) | the symbol in front of the back button's title |

The usual pairing in a tab bar is the outline icon at rest and the solid one when selected:

```go
ui.TabBar(selected, onSelect,
	ui.Tab("Home", outline.Home, home).SelectedIcon(solid.Home),
	ui.Tab("Inbox", outline.Inbox, inbox).SelectedIcon(solid.Inbox),
)
```

## Symbol is a value

A `ui.Symbol` holds no bitmap and no texture. It is a slice of encoded geometry, an edge length and an id, so it is
free to copy, to store in a map or a struct and to pass around. The mask and the texture are made on demand by the
icon view and cached separately.

The zero `Symbol` is the "no icon" value. `ui.Icon(ui.Symbol{})` takes up its space and draws nothing, and the
components above leave the icon out. That makes a lookup table safe even when the key is missing. Use
`Symbol.IsZero` to choose a fallback:

```go
var fileIcons = map[string]ui.Symbol{
	".pdf": outline.FilePdf,
	".csv": outline.FileCsv,
	".zip": outline.FileZip,
}

func fileIcon(ext string) gift.View {
	sym := fileIcons[ext] // the zero Symbol when ext is unknown
	if sym.IsZero() {
		sym = outline.File
	}
	return ui.Icon(sym)
}
```

{{< api "Symbol" >}}

## Your own icons: cmd/gift-icongen

`gift-icongen` turns a directory of SVG files into a package of symbols. It writes two files into the output
directory: `icons.bin`, the encoded geometry of all icons in one blob, and `icons.gen.go`, which embeds the blob and
declares one `ui.Symbol` per file. The two bundled sets are built with it.

```bash
gift-icongen -src DIR -out DIR -pkg NAME -set NAME
```

| Flag   | Meaning                                                        |
|--------|----------------------------------------------------------------|
| `-src` | directory of `.svg` files to read (required)                   |
| `-out` | existing directory to write `icons.bin` and `icons.gen.go` into (required) |
| `-pkg` | package name of the generated file (required)                  |
| `-set` | human readable name of the set, used in the generated comment  |

Put a `go:generate` line into a file of the target package, for example `myapp/icons/doc.go`:

```go
//go:generate go run github.com/worldiety/gift/cmd/gift-icongen -src ../assets/icons -out . -pkg icons -set "My app"
package icons
```

`go generate ./...` then writes the package, and you use it like the bundled ones: `ui.Icon(icons.Logo)`. Commit the
generated files, so that a normal `go build` needs neither the SVGs nor the generator.

The generator reads a deliberately small subset of SVG and refuses everything else, naming the file. A refused icon
is better than one that is silently missing a stroke.

- The `viewBox` must be a square anchored at the origin, for example `0 0 24 24`.
- Only `<path>` and `<rect>` elements are allowed. Any other element, such as `<g>`, `<defs>`, `<use>` or a
  gradient, stops the generator. So does an attribute outside the small set it understands, `transform` among
  them, a value in single quotes, and a `url(...)` paint, so flatten your icons in the editor before you export
  them.
- Every element must be filled or stroked, directly or through the `fill` and `stroke` of the `<svg>` element.
  An element with neither is refused.
- A stroke must be `currentColor`, since only that can be tinted. A fill that is not `currentColor` becomes a
  knockout, as described for `solid` above; a `url(...)` fill is refused instead.
- An `evenodd` fill rule is converted to nonzero by reorienting the contours. If that does not reproduce the
  original shape, the generator stops with an error.

`ui.NewSymbol(data, box)` is what the generated code calls. You can call it yourself if you have your own pipeline
for the encoded format, but data it cannot decode yields a symbol that draws nothing.

## What an icon costs

**Binary size.** An icon set costs only when you import it. The outline blob is 36,398 bytes and the solid one
56,607 bytes. Including symbol names and initialization, the outline package contributes about 93 KB of symbols
and the solid one about 104 KB; the whole binary grows by 100 to 136 KB per set, depending on the platform and
the build flags. A program that
imports `gift/ui` and never names an icon pays practically nothing. The exact figures and how they were measured
are in the [package documentation of outline](https://pkg.go.dev/github.com/worldiety/gift/icon/outline).

**Startup.** Nothing is rasterized at init. A symbol is ready-made geometry.

**Per frame.** The first frame that draws a symbol at a given size rasterizes one mask and uploads it as one texture.
Every later frame is a map lookup and one draw operation, and allocates nothing. The cache key is the symbol and
the edge in *device* pixels, so a 20 pixel icon on a 2x display is rasterized at 40 pixels and stays sharp. The
same symbol in two colors shares one mask, because the color is applied when drawing.

### The upload budget

Textures are not uploaded without limit. By default the Ebitengine backend admits at most 8 uploads and 2 MiB per
drawn frame,
and icons share this budget with [pictures](../images/). An icon whose upload is refused draws nothing in that frame
and asks again in the next one. In practice this means that a screen showing 20 icons that were never drawn before
takes at least three frames to fill in, eight icons at a time. You see this on the first appearance of a busy
screen, not later: once uploaded, an icon stays resident while it keeps being drawn.

A texture that is not drawn for 600 drawn frames, ten seconds at 60 Hz, is evicted. When the backend's texture memory
budget is full, the least recently drawn texture makes room. The next time the icon appears it
is rasterized and uploaded again, through the same budget.

### Pitfalls

{{< callout type="warning" >}}
**Do not animate `Size`.** The mask cache is keyed by size, so an icon whose size changes every frame is rasterized
and uploaded every frame, eats the upload budget of the whole screen and never reaches a warm state. Pick the
sizes an icon can have up front and switch between them; see [Animation](../animation/) for what to animate
instead.
{{< /callout >}}

- **Make an icon bigger with `Size`, not with `Frame` or a minimum size.** A `Frame`, or `MinWidth` and
  `MinHeight` together, larger than `Size` draws the artwork into the larger square, but the mask is still rasterized at `Size`, so the
  icon comes out magnified and blurry. To give an icon a larger hit area or more room, use `Padding`: it keeps the
  icon at its size. A `Frame` smaller than `Size` is fine, the icon is rasterized at the smaller size. A frame that
  is not square draws the icon as a square as large as its shorter side, centered.
- **Many sizes cost many masks.** Each distinct size of a symbol is its own mask and texture. Settle on a few sizes,
  for example 16, 20 and 24.
- **Icons in tests draw nothing.** The `gifttest` harness has no texture service unless you render through the GPU,
  so icons keep their bounds and labels but emit no draw operation. Assert on labels and keys, not on pixels; see
  [Testing](../testing/).

## Watching the cache: IconCacheStats

`ui.IconCacheStats` returns a snapshot of the icon cache:

| Field        | Meaning                                                                          |
|--------------|----------------------------------------------------------------------------------|
| `Entries`    | the number of masks the cache holds a texture for                               |
| `Rasterised` | the number of masks rasterized on the CPU since the process started              |
| `Uploads`    | the number of masks handed to the backend; the gap to `Rasterised` is uploads the frame budget refused |
| `Blanks`     | the number of symbol and size pairs known to produce no ink                      |

On a steady screen `Rasterised` stops climbing. If it keeps rising at frame rate, something asks for a new size every
frame.

The cache belongs to the UI goroutine, and its counters are plain integers. Read them in a view function, an event
handler or a closure passed to `App.Post`, never directly from another goroutine:

```go
func traceIcons(app *gift.App) {
	for range time.Tick(time.Second) {
		app.Post(func() {
			s := ui.IconCacheStats()
			fmt.Printf("icons: %d resident, %d rasterised, %d uploaded, %d blank\n",
				s.Entries, s.Rasterised, s.Uploads, s.Blanks)
		})
	}
}
```

The [kitchen sink example](../../examples/example-kitchensink/) does the same with the flag `-icons`: it prints the
counters for the first one and a half seconds, next to `Diagnostics().Frames`. If the counters freeze, compare them
with the frame count first: a window in the background draws no frames, so nothing is uploaded either. See
[Diagnostics](../diagnostics/) for the other counters.

{{< api "IconStats" >}}

## Related

- [Icon](../../components/basic/icon/): the component page, with a screenshot of both sets.
- [Images](../images/): pictures from files and the network, which go through the asset pipeline instead.
- [Theming](../theming/) and [Colors](../../components/styling/colors/): the colors an icon is tinted with.
- [Tab bar](../../components/navigation/tab-bar/), [Row](../../components/lists/row/) and
  [Navigation stack](../../components/navigation/navigation-stack/): components that take a symbol.
- [Kitchen sink](../../examples/example-kitchensink/): an icon strip of 20 symbols and the cache trace.
