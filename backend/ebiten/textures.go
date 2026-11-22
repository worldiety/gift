package ebiten

import (
	"image"
	"math"
	"sync/atomic"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift/cms"
	"github.com/worldiety/gift/render"
)

// TextureConfig configures a [TextureCache]. The zero value selects the
// defaults below and is what [NewRenderer] uses.
type TextureConfig struct {
	// UploadBytesPerFrame is the byte budget for uploads in one *drawn*
	// frame. Zero selects [DefaultUploadBytesPerFrame]; a negative value
	// removes the budget, which is what a test that wants every upload to
	// land immediately sets.
	UploadBytesPerFrame int64

	// UploadsPerFrame caps the number of uploads in one drawn frame, whatever
	// their size. Zero selects [DefaultUploadsPerFrame]; a negative value
	// removes the cap.
	//
	// It exists next to the byte budget because the two bound different
	// things. The bytes bound the transfer, and the count bounds the number
	// of glTexSubImage2D calls — each of which can cost a driver side
	// synchronisation, whatever its size. See [TextureCache].
	UploadsPerFrame int

	// MaxBytes is the residency budget across all textures, in logical pixel
	// bytes. Zero selects [DefaultTextureBytes].
	//
	// Logical, and the project plan, section 11, insists on the word: "Logische
	// Pixelbytes sind kein exaktes GPU-Budget: Padding, Fragmentierung,
	// Atlaswachstum, Zwischenziele und Staging brauchen zusaetzlichen
	// Speicher." A 256 by 256 thumbnail is accounted at 256 KiB here and
	// occupies more than that on the device.
	MaxBytes int64

	// MaxAge is the number of drawn frames a texture may go undrawn before
	// [TextureCache.Tick] deallocates it. Zero selects
	// [DefaultTextureMaxAge]; a negative value disables age eviction.
	MaxAge int
}

// Defaults for [TextureConfig].
const (
	// DefaultUploadBytesPerFrame is 2 MiB, that is eight thumbnails at 256
	// pixels or two at 512.
	//
	// It is a starting point and is documented as one. The project plan,
	// section 11, refuses to guess at GPU costs — "CPU-Zeit am Upload-Aufruf
	// ist wegen interner Queues kein GPU-Zeitmass" — so the number that
	// matters is the one measured on a Pi 4 with a cold cache, and the
	// scenario for it is in section 13. What the budget is *for* is not in
	// doubt: the OpenGL upload path calls glFinish before writing pixels when
	// a draw has been issued, so an unbudgeted frame that uploads a whole
	// screenful of newly decoded thumbnails stalls on the GPU no matter how
	// much decoding happened in the background.
	DefaultUploadBytesPerFrame int64 = 2 << 20

	// DefaultUploadsPerFrame is eight.
	DefaultUploadsPerFrame = 8

	// DefaultTextureBytes is 128 MiB of logical pixel bytes, half of the
	// 256 MiB the project plan, section 13, budgets for resident image
	// memory at a hundred thousand entries. The other half is the headroom
	// the word "logical" buys.
	DefaultTextureBytes int64 = 128 << 20

	// DefaultTextureMaxAge is 600 drawn frames, ten seconds at sixty hertz,
	// the same number the glyph atlas and the shaping cache use.
	DefaultTextureMaxAge = 600
)

// texRecord is one slot of the texture pool.
//
// It is plain data in a flat slice addressed by index, like [glyphEntry] and
// like internal/scene's nodes, so that resolving a handle in the frame path is
// a bounds check and two integer comparisons with nothing for the collector to
// scan.
type texRecord struct {
	img *eb.Image
	// gen is the generation of the current occupant. It is bumped on every
	// eviction, which is what makes a handle to an evicted texture detectably
	// stale instead of silently pointing at the next picture that lands in
	// the slot.
	gen uint32
	// w and h are the pixel dimensions and bytes the logical accounting.
	w, h  int
	bytes int64
	// used is the drawn frame number this texture was last resolved or
	// uploaded in. A texture with used == frame is referenced by the frame in
	// progress and must not be evicted.
	used uint64
	// live says whether the slot is occupied.
	live bool
	// pendingFree marks a slot whose owner asked for release while the frame
	// in progress was already drawing it; see [TextureCache.Deallocate].
	pendingFree bool
	// opaque says that every pixel has full alpha, which is what lets a
	// picture serve as the static backdrop of a glass pane; see backdrop.go.
	opaque bool
}

