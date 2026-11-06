package ui

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var dropDestinationType = gift.RegisterType("ui.DropDestination")

// DropInfo is what a [DropDestinationView] hands its OnDrop.
type DropInfo struct {
	// Items are the dropped things of the kinds the destination accepts, in
	// the order the source gave them. They belong to the receiver.
	Items []gift.DropItem
	// Location is where the drop happened, in logical pixels relative to the
	// top left corner of the destination.
	Location geom.Point
}

// Files returns the paths of the dropped files and folders.
func (d DropInfo) Files() []string {
	var out []string
	for _, it := range d.Items {
		if it.Kind == gift.DropFile {
			out = append(out, it.Path)
		}
	}
	return out
}

// DropDestinationView is content that takes files, text or web addresses
// dragged in from another application. It is created by [DropDestination].
type DropDestinationView struct {
	content     gift.View
	key         string
	flex        float32
	radius      float32
	accepts     gift.DropKind
	onDrop      func(DropInfo) bool
	onTargeted  func(bool)
	noHighlight bool
	disabled    bool
}

// DropDestination returns content that accepts what is dragged onto it from
// outside the application — a file from the Finder, the Explorer, Nautilus
// or Dolphin, and where the platform reports them, text and web addresses —
// the counterpart of SwiftUI's .dropDestination(for:action:isTargeted:).
//
// While a drag it would accept is over it, it shows a highlight: an accent
// border and a faint accent fill over the content. OnDrop receives what was
// dropped. A destination inside another one wins where both accept the drag;
// one that does not accept it, or whose OnDrop returns false, leaves it to
// the one around it. A destination around the root of the window takes drops
// anywhere.
//
// What arrives depends on the platform; see [gift.DropCapabilities], which
// [gift.BuildContext.DropCapabilities] reports. On macOS and Windows the
// highlight follows the drag and text and addresses arrive too. On Linux,
// under GNOME and KDE, files arrive with no highlight before the drop. On
// every other platform a destination is an ordinary view and nothing
// arrives. The content keeps its own hover and presses: a destination is not
// a hit target for the pointer.
func DropDestination(content gift.View) DropDestinationView {
	return DropDestinationView{content: content}
}

// Key sets the reconciliation key.
func (v DropDestinationView) Key(s string) DropDestinationView { v.key = s; return v }

// Flex sets the share of the free space of a stack this view takes.
func (v DropDestinationView) Flex(f float32) DropDestinationView { v.flex = f; return v }

// CornerRadius rounds the corners of the highlight, to match content with
// rounded corners.
func (v DropDestinationView) CornerRadius(r float32) DropDestinationView {
	v.radius = max(0, r)
	return v
}

// Accepts limits the destination to kinds. Zero, the default, accepts every
// kind.
func (v DropDestinationView) Accepts(kinds gift.DropKind) DropDestinationView {
	v.accepts = kinds
	return v
}

// OnDrop sets what happens with a drop. It receives only the items of the
// accepted kinds and returns whether it took them; false hands the drop to
// the destination around this one. It runs on the UI goroutine, where
// writing state is normal.
func (v DropDestinationView) OnDrop(fn func(DropInfo) bool) DropDestinationView {
	v.onDrop = fn
	return v
}

// OnTargeted is told true when a drag the destination accepts arrives over
// it and false when that drag leaves, is dropped or is given up, or the
// destination is hidden or disabled under it; the two come in pairs while
// the destination stays mounted. A destination removed from the tree during
// a drag is told nothing more, so what true started must not outlive it. It
// is never told anything where the platform does not report a drag before
// the drop.
func (v DropDestinationView) OnTargeted(fn func(bool)) DropDestinationView {
	v.onTargeted = fn
	return v
}

// Highlight switches the built-in highlight on, the default, or off, for a
// destination that shows the drag itself through OnTargeted.
func (v DropDestinationView) Highlight(on bool) DropDestinationView {
	v.noHighlight = !on
	return v
}

// Disabled takes the drop destination away; the content stays as it is.
func (v DropDestinationView) Disabled(d bool) DropDestinationView { v.disabled = d; return v }

// ViewType implements [gift.View].
func (DropDestinationView) ViewType() gift.TypeID { return dropDestinationType }

// Build implements [gift.View].
func (v DropDestinationView) Build(*gift.BuildContext) gift.Element {
	n := &dropNode{v: v}
	var kids []gift.View
	if v.content != nil {
		kids = []gift.View{v.content}
	}
	return gift.Element{
		Key:        v.key,
		Flex:       v.flex,
		Layouter:   anchorLayout{},
		Painter:    n,
		DropTarget: n,
		Disabled:   v.disabled,
		Children:   kids,
	}
}

type dropNode struct{ v DropDestinationView }

func (n *dropNode) accepts(k gift.DropKind) bool {
	return k == 0 || n.v.accepts == 0 || k&n.v.accepts != 0
}

func (n *dropNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	info, ok := ctx.Drop()
	if !ok {
		return false
	}
	switch e.Kind {
	case gift.EventDragEnter:
		if !n.accepts(info.Kinds) {
			return false
		}
		if n.v.onTargeted != nil && info.Live {
			n.v.onTargeted(true)
		}
		return true
	case gift.EventDragOver:
		return n.accepts(info.Kinds)
	case gift.EventDragLeave:
		if n.v.onTargeted != nil && info.Live {
			n.v.onTargeted(false)
		}
		return true
	case gift.EventDrop:
		if n.v.onDrop == nil {
			return false
		}
		d := DropInfo{Location: e.Pos.Sub(ctx.DeviceBounds().Min)}
		for _, it := range info.Data.Items {
			if n.v.accepts == 0 || it.Kind&n.v.accepts != 0 {
				d.Items = append(d.Items, it)
			}
		}
		if len(d.Items) == 0 {
			return false
		}
		return n.v.onDrop(d)
	}
	return false
}

// Paint draws the content and, while a drag is targeting the destination,
// the highlight over it.
func (n *dropNode) Paint(ctx *gift.PaintContext) {
	ctx.PaintChildren()
	if n.v.noHighlight || !ctx.Interaction().DropTargeted {
		return
	}
	b := ctx.Bounds()
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: ResolveColor(Fade(ColorAccent, 0.12)), CornerRadius: n.v.radius})
	ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: b.Inset(geom.InsetsAll(1)), Color: ResolveColor(ColorAccent), CornerRadius: max(0, n.v.radius-1), StrokeWidth: 2})
}
