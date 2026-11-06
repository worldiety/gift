package ebiten

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// virtualFS mimics the file system Ebitengine hands out for a drop
// (internal/file/file_notjs.go of v2.10.5): folders it makes up to hold paths
// from several directories have a FileInfo without Sys, every dropped entry
// carries the FileInfo of os.Stat and an AbsPath, and a folder whose listing
// fails at one entry returns the entries before it, nil after it, and the
// error. Ebitengine's own type is internal and cannot be built from here.
type virtualFS map[string][]fs.DirEntry

func (v virtualFS) Open(string) (fs.File, error) {
	return nil, errors.New("gift must not open a dropped file")
}

func (v virtualFS) ReadDir(name string) ([]fs.DirEntry, error) {
	ents := v[name]
	for _, e := range ents {
		if e == nil {
			return ents, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
		}
	}
	return ents, nil
}

type synthInfo struct{ name string }

func (s synthInfo) Name() string       { return s.name }
func (synthInfo) Size() int64          { return 0 }
func (synthInfo) Mode() fs.FileMode    { return 0555 | fs.ModeDir }
func (synthInfo) ModTime() time.Time   { return time.Time{} }
func (synthInfo) IsDir() bool          { return true }
func (synthInfo) Sys() any             { return nil }
func (s synthInfo) String() string     { return s.name }
func synthDir(name string) fs.DirEntry { return absEntry{fs.FileInfoToDirEntry(synthInfo{name}), ""} }

type absEntry struct {
	fs.DirEntry
	abs string
}

func (a absEntry) AbsPath() string { return a.abs }

func given(t *testing.T, p string) fs.DirEntry {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return absEntry{fs.FileInfoToDirEntry(fi), p}
}

// dropRecorder is a window-wide file target that records what it got.
type dropRecorder struct{ got *[]gift.DropInfo }

var dropRecorderType = gift.RegisterType("ebiten.test.DropRecorder")

func (dropRecorder) ViewType() gift.TypeID { return dropRecorderType }

func (r dropRecorder) Build(*gift.BuildContext) gift.Element {
	n := &dropRecorderNode{r}
	return gift.Element{Layouter: n, DropTarget: n}
}

type dropRecorderNode struct{ r dropRecorder }

func (n *dropRecorderNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(c.Max)
}

func (n *dropRecorderNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	info, _ := ctx.Drop()
	if e.Kind == gift.EventDrop || e.Kind == gift.EventDragEnter || e.Kind == gift.EventDragLeave {
		*n.r.got = append(*n.r.got, gift.DropInfo{Kinds: info.Kinds, Data: info.Data})
	}
	return true
}

func newDropBridge(t *testing.T) (*inputBridge, *[]gift.DropInfo) {
	t.Helper()
	got := &[]gift.DropInfo{}
	app := gift.New(gift.Options{Root: func(*gift.Context) gift.View { return dropRecorder{got: got} }})
	if err := app.Update(geom.Sz(400, 300)); err != nil {
		t.Fatal(err)
	}
	app.Paint()
	b := newInputBridge(app)
	b.cursor = func() (float64, float64) { return 20, 40 }
	b.dropped = func() fs.FS { return nil }
	return b, got
}

func drops(got []gift.DropInfo) (out []gift.DropData) {
	for _, g := range got {
		if len(g.Data.Items) > 0 {
			out = append(out, g.Data)
		}
	}
	return out
}

func TestEbitengineDroppedFilesBecomePaths(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	a := filepath.Join(dirA, "a b.txt")
	folder := filepath.Join(dirA, "Folder")
	c := filepath.Join(dirB, "c.txt")
	for _, f := range []string{a, c} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two folders made up by Ebitengine, because the paths come from two
	// directories; in the second one a path that cannot be stated, after
	// which Ebitengine lists nothing more.
	fsys := virtualFS{
		".": {synthDir("A"), synthDir("B")},
		"A": {given(t, a), given(t, folder)},
		"B": {given(t, c), nil, nil},
	}
	b, got := newDropBridge(t)
	delivered := false
	b.dropped = func() fs.FS {
		if delivered {
			return nil
		}
		delivered = true
		return fsys
	}
	b.density = 2
	b.app.BeginInput(0)
	b.pollDrops()

	d := drops(*got)
	if len(d) != 1 {
		t.Fatalf("delivered %d drops, want 1: %+v", len(d), *got)
	}
	items := d[0].Items
	if len(items) != 3 {
		t.Fatalf("items %+v, want a, the folder and c", items)
	}
	if items[0].Path != a || items[0].Dir || items[1].Path != folder || !items[1].Dir || items[2].Path != c {
		t.Errorf("items %+v", items)
	}
	// Nothing on the next tick: Ebitengine's report lasts one.
	*got = (*got)[:0]
	b.app.BeginInput(0)
	b.pollDrops()
	if len(*got) != 0 {
		t.Errorf("a second tick delivered %+v", *got)
	}
}

// fakeNative is a native layer driven by the test.
type fakeNative struct {
	q dropQueue
}

func (f *fakeNative) drain(dst []nativeDropEvent) []nativeDropEvent { return f.q.drain(dst) }
func (f *fakeNative) answer(a, h bool)                              { f.q.answer(a, h) }
func (f *fakeNative) close()                                        {}

