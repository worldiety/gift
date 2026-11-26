package ebiten

import (
	"image"
	"testing"

	"github.com/worldiety/gift/geom"
)

// TestIMEReplacement: every commit of exp/textinput, applied as the caret
// relative replacement of App.ReplaceText, gives the document the commit
// describes.
func TestIMEReplacement(t *testing.T) {
	cases := []struct {
		name                          string
		before, after, prefix, suffix string
		text                          string
		wantDB, wantDA                int
		wantInsert                    string
	}{
		{"insert at the caret", "Hal", "lo", "Hal", "lo", "x", 0, 0, "x"},
		{"backspace", "Hallo", "", "Hall", "", "", 1, 0, ""},
		{"backspace of an umlaut", "Grü", "", "Gr", "", "", 2, 0, ""},
		{"autocorrection of the word before", "Ich bin hier tesr", "", "Ich bin hier ", "", "test ", 4, 0, "test "},
		{"replacement across the caret", "ab", "cd", "a", "d", "X", 1, 1, "X"},
		{"forward delete", "ab", "cd", "ab", "d", "", 0, 1, ""},
		{"region after the caret", "ab", "cdef", "abcd", "f", "X", 0, 3, "cdX"},
		{"region before the caret", "abcdef", "", "a", "def", "X", 5, 0, "Xdef"},
	}
	for _, c := range cases {
		db, da, ins := imeReplacement(c.before, c.after, c.prefix, c.suffix, c.text)
		if db != c.wantDB || da != c.wantDA || ins != c.wantInsert {
			t.Errorf("%s: got (%d, %d, %q), want (%d, %d, %q)", c.name, db, da, ins, c.wantDB, c.wantDA, c.wantInsert)
			continue
		}
		// Apply it and compare with the document the commit describes.
		doc := c.before[:len(c.before)-db] + ins + c.after[da:]
		if want := c.prefix + c.text + c.suffix; doc != want {
			t.Errorf("%s: document %q, want %q", c.name, doc, want)
		}
	}
}

// TestScaleRectRoundsOutwards: the caret handed to the platform covers at
// least the pixels gift paints it on.
func TestScaleRectRoundsOutwards(t *testing.T) {
	got := scaleRect(geom.Rc(10.2, 20.5, 11.2, 40.1), 3)
	want := image.Rect(30, 61, 34, 121)
	if got != want {
		t.Fatalf("scaleRect = %v, want %v", got, want)
	}
}
