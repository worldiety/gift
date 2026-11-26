package gift

import (
	"math"

	"github.com/worldiety/gift/geom"
)

// The safe area is the part of the window nothing the user has to read or
// touch may leave: a phone draws its status bar, its navigation bar and the
// cutout of its camera on top of the application, and an application drawn
// edge to edge paints behind them. gift paints there too — the page, the
// bar of a tab bar — and lays out inside. See [App.SetSafeArea].
//
// It is window state, like the viewport and the density, and arrives the same
// way: from the backend, between two frames, on the UI goroutine. It is not
// [Element.Obstructs], which is the on-screen keyboard's and keeps a focused
// field clear of it; a keyboard comes and goes with the focus, the bars of a
// phone are there the whole time. The project plan, section 52, records the
// model.

// SetSafeArea sets the safe area of the window: per edge, how many logical
// pixels at that edge of the viewport the system covers or cuts off. The unit
// is the unit of the viewport [App.Update] is given.
//
// A negative or non finite edge is zero, and every edge is rounded up to a
// whole logical pixel, so that a bar of 24.4 pixels is not left a sliver of
// the 25th. Setting the value in force does nothing. A change rebuilds every
// scope, because views read it in build — see [BuildContext.SafeArea] — and
// it happens when a phone rotates, folds or shows its bars again, which is
// rarely.
//
// A backend calls it, with what the platform reports and what the application
// configured for a bezel; where there is neither, as on a desktop by default,
// it stays zero.
func (a *App) SetSafeArea(in geom.Insets) {
	a.assertUIGoroutine("SetSafeArea")
	in = geom.Insets{
		Top: safeEdge(in.Top), Right: safeEdge(in.Right),
		Bottom: safeEdge(in.Bottom), Left: safeEdge(in.Left),
	}
	if in == a.safeArea {
		return
	}
	a.safeArea = in
	a.InvalidateAll()
	// A field that was focused before a rotation is wherever the new layout
	// put it; the reveal that keeps it clear of a keyboard runs again.
	a.in.soft.revealPending = true
}

// safeEdge is one edge of [App.SetSafeArea] made whole.
func safeEdge(v float32) float32 {
	if !(v > 0) || math.IsInf(float64(v), 0) {
		return 0
	}
	// A sixty-fourth of a pixel of slack, so that a value that is whole up
	// to the rounding of a float conversion stays whole rather than growing
	// by a pixel.
	return float32(math.Ceil(float64(v) - 1.0/64))
}

// SafeArea returns the safe area in force; see [App.SetSafeArea].
func (a *App) SafeArea() geom.Insets { return a.safeArea }

// SafeRect returns the viewport without the safe area: the rectangle content
// is laid out in. It is never inverted; see [geom.Rect.Inset].
func (a *App) SafeRect() geom.Rect {
	return geom.Rc(0, 0, a.viewport.W, a.viewport.H).Inset(a.safeArea)
}

// SafeArea is [App.SafeArea] read from inside a build. A nil BuildContext
// answers zero, as [BuildContext.SoftKeyboardRequested] answers false.
func (b *BuildContext) SafeArea() geom.Insets {
	if b == nil || b.app == nil {
		return geom.Insets{}
	}
	return b.app.safeArea
}

// SafeArea is [App.SafeArea] read from a view function, for an application
// that places something against the edges itself — a full screen picture
// under ui.Window's IgnoresSafeArea, with its controls inside.
func (c *Context) SafeArea() geom.Insets {
	if c == nil || c.app == nil {
		return geom.Insets{}
	}
	return c.app.safeArea
}

// SafeArea is the safe area of the window, not of the node being laid out. It
// is for a layouter that is laid out against the whole window and decides
// there where its content goes. gift ships three: ui.Window, which insets its
// content; the sheet, which reaches its rounded surface to the window edge;
// and the top layer, which places toasts, popovers and menus in the safe
// rectangle.
func (l *LayoutContext) SafeArea() geom.Insets { return l.app.safeArea }

// Viewport is [App.Viewport] read in layout, so that a layouter can tell
// whether it was given the whole window.
func (l *LayoutContext) Viewport() geom.Size { return l.app.viewport }

// SafeAreaBleed returns, per edge, how far the node being painted may draw its
// background past its own bounds to reach the edge of the window: the safe
// area of that edge where the node lies on the edge of [App.SafeRect], and
// zero everywhere else. A bar laid out at the bottom of the safe rectangle
// paints its background behind the navigation bar of a phone with it, while
// its items, its size and its hit area stay where layout put them.
//
// The answer is in the node's local units, the units of [PaintContext.Bounds],
// so it can be added to them. Under a transform that is not a scale and a
// translation — a rotation, a shear — it is zero, as it is when the window
// has no safe area, which costs a comparison. The clips of the node's
// ancestors still apply; what bleeds is what they let through.
func (p *PaintContext) SafeAreaBleed() geom.Insets {
	s := p.app.safeArea
	if s == (geom.Insets{}) {
		return geom.Insets{}
	}
	m := p.list.Xform(p.xform)
	if !bleedsUnder(m) {
		return geom.Insets{}
	}
	// The corners mapped one by one rather than through TransformRect, which
	// answers an empty rectangle for an empty one: a bar of no height on the
	// bottom edge still bleeds.
	b := p.app.store.Get(p.cur).Bounds
	dev := geom.Rect{
		Min: geom.Pt(m.A*b.Min.X+m.TX, m.D*b.Min.Y+m.TY),
		Max: geom.Pt(m.A*b.Max.X+m.TX, m.D*b.Max.Y+m.TY),
	}
	out := p.app.deviceBleed(dev)
	return geom.Insets{Top: out.Top / m.D, Right: out.Right / m.A, Bottom: out.Bottom / m.D, Left: out.Left / m.A}
}

// bleedsUnder reports whether a node under the device transform m may bleed
// into the safe area at all: m has to be a positive scale and a translation.
// It is the one rule of both readers of the safe area in paint,
// [PaintContext.SafeAreaBleed] and the clip of [Element.ClipBleedsSafeArea],
// so that a subtree is not told it may not bleed while its clip grows.
func bleedsUnder(m geom.Affine2D) bool {
	return m.B == 0 && m.C == 0 && m.A > 0 && m.D > 0
}

// deviceBleed is the bleed of a rectangle in the display list's device
// space, which is logical pixels times the density (see
// [App.densityXform]); unlike the space of [Event.Pos] and of hit testing, it
// includes the density. Per edge it is the safe area of that edge in device
// pixels where r lies on the edge of the safe rectangle, within half a
// logical pixel, and zero elsewhere.
func (a *App) deviceBleed(r geom.Rect) geom.Insets {
	s := a.safeArea
	if s == (geom.Insets{}) {
		return geom.Insets{}
	}
	d := a.density
	if !(d > 0) {
		d = 1
	}
	safe := a.SafeRect()
	tol := 0.5 * d
	var out geom.Insets
	if s.Top > 0 && absf(r.Min.Y-safe.Min.Y*d) <= tol {
		out.Top = s.Top * d
	}
	if s.Bottom > 0 && absf(r.Max.Y-safe.Max.Y*d) <= tol {
		out.Bottom = s.Bottom * d
	}
	if s.Left > 0 && absf(r.Min.X-safe.Min.X*d) <= tol {
		out.Left = s.Left * d
	}
	if s.Right > 0 && absf(r.Max.X-safe.Max.X*d) <= tol {
		out.Right = s.Right * d
	}
	return out
}
