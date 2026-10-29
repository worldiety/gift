# Maintaining the documentation site

The documentation site in `docs/gift.dev` explains the concepts of gift and presents every component with a
screenshot, the code that drew it and its API. It follows the site of
[Nago](https://www.nago.dev/docs/getting-started/): Hugo with the [Hextra](https://imfing.github.io/hextra/) theme,
prose written by hand, screenshots rendered by a program. Unlike Nago, two more things are generated so that they
cannot drift from the code: the snippet next to each screenshot and the API tables.

## Layout

| Path                         | Purpose                                                                   |
|------------------------------|---------------------------------------------------------------------------|
| `docs/gift.dev/`             | Hugo site with the Hextra theme                                           |
| `docs/gift.dev/content/docs` | the documentation, see the sections below                                 |
| `docs/gift.dev/data/api.json`| constructors and methods of every exported type, written by `docs/apigen` |
| `docs/gallery/`              | one demo per component, only used for the screenshots and snippets        |
| `docs/capture/`              | renders all screenshots through `giftauto`                                |
| `docs/apigen/`               | writes `api.json` from the godoc of the source                            |
| `docs/screenshots/`          | the screenshots of the README, not part of the site                       |

Sections of `content/docs/`:

1. `getting-started/` what gift is, installation, the first application.
2. `concepts/` how a gift application works: views, modifiers, state, layout, theming, text and fonts, icons,
   images, input and focus, navigation, animation, concurrency, custom views, kiosks, testing, diagnostics.
3. `components/` one page bundle per component, grouped into basic, layout, lists, navigation, dialogs and
   styling.
4. `examples/` one page per example program in `cmd/example-*`.

## Build and preview

Hugo 0.164 or newer is required (the shortcodes use `hugo.Data`). Without a local installation, build it with Go:

```bash
go install github.com/gohugoio/hugo@v0.164.0
```

```bash
cd docs/gift.dev
hugo server            # preview on http://localhost:1313
hugo build --minify    # what a deployment does
```

## Generated parts

```bash
go run ./docs/apigen                        # rewrite data/api.json after an API change
go run ./docs/capture                       # all screenshots, needs a display
go run ./docs/capture -only button,card     # some demos or examples
go run ./docs/capture -examples=false       # only the component demos
go run ./docs/gallery -demo button          # look at a demo in a window
```

`go test ./docs/...` fails when `api.json` is out of date.

### Screenshots

All screenshots are framebuffer captures of a real window, taken through the `giftauto` automation interface,
never by hand and never offscreen. `docs/capture` builds the gallery with `-tags giftauto`, starts it once per
demo on a free port, takes one screenshot in the light and one in the dark theme, crops both to what the demo
drew plus a margin and writes `screenshot-light.png` and `screenshot-dark.png` into the page bundle. The site
shows the one that matches the reader's theme. Run it on macOS with a Retina display, so that every screenshot
has density 2: the `gallery-shot` shortcode shows them at half their pixel size.

### Component demos

Each file in `docs/gallery` registers one demo in its `init` function:

```go
func init() {
	register(Demo{Name: "button", Group: "basic", Width: 560, Height: 200}, buttonDemo)
}

func buttonDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	return ui.HStack(...)
	// snippet:end
}
```

`Name` equals the page bundle and the file name, except that the file name writes dashes as underscores
(`progress-bar` lives in `progress_bar.go`); `Group` is the section below `components/`. The lines between the
markers are what `{{< gallery-code >}}` shows on the page, so keep them self-contained and readable. A demo shows
the settled state of its component: present a sheet or an alert from the first frame instead of scripting a tap.
`Input` takes the body of a `POST /input` for the rare demo that needs one; `FullWindow` keeps the whole window
for components that fill it.

### API tables

`{{< api "ButtonView" >}}` renders the constructors and methods of a type of package `ui`, `pkg="gift"` selects
another package. The description is the first sentence of the godoc. The common modifiers (`Frame`, `Padding`,
`Background`, ...) are collapsed into one line that links to the concepts page about modifiers. `decl="true"`
also prints the type declaration, which is what a style struct needs. `{{< api-funcs pkg="ui"
names="SetTheme,CurrentTheme" >}}` lists package level functions. A shortcode that names something missing from
`api.json` fails the build.

## Writing conventions

- English, second person, present tense. Short sentences, no marketing. American spelling, as in the API
  (`Color`), even where the godoc of gift writes "colour".
- Every statement must be true for the current source code. Verify names, signatures and behaviour in the code
  before writing them down. The godoc of gift is long and explains the reasons behind a design; summarise what a
  reader needs and link to pkg.go.dev for the rest.
- Code snippets must compile against the current API. Prefer a demo in `docs/gallery` or an example in `cmd`
  over a hand-written snippet; a hand-written one is checked by compiling it once.
- Use Hextra shortcodes (`callout`, `cards`, `tabs`, `steps`) where they help. Theme CSS classes use the
  Tailwind v4 prefix `hx:`.
- Link between pages with relative links. Link to code on `https://github.com/worldiety/gift/tree/main/...`.
- Front matter: `title`, optional `weight` and `linkTitle`. Do not set `prev`/`next`.
- Do not refer to the sections of `PLAN.md`; the site has to stand on its own.

### Component pages

`content/docs/components/<group>/<name>/index.md`:

1. one or two paragraphs: what the component is and when to use it,
2. `{{< gallery-shot >}}` and `{{< gallery-code >}}`,
3. **Usage**: the few things a reader needs to know to use it well, e.g. how state flows in and out, sizing,
   keys, styles; with short snippets where they help,
4. **API**: `{{< api "TypeName" >}}` for the view type and its style or spec types,
5. **Related**: links to related components, concepts and examples.

### Example pages

`content/docs/examples/<example>/index.md`, the bundle named like the directory in `cmd`: one or two paragraphs
what the example shows, `{{< gallery-shot >}}`, how to run it, and `{{< example-code >}}`. Its screenshots are
captured by `docs/capture` too, see `exampleShots` there.

## Review

When the API changes: run `go run ./docs/apigen`, re-render the screenshots of the affected demos, update the
prose of the affected pages, and add pages for new components. `hugo build` must pass without errors or
warnings.
