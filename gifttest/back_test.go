package gifttest_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
)

var backStackType = gift.RegisterType("gifttest.BackStack")

// backStack is a navigation stack in the small: a node whose interactor is a
// [gift.BackHandler] that pops while it is more than one screen deep, and
// that is built with its depth, as a real stack decides with what it was
// built with.
type backStack struct{ depth *int }

func (backStack) ViewType() gift.TypeID { return backStackType }

func (s backStack) Build(*gift.BuildContext) gift.Element {
	n := &backStackNode{depth: s.depth, built: *s.depth}
	return gift.Element{Key: "stack", Layouter: n, Interactor: n}
}

type backStackNode struct {
	depth *int
	built int
}

func (*backStackNode) Layout(*gift.LayoutContext, geom.Constraints) geom.Size {
	return geom.Sz(100, 100)
}

func (*backStackNode) HandleEvent(*gift.EventContext, gift.Event) bool { return false }

func (n *backStackNode) CanGoBack() bool { return n.built > 1 }

func (n *backStackNode) GoBack(*gift.EventContext) { *n.depth-- }

// TestBackPopsUntilTheFirstScreenAndThenLeavesTheApplication is the harness
// verb as a test of back navigation uses it: Back reports what the platform
// would do, CanGoBack what it would be told beforehand, and each answers for
// the tree the settle before it left.
func TestBackPopsUntilTheFirstScreenAndThenLeavesTheApplication(t *testing.T) {
	depth := 3
	h := gifttest.New(t, gifttest.Options{View: backStack{depth: &depth}})
	for _, want := range []int{2, 1} {
		if !h.CanGoBack() {
			t.Fatalf("CanGoBack is false %d screens deep", depth)
		}
		if !h.Back() {
			t.Fatalf("Back did not take the action %d screens deep", depth)
		}
		if depth != want {
			t.Fatalf("depth after Back = %d, want %d", depth, want)
		}
		// The depth is the test's state, not gift's; an application's state
		// change would rebuild the stack on its own.
		h.App().InvalidateAll()
		h.Settle()
	}
	if h.CanGoBack() {
		t.Fatal("CanGoBack is true on the first screen")
	}
	if h.Back() {
		t.Fatal("Back took the action on the first screen; the platform would not leave the application")
	}
	if depth != 1 {
		t.Fatalf("Back on the first screen changed the depth to %d", depth)
	}
}
