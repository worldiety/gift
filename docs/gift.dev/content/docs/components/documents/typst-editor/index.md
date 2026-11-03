---
title: Typst Editor
---

The Typst editor edits a rich text document on its pages, the way a word processor does: you click into the text,
type, select and format it, and the page is set again with every keystroke. Underneath, the document is
[Typst](https://typst.app) markup. Every edit on the page is an edit of that source, made so that it stays a
document Typst sets. Use it where text has to look the same on the screen and on paper, such as a letter, an offer
or a report with tables, and where a PDF of it is wanted, see [Typst PDF](../typst-pdf/).

{{< gallery-shot alt="A page with the heading Release notes, a paragraph with a strong word, a bulleted list and a table, below a toolbar with buttons for strong, emphasis, two heading levels, paragraph, two kinds of list and a table" >}}

{{< gallery-code >}}

## Usage

```go
doc := typstview.Use(ctx, "letter", "#set text(font: \"Inter\")\n= Dear Ann\n\nThank you.")
typstview.Editor(doc).Flex(1)
```

### The document

A `typstview.Document` holds the source, a caret and a selection in it, and an engine that sets it on a goroutine
of its own. `typstview.Use` keeps one in the state of a component and closes it when the component goes away;
`NewDocument` makes one you keep and close yourself. A keystroke never waits for the setting: the pages shown are
those of the last source that was set without an error, until the next one is.

`Source` and `SetSource` read and replace the source, and `OnChange` is called after every edit made on the pages.
`Diagnostics` returns the errors of the last source, with their place in it, and `Pages` the pages that were set
last. A component that shows something of the document itself, its errors say, calls `typstview.Watch` to be built
again whenever the document changes.

### Fonts

The document sees every font registered with gift when it is made, and a document names its font with Typst's
own text function, as in `#set text(font: "Inter")`. `AddFont` adds the faces of another TrueType or OpenType file.
gift draws the glyphs Typst positions from the very same file, so a font of a document is one gift can draw too.

### Editing on the page

| Input                                      | Action                                                            |
|--------------------------------------------|-------------------------------------------------------------------|
| click                                      | put the caret where the click lands                               |
| drag, shift-click                          | select                                                            |
| double click, triple click                 | select the word, the paragraph                                    |
| typing                                     | insert at the caret, replacing the selection                      |
| backspace, delete                          | delete the character the page shows before or after the caret     |
| enter                                      | start a new paragraph, or the next item of a list                 |
| arrows, with shift                         | move the caret through the text the page shows, select            |
| shortcut modifier with A, C, X, V          | select all, copy, cut, paste plain text                           |

Typed characters that Typst would read as markup, such as `#`, `*` or `$`, are escaped, so that the page shows
what was typed. Deleting removes what the page shows and keeps the markup around it: a selection that starts in
strong text and ends outside it does not take the bracket that closes it along. At the start of a heading or a list
item, backspace turns the line into a paragraph; at the start of a table cell it does nothing. Enter in strong or
emphasised text closes it before the break and opens it again after it.

Where an edit puts two pieces of the source next to each other, it checks what Typst makes of them: a slash after a
slash, which would start a comment, a parenthesis right after a call, which would continue it, a dash or a number
that now starts a line, which would make a list, and text right after a set rule, which needs a line of its own.
Starting from an empty document, nothing done on the page or with the toolbar leaves a source Typst cannot set. A
source handed to the document may hold markup the editor does not know, and an edit there can still leave an error,
which the toolbar then reports.

### The toolbar

The toolbar makes the selection, or the word at the caret, strong or emphasised, and plain again; it turns the line
of the caret into a heading of the first or second level, a paragraph, or an item of a bulleted or numbered list,
in a table cell the text of the cell; and it inserts a table below the paragraph of the caret. Strong and emphasised
text is written as a call, `#strong[…]` and `#emph[…]`, which holds inside a word and across paragraphs, where stars
and underscores would not; stars a source already has become such a call where an edit would make a letter touch
them. At its right end it says how many pages the document has,
or the first error that keeps it from being set. `Toolbar(false)` hides it, for a program with a toolbar of its
own, which calls `ToggleStrong`, `ToggleEmphasis`, `SetHeading`, `ToggleList` and `InsertTable` of the document.

### Size

The pages are fitted to the width of the editor, at twice their size at most, and scroll. `Zoom` fixes the scale in
pixels per point instead.

### What it is not

There is no undo. Text that a document generates, such as page numbers or the numbers of a list, cannot take the
caret, and neither can text inside code, such as a call that is not content. Images, files and packages are not
available to documents yet.

## API

{{< api pkg="typstview" type="EditorView" >}}

{{< api pkg="typstview" type="Document" >}}

{{< api-funcs pkg="typstview" names="Watch" >}}

## Related

- [Typst PDF](../typst-pdf/) shows the pages without editing and exports them as a PDF file.
- [Code Editor](../../basic/code-editor/) edits the source as code.
- [Typst example](../../../examples/example-typst/) edits a document in its source and on its pages side by side.
