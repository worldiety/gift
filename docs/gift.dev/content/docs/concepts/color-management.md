---
title: Color Management
weight: 21
---

The numbers of a color only mean something together with the color space they are in. A photograph from a camera
is often in Adobe RGB, one from a phone in Display P3, an export from Lightroom in ProPhoto RGB, and the file says so
with an ICC profile. A display and a printer have color spaces of their own. Numbers shown in a space they were not
made for are why a picture looks dull or garish. Package [`cms`](https://github.com/worldiety/gift/tree/main/cms)
converts between color spaces with ICC profiles, the way LittleCMS does, in pure Go and without cgo.

Everything gift paints is sRGB: the theme, text, and the pictures the image pipeline hands over. The Ebitengine
backend tells the system so, which matters on a Mac, where the screen would otherwise be taken as Display P3 and
every sRGB color shown more saturated than it is.

The [image pipeline](../images/) reads the color space every picture declares and converts its thumbnails into
sRGB, so a photograph in Adobe RGB looks in gift as it does in Lightroom or a browser. You only need `cms`
yourself for pixels that do not come through the pipeline.

## Pictures in the image pipeline

A picture says which color space its numbers are in, in one of these ways. `asset.ReadProfile` returns the profile
a file declares, `nil` for none, which means sRGB.

| Format | What is read |
|--------|--------------|
| JPEG | An ICC profile in APP2 segments, split over several when it is large. Without one, the EXIF of a camera set to Adobe RGB: those write no profile but the interoperability index R03, or the color space 2. |
| PNG | The chunks `cICP`, `iCCP`, `sRGB`, and `gAMA` with `cHRM`, in that order of precedence, as the third edition of the PNG specification gives it. |
| WebP | An ICC profile in the `ICCP` chunk, when a WebP decoder is [registered](../images/#other-formats). |

The pipeline converts with the perceptual intent, which for the matrix profiles of cameras and displays is the
same as relative colorimetric. It converts *after* scaling, so that a thumbnail pays for its own pixels and not for
those of the photograph: a 512 pixel thumbnail takes about 2.5 ms on an Apple M5. Two kinds of picture are
converted before scaling, in bands on every core: CMYK, because its colors come only from its profile, and 16 bit
pictures, whose shadows would band in 8 bits of a linear or wide space. A 24 megapixel CMYK picture takes about
120 ms on the ten cores of an M5. A camera embeds the same profile in every picture, and the pipeline bakes each profile once.

A profile that cannot be read, or does not fit the pixels, such as a CMYK profile in an RGB file, is logged at
debug level and the picture is shown as sRGB, as a program without color management would show it. The HDR
transfers PQ and HLG of a `cICP` chunk are not converted: they need a tone mapping to show on a display that is not
HDR. `Stats.Converted` counts the conversions.

## Write a profile into a file

A picture you export should say what space its numbers are in. `asset.EmbedProfile` writes a profile into the bytes
an encoder wrote, a JPEG or a PNG, and removes whatever the file declared before:

```go
var buf bytes.Buffer
if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
	return err
}
tagged, err := asset.EmbedProfile(buf.Bytes(), cms.DisplayP3())
if err != nil {
	return err
}
return os.WriteFile("export.jpg", tagged, 0o644)
```

A file without a profile is read as sRGB everywhere, so an sRGB picture needs none.

## Convert a picture

A transform links two profiles: the one the pixels are in and the one they should be in.

```go
src, err := cms.ParseProfile(icc) // the profile embedded in the file
if err != nil {
	src = cms.SRGB() // what a file without one is taken to be
}
t, err := cms.NewTransform(src, cms.SRGB(), cms.Options{Intent: cms.Perceptual})
if err != nil {
	return err
}
// img is an *image.RGBA; source and destination may be the same slice.
if err := t.Pixels(img.Pix, cms.RGBAPremul8, img.Pix, cms.RGBAPremul8); err != nil {
	return err
}
```

`NewTransform` evaluates both profiles once and bakes the result into tables, which takes a few milliseconds. Make
one per pair of profiles and keep it: a `Transform` is immutable and can be used from many goroutines at once.
`Pixels` converts in the goroutine it is called from, so a large picture can be converted in bands in parallel.
`Identity` reports a transform that changes nothing, such as sRGB to sRGB, so that you can skip it. `Color` converts
a single color exactly, without the tables.

## Profiles

| Profile | What it is |
|---------|------------|
| `cms.SRGB()` | sRGB, the space of the web, of most photographs and of gift |
| `cms.DisplayP3()` | The space of Apple's displays: the primaries of DCI-P3 with the white and curve of sRGB |
| `cms.AdobeRGB()` | Adobe RGB (1998), common in cameras |
| `cms.ProPhotoRGB()` | ProPhoto RGB with a gamma of 1.8, as Adobe's profile has it |
| `cms.Rec2020()` | ITU-R BT.2020 with the curve of BT.709 |
| `cms.LinearSRGB()`, `cms.LinearRec2020()` | The same primaries on linear light, for working spaces |
| `cms.SGray()` | Gray with the curve of sRGB, for gray pictures without a profile |

`cms.ParseProfile` reads any other: version 2 and 4, for RGB, gray and CMYK devices, with curves and a matrix as most
displays have or with tables as cameras, scanners and printers have. `cms.NewRGBProfile` and `cms.NewGrayProfile`
make a profile from primaries, a white point and a curve. `Profile.Bytes` is the profile to embed in a file you
write, so that other programs read its colors as you meant them.

Not supported are device links, abstract and named color profiles, the floating point tables of ICC v4.3 and files
whose data is Lab or XYZ.

## Rendering intents

When the destination cannot show a color, the rendering intent decides what happens.

| Intent | What it does |
|--------|--------------|
| `Perceptual` | Compresses the whole source into the destination as the profile's maker chose. The usual one for photographs. Matrix profiles such as sRGB treat it as relative colorimetric. |
| `RelativeColorimetric` | Keeps every color the destination can show and clips the others. White stays white. |
| `Saturation` | Keeps colors vivid rather than accurate, for charts and business graphics. |
| `AbsoluteColorimetric` | Like relative colorimetric, but white is not mapped to white: a proof shows the paper color of the printer. |

`Options.BlackPointCompensation` maps the darkest color of the source to the darkest of the destination, so that
shadows keep their detail instead of clipping to black. It is always on for the perceptual and saturation intents
when the destination is an ICC v4 profile, as the specification says.

## Pixel formats

`Pixels` reads and writes the pixel slices of the image package directly:

| Format | Layout | Image type |
|--------|--------|------------|
| `Gray8`, `Gray16` | One channel | `image.Gray`, `image.Gray16` |
| `RGB8`, `RGB16` | Three channels, no alpha | |
| `RGBA8`, `RGBA16` | Straight alpha | `image.NRGBA`, `image.NRGBA64` |
| `RGBAPremul8`, `RGBAPremul16` | Premultiplied alpha | `image.RGBA`, `image.RGBA64` |
| `CMYK8`, `CMYK16` | Four channels, 0 is no ink | `image.CMYK` |

The 16 bit formats are big-endian, as in the image package. Alpha is carried over from the source, or opaque where
it has none. A premultiplied pixel is converted as the color it shows, not as the darker color its numbers would be.

## Agreement with LittleCMS

The package carries over the rules of LittleCMS 2: which table serves which intent, the older Lab encoding of
version 2 tables, black point detection including Adobe's algorithm for printers, and the media white point of the
absolute intent. Its tests convert colors through more than a hundred pairs of profiles, synthetic ones that use
every kind of table and, on a Mac, the profiles of macOS, and compare them with LittleCMS 2.17. A single color
converted with `Color` is within one 8 bit step of LittleCMS's exact result. `Pixels`, which uses the baked tables, is
no further off than LittleCMS's own optimized tables are, and mostly much closer.

Converting 8 bit RGB between two matrix profiles takes about 9 nanoseconds per pixel on one core of an Apple M5;
through the table of a CMYK printer profile about 40.

## API

{{< api-funcs pkg="asset" names="ReadProfile,EmbedProfile" >}}

{{< api pkg="cms" type="Profile" >}}

{{< api pkg="cms" type="Transform" >}}

{{< api pkg="cms" type="Options" >}}

{{< api pkg="cms" type="Format" >}}

{{< api pkg="cms" type="Curve" >}}
