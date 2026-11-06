package gift

import (
	"os"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/scene"
)

// This file is the core half of dropping content from outside the
// application: files from the Finder, the Explorer, Nautilus or Dolphin, and
// where the platform reports it, text and web addresses. The project plan,
// section 49, is the design; the short version is below.
//
// # The session
//
// A drag that comes in from another application is a *session*: it enters
// the window, moves over it, and ends either with a drop or by leaving. A
// backend whose platform reports the whole session — gift's Ebitengine backend
// on macOS and Windows — calls [App.DragOver] in every tick of it and
// [App.DragLeave] when it ends. A backend that only learns of the drop itself
// — the same backend on Linux, where Ebitengine reports nothing before the
// drop — calls [App.Drop] alone. Both reach the same targets with the same
// rules, and every session a target accepted ends with [EventDragLeave],
// including the one that ended in a drop; an ancestor that takes a drop the
// target declined hears [EventDrop] alone.
//
// # The target
//
// A node takes part by declaring [Element.DropTarget], an [Interactor] of its
// own that receives only [EventDragEnter], [EventDragOver], [EventDragLeave]
// and [EventDrop]. It is separate from [Element.Interactor] on purpose: a drop
// area around a list must not swallow the hover and the presses meant for the
// list, and a node whose Interactor is nil stays transparent to the pointer
// whatever its DropTarget says.
//
// The target is the innermost visible, enabled one under the position that
// accepts what is being dragged; one that declines hands the drag to the next
// one above it. An active [Element.FocusTrap] bounds the search to its own
// subtree, so a modal sheet over a window-wide drop area takes the drop away
// from it, and the top layer of popovers and menus is never a target at all.

// DropKind is a set of the kinds of content a drag carries.
type DropKind uint8

const (
	// DropFile is a file or a folder, delivered as an absolute path.
	DropFile DropKind = 1 << iota
	// DropText is plain text.
	DropText
	// DropURL is a web address. A file: address is a [DropFile] and never
	// arrives as one.
	DropURL
)

// Has reports whether every kind in k is in s.
func (s DropKind) Has(k DropKind) bool { return s&k == k }

// String names the kinds, joined by "+", or "none".
func (s DropKind) String() string {
	if s == 0 {
		return "none"
	}
	out := ""
	for _, k := range [...]struct {
		k DropKind
		n string
	}{{DropFile, "file"}, {DropText, "text"}, {DropURL, "url"}} {
		if s&k.k != 0 {
			if out != "" {
				out += "+"
			}
			out += k.n
		}
	}
	return out
}

// DropItem is one dropped thing. Exactly one of Path, Text and URL is
// meaningful, the one Kind names.
type DropItem struct {
	Kind DropKind
	// Path is the absolute path of a [DropFile]. gift does not open it.
	Path string
	// Dir says that Path is a folder.
	Dir bool
	// Text is the text of a [DropText].
	Text string
	// URL is the address of a [DropURL], as the source spelled it.
	URL string
}

// DropData is what a drop delivered. Its Items belong to the receiver of
// [EventDrop], who may keep them.
type DropData struct{ Items []DropItem }

// Kinds is the set of kinds among the items.
func (d DropData) Kinds() DropKind {
	var k DropKind
	for _, it := range d.Items {
		k |= it.Kind
	}
	return k
}

// Files returns the paths of the [DropFile] items, in order.
func (d DropData) Files() []string {
	var out []string
	for _, it := range d.Items {
		if it.Kind == DropFile {
			out = append(out, it.Path)
		}
	}
	return out
}

// DropFiles is a [DropData] of files, for tests and demos. Dir is set from
// [os.Stat]; a path that cannot be stated counts as a file.
func DropFiles(paths ...string) DropData {
	d := DropData{Items: make([]DropItem, 0, len(paths))}
	for _, p := range paths {
		it := DropItem{Kind: DropFile, Path: p}
		if fi, err := os.Stat(p); err == nil {
			it.Dir = fi.IsDir()
		}
		d.Items = append(d.Items, it)
	}
	return d
}

// DropTextData is a [DropData] of one piece of text.
func DropTextData(s string) DropData {
	return DropData{Items: []DropItem{{Kind: DropText, Text: s}}}
}

// DropURLData is a [DropData] of web addresses.
func DropURLData(urls ...string) DropData {
	d := DropData{Items: make([]DropItem, 0, len(urls))}
	for _, u := range urls {
		d.Items = append(d.Items, DropItem{Kind: DropURL, URL: u})
	}
	return d
}

