---
title: Text Field
---

A text field lets the user enter one line of text: a name, an email address, a search term. It handles the
whole editing interaction for you. A click places the caret, a drag selects, a double click selects a word and a
triple click selects everything. The arrow keys, home, end, backspace and delete work as on the desktop, and select
all, copy, cut and paste sit on the usual shortcuts. A value wider than the field scrolls horizontally and keeps
the caret in view.

The field is a declaration over a `ui.TextEditor`, which holds the text, the caret and the selection. Use a text
field for short free-form input. For a choice out of a few fixed values use a
[Segmented Control](../segmented-control/), for on/off a [Toggle](../toggle/).

{{< gallery-shot alt="Four text fields: a focused one with a selected word, one with a placeholder, a search box with an icon and a disabled one" >}}

{{< gallery-code >}}

## Usage

### Editor and field

`ui.TextField` takes a `*ui.TextEditor`. Your component rebuilds whenever state it reads changes, often while the
user types, and a view does not survive a rebuild. So the caret and the selection cannot live in the view. They live
in the editor, which your application keeps. Inside a
component, `ui.Editor(ctx, name, initial)` creates one on the first build and returns the same one afterwards. It
is ordinary component state underneath, so it goes away with its component:

```go
name := ui.Editor(ctx, "name", "")
return ui.VStack(
	ui.TextField(name).Placeholder("Your name"),
	ui.Button(ui.Text("Save"), func() { save(name.Text()) }),
)
```

`initial` is only used on the first build, like the initial value of any other state. Outside a component, create
the editor with `ui.NewTextEditor(s)` and keep it yourself, for example in a package variable. It starts with the
caret after the last character and nothing selected.

One editor belongs to one field. Two mounted fields over the same editor panic, because an editor has a single
caret, selection and scroll offset. A nil editor panics too.

### Reading and writing the value

`Text` returns the current value at any time, which is all a button that submits a form needs, as in the snippet
above. `SetText` replaces the value, moves the caret to the end and clears the selection. Use it to reset a form
or to load a record.

Typing repaints the field but does not rebuild your component. A view that shows the value somewhere else, or a
button that depends on it, needs state. Bind the field to a `gift.State` with `Bind`: every edit writes the state,
and a value you write to the state replaces the text in the field:

```go
query := ctx.State("query", "")
field := ui.TextField(ui.Editor(ctx, "query-editor", "")).
	Placeholder("Search").
	Bind(query.Binding())
return ui.VStack(
	field,
	ui.Text("Results for " + ctx.Read(query)),
	ui.Button(ui.Text("Clear"), func() { query.Set("") }),
)
```

The field still needs its editor; the state holds the value and the editor holds the caret.

`OnChange` runs after every change with the new value, one call per keystroke, without a debounce. `OnSubmit` runs
when the user presses enter, with the current value. Both compose with `Bind`; the binding is written first. There
is no commit on blur: the value the callbacks see is always the value on the screen.

### Placeholder and label

`Placeholder` sets the text shown while the field is empty, in `ui.ColorSecondaryLabel` unless `PlaceholderColor`
says otherwise. It also serves as the accessible name of the field, which is how tests and accessibility tools
find it. `Label` sets a different accessible name; it is never drawn. A field with a visible title above it, as in
the demo, can repeat that title in `Label`:

```go
ui.TextField(email).Placeholder("you@example.com").Label("Email")
```

### Size

A field is one line tall: the line height of its font plus the padding, 5 points at the top and bottom and 8 at the
sides by default. `FontSize` changes the line, and with it the height. `Padding` and `PaddingInsets` replace the
padding, and `MinHeight` gives a taller field with the line centered in it, for example
`MinHeight(ui.ControlHitTarget)` on a touch screen.

A field does not grow with its text. It takes the width it is offered, and where the width is unbounded, as for an
inflexible child of an `HStack`, it is 180 points wide. `Frame`, `MinWidth` and `MaxWidth` set another width;
`Flex(1)` lets it fill the rest of a row:

```go
ui.HStack(
	ui.TextField(city).Placeholder("City").OnSubmit(add).Flex(1),
	ui.Button(ui.Text("Add"), func() { add(city.Text()) }),
).Gap(8)
```

Next to other views in an `HStack`, the field reports the baseline of its text, so `AlignBaseline` lines it up with
a label in front of it.

### Look

