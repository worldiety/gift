package ui

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/layout"
)

var zstackType = gift.RegisterType("ui.ZStack")

// Overlay stacks its children on top of each other in Z order, sizes itself
// to the largest one and aligns every child inside that box. It is created by
// [ZStack].
type Overlay struct {
	base
	children []gift.View

	// coverContent is [Overlay.AvoidKeyboard] negated, so that the zero
	// value — every ZStack nobody configured — is the default of avoiding.
	coverContent bool

	// plate marks child 0 as the window background of [Window]; see
	// [node.layoutAvoiding] for why it is the one child that is not moved
	// out from under the keyboard.
	plate bool
}

// ZStack draws its children on top of each other, first child at the bottom.
// The ownership rule of [VStack] applies unchanged.
//
// Every child is measured with the same loose but bounded constraints, so a
// greedy child such as an unframed [Box] fills the whole box on both axes.
// A child's Flex is ignored; see [Overlay.Flex] for why.
//
// The one exception is a direct child that is an [OnScreenKeyboard]: while it
// is showing, the other children are laid out in the part of the box it does
// not cover. See [Overlay.AvoidKeyboard].
func ZStack(children ...gift.View) Overlay {
	return Overlay{children: children}
}

// ViewType implements gift.View.
func (o Overlay) ViewType() gift.TypeID { return zstackType }

// Build implements gift.View.
//
// It is also where the keyboard child is found, by its type, once per build;
// the layout pass then reads one int. See [Overlay.AvoidKeyboard].
func (o Overlay) Build(*gift.BuildContext) gift.Element {
	e := element(o.base, kindOverlay, 0, layout.Vertical, layout.CrossAlignPosition, o.children)
	if o.coverContent {
		return e
	}
	// The last keyboard wins, which is the rule [gift.Element.Obstructs]
	// already has for the same mistake of placing two.
	kb := -1
	for i, c := range o.children {
		if _, ok := c.(KeyboardView); ok {
			kb = i
		}
	}
	if kb >= 0 {
		n := e.Layouter.(*node)
		n.kb = kb + 1
		n.plate = o.plate
	}
	return e
}

// AvoidKeyboard says whether the other children make room for an
// [OnScreenKeyboard] that is a direct child of this overlay. The default is
// true.
//
//	ui.Window(screen, ui.OnScreenKeyboard()).Align(geom.Bottom)
//
// While the keyboard is showing, every other child is laid out in the part of
// the box the keyboard leaves free — above it for the bottom alignment that
// is the normal placement, below it for a keyboard aligned to the top — as if
// the window had become shorter by [OnScreenKeyboardHeight]. When the keyboard
// goes away they get the whole box back. It is what iOS does for a view that
// respects the keyboard layout guide and what Android calls "adjustResize".
//
// # Why this is the default, and why it lives here
//
// Because a keyboard laid *over* an application was only ever right for a
// screen with nothing at the bottom. On the 800x480 kiosk panel it covers more
// than half the window, and what it covered was typically the thing the user
// needs next: the button that submits the form they are typing into, the tab
// bar, the last field of a form that has no content left below it to scroll
// up. The obstruction reveal of [KeyboardView] could lift a field only as far
// as its container could still scroll, and every application worked round the
// rest with a spacer of the keyboard's height at the bottom of each scroll
// view. A default that every application has to undo in the same way is the
// wrong default.
//
// It is a property of the overlay rather than of [Window] because the
// keyboard's placement is the overlay's: [OnScreenKeyboard] has always been
// documented as a child of a ZStack, [Window] is a ZStack, and a ZStack is the
// only container in this package that puts two children in the same place.
// An avoidance that worked in one spelling of that composition and not in the
// other would be a difference nobody could see from the call site.
//
// It is decided in the layout and not in the build, from the height the
// keyboard actually measured. A hidden keyboard measures zero, so the switch
// being on costs nothing and changes nothing until a field asks for the
// keyboard; a keyboard given a different height with [KeyboardView.Frame]
// reserves that height, not the constant.
//
// # What stays under it
//
// Only a keyboard that is a *direct* child is recognised, because the
// recognition is by type in [Overlay.Build]: one wrapped in another container
// is laid out like any other child and the overlay cannot know it is there.
// The obstruction reveal of [KeyboardView] still works for that composition,
// with its old limit. The background plate of [Window] keeps the whole box,
// so the page is painted behind the keyboard as well.
//
// A vertical alignment other than the top or bottom edge reserves nothing: a
// keyboard in the middle of the box would split it into two bands, and a
// child can only be laid out into one.
//
// # When to turn it off
//
// AvoidKeyboard(false) restores the overlap. That is right for a screen that
// must not re-flow while somebody types — a full screen camera preview with
// one field over it, say, where a smaller preview would be a jump of the
// whole picture — and for nothing a form is part of.
//
// # Modals
//
// A [Modal] belongs *inside* the window, with the keyboard next to it:
//
//	ui.Window(ui.Modal(screen, sheet), ui.OnScreenKeyboard()).Align(geom.Bottom)
//
// The modal is then one of the children that avoid the keyboard, so its
// scrim and its centred sheet are laid out in the band above it, and a field
// in the sheet stays visible while it is being typed into. The other way
// round — a keyboard inside the modal's content — puts the keyboard under the
// scrim, where no key can be pressed.
func (o Overlay) AvoidKeyboard(v bool) Overlay { o.coverContent = !v; return o }

