package asset

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"

	"github.com/worldiety/gift/cms"
)

// EmbedProfile returns an encoded JPEG or PNG with p embedded, so that other
// programs read its numbers in the colour space they are in. It decodes
// nothing: the profile is spliced into the bytes an encoder wrote, such as
// those of image/jpeg or image/png.
//
// A JPEG gets the profile in APP2 segments after its JFIF and EXIF
// segments, a PNG in an iCCP chunk after its header. Whatever colour the
// file declared before is removed, because it would contradict the
// profile: the ICC segments of a JPEG, and the cICP, iCCP, sRGB, gAMA and
// cHRM chunks of a PNG, of which cICP would even take precedence.
//
// The profile must fit the pixels: gray for a gray picture, RGB for a colour
// one, and for a JPEG CMYK for CMYK; an RGB profile on a gray picture is
// allowed, as [ReadProfile] reads it. Anything else, and any format but
// JPEG and PNG, is an error.
func EmbedProfile(encoded []byte, p *cms.Profile) ([]byte, error) {
	if p == nil {
		return nil, errors.New("asset: EmbedProfile without a profile")
	}
	switch {
	case len(encoded) >= 3 && encoded[0] == 0xFF && encoded[1] == 0xD8 && encoded[2] == 0xFF:
		return embedJPEG(encoded, p)
	case len(encoded) >= 8 && string(encoded[:8]) == pngSignature:
		return embedPNG(encoded, p)
	}
	return nil, errors.New("asset: EmbedProfile writes into JPEG and PNG only")
}

// maxICCPart is what one APP2 segment holds of a profile: a segment is at
// most 65535 bytes including its length, the marker and the part numbers.
const maxICCPart = 65535 - 2 - len(iccMarker) - 2

func embedJPEG(b []byte, p *cms.Profile) ([]byte, error) {
	icc := p.Bytes()
	parts := (len(icc) + maxICCPart - 1) / maxICCPart
	if parts > 255 {
		return nil, errors.New("asset: profile too large for a JPEG")
	}
	d, _ := jpegColour(b)
	if !(declared{profile: p, stored: d.stored}).fits() {
		return nil, fmt.Errorf("asset: a %s profile for a JPEG of %s", p.Space(), d.stored)
	}

	// Copy the segments up to the first scan, leaving out ICC parts and
	// noting where the leading JFIF and EXIF segments end. Fill bytes and
	// markers without a length between segments are carried over as they
	// are.
	out := make([]byte, 0, len(b)+len(icc)+18*parts)
	at, prev := -1, 2
	out = append(out, b[:2]...)
	stop := jpegSegments(b, func(m byte, start, end int) bool {
		out = append(out, b[prev:start]...)
		prev = end
		seg := b[start+4 : end]
		if m == 0xE2 && len(seg) >= 14 && string(seg[:12]) == iccMarker {
			return true
		}
		if at < 0 && m != 0xE0 && m != 0xE1 {
			at = len(out)
		}
		out = append(out, b[start:end]...)
		return true
	})
	out = append(out, b[prev:stop]...)
	if at < 0 {
		at = len(out)
	}
	rest := b[stop:]

	var app2 []byte
	for k := range parts {
		chunk := icc[k*maxICCPart : min(len(icc), (k+1)*maxICCPart)]
		app2 = append(app2, 0xFF, 0xE2)
		app2 = binary.BigEndian.AppendUint16(app2, uint16(2+len(iccMarker)+2+len(chunk)))
		app2 = append(app2, iccMarker...)
		app2 = append(app2, byte(k+1), byte(parts))
		app2 = append(app2, chunk...)
	}
	res := make([]byte, 0, len(out)+len(app2)+len(rest))
	res = append(res, out[:at]...)
	res = append(res, app2...)
	res = append(res, out[at:]...)
	return append(res, rest...), nil
}

func embedPNG(b []byte, p *cms.Profile) ([]byte, error) {
	if p.Space() == cms.SpaceCMYK {
		return nil, errors.New("asset: a PNG cannot hold CMYK")
	}
	d, _ := pngColour(b)
	if !(declared{profile: p, stored: d.stored}).fits() {
		return nil, fmt.Errorf("asset: a %s profile for a PNG of %s", p.Space(), d.stored)
	}
	var z bytes.Buffer
	w, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	_, _ = w.Write(p.Bytes())
	if err := w.Close(); err != nil {
		return nil, err
	}
	name := profileName(p.Description())
	data := make([]byte, 0, len(name)+2+z.Len())
	data = append(data, name...)
	data = append(data, 0, 0)
	data = append(data, z.Bytes()...)

	out := make([]byte, 0, len(b)+len(data)+12)
	out = append(out, b[:8]...)
	for i := 8; i+12 <= len(b); {
		n := binary.BigEndian.Uint32(b[i:])
		if uint64(n) > uint64(len(b)-i-12) {
			return nil, errors.New("asset: PNG chunk runs past the end")
		}
		end := i + 12 + int(n)
		switch string(b[i+4 : i+8]) {
		case "cICP", "iCCP", "sRGB", "gAMA", "cHRM":
		case "IHDR":
			out = append(out, b[i:end]...)
			out = appendChunk(out, "iCCP", data)
		default:
			out = append(out, b[i:end]...)
		}
		i = end
	}
	return out, nil
}

func appendChunk(out []byte, typ string, data []byte) []byte {
	out = binary.BigEndian.AppendUint32(out, uint32(len(data)))
	start := len(out)
	out = append(out, typ...)
	out = append(out, data...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[start:]))
}

// profileName makes the name of an iCCP chunk from the description of a
// profile: printable Latin-1 of 1 to 79 bytes, without leading, trailing
// or doubled spaces.
func profileName(desc string) string {
	var b strings.Builder
	space := true
	for _, r := range desc {
		switch {
		case r == ' ':
			if space {
				continue
			}
			space = true
		case r > ' ' && r <= '~' || r >= 0xA1 && r <= 0xFF:
			space = false
		default:
			continue
		}
		if r > 0x7F {
			b.WriteByte(byte(r))
		} else {
			b.WriteRune(r)
		}
		if b.Len() >= 79 {
			break
		}
	}
	s := strings.TrimRight(b.String(), " ")
	if s == "" {
		return "ICC Profile"
	}
	return s
}
