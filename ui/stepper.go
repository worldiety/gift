package ui

import (
	"math"
	"strconv"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var stepperType = gift.RegisterType("ui.Stepper")

// Metrics and timings of a stepper: the ones of UIStepper.
const (
	stepperWidth  = float32(94)
	stepperHeight = float32(32)
	stepperRadius = float32(8)
	stepperGlyph  = float32(14)

	stepperDelay    = 500 * time.Millisecond
	stepperInterval = 100 * time.Millisecond
	stepperFastAt   = 2 * time.Second
	stepperFast     = 50 * time.Millisecond
)

// StepperView changes a number by a step at a time. It is created by
// [Stepper].
type StepperView struct {
	base
	value          float64
	min, max, step float64
	wraps          bool
	onChange       func(float64)
	name           string
	disabled       bool
}

// Stepper returns a minus and a plus button side by side, the counterpart of
// UIStepper and of the control of SwiftUI's Stepper. It shows no number; put
// the value next to it, as iOS does:
//
//	n := ctx.State("tickets", 1.0)
//	ui.HStack(ui.Text(fmt.Sprintf("%v tickets", ctx.Read(n))).Flex(1),
//		ui.Stepper(n.Get(), n.Set).Range(1, 10))
//
// A tap steps once, as soon as the finger is down; held, it steps again after
// half a second, ten times a second, and twenty after two. At the end of the
// [StepperView.Range] its half is greyed out and does nothing, unless the
// stepper [StepperView.Wraps]. Focused, the arrow keys step and home and end
// go to the ends. The value is 0 to 100 in steps of 1 by default.
func Stepper(value float64, onChange func(float64)) StepperView {
	return StepperView{value: checkFinite("Stepper", value), max: 100, step: 1, onChange: onChange}
}

// Range sets the smallest and the largest value.
func (v StepperView) Range(min, max float64) StepperView {
	if isFinite64(min) && isFinite64(max) && min <= max {
		v.min, v.max = min, max
	}
	return v
}

// Step sets how much one step changes the value; it must be greater than
// zero.
func (v StepperView) Step(s float64) StepperView {
	if isFinite64(s) && s > 0 {
		v.step = s
	}
	return v
}

// Wraps makes a step past one end go to the other, as UIStepper's wraps does.
func (v StepperView) Wraps(b bool) StepperView { v.wraps = b; return v }

// Label sets the accessible name.
func (v StepperView) Label(s string) StepperView { v.name = s; return v }

// Disabled takes the stepper out of input.
func (v StepperView) Disabled(b bool) StepperView { v.disabled = b; return v }

// Key sets the reconciliation key.
func (v StepperView) Key(s string) StepperView { v.setKey(s); return v }

// Flex sets the flex factor in a stack.
func (v StepperView) Flex(f float32) StepperView { v.setFlex(f); return v }

// ViewType implements gift.View.
func (v StepperView) ViewType() gift.TypeID { return stepperType }

func isFinite64(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// stepped is the value one step in dir away, and whether there is one.
func (v StepperView) stepped(dir int) (float64, bool) {
	x := v.value + float64(dir)*v.step
	eps := v.step * 1e-9
	switch {
	case x > v.max+eps:
		if !v.wraps {
			return v.value, false
		}
		x = v.min
	case x < v.min-eps:
		if !v.wraps {
			return v.value, false
		}
		x = v.max
	}
	// On the grid of the step from the minimum, so that ten steps of a
	// tenth from zero are one and not 0.9999999999999999.
	if k := math.Round((x - v.min) / v.step); math.Abs(x-(v.min+k*v.step)) < v.step*1e-6 {
		x = v.min + k*v.step
	}
	x = math.Max(v.min, math.Min(v.max, x))
	return x, x != v.value
}

func (v StepperView) set(x float64) {
	if v.onChange != nil && x != v.value {
		v.onChange(x)
	}
}

// Build implements gift.View.
func (v StepperView) Build(*gift.BuildContext) gift.Element {
	n := &stepperNode{
		v:         v,
		bg:        ResolveColor(ColorControl),
		fg:        ResolveColor(ColorLabel),
		off:       ResolveColor(Fade(ColorLabel, 0.25)),
		press:     ResolveColor(Fade(ColorLabel, 0.1)),
		rule:      ResolveColor(ColorSeparator),
		focusRing: resolveBorder(defaultFieldFocusRing),
	}
	if v.disabled {
		n.bg = ResolveColor(ColorControlDisabled)
	}
	return gift.Element{
		Key:        v.key,
		Flex:       v.flex,
		Label:      v.name,
		Layouter:   n,
		Painter:    n,
		Interactor: n,
		Focusable:  !v.disabled,
		Disabled:   v.disabled,
	}
}

// stepperState is the half under the finger and since when it is held; it
// survives the build every step causes.
type stepperState struct {
	dir    int
	inside bool
	since  time.Duration
}

type stepperNode struct {
	v                        StepperView
	bg, fg, off, press, rule Color
	focusRing                Border
}

func (n *stepperNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(geom.Sz(stepperWidth, stepperHeight))
}

func (n *stepperNode) Paint(ctx *gift.PaintContext) {
	st := ctx.Retained[stepperState]()
	b := ctx.Bounds()
	ia := ctx.Interaction()
	mid := (b.Min.X + b.Max.X) / 2
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: n.bg, CornerRadius: stepperRadius})
	if st.dir != 0 && st.inside && !ia.Disabled {
		half := geom.Rc(b.Min.X, b.Min.Y, mid, b.Max.Y)
		if st.dir > 0 {
			half = geom.Rc(mid, b.Min.Y, b.Max.X, b.Max.Y)
		}
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: half.Inset(geom.InsetsAll(2)), Color: n.press, CornerRadius: stepperRadius - 2})
	}
	h := b.Height() * 0.55
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(mid-0.5, b.Min.Y+(b.Height()-h)/2, mid+0.5, b.Min.Y+(b.Height()+h)/2), Color: n.rule})

	cy := (b.Min.Y + b.Max.Y) / 2
	colour := func(dir int) Color {
		if _, ok := n.v.stepped(dir); !ok || ia.Disabled {
			return n.off
		}
		return n.fg
	}
	bar := func(r geom.Rect, c Color) {
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: r, Color: c, CornerRadius: 1})
	}
	g := stepperGlyph / 2
	minus := (b.Min.X + mid) / 2
	bar(geom.Rc(minus-g, cy-1, minus+g, cy+1), colour(-1))
	plus, pc := (mid+b.Max.X)/2, colour(1)
	bar(geom.Rc(plus-g, cy-1, plus+g, cy+1), pc)
	bar(geom.Rc(plus-1, cy-g, plus+1, cy-1), pc)
	bar(geom.Rc(plus-1, cy+1, plus+1, cy+g), pc)

	if ia.FocusVisible && !ia.Disabled {
		paintBorder(ctx, styleSpec{border: n.focusRing, radius: focusRingRadius(stepperRadius)}, focusRingRect(b))
	}
}

