//go:build (darwin && arm64 && !ios) || (linux && (amd64 || arm64))

package embedded

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/worldiety/gift/typst"
)

// cache points the user's cache directory into the test's own directory.
func cache(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, ".cache"))
}

func TestTheLibraryIsUnpackedOnceAndAgainWhenItIsIncomplete(t *testing.T) {
	cache(t)
	path, err := unpack()
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() != size {
		t.Fatalf("unpacked %v, %v; want %d bytes", fi, err, size)
	}

	old := time.Now().Add(-time.Hour)
	os.Chtimes(path, old, old)
	if again, err := unpack(); err != nil || again != path {
		t.Fatalf("second unpack = %q, %v; want %q", again, err, path)
	}
	if fi, _ := os.Stat(path); !fi.ModTime().Equal(old) {
		t.Errorf("a complete library was written again")
	}

	os.Truncate(path, 1000)
	if _, err := unpack(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Size() != size {
		t.Errorf("an incomplete library was kept: %d bytes", fi.Size())
	}
}

// TestTheEmbeddedLibrarySetsAPage loads the library this package carries,
// not one built in typst/ffi, and sets a page with it.
func TestTheEmbeddedLibrarySetsAPage(t *testing.T) {
	cache(t)
	t.Setenv("GIFT_TYPST_LIB", "")
	e, err := typst.New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	t.Logf("loaded %s", typst.Version())
	data, err := os.ReadFile(filepath.Join("..", "..", "font", "inter", "Inter-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AddFont(data); err != nil {
		t.Fatal(err)
	}
	e.SetSource("#set text(font: \"Inter\")\n#table(columns: 2, [a], [b])\n")
	d, err := e.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Pages) != 1 || len(d.Pages[0].Glyphs) != 2 {
		t.Fatalf("pages = %+v", d.Pages)
	}
}
