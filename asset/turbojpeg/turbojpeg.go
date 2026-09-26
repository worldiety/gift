// Package turbojpeg decodes JPEG through libjpeg-turbo's TurboJPEG API, loaded
// at run time with purego, and decodes it *scaled* when a thumbnail is all
// that is wanted.
//
// # Why
//
// A camera JPEG is 12 to 50 megapixels. image/jpeg decodes every one of them
// at full resolution, and the pipeline then throws almost all of them away to
// make a 256 pixel tile; on a Raspberry Pi that is the better part of a second
// per picture, and a photo booth gallery is dozens of pictures. libjpeg-turbo
// is several times faster at the same work, and it can do less work: the
// inverse DCT can produce each 8x8 block at 1/2, 1/4 or 1/8 of its size
// directly, so a 24 megapixel photograph asked for at 1024 pixels comes out
// of the decoder as 1.5 megapixels without the other 22.5 ever existing.
//
// # No cgo
//
// gift builds without cgo, and this package keeps it that way: the library is
// opened with dlopen through purego the first time it is needed. A system
// without it loses the speed and nothing else: [Available] reports false, the
// helpers return [ErrUnavailable], and [Decoder] falls back to image/jpeg.
//
// The library is libturbojpeg.so.0 on Linux — the package libturbojpeg0 on
// Debian and Raspberry Pi OS — and libturbojpeg.0.dylib on macOS, where
// Homebrew's jpeg-turbo installs it under /opt/homebrew/lib (Apple silicon) or
// /usr/local/lib (Intel). Only the TurboJPEG 2.x API is used, because
// Raspberry Pi OS Bookworm ships libjpeg-turbo 2.1; libjpeg-turbo 3 still
// exports every function of it.
//
// # Registration is explicit
//
// Importing the package changes nothing, and neither does [Register]: it only
// loads the library and says whether that worked. Handing the decoder to gift
// is the application's own line,
//
//	if turbojpeg.Register() {
//		asset.RegisterDecoder(asset.MIMEJPEG, turbojpeg.Decoder{})
//	}
//
// which is the same shape as registering any other format. gift does not
// prefer the library on its own when it happens to be installed, for three
// reasons. The project plan, section 12, step 4, wants every decoder beyond
// the built in ones to be a visible, registered choice. The two decoders do
// not produce bit identical pixels, and a program whose thumbnails change
// with the packages installed on the machine it runs on is harder to test
// than one that chose. And a dlopen of a system library is a side effect
// that a toolkit should not perform behind an application's back.
//
// # Orientation
//
// The decoder returns the *stored* pixel grid and ignores EXIF, exactly like
// image/jpeg: the pipeline reads the orientation itself and applies it after
// scaling, which is cheaper by the square of the scale factor. Callers of
// [Decode] and [DecodeScaled] outside the pipeline have to do the same.
package turbojpeg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
)

// ErrUnavailable reports that libjpeg-turbo could not be loaded on this
// system, or is too old to have the TurboJPEG 2 functions.
var ErrUnavailable = errors.New("turbojpeg: libturbojpeg is not available")

// ErrUnsupported reports a JPEG that libjpeg-turbo reads but cannot convert to
// RGBA: CMYK and YCCK pictures, which TurboJPEG only decodes to CMYK. [Decoder]
// answers such a picture with image/jpeg instead.
var ErrUnsupported = errors.New("turbojpeg: CMYK and YCCK JPEGs cannot be decoded to RGBA")

// Available reports whether libjpeg-turbo was found and bound. The library is
// loaded on the first call to anything in this package that needs it, once,
// and the answer does not change afterwards.
func Available() bool { return load() == nil }

// Register loads libjpeg-turbo and reports whether it is usable.
//
// It registers nothing, anywhere: see the package documentation for the one
// line with which an application hands [Decoder] to gift, and for why gift
// does not do that on its own.
func Register() bool { return Available() }

// Decode decodes a whole JPEG at full resolution.
//
// The result is an *image.RGBA with alpha 0xFF everywhere. A JPEG has no
// alpha, so RGBA and NRGBA hold the same bytes, and RGBA is the type
// image/draw and x/image/draw have their fast paths for: drawing or scaling
// from it is a copy loop, not a call through the color.Color interface per
// pixel.
//
// Without the library it returns [ErrUnavailable]; for a CMYK picture
// [ErrUnsupported]. [Decoder.Decode] falls back to image/jpeg in both cases.
func Decode(data []byte) (*image.RGBA, error) {
	return DecodeScaled(data, 0, 0)
}

