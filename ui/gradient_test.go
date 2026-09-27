package ui_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// TestAGradientBackgroundIsOneFillWithAGradientMaterial, fading to clear
// included: a gradient to transparent must not be mistaken for none.
func TestAGradientBackgroundIsOneFillWithAGradientMaterial(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(200, 200),
		Root: func(*gift.Context) gift.View {
			return ui.VStack(ui.Box().Frame(100, 40).
				Background(ui.LinearGradient(ui.ColorSurface, ui.Fade(ui.ColorSurface, 0))).CornerRadius(8))
		},
	})
	h.Settle()
	list := h.List()
	found := false
	for _, op := range h.Ops() {
		if op.Kind != render.OpFillRoundRect || op.Material == 0 {
			continue
		}
		m := list.Material(op.Material)
		if m.Kind != render.MaterialGradient || !m.Gradient.To.IsTransparent() || op.Color.IsTransparent() {
			t.Fatalf("fill %+v with material %+v", op, m)
		}
		found = true
	}
	if !found {
		t.Fatal("no gradient fill in the list")
	}
}
