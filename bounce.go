package gift

import (
	"math"
	"time"

	"github.com/worldiety/gift/internal/scene"
)

// The rubber band of iOS, for every scroll container unless its
// [ScrollConfig.HardStop] is set.
//
// Dragged past an end, the content follows the finger with growing
// resistance – the curve UIKit uses, where the displacement approaches the
// viewport size but never reaches it. Let go, it springs back. A fling that
// reaches an end with speed left bounces off it and settles.
//
// It is all translation: the offset stays within its bounds, and what goes
// past them is an extra displacement in the container's transform. Nothing is
// laid out or built for it, and it costs a repaint of the container while it
// moves.

// bounceStiffness is the angular frequency of the spring back in 1/s. At 14
// a pulled list is home in about a third of a second, and a bounce off an end
// overshoots by a little over a hundredth of the fling speed.
const bounceStiffness = 14

// rubber is the displacement shown for a pull of p past an end, for a
// viewport of size dim: UIKit's (1 - 1/(p·c/dim + 1))·dim with c = 0.55.
func rubber(p, dim float32) float32 {
	if !(dim > 0) {
		return 0
	}
	a := p
	if a < 0 {
		a = -a
	}
	r := (1 - 1/(a*0.55/dim+1)) * dim
	if p < 0 {
		return -r
	}
	return r
}

// dragBounce moves a bouncing container by d offset units under a finger.
// Within the bounds it scrolls; past them the rest becomes pull.
func (a *App) dragBounce(h scene.Handle, s *scrollState, d float64) {
	s.bouncing = false
	if s.pull != 0 {
		p := s.pull + float32(d)
		if (p > 0) == (s.pull > 0) && p != 0 {
			s.pull = p
			s.over = rubber(p, s.viewport)
			a.markNeedsPaint(h)
			return
		}
		// Back across the end: the remainder scrolls.
		d = float64(p)
		s.pull, s.over = 0, 0
		a.markNeedsPaint(h)
	}
	target := s.off + d
	clamped := s.clamp(target)
	a.setScroll(h, s, clamped)
	if p := float32(target - clamped); p != 0 {
		s.pull = p
		s.over = rubber(p, s.viewport)
		a.markNeedsPaint(h)
	}
}

// releaseBounce springs a container that was let go past an end back.
func (a *App) releaseBounce(h scene.Handle, s *scrollState) {
	over := s.over
	s.pull = 0
	a.startBounce(h, s, over, 0)
}

// startBounce starts the spring from displacement x0 with velocity v0, both
// in offset units, and enrols the container in the scroll tick.
func (a *App) startBounce(h scene.Handle, s *scrollState, x0, v0 float32) {
	s.bouncing = true
	s.overFrom, s.overVel, s.overStart = x0, v0, a.in.now
	s.over = x0
	for _, existing := range a.in.flings {
		if existing == h {
			a.markNeedsPaint(h)
			return
		}
	}
	a.in.flings = append(a.in.flings, h)
	a.markNeedsPaint(h)
}

// stepBounce advances the spring: critically damped, so it returns without
// swinging through the end, x(t) = (x0 + (v0 + ω·x0)·t)·e^(−ω·t). It reports
// whether the container is still moving.
func (a *App) stepBounce(h scene.Handle, s *scrollState, now time.Duration) bool {
	t := (now - s.overStart).Seconds()
	const w = bounceStiffness
	x0, v0 := float64(s.overFrom), float64(s.overVel)
	e := math.Exp(-w * t)
	x := (x0 + (v0+w*x0)*t) * e
	v := (v0 - w*(v0+w*x0)*t) * e
	a.markNeedsPaint(h)
	if math.Abs(x) < 0.25 && math.Abs(v) < 20 {
		s.over, s.bouncing = 0, false
		return false
	}
	s.over = float32(x)
	return true
}
