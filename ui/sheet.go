package ui

import (
	"fmt"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var (
	sheetType      = gift.RegisterType("ui.Sheet")
	sheetPanelType = gift.RegisterType("ui.sheetPanel")
)

// Metrics of a sheet, in logical pixels unless said otherwise.
const (
	// sheetRadius is the corner radius of the top edge of a sheet.
	sheetRadius = float32(14)
	// sheetHeight is the default height of a sheet as a fraction of the
	// height it is presented in: tall enough to work in, short enough that
	// the dimmed screen above says "this goes back where it came from".
	sheetHeight = float32(0.9)
	// sheetMaxWidth is the default widest a sheet gets. A sheet across the
	// whole of a 1920 pixel kiosk panel is a page, not a sheet.
	sheetMaxWidth = float32(720)
)

// SheetView presents one view in a panel that slides up from the bottom edge
// over another view, and takes the input of everything underneath. It is
// created by [Sheet]; the zero value is not useful.
//
//	ui.Sheet(screen, picker).
//	    Presented(open).
//	    OnDismiss(func() { setOpen(false) })
//
// # What it is made of
//
// It is a [ModalView] with a different dialog, and the three things that make
// a modal modal are the same three things here, built from the same parts: the
// scrim that swallows every pointer event outside the panel, the focus trap,
// and the drawing order. What differs is the panel: it is anchored to the
// bottom edge, as wide as the window up to [SheetView.MaxWidth], as tall as
// [SheetView.Height] of it, and its whole height away while it is not
// presented, so that it arrives from below the window rather than from under
// its own footprint.
//
// # Dismissing it
//
// A tap on the scrim, and the escape key, call [SheetView.OnDismiss]. Escape is
// delivered to the sheet whether or not anything in it holds the focus — see
// [gift.Element.KeyFallback] — because a sheet opened by a tap on a touchscreen
// has nothing focused and a key that reached nobody would be a key that does
// nothing. A sheet without OnDismiss can be closed only by the application,
// which is what a sheet that demands an answer wants.
//
// There is no drag to dismiss. The panel is a surface, not a control.
//
// # Presented, not nil
//
// For the reason [ModalView.Presented] gives: a nil sheet is unmounted and
// cannot be seen to leave. Pass the sheet's view and say whether it is up.
//
// # The on-screen keyboard
//
// A [Window] with an [OnScreenKeyboard] among its content lays the rest of
// that content out above the keyboard while it is showing, so a sheet inside
// such a Window sits on the keyboard rather than under it, and a text field at
// the bottom of the sheet stays in sight. The sheet gets shorter by the
// keyboard's height while it is up, because its height is a fraction of the
// space it is given.
type SheetView struct {
	base
	content      gift.View
	sheet        gift.View
	onDismiss    func()
	presented    bool
	hasPresented bool
	scrim        Color
	hasScrim     bool
	height       float32
	maxWidth     float32
}

// Sheet returns content with sheet presented over it from the bottom edge. A
// nil sheet is the closed state with nothing mounted; see [SheetView] for why
// [SheetView.Presented] is usually the better way to close one.
//
// content must not be nil.
func Sheet(content, sheet gift.View) SheetView {
	if content == nil {
		panic("gift/ui: Sheet with a nil content view")
	}
	return SheetView{content: content, sheet: sheet}
}

// ViewType implements gift.View.
func (s SheetView) ViewType() gift.TypeID { return sheetType }

// Build implements gift.View.
func (s SheetView) Build(bc *gift.BuildContext) gift.Element {
	if s.sheet == nil {
		// Closed means absent, as for [ModalView].
		return ZStack(s.content).Key(s.key).Flex(s.flex).Build(bc)
	}
	wash := defaultScrimColor
	if s.hasScrim {
		wash = s.scrim
	}
	presented := true
	if s.hasPresented {
		presented = s.presented
	}
	h := s.height
	if h == 0 {
		h = sheetHeight
	}
	w := s.maxWidth
	if w == 0 {
		w = sheetMaxWidth
	}
	return ZStack(
		s.content,
		newLayer("sheet", ZStack(
			scrim{color: wash, onTap: s.onDismiss},
			sheetPanel{
				hidden:    !presented,
				child:     s.sheet,
				height:    h,
				maxWidth:  w,
				onDismiss: s.onDismiss,
			},
		).Align(geom.Bottom)).
			Trap(presented).
			Hidden(!presented),
	).Key(s.key).Flex(s.flex).Build(bc)
}

// --- modifiers -------------------------------------------------------------

// OnDismiss makes a tap on the scrim and the escape key call fn. A nil fn, the
// default, makes both do nothing; see [ModalView.OnDismiss].
func (s SheetView) OnDismiss(fn func()) SheetView { s.onDismiss = fn; return s }

// Presented says whether the sheet is on the screen right now; see
// [ModalView.Presented]. The default is "presented".
func (s SheetView) Presented(v bool) SheetView { s.presented, s.hasPresented = v, true; return s }

// Scrim sets the wash drawn over the content while the sheet is up; see
// [ModalView.Scrim].
func (s SheetView) Scrim(c Color) SheetView { s.scrim, s.hasScrim = c, true; return s }

// Height sets the height of the panel as a fraction of the height it is
// presented in, in (0, 1]. The default is 0.9.
func (s SheetView) Height(f float32) SheetView {
	if !isFinite(f) || f <= 0 || f > 1 {
		panic(fmt.Sprintf("gift/ui: Sheet.Height(%v) must be a fraction in (0, 1]", f))
	}
	s.height = f
	return s
}

// MaxWidth sets the widest the panel gets. A narrower window gets a panel as
// wide as itself. The default is 720.
func (s SheetView) MaxWidth(px float32) SheetView {
	if !isFinite(px) || px <= 0 {
		panic(fmt.Sprintf("gift/ui: Sheet.MaxWidth(%v) must be a positive number", px))
	}
	s.maxWidth = px
	return s
}

// Key sets the reconciliation key of this view among its siblings.
func (s SheetView) Key(v string) SheetView { s.setKey(v); return s }

// Flex makes the sheet host take a share of the remaining main axis space of
// its parent stack.
func (s SheetView) Flex(v float32) SheetView { s.setFlex(v); return s }

// --- the panel ---------------------------------------------------------------

// sheetPanel is the node the sheet hangs under: the thing that moves, as
// [modalDialog] is for a modal, and the thing that sizes the sheet.
type sheetPanel struct {
	hidden    bool
	child     gift.View
	height    float32
	maxWidth  float32
	onDismiss func()
}

// ViewType implements gift.View.
func (p sheetPanel) ViewType() gift.TypeID { return sheetPanelType }

// Build implements gift.View.
//
// The face is a [ZStack] with the surface colour and a uniform radius, and it
// is sheetRadius taller than the panel: the extra strip hangs below the window
// edge, so only the top corners are seen rounded. A radius per corner would be
// a new field in every style of this package for one caller.
func (p sheetPanel) Build(bc *gift.BuildContext) gift.Element {
	face := ZStack(p.child).
		Align(geom.TopLeading).
		PaddingInsets(geom.Insets{Bottom: sheetRadius}).
		Background(ColorSurface).
		CornerRadius(sheetRadius).
		Key("face")
	return gift.Element{
		Key:      "panel",
		Layouter: &sheetPanelNode{height: p.height, maxWidth: p.maxWidth},
		Children: []gift.View{face},
		Hidden:   p.hidden,
		// The whole height of the panel, so it starts and ends below the
		// window edge rather than a third of the way up it.
		Transition: gift.TransitionSpec{
			Parked:   geom.Pt(0, 1),
			Duration: ControlAnimation,
		},
		Interactor:  sheetKeys{onDismiss: p.onDismiss},
		KeyFallback: true,
	}
}

// sheetPanelNode sizes the panel from the space it is presented in.
type sheetPanelNode struct {
	height   float32
	maxWidth float32
}

// Layout takes Height of the available height and up to MaxWidth of the
// available width, and lays the face out sheetRadius taller than that.
//
// It needs bounded constraints, for the reason a modal does: a sheet in an
// unbounded axis would be zero tall, and the scrim under it zero sized, so
// every tap would go through a sheet that is drawn on the screen.
func (n *sheetPanelNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	if !c.HasBoundedWidth() || !c.HasBoundedHeight() {
		panic("gift/ui: a Sheet measured with an unbounded axis; put it where it gets the whole " +
			"window, as the outermost view inside ui.Window, the way ui.Modal is placed")
	}
	size := c.Constrain(geom.Sz(min(c.Max.W, n.maxWidth), roundf(c.Max.H*n.height)))
	ctx.Measure(0, geom.Tight(geom.Sz(size.W, size.H+sheetRadius)))
	ctx.Place(0, geom.Point{})
	// The strip under the bottom edge is the rounding trick of
	// [sheetPanel.Build], not content that does not fit.
	ctx.ReportOverflow(geom.Size{})
	return size
}

// sheetKeys is the interactor of the panel: the escape key and nothing else.
type sheetKeys struct{ onDismiss func() }

// HandleEvent implements gift.Interactor.
func (k sheetKeys) HandleEvent(_ *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventKeyDown || e.Key != gift.KeyEscape {
		return false
	}
	// Consumed even without a handler, for the reason [navBackKey] gives:
	// an escape that travelled out of the sheet would act on a screen the
	// user cannot reach from here.
	if k.onDismiss != nil {
		k.onDismiss()
	}
	return true
}