// TextureCache is the image resource service of this backend: residency,
// admission and eviction of decoded pictures on the GPU.
//
// It implements [render.Images].
//
// # One texture per picture, and no atlas of our own
//
// A thumbnail gets its own [eb.Image]. There is deliberately no bespoke page
// packer here, unlike the glyph atlas next door, and the project plan,
// section 11, is the reason twice over.
//
// First, because Ebitengine already has one. An ordinary eb.Image is put on
// Ebitengine's automatic atlas whenever it fits: internal/atlas's
// canBePutOnAtlas returns true for any regular image no larger than maxSize,
// which is floorPowerOf2 of the driver's maximum texture edge and is therefore
// at least 4096 on anything that can run GL ES 3.0. A 512 pixel thumbnail —
// the largest rung of asset.DefaultSizes — is comfortably inside that, so the
// thumbnails of a gallery share backend textures without this package packing
// anything. "Zunaechst automatischer Ebitengine-Atlas und explizites
// Deallocate bei Eviction" is exactly what this is.
//
// Second, because the draw call argument for a bespoke atlas does not survive
// contact with what Ebitengine does with the commands. A batch here ends
// wherever the source image changes, so sixty visible tiles are sixty
// DrawTriangles calls; but graphicscommand's CanMergeWithDrawTrianglesCommand
// merges two consecutive draw commands whose *backend* source images are
// identical, and two thumbnails on one automatic atlas page are exactly that.
// The GPU therefore sees far fewer draws than this package issues, and a
// second atlas on top of the first would be paying for a measurement nobody
// has taken. The plan's sentence for that case is "Falls noetig, wird ein
// begrenzter Pool eigener Atlas-Seiten verglichen. Keine vorsorgliche eigene
// Atlas-Engine ohne Messung", and [RendererStats.ImageBatches] is the number
// that would justify taking it.
//
// The honest cost of one texture per picture is stated rather than hidden:
// every thumbnail pays Ebitengine's atlas padding, a 240 by 180 thumbnail
// wastes nothing but a 513 pixel one would get a texture of its own, and this
// package cannot see which of the two happened. [TextureStats.Bytes] is
// logical pixel bytes and not device memory; see [TextureConfig.MaxBytes].
//
// # Admission
//
// [TextureCache.Acquire] uploads at most [TextureConfig.UploadsPerFrame]
// textures and [TextureConfig.UploadBytesPerFrame] bytes per *drawn* frame.
// The budget is reset in [TextureCache.BeginFrame], which the renderer calls
// from [Renderer.BeginFrame], and painting is the only thing that happens once
// per drawn frame — so several Ebitengine updates before one draw cannot spend
// the budget several times. That is the distinction the project plan,
// section 11, draws and the one this whole type is arranged around.
//
// # Eviction
//
// Least recently drawn first, on two triggers, exactly like [GlyphAtlas]:
// pressure against [TextureConfig.MaxBytes] when a new texture needs room, and
// age in [TextureCache.Tick]. Evicting calls [eb.Image.Deallocate], which is
// the explicit release the project plan, section 11, asks for instead of
// waiting for a cleanup function.
//
// A texture the frame in progress has already resolved or uploaded is never
// evicted. Its vertices may already be in the renderer's buffers, and
// deallocating underneath them is the one failure mode that produces a blank
// or a wrong picture with no diagnostic at all. This is the same rule the
// glyph atlas applies to a page, implemented the same way: a use stamp equal
// to the current frame number is a veto.
type TextureCache struct {
	cfg TextureConfig

	// recs is the slot storage. Index 0 is never used, so that the zero
	// [render.ImageID] can mean "nothing" without a special case in the
	// renderer.
	recs []texRecord
	free []uint32

	// frame is the drawn frame counter and the age stamp.
	frame uint64
	bytes int64

	// frameUploads and frameBytes are the budget spent in the frame in
	// progress.
	frameUploads int
	frameBytes   int64

	// frameDeferred counts the uploads the budget turned away in the frame
	// in progress, or, between two frames, in the frame last drawn. A
	// deferred picture is drawn as its placeholder and asks again when it is
	// next painted, so a backend that skips unchanged frames must draw
	// another one while this is not zero; see [TextureCache.Deferred].
	frameDeferred int

	stats TextureStats

	// partial are the pictures larger than the per frame budget that are
	// being uploaded a band of rows per frame; see [TextureCache.Acquire].
	partial []partialUpload

	// epoch makes the generations of this cache disjoint from those of every
	// other cache in the process; see [textureEpoch]. Every slot this cache
	// ever hands out starts its generation here.
	epoch uint32

	// onDeallocate, if non nil, is called with the image immediately before
	// it is deallocated. It exists so that a test can assert the explicit
	// release actually happens rather than merely being claimed in a comment,
	// which is what the glyph atlas does with the same hook.
	onDeallocate func(*eb.Image)
	// newImage overrides texture creation, so that the admission, eviction
	// and accounting logic is testable without a graphics context. A nil
	// value means [eb.NewImage].
	newImage func(w, h int) *eb.Image

	// conv converts pictures that are not in the working space as they are
	// uploaded, and scratch holds the converted rows; see
	// [TextureCache.SetSpace].
	conv    pixelConversions
	scratch []byte
}

