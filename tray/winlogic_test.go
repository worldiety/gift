package tray

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// The computations of the Windows half, which run everywhere; see
// winlogic.go.

func TestTheWindowsGUIDIsStableAndNameBased(t *testing.T) {
	u := winTrayGUID("com.example.Photos", 1)
	if u != winTrayGUID("com.example.Photos", 1) {
		t.Fatal("the GUID differs between two calls")
	}
	if u[6]>>4 != 5 {
		t.Errorf("version nibble %x, want 5", u[6]>>4)
	}
	if u[8]&0xc0 != 0x80 {
		t.Errorf("variant bits %02b, want 10", u[8]>>6)
	}
	// Pinned: a change would make every user's "always show" choice vanish.
	got := fmt.Sprintf("%x", u)
	if want := pinnedPhotosGUID; got != want {
		t.Errorf("GUID of com.example.Photos is %s, was %s", got, want)
	}
	seen := map[[16]byte]string{}
	for _, c := range []struct {
		id string
		n  int
	}{{"com.example.Photos", 1}, {"com.example.Photos", 2}, {"com.example.photos", 1}, {"", 1}, {"a#2", 1}, {"a", 2}, {"a", 1}} {
		g := winTrayGUID(c.id, c.n)
		if prev, ok := seen[g]; ok {
			t.Errorf("%s/%d has the GUID of %s", c.id, c.n, prev)
		}
		seen[g] = fmt.Sprint(c.id, "/", c.n)
		if g[6]>>4 != 5 || g[8]&0xc0 != 0x80 {
			t.Errorf("%s/%d: % x is no version 5 UUID of RFC 9562", c.id, c.n, g)
		}
	}
}

const pinnedPhotosGUID = "cb7e7fe543d35c5d922e1f12c7c60dad"

func TestTheTooltipIsCutWithoutSplittingASurrogatePair(t *testing.T) {
	short := "Photos"
	if u, cut := winCutUTF16(short, winTipUnits); cut || string(utf16.Decode(u)) != short {
		t.Fatalf("cut %v %q", cut, string(utf16.Decode(u)))
	}
	exact := strings.Repeat("a", winTipUnits)
	if u, cut := winCutUTF16(exact, winTipUnits); cut || len(u) != winTipUnits {
		t.Fatalf("127 units: cut %v, %d units", cut, len(u))
	}
	// 126 units and then an emoji, two units: the 127th would be its high
	// surrogate alone.
	pair := strings.Repeat("a", winTipUnits-1) + "😀b"
	u, cut := winCutUTF16(pair, winTipUnits)
	if !cut || len(u) != winTipUnits-1 {
		t.Fatalf("cut %v, %d units, want 126", cut, len(u))
	}
	// One unit earlier the pair fits whole.
	u, _ = winCutUTF16(strings.Repeat("a", winTipUnits-2)+"😀b", winTipUnits)
	if len(u) != winTipUnits || !utf16.IsSurrogate(rune(u[len(u)-1])) || u[len(u)-1] < 0xDC00 {
		t.Fatalf("%d units, last %04X, want the pair kept", len(u), u[len(u)-1])
	}
	// Non-BMP text all the way: never ends on a high surrogate.
	for n := 1; n < 10; n++ {
		u, _ := winCutUTF16(strings.Repeat("😀", 10), n)
		if len(u)%2 != 0 {
			t.Fatalf("limit %d: %d units, a pair is split", n, len(u))
		}
	}
}

func TestMenuTitlesKeepTheirAmpersands(t *testing.T) {
	for in, want := range map[string]string{
		"Import":        "Import",
		"Save & Quit":   "Save && Quit",
		"&&":            "&&&&",
		"Tab\there":     "Tab here",
		"Ünïcödé & co.": "Ünïcödé && co.",
	} {
		if got := winMenuText(in); got != want {
			t.Errorf("winMenuText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPixelsAreConvertedToBGRA(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	im.SetNRGBA(0, 0, color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff})
	im.SetNRGBA(1, 0, color.NRGBA{R: 0xff, G: 0x80, B: 0x00, A: 0x80})
	if got, want := winBGRA(im, false), []byte{0x30, 0x20, 0x10, 0xff, 0x00, 0x80, 0xff, 0x80}; string(got) != string(want) {
		t.Errorf("straight % x, want % x", got, want)
	}
	if got, want := winBGRA(im, true), []byte{0x30, 0x20, 0x10, 0xff, 0x00, 0x40, 0x80, 0x80}; string(got) != string(want) {
		t.Errorf("premultiplied % x, want % x", got, want)
	}
	if im.Pix[0] != 0x10 {
		t.Error("the image was changed")
	}
}

func TestTheANDMaskIsWordAlignedAndSetWhereTransparent(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 18, 2))
	im.SetNRGBA(0, 0, color.NRGBA{A: 0xff})
	im.SetNRGBA(9, 0, color.NRGBA{A: 0x80})
	im.SetNRGBA(17, 1, color.NRGBA{A: 0xff})
	m := winANDMask(im)
	// 18 pixels need 3 bytes, padded to 4.
	if len(m) != 8 {
		t.Fatalf("%d bytes, want 8", len(m))
	}
	want := []byte{0x7f, 0xbf, 0xc0, 0, 0xff, 0xff, 0x80, 0}
	if string(m) != string(want) {
		t.Errorf("mask % x, want % x", m, want)
	}
}

func TestColoursOfWindows(t *testing.T) {
	if got := winColorRef(0x00332211); got != (color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff}) {
		t.Errorf("COLORREF 0x00332211 is %v", got)
	}
	if winTaskbarInk(true) == winTaskbarInk(false) {
		t.Error("the same ink on a light and a dark taskbar")
	}
	if c := winTaskbarInk(false); c.R != 0xff || c.A != 0xff {
		t.Errorf("ink on a dark taskbar %v, want white", c)
	}
}

func TestAFailedAddIsRetriedSoonThenSeldom(t *testing.T) {
	var got []time.Duration
	d := time.Duration(0)
	for range 9 {
		d = winNextRetry(d)
		got = append(got, d)
	}
	want := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second,
		8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("delays %v, want %v", got, want)
	}
}
