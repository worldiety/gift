package filedialog

import (
	"log/slog"
	"testing"
	"testing/fstest"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/filepicker"
)

// stage is an application with one PresentView, the state that presents it,
// and a content component that counts how often it was mounted.
type stage struct {
	t        *testing.T
	h        *gifttest.Harness
	picking  *gift.State[bool]
	shown    *gift.State[bool]
	mounts   int
	o        outcome
	fallback bool
	// closeOnAnswer makes the application set picking back to false from
	// onPick and onCancel, as it should; without it, it never does.
	closeOnAnswer bool
	nilCancel     bool
	log           *slog.Logger
}

func newStage(t *testing.T, op *opener, fallback, closeOnAnswer bool) *stage {
	s := &stage{t: t, fallback: fallback, closeOnAnswer: closeOnAnswer}
	s.mount(op)
	return s
}

func (s *stage) mount(op *opener) {
	root := func(ctx *gift.Context) gift.View {
		s.picking = ctx.State("picking", false)
		s.shown = ctx.State("shown", true)
		if !ctx.Read(s.shown) {
			return ui.Window(ui.Text("gone"))
		}
		content := gift.Component("content", func(c *gift.Context) gift.View {
			id := c.State("id", (*int)(nil))
			if id.Get() == nil {
				v := 0
				id.Set(&v)
				s.mounts++
			}
			return ui.Text("content")
		})
		pick := func(p []string) {
			s.o.pick(p)
			if s.closeOnAnswer {
				s.picking.Set(false)
			}
		}
		cancel := func() {
			s.o.cancel()
			if s.closeOnAnswer {
				s.picking.Set(false)
			}
		}
		if s.nilCancel {
			cancel = nil
		}
		dlg := New(pick, cancel).Logger(s.log)
		dlg.op = op
		pv := Present(content, dlg).Presented(ctx.Read(s.picking))
		if s.fallback {
			pv = pv.Fallback(filepicker.New(fstest.MapFS{"a.txt": {}}, pick, cancel))
		}
		return ui.Window(pv)
	}
	s.h = gifttest.New(s.t, gifttest.Options{
		Root:  root,
		Size:  geom.Sz(420, 720),
		Theme: ui.LightTheme(),
		Font:  ui.MustFont(ui.FontQuery{Family: inter.Family}),
	})
	s.h.Settle()
}

func (s *stage) present(b bool) {
	s.picking.Set(b)
	s.h.Settle()
}

// until runs frames until cond holds; answers arrive from other goroutines,
// which Settle does not wait for.
func (s *stage) until(what string, cond func() bool) {
	s.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.h.Settle()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("%s did not happen within 5 s\n%s", what, s.h.Dump())
		}
		time.Sleep(time.Millisecond)
	}
}

// settleAWhile runs frames for a short while, for what must not happen.
func (s *stage) settleAWhile() {
	deadline := time.Now().Add(80 * time.Millisecond)
	for time.Now().Before(deadline) {
		s.h.Settle()
		time.Sleep(time.Millisecond)
	}
}

func (s *stage) pickerShown() bool {
	return s.h.Exists(gifttest.ByType("filepicker.Picker"))
}

func TestPresentOpensOnceAndDelivers(t *testing.T) {
	f := newFake()
	s := newStage(t, fakeOpener(f), false, true)
	s.present(true)
	f.waitOpened(t)
	s.h.Settle()
	s.h.Settle()
	f.notOpened(t)
	f.answer(t, 0, result{paths: []string{"/a"}})
	s.until("the pick", func() bool { return len(s.o.picked) == 1 })
	if s.picking.Get() {
		t.Fatal("the application did not close the dialog")
	}
	s.present(true)
	f.waitOpened(t)
}

func TestPresentClosesWithoutACallback(t *testing.T) {
	f := newFake()
	f.final = true
	s := newStage(t, fakeOpener(f), false, true)
	s.present(true)
	f.waitOpened(t)
	s.present(false)
	if f.closeCount() != 1 {
		t.Fatalf("close ran %d times, want 1", f.closeCount())
	}
	s.present(true)
	f.waitOpened(t)
	s.settleAWhile()
	if s.o.cancelled != 0 || len(s.o.picked) != 0 {
		t.Fatalf("closing reported %+v", s.o)
	}
}