// SetSpace sets the colour space the textures are drawn in, nil for sRGB. A
// picture in another space, [render.Pixels.Space], is converted as it is
// uploaded. The renderer calls it from [Renderer.SetWorkingSpace].
func (t *TextureCache) SetSpace(p *cms.Profile) {
	if p == cms.SRGB() {
		p = nil
	}
	if p != t.conv.dst {
		t.conv = pixelConversions{dst: p}
	}
}

// NewTextureCache returns an empty cache. It allocates no GPU memory until the
// first [TextureCache.Acquire].
func NewTextureCache(cfg TextureConfig) *TextureCache {
	if cfg.UploadBytesPerFrame == 0 {
		cfg.UploadBytesPerFrame = DefaultUploadBytesPerFrame
	}
	if cfg.UploadsPerFrame == 0 {
		cfg.UploadsPerFrame = DefaultUploadsPerFrame
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = DefaultTextureBytes
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = DefaultTextureMaxAge
	}
	// One dead slot so that id 0 is invalid.
	return &TextureCache{cfg: cfg, epoch: textureEpoch(), recs: make([]texRecord, 1, 64)}
}

// textureEpochCounter numbers the texture caches created in this process.
var textureEpochCounter atomic.Uint32

// textureEpoch returns the generation every slot of a freshly created cache
// starts at.
//
// # The defect this exists for
//
// A [render.ImageHandle] carries a slot index and a generation, and
// [TextureCache.Resolve] compares the generation so that a handle to an evicted
// texture reports stale rather than pointing at whatever landed in the slot
// afterwards. That is exactly right *within* one cache and says nothing at all
// between two of them: both used to start at slot 1, generation 0, so a handle
// minted by one cache resolved in another — to a completely unrelated picture,
// with no counter moving and no diagnosis anywhere.
//
// That is not a hypothetical. ui's icon mask cache is process wide and keyed
// on the symbol and the size, while a [Renderer] and its texture cache are
// created per window — and per [gifttest.Harness]. Two golden tests in one
// package therefore shared the mask cache and not the textures, and the second
// one drew a bell where a chevron belonged: observed, in
// TestEveryComponentLooksTheWayItLooks, before this function existed.
//
// The epoch is the cache's ordinal times a large odd constant, so the
// generations of two caches are far apart rather than merely different, and a
// cache would have to evict one slot about four billion times to walk into the
// range of another. The alternative — a cache identity field in
// [render.ImageHandle] — is the more complete answer and costs four bytes in a
// struct that is copied per drawn picture; this costs nothing per frame and
// closes the same hole.
func textureEpoch() uint32 {
	const stride = 0x9E3779B9 // the golden ratio in 32 bits, odd, well spread
	return textureEpochCounter.Add(1) * stride
}

