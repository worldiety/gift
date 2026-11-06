package filedialog

import (
	"fmt"
	"math"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
)

var presentType = gift.RegisterType("filedialog.Present")

// PresentView shows a native file dialog over its content while it is
// presented, in the shape of [ui.Sheet]. Create one with [Present].
//
//	dlg := filedialog.New(open, cancel).Mode(filedialog.OpenMulti).Extensions(".jpg", ".png")
//	filedialog.Present(screen, dlg).Presented(picking)
//
// The dialog opens when Presented turns true, and closes, without a
// callback, when it turns false or the view is unmounted. The application
// sets Presented back to false itself, from onPick and from onCancel, as it
// does for a sheet. A dialog that stays presented after it answered is not
// opened again until Presented has been false.
//
// With [PresentView.Fallback], a system without a native dialog shows the
// given view in a [ui.Sheet] instead, normally a [filepicker.PickerView]
// with the same callbacks; onCancel is then not called for the missing
// dialog. With a fallback, PresentView is a screen like a ui.Sheet and fills
// the area it is given; without one it takes the size of its content.
type PresentView struct {
	content      gift.View
	dialog       Dialog
	presented    bool
	hasPresented bool
	fallback     gift.View
	height       float32
	key          string
	flex         float32
}

// Present returns content with dialog shown over it while presented.
// content must not be nil.
func Present(content gift.View, dialog Dialog) PresentView {
	if content == nil {
		panic("gift/filedialog: Present with a nil content view")
	}
	return PresentView{content: content, dialog: dialog}
}

// Presented says whether the dialog is open right now. The default is
// "presented", as for [ui.SheetView.Presented].
func (p PresentView) Presented(b bool) PresentView { p.presented, p.hasPresented = b, true; return p }

// Fallback sets the view shown in a [ui.Sheet] where there is no native
// dialog. Its scrim and escape key call the dialog's onCancel.
func (p PresentView) Fallback(v gift.View) PresentView { p.fallback = v; return p }

// FallbackHeight sets the height of the fallback sheet as a fraction of the
// height it is presented in, in (0, 1]; see [ui.SheetView.Height].
func (p PresentView) FallbackHeight(f float32) PresentView {
	if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) || f <= 0 || f > 1 {
		panic(fmt.Sprintf("gift/filedialog: FallbackHeight(%v) must be a fraction in (0, 1]", f))
	}
	p.height = f
	return p
}

// Key sets the reconciliation key of this view among its siblings.
func (p PresentView) Key(s string) PresentView { p.key = s; return p }

// Flex makes the view take a share of the remaining main axis space of its
// parent stack.
func (p PresentView) Flex(f float32) PresentView { p.flex = f; return p }

// ViewType implements gift.View.
func (p PresentView) ViewType() gift.TypeID { return presentType }

// Build implements gift.View. The state — whether the dialog is open,
// whether the fallback is shown — lives in a component of its own.
func (p PresentView) Build(bc *gift.BuildContext) gift.Element {
	return ui.ZStack(gift.Component("filedialog", p.body)).Key(p.key).Flex(p.flex).Build(bc)
}

// session is what a PresentView keeps between builds.
type session struct {
	// was is the presented value the last build acted on.
	was bool
	// gen counts presentations, so that a late answer of an earlier one
	// changes nothing.
	gen    uint64
	cancel func()
}

func (p PresentView) isPresented() bool { return !p.hasPresented || p.presented }

func (p PresentView) body(ctx *gift.Context) gift.View {
	st := ctx.State("session", (*session)(nil))
	s := st.Get()
	if s == nil {
		s = &session{}
		st.Set(s)
	}
	fb := ctx.State("fallback", false)
	useFallback := ctx.Read(fb)
	presented := p.isPresented()

	switch {
	case presented && !s.was:
		s.was = true
		s.gen++
		if err := p.dialog.opener().unavailable(); p.fallback != nil && err != nil {
			p.dialog.opener().reportUnavailable(p.dialog.log, err)
			fb.Set(true)
			useFallback = true
			break
		}
		tok := ctx.Request()
		var unsupported func()
		if p.fallback != nil {
			gen := s.gen
			unsupported = func() {
				if s.gen == gen && s.was {
					fb.Set(true)
				}
			}
		}
		s.cancel, _ = p.dialog.open(tok.Post, unsupported)
	case !presented && s.was:
		s.was = false
		s.gen++
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
		if useFallback {
			fb.Set(false)
			useFallback = false
		}
	}
	ctx.OnUnmount("close", func() {
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
	})

	// The tree keeps one shape, so the content keeps its state when the
	// fallback comes and goes: the sheet's child is the fallback only while
	// it is used, and an empty stack otherwise, so that a hidden picker does
	// not read directories.
	var child gift.View
	if p.fallback != nil {
		child = ui.ZStack()
		if useFallback {
			child = p.fallback
		}
	}
	sh := ui.Sheet(p.content, child).Presented(presented && useFallback).OnDismiss(p.dialog.onCancel)
	if p.height != 0 {
		sh = sh.Height(p.height)
	}
	return sh
}