// DecodeScaled decodes a JPEG at the smallest of the scale factors 1/8, 1/4,
// 1/2 and 1/1 whose result is still at least minW by minH, so that nothing is
// lost that a following resampler would have used. The result is never
// smaller than asked unless the picture itself is, and is usually larger: the
// factors are halvings, and the caller scales the rest of the way.
//
// minW and minH are in stored pixels, before any EXIF orientation. Zero or
// less for either asks for full resolution in that axis.
//
// The result type and the errors are the ones of [Decode].
func DecodeScaled(data []byte, minW, minH int) (*image.RGBA, error) {
	if err := load(); err != nil {
		return nil, err
	}
	return decompress(data, minW, minH)
}

// Decoder is the gift binding: an asset.Decoder, asset.Sniffer and
// asset.ScaledDecoder for image/jpeg. It is stateless and safe for concurrent
// use; every decode gets its own TurboJPEG handle.
//
// Unlike the helpers it never fails for want of the library. A picture
// libjpeg-turbo cannot decode to RGBA, or every picture on a system without
// it, is decoded by image/jpeg at full size — which [Decoder.DecodeScaled]
// is allowed to return, since it asks for a minimum — and the memory figures
// switch to image/jpeg's with it. Registering it unconditionally is
// therefore harmless, but pointless: see the package documentation.
type Decoder struct{}

// DecodeConfig reads the dimensions from the head of the stream.
//
// It is image/jpeg's, which parses the markers in Go and stops at the frame
// header; handing libjpeg-turbo the header would first need the whole stream
// in one piece, and probing is the one place where the pipeline passes a
// reader that has no Bytes method to take it from without a copy.
func (Decoder) DecodeConfig(r io.Reader) (image.Config, error) { return jpeg.DecodeConfig(r) }

// Decode decodes at full resolution. See [Decoder.DecodeScaled].
func (d Decoder) Decode(r io.Reader) (image.Image, error) { return d.DecodeScaled(r, 0, 0) }

// DecodeScaled is [DecodeScaled] with image/jpeg as the fallback.
//
// If r has a Bytes method — a *bytes.Buffer, which is what gift's pipeline
// passes — the encoded picture is taken from it without a copy; any other
// reader is read to the end first.
func (Decoder) DecodeScaled(r io.Reader, minW, minH int) (image.Image, error) {
	data, err := readAll(r)
	if err != nil {
		return nil, err
	}
	if load() == nil {
		img, err := decompress(data, minW, minH)
		if !errors.Is(err, ErrUnsupported) {
			return img, err
		}
	}
	return jpeg.Decode(bytes.NewReader(data))
}

// Sniff recognises JPEG by its SOI marker followed by the start of the next
// marker, exactly as gift's built in decoder does. It has to: registering
// this decoder replaces that one, and the pipeline selects decoders by
// sniffing first, so a Decoder that did not sniff would only ever be used for
// sources that declare their media type.
func (Decoder) Sniff(h []byte) bool {
	return len(h) >= 3 && h[0] == 0xFF && h[1] == 0xD8 && h[2] == 0xFF
}

// MemoryFactor is the working memory of a full size decode per pixel of the
// picture.
//
// Four bytes are the RGBA result. The risk is a progressive file, for which
// libjpeg keeps every DCT coefficient of the whole picture until the last
// scan: two bytes per sample, which is three bytes per pixel at 4:2:0 and six
// at 4:4:4. Ten covers the worst case; [Decoder.ScaledMemory] computes the
// real figure from the header and is what the pipeline uses when it can.
// Without the library it is image/jpeg's six.
func (Decoder) MemoryFactor() float64 {
	if load() != nil {
		return goMemoryFactor
	}
	return 10
}

// goMemoryFactor is gift's figure for image/jpeg; asset/decode.go explains
// it. It is repeated rather than imported because this package does not
// depend on gift's asset package, and it only has to be right, not shared.
const goMemoryFactor = 6

