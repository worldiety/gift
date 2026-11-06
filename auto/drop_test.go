//go:build giftauto

package auto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var dropType = gift.RegisterType("auto.Drop")

// dropFixture is a 100 by 60 drop target for files that records what it got.
type dropFixture struct{ got *[]gift.DropData }

func (dropFixture) ViewType() gift.TypeID { return dropType }

func (f dropFixture) Build(*gift.BuildContext) gift.Element {
	n := &dropNode{f: f}
	return gift.Element{Key: "drop", Layouter: n, DropTarget: n}
}

type dropNode struct{ f dropFixture }

func (n *dropNode) Layout(*gift.LayoutContext, geom.Constraints) geom.Size { return geom.Sz(100, 60) }

func (n *dropNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	info, _ := ctx.Drop()
	if e.Kind == gift.EventDrop {
		*n.f.got = append(*n.f.got, info.Data)
	}
	return info.Kinds == 0 || info.Kinds&(gift.DropFile|gift.DropURL) != 0
}

func TestTheDragStepsReachADropTarget(t *testing.T) {
	var got []gift.DropData
	l := newFakeLoop(t, func(*gift.Context) gift.View { return dropFixture{got: &got} }, func() byte { return 0 })
	h := (&server{d: l.d}).handler()
	x, y := float32(50), float32(30)
	post := func(steps ...Step) {
		t.Helper()
		body, _ := json.Marshal(Batch{Steps: steps})
		if w := get(t, h, "POST", "/input", string(body)); w.Code != 200 {
			t.Fatalf("/input: status %d: %s", w.Code, w.Body.String())
		}
	}

	post(Step{Op: "dragOver", X: &x, Y: &y, Kinds: []string{"file"}})
	w := get(t, h, "GET", "/query?key=drop", "")
	if !strings.Contains(w.Body.String(), `"dropTargeted": true`) || !strings.Contains(w.Body.String(), `"dropTarget": true`) {
		t.Fatalf("/query after dragOver: %s", w.Body.String())
	}
	post(Step{Op: "dragLeave"})
	if w := get(t, h, "GET", "/query?key=drop", ""); strings.Contains(w.Body.String(), `"dropTargeted": true`) {
		t.Fatalf("/query after dragLeave: %s", w.Body.String())
	}
	post(Step{Op: "drop", X: &x, Y: &y, Files: []string{"/tmp/a b.txt"}, URLs: []string{"https://example.org"}})
	n, err := query(l.d, func(*gift.App) int { return len(got) })
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(got[0].Items) != 2 || got[0].Items[0].Path != "/tmp/a b.txt" || got[0].Items[1].URL != "https://example.org" {
		t.Fatalf("dropped %+v", got)
	}

	body, _ := json.Marshal(Batch{Steps: []Step{{Op: "dragOver", Kinds: []string{"pdf"}}}})
	if w := get(t, h, "POST", "/input", string(body)); w.Code == 200 {
		t.Error("an unknown drop kind was accepted")
	}
}