// BeginFrame resets the per drawn frame upload budget. The renderer calls it
// from [Renderer.BeginFrame] and from nowhere else.
func (t *TextureCache) BeginFrame() {
	t.frameUploads = 0
	t.frameBytes = 0
	t.frameDeferred = 0
}

// Deferred reports how many uploads the budget turned away in the frame last
// drawn. Those pictures are placeholders on the screen right now and become
// pictures only in a frame that paints them again.
func (t *TextureCache) Deferred() int { return t.frameDeferred }

// Tick advances the frame clock and evicts textures that have gone undrawn for
// longer than [TextureConfig.MaxAge]. The renderer calls it once per drawn
// frame, from [Renderer.EndFrame], after the last draw call has been issued —
// which is the point at which "used by the frame in progress" stops being
// true of anything.
func (t *TextureCache) Tick() {
	// Deferred releases first: a slot whose owner asked for it while the
	// frame was drawing it is now free to go.
	for i := range t.recs {
		if t.recs[i].live && t.recs[i].pendingFree {
			t.evict(uint32(i))
		}
	}
	t.frame++
	// A partial upload nobody asked for in two seconds of frames is a page
	// the user went away from; its texture and its bytes go back.
	for i := 0; i < len(t.partial); {
		if p := &t.partial[i]; t.frame-p.asked > partialMaxAge {
			t.dropPartial(p, true)
			continue
		}
		i++
	}
	if t.cfg.MaxAge < 0 {
		return
	}
	age := uint64(t.cfg.MaxAge)
	if t.frame <= age {
		return
	}
	deadline := t.frame - age
	for i := range t.recs {
		r := &t.recs[i]
		if r.live && r.used < deadline {
			t.evict(uint32(i))
			t.stats.AgeEvictions++
		}
	}
}

// Resolve implements [render.Images].
//
// The hit path is a bounds check, two integer comparisons and one integer
// write. It allocates nothing, which is what keeps a frame full of resident
// pictures inside the 0 B/op contract of the project plan, section 11.
func (t *TextureCache) Resolve(h render.ImageHandle) (render.ImageID, bool) {
	i := uint32(h.ID)
	if i == 0 || int(i) >= len(t.recs) {
		t.stats.Stale++
		return 0, false
	}
	r := &t.recs[i]
	if !r.live || r.gen != h.Gen {
		t.stats.Stale++
		return 0, false
	}
	r.used = t.frame
	t.stats.Resolves++
	return h.ID, true
}

