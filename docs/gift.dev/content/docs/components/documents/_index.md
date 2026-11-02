---
title: Documents
weight: 9
---

Document components show and edit paged documents: letters, offers, invoices, reports with tables. The pages are
set by [Typst](https://typst.app), a typesetting system that reads a markup language of its own, and drawn by gift's
own renderer, glyph by glyph. They live in package
[`ui/typstview`](https://github.com/worldiety/gift/tree/main/ui/typstview), on top of package
[`typst`](https://github.com/worldiety/gift/tree/main/typst), which loads the Typst compiler as a shared library at
run time, without cgo.

- [Typst Editor](typst-editor/): a rich text document, edited on the page it is set on, with a toolbar for strong
  and emphasised text, headings, lists and tables.
- [Typst PDF](typst-pdf/): the pages of a document in a viewer, and the same pages as a PDF file.

The source of a document is Typst markup, so a [Code Editor](../basic/code-editor/) edits it as code, with the
lines Typst reports an error on marked; the [Typst example](../../examples/example-typst/) shows both side by side.
