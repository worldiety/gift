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