// Acquire implements [render.Images].
func (t *TextureCache) Acquire(px render.Pixels) (render.ImageHandle, bool) {
	if px.IsEmpty() {
		t.stats.Malformed++
		return render.ImageHandle{}, false
	}
	// Ebitengine panics when an image larger than the device allows is
	// used, and the answer is zero outside a running game, where nothing is
	// drawn anyway. A picture that large is a request the caller should not
	// have made — a viewer caps its detail; see ui/imageviewer.MaxDetail —
	// and is refused like any other picture that does not fit.
	if m := eb.MaxImageSize(); m > 0 && (px.W > m || px.H > m) {
		t.stats.Rejected++
		return render.ImageHandle{}, false
	}
	need := px.Bytes()
	if px.Key != 0 && t.cfg.UploadBytesPerFrame > 0 && need > t.cfg.UploadBytesPerFrame {
		return t.acquirePartial(px, need)
	}
	if t.cfg.UploadsPerFrame >= 0 && t.frameUploads >= t.cfg.UploadsPerFrame {
		t.stats.Deferred++
		t.frameDeferred++
		return render.ImageHandle{}, false
	}
	if t.cfg.UploadBytesPerFrame >= 0 && t.frameBytes+need > t.cfg.UploadBytesPerFrame &&
		need <= t.cfg.UploadBytesPerFrame {
		// The second clause is the escape hatch for a picture that is larger
		// than the whole per frame budget: it can never fit and deferring it
		// would leave the tile a placeholder for ever, so it is admitted and
		// overshoots the budget once.
		//
		// WU-R corrected the condition. It used to read "and something was
		// already uploaded this frame", which grants the large picture a
		// frame to itself — but only if it is ever asked for in a frame
		// where nothing else came first, and paint order is tree order. A
		// smaller picture consistently ahead of it starves it for ever.
		// Whether the overshoot is affordable is a property of the picture
		// and not of the order its neighbours happen to be painted in.
		t.stats.Deferred++
		t.frameDeferred++
		return render.ImageHandle{}, false
	}
	if !t.makeRoom(need) {
		t.stats.Rejected++
		return render.ImageHandle{}, false
	}

	t.bytes += need
	img := t.newTexture(px.W, px.H)
	if img != nil {
		t.writeRows(img, px, 0, px.H)
	}
	t.frameUploads++
	t.frameBytes += need
	t.stats.UploadedBytes += uint64(need)
	return t.install(img, px.W, px.H, need, opaquePixels(px))
}

// install makes a texture whose pixels are all uploaded and whose bytes are
// already counted resident, and returns its handle.
func (t *TextureCache) install(img *eb.Image, w, h int, need int64, opaque bool) (render.ImageHandle, bool) {
	i := t.alloc()
	r := &t.recs[i]
	r.img = img
	r.w, r.h = w, h
	r.bytes = need
	r.used = t.frame
	r.live = true
	r.pendingFree = false
	r.opaque = opaque
	t.stats.Uploads++
	if n := len(t.recs) - len(t.free) - 1; n > t.stats.PeakTextures {
		t.stats.PeakTextures = n
	}
	if t.bytes > t.stats.PeakBytes {
		t.stats.PeakBytes = t.bytes
	}
	return render.ImageHandle{ID: render.ImageID(i), Gen: r.gen}, true
}

// partialUpload is a picture larger than the per frame budget on its way to
// the GPU: the texture, which rows of it are there, and when it was last
// asked for.
type partialUpload struct {
	key    uint64
	w, h   int
	img    *eb.Image
	need   int64
	row    int
	opaque bool
	asked  uint64
}

// partialMaxAge is how many drawn frames a partial upload waits to be asked
// for again before it is given up.
const partialMaxAge = 120

// maxPartial bounds the pictures that are uploaded in bands at once. A viewer
// prepares the page on either side of the one on screen; a fourth is room.
const maxPartial = 4

