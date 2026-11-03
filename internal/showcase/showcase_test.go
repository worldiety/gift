package showcase_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
)

// TestEveryDemoIsNamedAndInASection pins what the kitchen sink lists: a
// title, a summary and a section the site has.
func TestEveryDemoIsNamedAndInASection(t *testing.T) {
	groups := map[string]bool{}
	for _, g := range showcase.Groups() {
		groups[g.Key] = true
	}
	all := showcase.All()
	if len(all) < 50 {
		t.Fatalf("only %d demos", len(all))
	}
	for _, d := range all {
		if d.Title == "" || d.Summary == "" || !groups[d.Group] {
			t.Errorf("demo %q: title %q, summary %q, group %q", d.Name, d.Title, d.Summary, d.Group)
		}
	}
}

// TestEveryDemoBuildsAndDraws runs every demo for a moment at its own size,
// the way the gallery and the kitchen sink show it.
func TestEveryDemoBuildsAndDraws(t *testing.T) {
	if err := showcase.Samples(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	font := ui.MustFont(ui.FontQuery{Family: inter.Family})
	for _, d := range showcase.All() {
		t.Run(d.Name, func(t *testing.T) {
			defer ui.SetOnScreenKeyboard(nil, false) // the keyboard demo turns it on
			h := gifttest.New(t, gifttest.Options{
				Root: func(ctx *gift.Context) gift.View { return ui.Window(d.View(ctx)) },
				Size: geom.Sz(float32(d.Width), float32(d.Height)), Font: font, Theme: ui.LightTheme(),
			})
			h.AdvanceTicks(10, gifttest.FrameInterval)
			if len(h.Ops()) == 0 {
				t.Fatal("the demo drew nothing")
			}
		})
	}
}

// TestTheColourDemosShowTheColourTheyChoose taps the black swatch of the
// colour picker and of the picker behind the colour well. A demo that reads
// its colour with State.Get instead of Context.Read stores the new colour and
// never builds again, so the picker went on showing the old one, as if it
// took no tap.
func TestTheColourDemosShowTheColourTheyChoose(t *testing.T) {
	font := ui.MustFont(ui.FontQuery{Family: inter.Family})
	for _, name := range []string{"color-picker", "color-well"} {
		t.Run(name, func(t *testing.T) {
			d, ok := showcase.Lookup(name)
			if !ok {
				t.Fatalf("no demo %q", name)
			}
			h := gifttest.New(t, gifttest.Options{
				Root: func(ctx *gift.Context) gift.View { return ui.Window(d.View(ctx)) },
				Size: geom.Sz(float32(d.Width), float32(d.Height)), Font: font, Theme: ui.LightTheme(),
			})
			h.Settle()
			if name == "color-well" {
				h.Find(gifttest.ByKey("color-well")).Tap()
				h.AdvanceTicks(30, gifttest.FrameInterval)
			}
			g := h.Find(gifttest.ByKey("color-grid")).Bounds()
			h.TapAt(geom.Pt(g.Max.X-5, g.Min.Y+5))
			h.Settle()
			if hex := h.Find(gifttest.ByKey("color-hex")).Value(); hex[:7] != "#000000" {
				t.Fatalf("after a tap on black the picker shows %s", hex)
			}
		})
	}
}
