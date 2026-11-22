package raw

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"runtime"

	"github.com/worldiety/gift/cms"
)

// Decoder decodes raw files for package asset, whose Decoder,
// ScaledDecoder, Sniffer, ProfileReader and Versioned it is without this
// package importing asset. A program that shows raw files registers it for
// each format it wants:
//
//	asset.RegisterDecoder(raw.MIMEARW, raw.Decoder{Format: raw.ARW})
//	asset.RegisterDecoder(raw.MIMEDNG, raw.Decoder{Format: raw.DNG})
//
// By default it decodes the largest preview of the file: the photograph as
// the camera rendered it, in the style, white balance and colour space it
// was set to. The previews of an Alpha 7 III are 1616 by 1080 pixels.
//
// With Develop, it develops the raw data instead, as [File.RGBA] does: the
// whole resolution, in the colours of the camera's profile and the tones
// of this package, not the camera's style. A thumbnail takes the raw data
// at half size, which is a fifth of a second for 24 megapixels; a file this
// package cannot develop, of a camera it has no matrix for, shows its
// preview.
//
// A Decoder is a value without state and safe for concurrent use.
type Decoder struct {
	// Format is the format the decoder takes; FormatUnknown takes every
	// format this package reads.
	Format Format
	// Develop develops the raw data rather than decoding the preview.
	Develop bool
	// Space is the colour space developed pictures are made in, nil for
	// sRGB: the space of the pipeline, asset.Config.Space, so that what
	// the space can show of the camera's colours is not clipped twice.
	Space *cms.Profile
}

// Sniff reports whether head begins a raw file of the decoder's format.
func (d Decoder) Sniff(head []byte) bool {
	f := Sniff(head)
	return f != FormatUnknown && (d.Format == FormatUnknown || d.Format == f)
}

// source is what the decoder makes a picture of: the developed raw data of
// f, or its preview p.
type source struct {
	f       *File
	p       Preview
	develop bool
}

func (d Decoder) open(r io.Reader) (source, error) {
	ra, size, err := readerAt(r)
	if err != nil {
		return source{}, err
	}
	f, err := Open(ra, size)
	if err != nil {
		return source{}, err
	}
	if d.Format != FormatUnknown && f.Format != d.Format {
		return source{}, ErrFormat
	}
	if d.Develop && f.developable(d.Space) {
		return source{f: f, develop: true}, nil
	}
	p, ok := f.Largest()
	if !ok {
		return source{}, ErrNoPreview
	}
	return source{f: f, p: p}, nil
}

// developable reports whether f can be developed into space, as far as its
// metadata tells: a mosaic of red, green and blue in data this package
// decodes, of a camera it has a profile for.
func (f *File) developable(space *cms.Profile) bool {
	m := f.Mosaic
	if m.SamplesPerPixel != 1 || len(m.CFA) != 4 || m.Crop.Dx() < 2 || m.Crop.Dy() < 2 {
		return false
	}
	switch {
	case f.Format == ARW && (m.Compression == SonyCompressed || m.Compression == Uncompressed && m.BitsPerSample <= 16):
	case f.Format == DNG && f.layout.sampleFormat == 1 && m.BitsPerSample <= 16 &&
		(m.Compression == Uncompressed || m.Compression == LosslessJPEG || m.Compression == Deflate):
	default:
		return false
	}
	// A file cut short has no raw data to develop.
	for _, s := range f.segs {
		if s.n <= 0 || s.off < f.t.base || s.off > f.t.end-s.n {
			return false
		}
	}
	if len(f.segs) == 0 {
		return false
	}
	if _, err := f.Profile(); err != nil {
		return false
	}
	_, err := outputOf(cms.LinearRec2020(), space)
	return err == nil
}

// DecodeConfig is the size of the picture: the crop of the raw data when
// it is developed, else the largest preview.
func (d Decoder) DecodeConfig(r io.Reader) (image.Config, error) {
	s, err := d.open(r)
	if err != nil {
		return image.Config{}, err
	}
	if s.develop {
		b := s.f.Bounds(Options{})
		return image.Config{Width: b.Dx(), Height: b.Dy(), ColorModel: color.RGBAModel}, nil
	}
	return image.Config{Width: s.p.Width, Height: s.p.Height, ColorModel: s.p.model}, nil
}

// Decode decodes the picture whole: the raw data developed at full size,
// or the largest preview.
func (d Decoder) Decode(r io.Reader) (image.Image, error) {
	return d.DecodeScaled(r, math.MaxInt, math.MaxInt)
}

// DecodeScaled develops the raw data at half size when that is at least
// minW by minH, at full size otherwise; a preview it decodes whole.
func (d Decoder) DecodeScaled(r io.Reader, minW, minH int) (image.Image, error) {
	s, err := d.open(r)
	if err != nil {
		return nil, err
	}
	if !s.develop {
		return s.f.DecodePreview(s.p)
	}
	opt := Options{Space: d.Space}
	if half := s.f.Bounds(Options{Half: true}); minW <= half.Dx() && minH <= half.Dy() {
		opt.Half = true
	}
	return s.f.RGBA(opt)
}

// MemoryFactor is that of decoding a JPEG, which the preview is; reading
// the metadata takes a few kilobytes. Developed, [Decoder.ScaledMemory]
// tells.
func (Decoder) MemoryFactor() float64 { return 6 }

// ScaledMemory is the memory DecodeScaled takes: for the raw data, two
// bytes a photosite, the picture of four bytes a pixel, and the tiles the
// workers develop in.
func (d Decoder) ScaledMemory(header []byte, w, h, minW, minH int) int64 {
	s, err := d.open(bytes.NewReader(header))
	if err != nil || !s.develop {
		return int64(6 * float64(w) * float64(h))
	}
	m := s.f.Mosaic
	half := s.f.Bounds(Options{Half: true})
	out := s.f.Bounds(Options{})
	tile := int64(3 * 4 * (tileSize + 2*tileBorder) * (tileSize + 2*tileBorder))
	if minW <= half.Dx() && minH <= half.Dy() {
		out = half
		tile = int64(3 * 4 * tileSize * tileSize)
	} else {
		// RCD keeps some dozen planes of its tile.
		tile *= 12
	}
	return 2*int64(m.Width)*int64(m.Height) + 4*int64(out.Dx())*int64(out.Dy()) +
		int64(runtime.GOMAXPROCS(0))*tile
}

// ReadProfile is the colour space of the picture: that of the decoder when
// the raw data is developed, else that of the largest preview as the file
// declares it, the colour space the camera was set to, sRGB or Adobe RGB.
// A nil profile means sRGB.
func (d Decoder) ReadProfile(encoded []byte) (*cms.Profile, error) {
	s, err := d.open(bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	if s.develop {
		return d.Space, nil
	}
	return s.p.Space, nil
}

// DecoderVersion is what the pictures of a developing decoder depend on,
// for the keys of a cache of them: [ProcessVersion] and the colour space;
// empty for previews, which are the camera's.
func (d Decoder) DecoderVersion() string {
	if !d.Develop {
		return ""
	}
	v := fmt.Sprintf("raw%d", ProcessVersion)
	if d.Space != nil {
		sum := sha256.Sum256(d.Space.Bytes())
		v += "-" + hex.EncodeToString(sum[:6])
	}
	return v
}
