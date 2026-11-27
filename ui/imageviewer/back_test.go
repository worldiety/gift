package imageviewer_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/imageviewer"
)

// behind is a view that takes every back action that reaches it and counts
// them: what is under the viewer, a navigation stack that would pop or an
// application that would be left.
type behind struct {
	content gift.View
	taken   *int
}

var behindType = gift.RegisterType("imageviewer_test.behind")

func (behind) ViewType() gift.TypeID { return behindType }

func (b behind) Build(bc *gift.BuildContext) gift.Element {
	e := ui.ZStack(b.content).Build(bc)
	e.Interactor = behindHandler{b.taken}
	return e
}

type behindHandler struct{ taken *int }

func (behindHandler) HandleEvent(*gift.EventContext, gift.Event) bool { return false }
func (behindHandler) CanGoBack() bool                                 { return true }
func (h behindHandler) GoBack(*gift.EventContext)                     { *h.taken++ }

// setupBehind is [setup] inside a [behind], whose count it returns.
func setupBehind(t *testing.T, n int, configure func(imageviewer.FullscreenView) imageviewer.FullscreenView) (*env, *int) {
	t.Helper()
	taken := new(int)
	e := setupIn(t, n, configure, func(v gift.View) gift.View { return behind{content: v, taken: taken} })
	return e, taken
}

func TestBackPutsTheCardAwayThenClosesTheViewerAndThenPassesOn(t *testing.T) {
	e, taken := setupBehind(t, 2, func(f imageviewer.FullscreenView) imageviewer.FullscreenView {
		return f.Card(func(id asset.ID) gift.View { return ui.Text(string(id)).Key("details") })
	})
	e.open(0)
	e.h.Fling(geom.Pt(400, 400), geom.Pt(400, 250), 150*time.Millisecond)
	e.wait(time.Second)
	if b := e.h.Find(gifttest.ByKey("details")).LayoutBounds(); b.Min.Y > 300 {
		t.Fatalf("after a swipe up the card is at %v, want on screen", b)
	}
	if !e.h.CanGoBack() {
		t.Fatal("an open viewer with its card up does not take back")
	}
	if !e.h.Back() {
		t.Fatal("back over the open card was not taken")
	}
	e.wait(time.Second)
	if !e.v.IsOpen() {
		t.Fatal("back closed the viewer instead of the card")
	}
	if b := e.h.Find(gifttest.ByKey("details")).LayoutBounds(); b.Min.Y < 470 {
		t.Errorf("after back the card is at %v, want put away", b)
	}
	if !e.h.CanGoBack() || !e.h.Back() {
		t.Fatal("back over the open viewer was not taken")
	}
	e.wait(2 * time.Second)
	if e.v.IsOpen() {
		t.Fatal("the second back did not close the viewer")
	}
	if *taken != 0 {
		t.Errorf("what is behind the viewer took %d back actions while it was open, want none", *taken)
	}
	// Closed, the viewer is out of the way.
	e.h.Back()
	if *taken != 1 {
		t.Errorf("back with the viewer closed reached what is behind it %d times, want once", *taken)
	}
}

func TestBackOnAViewerThatIsNotClosableIsTakenAndDoesNothing(t *testing.T) {
	e, taken := setupBehind(t, 2, func(f imageviewer.FullscreenView) imageviewer.FullscreenView { return f.Closable(false) })
	e.open(0)
	if !e.h.CanGoBack() {
		t.Fatal("a viewer that is not closable lets back pass")
	}
	if !e.h.Back() {
		t.Fatal("back over a viewer that is not closable was not taken")
	}
	e.wait(2 * time.Second)
	if !e.v.IsOpen() {
		t.Error("back closed a viewer that is not closable")
	}
	if *taken != 0 {
		t.Errorf("back went past a viewer that is not closable %d times", *taken)
	}
}

func TestASecondBackWhileTheViewerClosesIsTakenAndDoesNotReachWhatIsBehind(t *testing.T) {
	e, taken := setupBehind(t, 2, nil)
	e.open(0)
	if !e.h.Back() {
		t.Fatal("back over the open viewer was not taken")
	}
	e.wait(50 * time.Millisecond)
	if !e.v.IsOpen() {
		t.Fatal("the viewer closed within 50 ms, too fast for this test")
	}
	if !e.h.CanGoBack() {
		t.Error("a closing viewer lets back pass")
	}
	if !e.h.Back() {
		t.Error("back during the close was not taken")
	}
	if *taken != 0 {
		t.Errorf("back during the close reached what is behind the viewer %d times", *taken)
	}
	e.wait(2 * time.Second)
	if e.v.IsOpen() {
		t.Error("the viewer did not finish closing")
	}
	if *taken != 0 {
		t.Errorf("what is behind the viewer took %d back actions, want none", *taken)
	}
}

func TestBackOnAClosedViewerIsNotTaken(t *testing.T) {
	e := setup(t, 2, nil)
	if e.h.CanGoBack() || e.h.Back() {
		t.Error("a closed viewer takes back, which would keep an application from being left")
	}
	e.open(0)
	if !e.h.CanGoBack() {
		t.Error("an open viewer does not take back")
	}
}

func TestAskingAViewerWhetherItGoesBackDoesNotAllocate(t *testing.T) {
	e := setup(t, 2, nil)
	e.open(0)
	app := e.h.App()
	if n := testing.AllocsPerRun(100, func() { app.CanGoBack() }); n != 0 {
		t.Errorf("CanGoBack over an open viewer allocates %v times", n)
	}
}
