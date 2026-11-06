package gift_test

import (
	"strings"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var dropAreaType = gift.RegisterType("test.DropArea")

// dropArea is a w by h node with its children placed at offsets. It is a drop
// target for accepts (zero: none, it declines everything) and, with hover
// set, a pointer target that only tracks hover.
type dropArea struct {
	name     string
	w, h     float32
	accepts  gift.DropKind
	declines bool // decline the drop itself
	disabled bool
	hidden   bool
	hover    bool
	trap     bool
	log      *log
	got      *[]gift.DropInfo
	children []gift.View
	offsets  []geom.Point
}

func (dropArea) ViewType() gift.TypeID { return dropAreaType }

func (d dropArea) Build(*gift.BuildContext) gift.Element {
	n := &dropAreaNode{d: d}
	e := gift.Element{
		Key:       d.name,
		Layouter:  &frameLayout{f: frameView{w: d.w, h: d.h, offsets: d.offsets}},
		Children:  d.children,
		Disabled:  d.disabled,
		Hidden:    d.hidden,
		FocusTrap: d.trap,
	}
	if d.accepts != 0 || d.declines {
		e.DropTarget = n
	}
	if d.hover {
		e.Interactor = hoverOnly{}
	}
	return e
}

type hoverOnly struct{}

func (hoverOnly) HandleEvent(*gift.EventContext, gift.Event) bool { return false }

type dropAreaNode struct{ d dropArea }

func (n *dropAreaNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	info, ok := ctx.Drop()
	if !ok || !e.IsDrag() {
		panic("a drop target got " + kindName(e.Kind) + " without drop info")
	}
	if n.d.log != nil {
		n.d.log.add(n.d.name + ":" + dragName(e.Kind))
	}
	switch e.Kind {
	case gift.EventDragEnter, gift.EventDragOver:
		return info.Kinds == 0 || info.Kinds&n.d.accepts != 0
	case gift.EventDrop:
		if n.d.declines || info.Kinds&n.d.accepts == 0 {
			return false
		}
		if n.d.got != nil {
			*n.d.got = append(*n.d.got, info)
		}
		return true
	}
	return false
}

func dragName(k gift.EventKind) string {
	switch k {
	case gift.EventDragEnter:
		return "enter"
	case gift.EventDragOver:
		return "over"
	case gift.EventDragLeave:
		return "leave"
	case gift.EventDrop:
		return "drop"
	}
	return "?"
}

func (l *log) String() string { return strings.Join(l.seen, " ") }

func dragOver(a *gift.App, p geom.Point, k gift.DropKind) {
	a.BeginInput(0)
	a.DragOver(p, k)
}

func drop(a *gift.App, p geom.Point, d gift.DropData) bool {
	a.BeginInput(0)
	return a.Drop(p, d)
}

func wantLog(t *testing.T, l *log, want string) {
	t.Helper()
	if got := l.String(); got != want {
		t.Errorf("events %q, want %q", got, want)
	}
	l.reset()
}

// nested is an outer file target over the whole 400 by 300 window with an
// inner text target at (100, 100), 100 by 50.
func nested(l *log, got *[]gift.DropInfo) func(*gift.Context) gift.View {
	return func(*gift.Context) gift.View {
		return dropArea{name: "outer", w: 800, h: 600, accepts: gift.DropFile, log: l, got: got,
			offsets: []geom.Point{at(100, 100)},
			children: []gift.View{
				dropArea{name: "inner", w: 100, h: 50, accepts: gift.DropText, log: l, got: got},
			}}
	}
}

func TestTheInnermostAcceptingDropTargetWins(t *testing.T) {
	var l log
	var got []gift.DropInfo
	a := newInputApp(t, nested(&l, &got))

	dragOver(a, at(120, 120), gift.DropText)
	wantLog(t, &l, "inner:enter")
	if !a.DropAccepted() {
		t.Error("DropAccepted is false over an accepting target")
	}
	// The same tick again: nothing changed, nothing is said.
	dragOver(a, at(120, 120), gift.DropText)
	wantLog(t, &l, "")
	dragOver(a, at(130, 120), gift.DropText)
	wantLog(t, &l, "inner:over")

	if !drop(a, at(130, 120), gift.DropTextData("hi")) {
		t.Fatal("the drop was not taken")
	}
	wantLog(t, &l, "inner:drop inner:leave")
	if len(got) != 1 || got[0].Data.Items[0].Text != "hi" || got[0].Kinds != gift.DropText {
		t.Errorf("delivered %+v", got)
	}
	if a.DropAccepted() {
		t.Error("the session survived its drop")
	}
}

func TestADeclinedDragGoesToTheNextTargetAbove(t *testing.T) {
	var l log
	var got []gift.DropInfo
	a := newInputApp(t, nested(&l, &got))

	// Files over the text target: it declines, the outer one takes them.
	dragOver(a, at(120, 120), gift.DropFile)
	wantLog(t, &l, "inner:enter outer:enter")
	// Leaving the inner area: the outer target stays and hears an over.
	dragOver(a, at(20, 20), gift.DropFile)
	wantLog(t, &l, "outer:over")
	a.BeginInput(0)
	a.DragLeave()
	wantLog(t, &l, "outer:leave")
	// A second leave is nothing.
	a.BeginInput(0)
	a.DragLeave()
	wantLog(t, &l, "")
}

func TestAnAncestorThatTakesADeclinedDropHearsOnlyTheDrop(t *testing.T) {
	var l log
	var got []gift.DropInfo
	root := func(*gift.Context) gift.View {
		return dropArea{name: "outer", w: 800, h: 600, accepts: gift.DropFile, log: &l, got: &got,
			offsets: []geom.Point{at(100, 100)},
			children: []gift.View{
				dropArea{name: "inner", w: 100, h: 50, accepts: gift.DropFile, declines: true, log: &l},
			}}
	}
	a := newInputApp(t, root)
	dragOver(a, at(120, 120), gift.DropFile)
	wantLog(t, &l, "inner:enter")
	if !drop(a, at(120, 120), gift.DropFiles("/tmp/x")) {
		t.Fatal("nobody took the drop")
	}
	wantLog(t, &l, "inner:drop outer:drop inner:leave")
	if len(got) != 1 || got[0].Data.Files()[0] != "/tmp/x" {
		t.Errorf("outer got %+v", got)
	}
}

func TestADropWithoutASessionOpensOne(t *testing.T) {
	var l log
	var got []gift.DropInfo
	a := newInputApp(t, nested(&l, &got))
	if !drop(a, at(10, 10), gift.DropFiles("/a", "/b")) {
		t.Fatal("not taken")
	}
	wantLog(t, &l, "outer:enter outer:drop outer:leave")
	if f := got[0].Data.Files(); len(f) != 2 || f[1] != "/b" {
		t.Errorf("files %v", f)
	}
	// Text where only files are taken: nobody.
	if drop(a, at(10, 10), gift.DropTextData("x")) {
		t.Error("the file target took text")
	}
	wantLog(t, &l, "outer:enter")
}

func TestADropOutsideTheWindowGoesToTheWindowWideTarget(t *testing.T) {
	var l log
	var got []gift.DropInfo
	a := newInputApp(t, nested(&l, &got))
	if !drop(a, at(-50, 900), gift.DropFiles("/a")) {
		t.Fatal("the window-wide target did not get the drop")
	}
	wantLog(t, &l, "outer:enter outer:drop outer:leave")

	// Without a target covering the window the drop is discarded.
	var l2 log
	small := func(*gift.Context) gift.View {
		return frameView{w: 800, h: 600, children: []gift.View{
			dropArea{name: "small", w: 100, h: 100, accepts: gift.DropFile, log: &l2},
		}}
	}
	b := newInputApp(t, small)
	if drop(b, at(-50, 900), gift.DropFiles("/a")) {
		t.Error("a drop outside the window reached a target that does not cover it")
	}
	wantLog(t, &l2, "")
}

func TestAModalBoundsTheDropTargets(t *testing.T) {
	var l log
	root := func(*gift.Context) gift.View {
		return dropArea{name: "outer", w: 800, h: 600, accepts: gift.DropFile, log: &l,
			children: []gift.View{
				// A sheet with a focus trap and a scrim that swallows the pointer.
				dropArea{name: "sheet", w: 800, h: 600, trap: true, hover: true},
			}}
	}
	a := newInputApp(t, root)
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "")
	if drop(a, at(50, 50), gift.DropFiles("/a")) {
		t.Error("a drop behind a modal reached the target under it")
	}
	wantLog(t, &l, "")
}