// ScaledMemory is the working memory, in bytes, that [Decoder.DecodeScaled]
// needs for the picture whose encoded bytes start with header and whose
// stored grid is w by h, asked for at least minW by minH.
//
// Scaling shrinks the result and the row buffers, but not everything: a
// progressive or multi-scan file still keeps every coefficient of the full
// size picture, because the later scans refine blocks that were decoded
// earlier, and that is where the risk is. The frame and first scan headers
// say which kind of file this is and how its colour is subsampled, so the
// figure is computed from them rather than assumed: a baseline 24 megapixel
// photograph thumbnailed at 1/8 needs about 3 MB, the same file saved
// progressive about 75 MB, and reserving the second for the first would
// serialise a gallery on its decode budget for nothing.
//
// A header this function cannot read is charged the progressive 4:4:4 worst
// case, and a picture that will go to image/jpeg — no library, or CMYK — is
// charged image/jpeg's full size figure.
func (Decoder) ScaledMemory(header []byte, w, h, minW, minH int) int64 {
	if load() != nil {
		return int64(float64(w) * float64(h) * goMemoryFactor)
	}
	return workingMemory(parseFrame(header), w, h, chooseScale(scales(), w, h, minW, minH))
}

// readAll takes the encoded picture from r, without a copy when r can hand
// out its bytes.
func readAll(r io.Reader) ([]byte, error) {
	if b, ok := r.(interface{ Bytes() []byte }); ok {
		return b.Bytes(), nil
	}
	return io.ReadAll(r)
}

// --- scale factors ----------------------------------------------------------

// scale is a TurboJPEG scaling factor, num/denom.
type scale struct{ num, denom int }

// of is TJSCALED: the scaled length, rounded up, which is what libjpeg-turbo
// actually produces.
func (s scale) of(n int) int { return (n*s.num + s.denom - 1) / s.denom }

// fallbackScales is the table libjpeg-turbo 2.x and 3.x return from
// tjGetScalingFactors, largest first. The library's own table is used when it
// is loaded; this one is for the tests, which must not need the library to
// check the choice.
var fallbackScales = []scale{
	{2, 1}, {15, 8}, {7, 4}, {13, 8}, {3, 2}, {11, 8}, {5, 4}, {9, 8},
	{1, 1}, {7, 8}, {3, 4}, {5, 8}, {1, 2}, {3, 8}, {1, 4}, {1, 8},
}

// chooseScale picks the smallest factor of at most 1/1 whose result is at
// least minW by minH, each clamped to the picture: asking for more than the
// picture has asks for all of it, not for an enlargement, which is the
// resampler's business if anybody's.
//
// Smallest is the whole point — every halving of the factor is a quarter of
// the IDCT, colour conversion and output work — and "at least" is the
// quality guarantee: the pipeline's resampler only ever scales down from
// here, so no detail that would have survived a full size decode is missing.
// 1/1 is always acceptable, so the answer exists even for a table without
// it.
//
// Only 1/2, 1/4 and 1/8 are considered below 1/1, although the library
// offers every eighth. The others go through libjpeg's generic k-by-k IDCT,
// which has no SIMD version, and measured slower than a full size decode:
// on an Apple M1 with a 12 megapixel photograph, 7/8 took 73 ms, 3/4 64 ms
// and 5/8 60 ms against 54 ms at 1/1, while 1/2 took 48 and 3/8 no less
// than 1/2. The power-of-two reductions have NEON and SSE2 kernels, the
// same ones a Raspberry Pi uses. Picking one of them over a nearer eighth
// costs at most a slightly larger intermediate for the resampler, which is
// cheap next to the IDCT.
func chooseScale(factors []scale, w, h, minW, minH int) scale {
	if minW <= 0 || minW > w {
		minW = w
	}
	if minH <= 0 || minH > h {
		minH = h
	}
	best := scale{1, 1}
	for _, f := range factors {
		if f.num != 1 || (f.denom != 1 && f.denom != 2 && f.denom != 4 && f.denom != 8) {
			continue
		}
		if f.of(w) < minW || f.of(h) < minH {
			continue
		}
		if f.num*best.denom < best.num*f.denom {
			best = f
		}
	}
	return best
}

// --- memory -----------------------------------------------------------------

