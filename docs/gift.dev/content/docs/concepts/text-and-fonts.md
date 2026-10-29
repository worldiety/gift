---
title: Text and Fonts
weight: 6
---

gift draws every glyph itself. It shapes text with
[go-text/typesetting](https://github.com/go-text/typesetting), rasterizes the glyphs into its own atlas and never
asks the operating system for a font. That makes a label look the same on macOS, on Linux and on a Raspberry Pi,
but it also means one thing you must do before the first frame: give gift a font.

## gift links no font unless you ask for one

The packages `gift` and `ui` embed no typeface. A binary that imports only them carries no font, pays no kilobytes
for one and is bound by no font license. There is also no system font lookup and no built-in fallback.

If you build a [Text](../../components/basic/text/) and no font is installed, the build panics with a message that
explains the fix. This is on purpose: the alternative would be an empty area on the screen and no hint why.

The fix is two lines. gift ships two opt-in font packages that embed a typeface and register it when you import
them:

| Package | Family constant | Faces | Embedded size |
|---------|-----------------|-------|---------------|
| [`font/inter`](https://github.com/worldiety/gift/tree/main/font/inter) | `inter.Family` (`"Inter"`) | Regular and Bold, upright | about 813 KiB |
| [`font/ibmplexmono`](https://github.com/worldiety/gift/tree/main/font/ibmplexmono) | `ibmplexmono.Family` (`"IBM Plex Mono"`) | Regular and Bold, upright and italic | about 256 KiB |

Both typefaces are licensed under the SIL Open Font License 1.1. The license text is in `LICENSE.txt` next to the
font files. It is not embedded in the binary, so ship it with your application when you distribute it.

Import the package, then make one of its faces the default font in `main`, before you start the app:

```go
package main

import (
	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/ui"
)

func main() {
	ui.SetDefaultFont(ui.MustFont(ui.FontQuery{Family: inter.Family}))

	app := gift.New(gift.Options{Root: func(ctx *gift.Context) gift.View {
		return ui.Window(ui.Text("Hello, gift"))
	}})
	if err := backend.Run(app, backend.Config{Title: "Hello", Width: 480, Height: 320}); err != nil {
		panic(err)
	}
}
```

A font package only registers its faces. It never installs itself as the default, because two packages that both
did so from `init` would overwrite each other in an order Go does not define. Choosing the default is always the
application's job.

## The font register

The register is a process-wide table of faces, keyed by family, weight and style.
[`RegisterFont`](https://pkg.go.dev/github.com/worldiety/gift/ui#RegisterFont) adds a face, and
[`ResolveFont`](https://pkg.go.dev/github.com/worldiety/gift/ui#ResolveFont) and
[`MustFont`](https://pkg.go.dev/github.com/worldiety/gift/ui#MustFont) look one up with a `FontQuery`:

```go
type FontQuery struct {
	Family string     // matched exactly and case sensitively
	Weight FontWeight // zero means WeightRegular
	Style  FontStyle  // StyleNormal (the zero value) or StyleItalic
}
```

The lookup follows three rules:

- **Family**: must match exactly. Nothing is substituted for an unknown family. Use the exported constants such as
  `inter.Family`, so that a typo is a compile error.
- **Style**: must be registered for that family. gift never slants an upright face to fake an italic, and never
  returns an upright face for a missing italic.
- **Weight**: the registered weight closest to the requested one wins. On a tie the heavier face wins. So
  `WeightSemiBold` (600) from `font/inter` gives you Bold (700), and `WeightMedium` (500) gives you Regular (400).

`ResolveFont` returns `ok == false` on a miss. `MustFont` panics instead, and its message lists every registered
face. Use `MustFont` right after the import that registers the face, where a miss can only be a forgotten import or
a misspelled family. Use `ResolveFont` when the face is optional, for example a font the user picked.

Weights are the usual 100 to 900 scale, where 400 is regular text and 700 is bold. The named constants run from
`WeightThin` (100) to `WeightBlack` (900), but `RegisterFont` accepts any value from 1 to 1000.

## Loading your own font

[`LoadFont`](https://pkg.go.dev/github.com/worldiety/gift/ui#LoadFont) parses the bytes of a font file. It accepts
TrueType and OpenType files; `font/ibmplexmono` shows that WOFF version 1 works too. A font collection (`.ttc`) is
rejected rather than reduced to its first face, and WOFF2 is not supported.

A corrupt file is a normal error, not a panic. Every such error wraps `ui.ErrBadFont`, so you can tell "the file
is missing" from "the file is not a font":

```go
func loadBrandFont(path string) (ui.Font, error) {
	data, err := os.ReadFile(path) // or go:embed
	if err != nil {
		return ui.Font{}, err // missing: look elsewhere or report a configuration problem
	}
	f, err := ui.LoadFont(data)
	if errors.Is(err, ui.ErrBadFont) {
		return ui.Font{}, fmt.Errorf("%s is not a usable font: %w", path, err)
	}
	return f, err
}
```

`LoadFont` does not copy `data`. The parser and the glyph rasterizer read glyph outlines from it lazily, on later
frames, so never modify the slice after you pass it in. Bytes from `go:embed` or from a fresh `os.ReadFile` are fine.

A loaded `Font` is a handle. Copy it freely; the parsed tables behind it are shared and immutable. The zero `Font`
means "no font", and `Font.IsZero` reports it.

You can use a loaded font directly with `SetDefaultFont` or `TextView.Font`. To make a whole family available by
name, register each face once at startup, typically from the `init` function of a package of your own:

```go
//go:embed Brand-Regular.ttf
var brandRegular []byte

//go:embed Brand-Bold.ttf
var brandBold []byte

const BrandFamily = "Brand"

func init() {
	ui.RegisterFont(BrandFamily, ui.WeightRegular, ui.StyleNormal, mustLoad(brandRegular))
	ui.RegisterFont(BrandFamily, ui.WeightBold, ui.StyleNormal, mustLoad(brandBold))
}

func mustLoad(data []byte) ui.Font {
	f, err := ui.LoadFont(data)
	if err != nil {
		panic(err) // the bytes are inside the binary, so this is a broken build
	}
	return f
}
```

`RegisterFont` panics on a programming error: an empty family, a weight outside 1 to 1000, an unknown style, the
zero `Font`, or a family, weight and style that is already registered. A duplicate panics because "last writer
wins" would silently replace another package's face. To use a different face for a slot that is taken, register it
under a family name of your own or pass the `Font` directly.

You can also add faces to an existing family. `font/inter` embeds no italics; load `Inter-Italic.ttf` yourself and
register it as `inter.Family`, `ui.WeightRegular`, `ui.StyleItalic`.

## The default font and per-view fonts

[`SetDefaultFont`](https://pkg.go.dev/github.com/worldiety/gift/ui#SetDefaultFont) fills one slot that every text
in the app falls back to. `DefaultFont` returns it, and passing the zero `Font` clears it. A text view picks its font
like this:

1. the font given to the view itself, if any,
2. otherwise the default font,
3. otherwise the build panics.

The views that shape text and take a font of their own are `TextView`, `TextFieldView`, `KeyboardView` and
`SegmentedControlView`, each with a `Font` method. Everything else that shows text uses the default font: the
title and subtitle of a [Row](../../components/lists/row/), the title of a
[navigation stack](../../components/navigation/navigation-stack/), tab captions, badges and the labels of a
[virtual list](../../components/lists/virtual-list/). A [Button](../../components/basic/button/) takes any view
as its label, so give that `Text` a font.

gift has no `Bold()` or `Italic()` modifier. Emphasis is a different face, so you resolve it and pass it in:

```go
var (
	bold   = ui.MustFont(ui.FontQuery{Family: inter.Family, Weight: ui.WeightBold})
	code   = ui.MustFont(ui.FontQuery{Family: ibmplexmono.Family})
	italic = ui.MustFont(ui.FontQuery{Family: ibmplexmono.Family, Style: ui.StyleItalic})
)

func summary(ctx *gift.Context) gift.View {
	return ui.VStack(
		ui.Text("Build finished").Font(bold).FontSize(20),
		ui.Text("go build ./...").Font(code),
		ui.Text("// 3 warnings").Font(italic).Foreground(ui.ColorSecondaryLabel),
	).Gap(8)
}
```

The package-level variables work because Go runs the `init` functions of the imported font packages before it
initializes the variables of your package. Resolving the fonts once is also cheaper than resolving them in every
build, though `ResolveFont` does not allocate either way.

A view resolves its font when it is built. `SetDefaultFont` stores the font and nothing else; it does not rebuild
anything. Set the default once in `main` before the app runs. If you change it later, only views that are built
again afterwards pick up the new font.

The default font and every `Font` belong to the UI goroutine, like everything else in a gift app. Register and set
fonts from `init` or `main`, or from code that runs on the UI executor. See
[Concurrency](../concurrency/).

## Size, color and alignment

A [Text](../../components/basic/text/) uses `ui.DefaultFontSize`, 14 pixels, unless you call `FontSize`. The value
is the em size in logical pixels and must be finite and greater than zero; anything else panics. The color is the
semantic `ColorLabel` unless you call `Foreground`, so text follows the light and the dark theme without extra code.
See [Theming](../theming/).

`Align` takes `AlignLeading` (the default), `AlignCenter` or `AlignTrailing`. It moves each line inside the width of
the view. A view is as wide as its longest line unless something makes it wider, so a single line only moves after
`Frame`, `MinWidth` or `Flex`; in wrapped text the shorter lines move too.

## Shaping and wrapping

gift turns a string, a font, a size and a width limit into lines of positioned glyphs. The same shaping result is
used to measure the view during layout and to draw it, so a label can never be measured differently from how it is
drawn.

What the shaper does:

- left-to-right text, with kerning and ligatures exactly as the font defines them,
- line breaks at word boundaries, following Unicode line breaking (UAX 14),
- whole-pixel baselines and glyph positions, with no subpixel positioning.

What it does not do, and what you should plan around:

- **No font fallback.** A paragraph is shaped with exactly one font. A character the font does not cover is drawn
  as the font's "missing glyph" box, usually a rectangle. If your text can contain other scripts, emoji or symbols,
  choose a font that covers them.
- **No bidirectional text.** Hebrew and Arabic are shaped but not reordered, so they come out visually reversed.
- No vertical writing.

Text wraps at the width the parent allows. A label in a vertical stack wraps at the width of the stack; to give a
label its own wrapping width, use `MaxWidth` or `Frame`:

```go
ui.Text(longDescription).MaxWidth(320)
```

A single word that is wider than the limit is not broken, hyphenated or scaled. It keeps its real width, the view
reports the width it was allowed, and the difference is reported as overflow. The text stays visible past the edge
unless you call `Clip(true)`. You find every such overflow in the diagnostics; see [Diagnostics](../diagnostics/)
and [Layout](../layout/).

## Line limits and truncation

`MaxLines(n)` keeps the first `n` lines. When the text does not fit, the last visible line ends in an ellipsis
after as much text as fits: "Hochzeit Anna & B…". `Truncation` picks where the ellipsis goes:

| Mode | Result | Use it for |
|------|--------|------------|
| `TruncateTail` (default) | `Hochzeit Anna & B…` | names, titles, most labels |
| `TruncateMiddle` | `/Volumes/fot…/IMG_0001.jpg` | file paths, file names, identifiers whose end matters |
| `TruncateHead` | `…/IMG_0001.jpg` | text where only the end matters |
| `TruncateNone` | hard cut, no ellipsis | when you draw your own "more" indicator |

```go
ui.VStack(
	ui.Text(title).MaxLines(1),                  // one line, ellipsis at the end
	ui.Text(description).MaxLines(3),            // up to three lines
	ui.Text(path).Truncation(ui.TruncateMiddle), // one line, ellipsis in the middle
	ui.Text(log).MaxLines(2).Truncation(ui.TruncateNone),
).MaxWidth(280)
```

Three details matter:

- `Truncation` without `MaxLines` limits the view to one line, unless the mode is `TruncateNone`.
- Only the last visible line is ever shortened. A word too wide for an earlier line still overflows.
- Truncation is not silent. The width the ellipsis hides and the height of the dropped lines are reported as
  overflow, like any other content that does not fit.

The built-in components already use this: row titles and subtitles, tab captions and navigation titles are limited
to one line with a tail ellipsis.

## The shaping cache

Shaping is cached by string, font, size, width limit, line limit and truncation mode. One cache with a budget of one mebibyte serves
the whole process: every text view, row label, button caption and keyboard key. It holds roughly 1,500 short
paragraphs.

gift does not cull painting, so every visible paragraph asks the cache for its layout on every frame. While all
distinct strings of the screen fit, that costs nothing. Past the budget, the cache evicts entries the same frame
needs again, and the whole screen is shaped again on every frame, even when nothing changed. Nothing reports this;
the app just gets slow.

Keep it in mind when a screen shows more than about a thousand distinct strings at once. Repeated strings, such as a
status from a fixed set of words, count only once. For long lists, use a
[VirtualList](../../components/lists/virtual-list/), which only produces the rows that are in view.

## Fonts in tests

The default font is process-wide, so a test that renders text needs one too. The `gifttest` harness has an
`Options.Font` field that installs a font for the test:

```go
h := gifttest.New(t, gifttest.Options{
	Root: myView,
	Font: ui.MustFont(ui.FontQuery{Family: inter.Family}),
})
```

Name the font in the test rather than relying on whatever another test installed. See [Testing](../testing/).

## API

{{< api "Font" >}}

{{< api-funcs pkg="ui" names="SetDefaultFont,RegisterFont" >}}

{{< api type="FontQuery" decl="true" >}}

{{< api type="FontFace" decl="true" >}}

The text modifiers (`Font`, `FontSize`, `Foreground`, `Align`, `MaxLines`, `Truncation`) are listed on the
[Text](../../components/basic/text/) page.

## Related

- [Text](../../components/basic/text/), [Text Field](../../components/basic/text-field/),
  [Segmented Control](../../components/basic/segmented-control/), [Keyboard](../../components/dialogs/keyboard/)
- [Theming](../theming/) for text colors, [Layout](../layout/) for sizing and overflow,
  [Diagnostics](../diagnostics/) for finding text that does not fit
- [Counter example](../../examples/example-counter/), which sets up Inter and lets you try another font with the
  `GIFT_FONT` environment variable
