// Package raw reads the raw files of cameras in pure Go: Sony's ARW and
// Adobe's DNG.
//
// # What it reads
//
// [Open] reads what a raw file says about itself, without its tens of
// megabytes of pixels:
//
//   - the sensor data, as [Mosaic] describes it: its size and the part of
//     it that is the picture, the colour filter pattern, the black and
//     white levels, and how it is stored;
//   - the white balance the photograph was taken with;
//   - the previews the camera rendered and embedded, which [File.DecodePreview]
//     decodes;
//   - of an ARW, the settings of the camera in [Sony]: the creative style
//     and its levels of contrast, saturation and sharpness, the dynamic
//     range optimiser, the white balance setting and the camera's presets.
//     Some of them are in the MakerNote, the rest in the SR2 block, which
//     the camera encrypts and this package decrypts;
//   - of a DNG, the colour description of [DNGColor]: the matrices of the
//     camera under two illuminants and the white of the photograph.
//
// # Showing raw files in a gallery
//
// [Decoder] is a decoder for package asset, which shows a raw file as its
// largest preview, the photograph as the camera rendered it, or, with
// Decoder.Develop, as its raw data developed by this package:
//
//	asset.RegisterDecoder(raw.MIMEARW, raw.Decoder{Format: raw.ARW})
//	asset.RegisterDecoder(raw.MIMEDNG, raw.Decoder{Format: raw.DNG, Develop: true, Space: space})
//
// # Developing the raw data
//
// [File.ReadMosaic] decodes the sensor data: of an ARW uncompressed or in
// Sony's compressed raw, of a DNG uncompressed in 8 to 16 bits, in lossless
// JPEG or Deflate, in tiles or strips, through its linearization table.
// [File.Develop] develops it in tiles of float32, in parallel: black and
// white levels, white balance, the highlights that clipped, which it
// reconstructs from the channels that did not, and demosaicing by RCD, or
// at half size without. [File.RGBA] and [File.RGBA64] make a picture of it:
//
//	img, err := f.RGBA(raw.Options{Half: true, Space: cms.DisplayP3()})
//
// # Colours and tones
//
// The colours of the camera become those of the light it saw through a
// camera profile, [Profile], as the colour model of the DNG specification
// has it: one or two colour matrices measured under two illuminants and
// interpolated for the white of the photograph, forward matrices, and the
// tables of hue, saturation and value and the look table of a profile that
// has them. A DNG carries its profile; for an ARW the package has the
// matrices of Sony's cameras up to 2018, the Alpha 7 III among them, as
// dcraw tabulates them; a DNG camera profile of the user's, a .dcp file,
// is read by [ParseProfile] and replaces either. The package ships no
// profiles of others.
//
// The light of the scene is linear Rec. 2020, [OutputScene], the camera's
// white at 1. A picture, [OutputPicture], is made of it as the camera made
// its own, [LookCamera], the default: the look of each file is measured
// against the preview the camera rendered from the same raw data, a matrix
// and a tone curve fitted to it in cells of 4 by 4 of the preview's pixels,
// which takes some 70 milliseconds once a file. It carries the camera's
// style with its contrast and saturation, the mean of what the dynamic
// range optimiser did, and black and white: a photograph taken so develops
// so. Against the Alpha 7 III's own JPEGs it is within a ΔE of 1 to 3.
//
// [LookNeutral], and every file without a usable preview, takes a tone
// curve in the manner of a camera's: an S in stops, applied channel by
// channel, which puts middle gray 3.7 stops below the camera's white and
// lets the highlights above it roll off; a profile that brings a tone
// curve has it applied instead, keeping the hue, as Adobe's raw converters
// do. Last, the colours are brought into the colour space of the picture,
// those beyond it compressed towards gray rather than clipped.
// [ProcessVersion] versions all of it.
//
// # What it does not do yet
//
// The look is measured as a whole: the creative style, its levels of
// contrast, saturation and sharpness and the dynamic range optimiser are
// not yet steps of their own that could be set apart from the camera's
// choice, and what the optimiser does locally, lifting shadows here more
// than there, is in the look only as its mean. Of DNG it does not decode
// floating point data, lossy JPEG, JPEG XL and linear, already demosaiced
// data, and it does not apply the opcode lists, such as the gain maps of
// phones. Profiles of a third illuminant are read as their first two, and
// DefaultBlackRender is not applied.
//
// The decryption of the SR2 block, the decoding of Sony's compressed raw
// and the table of camera matrices follow dcraw by Dave Coffin,
// demosaicing the description of RCD by Luis Sanz Rodríguez; see NOTICE.
package raw