func TestAHiddenOrDisabledTargetHearsTheDragLeave(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*dropArea)
	}{
		{"hidden", func(d *dropArea) { d.hidden = true }},
		{"disabled", func(d *dropArea) { d.disabled = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var l log
			off := false
			root := func(*gift.Context) gift.View {
				d := dropArea{name: "t", w: 100, h: 100, accepts: gift.DropFile, log: &l}
				if off {
					tc.set(&d)
				}
				return frameView{w: 800, h: 600, children: []gift.View{d}}
			}
			a := newInputApp(t, root)
			dragOver(a, at(50, 50), gift.DropFile)
			wantLog(t, &l, "t:enter")
			off = true
			a.InvalidateAll()
			mustUpdate(t, a)
			wantLog(t, &l, "t:leave")
			if a.DropAccepted() {
				t.Error("the session still has a target")
			}
			dragOver(a, at(51, 50), gift.DropFile)
			a.BeginInput(0)
			a.DragLeave()
			wantLog(t, &l, "")
		})
	}
}

func TestAnUnmountedTargetHearsNothing(t *testing.T) {
	var l log
	show := true
	root := func(*gift.Context) gift.View {
		kids := []gift.View{}
		if show {
			kids = append(kids, dropArea{name: "t", w: 100, h: 100, accepts: gift.DropFile, log: &l})
		}
		return frameView{w: 800, h: 600, children: kids}
	}
	a := newInputApp(t, root)
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "t:enter")
	if !a.HasDropTargets() {
		t.Error("HasDropTargets is false with a target mounted")
	}
	show = false
	a.InvalidateAll()
	mustUpdate(t, a)
	a.BeginInput(0)
	a.DragLeave()
	wantLog(t, &l, "")
	if a.HasDropTargets() {
		t.Error("HasDropTargets is true after the only target went")
	}
}

