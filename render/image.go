package render

import "github.com/worldiety/gift/geom"

// ImageID names one image resource that is resident in a backend right now.
//
// It is an index into backend owned storage and nothing else: no pointer, no
// interface and no string, because it travels inside an [Op] and [Op] is plain
// old data. Zero is the invalid id and draws nothing.
//
// An id is only meaningful *inside the frame that obtained it*. A backend may
// evict a texture between two frames and hand the same index out again for a
// different picture, so nothing may keep a bare ImageID across a frame
// boundary. What may be kept is an [ImageHandle], which carries the generation
// that makes the staleness detectable.
type ImageID uint32

// ImageHandle is a durable reference to an image resource.
//
// It is the pair an index based resource pool needs: the slot and the
// generation of the occupant of that slot. [Images.Resolve] compares the
// generation and reports false for a handle whose texture has been evicted,
// which turns "my texture is gone" from a wrong picture into a placeholder.
//
// This is the same shape [internal/scene] uses for retained nodes and for the
// same reason: a map from an application key to a pointer would let a consumer
// keep a dead texture alive, and a bare index would let it draw somebody
// else's.
//
// The zero handle is invalid.
type ImageHandle struct {
	// ID is the slot.
	ID ImageID
	// Gen is the generation of the occupant of the slot at the time the
	// handle was issued.
	Gen uint32
}

// IsZero reports whether h refers to nothing.
func (h ImageHandle) IsZero() bool { return h.ID == 0 }

// Pixels is a renderer neutral view of decoded image data.
//
// Premultiplied RGBA, eight bits per channel, which is what the project plan,
// section 8, fixes at the backend boundary and what asset.Thumbnail already
// produces. The slice is *borrowed* for the duration of the call it is passed
// to: a backend copies what it needs into its own storage and must not retain
// it, because the owner of the pixels releases them as soon as the call
// returns.
//
// It is a struct of four words rather than an *image.RGBA so that render stays
// free of image/draw and so that passing one allocates nothing.
type Pixels struct {
	// Pix is the premultiplied RGBA data, Stride bytes per row.
	Pix []byte
	// W and H are the pixel dimensions.
	W, H int
	// Stride is the distance between two rows in bytes.
	Stride int
}

// IsEmpty reports whether there is nothing to upload.
func (p Pixels) IsEmpty() bool {
	return p.W <= 0 || p.H <= 0 || p.Stride < p.W*4 || len(p.Pix) < (p.H-1)*p.Stride+p.W*4
}

// Bytes is the logical size of the pixel data.
//
// Logical, and the word matters. The project plan, section 11, is explicit
// that this number is not a GPU budget: padding, fragmentation, atlas growth,
// intermediate targets and staging all need memory this does not count. It is
// what a byte budget is *accounted* in, not what the driver allocates.
func (p Pixels) Bytes() int64 { return int64(p.W) * int64(p.H) * 4 }

// Images is the image resource service of a backend.
//
// It is the renderer neutral half of the GPU image path: a view asks for a
// picture to be resident, gets a handle, and puts the handle's [ImageID] into
// an [OpImage]. The view never sees a texture, a page or an upload queue, and
// the backend never sees an asset, a cache key or a tile.
//
// # Threading
//
// An Images belongs to the UI executor, like the [Backend] that owns it. Every
// method is called from a painter, that is from inside the draw callback.
//
// # The upload budget is per drawn frame
//
// [Images.Acquire] is admission controlled, and the admission window is one
// *drawn* frame and not one update. Ebitengine may run several updates before
// it draws, and the project plan, section 11, says so explicitly; an upload
// budget spent per update would therefore be spent several times over for one
// frame's worth of pictures. Painting is the only thing that happens exactly
// once per drawn frame, so the budget is reset by [Backend.BeginFrame] and
// consumed by the painters of that frame. A refused Acquire is not an error:
// the view draws its placeholder and asks again next frame.
type Images interface {
	// Resolve reports whether h still refers to a live resource and returns
	// the id to put into an operation.
	//
	// It also marks the resource as used by the frame in progress, which is
	// what keeps it from being evicted underneath the operations that
	// reference it. A caller that resolves a handle and then does not draw it
	// has merely kept it alive one frame longer.
	Resolve(h ImageHandle) (ImageID, bool)

	// Acquire uploads px and returns a handle to the resulting resource.
	//
	// It reports false when the per frame upload budget is exhausted, when
	// the pixels are malformed or when no room could be made without evicting
	// something the frame in progress already draws. All three are ordinary,
	// bounded outcomes and none of them is an error: the caller draws a
	// placeholder and tries again in the next frame.
	//
	// The pixels are consumed before the call returns; see [Pixels].
	Acquire(px Pixels) (ImageHandle, bool)

	// Deallocate releases the resource h refers to, or does nothing when the
	// handle is already stale.
	//
	// It is the explicit release of the project plan, section 11, exposed to
	// the owner of a handle: a view that knows a picture will not be needed
	// again — a detail view that closed, a source whose revision changed —
	// gives the memory back now instead of waiting for the least recently
	// used sweep to notice. A resource that the frame in progress has already
	// drawn is *not* released immediately; see the backend implementation.
	Deallocate(h ImageHandle)
}

