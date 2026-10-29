package ui

import (
	"fmt"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var spacerType = gift.RegisterType("ui.Spacer")

// SpacerView is a flexible, invisible gap. It is created by [Spacer].
//
// It has no style modifiers at all, which is the point of variant A: a spacer
// that accepted Background and then ignored it would be a lie. It has a flex
// of one by default, a minimum length of zero and draws nothing.
type SpacerView struct {
	key  string
	flex float32
	min  float32
}

// Spacer returns a flexible gap with a flex of one and no minimum length.
func Spacer() SpacerView { return SpacerView{flex: 1} }

// ViewType implements gift.View.
func (s SpacerView) ViewType() gift.TypeID { return spacerType }

// Build implements gift.View.
func (s SpacerView) Build(*gift.BuildContext) gift.Element {
	l := zeroSpacer
	if s.min != 0 {
		l = &spacerNode{min: s.min, flexible: s.flex > 0}
	}
	return gift.Element{Key: s.key, Flex: s.flex, Layouter: l}
}

// Flex sets the share of the remaining main axis space this spacer takes.
// Two spacers with flex 1 and 3 split the free space one to three.
func (s SpacerView) Flex(v float32) SpacerView { s.flex = checkFlex(v); return s }

// MinLength sets the smallest main axis extent of the spacer.
//
// It is honoured even when the stack has no space left, including in an
// unbounded axis, where the stack offers every flexible child a tight extent
// of zero. The spacer then deliberately returns more than its constraints
// allow and pushes its siblings along, which gift passes through unchanged;
// see [gift.Layouter]. A spacer that silently collapsed to zero would be the
// worse surprise.
//
// v must be finite and not negative, for the reason [SpacerView.Flex] and the
// padding of every other view are checked: an infinite length becomes an
// infinite origin for every sibling after it, and a NaN one a NaN vertex.
func (s SpacerView) MinLength(v float32) SpacerView {
	if !isFinite(v) || v < 0 {
		panic(fmt.Sprintf("gift/ui: Spacer.MinLength(%v) must be a finite, non negative number", v))
	}
	s.min = v
	return s
}

// Key sets the reconciliation key of this view among its siblings.
func (s SpacerView) Key(v string) SpacerView { s.key = v; return s }

// zeroSpacer is the layouter of every spacer without a minimum length, which
// is the common case. It carries no state, so one shared instance is enough
// and a build does not allocate for it.
var zeroSpacer = &spacerNode{}

// spacerNode is the layouter of a spacer. It takes exactly the minimum its
// constraints ask for, which is the extent a stack assigned to it.
type spacerNode struct {
	min float32
	// flexible is whether the spacer has a positive Flex, which decides how
	// Layout recognises the main axis.
	flexible bool
}

// Layout returns the minimum of the constraints, raised to the minimum length
// along the main axis.
//
// The main axis is recognised from the constraints, which is the reason a
// spacer does not have to know whether it sits in a VStack or an HStack. A
// stack measures a flexible child with a tight main axis and a loose cross
// axis, so for a flexible spacer the main axis is the tight one. It measures
// an inflexible child with an unbounded main axis and a bounded cross axis, so
// for a spacer with a Flex of zero the main axis is the unbounded one.
//
// The two rules cannot be merged into one, because each is ambiguous where the
// other is not: a stack whose cross axis is zero wide hands an inflexible
// child a cross axis of 0..0, which is tight, and an unbounded stack hands a
// flexible child a main axis of 0..0, which is bounded. Only the spacer's own
// flex tells the two apart. If the constraints match neither pattern, there is
// no main axis to speak of and the minimum length is not applied.
func (s *spacerNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	w, h := c.Min.W, c.Min.H
	if s.min > 0 {
		var mainW, mainH bool
		if s.flexible {
			mainW = c.Min.W == c.Max.W && c.Min.H != c.Max.H
			mainH = c.Min.H == c.Max.H && c.Min.W != c.Max.W
		} else {
			mainW = !c.HasBoundedWidth() && c.HasBoundedHeight()
			mainH = !c.HasBoundedHeight() && c.HasBoundedWidth()
		}
		switch {
		case mainH:
			h = max(h, s.min)
		case mainW:
			w = max(w, s.min)
		}
	}
	return geom.Sz(w, h)
}
