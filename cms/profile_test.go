package cms

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// FuzzParseProfile feeds broken profiles to the parser and to transforms
// in both directions. A profile comes out of a file anyone may have made;
// nothing it says may crash the program or make it allocate without
// bounds.
//
//	go test ./cms -run XXX -fuzz FuzzParseProfile -fuzztime 1m
func FuzzParseProfile(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "profiles", "*.icc"))
	for _, name := range files {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		start := time.Now()
		defer func() {
			if d := time.Since(start); d > 2*time.Second {
				t.Fatalf("%v for a profile of %d bytes", d, len(b))
			}
		}()
		p, err := ParseProfile(b)
		if err != nil {
			return
		}
		_ = p.Description()
		for _, intent := range []Intent{Perceptual, RelativeColorimetric, AbsoluteColorimetric} {
			if tr, err := NewTransform(p, SRGB(), Options{Intent: intent, BlackPointCompensation: true}); err == nil {
				in := make([]byte, 64*p.Space().Channels())
				for i := range in {
					in[i] = byte(i * 37)
				}
				out := make([]byte, 64*3)
				if err := tr.Pixels(out, RGB8, in, format8(p.Space())); err != nil {
					t.Fatal(err)
				}
			}
			if tr, err := NewTransform(SRGB(), p, Options{Intent: intent, BlackPointCompensation: true}); err == nil {
				out := make([]byte, 8*p.Space().Channels())
				if err := tr.Pixels(out, format8(p.Space()), make([]byte, 8*3), RGB8); err != nil {
					t.Fatal(err)
				}
			}
		}
	})
}