// DropInfo is what [EventDragEnter], [EventDragOver] and [EventDrop] carry;
// see [EventContext.Drop].
type DropInfo struct {
	// Kinds is, during the drag, the kinds the platform reported, and zero
	// when it reported none — a platform that only knows at the drop. For
	// [EventDrop] it is Data.Kinds().
	Kinds DropKind
	// Data is the dropped content, for [EventDrop] only.
	Data DropData
	// Live says the platform reported the drag before the drop, so the
	// enter and the leave are real; without it — a drop on Linux — gift
	// makes them up around the drop to let the target decide on the kinds,
	// and a target that shows something on enter should not.
	Live bool
}

// DropCapabilities is what the platform under the backend can do. The zero
// value is nothing: no drops reach this application.
type DropCapabilities struct {
	// Kinds are the kinds that can be dropped.
	Kinds DropKind
	// Live says that a drag is reported while it moves over the window —
	// [EventDragEnter], [EventDragOver] and [EventDragLeave] before the drop
	// — so that a target can show that it would accept it. Without it a
	// target only ever hears the drop.
	Live bool
}

// dragState is the one drag session the dispatcher tracks.
type dragState struct {
	// active is set from the first [App.DragOver] or [App.Drop] to the
	// [App.DragLeave] that ends the session.
	active bool
	// live says the session was opened by [App.DragOver] and not by a bare
	// drop; only a live session suspends the hover.
	live bool
	pos  geom.Point
	// kinds are the kinds of the latest [App.DragOver].
	kinds DropKind
	// hit is the raw hit of the latest walk, so that a tick in which neither
	// the position, the kinds nor the node under them changed walks nothing.
	hit scene.Handle
	// target is the node that accepted the drag, zero while nobody does.
	target scene.Handle
	// walked says that hit and target describe the current position.
	walked bool
	// rewalk asks the next tick to search again although the drag did not
	// move: a target appeared, was enabled, or a focus trap changed. Unlike
	// a cleared walked it does not tell the current target the drag moved.
	rewalk bool

	// delivering and info are what [EventContext.Drop] answers from.
	delivering bool
	info       DropInfo
}

// DragOver reports that a drag from outside the application is over the
// window at pos, in device space, carrying kinds — zero when the platform
// does not say. A backend calls it in every tick of a live session; the
// first call opens the session.
//
// The node under pos is walked up to the first [Element.DropTarget] that
// accepts: the target hears [EventDragEnter] once and [EventDragOver] when the
// position or the kinds change, and the target it replaces hears
// [EventDragLeave]. [Interaction.DropTargeted] follows. While the session
// lasts, the mouse hover is suspended, because the cursor is carrying
// something and the buttons under it are not what it is pointing at.
func (a *App) DragOver(pos geom.Point, kinds DropKind) {
	a.assertInputPhase("DragOver")
	d := &a.in.drag
	if !d.active || !d.live {
		d.active, d.live = true, true
		a.clearHover(a.pointerFor(MousePointer, PointerMouse, false))
	}
	a.dragOver(pos, kinds)
}

// dragOver moves the session to pos and kinds and retargets it.
func (a *App) dragOver(pos geom.Point, kinds DropKind) {
	d := &a.in.drag
	changed := !d.walked || pos != d.pos || kinds != d.kinds
	rewalk := d.rewalk
	d.rewalk = false
	d.pos, d.kinds = pos, kinds
	hit := a.dragHit(pos)
	if !changed && !rewalk && hit == d.hit {
		return
	}
	d.hit, d.walked = hit, true
	info := DropInfo{Kinds: kinds}
	next := scene.Handle{}
	limit := a.dragLimit()
	for h := a.nextDropTarget(hit, limit, 0); !h.IsZero(); h = a.nextDropTarget(a.store.Get(h).Parent, limit, 0) {
		if h == d.target {
			if !changed || a.deliverDrag(h, EventDragOver, info) {
				next = h
				break
			}
			continue
		}
		if a.deliverDrag(h, EventDragEnter, info) {
			next = h
			break
		}
	}
	a.setDropTarget(next)
}

