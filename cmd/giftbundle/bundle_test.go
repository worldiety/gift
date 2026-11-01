package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultsComeFromTheBinaryTheGoCommandBuilt(t *testing.T) {
	for _, tc := range []struct{ binary, id, name, exe string }{
		{"example-notify", "dev.gift.example-notify", "example-notify", "example-notify"},
		{"notify.test", "dev.gift.notify.test", "notify.test", "notify.test"},
		{"my app", "dev.gift.my-app", "my app", "my-app"},
	} {
		s := bundleSpec{}
		s.fill(tc.binary)
		if s.ID != tc.id || s.Name != tc.name || s.Exe != tc.exe {
			t.Errorf("%q: got %+v", tc.binary, s)
		}
		if err := s.validate(); err != nil {
			t.Errorf("%q: %v", tc.binary, err)
		}
	}
}

func TestValidateRefusesWhatIsNotABundleIdentifier(t *testing.T) {
	for _, id := range []string{"com/example", "com..example", ".com.example", "com.example.", "com example"} {
		s := bundleSpec{ID: id}
		s.fill("x")
		if err := s.validate(); err == nil {
			t.Errorf("%q was accepted", id)
		}
	}
	s := bundleSpec{ID: "com.example.X", Icon: "icon.png"}
	if err := s.validate(); err == nil {
		t.Error("a PNG icon was accepted")
	}
}

func TestWriteLaysOutABundle(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "prog")
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := bundleSpec{ID: "com.example.Photos", Name: "Photos & Co"}
	s.fill("prog")
	dir := filepath.Join(tmp, "Photos.app")
	exe, err := s.write(dir, bin)
	if err != nil {
		t.Fatal(err)
	}
	if exe != filepath.Join(dir, "Contents", "MacOS", "prog") {
		t.Errorf("binary at %s", exe)
	}
	if fi, err := os.Stat(exe); err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("binary not executable: %v %v", fi, err)
	}
	plist, err := os.ReadFile(filepath.Join(dir, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<key>CFBundleIdentifier</key>\n\t<string>com.example.Photos</string>",
		"<key>CFBundleName</key>\n\t<string>Photos &amp; Co</string>",
		"<key>CFBundleExecutable</key>\n\t<string>prog</string>",
		"<key>CFBundlePackageType</key>\n\t<string>APPL</string>",
		"<key>NSHighResolutionCapable</key>\n\t<true/>",
	} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, plist)
		}
	}
	// A second build replaces the binary in place.
	os.WriteFile(bin, []byte("binary 2"), 0o755)
	if _, err := s.write(dir, bin); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "binary 2" {
		t.Errorf("the second build left %q", b)
	}
}
