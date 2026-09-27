package render

import (
	"math"

	"github.com/worldiety/gift/geom"
)

// Cached layers
//
// A layer is a subtree whose drawing a backend may keep. The producer emits
// an [OpLayer] followed by the subtree's operations every frame, as it always
// would; what changes is the space those operations live in. Inside a layer
// the transform is [LayerXform]: local coordinates mapped onto the pixels of
// the layer's own texture, without the translation of anything above it. Two
// frames in which only the layer moved therefore produce identical
// operations, and a backend that compares them can composite the texture it
// already has instead of drawing the subtree again.
//
// That comparison is the whole invalidation scheme. There is no dirty flag to
// forget: a picture that finished loading, a caret that blinked or a label
// that changed changes the operations, and the layer is drawn again.
//
// What a layer cannot do is read what lies behind it. A material inside a
// layer has no backdrop but the layer's own content, so a backend draws a
// layer with a glass material in it straight into the frame, mapped out of
// layer space, and keeps nothing.

// MaxLayerSide is the largest texture side a layer is opened with. It is the
// texture limit of the GPUs gift is measured on – the VideoCore of a
// Raspberry Pi among them – and a layer that large is already more memory
// than it is likely to save in draw calls.
const MaxLayerSide = 4096

// LayerScale is the pixel density a layer is rasterised at under the
// transform parent: its larger scale factor, so that a layer composited at
// that transform maps one texel to one device pixel.
func LayerScale(parent geom.Affine2D) float32 {
	sx, sy := parent.ScaleFactors()
	s := max(sx, sy)
	if !(s > 0) || math.IsInf(float64(s), 0) {
		return 1
	}

	return s
}

// LayerSize is the texture size of a layer with the given bounds at scale s.
func LayerSize(bounds geom.Rect, s float32) (int, int) {
	w := int(math.Ceil(float64(bounds.Width() * s)))
	h := int(math.Ceil(float64(bounds.Height() * s)))

	return max(w, 0), max(h, 0)
}

// LayerXform maps the local space of a layer with the given bounds onto the
// pixels of its texture at scale s.
func LayerXform(bounds geom.Rect, s float32) geom.Affine2D {
	return geom.Translate(geom.Pt(-bounds.Min.X, -bounds.Min.Y)).Mul(geom.Scale(s, s))
}

// DeviceXform is the transform operation i is drawn with on the device,
// through the layers it lies in; for an operation outside every layer it is
// Xform(op.Xform). It walks the list from the start and is meant for tests
// and tools that ask where something landed, not for a frame path.
func (l *List) DeviceXform(i int) geom.Affine2D {
	xf := l.Xform(l.ops[i].Xform)
	if m, ok := l.layerSpace(i); ok {
		xf = xf.Mul(m)
	}

	return xf
}

// DeviceClip is the clip of operation i on the device, through the layers it
// lies in; see [List.DeviceXform].
func (l *List) DeviceClip(i int) geom.Rect {
	c := l.Clip(l.ops[i].Clip)
	inner := -1
	for j := 0; j < i; j++ {
		op := l.ops[j]
		if op.Kind != OpLayer {
			continue
		}
		if end := j + op.LayerOps(); i > end {
			j = end
			continue
		}
		inner = j
	}
	if inner < 0 {
		return c
	}
	m, _ := l.layerSpace(i)

	// What the innermost layer shows is bounded by its composite's clip,
	// which is itself in the layers around it.
	return m.TransformRect(c).Canon().Intersect(l.DeviceClip(inner))
}

// layerSpace maps the space of the innermost layer around operation i onto
// the device, and reports false when i lies in no layer.
func (l *List) layerSpace(i int) (geom.Affine2D, bool) {
	var m geom.Affine2D
	ok := false
	for j := 0; j < i; j++ {
		op := l.ops[j]
		if op.Kind != OpLayer {
			continue
		}
		if end := j + op.LayerOps(); i > end {
			j = end
			continue
		}
		comp := l.Xform(op.Xform)
		if ok {
			comp = comp.Mul(m)
		}
		s := op.LayerScale()
		m, ok = geom.Scale(1/s, 1/s).Mul(geom.Translate(op.Bounds.Min)).Mul(comp), true
	}

	return m, ok
}
