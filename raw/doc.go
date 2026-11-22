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
// largest preview, the photograph as the camera rendered it:
//
//	asset.RegisterDecoder(raw.MIMEARW, raw.Decoder{Format: raw.ARW})
//	asset.RegisterDecoder(raw.MIMEDNG, raw.Decoder{Format: raw.DNG})
//
// # What it does not do yet
//
// It does not develop the raw data. Decoding the mosaic, demosaicing it and
// rendering its colours and tones as the camera would are the stages R2 to
// R4 of the project plan, section 51.
//
// The decryption of the SR2 block follows dcraw by Dave Coffin; see NOTICE.
package raw
