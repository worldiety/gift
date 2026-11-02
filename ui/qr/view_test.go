package qr_test

import (
	"math"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/qr"
)

// TestMain runs the tests on Ebitengine's main thread when the GPU half is
// built in; see gifttest.Main.
func TestMain(m *testing.M) { gifttest.Main(m) }

func mount(t *testing.T, v gift.View, size geom.Size, density float64) *gifttest.Harness {
	t.Helper()
	h := gifttest.New(t, gifttest.Options{View: v, Size: size, Density: density,
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}), Theme: ui.LightTheme()})
	h.Settle()
	return h
}

const link = "https://worldiety.github.io/gift/"

func TestAViewIsFourPointsAModuleAndNoLargerThanTheRoom(t *testing.T) {
	code, _ := qr.Encode(link, qr.M)
	natural := float32((code.Size() + 2*qr.QuietZone) * 4)
	for _, c := range []struct {
		v    gift.View
		room geom.Size
		want float32
	}{
		{qr.View(link).Key("v"), geom.Sz(400, 400), natural},
		{qr.View(link).Size(240).Key("v"), geom.Sz(400, 400), 240},
		{qr.View(link).Size(240).Key("v"), geom.Sz(400, 100), 100},
	} {
		h := mount(t, ui.ZStack(c.v).Align(geom.TopLeading), c.room, 1)
		if got := h.Find(gifttest.ByKey("v")).LayoutBounds().Size(); got != geom.Sz(c.want, c.want) {
			t.Errorf("in %v: %v, want %v", c.room, got, c.want)
		}
	}
	// A flexible code takes the room of the stack and draws a square in it.
	h := mount(t, ui.HStack(qr.View(link).Flex(1)).Frame(300, 200), geom.Sz(400, 400), 1)
	h.AssertOps("a square of 200", 1, func(op render.Op) bool {
		return op.Kind == render.OpFillRoundRect && op.Bounds == geom.Rc(50, 0, 250, 200)
	})
}

func TestTheModulesAreWholePixels(t *testing.T) {
	code, _ := qr.Encode(link, qr.M)
	runs := 0
	for y := range code.Size() {
		for x := range code.Size() {
			if code.Dark(x, y) && !code.Dark(x-1, y) {
				runs++
			}
		}
	}
	for _, density := range []float64{1, 1.25, 2} {
		// 237 points is no whole number of modules at any density.
		h := mount(t, qr.View(link).Size(237).Label("Link"), geom.Sz(300, 300), density)
		h.AssertExists(gifttest.ByText("Link"))
		black := func(op render.Op) bool { return op.Kind == render.OpFillRect && op.Color == render.RGBA(0, 0, 0, 255) }
		h.AssertOps("one rectangle per run of dark modules", runs, black)
		scale := float64(h.Density())
		whole := func(v float32) bool {
			d := float64(v) * scale
			return math.Abs(d-math.Round(d)) < 1e-3
		}
		h.AssertOps("rectangles off the pixel grid", 0, func(op render.Op) bool {
			b := op.Bounds
			return black(op) && !(whole(b.Min.X) && whole(b.Min.Y) && whole(b.Max.X) && whole(b.Max.Y))
		})
		h.AssertOps("the white square", 1, func(op render.Op) bool {
			return op.Kind == render.OpFillRoundRect && op.Color == render.RGBA(255, 255, 255, 255)
		})
	}
}

func TestATextTooLongShowsTheSquareAlone(t *testing.T) {
	long := make([]byte, 3000)
	for i := range long {
		long[i] = 'x'
	}
	h := mount(t, qr.View(string(long)).Size(100), geom.Sz(200, 200), 1)
	h.AssertOps("modules", 0, func(op render.Op) bool { return op.Kind == render.OpFillRect })
	h.AssertOps("the white square", 1, func(op render.Op) bool { return op.Kind == render.OpFillRoundRect })
}

// TestTheDemoLooksTheWayItLooks pins the picture of the component page in
// both themes.
//
//	GIFT_UPDATE_GOLDEN=1 go test -tags giftgpu -run TestTheDemo ./ui/qr
func TestTheDemoLooksTheWayItLooks(t *testing.T) {
	d, ok := showcase.Lookup("qr-code")
	if !ok {
		t.Fatal("no demo qr-code")
	}
	for _, theme := range []struct {
		name string
		t    ui.Theme
	}{{"light", ui.LightTheme()}, {"dark", ui.DarkTheme()}} {
		h := gifttest.New(t, gifttest.Options{
			Root:  func(ctx *gift.Context) gift.View { return ui.Window(ui.VStack(d.View(ctx)).Padding(20)) },
			Size:  geom.Sz(float32(d.Width)+40, float32(d.Height)+40),
			Theme: theme.t,
			Font:  ui.MustFont(ui.FontQuery{Family: inter.Family}),
		})
		h.Settle()
		h.AssertNoOverflow()
		h.AssertOpaque()
		h.Warm()
		h.AssertGolden("qr-code-" + theme.name)
	}
}
