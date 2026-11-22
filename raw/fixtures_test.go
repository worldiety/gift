package raw

import (
	"bytes"
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
