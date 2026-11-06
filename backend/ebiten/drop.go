package ebiten

import (
	"io/fs"
	"log/slog"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// This file brings content dropped from other applications into gift; see
// the project plan, section 49, and gift's drop.go for the session it feeds.
//
// There are two sources. Ebitengine reports dropped files on macOS, Windows
// and Linux through [eb.DroppedFiles], for one tick, with nothing before the
// drop: that is [App.Drop] alone, at the polled cursor. On macOS and Windows a
// native layer adds what Ebitengine leaves out — the drag while it moves over
// the window, text and web addresses — and reports into a queue that
// [inputBridge.pollDrops] drains. Where a native layer is active it is the
// one source of truth, and Ebitengine's report of the same drop is dropped.

// nativeDropOp is what a native layer reports.
type nativeDropOp uint8

const (
	nativeOver nativeDropOp = iota
	nativeLeave
	nativeDrop
)

// nativeDropEvent is one report of a native layer. x and y are in the unit of
// [eb.CursorPositionF], so that [inputBridge.logical] applies to them as to the
// cursor.
type nativeDropEvent struct {
	op    nativeDropOp
	x, y  float64
	kinds gift.DropKind
	data  gift.DropData
}

// nativeDrops is a native drag layer.
type nativeDrops interface {
	// drain appends what was reported since the last call.
	drain(dst []nativeDropEvent) []nativeDropEvent
	// answer is what the layer tells the system while a drag is over the
	// window: whether the target under it accepts, and before the first
	// answer of a session whether the window has targets at all.
	answer(accepted, hasTargets bool)
	// close takes the layer down. It runs on the main thread.
	close()
}

// dropQueue is the queue a native layer reports into from the main thread
// and the bridge drains on the UI goroutine. The slice is reused.
type dropQueue struct {
	mu   sync.Mutex
	evs  []nativeDropEvent
	wake func()

	accepted, hasTargets atomic.Bool

	// sess counts the sessions the layer opened, drained is the session of
	// the newest events the bridge took, under mu, and answered the session
	// the stored answer was computed for. Until the answer belongs to the
	// session in progress, the layer answers with hasTargets: an answer left
	// over from the previous drag says nothing about this one.
	sess, drained, answered atomic.Uint64
}

func (q *dropQueue) push(e nativeDropEvent) {
	q.mu.Lock()
	n := len(q.evs)
	switch {
	case e.op == nativeLeave && n > 0 && q.evs[n-1].op == nativeLeave:
		// Leaves collapse: one session ends once.
	case e.op == nativeOver && n > 0 && q.evs[n-1].op == nativeOver:
		// So do moves within one tick: only the latest position matters.
		q.evs[n-1] = e
	default:
		q.evs = append(q.evs, e)
	}
	q.mu.Unlock()
	if q.wake != nil {
		q.wake()
	}
}

func (q *dropQueue) drain(dst []nativeDropEvent) []nativeDropEvent {
	q.mu.Lock()
	q.drained.Store(q.sess.Load())
	dst = append(dst, q.evs...)
	clear(q.evs)
	q.evs = q.evs[:0]
	q.mu.Unlock()
	return dst
}

func (q *dropQueue) answer(accepted, hasTargets bool) {
	q.accepted.Store(accepted)
	q.hasTargets.Store(hasTargets)
	q.answered.Store(q.drained.Load())
}

// open marks the start of a session, before its first event is pushed.
func (q *dropQueue) open() { q.sess.Add(1) }

// wouldAccept is the answer a layer gives the system: the target's, and
// before the session has one, whether the window has targets at all.
func (q *dropQueue) wouldAccept() bool {
	if q.answered.Load() != q.sess.Load() {
		return q.hasTargets.Load()
	}
	return q.accepted.Load()
}

// nativeDropGrace is how many updates after a native drop a report of
// Ebitengine's is taken as the same drop: the one of the drop and the two
// before it, because the two can land in neighbouring ticks.
const nativeDropGrace = 3

// pollDrops drains the native layer and Ebitengine's report into gift.
func (b *inputBridge) pollDrops() {
	if b.native != nil {
		b.dropEvs = b.native.drain(b.dropEvs[:0])
		for i := range b.dropEvs {
			e := &b.dropEvs[i]
			switch e.op {
			case nativeOver:
				// Also under NoMouse: a drag from another application is
				// an event of the system, not a reading of the mouse.
				b.app.DragOver(b.logical(e.x, e.y), e.kinds)
			case nativeLeave:
				b.app.DragLeave()
			case nativeDrop:
				b.app.Drop(b.logical(e.x, e.y), e.data)
				b.nativeRecent = nativeDropGrace
			}
			*e = nativeDropEvent{}
		}
	}
	if b.dropped != nil {
		if fsys := b.dropped(); fsys != nil {
			switch {
			case b.nativeRecent > 0:
				b.debugDrop("gift/backend/ebiten: Ebitengine's report of a drop the native layer delivered was discarded")
			case b.native != nil:
				// Its native report may still be on its way; it is held for
				// the grace period, and delivered only if none comes, which
				// is a source that knows no more than WM_DROPFILES.
				x, y := b.cursor()
				b.heldFS, b.heldPos, b.heldFor = fsys, b.logical(x, y), nativeDropGrace
			default:
				x, y := b.cursor()
				b.deliverDropped(fsys, b.logical(x, y))
			}
		}
	}
	if b.heldFS != nil {
		switch {
		case b.nativeRecent > 0:
			b.heldFS = nil
			b.debugDrop("gift/backend/ebiten: Ebitengine's report of a drop the native layer delivered was discarded")
		case b.heldFor <= 0:
			fsys := b.heldFS
			b.heldFS = nil
			b.deliverDropped(fsys, b.heldPos)
		default:
			b.heldFor--
		}
	}
	if b.nativeRecent > 0 {
		b.nativeRecent--
	}
}

func (b *inputBridge) deliverDropped(fsys fs.FS, pos geom.Point) {
	if d := b.readDropped(fsys); len(d.Items) > 0 {
		b.app.Drop(pos, d)
	}
}

// readDropped turns Ebitengine's file system of a drop into paths.
//
// It reads directories and never opens a file: an entry whose FileInfo has no
// Sys value is one of the folders Ebitengine makes up to hold paths from
// different directories, and is descended into; every other entry was
// dropped, and its [eb.AbsPather] is the path. Ebitengine stops reading a
// folder at the first path it cannot stat — a KDE smb: address becomes such
// a path on Linux — so what follows it in the same folder is lost; that is
// Ebitengine's limit, and the entries before it arrive.
func (b *inputBridge) readDropped(fsys fs.FS) gift.DropData {
	var d gift.DropData
	b.walkDropped(fsys, ".", &d, 0)
	return d
}

func (b *inputBridge) walkDropped(fsys fs.FS, dir string, d *gift.DropData, depth int) {
	if depth > 64 {
		return
	}
	ents, err := fs.ReadDir(fsys, dir)
	for _, e := range ents {
		if e == nil {
			break
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		if info.Sys() == nil && info.IsDir() {
			b.walkDropped(fsys, path.Join(dir, e.Name()), d, depth+1)
			continue
		}
		ap, ok := e.(eb.AbsPather)
		if !ok {
			continue
		}
		d.Items = append(d.Items, gift.DropItem{Kind: gift.DropFile, Path: ap.AbsPath(), Dir: e.IsDir()})
	}
	if err != nil {
		b.debugDrop("gift/backend/ebiten: a dropped path could not be read; the paths after it in its folder are lost",
			slog.String("error", err.Error()))
	}
}

func (b *inputBridge) debugDrop(msg string, args ...any) {
	if b.log != nil {
		b.log.Debug(msg, args...)
	}
}

// installDrops sets up the native drag layer, once, from the first update:
// Ebitengine runs functions on the main thread only once the window exists.
// It also tells gift what can be dropped.
func (g *game) installDrops() {
	b := g.input
	if b == nil || b.dropsInstalled {
		return
	}
	b.dropsInstalled = true
	if b.dropped == nil {
		return
	}
	if os.Getenv("GIFT_DROP_NATIVE") == "0" {
		b.debugDrop("gift/backend/ebiten: GIFT_DROP_NATIVE=0, the native drag layer stays off")
		return
	}
	var n nativeDrops
	eb.RunOnMainThread(func() { n = openNativeDrops(g.wake, b.log) })
	if n == nil {
		return
	}
	b.native = n
	b.dropEvs = make([]nativeDropEvent, 0, 8)
	g.app.SetDropCapabilities(gift.DropCapabilities{Kinds: gift.DropFile | gift.DropText | gift.DropURL, Live: true})
}

// answerDrops hands the native layer the answer of this update.
func (g *game) answerDrops() {
	if b := g.input; b != nil && b.native != nil {
		b.native.answer(g.app.DropAccepted(), g.app.HasDropTargets())
	}
}

// closeDrops takes the native layer down, on the main thread after the game
// ended.
func (g *game) closeDrops() {
	if b := g.input; b != nil && b.native != nil {
		b.native.close()
		b.native = nil
	}
}

// dropPrecedence turns what a macOS pasteboard or a Windows data object
// offered into one drop: the files alone when there are any, which is what
// makes a Finder or Explorer drag that also carries the paths as text land at
// a file destination; otherwise the addresses, with the text only when it is
// not just one of them; and otherwise the text. The kinds during the drag
// follow the same rule.
func dropPrecedence(files []string, urls []string, text string, hasText bool) gift.DropData {
	var d gift.DropData
	if len(files) > 0 {
		for _, p := range files {
			it := gift.DropItem{Kind: gift.DropFile, Path: p}
			if fi, err := os.Stat(p); err == nil {
				it.Dir = fi.IsDir()
			}
			d.Items = append(d.Items, it)
		}
		return d
	}
	for _, u := range urls {
		d.Items = append(d.Items, gift.DropItem{Kind: gift.DropURL, URL: u})
	}
	if hasText && text != "" && !sameAsURLs(text, urls) {
		d.Items = append(d.Items, gift.DropItem{Kind: gift.DropText, Text: text})
	}
	return d
}

// sameAsURLs reports whether text, trimmed, is one of the addresses.
func sameAsURLs(text string, urls []string) bool {
	t := strings.TrimSpace(text)
	for _, u := range urls {
		if t == u {
			return true
		}
	}
	return false
}
