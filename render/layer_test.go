package render

import (
	"testing"

	"github.com/worldiety/gift/geom"
)

// TestBeginLayerEmitsItsContentInLayerSpace is the contract a backend relies
// on: the layer op carries the composite, the content is in texture pixels.
func TestBeginLayerEmitsItsContentInLayerSpace(t *testing.T) {
	var l List
	l.Reset()
	parent := l.PushXform(geom.Scale(2, 2).Mul(geom.Translate(geom.Pt(5, 0))))
	bounds := geom.Rc(10, 20, 110, 70)

	inner := l.BeginLayer(7, bounds, parent, 0)
	l.Add(Op{Kind: OpFillRect, Bounds: bounds, Color: Color{A: 1}, Clip: l.CurrentClip(), Xform: inner})
	l.EndLayer()
	l.Add(Op{Kind: OpFillRect, Bounds: bounds, Color: Color{A: 1}, Clip: l.CurrentClip(), Xform: parent})

	ops := l.Ops()
	if len(ops) != 3 || ops[0].Kind != OpLayer {
		t.Fatalf("want a layer, its content and one op after it, got %d ops", len(ops))
	}
	layer := ops[0]
	if layer.LayerOps() != 1 || layer.LayerScale() != 2 || layer.Image != 7 || layer.Xform != parent {
		t.Fatalf("layer op = %+v", layer)
	}
	if got, want := l.Xform(ops[1].Xform).TransformRect(bounds), geom.Rc(0, 0, 200, 100); got != want {
		t.Errorf("the content lands at %v in the texture, want %v", got, want)
	}
	if got, want := l.Clip(ops[1].Clip), geom.Rc(0, 0, 200, 100); got != want {
		t.Errorf("the content is clipped to %v, want the texture %v", got, want)
	}
	if ops[2].Clip != layer.Clip {
		t.Error("EndLayer did not restore the clip outside the layer")
	}
}

// TestALayerTooLargeIsNotOpened keeps an oversized subtree drawable: it is
// emitted as if there were no layer.
func TestALayerTooLargeIsNotOpened(t *testing.T) {
	var l List
	l.Reset()
	parent := l.PushXform(geom.Identity())
	got := l.BeginLayer(1, geom.Rc(0, 0, MaxLayerSide+1, 10), parent, 1)
	if got != parent {
		t.Error("an oversized layer changed the transform of its content")
	}
	l.EndLayer()
	if len(l.Ops()) != 0 {
		t.Errorf("an oversized layer emitted %d ops", len(l.Ops()))
	}
}

func TestEndLayerWithoutBeginPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an unbalanced EndLayer did not panic")
		}
	}()
	var l List
	l.Reset()
	l.EndLayer()
}

// TestDeviceXformSeesThroughLayers: a nested layer's content resolves to
// where it is drawn.
func TestDeviceXformSeesThroughLayers(t *testing.T) {
	var l List
	l.Reset()
	root := l.PushXform(geom.Scale(2, 2))
	outer := l.BeginLayer(1, geom.Rc(10, 10, 110, 110), l.PushXform(geom.Translate(geom.Pt(30, 0)).Mul(l.Xform(root))), 0)
	inner := l.BeginLayer(2, geom.Rc(20, 20, 60, 60), outer, 0)
	box := geom.Rc(25, 25, 35, 35)
	l.Add(Op{Kind: OpFillRect, Bounds: box, Color: Color{A: 1}, Clip: l.CurrentClip(), Xform: inner})
	l.EndLayer()
	l.EndLayer()

	i := len(l.Ops()) - 1
	if got, want := l.DeviceXform(i).TransformRect(box), geom.Rc(110, 50, 130, 70); got != want {
		t.Errorf("the content resolves to %v, want %v", got, want)
	}
	if got, want := l.DeviceClip(i), geom.Rc(100, 40, 180, 120); got != want {
		t.Errorf("the content is clipped to %v, want the inner layer %v", got, want)
	}
}
