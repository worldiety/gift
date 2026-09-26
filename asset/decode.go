package asset

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"sync"
)

// Decoder turns encoded bytes into an image.
//
// The two built in decoders are image/jpeg and image/png. Further formats are
// added with [RegisterDecoder], which the project plan, section 12, step 4,
// requires to be an explicit act: "weitere Formate nur ueber klar registrierte
// Decoder. Kein RAW-/Video-Support."
//
// A Decoder is used from several goroutines at once and must be safe for
// concurrent use.
//
// The reader the pipeline passes to Decode, and to [ScaledDecoder.DecodeScaled],
// is a *bytes.Buffer over the whole encoded picture, which the pipeline has in
// memory anyway. A decoder that needs the picture in one piece — one that
// hands it to a C library, say — can take it with Bytes instead of copying
// megabytes it has already been given; it must not modify them.
type Decoder interface {
	// DecodeConfig reads the dimensions and colour model from the head of
	// the stream, without decoding pixels.
	DecodeConfig(r io.Reader) (image.Config, error)

	// Decode decodes the whole picture.
	Decode(r io.Reader) (image.Image, error)

	// MemoryFactor is how many bytes of working memory the decoder needs per
	// pixel of the declared picture size. It is the number the pipeline
	// reserves its decode budget with, and the project plan, section 9, is
	// explicit that it is computed from the *picture's* dimensions and the
	// codec's risk, not from the size of the thumbnail that comes out.
	MemoryFactor() float64
}

// ScaledDecoder is an optional extension of [Decoder]: a decoder that can
// produce a smaller picture than the stored one directly, for less work than
// decoding all of it.
//
// JPEG is the case this exists for. Its inverse DCT can produce each 8x8
// block at an eighth, a quarter or a half of its size, so a 24 megapixel
// photograph wanted as a 512 pixel thumbnail need never exist at more than
// 750 by 500 pixels. image/jpeg cannot do that; libjpeg-turbo can, and
// github.com/worldiety/gift/asset/turbojpeg is the decoder that implements
// this interface with it.
//
// When the registered decoder has it, the pipeline uses it for every decode,
// and [Decoder.Decode] and [Decoder.MemoryFactor] are not called.
type ScaledDecoder interface {
	Decoder

	// DecodeScaled decodes the picture at some size of at least minW by
	// minH, in *stored* pixels, before any orientation is applied. Larger
	// is allowed — the pipeline scales the rest of the way, and a decoder
	// with fixed factors will rarely hit the size exactly — and so is the
	// full picture; smaller is not, unless the picture itself is, because
	// the resampler would then enlarge and the thumbnail lose sharpness it
	// could have had.
	DecodeScaled(r io.Reader, minW, minH int) (image.Image, error)

	// ScaledMemory is the working memory in bytes that DecodeScaled needs
	// for this picture: header is the encoded picture, w by h its stored
	// dimensions, minW by minH what will be asked for. It replaces
	// MemoryFactor times the stored pixels as the pipeline's decode
	// reservation, and the project plan, section 9, still holds for it: it
	// is computed from the picture and the codec's risk — a progressive
	// JPEG keeps every coefficient of the full picture however small the
	// output — and not merely from the size that comes out.
	ScaledMemory(header []byte, w, h, minW, minH int) int64
}

// Sniffer is an optional extension of [Decoder]: a decoder that can recognise
// its own format from the first bytes. Without it a decoder is only selected by
// media type.
type Sniffer interface {
	Sniff(header []byte) bool
}

type decoderEntry struct {
	mime string
	dec  Decoder
}

var (
	decoderMu sync.RWMutex
	decoders  = []decoderEntry{
		{MIMEJPEG, jpegDecoder{}},
		{MIMEPNG, pngDecoder{}},
	}
)

// RegisterDecoder registers a decoder for a media type, replacing any earlier
// registration of the same type.
//
// It is process wide, like image.RegisterFormat, and is meant to be called
// during start up, before a pipeline runs. Registering while decodes are in
// flight is safe but which decoder a request in flight uses is unspecified.
func RegisterDecoder(mimeType string, d Decoder) {
	if mimeType == "" || d == nil {
		panic("gift/asset: RegisterDecoder needs a media type and a decoder")
	}
	decoderMu.Lock()
	defer decoderMu.Unlock()
	for i := range decoders {
		if decoders[i].mime == mimeType {
			decoders[i].dec = d
			return
		}
	}
	decoders = append(decoders, decoderEntry{mimeType, d})
}

