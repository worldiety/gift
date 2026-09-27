package ui

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// The press feedback of iOS 26: a touched control grows a little, the glass
// lights up under the finger, and on release it settles back with a slight
// overshoot, as if it were on a spring.
//
// It is cheap by construction. The growth is a transform of what the button
// draws anyway, the light is one soft blob – the analytic Gaussian of the
// shadow shader, in white – and the spring is a closed-form curve evaluated
// per frame: no integrator state, nothing uploaded, nothing cached. Frames are
// only asked for while it moves.

const (
	// liftScale is how much a pressed control grows: six percent.
	liftScale = 0.06
	// liftIn is how long the growth on touch takes, and liftOut the settle on
	// release, which is longer because it overshoots.
	liftIn  = 140 * time.Millisecond
	liftOut = 420 * time.Millisecond
)

// Lift gives the button the press feedback of iOS 26: it grows by a few
// percent while pressed, lights up under the finger, and springs back on
// release. The default is off, because it moves the button's picture, and a
// row in a list should not.
func (b ButtonView) Lift(v bool) ButtonView { b.lift = v; return b }

// liftTo starts the lift towards v, 1 pressed and 0 released.
func (n *buttonNode) liftTo(ctx *gift.EventContext, e gift.Event, v float32) {
	if !n.lift {
		return
	}
	st := ctx.ControlState()
	now := ctx.Now()
	if !st.Armed {
		st.Armed, st.Target, st.From, st.Start = true, 0, 0, now
	}
	if v > 0 {
		st.At = e.Pos.Sub(ctx.Bounds().Min)
	}
	if st.Target == v {
		ctx.SetControlState(st)
		return
	}
	st.From = liftPhase(st, now)
	st.Target, st.Start = v, now
	ctx.SetControlState(st)
	if v > 0 {
		ctx.Animate(liftIn)
	} else {
		ctx.Animate(liftOut)
	}
}

// liftPhase is how far the control is lifted at now: 0 at rest, 1 pressed,
// briefly below 0 while it settles.
func liftPhase(st gift.ControlState, now time.Duration) float32 {
	if !st.Armed {
		return 0
	}
	d := now - st.Start
	if d <= 0 {
		return st.From
	}
	dur := liftOut
	if st.Target > st.From {
		dur = liftIn
	}
	if d >= dur {
		return st.Target
	}
	t := float32(d) / float32(dur)
	k := easeOut(t)
	if st.Target < st.From {
		k = spring(t)
	}
	return st.From + (st.Target-st.From)*k
}

// easeOut is fast at first and gentle at the end: a touch answers at once.
func easeOut(t float32) float32 { u := 1 - t; return 1 - u*u*u }

// spring is a damped oscillation that runs from 0 to 1 and overshoots by
// about ten percent before it settles, the release of iOS 26. At t = 1 it is
// within half a percent of 1, and liftPhase returns the target exactly from
// there on.
func spring(t float32) float32 {
	return 1 - float32(math.Exp(-5.5*float64(t))*math.Cos(7.5*float64(t)))
}

// paintLiftGlow draws the light under the finger: a soft white spot at at,
// relative to the top left of b, as bright as the lift is far along. It is
// clipped to the button, so it never spills onto its neighbours.
func paintLiftGlow(ctx *gift.PaintContext, b geom.Rect, at geom.Point, lift float32) {
	r := min(b.Width(), b.Height()) * 0.55
	c := geom.Pt(b.Min.X+at.X, b.Min.Y+at.Y)
	ctx.PushClip(ctx.DeviceBounds())
	ctx.Add(render.Op{
		Kind:         render.OpShadow,
		Bounds:       geom.Rc(c.X-r/2, c.Y-r/2, c.X+r/2, c.Y+r/2),
		CornerRadius: r / 2,
		Blur:         r,
		Color:        render.RGBA(255, 255, 255, uint8(70*min(lift, 1))),
	})
	ctx.PopClip()
}