Without styling, a field uses the theme: the face is `ui.ColorControl` with a one point hairline in
`ui.ColorSeparator` and a corner radius of 6. The text is `ui.ColorLabel`, the caret has the color of the text, and
the selection is the accent at 30 percent. `Background`, `Border`, `CornerRadius` and `Shadow` replace the box, and
`Foreground`, `CaretColor` and `SelectionColor` the colors inside it. `Font` sets a font for this field only.

While the field holds the keyboard focus and the focus is visible, it draws a two point ring in `ui.ColorAccent`.
`FocusRing` replaces it, and a zero width border removes it. The search box of the demo uses that: a field with
`ui.ColorClear` as background, `ui.Border{}` as border and no focus ring, inside a rounded stack that carries the
face and the icon.

The text is always clipped to the field, because it scrolls inside it. `Clip(false)` panics.

### Focus and keyboard

A click or a tap on the field takes the focus, and so does tab. A field that takes the focus scrolls itself into
view if it sits in a scroll view. A drag that leaves the press more vertically than horizontally belongs to that
scroll view, so a column of fields on a touch screen still scrolls. `Disabled(true)` takes the field out of input: it is skipped by the focus order,
receives no events, shows no caret and draws quieter text on `ui.ColorControlDisabled`.

Inside a focused field:

| Keys                                   | Action                                    |
|----------------------------------------|-------------------------------------------|
| left, right                            | move the caret, with shift extend the selection |
| left, right with the word modifier     | move by words: option on macOS, control elsewhere |
| left, right with command on macOS      | go to the start or the end of the line    |
| home, end, up, down                    | go to the start or the end of the line    |
| backspace, delete                      | delete before or after the caret, or the selection |
| shortcut modifier with A, C, X, V      | select all, copy, cut, paste: command on macOS, control elsewhere |
| enter                                  | run `OnSubmit`                            |

Shift extends the selection with every motion. Outside macOS the word modifier and the shortcut modifier are both
control, and control with an arrow moves by words. The modifiers are the variables `gift.WordModifier` and
`gift.ShortcutModifier`; assign to them before the first frame if your keyboard disagrees with the operating system.

The shortcut letters are physical key positions of a US keyboard, so on other layouts they sit where the US keys
would be. There is no undo, and no multi-line input: enter submits, and pasted text ends at its first line break.
The field is not an input method for CJK composition, but umlauts, accents, AltGr and dead keys arrive as ordinary
characters and are inserted like any other.

### Clipboard

By default, copy and paste go through `ui.MemoryClipboard`, which only exchanges text inside your process. To
exchange text with other programs, import the clipboard package for its side effect:

```go
import _ "github.com/worldiety/gift/clipboard"
```

It supports macOS and Linux with X11 and needs no cgo. `ui.SetClipboard` installs any other implementation of
`ui.Clipboard`; call it before the first frame. See the
[package documentation](https://pkg.go.dev/github.com/worldiety/gift/clipboard) for the size cap and the platform
details.

### On-screen keyboard

A kiosk without a hardware keyboard needs gift's own on-screen keyboard. Turn it on with `ui.SetOnScreenKeyboard`
and place `ui.OnScreenKeyboard()` once, as a direct child of the window, aligned to the bottom:

```go
ui.SetOnScreenKeyboard(nil, true) // in main, next to ui.SetDefaultFont

app := gift.New(gift.Options{Root: func(ctx *gift.Context) gift.View {
	return ui.Window(
		form(ctx),
		ui.OnScreenKeyboard(),
	).Align(geom.Bottom)
}})
```

With the switch on, a field asks for the keyboard when it gains the focus and dismisses it when it loses it, and
the window lays the form out above the keyboard while it shows. The switch is off by default, because gift cannot
tell a kiosk from a desktop. Passing the app instead of nil rebuilds the window right away. See
[Keyboard](../../dialogs/keyboard/) for the keyboard itself.

## API

{{< api "TextFieldView" >}}

{{< api "TextEditor" >}}

{{< api-funcs pkg="ui" names="SetOnScreenKeyboard,SetClipboard" >}}

## Related

- [Keyboard](../../dialogs/keyboard/), the on-screen keyboard of a kiosk
- [Button](../button/), which usually submits what a field holds
- [Segmented Control](../segmented-control/) and [Toggle](../toggle/), controls for fixed values
- [Scroll View](../../layout/scroll-view/), for a form taller than the window
- [State](../../../concepts/state/), [Input and Focus](../../../concepts/input-and-focus/),
  [Text and Fonts](../../../concepts/text-and-fonts/) and [Kiosks](../../../concepts/kiosks/)
- The example [kitchen sink](../../../examples/example-kitchensink/), whose form tab has a field with the
  on-screen keyboard
