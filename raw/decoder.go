package raw

import (
	"bytes"
	"image"
	"io"

	"github.com/worldiety/gift/cms"
)

// Decoder decodes raw files for package asset, whose Decoder, Sniffer and
// ProfileReader it is without this package importing asset. A program that
// shows raw files registers it for each format it wants:
//
//	asset.RegisterDecoder(raw.MIMEARW, raw.Decoder{Format: raw.ARW})
//	asset.RegisterDecoder(raw.MIMEDNG, raw.Decoder{Format: raw.DNG})
//
// It decodes the largest preview of the file: the photograph as the camera
// rendered it, in the style, white balance and colour space it was set to.
// A thumbnail of a raw file is that picture. The previews of an Alpha 7 III
// are 1616 by 1080 pixels, which is what a raw file shows as until the raw
// data is developed itself.
//
// A Decoder is a value without state and safe for concurrent use.
type Decoder struct {
	// Format is the format the decoder takes; FormatUnknown takes every
	// format this package reads.
	Format Format
}

// Sniff reports whether head begins a raw file of the decoder's format.
func (d Decoder) Sniff(head []byte) bool {
	f := Sniff(head)
	return f != FormatUnknown && (d.Format == FormatUnknown || d.Format == f)
}

func (d Decoder) open(r io.Reader) (*File, Preview, error) {
	ra, size, err := readerAt(r)
	if err != nil {
		return nil, Preview{}, err
	}
	f, err := Open(ra, size)
	if err != nil {
		return nil, Preview{}, err
	}
	if d.Format != FormatUnknown && f.Format != d.Format {
		return nil, Preview{}, ErrFormat
	}
	p, ok := f.Largest()
	if !ok {
		return nil, Preview{}, ErrNoPreview
	}
	return f, p, nil
}

// DecodeConfig is the size and colour model of the largest preview.
func (d Decoder) DecodeConfig(r io.Reader) (image.Config, error) {
	_, p, err := d.open(r)
	if err != nil {
		return image.Config{}, err
	}
	return image.Config{Width: p.Width, Height: p.Height, ColorModel: p.model}, nil
}

// Decode decodes the largest preview.
func (d Decoder) Decode(r io.Reader) (image.Image, error) {
	f, p, err := d.open(r)
	if err != nil {
		return nil, err
	}
	return f.DecodePreview(p)
}

// MemoryFactor is that of decoding a JPEG, which the preview is; reading
// the metadata takes a few kilobytes.
func (Decoder) MemoryFactor() float64 { return 6 }

// ReadProfile is the colour space of the largest preview, as the file
// declares it: the colour space the camera was set to, sRGB or Adobe RGB.
// A nil profile means the file does not say, and the picture is sRGB.
func (d Decoder) ReadProfile(encoded []byte) (*cms.Profile, error) {
	_, p, err := d.open(bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	return p.Space, nil
}
