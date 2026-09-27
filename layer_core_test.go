package gift_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var layerType = gift.RegisterType("test.Layer")

// layerLeaf is transLeaf with Element.Layer set.
type layerLeaf struct {
	hidden bool
	spec   gift.TransitionSpec
}

func (v layerLeaf) ViewType() gift.TypeID { return layerType }

func (v layerLeaf) Build(*gift.BuildContext) gift.Element {
	return gift.Element{
		Key:        "layer",
		Layouter:   fixedLayouter{geom.Sz(100, 50)},
		Painter:    transPainter{transColour},
		Hidden:     v.hidden,
		Transition: v.spec,
		Layer:      true,
	}
}

// layerOf returns the layer op of list and the resolved transform of its
// content.
func layerOf(t *testing.T, list *render.List) (render.Op, geom.Affine2D) {
	t.Helper()
	ops := list.Ops()
	for i, op := range ops {
		if op.Kind != render.OpLayer {
			continue
		}
		if op.LayerOps() != 1 || i+1 >= len(ops) || ops[i+1].Color != transColour {
			t.Fatalf("the layer does not hold exactly the node's fill: %+v", op)
		}
		return op, list.Xform(ops[i+1].Xform)
	}
	t.Fatal("no layer in the list")
	return render.Op{}, geom.Affine2D{}
}

// TestALayerMovedByATransitionKeepsItsContent is what Element.Layer is for:
// during a slide only the composite moves, so a backend can reuse the picture.
func TestALayerMovedByATransitionKeepsItsContent(t *testing.T) {
	hidden := false
	now := time.Duration(0)
	app := gift.New(gift.Options{Root: func(*gift.Context) gift.View {
		return layerLeaf{hidden: hidden, spec: gift.TransitionSpec{Parked: geom.Pt(1, 0), Duration: testTransition}}
	}})
	frame := func() *render.List {
		app.BeginInput(now)
		if err := app.Update(geom.Sz(200, 200)); err != nil {
			t.Fatal(err)
		}
		return app.Paint()
	}

	restList := frame()
	restOp, restContent := layerOf(t, restList)
	restAt := restList.Xform(restOp.Xform).TransformRect(restOp.Bounds)
	if got := restOp.Bounds; got != geom.Rc(0, 0, 100, 50) {
		t.Errorf("the layer covers %v, want the node's bounds", got)
	}
	if got := restContent.TransformRect(restOp.Bounds); got != geom.Rc(0, 0, 100, 50) {
		t.Errorf("the content lands at %v in the texture", got)
	}

	hidden = true
	app.Invalidate()
	frame()
	now += testTransition / 2
	midList := frame()
	midOp, midContent := layerOf(t, midList)
	if midContent != restContent {
		t.Errorf("the content moved with the transition: %v, was %v", midContent, restContent)
	}
	midAt := midList.Xform(midOp.Xform).TransformRect(midOp.Bounds)
	if !(midAt.Min.X > restAt.Min.X) {
		t.Errorf("the composite did not move: at %v, was %v", midAt, restAt)
	}
}

// TestATransitionIsALayerOnlyWhileItMoves: a node without Element.Layer is
// painted as one for exactly the frames its transition moves it.
func TestATransitionIsALayerOnlyWhileItMoves(t *testing.T) {
	hasLayer := func(l *render.List) bool {
		for _, op := range l.Ops() {
			if op.Kind == render.OpLayer {
				return true
			}
		}
		return false
	}
	ta := newTransApp(t, gift.TransitionSpec{Parked: geom.Pt(1, 0), Duration: testTransition}, false)
	if hasLayer(ta.frame()) {
		t.Error("a node at rest was painted as a layer")
	}
	if !hasLayer(ta.set(true)) {
		t.Error("the first frame of the movement, in which the node has not moved yet, was not a layer")
	}
	if !hasLayer(ta.advance(testTransition / 2)) {
		t.Error("a node its transition is moving was not painted as a layer")
	}
	ta.set(false)
	ta.advance(2 * testTransition)
	if hasLayer(ta.frame()) {
		t.Error("the node stayed a layer after it came to rest")
	}
}
