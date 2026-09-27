package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

var liftFace = ui.RGB(10, 102, 217)

// liftedWidth is the device width the button's face is drawn at, and whether
// the glow is there.
func liftedWidth(h *gifttest.Harness) (float32, bool) {
	r, glow := liftedRect(h)
	return r.Width(), glow
}

func liftedRect(h *gifttest.Harness) (geom.Rect, bool) {
	var w geom.Rect
	glow := false
	list := h.List()
	for i, op := range h.Ops() {
		if (op.Kind == render.OpFillRoundRect || op.Kind == render.OpFillRect) && op.Color == render.Color(ui.ResolveColor(liftFace)) {
			w = list.DeviceXform(i).TransformRect(op.Bounds)
		}
		if op.Kind == render.OpShadow && op.Color.R > 0 && op.Color.R == op.Color.A {
			glow = true
		}
	}
	return w, glow
}

// TestALiftedButtonGrowsWhilePressedAndSpringsBack is the press feedback of
// iOS 26: larger and lit while the finger is down, back to rest after an
// overshoot, and not a single extra operation at rest.
func TestALiftedButtonGrowsWhilePressedAndSpringsBack(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(400, 200),
		Root: func(*gift.Context) gift.View {
			face := ui.ButtonStyle{Background: liftFace, CornerRadius: 20}
			return ui.VStack(ui.Button(ui.Text("Drucken"), func() {}).Style(face).PressedStyle(face).
				Lift(true).Frame(200, 40))
		},
	})
	h.Settle()
	r, _ := liftedRect(h)
	rest, glow := liftedWidth(h)
	if rest != 200 || glow {
		t.Fatalf("at rest the face is %v wide with glow=%v, want 200 and none", rest, glow)
	}

	h.PressAt(geom.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2))
	h.Advance(200 * time.Millisecond)
	pressed, glow := liftedWidth(h)
	if !(pressed > 208 && pressed < 216) || !glow {
		t.Errorf("pressed the face is %v wide with glow=%v, want about 212 and a glow", pressed, glow)
	}

	h.Release()
	h.Advance(170 * time.Millisecond)
	if w, _ := liftedWidth(h); !(w < 200) {
		t.Errorf("on the way back the face is %v wide; a spring overshoots below rest", w)
	}
	h.Advance(500 * time.Millisecond)
	if w, glow := liftedWidth(h); w != 200 || glow {
		t.Errorf("settled the face is %v wide with glow=%v, want 200 and none", w, glow)
	}
}
