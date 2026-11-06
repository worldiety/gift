package filedialog

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
)

// platform is what each operating system provides.
type platform interface {
	// open shows the dialog r describes and returns at once. done is called
	// exactly once, on any goroutine, and never blocks the macOS main
	// thread. close is best effort and may be called more than once; it
	// reports whether no done can follow it (the portal), or whether done
	// still comes, with a cancel the core drops (macOS, Windows).
	//
	// An error wrapped by [permanent] marks the process as one without a
	// native dialog for good; an [ErrUnsupported] that is not answers this
	// request only.
	open(r request, done func(result)) (close func() (final bool), err error)
}

// openConfig is what opening a platform layer is handed.
type openConfig struct {
	// log is the logger of the dialog that opened the layer.
	log *slog.Logger
	// getenv is os.Getenv; tests replace it.
	getenv func(string) string
}

// permanentError marks a failure after which the process has no native
// dialog for good.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// permanent wraps err as a lasting failure; see [platform].
func permanent(err error) error { return permanentError{err} }

// kioskEnv is the variable a gift kiosk session sets, the same constant as
// kioskEnv in kiosk/system/install.go. It is copied rather than imported so
// that this package does not depend on the kiosk.
const kioskEnv = "GIFT_KIOSK"

// opener is the one platform layer of a process, opened on first use, and
// the bookkeeping that allows one native dialog at a time.
type opener struct {
	once   sync.Once
	p      platform
	failed atomic.Pointer[error]
	logged atomic.Bool

	mu    sync.Mutex
	cur   uint64 // generation of the newest request
	owner uint64 // generation of the request holding the dialog; 0 is none

	open   func(openConfig) (platform, error)
	avail  func() bool
	getenv func(string) string
}

// std is the opener of the process.
var std = newOpener(openPlatform, sync.OnceValue(platformAvailable), os.Getenv)

func newOpener(open func(openConfig) (platform, error), avail func() bool, getenv func(string) string) *opener {
	return &opener{open: open, avail: avail, getenv: getenv}
}

// available reports whether a dialog is attempted at all.
func (o *opener) available() bool { return o.unavailable() == nil }

// unavailable is the reason no dialog is attempted, or nil.
func (o *opener) unavailable() error {
	if o.getenv(kioskEnv) != "" {
		return fmt.Errorf("%w: a gift kiosk session (%s is set)", ErrUnsupported, kioskEnv)
	}
	if err := o.failed.Load(); err != nil {
		return *err
	}
	if !o.avail() {
		return fmt.Errorf("%w: not macOS, Windows or a GNOME or KDE session with a session bus", ErrUnsupported)
	}
	return nil
}

// impl opens the platform layer once. It may block; it is only called off
// the UI goroutine.
func (o *opener) impl(log *slog.Logger) (platform, error) {
	o.once.Do(func() {
		p, err := o.open(openConfig{log: log, getenv: o.getenv})
		if err != nil {
			o.fail(err)
			return
		}
		o.p = p
	})
	if o.p == nil {
		if err := o.failed.Load(); err != nil {
			return nil, *err
		}
		return nil, ErrUnsupported
	}
	return o.p, nil
}

