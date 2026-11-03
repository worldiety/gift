// Command example-typst shows Typst as gift's layout engine for paged
// documents: Typst sets the pages, gift draws them, and the document can be
// edited in its source and on its pages alike.
//
// The Typst compiler is a separate library, libgifttypst, built from the Rust
// crate in typst/ffi and loaded at run time without cgo. The example imports
// typst/embedded, which carries the library for macOS on arm64 and Linux on
// amd64 and arm64, so that it runs as it is:
//
//	go run ./cmd/example-typst
//
// -lib loads another build of the library instead, such as one just built
// with (cd typst/ffi && cargo build --release).
//
// On the left is the source in a ui.CodeEditor, coloured by syntax.Typst,
// with the lines Typst reports an error on marked red; on the right the
// pages in a typstview.Editor. Both edit one typstview.Document: typing in
// either changes the source, and the other follows. The status line says how
// long the last setting took, and the button writes the document as a PDF.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	_ "github.com/worldiety/gift/font/ibmplexmono"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/typst"
	_ "github.com/worldiety/gift/typst/embedded"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/syntax"
	"github.com/worldiety/gift/ui/typstview"
)

const sample = `#set page(width: 420pt, height: 595pt, margin: 36pt, numbering: "1")
#set text(font: "Inter", size: 10.5pt, lang: "de")
#set par(justify: true)

= Angebot 2026-117

Sehr geehrte Damen und Herren,

vielen Dank für Ihre Anfrage. Klicken Sie in diesen Text und tippen Sie: Die Seite wird bei jedem Tastendruck neu gesetzt, und der Cursor folgt dem Text. Markieren Sie ein Wort und machen Sie es mit der Werkzeugleiste *fett* oder _kursiv_.

#table(
  columns: (auto, 1fr, auto, auto),
  align: (left, left, right, right),
  fill: (_, y) => if y == 0 { rgb("#e8eef7") },
  stroke: 0.5pt + rgb("#9aa5b4"),
  table.header([*Pos.*], [*Leistung*], [*Menge*], [*Preis*]),
  [1], [Konzeption und Workshop], [2], [1.200,00 €],
  [2], [Gestaltung der Oberfläche], [5], [3.000,00 €],
  [3], [Umsetzung des Editors], [12], [9.600,00 €],
  table.cell(colspan: 3, align: right)[*Summe netto*], [*13.800,00 €*],
)

== Positionen im Einzelnen

Die folgende Tabelle ist länger als eine Seite. Typst wiederholt ihren Kopf auf jeder neuen Seite.

#table(
  columns: (auto, 1fr, auto),
  align: (right, left, right),
  stroke: 0.5pt + rgb("#9aa5b4"),
  table.header([*Nr.*], [*Arbeitspaket*], [*Stunden*]),
  ..range(1, 61).map(i => ([#i], [Arbeitspaket #i], [#(calc.rem(i * 7, 13) + 2)])).flatten(),
)

Mit freundlichen Grüßen
`

func main() {
	lib := flag.String("lib", "", "path of libgifttypst, instead of the embedded one")
	fonts := flag.String("fonts", "", "directory with more .ttf fonts for Typst")
	flag.Parse()

	if err := run(*lib, *fonts); err != nil {
		fmt.Fprintln(os.Stderr, "example-typst:", err)
		os.Exit(1)
	}
}

func run(lib, fonts string) error {
	if err := example.LoadFont(); err != nil {
		return err
	}
	if err := typst.Open(lib); err != nil {
		return fmt.Errorf("%w (build it with: cd typst/ffi && cargo build --release, and pass -lib)", err)
	}
	var extra [][]byte
	if fonts != "" {
		files, _ := filepath.Glob(filepath.Join(fonts, "*.ttf"))
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			extra = append(extra, data)
		}
	}
	app := gift.New(gift.Options{Root: func(ctx *gift.Context) gift.View { return root(ctx, extra) }})
	return backend.Run(app, backend.Config{Title: "gift Typst – " + typst.Version(), Width: 1280, Height: 860})
}

func root(ctx *gift.Context, fonts [][]byte) gift.View {
	doc := typstview.Use(ctx, "doc", sample)
	src := ui.Editor(ctx, "src", sample)
	saved := ctx.State("saved", "")
	if fresh := ctx.State("fonts", false); !fresh.Get() {
		fresh.Set(true)
		for _, f := range fonts {
			if err := doc.AddFont(f); err != nil {
				saved.Set(err.Error())
			}
		}
	}
	// An edit on the pages is an edit of the source, which the code editor
	// then shows, with its caret where the page has it.
	doc.OnChange(func(s string) {
		src.SetText(s)
		src.SetCaret(doc.Caret())
	})
	typstview.Watch(ctx, doc)

	// Typst reports an error by its place in the source: the code editor
	// marks the lines it touches.
	var marks []ui.CodeMark
	for _, g := range doc.Diagnostics() {
		marks = append(marks, ui.CodeMark{Start: g.Start, End: g.End})
	}

	// Read, not Get: the button below sets it, and only a build that read it
	// runs again, which shows what the button did.
	line := ctx.Read(saved)
	switch {
	case doc.Err() != nil:
		line = doc.Err().Error()
	case len(doc.Diagnostics()) > 0:
		g := doc.Diagnostics()[0]
		line = fmt.Sprintf("Zeile %d: %s", strings.Count(doc.Source()[:min(g.Start, len(doc.Source()))], "\n")+1, g.Message)
	case line == "" && doc.Pages() != nil:
		line = fmt.Sprintf("%d Seiten in %.1f ms gesetzt", len(doc.Pages().Pages), float64(doc.Took().Microseconds())/1000)
	}
	return ui.Window(ui.VStack(
		ui.HStack(
			ui.Text("Typst in gift").FontSize(18),
			ui.Text(line).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			ui.Spacer(),
			ui.Button(ui.Text("PDF speichern").FontSize(13), func() {
				pdf, err := doc.PDF()
				if err == nil {
					err = os.WriteFile("example-typst.pdf", pdf, 0o644)
				}
				if err != nil {
					saved.Set(err.Error())
				} else {
					saved.Set(fmt.Sprintf("example-typst.pdf geschrieben, %d KiB", len(pdf)/1024))
				}
			}),
		).Gap(16).AlignBaseline(),
		ui.HStack(
			ui.CodeEditor(src).Highlighter(syntax.Typst).Indent("  ").Marks(marks...).
				OnChange(func(s string) {
					saved.Set("")
					doc.SetSource(s)
				}).Flex(1),
			typstview.Editor(doc).Flex(1.3),
		).Gap(12).Flex(1),
	).Gap(12).Padding(16).Background(ui.ColorSurface))
}
