package ui

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// Edges is a set of the four edges of a rectangle. It says which edges of a
// view's background may reach past the safe area to the edge of the window;
// see [Stack.BleedSafeArea].
//
//	ui.HStack(controls...).Background(ui.ColorSurface).
//		BleedSafeArea(ui.EdgesLeft | ui.EdgesRight | ui.EdgesBottom)
//
// It is a bit set and not four booleans because the question it answers is
// always asked of a whole bar at once — a bar at the bottom of a screen bleeds
// at the bottom and at both sides, never at the top, where its neighbour is —
// and a set spells that in one expression.
type Edges uint8

const (
	// EdgesTop is the top edge: the status bar of a phone held upright.
	EdgesTop Edges = 1 << iota
	// EdgesRight is the right edge: the cutout or the navigation bar of a
	// phone turned on its side.
	EdgesRight
	// EdgesBottom is the bottom edge: the navigation bar or the gesture
	// area of a phone held upright.
	EdgesBottom
	// EdgesLeft is the left edge, the mirror image of [EdgesRight].
	EdgesLeft

	// EdgesAll is every edge, for a view that covers the whole safe area.
	EdgesAll = EdgesTop | EdgesRight | EdgesBottom | EdgesLeft
)

// of returns the insets of in at the edges in e, and zero at the others.
func (e Edges) of(in geom.Insets) geom.Insets {
	var out geom.Insets
	if e&EdgesTop != 0 {
		out.Top = in.Top
	}
	if e&EdgesRight != 0 {
		out.Right = in.Right
	}
	if e&EdgesBottom != 0 {
		out.Bottom = in.Bottom
	}
	if e&EdgesLeft != 0 {
		out.Left = in.Left
	}
	return out
}

// setBleed is the body of every BleedSafeArea modifier. The edges are a
// property of the style and not of the layout, because nothing about the
// layout changes: see [paintBackground].
func (b *base) setBleed(e Edges) { b.style.bleed = e & EdgesAll }

// bleedRect is the rectangle the background of a node with the style st is
// filled in: its bounds b, grown at each edge in st.bleed by how far the node
// may reach past the safe area there — see [gift.PaintContext.SafeAreaBleed].
//
// A rounded background keeps its bounds. Its corners are the shape of a card,
// a dialog or a sheet, and a card at the bottom of the safe area that grew
// into the navigation bar would lose its lower corners behind the bar and
// gain a slab of colour under it, which is neither the card nor the bar. A
// rounded surface that is meant to reach the edge of the window, a sheet
// rising from the bottom, has to get there by growing its own layout.
//
// A node without edges costs one comparison, and a window without a safe
// area, the desktop, one more inside SafeAreaBleed.
func bleedRect(ctx *gift.PaintContext, st styleSpec, b geom.Rect) geom.Rect {
	if st.bleed == 0 || st.radius > 0 {
		return b
	}
	in := st.bleed.of(ctx.SafeAreaBleed())
	if in == (geom.Insets{}) {
		return b
	}
	return b.Outset(in)
}
