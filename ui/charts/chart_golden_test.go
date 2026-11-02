package charts_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
)

// TestEveryChartLooksTheWayItLooks pins the demos of the component pages in
// both themes: the lines and their gaps, the bars and their categories, the
// area and its fade, the dots and the band, the axes, the legend.
//
//	GIFT_UPDATE_GOLDEN=1 go test -tags giftgpu -run TestEveryChart ./ui/charts
func TestEveryChartLooksTheWayItLooks(t *testing.T) {
	for _, theme := range []struct {
		name string
		t    ui.Theme
	}{{"light", ui.LightTheme()}, {"dark", ui.DarkTheme()}} {
		for _, name := range []string{"line-chart", "area-chart", "bar-chart", "scatter-chart", "pie-chart", "linked-charts"} {
			d, ok := showcase.Lookup(name)
			if !ok {
				t.Fatalf("no demo %q", name)
			}
			h := gifttest.New(t, gifttest.Options{
				Root: func(ctx *gift.Context) gift.View {
					return ui.Window(ui.VStack(d.View(ctx)).Padding(20))
				},
				Size:  geom.Sz(float32(d.Width)+40, float32(d.Height)+40),
				Theme: theme.t,
				Font:  ui.MustFont(ui.FontQuery{Family: inter.Family}),
			})
			h.Settle()
			h.AssertNoOverflow()
			h.AssertOpaque()
			h.Warm()
			h.AssertGolden("chart-" + name + "-" + theme.name)
		}
	}
}
