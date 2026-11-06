package filedialog

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/worldiety/gift/ui"
)

// fakePlatform records what it is asked to show and lets the test answer.
type fakePlatform struct {
	mu     sync.Mutex
	reqs   []request
	dones  []func(result)
	closes int
	// final is what close reports.
	final bool
	// openErr is returned by open.
	openErr error
	opened  chan request
}

func newFake() *fakePlatform { return &fakePlatform{opened: make(chan request, 16)} }

func (f *fakePlatform) open(r request, done func(result)) (func() bool, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	f.dones = append(f.dones, done)
	err := f.openErr
	f.mu.Unlock()
	f.opened <- r
	if err != nil {
		return nil, err
	}
	return func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.closes++
		return f.final
	}, nil
}

func (f *fakePlatform) answer(t *testing.T, i int, res result) {
	t.Helper()
	f.mu.Lock()
	done := f.dones[i]
	f.mu.Unlock()
	done(res)
}

func (f *fakePlatform) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

// waitOpened waits for the platform to be asked to show a dialog.
func (f *fakePlatform) waitOpened(t *testing.T) request {
	t.Helper()
	select {
	case r := <-f.opened:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("the platform was not asked to open a dialog within 5 s")
	}
	return request{}
}

func (f *fakePlatform) notOpened(t *testing.T) {
	t.Helper()
	select {
	case <-f.opened:
		t.Fatal("the platform was asked to open a dialog")
	case <-time.After(50 * time.Millisecond):
	}
}

func noEnv(string) string { return "" }

// fakeOpener is an opener over f on a supported system.
func fakeOpener(f *fakePlatform) *opener {
	return newOpener(func(openConfig) (platform, error) { return f, nil }, func() bool { return true }, noEnv)
}

// unsupportedOpener is an opener on a system without a native dialog.
func unsupportedOpener() *opener {
	return newOpener(func(openConfig) (platform, error) {
		return nil, permanent(ErrUnsupported)
	}, func() bool { return false }, noEnv)
}

// mailbox is a post function whose closures the test runs, standing in for
// the UI executor.
type mailbox chan func()

func newMailbox() mailbox { return make(mailbox, 16) }

func (m mailbox) post(fn func()) { m <- fn }

// run runs the next posted closure.
func (m mailbox) run(t *testing.T) {
	t.Helper()
	select {
	case fn := <-m:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was posted within 5 s")
	}
}

// drain runs every closure posted within a short while.
func (m mailbox) drain() {
	for {
		select {
		case fn := <-m:
			fn()
		case <-time.After(50 * time.Millisecond):
			return
		}
	}
}

// outcome records what a dialog reported.
type outcome struct {
	picked    [][]string
	cancelled int
}

func (o *outcome) pick(paths []string) { o.picked = append(o.picked, paths) }
func (o *outcome) cancel()             { o.cancelled++ }

// records is a slog handler that keeps what was logged.
type records struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (r *records) Enabled(context.Context, slog.Level) bool { return true }
func (r *records) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	r.recs = append(r.recs, rec)
	r.mu.Unlock()
	return nil
}
func (r *records) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *records) WithGroup(string) slog.Handler      { return r }

func (r *records) count(level slog.Level) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, rec := range r.recs {
		if rec.Level == level {
			n++
		}
	}
	return n
}

func (r *records) has(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.recs {
		if strings.Contains(rec.Message, sub) {
			return true
		}
	}
	return false
}

func TestOpenDeliversThePicks(t *testing.T) {
	f := newFake()
	var o outcome
	mb := newMailbox()
	d := New(o.pick, o.cancel).Mode(OpenMulti)
	d.op = fakeOpener(f)
	if _, err := d.Open(mb.post); err != nil {
		t.Fatal(err)
	}
	f.waitOpened(t)
	f.answer(t, 0, result{paths: []string{"/a", "/b"}})
	mb.run(t)
	if len(o.picked) != 1 || !slices.Equal(o.picked[0], []string{"/a", "/b"}) || o.cancelled != 0 {
		t.Fatalf("got %+v", o)
	}
}

