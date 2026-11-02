// Package qr encodes text as a QR code and shows it: a link a phone opens
// with its camera, the way a kiosk hands a login or a payment to the
// customer's own device (see web/handoff).
//
//	qr.View("https://example.com/pay/4711").Size(240)
//
//	code, err := qr.Encode("https://example.com", qr.M)
//	png.Encode(f, code.Image(8))
//
// [Encode] writes the model 2 codes of ISO/IEC 18004, versions 1 to 40, at
// all four levels of error correction, with the mask chosen by the penalty
// rules of the standard. A text is one segment, numeric, alphanumeric or
// bytes; mixed segments, Kanji, ECI and structured append are left out,
// none of which a link needs.
//
// [View] draws the modules as rectangles snapped to the pixels of the
// screen, black on white whatever the appearance, inside the quiet zone of
// four modules the standard asks for.
//
// SwiftUI has no QR view; an iOS application asks Core Image's
// CIQRCodeGenerator for a picture and scales it without smoothing. This
// package is that filter and the view in one.
package qr
