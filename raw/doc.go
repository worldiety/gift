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
// # Developing the raw data
//
// [File.ReadMosaic] decodes the sensor data: of an ARW uncompressed or in
// Sony's compressed raw, of a DNG uncompressed in 8 to 16 bits, in lossless
// JPEG or Deflate, in tiles or strips, through its linearization table.
// [File.Develop] develops it in tiles of float32, in
// parallel: black and white levels, white balance, the highlights that
// clipped, which it reconstructs from the channels that did not, and
// demosaicing by RCD, or at half size without. What comes out is linear
// light in the colours of the camera, white at 1:
//
//	err := f.Develop(raw.Options{}, func(t raw.Tile) {
//	    // t.R, t.G, t.B for t.Rect of the picture; from several goroutines
//	})
//
// # What it does not do yet
//
// The colours of the camera are not yet those of a display, nor its tones
// those of the camera's pictures: the stages R3 and R4 of the project plan,
// section 51. Of DNG it does not decode floating point data, lossy JPEG,
// JPEG XL and linear, already demosaiced data, and it does not apply the
// opcode lists, such as the gain maps of phones.
//
// The decryption of the SR2 block and the decoding of Sony's compressed raw
// follow dcraw by Dave Coffin, demosaicing the description of RCD by Luis
// Sanz Rodríguez; see NOTICE.
package raw