// frame is what the frame header and the first scan header of a JPEG say
// about the decoder's memory.
type frame struct {
	ok          bool
	progressive bool
	// comps are the sampling factors of each component.
	comps [][2]int
	// firstScan is the number of components in the first scan. Fewer than
	// len(comps) makes a sequential file multi-scan, which costs libjpeg the
	// same whole-picture coefficient buffer a progressive one does.
	firstScan int
}

// parseFrame walks the markers up to the first SOS. It reads only what it
// needs and gives up — ok false — on anything it does not understand.
func parseFrame(b []byte) frame {
	var f frame
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return f
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return frame{}
		}
		m := b[i+1]
		if m == 0xFF {
			i++
			continue
		}
		if m == 0x01 || (m >= 0xD0 && m <= 0xD8) {
			i += 2
			continue
		}
		if m == 0xD9 {
			return frame{}
		}
		size := int(binary.BigEndian.Uint16(b[i+2:]))
		end := i + 2 + size
		if size < 2 || end > len(b) {
			return frame{}
		}
		seg := b[i+4 : end]
		switch {
		case m >= 0xC0 && m <= 0xCF && m != 0xC4 && m != 0xC8 && m != 0xCC:
			// SOFn. Precision, height, width, count, then three bytes
			// per component.
			if len(seg) < 6 {
				return frame{}
			}
			n := int(seg[5])
			if n == 0 || len(seg) < 6+3*n {
				return frame{}
			}
			f.progressive = m == 0xC2 || m == 0xC6 || m == 0xCA || m == 0xCE
			f.comps = make([][2]int, n)
			for c := range n {
				hv := seg[6+3*c+1]
				f.comps[c] = [2]int{max(1, int(hv>>4)), max(1, int(hv&0x0F))}
			}
		case m == 0xDA:
			if f.comps == nil || len(seg) < 1 {
				return frame{}
			}
			f.firstScan = int(seg[0])
			f.ok = true
			return f
		}
		i = end
	}
	return frame{}
}

// workingMemory is the byte figure behind [Decoder.ScaledMemory].
//
// It follows what libjpeg allocates: the output, the coefficient buffer —
// the whole picture's for a multi-scan file, one MCU row otherwise — and the
// sample rows between the IDCT, the upsampler and the colour converter. The
// row terms are generous rather than exact; they are small next to the other
// two for any picture this is worth calling for.
func workingMemory(f frame, w, h int, s scale) int64 {
	ow, oh := int64(s.of(w)), int64(s.of(h))
	out := 4 * ow * oh
	const fixed = 1 << 20 // decompressor structs, Huffman tables, the handle
	if !f.ok {
		// Unknown layout: progressive, 4:4:4, three components.
		return out + 6*int64(w)*int64(h) + fixed
	}
	if len(f.comps) == 4 {
		// CMYK or YCCK: image/jpeg decodes it, at full size.
		return int64(float64(w)*float64(h)*goMemoryFactor) + fixed
	}
	hmax, vmax := 1, 1
	for _, c := range f.comps {
		hmax, vmax = max(hmax, c[0]), max(vmax, c[1])
	}
	var coef, rows int64
	for _, c := range f.comps {
		// Blocks per component, rounded up to whole MCUs the way
		// jdcoefct.c sizes its virtual arrays.
		bw := roundUp(ceilDiv(ceilDiv(w*c[0], hmax), 8), c[0])
		bh := roundUp(ceilDiv(ceilDiv(h*c[1], vmax), 8), c[1])
		if f.progressive || f.firstScan < len(f.comps) {
			coef += int64(bw) * int64(bh) * 128
		} else {
			coef += int64(bw) * int64(c[1]) * 128
		}
		// Two MCU rows of scaled samples per component, for the IDCT
		// output and the context rows of fancy upsampling.
		rows += 2 * int64(s.of(bw*8)) * int64(s.of(8*vmax))
	}
	// Plus the upsampled full-width rows the colour converter reads.
	rows += int64(len(f.comps)) * ow * int64(s.of(8*vmax))
	return out + coef + rows + fixed
}

func ceilDiv(a, b int) int { return (a + b - 1) / b }
func roundUp(a, m int) int { return ceilDiv(a, m) * m }

// errorf is the one place errors of the library get their prefix.
func errorf(op, msg string) error {
	return fmt.Errorf("turbojpeg: %s: %s", op, msg)
}
