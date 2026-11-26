---
title: Secure Field
---

`ui.SecureField` is a [Text Field](../text-field/) for a password or a PIN, the counterpart of SwiftUI's
`SecureField`: it shows a dot for every character. Everything else works as in a text field: the editor, the
binding, the callbacks, the look, the focus and the on-screen keyboard.

{{< gallery-shot alt="A user name field, a password field showing dots with an eye button, and a focused PIN field whose last digit is still shown" >}}

{{< gallery-code >}}

## Usage

```go
password := ui.Editor(ctx, "password", "")
ui.SecureField(password).Placeholder("Password").OnSubmit(signIn)
```

`password.Text()` is the password in the clear; only the screen and the accessible value see dots.

## What is different from a text field

- **Dots.** The field draws one dot per character, about four tenths of the font size across, drawn by the field
  rather than taken from the font, so they look the same in every font. A click, the caret and the selection still
  land between the same characters as in the clear.
- **No copy, no cut.** The shortcuts do nothing, so the password does not end up on a clipboard that every other
  program can read. Paste works, for a password manager.
- **No words.** A double click selects everything, and the word motions go to the start and the end. Stopping at a
  space would show where the spaces are.
- **Nothing to read out.** The value the field reports to accessibility tools, tests and giftauto is the dots, not
  the text. A test reads the password from the editor.

## The last character

On a touch keyboard a mistyped letter is otherwise only found when the sign-in fails, so iOS shows the character just
typed for a moment. `ShowsLastCharacter(true)` does the same: the character stays in the clear for
`ui.SecureRevealDuration` (one and a half seconds), or until the next key, press or focus change. The default is on
for a kiosk with `ui.SetOnScreenKeyboard` and where the backend reports a system keyboard (Android and iOS with the
Ebitengine backend, unless `Config.NoNativeKeyboard`), off on a desktop, where macOS never shows it.

## Showing the password

`Revealed(true)` shows the text in the clear. Bind it to state and put an eye button next to the field, as the demo
does:

```go
show := ctx.State("show", false)
eye := outline.Eye
if ctx.Read(show) {
	eye = outline.EyeSlash
}
ui.HStack(
	ui.SecureField(password).Revealed(show.Get()).
		Background(ui.ColorClear).Border(ui.Border{}).FocusRing(ui.Border{}).Flex(1),
	ui.Button(ui.Icon(eye).Size(16), func() { show.Set(!show.Get()) }).
		Background(ui.ColorClear).Border(ui.Border{}).Label("Show password"),
).Background(ui.ColorControl).CornerRadius(6)
```

## API

`SecureField` returns a `ui.TextFieldView`, so it is listed with that type; `Revealed` and `ShowsLastCharacter` are
its modifiers and do nothing to a plain text field. See [Text Field](../text-field/) for the rest.

{{< api "TextFieldView" >}}

## Related

- [Text Field](../text-field/), the field it is built on
- [Keyboard](../../dialogs/keyboard/), the on-screen keyboard of a kiosk