// Padding sets the same padding on all four edges, replacing any previous
// padding. It must be finite and non negative; see [Stack.Padding].
func (o Overlay) Padding(v float32) Overlay { o.setPadding(v); return o }

// PaddingInsets sets the padding per edge, replacing any previous padding.
func (o Overlay) PaddingInsets(v geom.Insets) Overlay { o.setPaddingInsets(v); return o }

// Align sets how the children are placed inside the box. Unlike in a [Stack],
// both components are used, because neither axis is a stacking axis.
func (o Overlay) Align(v geom.Alignment) Overlay { o.setAlign(v); return o }

// Frame fixes both axes. Pass [geom.Unbounded] for an axis that should stay
// free.
//
// Precedence: Frame is applied first, then Max, then Min, and the call order
// of the modifiers does not matter. Frame(200, 100).MaxWidth(50) is 50 wide
// and Frame(20, 20).MinWidth(80) is 80 wide; see [frameSpec].
func (o Overlay) Frame(w, h float32) Overlay { o.setFrame(w, h); return o }

// MinWidth raises the minimum width of the node. It also raises the maximum
// if that is lower: a minimum wins over a Frame and over a MaxWidth. See
// [frameSpec] for the full precedence rule.
func (o Overlay) MinWidth(v float32) Overlay { o.setMinWidth(v); return o }

// MinHeight raises the minimum height of the node, and the maximum with it if
// that is lower; see [frameSpec].
func (o Overlay) MinHeight(v float32) Overlay { o.setMinHeight(v); return o }

// MaxWidth lowers the maximum width of the node, and the minimum with it if
// that is higher. A MinWidth applied on top of it still wins; see [frameSpec].
func (o Overlay) MaxWidth(v float32) Overlay { o.setMaxWidth(v); return o }

// MaxHeight lowers the maximum height of the node, and the minimum with it if
// that is higher; see [frameSpec].
func (o Overlay) MaxHeight(v float32) Overlay { o.setMaxHeight(v); return o }

// Background fills the bounds behind the children.
func (o Overlay) Background(v Background) Overlay { o.setBackgroundSpec(v); return o }

// Border strokes the inside of the bounds after the children were drawn.
func (o Overlay) Border(v Border) Overlay { o.setBorder(v); return o }

// Shadow draws a blurred copy of the background shape behind the view.
//
// It extends the paint bounds but not the layout size and not the hit area, so
// a shadow never moves a sibling and never makes a gap clickable; the project
// plan, section 8, fixes that. A parent clip cuts it.
func (o Overlay) Shadow(v Shadow) Overlay { o.setShadow(v); return o }

// CornerRadius rounds the background and the border.
func (o Overlay) CornerRadius(v float32) Overlay { o.setCornerRadius(v); return o }

// Clip confines the children to the padded bounds.
func (o Overlay) Clip(v bool) Overlay { o.setClip(v); return o }

// Key sets the reconciliation key of this view among its siblings.
func (o Overlay) Key(v string) Overlay { o.setKey(v); return o }

// Flex makes the overlay take a share of the remaining main axis space of its
// parent stack, proportional to v.
//
// # A ZStack does not honour the Flex of its own children
//
// This is the one documented exception to the rule in the package
// documentation that a view never accepts a modifier it then ignores. Flex on
// this type is honoured — by the parent stack, which is whose space is being
// divided. Flex on a child of this ZStack is not: a Z stack has no main axis,
// so there is no remainder and no direction to divide it along, and the
// question "how do I make this child fill the box" already has an answer that
// does not need Flex. Every child is measured with the full, bounded
// constraints of the box, so a greedy child such as an unframed [Box] fills it
// on both axes.
//
// The alternative would have been to make Flex mean "fill" inside a ZStack,
// which is a second, incompatible meaning for the same word and would then
// have to answer what Flex(2) is supposed to be. It is documented instead.
func (o Overlay) Flex(v float32) Overlay { o.setFlex(v); return o }
