---
title: SVG Image
---

An SVG image shows a vector picture from an SVG document: an illustration, a logo, an icon of a set gift does not
bring. It is drawn at the resolution of the screen, sharp at any size, and an icon in one colour can be tinted like
text. Package [`ui/svg`](https://github.com/worldiety/gift/tree/main/ui/svg) reads a common subset of SVG, the one
that drawing programs and icon sets write, and leaves out the rest.

{{< gallery-shot alt="A picture of a sun behind a cloud over green hills with a dashed path, large and small, beside a leaf icon in blue, black and green" >}}

{{< gallery-code >}}

## Usage

Parse a document once and keep it; a build only declares the view:

```go
logo, err := svg.ParseFile("logo.svg") // or svg.Parse(bytes), from go:embed say

svg.View(logo)                   // as large as the document asks for
svg.View(logo).Frame(120, 0)     // 120 wide, as high as its aspect makes it
svg.View(icon).Frame(24, 24).Tint(ui.ColorLabel)
```

A view is as large as the document's width and height, or its viewBox, and never larger than there is room;
`Frame` sets a size, and with `Flex` the view takes the room a stack gives it. The viewBox is fitted into the view
as the document's `preserveAspectRatio` says, centred by default. `Tint` draws every paint in one colour and keeps
its opacity, the way SwiftUI's template rendering mode draws an icon.

## What is drawn

| Drawn                                                                                       | Left out           |
|---------------------------------------------------------------------------------------------|--------------------|
| `path` with all commands, `rect` with rounded corners, `circle`, `ellipse`, `line`, `polyline`, `polygon` | `text`, `image`    |
| `g`, `svg`, `use` of an element or a `symbol`, `defs`                                        | `filter`, `mask`, `pattern`, `marker` |
| fill and stroke in colour — hex, `rgb()`, the named colours of CSS, `currentColor` — and their opacities | style sheets (`<style>`, `class`) |
| `linearGradient` and `radialGradient` with their stops, units, transform and spread         | animation, scripts |
| `fill-rule`, `stroke-width`, `stroke-linecap`, `stroke-linejoin`, `stroke-miterlimit`, `stroke-dasharray` | |
| `transform`, `opacity`, `display`, `visibility`, `clip-path`, the `style` attribute         |                    |

`Skipped` names what a document has that is not drawn, so a program can tell. Some things are drawn close rather
than exactly: the opacity of a group is given to its parts, which differs where they overlap; a stroke under a
transform that stretches one axis more than the other keeps one width; and an even-odd fill is exact for contours
that do not cross, which is every hole a drawing program makes.

## Drawing

A document is rasterised on the CPU into a picture of the view's pixels, and the picture is drawn as a texture. A
large one is rasterised on a goroutine, so that the frame does not wait; until it is done the view shows the
picture it had, scaled. A small one, an icon, is rasterised at once, so it does not flash. A document keeps its last
four pictures, so the same icon in every row of a list is rasterised once. The Ghostscript tiger, 240 paths with
their strokes, takes 30 ms at 900 by 900 pixels on an Apple M1 and 4 ms at 300 by 300.

## Swift

SwiftUI's `Image` draws an SVG from an asset catalog, which Xcode keeps as a vector when it is told to preserve the
vector data. An SVG file read at run time it does not draw: an app shows one in a web view or with a library.

## API

{{< api pkg="svg" type="ImageView" >}}

{{< api pkg="svg" type="Image" >}}

## Related

- [Icon](../icon/) draws the icons gift brings, from SVG converted ahead of time.
- [Image](../image/) shows a photograph or a picture in pixels.
