---
title: QR Code
---

A QR code carries a link or a short text to the camera of a phone: the page of a login, a payment link, a code to
type. Package [`ui/qr`](https://github.com/worldiety/gift/tree/main/ui/qr) encodes the text and draws it, black on
white in either theme, because not every reader takes a code with its colours inverted.

{{< gallery-shot alt="Three QR codes on white squares: a link at 200 points, a short code at 120 points and the same link at the highest error correction, with a denser pattern" >}}

{{< gallery-code >}}

## Usage

```go
qr.View("https://example.com/pay/4711")             // four points a module, no larger than there is room
qr.View(link).Size(240)                             // 240 points a side, quiet zone included
qr.View(link).Level(qr.H).Label("Link to the menu") // more error correction, an accessible name
```

A view is four points a module without a size, which a phone reads from an arm's length, and never larger than there
is room; with `Flex` it takes the room a stack gives it and draws a square in the middle. The modules are snapped to
whole pixels of the screen, so that no module is a pixel wider than the next, and the square includes the quiet zone
of four modules that readers need around a code.

The error correction level says how much of a code may be hidden or damaged: `L` about 7 %, `M` 15 % (the default),
`Q` 25 %, `H` 30 %. A higher level makes the same text a larger code. `Encode` raises the level by itself as far as
the size of the code allows, so a short text gets the most robust code of its size.

## Encoding

`qr.Encode(text, level)` returns the code as a square of modules, for a view of your own or a file:

```go
code, err := qr.Encode("https://example.com", qr.M)
png.Encode(f, code.Image(8)) // eight pixels a module, black on white, with the quiet zone
```

It writes QR codes of model 2, versions 1 to 40, at all four levels, and chooses the mask by the penalty rules of
the standard. A text is encoded whole in one mode: digits only as numeric, upper case letters, digits and
` $%*+-./:` as alphanumeric, everything else as its UTF-8 bytes. An upper case code such as `WDJB-MJHT` is therefore
smaller than the same code in lower case. Mixed segments, Kanji, ECI and structured append are left out; a link
needs none of them. A text longer than 2953 bytes at level `L` is `ErrTooLong`, and a view of it shows the white square
alone.

## Swift

SwiftUI has no QR view. An app asks Core Image's `CIQRCodeGenerator` for a picture and scales it up without
smoothing; `qr.View` is that filter and the view in one.

## API

{{< api pkg="qr" type="CodeView" >}}

{{< api pkg="qr" type="Code" >}}

## Related

- [Device Login](../../web/device-login/) and [Payment Handoff](../../web/pay-handoff/) show a code with what the
  user does with it.
- [SVG Image](../svg-image/) draws a vector picture.