func TestTheHoverRestsDuringALiveDragOnly(t *testing.T) {
	var l log
	root := func(*gift.Context) gift.View {
		return frameView{w: 800, h: 600, children: []gift.View{
			dropArea{name: "t", w: 100, h: 100, accepts: gift.DropFile, hover: true, log: &l},
		}}
	}
	a := newInputApp(t, root)
	ref, _ := a.HitTest(at(50, 50))
	a.BeginInput(0)
	a.PointerMove(gift.MousePointer, gift.PointerMouse, at(50, 50))
	if !a.NodeInteraction(ref).Hover {
		t.Fatal("no hover to begin with")
	}
	dragOver(a, at(50, 50), gift.DropFile)
	if a.NodeInteraction(ref).Hover {
		t.Error("the hover survived the start of a live drag")
	}
	if !a.NodeDropTargeted(ref) {
		t.Error("the target is not DropTargeted")
	}
	a.BeginInput(0)
	a.PointerMove(gift.MousePointer, gift.PointerMouse, at(52, 50))
	if a.NodeInteraction(ref).Hover {
		t.Error("a move during the drag brought the hover back")
	}
	a.BeginInput(0)
	a.DragLeave()
	if a.NodeDropTargeted(ref) {
		t.Error("DropTargeted survived the leave")
	}
	a.BeginInput(0)
	a.PointerMove(gift.MousePointer, gift.PointerMouse, at(53, 50))
	if !a.NodeInteraction(ref).Hover {
		t.Error("the first move after the drag did not bring the hover back")
	}

	// A bare drop leaves the hover alone.
	drop(a, at(53, 50), gift.DropFiles("/a"))
	if !a.NodeInteraction(ref).Hover {
		t.Error("a drop without a session cleared the hover")
	}
}

