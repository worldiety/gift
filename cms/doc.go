// Package cms is colour management in pure Go: ICC profiles and the
// transforms between them, as LittleCMS makes them, without cgo.
//
// # What it is for
//
// A photograph says which colour space its numbers are in, Adobe RGB from
// a camera, Display P3 from a phone, ProPhoto from Lightroom, with an ICC
// profile in the file; a display and a printer have colour spaces of their
// own. Numbers shown in a space they were not made for are why a picture
// looks dull or garish. This package reads profiles, links two of them and
// converts pixels from the one to the other:
//
//	src, err := cms.ParseProfile(icc) // the profile embedded in the file
//	if err != nil {
//	    src = cms.SRGB()              // what a file without one is taken to be
//	}
//	t, err := cms.NewTransform(src, cms.SRGB(), cms.Options{Intent: cms.Perceptual})
//	...
//	err = t.Pixels(img.Pix, cms.RGBAPremul8, img.Pix, cms.RGBAPremul8)
//
// SRGB, DisplayP3, AdobeRGB, ProPhotoRGB, Rec2020, their linear forms and
// SGray are built in; NewRGBProfile and NewGrayProfile make others, and
// Profile.Bytes is the profile to embed in a file that is written.
//
// # What it reads
//
// ICC profiles of version 2 and 4 for RGB, Gray and CMYK devices that
// connect through XYZ or Lab: matrix profiles, as sRGB and most displays
// have, and table profiles, as cameras, scanners and printers have, with
// lut8, lut16, lutAToB and lutBToA tables and every element of them, and
// curves as gammas, the five parametric functions and tables. Not read are
// device links, abstract and named colour profiles, the floating point
// tables of ICC v4.3, and Lab or XYZ as the data of a file.
//
// # How it converts
//
// The colour science is that of LittleCMS 2, so that a picture converted
// here agrees with one converted by any of the many programs built on it:
// which table serves which rendering intent and what stands in for a
// missing one, the v2 encoding of Lab in lut16 tables, the media white of
// the absolute intent, black point detection, including Adobe's algorithm
// for the black of a printer, and black point compensation, which ICC v4
// destinations get for the perceptual intent whether asked for or not.
// TestAgainstLittleCMS holds the package to it: against LittleCMS's exact
// pipeline, Color is within one 8 bit step, which is rounding, and Pixels
// is no further off than LittleCMS's own optimised tables are, or two
// steps, whichever is more.
//
// NewTransform evaluates the profiles once and bakes the result: two
// matrix profiles become curve tables and a matrix, anything else a lookup
// table of 49 points per input channel, 17 for CMYK, with the curves at
// its ends kept out of the table, where they would bend between grid
// points. Baking takes a few milliseconds; converting 8 bit RGB between
// two matrix profiles takes about 9 ns a pixel on one core of an Apple M5,
// through a CMYK table about 40. Pixels works in the goroutine it is called
// from; a Transform is immutable and safe to share, so a large picture can
// be converted in bands at once.
//
// # Provenance
//
// The rules above are those of LittleCMS by Marti Maria Saguer, MIT
// licensed. The way they are carried over follows prism/meta/icc in
// github.com/kovidgoyal/imaging by Kovid Goyal, MIT licensed, which found
// most of them in the LittleCMS sources first. The code here is written for
// this package; NOTICE has both licences.
package cms