func (o *opener) fail(err error) {
	if !errors.Is(err, ErrUnsupported) {
		err = fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	o.failed.CompareAndSwap(nil, &err)
}

// reportUnavailable logs that no dialog will be shown: once per process at
// Error, on the first dialog that has a logger, and at Debug after that.
func (o *opener) reportUnavailable(log *slog.Logger, err error) {
	if log == nil {
		return
	}
	if o.logged.CompareAndSwap(false, true) {
		logf(log, slog.LevelError, "native file dialogs unavailable, nothing will be shown", "error", err)
		return
	}
	logf(log, slog.LevelDebug, "no native file dialog", "error", err)
}

// acquire claims the dialog for a new request.
func (o *opener) acquire() (gen uint64, ok bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.owner != 0 {
		return 0, false
	}
	o.cur++
	o.owner = o.cur
	return o.cur, true
}

// release gives the dialog back, if gen still holds it.
func (o *opener) release(gen uint64) {
	o.mu.Lock()
	if o.owner == gen {
		o.owner = 0
	}
	o.mu.Unlock()
}

func noop() {}

// open is [Dialog.Open], with what an unavailable dialog does in place of
// onCancel: [PresentView] shows its fallback instead. A nil unsupported
// means onCancel.
func (d Dialog) open(post func(func()), unsupported func()) (func(), error) {
	o := d.opener()
	if post == nil {
		logf(d.log, slog.LevelWarn, "Open with a nil post: nothing is shown and nothing will be reported")
		return noop, fmt.Errorf("%w: Open with a nil post", ErrUnsupported)
	}
	if unsupported == nil {
		unsupported = d.onCancel
	}
	// An answer given at once goes through a pending too, so that the
	// returned cancel drops it like any other.
	if err := o.unavailable(); err != nil {
		o.reportUnavailable(d.log, err)
		q := &pending{post: post}
		q.say(unsupported)
		return q.cancel, err
	}
	gen, ok := o.acquire()
	if !ok {
		logf(d.log, slog.LevelDebug, "a native file dialog is already open")
		q := &pending{post: post}
		q.say(d.onCancel)
		return q.cancel, ErrBusy
	}
	q := &pending{o: o, gen: gen, d: d, post: post, unsupported: unsupported}
	r := d.request()
	go q.run(r)
	return q.cancel, nil
}

// pending is one request between Open and its answer.
type pending struct {
	o           *opener
	gen         uint64
	d           Dialog
	post        func(func())
	unsupported func()

	freed sync.Once

	mu        sync.Mutex
	cancelled bool
	close     func() bool
}

func (q *pending) free() {
	q.freed.Do(func() {
		if q.o != nil {
			q.o.release(q.gen)
		}
	})
}

func (q *pending) isCancelled() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.cancelled
}

// cancel is the function [Dialog.Open] returns.
func (q *pending) cancel() {
	q.mu.Lock()
	if q.cancelled {
		q.mu.Unlock()
		return
	}
	q.cancelled = true
	c := q.close
	q.mu.Unlock()
	if c != nil && c() {
		q.free()
	}
}

// say posts fn, which runs only if the request was not cancelled by then.
func (q *pending) say(fn func()) {
	if fn == nil {
		return
	}
	q.post(func() {
		if !q.isCancelled() {
			fn()
		}
	})
}

func (q *pending) run(r request) {
	log := q.d.log
	p, err := q.o.impl(log)
	if err != nil {
		q.free()
		q.o.reportUnavailable(log, err)
		q.say(q.unsupported)
		return
	}
	if q.isCancelled() {
		q.free()
		return
	}
	c, err := p.open(r, q.done)
	if err != nil {
		q.free()
		var pe permanentError
		switch {
		case errors.As(err, &pe):
			q.o.fail(err)
			q.o.reportUnavailable(log, err)
			q.say(q.unsupported)
		case errors.Is(err, ErrUnsupported):
			logf(log, slog.LevelWarn, "the native file dialog cannot be shown now", "error", err)
			q.say(q.unsupported)
		default:
			logf(log, slog.LevelError, "the native file dialog failed", "error", err)
			q.say(q.d.onCancel)
		}
		return
	}
	q.mu.Lock()
	q.close = c
	cancelled := q.cancelled
	q.mu.Unlock()
	if cancelled && c() {
		q.free()
	}
}

// done is the answer of the platform layer.
func (q *pending) done(res result) {
	q.free()
	if q.isCancelled() {
		return
	}
	d, log := q.d, q.d.log
	switch {
	case res.err != nil:
		logf(log, slog.LevelError, "the native file dialog failed", "error", res.err)
		q.say(d.onCancel)
		return
	case res.cancelled:
		q.say(d.onCancel)
		return
	case len(res.paths) == 0:
		logf(log, slog.LevelError, "the native file dialog answered with no path and no cancel")
		q.say(d.onCancel)
		return
	}
	paths := res.paths
	if d.mode != OpenMulti && len(paths) > 1 {
		logf(log, slog.LevelWarn, "the native file dialog answered with more than one path, the first is kept", "count", len(paths))
		paths = paths[:1]
	}
	if d.root != "" {
		rel, err := relToRoot(d.root, paths)
		if err != nil {
			logf(log, slog.LevelWarn, "the picked path is outside the root, reported as cancel", "root", d.root, "paths", paths, "error", err)
			q.say(d.onCancel)
			return
		}
		paths = rel
	}
	onPick := d.onPick
	q.say(func() { onPick(paths) })
}
