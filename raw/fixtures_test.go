package raw

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The fixtures in testdata are ARWs of an Alpha 7 III, taken for this
// package, cut short: a file ends before the raw data, and all but one
// before the large preview, so that six of them weigh about a megabyte
// instead of 170. Their metadata is the camera's, with two changes: the
// name of the owner is blanked, and the GPS position is moved to Berlin.
//
// TestWriteFixtures makes them anew from the camera's files:
//
//	GIFT_RAW_FIXTURES=../samples/a7iii go test -run WriteFixtures ./raw/
var fixtures = []struct {
	from, to string
	// preview keeps the large preview.
	preview bool
}{
	{"DSC04204.ARW", "a7iii-bw.arw", false},
	{"DSC04205.ARW", "a7iii-portrait.arw", false},
	{"DSC04206.ARW", "a7iii-portrait-shade.arw", false},
	{"DSC04207.ARW", "a7iii-landscape-iso16000.arw", false},
	{"DSC04267.ARW", "a7iii-preview.arw", true},
	{"DSC04271.ARW", "a7iii-uncompressed.arw", false},
}

// crops are fixtures with raw data: 256 by 256 photosites of the camera's
// file at (x, y), the directory of the raw data changed to say so, with a
// crop of the 240 by 240 inside. Sony's compressed raw is in blocks of 32
// columns, which stand alone, so x is a multiple of 32.
var crops = []struct {
	from, to string
	x, y     int
}{
	// The lamp in focus, in the light of a low sun.
	{"DSC04205.ARW", "a7iii-crop-compressed.arw", 1920, 1280},
	// The leaves of a plant against a lamp that clipped, uncompressed.
	{"DSC04271.ARW", "a7iii-crop-uncompressed.arw", 4160, 576},
}

const cropSize = 256

// berlin is the position the fixtures say they were taken at:
// 52° 31' 12" N, 13° 24' 36" E.
var berlin = [2][3][2]uint32{{{52, 1}, {31, 1}, {12000, 1000}}, {{13, 1}, {24, 1}, {36000, 1000}}}

