package qr

import (
	"errors"
	"image/color"
	"os"
	"strings"
	"testing"
)

// The tables and the placement were compared module by module with
// rsc.io/qr for every version and level, at the capacity of each, and for
// texts of all three modes at every mask; these tests keep what that showed.

func TestCapacities(t *testing.T) {
	for _, c := range []struct {
		mode    string
		level   Level
		version int
		max     int
	}{
		// From the capacity table of ISO/IEC 18004.
		{"byte", L, 1, 17}, {"byte", M, 1, 14}, {"byte", Q, 1, 11}, {"byte", H, 1, 7},
		{"numeric", L, 1, 41}, {"alnum", L, 1, 25}, {"alnum", H, 1, 10},
		{"byte", M, 10, 213}, {"byte", L, 40, 2953}, {"byte", H, 40, 1273},
		{"numeric", L, 40, 7089}, {"alnum", L, 40, 4296},
	} {
		ch := map[string]string{"byte": "a", "numeric": "7", "alnum": "A"}[c.mode]
		code, err := encode(strings.Repeat(ch, c.max), c.level, 0, false)
		if err != nil || code.version != c.version {
			t.Errorf("%d %s at %v: version %v, %v; want %d", c.max, c.mode, c.level, code, err, c.version)
			continue
		}
		code, err = encode(strings.Repeat(ch, c.max+1), c.level, 0, false)
		if c.version == 40 {
			if !errors.Is(err, ErrTooLong) {
				t.Errorf("%d %s at %v: %v, want ErrTooLong", c.max+1, c.mode, c.level, err)
			}
		} else if err != nil || code.version != c.version+1 {
			t.Errorf("%d %s at %v: version %d, want %d", c.max+1, c.mode, c.level, code.version, c.version+1)
		}
	}
}

func TestModes(t *testing.T) {
	for text, mode := range map[string]int{
		"0123": modeNumeric, "WDJB-MJHT": modeAlphanumeric, "HTTPS://EXAMPLE.COM/A": modeAlphanumeric,
		"https://example.com": modeBytes, "Grüße": modeBytes, "": modeBytes, "12a": modeBytes,
	} {
		if got := segmentOf(text).mode; got != mode {
			t.Errorf("%q: mode %d, want %d", text, got, mode)
		}
	}
}

func TestEncodeRaisesTheLevelWithinTheVersion(t *testing.T) {
	c, err := Encode("hi", L)
	if err != nil || c.Version() != 1 || c.Level() != H {
		t.Fatalf("hi at L: version %d level %v, %v", c.Version(), c.Level(), err)
	}
	c, _ = Encode(strings.Repeat("a", 17), L)
	if c.Version() != 1 || c.Level() != L {
		t.Fatalf("a full version 1 at L: version %d level %v", c.Version(), c.Level())
	}
}

// formatOf reads both copies of the format information back.
func formatOf(c *Code) (first, second int) {
	n := c.size
	bit := func(x, y int) int {
		if c.Dark(x, y) {
			return 1
		}
		return 0
	}
	for i := range 6 {
		first |= bit(8, i) << i
	}
	first |= bit(8, 7)<<6 | bit(8, 8)<<7 | bit(7, 8)<<8
	for i := 9; i < 15; i++ {
		first |= bit(14-i, 8) << i
	}
	for i := range 8 {
		second |= bit(n-1-i, 8) << i
	}
	for i := 8; i < 15; i++ {
		second |= bit(8, n-15+i) << i
	}
	return first ^ 0x5412, second ^ 0x5412
}

func TestStructure(t *testing.T) {
	for _, text := range []string{"https://example.com/device?code=WDJB-MJHT", strings.Repeat("x", 300)} {
		c, err := Encode(text, M)
		if err != nil {
			t.Fatal(err)
		}
		n := c.Size()
		if n != c.Version()*4+17 {
			t.Fatalf("size %d for version %d", n, c.Version())
		}
		// The finders: a dark ring, a light ring and a dark centre of 3×3,
		// in three corners, each with a light separator.
		for _, o := range [][2]int{{0, 0}, {n - 7, 0}, {0, n - 7}} {
			for y := -1; y <= 7; y++ {
				for x := -1; x <= 7; x++ {
					d := max(abs(x-3), abs(y-3))
					want := d == 3 || d <= 1
					if got := c.Dark(o[0]+x, o[1]+y); got != want && d <= 4 {
						t.Fatalf("finder at %v: module %d,%d is %v", o, x, y, got)
					}
				}
			}
		}
		for i := 8; i < n-8; i++ {
			if c.Dark(i, 6) != (i%2 == 0) || c.Dark(6, i) != (i%2 == 0) {
				t.Fatalf("timing pattern broken at %d", i)
			}
		}
		if !c.Dark(8, n-8) {
			t.Fatal("the dark module is light")
		}
		first, second := formatOf(c)
		if first != second {
			t.Fatalf("format copies differ: %015b, %015b", first, second)
		}
		if lv, mask := first>>13, first>>10&7; lv != [4]int{1, 0, 3, 2}[c.Level()] || mask != c.mask {
			t.Fatalf("format says level bits %d mask %d, code has %v and %d", lv, mask, c.Level(), c.mask)
		}
	}
}

func TestInvalidLevelPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	_, _ = Encode("x", Level(4))
}

func TestImage(t *testing.T) {
	c, _ := Encode("WDJB-MJHT", M)
	im := c.Image(3)
	side := (c.Size() + 2*QuietZone) * 3
	if b := im.Bounds(); b.Dx() != side || b.Dy() != side {
		t.Fatalf("image %v, want %d", b, side)
	}
	if im.GrayAt(0, 0) != (color.Gray{Y: 0xff}) {
		t.Fatal("quiet zone is not white")
	}
	q := QuietZone * 3
	if im.GrayAt(q+1, q+1) != (color.Gray{}) || im.GrayAt(q+4, q+4) != (color.Gray{Y: 0xff}) {
		t.Fatal("finder corner is not black with a white ring")
	}
}

// TestCodesStayTheSame pins the codes of a few texts, masks included, so
// that a change to the encoder shows.
//
//	GIFT_UPDATE_GOLDEN=1 go test -run TestCodesStayTheSame ./ui/qr
func TestCodesStayTheSame(t *testing.T) {
	var sb strings.Builder
	for _, text := range []string{"WDJB-MJHT", "https://worldiety.github.io/gift/", "0123456789"} {
		c, err := Encode(text, M)
		if err != nil {
			t.Fatal(err)
		}
		sb.WriteString(text + "\n" + strings.ReplaceAll(c.String(), "██", "#") + "\n")
	}
	got := strings.ReplaceAll(sb.String(), "  ", ".")
	const file = "testdata/codes.txt"
	if os.Getenv("GIFT_UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("codes changed:\n%s", got)
	}
}