func TestPresentClosesOnUnmount(t *testing.T) {
	f := newFake()
	f.final = true
	s := newStage(t, fakeOpener(f), false, true)
	s.present(true)
	f.waitOpened(t)
	s.shown.Set(false)
	s.h.Settle()
	if f.closeCount() != 1 {
		t.Fatalf("close ran %d times on unmount, want 1", f.closeCount())
	}
}

func TestPresentOpensOnlyOnTheEdge(t *testing.T) {
	f := newFake()
	s := &stage{t: t, nilCancel: true}
	s.mount(fakeOpener(f))
	s.present(true)
	f.waitOpened(t)
	f.answer(t, 0, result{cancelled: true})
	s.settleAWhile()
	f.notOpened(t)
	if !s.picking.Get() {
		t.Fatal("picking changed on its own")
	}
}

func TestPresentUnsupportedDoesNotLoop(t *testing.T) {
	s := newStage(t, unsupportedOpener(), false, false)
	s.present(true)
	s.settleAWhile()
	if s.o.cancelled != 1 || len(s.o.picked) != 0 {
		t.Fatalf("got %+v, want exactly one cancel", s.o)
	}
}

func TestPresentFallbackOnAnUnsupportedSystem(t *testing.T) {
	s := newStage(t, unsupportedOpener(), true, true)
	if s.pickerShown() {
		t.Fatal("the fallback is mounted before it is presented")
	}
	s.present(true)
	if !s.pickerShown() {
		t.Fatalf("no fallback picker\n%s", s.h.Dump())
	}
	if s.o.cancelled != 0 {
		t.Fatalf("onCancel was called: %+v", s.o)
	}
	s.present(false)
	if s.pickerShown() {
		t.Fatal("the fallback stayed after Presented(false)")
	}
	if s.mounts != 1 {
		t.Fatalf("the content was mounted %d times", s.mounts)
	}
}

func TestPresentFallbackWhenOpeningFails(t *testing.T) {
	op := newOpener(func(openConfig) (platform, error) {
		return nil, permanent(ErrUnsupported)
	}, func() bool { return true }, noEnv)
	s := newStage(t, op, true, true)
	s.present(true)
	s.until("the fallback", s.pickerShown)
	if s.o.cancelled != 0 {
		t.Fatalf("onCancel was called: %+v", s.o)
	}
	if s.mounts != 1 {
		t.Fatalf("the content was mounted %d times", s.mounts)
	}
}

func TestPresentLateFailureAfterCloseShowsNoFallback(t *testing.T) {
	gate := make(chan struct{})
	op := newOpener(func(openConfig) (platform, error) {
		<-gate
		return nil, permanent(ErrUnsupported)
	}, func() bool { return true }, noEnv)
	s := newStage(t, op, true, true)
	s.present(true)
	s.present(false)
	close(gate)
	s.settleAWhile()
	if s.pickerShown() || s.o.cancelled != 0 {
		t.Fatalf("a late failure changed a closed presentation: picker %v, %+v", s.pickerShown(), s.o)
	}
}

func TestPresentFallbackOnlyWhenNeeded(t *testing.T) {
	f := newFake()
	s := newStage(t, fakeOpener(f), true, true)
	s.present(true)
	f.waitOpened(t)
	s.h.Settle()
	if s.pickerShown() {
		t.Fatal("the fallback is shown next to a native dialog")
	}
}

func TestPresentFallbackLogsTheReasonOnce(t *testing.T) {
	var logs records
	s := &stage{t: t, fallback: true, closeOnAnswer: true, log: slog.New(&logs)}
	s.mount(unsupportedOpener())
	s.present(true)
	s.present(false)
	s.present(true)
	if !s.pickerShown() {
		t.Fatal("no fallback")
	}
	if logs.count(slog.LevelError) != 1 {
		t.Fatalf("want one Error record, got %d", logs.count(slog.LevelError))
	}
}