func TestWriteFixtures(t *testing.T) {
	src := os.Getenv("GIFT_RAW_FIXTURES")
	if src == "" {
		t.Skip("set GIFT_RAW_FIXTURES to the directory of the camera's files")
	}
	for _, fx := range fixtures {
		b, err := os.ReadFile(filepath.Join(src, fx.from))
		if err != nil {
			t.Fatal(err)
		}
		orig, err := Open(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Fatal(fx.from, err)
		}
		cut := orig.segs[0].off
		large := orig.Previews[0].segs[0]
		if fx.preview {
			cut = min(cut, large.off+large.n)
		} else {
			cut = min(cut, large.off)
		}
		out := scrub(t, append([]byte(nil), b[:cut]...))

		// The metadata of the cut file is that of the camera's.
		got, err := Open(bytes.NewReader(out), int64(len(out)))
		if err != nil {
			t.Fatal(fx.to, err)
		}
		want := len(orig.Previews)
		if !fx.preview {
			want--
		}
		if !reflect.DeepEqual(got.Mosaic, orig.Mosaic) || !reflect.DeepEqual(got.Sony, orig.Sony) ||
			got.WhiteBalance != orig.WhiteBalance || len(got.Previews) != want {
			t.Fatalf("%s: the cut file reads otherwise:\n%+v\n%+v", fx.to, got, orig)
		}
		if err := os.WriteFile(filepath.Join("testdata", fx.to), out, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d bytes of %d", fx.to, len(out), len(b))
	}
	for _, c := range crops {
		writeCrop(t, src, c.from, c.to, c.x, c.y)
	}
}

// writeCrop writes the metadata of from, cut before the large preview, with
// the raw data of the crop at (x, y) after it.
func writeCrop(t *testing.T, src, from, to string, x0, y0 int) {
	b, err := os.ReadFile(filepath.Join(src, from))
	if err != nil {
		t.Fatal(err)
	}
	orig, err := Open(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	full, err := orig.ReadMosaic()
	if err != nil {
		t.Fatal(err)
	}
	w := orig.Mosaic.Width
	bpp := 1
	if orig.Mosaic.Compression == Uncompressed {
		bpp = 2
	}
	strip := orig.segs[0].off
	var data []byte
	for y := y0; y < y0+cropSize; y++ {
		o := strip + int64((y*w+x0)*bpp)
		data = append(data, b[o:o+int64(cropSize*bpp)]...)
	}
	out := scrub(t, append([]byte(nil), b[:orig.Previews[0].segs[0].off]...))
	for len(out)%16 != 0 {
		out = append(out, 0)
	}
	at := len(out)
	out = append(out, data...)

	bo, first, _ := header(out)
	ifd0 := dirEntries(out, bo, first)
	rawDir := int64(bo.Uint32(out[ifd0[0x014a]+8:]))
	e := dirEntries(out, bo, rawDir)
	bo.PutUint16(out[e[0x0100]+8:], cropSize)
	bo.PutUint16(out[e[0x0101]+8:], cropSize)
	bo.PutUint16(out[e[0x0116]+8:], cropSize)
	bo.PutUint32(out[e[0x0111]+8:], uint32(at))
	bo.PutUint32(out[e[0x0117]+8:], uint32(len(data)))
	for tag, v := range map[uint16][2]uint32{0xc61f: {8, 8}, 0xc620: {cropSize - 16, cropSize - 16}} {
		o := bo.Uint32(out[e[tag]+8:])
		bo.PutUint32(out[o:], v[0])
		bo.PutUint32(out[o+4:], v[1])
	}

	// The crop decodes to what the camera's file does there.
	got, err := Open(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	m, err := got.ReadMosaic()
	if err != nil {
		t.Fatal(err)
	}
	clipped := 0
	for y := range cropSize {
		for x := range cropSize {
			v := m[y*cropSize+x]
			if v != full[(y0+y)*w+x0+x] {
				t.Fatalf("%s: (%d, %d) decodes to %d, the camera's file to %d", to, x, y, v, full[(y0+y)*w+x0+x])
			}
			if float64(v) >= orig.Mosaic.White*clipFraction {
				clipped++
			}
		}
	}
	if err := os.WriteFile(filepath.Join("testdata", to), out, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d bytes, %d photosites clipped, sha256 %x", to, len(out), clipped, mosaicHash(m))
}

// dirEntries is where each entry of the directory at off begins.
func dirEntries(b []byte, bo binary.ByteOrder, off int64) map[uint16]int {
	n := int(bo.Uint16(b[off:]))
	e := make(map[uint16]int, n)
	for i := range n {
		p := int(off) + 2 + 12*i
		e[bo.Uint16(b[p:])] = p
	}
	return e
}

// scrub blanks the owner's name in every directory of the chain and puts
// the GPS position at Berlin.
func scrub(t *testing.T, b []byte) []byte {
	bo, first, _ := header(b)
	entries := func(off int64, f func(p int, tag, typ uint16, count uint32)) int64 {
		n := int(bo.Uint16(b[off:]))
		for i := range n {
			p := int(off) + 2 + 12*i
			f(p, bo.Uint16(b[p:]), bo.Uint16(b[p+2:]), bo.Uint32(b[p+4:]))
		}
		return int64(bo.Uint32(b[int(off)+2+12*n:]))
	}
	for off := first; off != 0; {
		off = entries(off, func(p int, tag, typ uint16, count uint32) {
			switch tag {
			case 0x013b: // Artist
				clear(valueBytes(b, bo, p, int(count)))
			case 0x8825: // GPS
				entries(int64(bo.Uint32(b[p+8:])), func(p int, tag, typ uint16, count uint32) {
					if (tag == 2 || tag == 4) && typ == typeRational && count == 3 {
						v := valueBytes(b, bo, p, 24)
						for i, r := range berlin[tag/2-1] {
							bo.PutUint32(v[8*i:], r[0])
							bo.PutUint32(v[8*i+4:], r[1])
						}
					}
				})
			}
		})
	}
	return b
}

// valueBytes is the value of the entry at p, of n bytes.
func valueBytes(b []byte, bo binary.ByteOrder, p, n int) []byte {
	if n <= 4 {
		return b[p+8 : p+8+n]
	}
	o := int(bo.Uint32(b[p+8:]))
	return b[o : o+n]
}

// mosaicHash is the SHA-256 of decoded photosites, little endian.
func mosaicHash(m []uint16) [32]byte {
	b := make([]byte, 2*len(m))
	for i, v := range m {
		binary.LittleEndian.PutUint16(b[2*i:], v)
	}
	return sha256.Sum256(b)
}