func TestANativeLayerIsTheOneSourceOfTruth(t *testing.T) {
	b, got := newDropBridge(t)
	n := &fakeNative{}
	b.native = n
	b.density = 2

	n.q.push(nativeDropEvent{op: nativeOver, x: 100, y: 100, kinds: gift.DropFile})
	b.app.BeginInput(0)
	b.pollDrops()
	if !b.app.DropAccepted() {
		t.Fatal("the native over did not open a session")
	}
	b.native.answer(b.app.DropAccepted(), b.app.HasDropTargets())
	if !n.q.wouldAccept() {
		t.Error("the layer would tell the system the drag is refused")
	}

	n.q.push(nativeDropEvent{op: nativeDrop, x: 100, y: 100, data: gift.DropFiles("/n/a")})
	b.app.BeginInput(0)
	b.pollDrops()
	d := drops(*got)
	if len(d) != 1 || d[0].Items[0].Path != "/n/a" {
		t.Fatalf("native drop delivered %+v", d)
	}
	// Ebitengine reports the same drop a tick later: discarded.
	b.dropped = func() fs.FS { return virtualFS{".": {}} }
	b.app.BeginInput(0)
	b.pollDrops()
	b.app.BeginInput(0)
	b.pollDrops()
	if d := drops(*got); len(d) != 1 {
		t.Errorf("Ebitengine's duplicate was delivered: %+v", d)
	}
}

func TestTheDropQueueCollapses(t *testing.T) {
	var q dropQueue
	q.push(nativeDropEvent{op: nativeOver, x: 1})
	q.push(nativeDropEvent{op: nativeOver, x: 2})
	q.push(nativeDropEvent{op: nativeLeave})
	q.push(nativeDropEvent{op: nativeLeave})
	evs := q.drain(nil)
	if len(evs) != 2 || evs[0].x != 2 || evs[1].op != nativeLeave {
		t.Errorf("drained %+v", evs)
	}
}

func TestAQuietPollDropsDoesNotAllocate(t *testing.T) {
	b, _ := newDropBridge(t)
	b.native = &fakeNative{}
	b.dropEvs = make([]nativeDropEvent, 0, 8)
	if got := testing.AllocsPerRun(200, func() {
		b.app.BeginInput(0)
		b.pollDrops()
	}); got != 0 {
		t.Errorf("pollDrops allocates %v times in a tick without a drop", got)
	}
}

func TestEbitenginesReportWaitsForTheNativeDrop(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys := virtualFS{".": {given(t, a)}}

	// Ebitengine's report first, the native one a tick later: one drop.
	b, got := newDropBridge(t)
	n := &fakeNative{}
	b.native = n
	once := func() func() fs.FS {
		done := false
		return func() fs.FS {
			if done {
				return nil
			}
			done = true
			return fsys
		}
	}
	b.dropped = once()
	b.app.BeginInput(0)
	b.pollDrops()
	n.q.push(nativeDropEvent{op: nativeDrop, x: 10, y: 10, data: gift.DropFiles(a)})
	for range 4 {
		b.app.BeginInput(0)
		b.pollDrops()
	}
	if d := drops(*got); len(d) != 1 {
		t.Errorf("delivered %d drops, want the native one only", len(d))
	}

	// No native report at all, as from a source that only knows
	// WM_DROPFILES: Ebitengine's arrives after the grace period.
	b, got = newDropBridge(t)
	b.native = &fakeNative{}
	b.dropped = once()
	for range nativeDropGrace + 2 {
		b.app.BeginInput(0)
		b.pollDrops()
	}
	if d := drops(*got); len(d) != 1 || d[0].Items[0].Path != a {
		t.Errorf("Ebitengine's report alone delivered %+v", d)
	}
}

func TestTheAnswerBelongsToTheSession(t *testing.T) {
	var q dropQueue
	q.answer(false, true) // the end of a previous drag: nothing accepts
	q.open()
	if !q.wouldAccept() {
		t.Error("a new session got the previous session's answer, not whether the window has targets")
	}
	q.push(nativeDropEvent{op: nativeOver})
	q.drain(nil)
	q.answer(false, true)
	if q.wouldAccept() {
		t.Error("the answer of the session itself was not used")
	}
}

func TestADragIsLiveWithoutTheMouseToo(t *testing.T) {
	b, got := newDropBridge(t)
	n := &fakeNative{}
	b.native, b.noMouse = n, true
	g := &game{app: b.app, input: b}
	n.q.open()
	n.q.push(nativeDropEvent{op: nativeOver, x: 10, y: 10, kinds: gift.DropFile})
	b.app.BeginInput(0)
	b.pollDrops()
	g.answerDrops()
	if !b.app.DropAccepted() || !n.q.wouldAccept() {
		t.Fatal("under NoMouse the drag over the window was not reported, so the system is told it is refused")
	}
	n.q.push(nativeDropEvent{op: nativeDrop, x: 10, y: 10, data: gift.DropFiles("/a")})
	b.app.BeginInput(0)
	b.pollDrops()
	if d := drops(*got); len(d) != 1 {
		t.Errorf("delivered %+v", d)
	}
}