func TestOneModeKeepsTheFirstPath(t *testing.T) {
	f := newFake()
	var o outcome
	var logs records
	mb := newMailbox()
	d := New(o.pick, o.cancel).Logger(slog.New(&logs))
	d.op = fakeOpener(f)
	d.Open(mb.post)
	f.waitOpened(t)
	f.answer(t, 0, result{paths: []string{"/a", "/b"}})
	mb.run(t)
	if len(o.picked) != 1 || !slices.Equal(o.picked[0], []string{"/a"}) {
		t.Fatalf("got %+v", o)
	}
	if logs.count(slog.LevelWarn) != 1 {
		t.Fatalf("want one warning, got %d", logs.count(slog.LevelWarn))
	}
}

func TestAnswersThatAreCancels(t *testing.T) {
	for name, res := range map[string]result{
		"cancelled": {cancelled: true},
		"empty":     {},
		"error":     {err: errors.New("boom")},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			var o outcome
			mb := newMailbox()
			d := New(o.pick, o.cancel)
			d.op = fakeOpener(f)
			d.Open(mb.post)
			f.waitOpened(t)
			f.answer(t, 0, res)
			mb.run(t)
			if o.cancelled != 1 || len(o.picked) != 0 {
				t.Fatalf("got %+v", o)
			}
		})
	}
}

func TestOneDialogAtATime(t *testing.T) {
	f := newFake()
	op := fakeOpener(f)
	var o1, o2, o3 outcome
	mb := newMailbox()
	d1 := New(o1.pick, o1.cancel)
	d1.op = op
	d2 := New(o2.pick, o2.cancel)
	d2.op = op
	d3 := New(o3.pick, o3.cancel)
	d3.op = op
	if _, err := d1.Open(mb.post); err != nil {
		t.Fatal(err)
	}
	f.waitOpened(t)
	if _, err := d2.Open(mb.post); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Open: %v, want ErrBusy", err)
	}
	mb.run(t)
	if o2.cancelled != 1 {
		t.Fatalf("the busy dialog was not answered with cancel: %+v", o2)
	}
	f.answer(t, 0, result{paths: []string{"/x"}})
	mb.run(t)
	if _, err := d3.Open(mb.post); err != nil {
		t.Fatalf("Open after the answer: %v", err)
	}
	f.waitOpened(t)
}

func TestFinalCloseReleases(t *testing.T) {
	f := newFake()
	f.final = true
	op := fakeOpener(f)
	var o outcome
	mb := newMailbox()
	d := New(o.pick, o.cancel)
	d.op = op
	cancel, _ := d.Open(mb.post)
	f.waitOpened(t)
	cancel()
	cancel()
	if f.closeCount() != 1 {
		t.Fatalf("close ran %d times, want 1", f.closeCount())
	}
	if _, err := d.Open(mb.post); err != nil {
		t.Fatalf("Open after a final close: %v", err)
	}
	f.waitOpened(t)
	// A late answer of the first request frees nothing of the second.
	f.answer(t, 0, result{cancelled: true})
	if _, err := d.Open(mb.post); !errors.Is(err, ErrBusy) {
		t.Fatalf("a late answer freed the dialog of a newer request: %v", err)
	}
	mb.drain()
	if len(o.picked) != 0 || o.cancelled != 1 {
		t.Fatalf("got %+v, want the one busy cancel only", o)
	}
}

func TestCloseThatIsNotFinalWaitsForTheAnswer(t *testing.T) {
	f := newFake()
	op := fakeOpener(f)
	var o outcome
	mb := newMailbox()
	d := New(o.pick, o.cancel)
	d.op = op
	cancel, _ := d.Open(mb.post)
	f.waitOpened(t)
	cancel()
	if _, err := d.Open(mb.post); !errors.Is(err, ErrBusy) {
		t.Fatalf("reopening before the system answered the close: %v, want ErrBusy", err)
	}
	mb.run(t) // the busy cancel
	f.answer(t, 0, result{cancelled: true})
	mb.drain()
	if o.cancelled != 1 {
		t.Fatalf("the dropped answer was reported: %+v", o)
	}
	if _, err := d.Open(mb.post); err != nil {
		t.Fatalf("Open after the dropped answer: %v", err)
	}
	f.waitOpened(t)
}

