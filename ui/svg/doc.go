// Package svg draws SVG documents: the common subset that drawing programs
// and icon sets write, sharp at any size.
//
//	logo, err := svg.ParseFile("logo.svg")
//	svg.View(logo).Frame(120, 0)
//
// [Parse] reads paths and the basic shapes, groups, use and defs, colours,
// linear and radial gradients, strokes with their joins, caps and dashes,
// transforms, opacity and clip paths. What it does not draw — text, images,
// filters, masks, patterns, style sheets — it leaves out and names in
// [Image.Skipped]; a document that is no SVG at all is an error.
//
// An [Image] is rasterised on the CPU with golang.org/x/image/vector, shape
// by shape into the pixels it covers, and blended with its paint: there is
// no SVG parser in the frame, only a finished picture. [View] rasterises at
// the resolution of the screen, a large picture on a goroutine of its own,
// and shows it as a texture.
//
// SwiftUI draws an SVG of an asset catalog, not one read at run time; this
// package draws either.
package svg
