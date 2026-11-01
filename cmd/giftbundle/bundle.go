package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// This file builds the bundle on disk. It has no build tag, so that its
// tests run on every machine; only the signing, the registration and the exec
// are macOS.

// bundleSpec is what goes into Info.plist.
type bundleSpec struct {
	ID, Name, Icon string

	// Exe is the name of the binary inside Contents/MacOS.
	Exe string
}

// fill derives what was not given from the name of the binary the go command
// built: the package's directory name for go run, "<package>.test" for go
// test.
func (s *bundleSpec) fill(binary string) {
	base := strings.TrimSuffix(binary, filepath.Ext(binary))
	if strings.HasSuffix(binary, ".test") {
		base = binary
	}
	s.Exe = sanitize(base)
	if s.Name == "" {
		s.Name = base
	}
	if s.ID == "" {
		s.ID = "dev.gift." + s.Exe
	}
}

// sanitize keeps what a bundle identifier allows: letters, digits, '-' and
// '.'. Everything else becomes '-'.
func sanitize(s string) string {
	b := []byte(s)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.') {
			b[i] = '-'
		}
	}
	if len(b) == 0 {
		return "app"
	}
	return string(b)
}

func (s *bundleSpec) validate() error {
	if s.ID == "" || sanitize(s.ID) != s.ID || strings.Contains(s.ID, "..") ||
		strings.HasPrefix(s.ID, ".") || strings.HasSuffix(s.ID, ".") {
		return fmt.Errorf("%q is not a bundle identifier: use letters, digits, '-' and '.', as in com.example.Photos", s.ID)
	}
	if s.Icon != "" && filepath.Ext(s.Icon) != ".icns" {
		return fmt.Errorf("the icon %s is not an .icns file", s.Icon)
	}
	return nil
}

// write lays the bundle out under dir and returns the path of the binary in
// it. The binary is copied through a temporary file and renamed into place,
// so that an instance of the previous build that still runs keeps its file.
func (s *bundleSpec) write(dir, binary string) (string, error) {
	macos := filepath.Join(dir, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		return "", err
	}
	exe := filepath.Join(macos, s.Exe)
	if err := copyFile(binary, exe, 0o755); err != nil {
		return "", err
	}
	if s.Icon != "" {
		res := filepath.Join(dir, "Contents", "Resources")
		if err := os.MkdirAll(res, 0o755); err != nil {
			return "", err
		}
		if err := copyFile(s.Icon, filepath.Join(res, "AppIcon.icns"), 0o644); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "Contents", "Info.plist"), []byte(s.plist()), 0o644); err != nil {
		return "", err
	}
	return exe, nil
}

func copyFile(from, to string, mode os.FileMode) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(to), ".giftbundle-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), to)
}

// plist is the Info.plist of the bundle. CFBundleIdentifier is what
// UNUserNotificationCenter needs; CFBundleExecutable is what LaunchServices
// starts when a notification of a program that has exited is clicked;
// NSHighResolutionCapable keeps the window at the display's resolution.
func (s *bundleSpec) plist() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	entry := func(k, v string) {
		fmt.Fprintf(&b, "\t<key>%s</key>\n\t<string>%s</string>\n", k, xmlEscape(v))
	}
	entry("CFBundleIdentifier", s.ID)
	entry("CFBundleName", s.Name)
	entry("CFBundleDisplayName", s.Name)
	entry("CFBundleExecutable", s.Exe)
	entry("CFBundlePackageType", "APPL")
	entry("CFBundleVersion", "1")
	entry("CFBundleShortVersionString", "0.0.0")
	if s.Icon != "" {
		entry("CFBundleIconFile", "AppIcon")
	}
	b.WriteString("\t<key>NSHighResolutionCapable</key>\n\t<true/>\n</dict>\n</plist>\n")
	return b.String()
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

func xmlEscape(s string) string { return xmlEscaper.Replace(s) }