func TestCancelBeforeTheDialogIsShown(t *testing.T) {
	f := newFake()
	gate := make(chan struct{})
	op := newOpener(func(openConfig) (platform, error) { <-gate; return f, nil }, func() bool { return true }, noEnv)
	var o outcome
	mb := newMailbox()
	d := New(o.pick, o.cancel)
	d.op = op
	start := time.Now()
	cancel, err := d.Open(mb.post)
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 50*time.Millisecond {
		t.Fatalf("Open took %v while the platform was opening", el)
	}
	cancel()
	close(gate)
	f.notOpened(t)
	mb.drain()
	if o.cancelled != 0 || len(o.picked) != 0 {
		t.Fatalf("a cancelled request reported %+v", o)
	}
	if _, err := d.Open(mb.post); err != nil {
		t.Fatalf("Open after a cancel before showing: %v", err)
	}
	f.waitOpened(t)
}

func TestCancelWhileTheDialogIsOpening(t *testing.T) {
	f := newFake()
	f.final = true
	op := fakeOpener(f)
	var o outcome
	mb := newMailbox()
	d := New(o.pick, o.cancel)
	d.op = op
	cancel, _ := d.Open(mb.post)
	f.waitOpened(t)
	cancel()
	// close is called whether cancel came before or after open returned.
	deadline := time.Now().Add(5 * time.Second)
	for f.closeCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("close was never called")
		}
		time.Sleep(time.Millisecond)
	}
	f.answer(t, 0, result{paths: []string{"/late"}})
	mb.drain()
	if len(o.picked) != 0 {
		t.Fatalf("a cancelled dialog reported a pick: %+v", o)
	}
}

func TestUnsupported(t *testing.T) {
	var o outcome
	var logs records
	mb := newMailbox()
	op := unsupportedOpener()
	quiet := New(o.pick, o.cancel)
	quiet.op = op
	if _, err := quiet.Open(mb.post); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Open: %v, want ErrUnsupported", err)
	}
	mb.run(t)
	loud := quiet.Logger(slog.New(&logs))
	loud.Open(mb.post)
	mb.run(t)
	loud.Open(mb.post)
	mb.run(t)
	if o.cancelled != 3 || len(o.picked) != 0 {
		t.Fatalf("got %+v", o)
	}
	if logs.count(slog.LevelError) != 1 || logs.count(slog.LevelDebug) != 1 {
		t.Fatalf("want one Error and one Debug record, got %d and %d", logs.count(slog.LevelError), logs.count(slog.LevelDebug))
	}
	if op.available() {
		t.Fatal("available on an unsupported system")
	}
}

func TestNilCancelOnUnsupportedReportsNothing(t *testing.T) {
	var o outcome
	mb := newMailbox()
	d := New(o.pick, nil)
	d.op = unsupportedOpener()
	d.Open(mb.post)
	mb.drain()
	if len(o.picked) != 0 {
		t.Fatalf("got %+v", o)
	}
}

func TestLastingFailureWhenOpening(t *testing.T) {
	op := newOpener(func(openConfig) (platform, error) {
		return nil, permanent(errors.New("no AppKit"))
	}, func() bool { return true }, noEnv)
	var o outcome
	var logs records
	mb := newMailbox()
	d := New(o.pick, o.cancel).Logger(slog.New(&logs))
	d.op = op
	if _, err := d.Open(mb.post); err != nil {
		t.Fatalf("Open: %v; a failure not yet known is reported asynchronously", err)
	}
	mb.run(t)
	if o.cancelled != 1 || op.available() {
		t.Fatalf("got %+v, available %v", o, op.available())
	}
	if _, err := d.Open(mb.post); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("second Open: %v, want ErrUnsupported", err)
	}
	mb.run(t)
	if logs.count(slog.LevelError) != 1 {
		t.Fatalf("want one Error record, got %d", logs.count(slog.LevelError))
	}
}

