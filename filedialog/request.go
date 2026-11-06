package filedialog

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// request is a dialog as the platform layers see it: defaults applied, text
// valid UTF-8, the start directory checked.
type request struct {
	mode Mode
	// title is the title of the dialog, accept the label of its primary
	// button.
	title, accept string
	// start is an absolute, cleaned directory, or "" for the system's
	// choice.
	start string
	// exts are normalised ".jpg" extensions; empty in Directory mode.
	exts       []string
	filterName string
	// suggested is the name for Save mode, without a separator.
	suggested string
}

// result is the answer of a platform layer.
type result struct {
	paths     []string
	cancelled bool
	err       error
}

// request turns d into what a platform layer is handed, logging what it had
// to drop.
func (d Dialog) request() request {
	r := request{
		mode:       d.mode,
		title:      valid(d.title),
		filterName: valid(d.filterName),
	}
	if r.title == "" {
		r.title = defaultTitle(d.mode)
	}
	r.accept = acceptLabel(d.mode)
	if r.filterName == "" {
		r.filterName = "Supported files"
	}
	if d.mode != Directory {
		r.exts = d.exts
	}
	start := d.start
	if start == "" {
		start = d.root
	}
	if start != "" {
		if abs, err := filepath.Abs(start); err == nil {
			if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
				r.start = filepath.Clean(abs)
			}
		}
		if r.start == "" && d.log != nil {
			d.log.Debug("filedialog: the start directory is not a directory, the system chooses", "start", start)
		}
	}
	if d.mode == Save && d.suggested != "" {
		name := valid(d.suggested)
		if strings.ContainsAny(name, `/\`) {
			if d.log != nil {
				d.log.Warn("filedialog: a suggested name with a path separator is dropped", "name", d.suggested)
			}
			name = ""
		}
		r.suggested = name
	}
	return r
}

func valid(s string) string { return strings.ToValidUTF8(s, "�") }

func defaultTitle(m Mode) string {
	switch m {
	case Directory:
		return "Choose a folder"
	case Save:
		return "Save"
	}
	return "Open"
}

func acceptLabel(m Mode) string {
	switch m {
	case Directory:
		return "Choose"
	case Save:
		return "Save"
	}
	return "Open"
}

// normalizeExts turns ".JPG", "jpg" and "jpg " into ".jpg".
//
// It is a copy of normalizeExts in ui/filepicker/model.go, which is not
// exported, so that both pickers read Extensions the same way.
func normalizeExts(exts []string) []string {
	out := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		out = append(out, e)
	}
	return out
}

var errOutsideRoot = errors.New("the picked path is outside the root")

// relToRoot turns the absolute paths a dialog answered with into fs.FS paths
// relative to root. Symbolic links are resolved on both sides first, so that
// /var and /private/var on macOS are the same place; a path that does not
// exist, such as a Save target, through its nearest existing parent.
func relToRoot(root string, paths []string) ([]string, error) {
	rr, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	rr = resolve(rr)
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rel, err := filepath.Rel(rr, resolve(p))
		if err != nil {
			return nil, errOutsideRoot
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") || !fs.ValidPath(rel) {
			return nil, errOutsideRoot
		}
		out = append(out, rel)
	}
	return out, nil
}

// resolve resolves the symbolic links of the longest existing prefix of p.
func resolve(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for dir := p; ; {
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return p
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

// logf logs to l, which may be nil.
func logf(l *slog.Logger, level slog.Level, msg string, args ...any) {
	if l != nil {
		l.Log(context.Background(), level, "filedialog: "+msg, args...)
	}
}