// AccessibleValue implements [gift.ValueReporter]: the number.
func (n *stepperNode) AccessibleValue(gift.ControlState) string {
	return strconv.FormatFloat(n.v.value, 'f', -1, 64)
}

func (n *stepperNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st := ctx.Retained[stepperState]()
	if n.v.disabled {
		return false
	}
	half := func(p geom.Point) (int, bool) {
		b := ctx.DeviceBounds()
		dir := -1
		if p.X >= (b.Min.X+b.Max.X)/2 {
			dir = 1
		}
		return dir, b.Contains(p)
	}
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
		st.dir, _ = half(e.Pos)
		st.inside, st.since = true, e.Time
		n.stepOnce(st.dir)
		ctx.After(stepperDelay)
		ctx.Repaint()
		return true
	case gift.EventPointerMove:
		if st.dir == 0 {
			return false
		}
		// Sliding onto the other half does not change sides, as on iOS;
		// leaving the stepper pauses the repeat.
		dir, in := half(e.Pos)
		if in := in && dir == st.dir; in != st.inside {
			st.inside = in
			ctx.Repaint()
		}
		return true
	case gift.EventPointerUp, gift.EventPointerCancel:
		if st.dir == 0 {
			return false
		}
		st.dir, st.inside = 0, false
		ctx.After(0)
		ctx.Repaint()
		return true
	case gift.EventTimer:
		if st.dir == 0 {
			return true
		}
		if st.inside {
			n.stepOnce(st.dir)
		}
		next := stepperInterval
		if e.Time-st.since >= stepperFastAt {
			next = stepperFast
		}
		ctx.After(next)
		return true
	case gift.EventKeyDown:
		switch e.Key {
		case gift.KeyLeft, gift.KeyDown:
			n.stepOnce(-1)
		case gift.KeyRight, gift.KeyUp:
			n.stepOnce(1)
		case gift.KeyHome:
			n.v.set(n.v.min)
		case gift.KeyEnd:
			n.v.set(n.v.max)
		default:
			return false
		}
		return true
	}
	return false
}

func (n *stepperNode) stepOnce(dir int) {
	if x, ok := n.v.stepped(dir); ok {
		n.v.set(x)
		// The next repeat steps from here even when the application has
		// not rebuilt in between.
		n.v.value = x
	}
}
