package asset

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"time"

	"github.com/worldiety/gift/geo"
)

// Exif is what gift reads from the EXIF of a photograph besides its
// orientation: where and when it was taken.
type Exif struct {
	// Location is where the photograph was taken, from the GPS IFD, and
	// HasLocation whether it says so at all.
	Location    geo.Coordinate
	HasLocation bool
	// Taken is DateTimeOriginal, or DateTime when that is missing, in the
	// local time of the camera, which EXIF does not name; the zero time
	// when neither is there.
	Taken time.Time
}

// exifWindow is how much of a file ReadExif reads: the APP1 segment of a
// JPEG is at most 64 KiB and comes before the picture data, and the raw file
// of a camera has its EXIF in its first few dozen kilobytes.
const exifWindow = 128 << 10

// ReadExif reads the location and time of a photograph from the head of
// src, without decoding it: a JPEG, or a TIFF, as the raw files of cameras
// are. A file without EXIF, or of another format, yields the zero Exif and
// no error; an error is a failure to read.
//
// It is the function a map of photographs builds its points with; see the
// project plan, section 37. A catalogue of thousands calls it from a few
// goroutines once and keeps the result, as it would any index.
func ReadExif(ctx context.Context, src Source) (Exif, error) {
	r, err := src.Open(ctx)
	if err != nil {
		return Exif{}, err
	}
	defer r.Close()
	head := make([]byte, exifWindow)
	n, err := io.ReadFull(r, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return Exif{}, err
	}
	return parseExif(head[:n]), nil
}

// parseExif finds the APP1 segment of a JPEG and reads the GPS IFD and the
// EXIF sub-IFD that IFD0 points to. A TIFF is its own EXIF.
func parseExif(b []byte) Exif {
	tiff := app1(b)
	if tiff == nil {
		tiff = b
	}
	t, ok := newTiff(tiff)
	if !ok {
		return Exif{}
	}
	var out Exif
	ifd0, _ := t.ifd(t.first)
	var gps, sub uint32
	for _, e := range ifd0 {
		switch e.tag {
		case 0x8825:
			gps = e.u32(t)
		case 0x8769:
			sub = e.u32(t)
		case 0x0132:
			if out.Taken.IsZero() {
				out.Taken = exifTime(e.ascii(t))
			}
		}
	}
	if sub != 0 {
		es, _ := t.ifd(sub)
		for _, e := range es {
			if e.tag == 0x9003 {
				if tm := exifTime(e.ascii(t)); !tm.IsZero() {
					out.Taken = tm
				}
			}
		}
	}
	if gps != 0 {
		es, _ := t.ifd(gps)
		var latRef, lonRef string
		var lat, lon float64
		var haveLat, haveLon bool
		for _, e := range es {
			switch e.tag {
			case 1:
				latRef = e.ascii(t)
			case 2:
				lat, haveLat = e.degrees(t)
			case 3:
				lonRef = e.ascii(t)
			case 4:
				lon, haveLon = e.degrees(t)
			}
		}
		if latRef == "S" {
			lat = -lat
		}
		if lonRef == "W" {
			lon = -lon
		}
		c := geo.LatLon(lat, lon)
		// 0, 0 is what a camera without a fix writes, not a photograph
		// taken in the Gulf of Guinea.
		if haveLat && haveLon && c.Valid() && (lat != 0 || lon != 0) {
			out.Location, out.HasLocation = c, true
		}
	}
	return out
}

// app1 returns the TIFF data of the first EXIF APP1 segment of a JPEG.
func app1(b []byte) []byte {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return nil
	}
	for i := 2; i+4 <= len(b); {
		if b[i] != 0xFF {
			return nil
		}
		marker := b[i+1]
		if marker == 0xFF {
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 {
			return nil
		}
		size := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		end := i + 2 + size
		if size < 2 || end > len(b) {
			return nil
		}
		seg := b[i+4 : end]
		if marker == 0xE1 && len(seg) > 6 && string(seg[:6]) == "Exif\x00\x00" {
			return seg[6:]
		}
		i = end
	}
	return nil
}

// tiffData is a TIFF structure in a byte order.
type tiffData struct {
	b     []byte
	bo    binary.ByteOrder
	first uint32
}

type tiffEntry struct {
	tag, typ uint16
	count    uint32
	value    []byte // the four bytes of the value field
}

func newTiff(b []byte) (tiffData, bool) {
	if len(b) < 8 {
		return tiffData{}, false
	}
	var bo binary.ByteOrder
	switch {
	case b[0] == 'I' && b[1] == 'I':
		bo = binary.LittleEndian
	case b[0] == 'M' && b[1] == 'M':
		bo = binary.BigEndian
	default:
		return tiffData{}, false
	}
	if bo.Uint16(b[2:4]) != 42 {
		return tiffData{}, false
	}
	return tiffData{b: b, bo: bo, first: bo.Uint32(b[4:8])}, true
}

// ifd reads the entries of the directory at off.
func (t tiffData) ifd(off uint32) ([]tiffEntry, bool) {
	o := int(off)
	if o < 8 || o+2 > len(t.b) {
		return nil, false
	}
	n := int(t.bo.Uint16(t.b[o : o+2]))
	o += 2
	if n > 512 || o+12*n > len(t.b) {
		return nil, false
	}
	es := make([]tiffEntry, n)
	for i := range es {
		p := o + 12*i
		es[i] = tiffEntry{
			tag: t.bo.Uint16(t.b[p:]), typ: t.bo.Uint16(t.b[p+2:]),
			count: t.bo.Uint32(t.b[p+4:]), value: t.b[p+8 : p+12],
		}
	}
	return es, true
}

func (e tiffEntry) u32(t tiffData) uint32 {
	if e.typ == 3 {
		return uint32(t.bo.Uint16(e.value))
	}
	return t.bo.Uint32(e.value)
}

// data is the bytes of the entry's value, inline or at its offset.
func (e tiffEntry) data(t tiffData, size int) []byte {
	n := size * int(e.count)
	if n <= 4 {
		return e.value[:n]
	}
	o := int(t.bo.Uint32(e.value))
	if o < 0 || o+n > len(t.b) {
		return nil
	}
	return t.b[o : o+n]
}

func (e tiffEntry) ascii(t tiffData) string {
	if e.typ != 2 {
		return ""
	}
	d := e.data(t, 1)
	for len(d) > 0 && d[len(d)-1] == 0 {
		d = d[:len(d)-1]
	}
	return string(d)
}

// degrees reads three RATIONALs, degrees, minutes and seconds.
func (e tiffEntry) degrees(t tiffData) (float64, bool) {
	if e.typ != 5 || e.count != 3 {
		return 0, false
	}
	d := e.data(t, 8)
	if d == nil {
		return 0, false
	}
	var v [3]float64
	for i := range v {
		num, den := t.bo.Uint32(d[8*i:]), t.bo.Uint32(d[8*i+4:])
		if den == 0 {
			if num != 0 {
				return 0, false
			}
			continue
		}
		v[i] = float64(num) / float64(den)
	}
	deg := v[0] + v[1]/60 + v[2]/3600
	return deg, !math.IsNaN(deg)
}

// exifTime parses "2006:01:02 15:04:05".
func exifTime(s string) time.Time {
	tm, err := time.Parse("2006:01:02 15:04:05", s)
	if err != nil {
		return time.Time{}
	}
	return tm
}
