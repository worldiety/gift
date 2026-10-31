---
title: Text Area
---

A text area lets the user enter text of several lines: a note, a message, an address. It is the multi-line
sibling of the [Text Field](../text-field/), over the same `ui.TextEditor`, and the counterpart of SwiftUI's
`TextEditor`. Lines wrap at the width of the area and enter starts a new one. The area grows with its text and
scrolls its lines once it has reached its largest size.

{{< gallery-shot alt="Two text areas: a focused one with two paragraphs, the second wrapped, and a selection over two lines; and an empty one with a placeholder" >}}

{{< gallery-code >}}

## Usage

```go
notes := ui.Editor(ctx, "notes", "")
ui.TextArea(notes).Placeholder("Notes").Lines(3, 8)
```

The editor, `Bind`, `OnChange`, `Placeholder`, `Label`, `Disabled`, the fonts, the colors and the box modifiers work
as in a [Text Field](../text-field/). Enter is a line break here, so `OnSubmit` runs on the shortcut modifier plus
enter. Tab still moves the focus on, as in every form.

### Size

`Lines(min, max)` sets the height in lines of the font: at least `min`, growing with the text up to `max`, past which
the lines scroll. A `max` of zero grows without limit, which suits an area inside a scrolling form. The default is
three lines growing without limit. A `Frame`, `MinHeight` or `MaxHeight` overrides both, and the lines scroll inside
whatever height the area gets.

The width is the width it is offered, 280 points where nothing bounds it, and never the width of the text.

### Scrolling

When the text has more lines than fit, the area scrolls them itself:

- the **wheel** scrolls them, and passes on to the form around the area once they are at their end;
- a **finger** that moves more up or down than sideways scrolls them, or the form when they all fit;
- the **caret** is kept in view while typing and moving, and a drag past the top or the bottom edge scrolls on.

A thin indicator shows at the right edge while the lines scroll.

### Mouse and finger

A mouse click places the caret, a drag selects, in any direction, a double click selects a word and a triple
click the paragraph, the text between two line breaks. A finger places the caret when it lifts, so that a finger
that scrolls leaves the caret where it was; a finger that moves sideways selects from where it pressed.

### Keyboard

| Keys                                         | Action                                                   |
|----------------------------------------------|----------------------------------------------------------|
| left, right                                  | move the caret, with shift extend the selection          |
| up, down                                     | move to the line above or below, keeping the column      |
| page up, page down                           | move by the lines in view                                |
| home, end                                    | go to the start or the end of the line on the screen     |
| left, right with command on macOS            | go to the start or the end of the line on the screen     |
| left, right with the word modifier           | move by words                                            |
| home, end, up, down with the shortcut modifier | go to the start or the end of the text                 |
| enter                                        | start a new line                                         |
| shortcut + enter                             | `OnSubmit`, or a new line without one                    |
| backspace, delete                            | delete before or after the caret, or the selection       |
| shortcut modifier with A, C, X, V            | select all, copy, cut, paste                             |

Up and down keep the column they started in across shorter lines, as in every editor. A pasted text keeps its
line breaks; carriage returns become plain line breaks and a tab becomes four spaces.

### What it is not

There is no undo and no rich text. A word longer than the width is not broken; it runs past the right edge and is
cut there.

## API

{{< api "TextAreaView" >}}

## Related

- [Text Field](../text-field/), for one line
- [Secure Field](../secure-field/), for a password
- [Keyboard](../../dialogs/keyboard/), the on-screen keyboard of a kiosk, whose return key starts a new line here