// decoderFor picks a decoder by sniffing the header first and by media type
// second. Sniffing wins because a server's Content-Type is a claim and the
// magic bytes are a fact.
func decoderFor(header []byte, mimeType string) (Decoder, string, bool) {
	decoderMu.RLock()
	defer decoderMu.RUnlock()
	for _, e := range decoders {
		if s, ok := e.dec.(Sniffer); ok && s.Sniff(header) {
			return e.dec, e.mime, true
		}
	}
	for _, e := range decoders {
		if e.mime == mimeType && mimeType != "" {
			return e.dec, e.mime, true
		}
	}
	return nil, "", false
}

type jpegDecoder struct{}

func (jpegDecoder) DecodeConfig(r io.Reader) (image.Config, error) { return jpeg.DecodeConfig(r) }
func (jpegDecoder) Decode(r io.Reader) (image.Image, error)        { return jpeg.Decode(r) }
func (jpegDecoder) Sniff(h []byte) bool {
	return len(h) >= 3 && h[0] == 0xFF && h[1] == 0xD8 && h[2] == 0xFF
}

// MemoryFactor for JPEG.
//
// image/jpeg allocates a YCbCr image, which is 1.5 bytes per pixel at 4:2:0 and
// 3 at 4:4:4, plus the block scratch and, for a progressive file, one int32
// coefficient plane per component, which is 12 bytes per pixel and the real
// risk in this codec. Six is the honest reservation for a progressive worst
// case; a baseline 4:2:0 file will use a quarter of it and give the rest back
// immediately.
func (jpegDecoder) MemoryFactor() float64 { return 6 }

type pngDecoder struct{}

func (pngDecoder) DecodeConfig(r io.Reader) (image.Config, error) { return png.DecodeConfig(r) }
func (pngDecoder) Decode(r io.Reader) (image.Image, error)        { return png.Decode(r) }
func (pngDecoder) Sniff(h []byte) bool {
	return len(h) >= 8 && string(h[:8]) == "\x89PNG\r\n\x1a\n"
}

// MemoryFactor for PNG.
//
// image/png allocates the destination image — up to 8 bytes per pixel for
// 16 bit RGBA — plus two row buffers. Nine covers the widest pixel format with
// room for the scratch.
func (pngDecoder) MemoryFactor() float64 { return 9 }

// probeInfo is what the encoded bytes of a picture reveal about it before it
// is decoded.
type probeInfo struct {
	// W and H are the *oriented* dimensions, because that is what
	// [Metadata] is defined to carry and what the gallery lays out with.
	// See [Orientation.SwapsAxes].
	W, H int
	// StoredW and StoredH are the dimensions of the stored pixel grid, which
	// is what the decoder will allocate and therefore what the decode budget
	// is computed from.
	StoredW, StoredH int
	Orientation      Orientation
	MIME             string
	Dec              Decoder
}

// probeHeader answers dimensions, media type and orientation from the encoded
// bytes.
//
// It takes the whole encoded picture, which the pipeline has in memory anyway
// and has already bounded against [Config.MaxEncodedBytes]. An earlier version
// took a buffered head and a reader for the rest, with a fallback that
// consumed the stream when the head was too small; nothing has called it that
// way since the input became a byte slice, and WU-R removed the unused half
// rather than leave a second, untested path through this function.
func probeHeader(raw []byte, mimeType string) (probeInfo, error) {
	dec, mt, ok := decoderFor(raw, mimeType)
	if !ok {
		return probeInfo{}, ErrNotAPicture
	}
	info := probeInfo{MIME: mt, Dec: dec}
	cfg, err := dec.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return info, fmt.Errorf("%w: %v", ErrNotAPicture, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return info, ErrNotAPicture
	}
	if mt == MIMEJPEG {
		info.Orientation = exifOrientation(raw)
	}
	info.StoredW, info.StoredH = cfg.Width, cfg.Height
	info.W, info.H = info.Orientation.Oriented(cfg.Width, cfg.Height)
	return info, nil
}
