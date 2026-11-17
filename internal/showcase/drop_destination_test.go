package showcase

import (
	"encoding/json"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// TestTheDropDestinationShotAimsAtTheNote runs the drag of the demo's Input
// in the window docs/gallery puts it in, so the screenshot shows the
// highlight it is meant to show.
func TestTheDropDestinationShotAimsAtTheNote(t *testing.T) {
	d, ok := Lookup("drop-destination")
	if !ok {
		t.Fatal("no demo")
	}
	var in struct {
		Steps []struct {
			Op    string
			X, Y  float32
			Kinds []string
		}
	}
	if err := json.Unmarshal([]byte(d.Input), &in); err != nil {
		t.Fatal(err)
	}
	h := gifttest.New(t, gifttest.Options{
		Root: func(ctx *gift.Context) gift.View {
			return ui.Window(ui.VStack(d.View(ctx)).Padding(24)).Align(geom.Alignment{X: 0.5, Y: 0.5})
		},
		Size: geom.Sz(float32(d.Width), float32(d.Height)),
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}),
	})
	for _, s := range in.Steps {
		if s.Op == "dragOver" {
			h.DragOverAt(geom.Pt(s.X, s.Y), gift.DropText)
		}
	}
	h.Find(gifttest.ByText("Drop text here")).AssertDropTargeted()
}
