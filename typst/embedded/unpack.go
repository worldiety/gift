//go:build (darwin && arm64 && !ios) || (linux && (amd64 || arm64))

package embedded

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/worldiety/gift/typst"
)

func init() { typst.Provide(unpack) }

// unpack writes the library into the user's cache directory, unless an
// earlier run did, and returns its path. The directory is named after the
// library's checksum, so that two builds of gift never load each other's
// library. The file is written under a temporary name and renamed when it is
// complete, so that a program stopped while writing, or two writing at once,
// never leave a partial library under the real name.
func unpack() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "gift", "typst", sum[:16])
	path := filepath.Join(dir, name)
	if fi, err := os.Stat(path); err == nil && fi.Size() == size {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("typst/embedded: %w", err)
	}
	f, err := os.CreateTemp(dir, name+".*")
	if err != nil {
		return "", fmt.Errorf("typst/embedded: %w", err)
	}
	defer os.Remove(f.Name()) // fails harmlessly once renamed
	err = write(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		return "", fmt.Errorf("typst/embedded: %w", err)
	}
	return path, nil
}

// write decompresses the library into f and checks it against its checksum.
func write(f *os.File) error {
	z, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), z)
	if err != nil {
		return err
	}
	if n != size || hex.EncodeToString(h.Sum(nil)) != sum {
		return errors.New("the embedded library does not match its checksum")
	}
	return f.Chmod(0o755)
}
