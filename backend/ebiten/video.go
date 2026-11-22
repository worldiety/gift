package ebiten

import (
	_ "embed"
	"fmt"
	"image"
	"strings"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

//go:embed video.kage
var videoConvertSrc []byte

//go:embed videodirect.kage
var videoDirectSrc []byte

// VideoCache is the [render.Videos] of this backend: one YUV texture per video,
// written once per new picture, and an RGB texture beside it for the frames
// that cannot convert in the pass that draws.
//
// # Two ways to draw a picture
//
// A picture is YUV until a shader converts it, and every full screen pass
// costs on a Raspberry Pi, measured in the project plan, section 25. So an
// [render.OpVideo] is drawn one of two ways, chosen per operation by
// [Renderer.appendVideo]:
//
//   - Converted straight into the target, by videodirect.kage, when the
//     operation maps one picture pixel onto one device pixel, is square,
//     untinted and progressive. That is a full screen video on a screen of
//     its size, the case where a second pass is a third of the frame.
//   - Through the RGB texture otherwise, converted by video.kage at the
//     picture's own size once per new picture, and drawn from there like any
//     image, with Ebitengine's filter, rounded corners and tint. Scaled up,
//     this is the cheaper way: the conversion runs over the smaller picture
//     and the texture unit interpolates, where a fused shader would
//     interpolate luma itself at four fetches a pixel of the larger one.
//
// # Not budgeted
//
// A video holds two textures for as long as it lives and is never evicted:
// it is one picture, its producer releases it, and a budget that evicted the
// texture a playing video writes into every frame would be a budget that
// only costs.
type VideoCache struct {
	recs []videoRec // index 0 is the invalid id
	free []uint32

	convert *eb.Shader
	direct  *eb.Shader

	// ws is the working space of the renderer, nil for sRGB. In another
	// one the pictures are converted by wide variants of the two shaders;
	// see [VideoCache.setSpace]. base keeps the sRGB pair.
	ws   *workingSpace
	base [2]*eb.Shader

	// presented counts calls to Present, for tests and diagnostics.
	presented uint64
	// conversions counts runs of the RGB conversion.
	conversions uint64
}

type videoRec struct {
	live bool
	gen  uint32

	yuv *eb.Image
	rgb *eb.Image
	f   render.VideoFrame // without Pix

	// seq is bumped by every Present and is what a cached layer compares;
	// converted is the seq the RGB texture shows.
	seq, converted uint64

	// uniforms are the shader parameters of the current picture, built by
	// Present so that drawing allocates nothing.
	uniforms map[string]any
}

// NewVideoCache compiles the two conversion shaders.
func NewVideoCache() (*VideoCache, error) {
	conv, err := eb.NewShader(videoConvertSrc)
	if err != nil {
		return nil, fmt.Errorf("gift/backend/ebiten: compiling the video conversion shader: %w", err)
	}
	dir, err := eb.NewShader(videoDirectSrc)
	if err != nil {
		return nil, fmt.Errorf("gift/backend/ebiten: compiling the direct video shader: %w", err)
	}
	return &VideoCache{recs: make([]videoRec, 1), convert: conv, direct: dir, base: [2]*eb.Shader{conv, dir}}, nil
}

// setSpace makes the two shaders convert into the working space ws, or into
// sRGB for nil.
//
// # Why a variant and not a uniform
//
// The picture's RGB is taken to be sRGB, as on every other path, and in
// another working space it is converted on the GPU: the curve undone, a
// matrix, the curve again, six powers a pixel. A Mac does not notice. The
// Raspberry Pi the direct shader exists for paints sRGB, and a uniform
// switch would leave the code and the branch in its shader; the wide pair
// is compiled only where it is used, from the same source.
func (c *VideoCache) setSpace(ws *workingSpace) error {
	if ws == c.ws {
		return nil
	}
	conv, dir := c.base[0], c.base[1]
	if ws != nil {
		var err error
		if conv, err = eb.NewShader(wideVideoSource(videoConvertSrc)); err != nil {
			return fmt.Errorf("gift/backend/ebiten: compiling the wide video conversion shader: %w", err)
		}
		if dir, err = eb.NewShader(wideVideoSource(videoDirectSrc)); err != nil {
			return fmt.Errorf("gift/backend/ebiten: compiling the wide direct video shader: %w", err)
		}
	}
	c.ws, c.convert, c.direct = ws, conv, dir
	for i := range c.recs {
		if r := &c.recs[i]; r.live && r.uniforms != nil {
			r.uniforms = videoUniforms(r.f, ws)
			r.converted = 0
		}
	}
	return nil
}

// The line both video shaders end with, and what the wide variants add.
const (
	videoReturn     = "return vec4(clamp(vec3(dot(RowR, yuv), dot(RowG, yuv), dot(RowB, yuv)), 0, 1), 1)"
	videoWideReturn = "return vec4(toWorking(clamp(vec3(dot(RowR, yuv), dot(RowG, yuv), dot(RowB, yuv)), 0, 1)), 1)"
	videoUniformsAt = "var RowB vec4\n"
	videoWide       = `var RowB vec4

// ToR, ToG and ToB turn linear sRGB into the linear working space.
var ToR vec3
var ToG vec3
var ToB vec3

// toWorking converts an sRGB colour into the working space, whose curve is
// the sRGB curve.
func toWorking(c vec3) vec3 {
	lin := mix(c/12.92, pow((c+0.055)/1.055, vec3(2.4)), step(vec3(0.04045), c))
	w := clamp(vec3(dot(ToR, lin), dot(ToG, lin), dot(ToB, lin)), 0, 1)
	return mix(w*12.92, 1.055*pow(w, vec3(1/2.4))-0.055, step(vec3(0.0031308), w))
}
`
)

// wideVideoSource is a video shader that converts into the working space.
// It panics when the shader no longer has the lines it is made from, which
// TestWideVideoShadersCompile notices before anyone else.
func wideVideoSource(src []byte) []byte {
	s := string(src)
	if strings.Count(s, videoReturn) != 1 || strings.Count(s, videoUniformsAt) != 1 {
		panic("gift/backend/ebiten: a video shader no longer ends the way its wide variant expects")
	}
	s = strings.Replace(s, videoReturn, videoWideReturn, 1)
	s = strings.Replace(s, videoUniformsAt, videoWide, 1)
	return []byte(s)
}

// Present implements [render.Videos].
func (c *VideoCache) Present(h render.ImageHandle, f render.VideoFrame) (render.ImageHandle, bool) {
	if f.IsEmpty() || f.Stride%4 != 0 {
		return h, false
	}
	tw, th := f.Stride/4, f.Height+(f.Height+1)/2
	if f.Pix != nil && len(f.Pix) < f.Stride*th {
		return h, false
	}
	i := c.slot(h)
	if i == 0 {
		if f.Pix == nil {
			return h, false
		}
		i = c.alloc()
	}
	r := &c.recs[i]
	if f.Pix != nil {
		if r.yuv == nil || r.yuv.Bounds().Dx() != tw || r.yuv.Bounds().Dy() != th {
			if r.yuv != nil {
				r.yuv.Deallocate()
			}
			// Unmanaged: a texture of its own and not a region of
			// Ebitengine's atlas, which a picture rewritten every frame
			// would only fragment.
			r.yuv = eb.NewImageWithOptions(image.Rect(0, 0, tw, th), &eb.NewImageOptions{Unmanaged: true})
		}
		r.yuv.WritePixels(f.Pix[:f.Stride*th])
	} else if sameParams(r.f, f) {
		return render.ImageHandle{ID: render.ImageID(i), Gen: r.gen}, true
	}
	if r.uniforms == nil || !sameParams(r.f, f) {
		r.uniforms = videoUniforms(f, c.ws)
	}
	r.f = withoutPix(f)
	r.seq++
	c.presented++
	return render.ImageHandle{ID: render.ImageID(i), Gen: r.gen}, true
}

func withoutPix(f render.VideoFrame) render.VideoFrame { f.Pix = nil; return f }

// sameParams compares everything of two frames but the pixels.
func sameParams(a, b render.VideoFrame) bool {
	return a.Stride == b.Stride && a.Width == b.Width && a.Height == b.Height &&
		a.Layout == b.Layout && a.Matrix == b.Matrix && a.FullRange == b.FullRange && a.Field == b.Field
}

// Resolve implements [render.Videos].
func (c *VideoCache) Resolve(h render.ImageHandle) (render.ImageID, bool) {
	i := c.slot(h)
	return render.ImageID(i), i != 0
}

// Release implements [render.Videos].
func (c *VideoCache) Release(h render.ImageHandle) {
	i := c.slot(h)
	if i == 0 {
		return
	}
	r := &c.recs[i]
	if r.yuv != nil {
		r.yuv.Deallocate()
	}
	if r.rgb != nil {
		r.rgb.Deallocate()
	}
	*r = videoRec{gen: r.gen + 1}
	c.free = append(c.free, i)
}

func (c *VideoCache) slot(h render.ImageHandle) uint32 {
	i := uint32(h.ID)
	if i == 0 || int(i) >= len(c.recs) || !c.recs[i].live || c.recs[i].gen != h.Gen {
		return 0
	}
	return i
}

func (c *VideoCache) alloc() uint32 {
	if n := len(c.free); n > 0 {
		i := c.free[n-1]
		c.free = c.free[:n-1]
		c.recs[i].live = true
		return i
	}
	c.recs = append(c.recs, videoRec{live: true, gen: 1})
	return uint32(len(c.recs) - 1)
}

// rec returns the live record behind an id of the frame in progress.
func (c *VideoCache) rec(id render.ImageID) *videoRec {
	i := uint32(id)
	if i == 0 || int(i) >= len(c.recs) || !c.recs[i].live || c.recs[i].yuv == nil {
		return nil
	}
	return &c.recs[i]
}

// stamp identifies the picture behind id for the layer cache: it changes
// with every presented picture.
func (c *VideoCache) stamp(id render.ImageID) uint64 {
	r := c.rec(id)
	if r == nil {
		return 0
	}
	return r.seq<<32 | uint64(id) | 1<<63
}

// rgbTexture returns the RGB conversion of the current picture of r, running
// the conversion when the picture changed since the last one.
func (c *VideoCache) rgbTexture(r *videoRec) *eb.Image {
	w, h := r.f.Width, r.f.Height
	if r.rgb == nil || r.rgb.Bounds().Dx() != w || r.rgb.Bounds().Dy() != h {
		if r.rgb != nil {
			r.rgb.Deallocate()
		}
		r.rgb = eb.NewImageWithOptions(image.Rect(0, 0, w, h), &eb.NewImageOptions{Unmanaged: true})
		r.converted = 0
	}
	if r.converted == r.seq {
		return r.rgb
	}
	fw, fh := float32(w), float32(h)
	vs := [4]eb.Vertex{
		{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: fw, DstY: 0, SrcX: fw, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 0, DstY: fh, SrcX: 0, SrcY: fh, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: fw, DstY: fh, SrcX: fw, SrcY: fh, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	op := &eb.DrawTrianglesShaderOptions{Blend: eb.BlendCopy, Uniforms: r.uniforms}
	op.Images[0] = r.yuv
	r.rgb.DrawTrianglesShader(vs[:], []uint16{0, 1, 2, 1, 2, 3}, c.convert, op)
	r.converted = r.seq
	c.conversions++
	return r.rgb
}

// videoUniforms are the shader parameters of a picture. Both shaders declare
// the same names; each ignores what it does not use.
func videoUniforms(f render.VideoFrame, ws *workingSpace) map[string]any {
	r, g, b := videoMatrix(f.Matrix, f.FullRange)
	u := map[string]any{
		"Layout":       float32(f.Layout),
		"Width":        float32(f.Width),
		"Height":       float32(f.Height),
		"Field":        float32(f.Field),
		"ChromaLinear": float32(1),
		"LumaLinear":   float32(0),
		"RowR":         r[:],
		"RowG":         g[:],
		"RowB":         b[:],
	}
	if ws != nil {
		tr, tg, tb := ws.matrix()
		u["ToR"], u["ToG"], u["ToB"] = tr[:], tg[:], tb[:]
	}
	return u
}

// videoMatrix returns the rows that turn normalised (Y, U, V, 1) into R, G and
// B, from the luma coefficients of the matrix and the value range.
func videoMatrix(m render.VideoMatrix, full bool) (r, g, b [4]float32) {
	kr, kb := 0.2126, 0.0722 // BT.709
	switch m {
	case render.BT601:
		kr, kb = 0.299, 0.114
	case render.BT2020:
		kr, kb = 0.2627, 0.0593
	}
	kg := 1 - kr - kb
	ys, cs, yo := 1.0, 1.0, 0.0
	if !full {
		ys, cs, yo = 255.0/219, 255.0/224, 16.0/255
	}
	const co = 128.0 / 255
	rv := 2 * (1 - kr) * cs
	bu := 2 * (1 - kb) * cs
	gu := -2 * kb * (1 - kb) / kg * cs
	gv := -2 * kr * (1 - kr) / kg * cs
	row := func(cu, cv float64) [4]float32 {
		return [4]float32{float32(ys), float32(cu), float32(cv), float32(-ys*yo - (cu+cv)*co)}
	}
	return row(0, rv), row(gu, gv), row(bu, 0)
}

// appendVideo turns one [render.OpVideo] into a quad: drawn by the direct
// conversion shader where that is possible, as an image of the converted
// texture everywhere else. See [VideoCache] for the rule.
func (r *Renderer) appendVideo(l *render.List, op render.Op) {
	if op.Color.IsTransparent() {
		r.skipTransparent++
		return
	}
	b := op.Bounds
	if b.IsEmpty() {
		r.skipEmptyBounds++
		return
	}
	clip := r.opClip(l, op.Clip)
	if clip.IsEmpty() {
		r.skipEmptyClip++
		return
	}
	var rec *videoRec
	if r.videos != nil {
		rec = r.videos.rec(op.Image)
	}
	if rec == nil {
		r.skipNoImage++
		return
	}
	xf := r.opXform(l, op.Xform)
	radius := min(op.CornerRadius, b.Width()*0.5, b.Height()*0.5)
	if r.videoDirect(rec, b, xf, radius, op) {
		r.material(MaterialVideo, rec.yuv)
		r.curVideo = rec
		src := geom.Rc(0, 0, float32(rec.f.Width), float32(rec.f.Height))
		if !r.appendTexturedQuad(b, src, clip, xf, op.Color) {
			r.skipOutsideClip++
			return
		}
		r.videoOps++
		r.videoDirectOps++
		r.emitted++
		return
	}
	img := r.videos.rgbTexture(rec)
	if r.drawImageTexture(l, op, img, rec.f.Width, rec.f.Height, clip, xf, radius) {
		r.videoOps++
		r.emitted++
	}
}

// videoDirect decides whether op can be converted straight into the target:
// one picture pixel per device pixel, an axis aligned transform, square
// corners, no tint and a progressive picture. The conversion shader has no
// filter, no rounding and no bob; everything else takes the converted
// texture.
func (r *Renderer) videoDirect(rec *videoRec, b geom.Rect, xf geom.Affine2D, radius float32, op render.Op) bool {
	if radius > 0 || rec.f.Field != render.FieldProgressive || op.Fit != render.ImageStretch {
		return false
	}
	if xf.B != 0 || xf.C != 0 || xf.A <= 0 || xf.D <= 0 {
		return false
	}
	if op.Color != opaqueWhite {
		return false
	}
	dw, dh := b.Width()*xf.A, b.Height()*xf.D
	return videoAbs(dw-float32(rec.f.Width)) < 0.5 && videoAbs(dh-float32(rec.f.Height)) < 0.5
}

// opaqueWhite is the tint that leaves a picture alone.
var opaqueWhite = render.RGB(255, 255, 255)

func videoAbs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}
