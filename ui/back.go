package ui

import (
	"github.com/worldiety/gift"
)

var onBackType = gift.RegisterType("ui.OnBack")

// OnBackView is content with a back action of its own: what the back button
// and the back gesture of Android do while the content is on the screen. It is
// created by [OnBack]; the zero value is not useful.
//
//	ui.OnBack(wizard, func() { step.Set(step.Get() - 1) }).Disabled(ctx.Read(step) == 0)
//
// # Who needs it
//
// The containers of this package answer the back action themselves: a
// [NavigationStack] pops, a [Sheet], a [Modal], a [Popover] and a [Menu] are
// dismissed. OnBack is for the application whose navigation is its own — a
// wizard that is a state machine, a pager, a detail pane that replaces a list
// inside one screen — and which would otherwise find that back leaves the
// whole application from its second step. It is the counterpart of Android's
// OnBackPressedCallback and of BackHandler in Jetpack Compose.
//
// # Disabled is a question asked before the gesture
//
// [OnBackView.Disabled] says that there is nowhere to go back to, and it has
// to be said in the build, not found out when back arrives. Android asks
// before the user lifts the finger: its predictive back animates either the
// page underneath or the home screen while the gesture is still under way, and
// it delivers back only to an application that said beforehand that it wants
// it; see [gift.App.CanGoBack]. So a disabled OnBack is as if it were not
// there, and back goes on to what is around it — and on the first view of the
// application to the system, which leaves it. A nil action is the same as
// Disabled(true).
//
// # Where it sits on the route
//
// Inside the active focus trap, the last [gift.BackHandler] in document order
// that takes the action is the candidate; the focused node and its ancestors
// are asked as well, and the first of them that takes it goes back unless the
// candidate lies inside it. See [gift.App.Back]. An OnBack around a navigation
// stack on its root screen therefore gets back after the stack declined it,
// and one inside a pushed screen gets it before the stack pops. One that is
// disabled is skipped and hides nothing: a stack before it in document order
// still pops. A [Modal] or a [Sheet] that is up bounds the search, so an
// OnBack behind it is not asked.
//
// # It is transparent to everything else
//
// The back action is the node's [gift.Element.Back], and the node has no
// interactor. It is not a hit target: a press on a part of the content where
// no child is a hit target goes through to whatever is under the node — a
// sibling in a [ZStack] behind it, or nothing — exactly as it would without
// the wrapper. It is not a stop for bubbling events, so a press, a release, a
// wheel or a key travels past it to the ancestors unchanged. It is not
// focusable. It does not declare [gift.Element.KeyFallback], so escape goes
// where it went; back and escape are separate on purpose, see
// [gift.BackHandler].
type OnBackView struct {
	content  gift.View
	action   func()
	disabled bool
	key      string
	flex     float32
}

// OnBack returns content whose back action calls action; see [OnBackView].
// content must not be nil; a nil action is the same as Disabled(true).
//
// The action runs on the UI goroutine, inside the input phase, where writing
// state is normal. It is a request like the back affordance of a
// [NavigationStack]: nothing changes on the screen until the application
// changes its state.
func OnBack(content gift.View, action func()) OnBackView {
	if content == nil {
		panic("gift/ui: OnBack with a nil content view")
	}
	return OnBackView{content: content, action: action}
}

// Disabled says that there is nowhere to go back to: back passes on to what is
// around the view, as if the wrapper were not there; the content stays as it
// is. It has to be said in the build; see "Disabled is a question asked
// before the gesture".
func (v OnBackView) Disabled(b bool) OnBackView { v.disabled = b; return v }

// Key sets the reconciliation key.
func (v OnBackView) Key(s string) OnBackView { v.key = s; return v }

// Flex sets the share of the free space of a stack this view takes.
func (v OnBackView) Flex(f float32) OnBackView { v.flex = checkFlex(f); return v }

// ViewType implements [gift.View].
func (OnBackView) ViewType() gift.TypeID { return onBackType }

// Build implements [gift.View].
//
// The node takes the size of its content, draws nothing and has no
// interactor; see "It is transparent to everything else".
func (v OnBackView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{
		Key:      v.key,
		Flex:     v.flex,
		Layouter: anchorLayout{},
		Back:     backStop{take: !v.disabled && v.action != nil, action: v.action},
		Children: []gift.View{v.content},
	}
}

// backStop is the [gift.Element.Back] of a node that has a back action and
// nothing else: [OnBackView], and the body of a [ModalView]. It is not an
// interactor on purpose: an interactor would make the node a hit target and a
// stop for every bubbling event, and these nodes must not change where a tap
// lands.
//
// take is decided in the build, so that [gift.BackHandler.CanGoBack] is a
// read of a bool. A take with a nil action is a block: the back action is
// consumed and nothing happens, which is what a modal without OnDismiss wants.
type backStop struct {
	take   bool
	action func()
}

// CanGoBack implements gift.BackHandler.
func (s backStop) CanGoBack() bool { return s.take }

// GoBack implements gift.BackHandler.
func (s backStop) GoBack(*gift.EventContext) {
	if s.action != nil {
		s.action()
	}
}