func TestPassingFailureKeepsTheSystemAvailable(t *testing.T) {
	f := newFake()
	f.openErr = errors.Join(ErrUnsupported, errors.New("no NSApplication yet"))
	op := fakeOpener(f)
	var o outcome
	mb := newMailbox()
	d := New(o.pick, o.cancel)
	d.op = op
	d.Open(mb.post)
	f.waitOpened(t)
	mb.run(t)
	if o.cancelled != 1 || !op.available() {
		t.Fatalf("got %+v, available %v", o, op.available())
	}
	f.mu.Lock()
	f.openErr = nil
	f.mu.Unlock()
	if _, err := d.Open(mb.post); err != nil {
		t.Fatalf("Open after a passing failure: %v", err)
	}
	f.waitOpened(t)
}

func TestOtherFailureIsACancel(t *testing.T) {
	f := newFake()
	f.openErr = errors.New("E_FAIL")
	var o outcome
	mb := newMailbox()
	d := New(o.pick, o.cancel)
	d.op = fakeOpener(f)
	d.Open(mb.post)
	f.waitOpened(t)
	mb.run(t)
	if o.cancelled != 1 || !d.op.available() {
		t.Fatalf("got %+v", o)
	}
}

func TestKioskIsUnsupported(t *testing.T) {
	f := newFake()
	op := newOpener(func(openConfig) (platform, error) { return f, nil }, func() bool { return true },
		func(k string) string {
			if k == kioskEnv {
				return "1"
			}
			return ""
		})
	if op.available() {
		t.Fatal("available in a kiosk")
	}
	var o outcome
	mb := newMailbox()
	d := New(o.pick, o.cancel)
	d.op = op
	if _, err := d.Open(mb.post); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Open: %v", err)
	}
	mb.run(t)
	f.notOpened(t)
}

func TestNilPost(t *testing.T) {
	f := newFake()
	var o outcome
	d := New(o.pick, o.cancel)
	d.op = fakeOpener(f)
	cancel, err := d.Open(nil)
	if !errors.Is(err, ErrUnsupported) || cancel == nil {
		t.Fatalf("Open(nil): %v, cancel nil %v", err, cancel == nil)
	}
	cancel()
	f.notOpened(t)
}

func TestPanics(t *testing.T) {
	for name, fn := range map[string]func(){
		"nil onPick": func() { New(nil, nil) },
		"mode":       func() { New(func([]string) {}, nil).Mode(Save + 1) },
		"height":     func() { Present(ui.ZStack(), New(func([]string) {}, nil)).FallbackHeight(0) },
		"content":    func() { Present(nil, New(func([]string) {}, nil)) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("no panic")
				}
			}()
			fn()
		})
	}
}

