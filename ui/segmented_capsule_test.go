package ui_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// TestACapsuleSegmentedControlIsFullyRound: tray radius is half the height,
// and the indicator floats on a shadow.
func TestACapsuleSegmentedControlIsFullyRound(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(400, 100),
		Root: func(*gift.Context) gift.View {
			return ui.VStack(ui.SegmentedControl(1, []string{"A", "B", "C"}, func(int) {}).Capsule(true).Frame(300, 40))
		},
	})
	h.Settle()
	var radii []float32
	shadows := 0
	for _, op := range h.Ops() {
		switch op.Kind {
		case render.OpFillRoundRect:
			radii = append(radii, op.CornerRadius)
		case render.OpShadow:
			shadows++
		}
	}
	if len(radii) < 2 || radii[0] != 20 || radii[1] != 18 {
		t.Errorf("tray and indicator radii are %v, want 20 and 18", radii)
	}
	if shadows != 1 {
		t.Errorf("the indicator has %d shadows, want 1", shadows)
	}
}

// TestACapsuleThumbStretchesOnItsWay: between two segments the indicator is
// wider than at rest, and round again when it arrives.
func TestACapsuleThumbStretchesOnItsWay(t *testing.T) {
	sel := 0
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(400, 100),
		Root: func(ctx *gift.Context) gift.View {
			s := ctx.State("sel", 0)
			sel = ctx.Read(s)
			return ui.VStack(ui.SegmentedControl(sel, []string{"A", "B"}, s.Set).Capsule(true).Frame(300, 40))
		},
	})
	h.Settle()
	indicator := func() geom.Rect {
		var rs []geom.Rect
		for _, op := range h.Ops() {
			if op.Kind == render.OpFillRoundRect {
				rs = append(rs, op.Bounds)
			}
		}
		return rs[1]
	}
	tray := func() geom.Rect {
		for _, op := range h.Ops() {
			if op.Kind == render.OpFillRoundRect {
				return op.Bounds
			}
		}
		return geom.Rect{}
	}()
	glyphRuns := func() int {
		n := 0
		for _, op := range h.Ops() {
			if op.Kind == render.OpGlyphs {
				n++
			}
		}
		return n
	}
	rest := indicator().Width()
	restRuns := glyphRuns()
	h.ClickAt(geom.Pt(tray.Max.X-20, (tray.Min.Y+tray.Max.Y)/2))
	h.Advance(ui.ControlAnimation / 2)
	if w := indicator().Width(); !(w > rest*1.2) {
		t.Errorf("half way the thumb is %v wide, at rest %v; it does not stretch", w, rest)
	}
	if n := glyphRuns(); n <= restRuns {
		t.Errorf("half way %d label runs are drawn, at rest %d; the lens draws none", n, restRuns)
	}
	h.Advance(ui.ControlAnimation)
	if n := glyphRuns(); n != restRuns {
		t.Errorf("arrived %d label runs are drawn, want %d; the lens stays", n, restRuns)
	}
	if w := indicator().Width(); w != rest || sel != 1 {
		t.Errorf("arrived the thumb is %v wide (rest %v), selection %d", w, rest, sel)
	}
}