// setDropTarget makes h the accepting target, telling the previous one that
// the drag left it.
func (a *App) setDropTarget(h scene.Handle) {
	d := &a.in.drag
	prev := d.target
	if prev == h {
		return
	}
	d.target = h
	if a.store.Valid(prev) {
		a.setDropTargeted(prev, false)
		a.deliverDrag(prev, EventDragLeave, DropInfo{Kinds: d.kinds, Live: d.live})
	}
	if !h.IsZero() {
		a.setDropTargeted(h, true)
	}
}

func (a *App) setDropTargeted(h scene.Handle, v bool) {
	if !a.store.Valid(h) {
		return
	}
	nd := a.data(h)
	if nd.ia.DropTargeted == v {
		return
	}
	nd.ia.DropTargeted = v
	a.markNeedsPaint(h)
}

// DragLeave ends the drag session: the drag left the window or was given up.
// The target hears [EventDragLeave]. Without a session it does nothing.
func (a *App) DragLeave() {
	a.assertInputPhase("DragLeave")
	a.endDrag()
}

func (a *App) endDrag() {
	d := &a.in.drag
	if !d.active {
		return
	}
	a.setDropTarget(scene.Handle{})
	*d = dragState{}
}

// Drop reports that content was dropped at pos, in device space, and
// reports whether a target took it.
//
// Without a session — a platform that reports nothing before the drop — the
// drop opens one first, so the target hears [EventDragEnter] and decides on
// the kinds as it would during a drag. [EventDrop] then goes to the target;
// when it declines, to the next target above it, which hears the drop and
// nothing else. The session always ends with [EventDragLeave] afterwards.
//
// A position outside the window — a stale cursor under XWayland — sends the
// drop to the outermost target that covers the whole window, if there is
// one; that is where a drop the user aimed at the window most plausibly
// belongs.
func (a *App) Drop(pos geom.Point, data DropData) bool {
	a.assertInputPhase("Drop")
	d := &a.in.drag
	kinds := data.Kinds()
	if !d.active {
		d.active = true
	}
	limit := a.dragLimit()
	if !a.insideWindow(pos) {
		w := a.windowDropTarget(limit)
		if w.IsZero() {
			a.debug("gift: a drop outside the window found no window-wide target and was discarded", "x", pos.X, "y", pos.Y)
			a.endDrag()
			return false
		}
		// The stale position is replaced by the nearest point inside the
		// window, which the window-wide target covers, so that its events
		// carry a position it contains.
		win := a.store.Get(a.root.node).Bounds
		p := geom.Pt(min(max(pos.X, win.Min.X), win.Max.X-1), min(max(pos.Y, win.Min.Y), win.Max.Y-1))
		d.pos, d.kinds, d.walked = p, kinds, true
		info := DropInfo{Kinds: kinds}
		if w != d.target && !a.deliverDrag(w, EventDragEnter, info) {
			a.endDrag()
			return false
		}
		a.setDropTarget(w)
	} else {
		a.dragOver(pos, kinds)
	}
	accepted := false
	info := DropInfo{Kinds: kinds, Data: data}
	start := d.target
	if !start.IsZero() && !limit.IsZero() && start != limit && !a.isAncestor(limit, start) {
		// A modal opened under a drag that did not move since.
		a.setDropTarget(scene.Handle{})
		start = scene.Handle{}
	}
	for h := start; !h.IsZero(); h = a.nextDropTarget(a.store.Get(h).Parent, limit, 0) {
		if a.deliverDrag(h, EventDrop, info) {
			accepted = true
			break
		}
	}
	a.endDrag()
	return accepted
}

// DropAccepted reports whether the drag session currently has a target that
// accepts it. A backend answers the platform's question "would a drop here be
// taken" with it.
func (a *App) DropAccepted() bool {
	d := &a.in.drag
	return d.active && a.store.Valid(d.target)
}

// HasDropTargets reports whether a mounted, enabled node declares
// [Element.DropTarget]. A backend asks before a session exists, to decide
// whether the window wants drags at all.
func (a *App) HasDropTargets() bool { return a.dropTargets > 0 }

// SetDropCapabilities tells the application what the platform can drop; see
// [BuildContext.DropCapabilities]. A backend calls it once it knows. A change
// rebuilds every view, because the answer is read in builds.
func (a *App) SetDropCapabilities(c DropCapabilities) {
	a.assertUIGoroutine("SetDropCapabilities")
	if a.dropCaps == c {
		return
	}
	a.dropCaps = c
	a.InvalidateAll()
}

