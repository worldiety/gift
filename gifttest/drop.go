package gifttest

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// Drag and drop from outside the application, as a backend reports it; see
// [gift.App.DragOver], [gift.App.Drop] and the project plan, section 48.
//
// A drag is not a pointer: nothing is pressed, nothing is captured, and the
// mouse hover rests while it lasts. These actions therefore take kinds and
// data rather than buttons, and they perform no aim check, because what a
// drag reaches is decided by the drop targets above the hit and not by the
// hit itself.

// DragOverAt reports a drag carrying kinds over p, in one tick, and settles.
// The first call opens a live session, the way gift's backend opens one on
// macOS and Windows.
func (h *Harness) DragOverAt(p geom.Point, kinds gift.DropKind) {
	h.t.Helper()
	h.beginInput()
	h.app.DragOver(p, kinds)
	h.Settle()
}

// DragLeave ends the drag session without a drop: the drag left the window.
func (h *Harness) DragLeave() {
	h.t.Helper()
	h.beginInput()
	h.app.DragLeave()
	h.Settle()
}

// DropAt drops d at p and reports whether a target took it. Without a
// session open it is the drop of a platform that reports nothing before it,
// as on Linux.
func (h *Harness) DropAt(p geom.Point, d gift.DropData) bool {
	h.t.Helper()
	h.beginInput()
	ok := h.app.Drop(p, d)
	h.Settle()
	return ok
}

// DragOver drags kinds over the centre of the node; see
// [Harness.DragOverAt].
func (n Node) DragOver(kinds gift.DropKind) Node {
	n.h.t.Helper()
	n.check("DragOver")
	n.h.DragOverAt(n.Center(), kinds)
	return n
}

// DropOnto drops d on the centre of the node and reports whether a target
// took it; see [Harness.DropAt].
func (n Node) DropOnto(d gift.DropData) bool {
	n.h.t.Helper()
	n.check("DropOnto")
	return n.h.DropAt(n.Center(), d)
}

// AssertDropTargeted fails when the drop target the node belongs to — the
// node itself or its nearest ancestor that declares [gift.Element.DropTarget]
// — is not the accepting target of the current drag.
func (n Node) AssertDropTargeted() Node {
	n.h.t.Helper()
	return n.assertDropTargeted(true)
}

// AssertNotDropTargeted is the opposite of [Node.AssertDropTargeted].
func (n Node) AssertNotDropTargeted() Node {
	n.h.t.Helper()
	return n.assertDropTargeted(false)
}

func (n Node) assertDropTargeted(want bool) Node {
	n.h.t.Helper()
	n.check("AssertDropTargeted")
	t := n
	for cur := n.ref; !cur.IsZero(); cur = n.h.app.NodeParent(cur) {
		if n.h.app.NodeIsDropTarget(cur) {
			t = Node{h: n.h, ref: cur}
			break
		}
	}
	if got := n.h.app.NodeDropTargeted(t.ref); got != want {
		desc := n.describe()
		if t.ref != n.ref {
			desc += "\n  drop target " + t.describe()
		}
		n.h.t.Errorf("gifttest: %s\n  want drop targeted=%v, got %v", desc, want, got)
	}
	return n
}