func TestADropTargetWithoutInteractorLeavesThePointerToItsParent(t *testing.T) {
	var l log
	root := func(*gift.Context) gift.View {
		return dropArea{name: "parent", w: 800, h: 600, hover: true, children: []gift.View{
			dropArea{name: "t", w: 100, h: 100, accepts: gift.DropFile, log: &l},
		}}
	}
	a := newInputApp(t, root)
	ref, ok := a.HitTest(at(50, 50))
	if !ok || a.NodeKey(ref) != "parent" {
		t.Fatalf("the pointer hit %q, want the parent through the drop target", a.NodeKey(ref))
	}
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "t:enter")
}

func TestScrollingUnderAStandingDragChangesTheTarget(t *testing.T) {
	var l log
	x := float32(0)
	root := func(*gift.Context) gift.View {
		return frameView{w: 800, h: 600, offsets: []geom.Point{at(x, 0), at(x+100, 0)}, children: []gift.View{
			dropArea{name: "a", w: 100, h: 100, accepts: gift.DropFile, log: &l},
			dropArea{name: "b", w: 100, h: 100, accepts: gift.DropFile, log: &l},
		}}
	}
	a := newInputApp(t, root)
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "a:enter")
	x = -100
	a.InvalidateAll()
	mustUpdate(t, a)
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "b:enter a:leave")
}

func TestAPopoverIsNoDropTarget(t *testing.T) {
	var l log
	presses := 0
	var seen geom.Rect
	root := func(*gift.Context) gift.View {
		return dropArea{name: "outer", w: 800, h: 600, accepts: gift.DropFile, log: &l,
			children: []gift.View{presenter{open: true, sheet: overlaySheet{seen: &seen, presses: &presses}}}}
	}
	a := newInputApp(t, root)
	dragOver(a, at(300, 300), gift.DropFile)
	wantLog(t, &l, "")
}

func TestDragOverDoesNotAllocate(t *testing.T) {
	x := float32(0)
	quiet := newInputApp(t, nested(nil, nil))
	dragOver(quiet, at(120, 120), gift.DropFile)
	allocs := testing.AllocsPerRun(200, func() {
		x++
		quiet.BeginInput(0)
		quiet.DragOver(at(110+float32(int(x)%20), 120), gift.DropFile)
	})
	if allocs != 0 {
		t.Errorf("DragOver allocates %v times per tick", allocs)
	}
}

func TestDropCapabilitiesReachTheBuild(t *testing.T) {
	var seen gift.DropCapabilities
	a := gift.New(gift.Options{Root: func(ctx *gift.Context) gift.View {
		seen = ctx.DropCapabilities()
		return frameView{w: 10, h: 10}
	}})
	mustUpdate(t, a)
	if seen != (gift.DropCapabilities{}) {
		t.Errorf("before any backend said anything: %+v", seen)
	}
	c := gift.DropCapabilities{Kinds: gift.DropFile | gift.DropText, Live: true}
	a.SetDropCapabilities(c)
	mustUpdate(t, a)
	if seen != c {
		t.Errorf("after SetDropCapabilities: %+v, want %+v", seen, c)
	}
	var b *gift.BuildContext
	if b.DropCapabilities() != (gift.DropCapabilities{}) {
		t.Error("a nil BuildContext answered something")
	}
}

func TestDropKindString(t *testing.T) {
	if got := (gift.DropFile | gift.DropURL).String(); got != "file+url" {
		t.Errorf("String = %q", got)
	}
	if got := gift.DropKind(0).String(); got != "none" {
		t.Errorf("String = %q", got)
	}
}

func TestAModalOpenedUnderAStillDragTakesTheDropAway(t *testing.T) {
	var l log
	modal := false
	root := func(*gift.Context) gift.View {
		kids := []gift.View{}
		if modal {
			// Away from the drag, at (400, 300).
			kids = append(kids, dropArea{name: "sheet", w: 100, h: 100, trap: true, hover: true})
		}
		return dropArea{name: "outer", w: 800, h: 600, accepts: gift.DropFile, log: &l,
			offsets: []geom.Point{at(400, 300)}, children: kids}
	}
	a := newInputApp(t, root)
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "outer:enter")
	modal = true
	a.InvalidateAll()
	mustUpdate(t, a)
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "outer:leave")
	if a.DropAccepted() {
		t.Error("the target behind the modal still accepts")
	}
	if drop(a, at(50, 50), gift.DropFiles("/a")) {
		t.Error("the target behind the modal got the drop")
	}
}

