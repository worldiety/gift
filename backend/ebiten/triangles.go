package ebiten

import (
	"image/color"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// appendTriangles turns an [render.OpTriangles] into textured triangles of a
// white texture, so that they are drawn by the image material with their
// vertex colours and batch with each other.
//
// A triangle inside the clip goes out as it is; one across its edge is cut
// to the clip as a convex polygon and drawn in the colour of its first
// vertex, which for the few pixels of a particle at the edge of a map is
// what nobody sees; one outside it is skipped.
func (r *Renderer) appendTriangles(l *render.List, op render.Op) {
	first, count := op.Triangles()
	vs := l.Vertices(first, count)
	if len(vs) < 3 {
		r.skipEmptyBounds++
		return
	}
	clip := r.opClip(l, op.Clip)
	if clip.IsEmpty() {
		r.skipEmptyClip++
		return
	}
	xf := r.opXform(l, op.Xform)
	white := r.whiteTexture()
	r.material(MaterialImage, white)
	const u, v = 1.5, 1.5 // the centre of the white texture
	wrote := false
	for i := 0; i+2 < len(vs); i += 3 {
		var d [3][2]float32
		for k := range 3 {
			p := xf.Apply(geom.Pt(vs[i+k].X, vs[i+k].Y))
			d[k] = [2]float32{p.X, p.Y}
		}
		minX, maxX := min(d[0][0], d[1][0], d[2][0]), max(d[0][0], d[1][0], d[2][0])
		minY, maxY := min(d[0][1], d[1][1], d[2][1]), max(d[0][1], d[1][1], d[2][1])
		if maxX <= clip.Min.X || minX >= clip.Max.X || maxY <= clip.Min.Y || minY >= clip.Max.Y {
			continue
		}
		if minX >= clip.Min.X && maxX <= clip.Max.X && minY >= clip.Min.Y && maxY <= clip.Max.Y {
			base := uint32(len(r.verts))
			for k := range 3 {
				r.verts = append(r.verts, texturedVertex(d[k][0], d[k][1], u, v, r.ws.color(vs[i+k].Color)))
			}
			r.idx = append(r.idx, base, base+1, base+2)
			wrote = true
			continue
		}
		poly, other := &r.poly[0], &r.poly[1]
		for k := range 3 {
			poly[k] = clipVertex{dx: d[k][0], dy: d[k][1]}
		}
		n := clipHalfPlane(poly, 3, other, edgeLeft, clip.Min.X)
		poly, other = other, poly
		n = clipHalfPlane(poly, n, other, edgeRight, clip.Max.X)
		poly, other = other, poly
		n = clipHalfPlane(poly, n, other, edgeTop, clip.Min.Y)
		poly, other = other, poly
		n = clipHalfPlane(poly, n, other, edgeBottom, clip.Max.Y)
		poly = other
		if n < 3 {
			continue
		}
		base := uint32(len(r.verts))
		c := r.ws.color(vs[i].Color)
		for k := range n {
			r.verts = append(r.verts, texturedVertex(poly[k].dx, poly[k].dy, u, v, c))
		}
		for k := 1; k < n-1; k++ {
			r.idx = append(r.idx, base, base+uint32(k), base+uint32(k)+1)
		}
		wrote = true
	}
	if wrote {
		r.emitted++
		r.triangleOps++
	}
}

// whiteTexture is a small white image whose centre texel the triangles
// sample, created on first use.
func (r *Renderer) whiteTexture() *eb.Image {
	if r.white == nil {
		r.white = eb.NewImage(3, 3)
		r.white.Fill(color.White)
	}
	return r.white
}
