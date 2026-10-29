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

// gradientFills counts the fills in h's list that carry a gradient material.
func gradientFills(h *gifttest.Harness) int {
	n := 0
	for _, op := range h.Ops() {
		if op.Material != 0 && h.List().Material(op.Material).Kind == render.MaterialGradient {
			n++
		}
	}
	return n
}

// TestANilBackgroundClearsAGradient: ".Background(nil)" clears the
// background, and a gradient is a background. It used to clear only the
// start colour, so the box went on painting a fade from transparent to the
// end colour.
func TestANilBackgroundClearsAGradient(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(200, 200),
		Root: func(*gift.Context) gift.View {
			return ui.VStack(ui.Box().Frame(100, 40).
				Background(ui.LinearGradient(ui.ColorAccent, ui.ColorSurface)).Background(nil))
		},
	})
	h.Settle()
	for _, op := range h.Ops() {
		if op.Kind == render.OpFillRect || op.Kind == render.OpFillRoundRect {
			t.Fatalf("a cleared background still paints %+v", op)
		}
	}
}

// TestAButtonPaintsAGradientBackground covers both spellings of a gradient
// face in the normal state, the modifier and the aggregate style. Each used
// to paint the top colour flat.
func TestAButtonPaintsAGradientBackground(t *testing.T) {
	for name, b := range map[string]ui.ButtonView{
		"Background":        ui.Button(ui.Text("Go"), nil).Background(ui.LinearGradient(ui.ColorAccent, ui.ColorSurface)),
		"Style":             ui.Button(ui.Text("Go"), nil).Style(ui.ButtonStyle{Background: ui.ColorAccent, BackgroundTo: ui.ColorSurface}),
		"Style, themed top": ui.Button(ui.Text("Go"), nil).Style(ui.ButtonStyle{BackgroundTo: ui.ColorSurface}),
	} {
		t.Run(name, func(t *testing.T) {
			h := gifttest.New(t, gifttest.Options{
				Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(200, 200),
				Root: func(*gift.Context) gift.View { return ui.VStack(b) },
			})
			h.Settle()
			if gradientFills(h) != 1 {
				t.Fatalf("%d gradient fills, want 1", gradientFills(h))
			}
		})
	}
}

// TestAButtonRejectsAGlassBackground: a button's state styles cannot carry a
// material, so a glass face used to be dropped without a word.
func TestAButtonRejectsAGlassBackground(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Button.Background(ui.Glass()) did not panic")
		}
	}()
	ui.Button(ui.Text("Go"), nil).Background(ui.Glass())
}