// acquirePartial uploads a picture larger than the per frame byte budget in
// bands of rows, as much of the budget as the frame has left each time it is
// asked for, and hands out the texture when the last band is there.
//
// # Why
//
// The escape hatch below lets such a picture overshoot the budget once, in
// one frame, which is the right answer for a thumbnail that is a little too
// large and the wrong one for a picture the size of a screen: measured on a
// Raspberry Pi 5, a full screen picture of 11 MB took a frame of its own and
// a zoomed one of 45 MB a draw of 41 ms, a hitch of 90 ms in the middle of a
// swipe. In bands of the budget the same picture arrives over a few frames
// in which nothing stalls, and a viewer that prepares the next page while
// the current one is looked at has it resident before the swipe begins.
//
// The picture is recognised from frame to frame by [render.Pixels.Key]. Its
// bytes count against the residency budget from the first band, and a
// picture that is not asked for again within [TextureConfig.MaxAge] frames
// is given up; see [TextureCache.Tick].
func (t *TextureCache) acquirePartial(px render.Pixels, need int64) (render.ImageHandle, bool) {
	var p *partialUpload
	for i := range t.partial {
		if q := &t.partial[i]; q.key == px.Key && q.w == px.W && q.h == px.H {
			p = q
			break
		}
	}
	if p == nil {
		if len(t.partial) >= maxPartial {
			// Not a refusal but a wait: a band of the pictures already on
			// their way is uploaded each frame, and this one starts when
			// one of them is done. It counts as deferred, so that a backend
			// that draws only on demand draws the frames in which that
			// happens. Counted as rejected it asked for none, and a map
			// whose screen held more than maxPartial tiles larger than the
			// budget stopped with the rest of them missing.
			t.stats.Deferred++
			t.frameDeferred++
			return render.ImageHandle{}, false
		}
		if !t.makeRoom(need) {
			t.stats.Rejected++
			return render.ImageHandle{}, false
		}
		t.bytes += need
		t.partial = append(t.partial, partialUpload{
			key: px.Key, w: px.W, h: px.H, need: need, opaque: true,
			img: t.newTexture(px.W, px.H),
		})
		p = &t.partial[len(t.partial)-1]
	}
	p.asked = t.frame
	rowBytes := int64(px.W * 4)
	left := t.cfg.UploadBytesPerFrame - t.frameBytes
	rows := int(min(left/rowBytes, int64(px.H-p.row)))
	if rows > 0 {
		if p.img != nil {
			t.writeRows(p.img, px, p.row, p.row+rows)
		}
		if p.opaque {
			p.opaque = opaquePixels(render.Pixels{Pix: px.Pix[p.row*px.Stride:], W: px.W, H: rows, Stride: px.Stride})
		}
		p.row += rows
		t.frameBytes += int64(rows) * rowBytes
		t.stats.UploadedBytes += uint64(int64(rows) * rowBytes)
	}
	if p.row < px.H {
		t.stats.Deferred++
		t.frameDeferred++
		return render.ImageHandle{}, false
	}
	t.frameUploads++
	img, opaque := p.img, p.opaque
	t.dropPartial(p, false)
	return t.install(img, px.W, px.H, need, opaque)
}

// dropPartial removes a partial upload; release gives its texture and its
// bytes back, which a finished one, now installed, keeps.
func (t *TextureCache) dropPartial(p *partialUpload, release bool) {
	for i := range t.partial {
		if &t.partial[i] != p {
			continue
		}
		if release {
			if p.img != nil {
				if t.onDeallocate != nil {
					t.onDeallocate(p.img)
				}
				p.img.Deallocate()
			}
			t.bytes -= p.need
		}
		t.partial = append(t.partial[:i], t.partial[i+1:]...)
		return
	}
}

// Deallocate implements [render.Images].
//
// A texture the frame in progress has already drawn is *not* released here.
// Its vertices are in the renderer's buffers and the draw call has not been
// issued yet, so deallocating now would draw from a dead texture. The slot is
// marked instead and released by the next [TextureCache.Tick], which runs
// after the last draw call of the frame.
func (t *TextureCache) Deallocate(h render.ImageHandle) {
	i := uint32(h.ID)
	if i == 0 || int(i) >= len(t.recs) {
		return
	}
	r := &t.recs[i]
	if !r.live || r.gen != h.Gen {
		return
	}
	if r.used == t.frame {
		r.pendingFree = true
		return
	}
	t.evict(i)
	t.stats.ExplicitReleases++
}

