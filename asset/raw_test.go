package asset_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/cms"
	"github.com/worldiety/gift/raw"
)

// rawFixture is an ARW of an Alpha 7 III with its large preview; see
// raw/fixtures_test.go.
func rawFixture(t *testing.T) []byte {
	t.Helper()
	asset.RegisterDecoder(raw.MIMEARW, raw.Decoder{Format: raw.ARW})
	b, err := os.ReadFile(filepath.Join("..", "raw", "testdata", "a7iii-preview.arw"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// tiffEntry finds the entry of tag in the directory at off of a little
// endian TIFF, and returns where it is.
func tiffEntry(t *testing.T, b []byte, off int, tag uint16) int {
	t.Helper()
	n := int(binary.LittleEndian.Uint16(b[off:]))
	for i := range n {
		p := off + 2 + 12*i
		if binary.LittleEndian.Uint16(b[p:]) == tag {
			return p
		}
	}
	t.Fatalf("no tag %#04x at %d", tag, off)
	return 0
}

// A raw file, with the decoder of package raw registered, is shown as the
// preview the camera rendered: oriented as the file says, in the colour
// space it declares.
func TestARawFileIsShownAsItsPreview(t *testing.T) {
	arw := rawFixture(t)
	img, st, _ := thumbIn(t, cms.SRGB(), "photo.arw", arw)
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if math.Abs(float64(w)/float64(h)-1616.0/1080) > 0.05 {
		t.Errorf("a thumbnail of %dx%d for a preview of 1616x1080", w, h)
	}
	if st.Converted != 0 {
		t.Errorf("an sRGB preview converted %d times", st.Converted)
	}
	if p, err := asset.ReadProfile(arw); err != nil || p != cms.SRGB() {
		t.Errorf("profile %v, %v", p, err)
	}

	// Turned as IFD 0 says.
	rot := bytes.Clone(arw)
	ifd0 := int(binary.LittleEndian.Uint32(rot[4:]))
	binary.LittleEndian.PutUint16(rot[tiffEntry(t, rot, ifd0, 0x0112)+8:], 6)
	img, _, _ = thumbIn(t, cms.SRGB(), "turned.arw", rot)
	if img.Bounds().Dx() >= img.Bounds().Dy() {
		t.Errorf("orientation 6 gave %v", img.Bounds())
	}

	// A camera set to Adobe RGB says so as DCF has it: the colour space
	// uncalibrated, the interoperability index R03.
	adobe := bytes.Clone(arw)
	exif := int(binary.LittleEndian.Uint32(adobe[tiffEntry(t, adobe, ifd0, 0x8769)+8:]))
	binary.LittleEndian.PutUint16(adobe[tiffEntry(t, adobe, exif, 0xa001)+8:], 0xffff)
	interop := int(binary.LittleEndian.Uint32(adobe[tiffEntry(t, adobe, exif, 0xa005)+8:]))
	copy(adobe[tiffEntry(t, adobe, interop, 0x0001)+8:], "R03\x00")
	if p, err := asset.ReadProfile(adobe); err != nil || p != cms.AdobeRGB() {
		t.Errorf("profile %v, %v", p, err)
	}
	if _, st, _ := thumbIn(t, cms.SRGB(), "adobe.arw", adobe); st.Converted != 1 {
		t.Errorf("an Adobe RGB preview converted %d times", st.Converted)
	}
}

// The EXIF of a raw file is read as that of a JPEG. The fixture says it was
// taken in Berlin.
func TestTheExifOfARawFileIsRead(t *testing.T) {
	path := writeFile(t, t.TempDir(), "photo.arw", rawFixture(t))
	x, err := asset.ReadExif(context.Background(), asset.File(path))
	if err != nil {
		t.Fatal(err)
	}
	lat, lon := 52+31/60.0+12/3600.0, 13+24/60.0+36/3600.0
	if !x.HasLocation || math.Abs(x.Location.Lat-lat) > 1e-6 || math.Abs(x.Location.Lon-lon) > 1e-6 {
		t.Errorf("location %v (%v), want %v, %v", x.Location, x.HasLocation, lat, lon)
	}
	if x.Taken.Year() != 2026 || x.Taken.Month() != 11 {
		t.Errorf("taken %v", x.Taken)
	}
}
