package cms

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// oracle is testdata/littlecms.json, written by testdata/littlecms.py.
type oracle struct {
	LCMS     string            `json:"lcms"`
	Profiles map[string]string `json:"profiles"`
	Cases    []struct {
		Src    string `json:"src"`
		Dst    string `json:"dst"`
		Intent Intent `json:"intent"`
		BPC    bool   `json:"bpc"`
		// Optimized is how far LittleCMS's own baked tables are from its
		// exact pipeline, in 8 bit steps.
		Optimized int    `json:"optimized"`
		Out       string `json:"out"`
	} `json:"cases"`
}

// The colours of littlecms.py, per space.
var (
	oracleRGB  = []byte{0, 51, 102, 153, 204, 255}
	oracleCMYK = []byte{0, 85, 170, 255}
)

func oraclePixels(s Space) ([]byte, Format) {
	var b []byte
	switch s {
	case SpaceRGB:
		for _, r := range oracleRGB {
			for _, g := range oracleRGB {
				for _, bl := range oracleRGB {
					b = append(b, r, g, bl)
				}
			}
		}
		return b, RGB8
	case SpaceCMYK:
		for _, c := range oracleCMYK {
			for _, m := range oracleCMYK {
				for _, y := range oracleCMYK {
					for _, k := range oracleCMYK {
						b = append(b, c, m, y, k)
					}
				}
			}
		}
		return b, CMYK8
	}
	for v := 0; v < 256; v += 5 {
		b = append(b, byte(v))
	}
	return b, Gray8
}

func format8(s Space) Format {
	switch s {
	case SpaceCMYK:
		return CMYK8
	case SpaceGray:
		return Gray8
	}
	return RGB8
}

// oracleProfile loads a profile of the oracle, or reports why a local one
// cannot be used here.
func oracleProfile(name string, sums map[string]string) (*Profile, string, error) {
	var path string
	switch {
	case strings.HasPrefix(name, "system:"):
		path = strings.TrimPrefix(name, "system:")
	case strings.HasPrefix(name, "modcache:"):
		dir := os.Getenv("GOMODCACHE")
		if dir == "" {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, "go", "pkg", "mod")
		}
		path = filepath.Join(dir, strings.TrimPrefix(name, "modcache:"))
	default:
		path = filepath.Join("testdata", "profiles", name+".icc")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if strings.Contains(name, ":") {
			return nil, "not here", nil
		}
		return nil, "", err
	}
	if want, ok := sums[name]; ok {
		if got := sha256.Sum256(b); hex.EncodeToString(got[:]) != want {
			return nil, "a different file here", nil
		}
	}
	p, err := ParseProfile(b)
	return p, "", err
}

// TestAgainstLittleCMS converts colours as LittleCMS does: exactly within
// one 8 bit step, which is rounding, and with the baked tables no further
// from LittleCMS's exact pipeline than its own optimised tables are, or
// two steps where those are closer.
func TestAgainstLittleCMS(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "littlecms.json"))
	if err != nil {
		t.Fatal(err)
	}
	var o oracle
	if err := json.Unmarshal(raw, &o); err != nil {
		t.Fatal(err)
	}
	skipped := map[string]string{}
	for _, c := range o.Cases {
		name := fmt.Sprintf("%s to %s, %v, bpc %v", c.Src, c.Dst, c.Intent, c.BPC)
		src, why, err := oracleProfile(c.Src, o.Profiles)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		dst, why2, err := oracleProfile(c.Dst, o.Profiles)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if src == nil || dst == nil {
			skipped[c.Src+c.Dst] = why + why2
			continue
		}
		want, _ := base64.StdEncoding.DecodeString(c.Out)
		tr, err := NewTransform(src, dst, Options{Intent: c.Intent, BlackPointCompensation: c.BPC})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		in, sf := oraclePixels(src.Space())
		df := format8(dst.Space())
		got := make([]byte, len(want))
		if err := tr.Pixels(got, df, in, sf); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if d, at := maxDiff(got, want); d > max(2, c.Optimized) {
			t.Errorf("%s: baked differs by %d at byte %d (got %d, LittleCMS %d)", name, d, at, got[at], want[at])
		}
		exact := make([]byte, 0, len(want))
		n := sf.Size()
		for p := 0; p+n <= len(in); p += n {
			v := make([]float64, n)
			for i := range v {
				v[i] = float64(in[p+i]) / 255
			}
			for _, r := range tr.Color(v...) {
				exact = append(exact, byte(math.Round(min(max(r, 0), 1)*255)))
			}
		}
		if d, at := maxDiff(exact, want); d > 1 {
			t.Errorf("%s: exact differs by %d at byte %d (got %d, LittleCMS %d)", name, d, at, exact[at], want[at])
		}
	}
	if len(skipped) > 0 {
		t.Logf("%d pairs of local profiles not here", len(skipped))
	}
}

func maxDiff(a, b []byte) (d, at int) {
	for i := range a {
		if x := int(a[i]) - int(b[i]); x > d || -x > d {
			d, at = max(x, -x), i
		}
	}
	return d, at
}
