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

var bounceMark = ui.RGB(200, 30, 30)

// firstRowY is where the first row of the list is drawn.
func firstRowY(h *gifttest.Harness) float32 {
	list := h.List()
	for i, op := range h.Ops() {
		if op.Kind == render.OpFillRect && op.Color == render.Color(ui.ResolveColor(bounceMark)) {
			return list.DeviceXform(i).TransformRect(op.Bounds).Min.Y
		}
	}
	return -1
}

// TestABouncingListIsPulledPastItsTopAndSpringsBack is the rubber band of
// iOS: pulled down at the top, the list follows with resistance; let go, it
// springs home.
func TestABouncingListIsPulledPastItsTopAndSpringsBack(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(300, 300),
		Root: func(*gift.Context) gift.View {
			rows := []gift.View{ui.Box().Frame(300, 60).Background(bounceMark)}
			for range 20 {
				rows = append(rows, ui.Box().Frame(300, 60))
			}
			return ui.VScroll(ui.VStack(rows...)).Bounce(true).Flex(1)
		},
	})
	h.Settle()
	rest := firstRowY(h)

	h.PressAt(geom.Pt(150, 100))
	for y := float32(110); y <= 200; y += 10 {
		h.MoveTo(geom.Pt(150, y))
	}
	h.Advance(16 * time.Millisecond)
	pulled := firstRowY(h)
	if !(pulled > rest+20 && pulled < rest+100) {
		t.Fatalf("pulled 100 past the top the first row is at %v (rest %v); want it to follow with resistance", pulled, rest)
	}

	h.Release()
	h.Advance(100 * time.Millisecond)
	if y := firstRowY(h); !(y < pulled && y > rest) {
		t.Errorf("100 ms after release the first row is at %v; want it on its way home from %v", y, pulled)
	}
	h.Advance(time.Second)
	if y := firstRowY(h); y != rest {
		t.Errorf("settled the first row is at %v, want %v", y, rest)
	}
	if d := h.App().Diagnostics(); d.Animating {
		t.Error("the list keeps animating after it came home")
	}
}
