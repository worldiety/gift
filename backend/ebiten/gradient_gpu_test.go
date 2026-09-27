//go:build giftgpu

package ebiten

import (
	"image/color"
	"testing"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// TestAGradientRunsFromTopToBottom: a rounded fill with a gradient material
// is its colour at the top, the gradient's end at the bottom, linear in
// between, and still rounded.
func TestAGradientRunsFromTopToBottom(t *testing.T) {
	dst := drawList(t, 64, 64, color.RGBA{0, 0, 0, 255}, func(l *render.List) {
		m := l.AddMaterial(render.LinearGradient(render.RGB(255, 0, 0), render.RGB(0, 0, 255)).Material())
		l.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(0, 0, 64, 64), CornerRadius: 12,
			Color: render.RGB(255, 0, 0), Material: m})
	})
	top, mid, bottom := rgbaAt(dst, 32, 0), rgbaAt(dst, 32, 32), rgbaAt(dst, 32, 63)
	if top.R < 245 || top.B > 10 {
		t.Errorf("top is %v, want red", top)
	}
	if bottom.B < 245 || bottom.R > 10 {
		t.Errorf("bottom is %v, want blue", bottom)
	}
	if d := int(mid.R) - int(mid.B); d > 8 || d < -8 || mid.R < 110 || mid.R > 145 {
		t.Errorf("middle is %v, want an even mix", mid)
	}
	if c := rgbaAt(dst, 0, 0); c.R > 20 || c.B > 20 {
		t.Errorf("the corner is %v; the gradient is not rounded", c)
	}
}