func TestNormalizeExtsLikeTheInAppPicker(t *testing.T) {
	got := normalizeExts([]string{"JPG", ".png ", "", " .Tar.GZ"})
	if want := []string{".jpg", ".png", ".tar.gz"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRequest(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	pick := func([]string) {}

	r := New(pick, nil).request()
	if r.title != "Open" || r.accept != "Open" || r.filterName != "Supported files" || r.start != "" {
		t.Fatalf("defaults: %+v", r)
	}
	if r := New(pick, nil).Mode(Directory).Extensions(".jpg").request(); r.title != "Choose a folder" || r.accept != "Choose" || len(r.exts) != 0 {
		t.Fatalf("directory: %+v", r)
	}
	if r := New(pick, nil).Mode(Save).request(); r.title != "Save" || r.accept != "Save" {
		t.Fatalf("save: %+v", r)
	}
	if r := New(pick, nil).Root(dir).request(); r.start != filepath.Clean(dir) {
		t.Fatalf("root as start: %q", r.start)
	}
	if r := New(pick, nil).Start(file).request(); r.start != "" {
		t.Fatalf("a file as start: %q", r.start)
	}
	if r := New(pick, nil).Start(filepath.Join(dir, "missing")).request(); r.start != "" {
		t.Fatalf("a missing start: %q", r.start)
	}
	if r := New(pick, nil).Mode(Save).SuggestedName("a/b.txt").request(); r.suggested != "" {
		t.Fatalf("a name with a separator: %q", r.suggested)
	}
	if r := New(pick, nil).SuggestedName("x.txt").request(); r.suggested != "" {
		t.Fatalf("a name outside Save mode: %q", r.suggested)
	}
	if r := New(pick, nil).Mode(Save).SuggestedName("x.txt").Title("a\xffb").request(); r.suggested != "x.txt" || r.title != "a�b" {
		t.Fatalf("save name and title: %+v", r)
	}
}

func TestRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "photos")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	pic := filepath.Join(sub, "a.jpg")
	if err := os.WriteFile(pic, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()

	got, err := relToRoot(root, []string{pic, sub})
	if err != nil || !slices.Equal(got, []string{"photos/a.jpg", "photos"}) {
		t.Fatalf("inside: %q, %v", got, err)
	}
	if got, err := relToRoot(root, []string{root}); err != nil || !slices.Equal(got, []string{"."}) {
		t.Fatalf("the root itself: %q, %v", got, err)
	}
	if _, err := relToRoot(root, []string{filepath.Join(outside, "x")}); !errors.Is(err, errOutsideRoot) {
		t.Fatalf("outside: %v", err)
	}
	if got, err := relToRoot(root, []string{filepath.Join(sub, "new.jpg")}); err != nil || !slices.Equal(got, []string{"photos/new.jpg"}) {
		t.Fatalf("save inside: %q, %v", got, err)
	}
	if _, err := relToRoot(root, []string{filepath.Join(outside, "new.jpg")}); !errors.Is(err, errOutsideRoot) {
		t.Fatalf("save outside: %v", err)
	}

	if runtime.GOOS == "windows" {
		return
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if got, err := relToRoot(link, []string{pic}); err != nil || !slices.Equal(got, []string{"photos/a.jpg"}) {
		t.Fatalf("a linked root, real path: %q, %v", got, err)
	}
	if got, err := relToRoot(root, []string{filepath.Join(link, "photos", "new.jpg")}); err != nil || !slices.Equal(got, []string{"photos/new.jpg"}) {
		t.Fatalf("save through a link: %q, %v", got, err)
	}
	if _, err := relToRoot(link, []string{filepath.Join(outside, "new.jpg")}); !errors.Is(err, errOutsideRoot) {
		t.Fatalf("save outside a linked root: %v", err)
	}
}

func TestRootTurnsAnswersIntoFSPaths(t *testing.T) {
	root := t.TempDir()
	f := newFake()
	var o outcome
	var logs records
	mb := newMailbox()
	d := New(o.pick, o.cancel).Root(root).Logger(slog.New(&logs))
	d.op = fakeOpener(f)
	d.Open(mb.post)
	f.waitOpened(t)
	f.answer(t, 0, result{paths: []string{filepath.Join(root, "a.txt")}})
	mb.run(t)
	d.Open(mb.post)
	f.waitOpened(t)
	f.answer(t, 1, result{paths: []string{filepath.Join(t.TempDir(), "b.txt")}})
	mb.run(t)
	if len(o.picked) != 1 || !slices.Equal(o.picked[0], []string{"a.txt"}) || o.cancelled != 1 {
		t.Fatalf("got %+v", o)
	}
	if !logs.has("outside the root") {
		var b bytes.Buffer
		for _, r := range logs.recs {
			b.WriteString(r.Message + "\n")
		}
		t.Fatalf("no warning about the root:\n%s", b.String())
	}
}

func TestCancelDropsAnImmediateAnswer(t *testing.T) {
	var o outcome
	mb := newMailbox()
	d := New(o.pick, o.cancel)
	d.op = unsupportedOpener()
	cancel, err := d.Open(mb.post)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	cancel()
	mb.drain()
	if o.cancelled != 0 {
		t.Fatalf("a cancelled unsupported answer was reported: %+v", o)
	}

	f := newFake()
	op := fakeOpener(f)
	first := New(o.pick, o.cancel)
	first.op = op
	first.Open(mb.post)
	f.waitOpened(t)
	busy := first
	cancel, err = busy.Open(mb.post)
	if !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	cancel()
	mb.drain()
	if o.cancelled != 0 {
		t.Fatalf("a cancelled busy answer was reported: %+v", o)
	}
}