// makeRoom evicts least recently drawn textures until need bytes fit, and
// reports whether they do.
func (t *TextureCache) makeRoom(need int64) bool {
	if need > t.cfg.MaxBytes {
		// A single picture larger than the whole residency budget. Refusing
		// it is the only bounded answer; growing the budget for one picture
		// would make it depend on the largest thumbnail the application ever
		// asked for.
		return false
	}
	for t.bytes+need > t.cfg.MaxBytes {
		if !t.evictLRU() {
			return false
		}
	}
	return true
}

// evictLRU releases the least recently drawn texture that the frame in
// progress does not reference, and reports whether it found one.
func (t *TextureCache) evictLRU() bool {
	best, bestUsed := -1, uint64(math.MaxUint64)
	for i := range t.recs {
		r := &t.recs[i]
		if !r.live || r.used == t.frame {
			continue
		}
		if r.used < bestUsed {
			best, bestUsed = i, r.used
		}
	}
	if best < 0 {
		return false
	}
	t.evict(uint32(best))
	t.stats.Evictions++
	return true
}

// evict releases one texture and invalidates every handle to it.
func (t *TextureCache) evict(i uint32) {
	r := &t.recs[i]
	if !r.live {
		return
	}
	if r.img != nil {
		if t.onDeallocate != nil {
			t.onDeallocate(r.img)
		}
		// Deallocate and not "keep the image and forget its contents": the
		// second never gives memory back, which is the whole point of the
		// explicit release the project plan, section 11, asks for.
		r.img.Deallocate()
	}
	t.bytes -= r.bytes
	*r = texRecord{gen: r.gen + 1}
	t.free = append(t.free, i)
	t.stats.Deallocations++
}

// alloc returns a free slot index, recycling one if there is any.
func (t *TextureCache) alloc() uint32 {
	if n := len(t.free); n > 0 {
		i := t.free[n-1]
		t.free = t.free[:n-1]
		return i
	}
	t.recs = append(t.recs, texRecord{gen: t.epoch})
	return uint32(len(t.recs) - 1)
}

func (t *TextureCache) newTexture(w, h int) *eb.Image {
	if t.newImage != nil {
		return t.newImage(w, h)
	}
	return eb.NewImage(w, h)
}

// writeRows uploads the rows y0 to y1 of the pixel data, converted into the
// working space if they are in another one.
//
// A conversion goes through scratch in bands of up to 4 MiB, each one
// WritePixels. A picture that cannot be converted is uploaded as it is: the
// only such space is one that is not RGB, which no producer in gift hands
// over.
func (t *TextureCache) writeRows(img *eb.Image, px render.Pixels, y0, y1 int) {
	tr, err := t.conv.get(px.Space)
	if tr == nil || err != nil {
		writeRows(img, px, y0, y1)
		return
	}
	n := px.W * 4
	band := max(1, (4<<20)/n)
	for y := y0; y < y1; y += band {
		rows := min(band, y1-y)
		if cap(t.scratch) < rows*n {
			t.scratch = make([]byte, rows*n)
		}
		buf := t.scratch[:rows*n]
		if convertPixels(tr, buf, px.Pix[y*px.Stride:], px.W, rows, px.Stride) != nil {
			writeRows(img, px, y, y1)
			return
		}
		img.SubImage(image.Rect(0, y, px.W, y+rows)).(*eb.Image).WritePixels(buf)
	}
	t.stats.Converted++
}

// writeRows uploads the rows y0 to y1 of the pixel data as they are.
//
// The rows are written one call at a time only when the stride is padded,
// which asset.Thumbnail's never is; the common case is a single WritePixels of
// a contiguous slice and therefore a single glTexSubImage2D.
func writeRows(img *eb.Image, px render.Pixels, y0, y1 int) {
	n := px.W * 4
	if px.Stride == n {
		dst := img
		if y0 != 0 || y1 != px.H {
			dst = img.SubImage(image.Rect(0, y0, px.W, y1)).(*eb.Image)
		}
		dst.WritePixels(px.Pix[y0*px.Stride : y1*px.Stride])
		return
	}
	for y := y0; y < y1; y++ {
		row := px.Pix[y*px.Stride : y*px.Stride+n]
		sub := img.SubImage(image.Rect(0, y, px.W, y+1)).(*eb.Image)
		sub.WritePixels(row)
	}
}