// DropCapabilities is what [App.SetDropCapabilities] last said.
func (a *App) DropCapabilities() DropCapabilities { return a.dropCaps }

// NodeDropTargeted reports whether r is the current, accepting target of a
// drag session; see [Interaction.DropTargeted].
func (a *App) NodeDropTargeted(r NodeRef) bool {
	if !a.store.Valid(r.h) {
		return false
	}
	return a.data(r.h).ia.DropTargeted
}

// DropCapabilities reports what the platform can drop into this
// application. A nil BuildContext answers the zero value, like
// [BuildContext.SoftKeyboardRequested].
func (b *BuildContext) DropCapabilities() DropCapabilities {
	if b == nil || b.app == nil {
		return DropCapabilities{}
	}
	return b.app.dropCaps
}

// DropCapabilities reports what the platform can drop into this
// application; see [BuildContext.DropCapabilities].
func (c *Context) DropCapabilities() DropCapabilities {
	if c == nil || c.app == nil {
		return DropCapabilities{}
	}
	return c.app.dropCaps
}

// Drop returns what the drag event being handled carries, and false for
// every other event.
func (c *EventContext) Drop() (DropInfo, bool) {
	d := &c.app.in.drag
	if !d.delivering {
		return DropInfo{}, false
	}
	return d.info, true
}

// deliverDrag hands a drag event to the drop target of h.
func (a *App) deliverDrag(h scene.Handle, k EventKind, info DropInfo) bool {
	if !a.store.Valid(h) {
		return false
	}
	return a.deliverDragTo(h, a.data(h).dropTarget, k, info)
}

// deliverDragTo hands a drag event to t as the drop target of h: the one
// that heard the enter, when the node has another one by now.
func (a *App) deliverDragTo(h scene.Handle, t Interactor, k EventKind, info DropInfo) bool {
	if t == nil || !a.store.Valid(h) {
		return false
	}
	nd := a.data(h)
	d := &a.in.drag
	info.Live = info.Live || d.live
	e := Event{Kind: k, Pointer: MousePointer, Device: PointerMouse, Pos: d.pos, Inside: true, Mods: a.in.mods, Time: a.in.now}
	a.diag.InputEvents++
	c := &a.in.ectx
	prevH, prevD := c.cur, c.nd
	prevDel, prevInfo := d.delivering, d.info
	c.cur, c.nd = h, nd
	d.delivering, d.info = true, info
	handled := t.HandleEvent(c, e)
	d.delivering, d.info = prevDel, prevInfo
	c.cur, c.nd = prevH, prevD
	return handled
}

// dragHit is the hit test of a drag: like the pointer's, but a node with a
// drop target and no interactor is a hit target too.
func (a *App) dragHit(p geom.Point) scene.Handle {
	a.diag.HitTests++
	if a.root == nil || !a.store.Valid(a.root.node) {
		return scene.Handle{}
	}
	return a.hitNodeFor(a.root.node, p, unboundedClip, geom.Identity(), 0, true)
}

// dragLimit is the node the search for a target stops at: the active focus
// trap, or the root.
func (a *App) dragLimit() scene.Handle {
	if a.root == nil || !a.store.Valid(a.root.node) {
		return scene.Handle{}
	}
	return a.focusRoot()
}

// nextDropTarget walks up from h to the first enabled drop target inside
// limit. It returns zero when the walk leaves limit, enters the top layer, or
// finds nothing.
func (a *App) nextDropTarget(h, limit scene.Handle, depth int) scene.Handle {
	for ; !h.IsZero() && a.store.Valid(h); depth++ {
		if depth > scene.MaxDepth {
			return scene.Handle{}
		}
		n := a.store.Get(h)
		nd := &n.Payload
		if _, ok := nd.layouter.(*overlayHostNode); ok {
			// A popover or a menu: it covers what is under it and takes
			// nothing.
			return scene.Handle{}
		}
		if nd.dropTarget != nil && !nd.disabled {
			if limit.IsZero() || h == limit || a.isAncestor(limit, h) {
				return h
			}
			return scene.Handle{}
		}
		if h == limit {
			return scene.Handle{}
		}
		h = n.Parent
	}
	return scene.Handle{}
}

// insideWindow reports whether p lies in the root's bounds.
func (a *App) insideWindow(p geom.Point) bool {
	if a.root == nil || !a.store.Valid(a.root.node) {
		return false
	}
	return a.store.Get(a.root.node).Bounds.Contains(p)
}

