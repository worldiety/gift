package svg_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/svg"
)

// TestMain runs the tests on Ebitengine's main thread when the GPU half is
// built in; see gifttest.Main.
func TestMain(m *testing.M) { gifttest.Main(m) }

const wide = `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="32" viewBox="0 0 64 32"><rect width="64" height="32"/></svg>`

func mount(t *testing.T, v gift.View, size geom.Size) *gifttest.Harness {
	t.Helper()
	h := gifttest.New(t, gifttest.Options{View: v, Size: size,
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}), Theme: ui.LightTheme()})
	h.Settle()
	return h
}

func TestAViewIsAsLargeAsTheDocumentAndNoLargerThanTheRoom(t *testing.T) {
	im, err := svg.Parse([]byte(wide))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		v    gift.View
		room geom.Size
		want geom.Size
	}{
		{svg.View(im).Key("v"), geom.Sz(400, 300), geom.Sz(64, 32)},
		{svg.View(im).Frame(128, 0).Key("v"), geom.Sz(400, 300), geom.Sz(128, 64)},
		{svg.View(im).Frame(0, 16).Key("v"), geom.Sz(400, 300), geom.Sz(32, 16)},
		{svg.View(im).Frame(400, 0).Key("v"), geom.Sz(200, 300), geom.Sz(200, 100)},
	} {
		h := mount(t, ui.ZStack(c.v).Align(geom.TopLeading), c.room)
		if got := h.Find(gifttest.ByKey("v")).LayoutBounds().Size(); got != c.want {
			t.Errorf("in %v: %v, want %v", c.room, got, c.want)
		}
	}
}

func TestAViewIsNamed(t *testing.T) {
	im, _ := svg.Parse([]byte(wide))
	h := mount(t, svg.View(im).Label("Logo"), geom.Sz(100, 100))
	h.AssertExists(gifttest.ByText("Logo"))
}

// TestTheDemoLooksTheWayItLooks pins the picture of the component page in
// both themes, the large one rasterised on a goroutine.
//
//	GIFT_UPDATE_GOLDEN=1 go test -tags giftgpu -run TestTheDemo ./ui/svg
func TestTheDemoLooksTheWayItLooks(t *testing.T) {
	d, ok := showcase.Lookup("svg-image")
	if !ok {
		t.Fatal("no demo")
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
		awaitRasters(t, h)
		h.AssertNoOverflow()
		h.AssertOpaque()
		h.Warm()
		h.AssertGolden("svg-image-" + theme.name)
	}
}