// ImageFit is how an [OpImage] maps its texture onto [Op.Bounds].
//
// # Why the display list learned a crop after all
//
// [OpImage] used to state that a crop is a clip: the producer pushed the tile
// rectangle, emitted an oversized Bounds and popped, and the backend trimmed
// the geometry. That is exact for a rectangle and it stopped being enough the
// moment a picture had to be *rounded*. The rounded shape belongs to the
// rectangle the picture is seen in — the tile — and a clip cannot carry it:
// the clip table holds the intersection of every enclosing clip, so a tile
// half scrolled out of its gallery would have been rounded where the viewport
// cut it rather than where its own corners are. The operation therefore has to
// know the destination rectangle and the crop separately, and there were three
// ways to say so:
//
//   - A source rectangle in [Op]. Sixteen bytes on every operation of every
//     list, which is the growth [OpImage] refused the first time, for reasons
//     that have not changed.
//   - A side table of source rectangles and an index into it. Four bytes on
//     every operation, and the only crop gift ever produces is the one below.
//   - This: one byte that names the mapping, in the three bytes of padding
//     [Op] already had after Kind. It costs nothing — TestOpIsStillPlainOldData
//     still says 72 — and the backend computes the crop from the texture size
//     it already knows, with the arithmetic of [ImageFit.Source] so that the
//     two sides cannot drift.
//
// The clip is still the answer for any *other* crop: an off centre or an
// animated one is an oversized Bounds under a clip, exactly as before, and
// only its corners are square.
type ImageFit uint8

const (
	// ImageStretch maps the whole texture onto Bounds, whatever the aspect
	// ratios. It is the zero value and the historical meaning of [OpImage];
	// a letterboxed picture is a smaller Bounds, which the producer computes.
	ImageStretch ImageFit = iota
	// ImageCover preserves the aspect ratio of the texture, scales it until
	// it covers Bounds and crops the overhang symmetrically, so the centre of
	// the picture is the centre of Bounds. Bounds is then the rectangle the
	// picture is *seen* in, which is what a rounded corner is measured on.
	ImageCover
)

// Source returns the rectangle of a w by h texture, in texels, that f maps
// onto dst.
//
// For [ImageStretch] that is the whole texture. For [ImageCover] it is the
// largest centred rectangle with the aspect ratio of dst that fits inside the
// texture: the part of the picture that survives the crop. A degenerate dst or
// texture returns the whole texture,
// because there is no aspect ratio to preserve and the operation is skipped
// for its empty bounds anyway.
//
// It is exported so that a backend and a test compute the crop with one piece
// of arithmetic; see [Shadow.Extent] for the same rule applied to shadows.
func (f ImageFit) Source(dst geom.Rect, w, h int) geom.Rect {
	full := geom.Rc(0, 0, float32(w), float32(h))
	if f != ImageCover || w <= 0 || h <= 0 {
		return full
	}
	dw, dh := dst.Width(), dst.Height()
	if !(dw > 0) || !(dh > 0) {
		return full
	}
	tw, th := float32(w), float32(h)
	// The scale that makes the texture cover dst is the larger of the two
	// axis ratios; the visible part of the texture is dst divided by it.
	s := max(dw/tw, dh/th)
	sw, sh := min(dw/s, tw), min(dh/s, th)
	x0, y0 := (tw-sw)*0.5, (th-sh)*0.5
	return geom.Rc(x0, y0, x0+sw, y0+sh)
}
