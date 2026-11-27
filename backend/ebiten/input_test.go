package ebiten

import (
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// The Ebitengine readings mean nothing outside a running game loop, so every
// one of them goes through a function value described on [inputBridge]:
// keyboard_test.go drives the keyboard through them, density_test.go and
// drop_test.go the mouse, and this file the touches, which no desktop ever
// reports.
//
// What is here besides is the table the key half is built on, and it is worth
// checking on its own, because a wrong entry there is a key that silently
// never arrives.

func TestTrackedKeysAreDistinctAndComplete(t *testing.T) {
	seen := map[eb.Key]bool{}
	for _, k := range trackedKeys {
		if seen[k.eb] {
			t.Errorf("the Ebitengine key %v is listed twice; the second entry would produce a duplicate event", k.eb)
		}
		seen[k.eb] = true
	}

	// Every gift key except KeyOther must be produced by at least one entry.
	// KeyOther is what a backend reports for a key gift has no name for, so
	// it is deliberately absent.
	want := []gift.Key{
		gift.KeyTab, gift.KeySpace, gift.KeyEnter, gift.KeyEscape,
		gift.KeyLeft, gift.KeyRight, gift.KeyUp, gift.KeyDown,
		gift.KeyHome, gift.KeyEnd, gift.KeyPageUp, gift.KeyPageDown,
		// Added by the project plan, section 19: the editing keys and the
		// operands of the five shortcuts. They are here for the same reason
		// as the rest — an entry missing from the table is a key that never
		// arrives, and for backspace that is a text field that cannot delete.
		gift.KeyBackspace, gift.KeyDelete,
		gift.KeyA, gift.KeyC, gift.KeyV, gift.KeyX, gift.KeyZ,
	}
	for _, w := range want {
		found := false
		for _, k := range trackedKeys {
			if k.key == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no Ebitengine key maps to gift key %d; it can never be delivered", w)
		}
	}
}

// touchApp is an application whose single node takes every finger and records
// the pointer events it receives.
type touchApp struct {
	app    *gift.App
	events []gift.Event
}

var touchRecorderType = gift.RegisterType("backend.test.TouchRecorder")

type touchRecorderView struct{ app *touchApp }

func (touchRecorderView) ViewType() gift.TypeID { return touchRecorderType }

func (v touchRecorderView) Build(*gift.BuildContext) gift.Element {
	n := &touchRecorderNode{app: v.app}
	return gift.Element{Key: "touch", Layouter: n, Interactor: n, MultiTouch: true}
}

type touchRecorderNode struct{ app *touchApp }

func (n *touchRecorderNode) Layout(*gift.LayoutContext, geom.Constraints) geom.Size {
	return geom.Sz(100, 40)
}

func (n *touchRecorderNode) HandleEvent(_ *gift.EventContext, e gift.Event) bool {
	n.app.events = append(n.app.events, e)
	return true
}

// touchBridge builds the application and a bridge whose touch readings are
// the ids *ids names, every one of them inside the node.
func touchBridge(t *testing.T, ids *[]eb.TouchID) (*touchApp, *inputBridge) {
	t.Helper()
	ta := &touchApp{}
	ta.app = gift.New(gift.Options{Root: func(*gift.Context) gift.View { return touchRecorderView{app: ta} }})
	if err := ta.app.Update(geom.Sz(200, 200)); err != nil {
		t.Fatal(err)
	}
	ta.app.Paint()
	b := newInputBridge(ta.app)
	b.density = 1
	b.touchIDs = func(dst []eb.TouchID) []eb.TouchID { return append(dst, (*ids)...) }
	b.touchPos = func(eb.TouchID) (float64, float64) { return 10, 10 }
	return ta, b
}

// tick runs one tick of the touch half of the bridge and returns the pointer
// events it delivered, as "down:1", "move:1", "up:1" and "cancel:1".
func (ta *touchApp) tick(b *inputBridge) []string {
	ta.events = ta.events[:0]
	ta.app.BeginInput(0)
	b.pollTouches()
	var got []string
	for _, e := range ta.events {
		var kind string
		switch e.Kind {
		case gift.EventPointerDown:
			kind = "down"
		case gift.EventPointerMove:
			kind = "move"
		case gift.EventPointerUp:
			kind = "up"
		case gift.EventPointerCancel:
			kind = "cancel"
		default:
			continue
		}
		got = append(got, kind+":"+itoa(int(e.Pointer)))
	}
	return got
}

// TestTouchDiffing drives the edge detection of pollTouches through a scripted
// sequence of touch ids, without Ebitengine.
//
// It is the part of the bridge that has real logic in it: everything else is a
// boolean compared with the previous tick. Ebitengine's own documentation says
// "AppendTouchIDs always does nothing on desktops", so on the development
// machine this code path is never executed at all and a bug in it would only
// appear on the target device.
func TestTouchDiffing(t *testing.T) {
	type step struct {
		ids  []eb.TouchID
		want []string
	}
	// The script is written against the diff, not against Ebitengine: down
	// for an id that is new, move for one that persists, up for one that
	// vanished.
	steps := []step{
		{ids: []eb.TouchID{1}, want: []string{"down:1"}},
		{ids: []eb.TouchID{1}, want: []string{"move:1"}},
		{ids: []eb.TouchID{1, 2}, want: []string{"move:1", "down:2"}},
		{ids: []eb.TouchID{2}, want: []string{"move:2", "up:1"}},
		{ids: nil, want: []string{"up:2"}},
		{ids: nil, want: nil},
	}

	var ids []eb.TouchID
	ta, b := touchBridge(t, &ids)
	for i, s := range steps {
		ids = s.ids
		if got := ta.tick(b); !equalStrings(got, s.want) {
			t.Fatalf("step %d: got %v, want %v", i, got, s.want)
		}
	}
}

// TestCancelledTouchesEndWithACancel: a touch the platform took over, which
// [CancelTouches] reports, ends with a cancel and with nothing else — not
// with a release, which would activate what it pressed, and not with the moves
// of a finger that is now the system's — whichever of the host's two calls
// Ebitengine's own list of touches sees first. An id it lists again later is
// a new touch.
func TestCancelledTouchesEndWithACancel(t *testing.T) {
	type step struct {
		ids    []eb.TouchID
		cancel bool
		want   []string
	}
	cases := []struct {
		name  string
		steps []step
	}{
		{"still listed", []step{
			{ids: []eb.TouchID{1}, want: []string{"down:1"}},
			{ids: []eb.TouchID{1}, cancel: true, want: []string{"cancel:1"}},
			{ids: []eb.TouchID{1}, want: nil},
			{ids: nil, want: nil},
			{ids: []eb.TouchID{1}, want: []string{"down:1"}},
		}},
		{"already gone", []step{
			{ids: []eb.TouchID{1}, want: []string{"down:1"}},
			{ids: nil, cancel: true, want: []string{"cancel:1"}},
			{ids: nil, want: nil},
			{ids: []eb.TouchID{1}, want: []string{"down:1"}},
		}},
		{"before the first report", []step{
			{ids: []eb.TouchID{1}, cancel: true, want: nil},
			{ids: []eb.TouchID{1}, want: nil},
			{ids: nil, want: nil},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The mailbox is process wide.
			t.Cleanup(func() { touchCancels.Store(false) })
			var ids []eb.TouchID
			ta, b := touchBridge(t, &ids)
			for i, s := range c.steps {
				ids = s.ids
				if s.cancel {
					touchCancels.Store(true)
				}
				if got := ta.tick(b); !equalStrings(got, s.want) {
					t.Fatalf("step %d: got %v, want %v; a cancelled touch must end with a cancel only, never be reported as released or moved", i, got, s.want)
				}
			}
		})
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