func TestATargetEnabledUnderAStillDragIsOffered(t *testing.T) {
	var l log
	off := true
	root := func(*gift.Context) gift.View {
		return frameView{w: 800, h: 600, children: []gift.View{
			dropArea{name: "t", w: 100, h: 100, accepts: gift.DropFile, disabled: off, log: &l},
		}}
	}
	a := newInputApp(t, root)
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "")
	off = false
	a.InvalidateAll()
	mustUpdate(t, a)
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "t:enter")
	if !a.DropAccepted() {
		t.Error("the enabled target does not accept")
	}
}

func TestDropInfoSaysWhetherTheSessionWasLive(t *testing.T) {
	var lives []bool
	root := func(*gift.Context) gift.View {
		return liveProbe{lives: &lives}
	}
	a := newInputApp(t, root)
	drop(a, at(10, 10), gift.DropFiles("/a"))
	for _, l := range lives {
		if l {
			t.Fatalf("a bare drop reported a live session: %v", lives)
		}
	}
	lives = lives[:0]
	dragOver(a, at(10, 10), gift.DropFile)
	drop(a, at(10, 10), gift.DropFiles("/a"))
	for _, l := range lives {
		if !l {
			t.Fatalf("a live session reported a bare drop: %v", lives)
		}
	}
}

func TestADropOutsideTheWindowCarriesAPositionInsideIt(t *testing.T) {
	var pos []geom.Point
	a := newInputApp(t, func(*gift.Context) gift.View { return liveProbe{pos: &pos} })
	drop(a, at(-50, 900), gift.DropFiles("/a"))
	for _, p := range pos {
		if p.X < 0 || p.Y < 0 || p.X >= 800 || p.Y >= 600 {
			t.Errorf("an event carried %v, outside the window", p)
		}
	}
	if len(pos) == 0 {
		t.Error("no event")
	}
}

var liveProbeType = gift.RegisterType("test.LiveProbe")

// liveProbe is a window-wide file target recording DropInfo.Live and the
// positions, and which delivers a focus change from its drop handler to see
// EventContext.Drop stay out of it.
type liveProbe struct {
	lives *[]bool
	pos   *[]geom.Point
}

func (liveProbe) ViewType() gift.TypeID { return liveProbeType }

func (p liveProbe) Build(*gift.BuildContext) gift.Element {
	n := &liveProbeNode{p}
	return gift.Element{Layouter: &frameLayout{f: frameView{w: 800, h: 600}}, DropTarget: n, Interactor: n, Focusable: true}
}

type liveProbeNode struct{ p liveProbe }

func (n *liveProbeNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	info, ok := ctx.Drop()
	if e.IsDrag() != ok {
		panic("EventContext.Drop answered " + strings.Repeat("true", btoi(ok)) + " for " + kindName(e.Kind) + dragName(e.Kind))
	}
	if !ok {
		return false
	}
	if n.p.lives != nil {
		*n.p.lives = append(*n.p.lives, info.Live)
	}
	if n.p.pos != nil {
		*n.p.pos = append(*n.p.pos, e.Pos)
	}
	if e.Kind == gift.EventDrop {
		// A focus change from inside the drop: EventFocusGained arrives
		// here, nested, and must not see drag data.
		ctx.RequestFocus()
	}
	return true
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestARebuildUnderAStillDragDoesNotTellTheTargetItMoved(t *testing.T) {
	var l log
	n := 0
	root := func(*gift.Context) gift.View {
		n++
		return frameView{w: 800, h: 600, offsets: []geom.Point{at(0, 0), at(300, 0)}, children: []gift.View{
			dropArea{name: "a", w: 100, h: 100, accepts: gift.DropFile, log: &l},
			dropArea{name: "b", w: 100, h: 100, accepts: gift.DropFile, log: &l},
		}}
	}
	a := newInputApp(t, root)
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "a:enter")
	a.InvalidateAll()
	mustUpdate(t, a)
	dragOver(a, at(50, 50), gift.DropFile)
	wantLog(t, &l, "")
}