// image returns the texture of an id, or nil.
func (t *TextureCache) image(id render.ImageID) *eb.Image {
	i := uint32(id)
	if i == 0 || int(i) >= len(t.recs) || !t.recs[i].live {
		return nil
	}
	return t.recs[i].img
}

// opaque reports whether every pixel of an id has full alpha.
func (t *TextureCache) opaque(id render.ImageID) bool {
	i := uint32(id)
	return i != 0 && int(i) < len(t.recs) && t.recs[i].live && t.recs[i].opaque
}

// opaquePixels reports whether every alpha byte of px is 255. It reads one
// byte in four and stops at the first translucent pixel, so a photograph
// costs one pass over its alpha channel at upload and a picture with
// transparency costs almost nothing.
func opaquePixels(px render.Pixels) bool {
	n := px.W * 4
	for y := range px.H {
		row := px.Pix[y*px.Stride : y*px.Stride+n]
		for x := 3; x < n; x += 4 {
			if row[x] != 255 {
				return false
			}
		}
	}
	return true
}

// size returns the pixel dimensions of an id.
func (t *TextureCache) size(id render.ImageID) (w, h int, ok bool) {
	i := uint32(id)
	if i == 0 || int(i) >= len(t.recs) || !t.recs[i].live {
		return 0, 0, false
	}
	return t.recs[i].w, t.recs[i].h, true
}

// TextureStats are the counters of the texture cache. Like every other counter
// in gift they are plain numbers written in the frame path and read out of
// band; see the project plan, section 15.
type TextureStats struct {
	// Uploads and UploadedBytes are what actually reached the GPU.
	Uploads       uint64
	UploadedBytes uint64
	// Converted is the number of uploads, or bands of a picture uploaded in
	// bands, whose pixels were converted into the working space on the way;
	// see [TextureCache.SetSpace].
	Converted uint64
	// Deferred is the number of [TextureCache.Acquire] calls refused by the
	// per drawn frame upload budget. It is not an error: the caller drew a
	// placeholder and the picture arrived a frame or two later. A Deferred
	// that keeps climbing in a steady scene means the budget is smaller than
	// the rate at which pictures become ready.
	Deferred uint64
	// Rejected is the number refused because no room could be made: every
	// resident texture was already drawn by the frame in progress, or the
	// picture was larger than the whole residency budget. Unlike Deferred
	// this one says the budget is too small for the scene.
	Rejected uint64
	// Malformed is the number refused for bad pixel data.
	Malformed uint64
	// Resolves is the number of successful handle resolutions and Stale the
	// number of failed ones. A Stale in a steady scene is a consumer holding
	// handles across an eviction, which is legal and is exactly what the
	// generation in [render.ImageHandle] is for.
	Resolves, Stale uint64
	// Evictions is the number of textures released by pressure,
	// AgeEvictions the subset released by [TextureCache.Tick] for going
	// undrawn, and ExplicitReleases the ones an owner asked to release.
	// Deallocations is the total number of [eb.Image.Deallocate] calls and
	// must equal the sum of the reasons.
	Evictions, AgeEvictions, ExplicitReleases, Deallocations uint64
	// Textures and Bytes are the current residency, PeakTextures and
	// PeakBytes their high water marks. Bytes is logical pixel bytes; see
	// [TextureConfig.MaxBytes].
	Textures     int
	Bytes        int64
	PeakTextures int
	PeakBytes    int64
}

// Stats returns a snapshot of the counters.
func (t *TextureCache) Stats() TextureStats {
	s := t.stats
	s.Textures = len(t.recs) - len(t.free) - 1
	s.Bytes = t.bytes
	return s
}