// windowDropTarget is the shallowest visible, enabled drop target inside
// limit whose device bounds cover the whole window.
func (a *App) windowDropTarget(limit scene.Handle) scene.Handle {
	if limit.IsZero() {
		return scene.Handle{}
	}
	win := a.store.Get(a.root.node).Bounds
	best, bestDepth := scene.Handle{}, scene.MaxDepth+1
	a.findWindowTarget(limit, win, 0, &best, &bestDepth)
	return best
}

func (a *App) findWindowTarget(h scene.Handle, win geom.Rect, depth int, best *scene.Handle, bestDepth *int) {
	if depth >= *bestDepth || depth > scene.MaxDepth || !a.store.Valid(h) {
		return
	}
	nd := a.data(h)
	if nd.hidden {
		return
	}
	if _, ok := nd.layouter.(*overlayHostNode); ok {
		return
	}
	if nd.dropTarget != nil && !nd.disabled {
		if _, m, ok := a.deviceSpace(h); ok {
			b := m.TransformRect(a.store.Get(h).Bounds)
			if b.Min.X <= win.Min.X && b.Min.Y <= win.Min.Y && b.Max.X >= win.Max.X && b.Max.Y >= win.Max.Y {
				*best, *bestDepth = h, depth
				return
			}
		}
	}
	for _, c := range nd.children {
		a.findWindowTarget(c, win, depth+1, best, bestDepth)
	}
}

// applyDropTarget writes [Element.DropTarget] and [Element.Disabled] onto the
// node's drop state: the counter behind [App.HasDropTargets], and a target
// that stops being one in the middle of a session.
func (a *App) applyDropTarget(h scene.Handle, nd *nodeData, t Interactor, disabled bool) {
	was := nd.dropTarget != nil && !nd.disabled
	now := t != nil && !disabled
	old := nd.dropTarget
	nd.dropTarget = t
	switch {
	case now && !was:
		a.countDropTarget(1)
	case !now && was:
		a.countDropTarget(-1)
	}
	d := &a.in.drag
	if !now && d.target == h {
		a.loseDropTarget(h, old)
		return
	}
	if now && d.active && h != d.target {
		// A target that appeared, was enabled or may accept something else
		// now, under a drag that does not move: the next tick asks again.
		d.rewalk = true
	}
}

func (a *App) countDropTarget(n int) {
	a.dropTargets += n
	if n > 0 && a.dropTargets == 1 && a.dropCaps == (DropCapabilities{}) && !a.dropWarned {
		a.dropWarned = true
		a.debug("gift: a drop target was built, but this platform drops nothing into the application; it stays an ordinary view")
	}
}

// loseDropTarget takes the session's target away because the node was
// hidden or disabled: it loses its highlight now and hears that the drag left
// it once the build is over.
func (a *App) loseDropTarget(h scene.Handle, heard Interactor) {
	d := &a.in.drag
	d.target = scene.Handle{}
	d.walked = false
	a.setDropTargeted(h, false)
	a.deferDragLeave(h, heard, d.live)
}

// hideDropTarget is the drag half of [App.applyHidden].
func (a *App) hideDropTarget(h scene.Handle) {
	d := &a.in.drag
	if a.store.Valid(d.target) && a.isAncestor(h, d.target) {
		a.loseDropTarget(d.target, a.data(d.target).dropTarget)
	}
}

// forgetDrop is the drag half of [App.forgetNode]: an unmounted target hears
// nothing more.
func (a *App) forgetDrop(h scene.Handle, nd *nodeData) {
	d := &a.in.drag
	if d.target == h {
		d.target = scene.Handle{}
		d.walked = false
	}
	if d.hit == h {
		d.hit = scene.Handle{}
		d.walked = false
	}
	if nd.dropTarget != nil && !nd.disabled {
		a.countDropTarget(-1)
	}
	nd.dropTarget = nil
}

// NodeIsDropTarget reports whether r declares an [Element.DropTarget],
// enabled or not. gifttest climbs to such a node before it asserts on
// [App.NodeDropTargeted].
func (a *App) NodeIsDropTarget(r NodeRef) bool {
	if !a.store.Valid(r.h) {
		return false
	}
	return a.data(r.h).dropTarget != nil
}

func (a *App) debug(msg string, args ...any) {
	if a.log != nil {
		a.log.Debug(msg, args...)
	}
}
