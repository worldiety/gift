package raw

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"slices"
	"testing"

	"github.com/worldiety/gift/cms"
)

// tiffWriter lays out a little endian TIFF: data and directories are
// appended, and a directory is written after those it points to.
type tiffWriter struct{ b []byte }

type tiffEntry struct {
	tag, typ uint16
	count    uint32
	data     []byte
}

func (w *tiffWriter) blob(p []byte) uint32 {
	if len(w.b)%2 == 1 {
		w.b = append(w.b, 0)
	}
	off := uint32(len(w.b))
	w.b = append(w.b, p...)
	return off
}

func (w *tiffWriter) dir(es ...tiffEntry) uint32 {
	slices.SortFunc(es, func(a, b tiffEntry) int { return int(a.tag) - int(b.tag) })
	vals := make([][4]byte, len(es))
	for i, e := range es {
		if len(e.data) <= 4 {
			copy(vals[i][:], e.data)
		} else {
			binary.LittleEndian.PutUint32(vals[i][:], w.blob(e.data))
		}
	}
	d := binary.LittleEndian.AppendUint16(nil, uint16(len(es)))
	for i, e := range es {
		d = binary.LittleEndian.AppendUint16(d, e.tag)
		d = binary.LittleEndian.AppendUint16(d, e.typ)
		d = binary.LittleEndian.AppendUint32(d, e.count)
		d = append(d, vals[i][:]...)
	}
	return w.blob(binary.LittleEndian.AppendUint32(d, 0))
}

func shorts(tag uint16, v ...uint16) tiffEntry {
	var d []byte
	for _, x := range v {
		d = binary.LittleEndian.AppendUint16(d, x)
	}
	return tiffEntry{tag, typeShort, uint32(len(v)), d}
}

func longs(tag uint16, v ...uint32) tiffEntry {
	var d []byte
	for _, x := range v {
		d = binary.LittleEndian.AppendUint32(d, x)
	}
	return tiffEntry{tag, typeLong, uint32(len(v)), d}
}

func ascii(tag uint16, s string) tiffEntry {
	return tiffEntry{tag, typeASCII, uint32(len(s) + 1), append([]byte(s), 0)}
}

func bytesOf(tag uint16, v ...byte) tiffEntry { return tiffEntry{tag, typeByte, uint32(len(v)), v} }

// srationals are numerator and denominator pairs.
func srationals(tag uint16, v ...int32) tiffEntry {
	var d []byte
	for _, x := range v {
		d = binary.LittleEndian.AppendUint32(d, uint32(x))
	}
	return tiffEntry{tag, typeSRational, uint32(len(v) / 2), d}
}

func rationals(tag uint16, v ...uint32) tiffEntry {
	e := longs(tag, v...)
	e.typ, e.count = typeRational, uint32(len(v)/2)
	return e
}

// buildDNG makes a DNG as converters write them: IFD 0 a small RGB
// picture with the colour tags, its SubIFDs the raw data, a 16 by 8 mosaic
// of 16 bits in a pattern of GRBG, and a JPEG preview in Adobe RGB.
func buildDNG(t testing.TB) []byte {
	w := &tiffWriter{b: []byte("II*\x00\x00\x00\x00\x00")}

	mosaic := make([]byte, 16*8*2)
	rawDir := w.dir(
		longs(0x00fe, 0), longs(0x0100, 16), longs(0x0101, 8), shorts(0x0102, 16),
		shorts(0x0103, 1), shorts(0x0106, 32803), shorts(0x0115, 1),
		longs(0x0111, w.blob(mosaic)), longs(0x0116, 8), longs(0x0117, uint32(len(mosaic))),
		shorts(0x828d, 2, 2), bytesOf(0x828e, 1, 0, 2, 1),
		shorts(0xc619, 2, 2), shorts(0xc61a, 64, 65, 66, 67), longs(0xc61d, 4095),
		longs(0xc68d, 1, 2, 7, 14), longs(0xc61f, 1, 1), longs(0xc620, 10, 4),
	)

	var jb bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for i := range img.Pix {
		img.Pix[i] = byte(i)
	}
	if err := jpeg.Encode(&jb, img, nil); err != nil {
		t.Fatal(err)
	}
	previewDir := w.dir(
		longs(0x00fe, 1), longs(0x0100, 32), longs(0x0101, 24), shorts(0x0102, 8, 8, 8),
		shorts(0x0103, 7), shorts(0x0106, 6), shorts(0x0115, 3),
		longs(0x0111, w.blob(jb.Bytes())), longs(0x0117, uint32(jb.Len())),
		longs(0xc71a, 3),
	)

	thumb := make([]byte, 4*3*3)
	for i := range thumb {
		thumb[i] = byte(10 * i)
	}
	ifd0 := w.dir(
		longs(0x00fe, 1), longs(0x0100, 4), longs(0x0101, 3), shorts(0x0102, 8, 8, 8),
		shorts(0x0103, 1), shorts(0x0106, 2), shorts(0x0115, 3), shorts(0x011c, 1),
		longs(0x0111, w.blob(thumb)), longs(0x0116, 3), longs(0x0117, uint32(len(thumb))),
		ascii(0x010f, "Testmaker"), ascii(0x0110, "T1"), shorts(0x0112, 6),
		longs(0x014a, rawDir, previewDir),
		bytesOf(0xc612, 1, 4, 0, 0), ascii(0xc614, "Testmaker T1"),
		srationals(0xc621, 1, 1, 0, 1, 0, 1, 0, 1, 1, 1, 0, 1, 0, 1, 0, 1, 1, 1),
		srationals(0xc622, 2, 1, 0, 1, 0, 1, 0, 1, 2, 1, 0, 1, 0, 1, 0, 1, 2, 1),
		shorts(0xc65a, 17), shorts(0xc65b, 21),
		rationals(0xc628, 1, 2, 1, 1, 4, 5),
		srationals(0xc62a, -1, 2),
	)
	binary.LittleEndian.PutUint32(w.b[4:], ifd0)
	return w.b
}

func TestADNGIsRead(t *testing.T) {
	b := buildDNG(t)
	f, err := Open(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != DNG || f.Make != "Testmaker" || f.Model != "T1" || f.Orientation != 6 || f.Sony != nil {
		t.Errorf("%v %q %q, orientation %d, Sony %v", f.Format, f.Make, f.Model, f.Orientation, f.Sony)
	}
	want := Mosaic{
		Width: 16, Height: 8, BitsPerSample: 16, Compression: Uncompressed, SamplesPerPixel: 1,
		// The default crop lies in the active area, from (2, 1) on.
		Crop: image.Rect(3, 2, 13, 6), CFA: "GRBG", Black: [4]float64{64, 65, 66, 67}, White: 4095,
	}
	if f.Mosaic != want {
		t.Errorf("mosaic\n%+v, want\n%+v", f.Mosaic, want)
	}
	if f.WhiteBalance != [3]float64{2, 1, 1.25} {
		t.Errorf("white balance %v", f.WhiteBalance)
	}
	c := f.DNG
	if c == nil || c.Version != [4]byte{1, 4, 0, 0} || c.UniqueCameraModel != "Testmaker T1" ||
		c.CalibrationIlluminant1 != 17 || c.CalibrationIlluminant2 != 21 || c.BaselineExposure != -0.5 ||
		!slices.Equal(c.ColorMatrix2, []float64{2, 0, 0, 0, 2, 0, 0, 0, 2}) || len(c.ColorMatrix1) != 9 ||
		c.ForwardMatrix1 != nil {
		t.Errorf("colour %+v", c)
	}

	if len(f.Previews) != 2 {
		t.Fatalf("%d previews", len(f.Previews))
	}
	jp, rgb := f.Previews[0], f.Previews[1]
	if !jp.JPEG || jp.Width != 32 || jp.Height != 24 || jp.Space != cms.AdobeRGB() {
		t.Errorf("JPEG preview %+v", jp)
	}
	if rgb.JPEG || rgb.Width != 4 || rgb.Height != 3 || rgb.Space != nil {
		t.Errorf("RGB preview %+v", rgb)
	}
	img, err := f.DecodePreview(rgb)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.At(1, 0).(color.NRGBA); got != (color.NRGBA{30, 40, 50, 255}) {
		t.Errorf("the second pixel is %v", got)
	}
	if img, err := f.DecodePreview(jp); err != nil || img.Bounds().Dx() != 32 {
		t.Errorf("%v, %v", img, err)
	}
	if p, err := (Decoder{}).ReadProfile(b); err != nil || p != cms.AdobeRGB() {
		t.Errorf("profile %v, %v", p, err)
	}
}
