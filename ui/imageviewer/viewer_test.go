package imageviewer_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/imageviewer"
)

func TestMain(m *testing.M) { gifttest.Main(m) }

// --- fixtures ----------------------------------------------------------------

// images is a texture table that accepts everything, so a picture that has
// arrived is drawn.
type images struct{ n int }

func (f *images) Resolve(h render.ImageHandle) (render.ImageID, bool) { return h.ID, h.ID > 0 }
func (f *images) Acquire(px render.Pixels) (render.ImageHandle, bool) {
	if px.IsEmpty() {
		return render.ImageHandle{}, false
	}
	f.n++
	return render.ImageHandle{ID: render.ImageID(f.n), Gen: 1}, true
}
func (f *images) Deallocate(render.ImageHandle) {}

// origin is a gallery of one row: thumbnail i at (10 + 60 i, 10), 50 by 50.
type origin struct {
	ids       []asset.ID
	concealed asset.ID
	revealed  []asset.ID
}

func (o *origin) ThumbnailFrame(id asset.ID) (geom.Rect, float32, bool) {
	for i, v := range o.ids {
		if v == id {
			x := float32(10 + 60*i)
			return geom.Rc(x, 10, x+50, 60), 6, true
		}
	}
	return geom.Rect{}, 0, false
}
func (o *origin) Conceal(id asset.ID)     { o.concealed = id }
func (o *origin) Reveal(id asset.ID) bool { o.revealed = append(o.revealed, id); return true }

type env struct {
	t      *testing.T
	h      *gifttest.Harness
	v      *imageviewer.Viewer
	pipe   *asset.Pipeline
	ch     chan func()
	ids    []asset.ID
	origin *origin
	shown  []asset.ID
}

// photos writes n landscape JPEGs of 900 by 600 and returns their catalogue.
func photos(t *testing.T, n int) ([]asset.Metadata, map[asset.ID]asset.Source) {
	t.Helper()
	dir := t.TempDir()
	items := make([]asset.Metadata, n)
	srcs := make(map[asset.ID]asset.Source, n)
	for i := range n {
		img := image.NewRGBA(image.Rect(0, 0, 900, 600))
		c := color.RGBA{R: uint8(40 + 50*i), G: 120, B: uint8(200 - 30*i), A: 255}
		for p := 0; p < len(img.Pix); p += 4 {
			img.Pix[p], img.Pix[p+1], img.Pix[p+2], img.Pix[p+3] = c.R, c.G, c.B, 255
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, nil); err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(dir, "p"+strconv.Itoa(i)+".jpg")
		if err := os.WriteFile(name, buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		src := asset.File(name)
		m := src.Metadata()
		m.Width, m.Height = 900, 600
		items[i], srcs[m.ID] = m, src
	}
	return items, srcs
}

// setup mounts a viewer over n photos in an 800 by 480 window, the size of a
// seven inch panel.
func setup(t *testing.T, n int, configure func(imageviewer.FullscreenView) imageviewer.FullscreenView) *env {
	t.Helper()
	return setupIn(t, n, configure, nil)
}

// setupIn is [setup] with the window's view passed through wrap, when it is
// not nil.
func setupIn(t *testing.T, n int, configure func(imageviewer.FullscreenView) imageviewer.FullscreenView, wrap func(gift.View) gift.View) *env {
	t.Helper()
	items, srcs := photos(t, n)
	e := &env{t: t, ch: make(chan func(), 1024)}
	e.pipe = asset.NewPipeline(asset.Config{
		Deliver: func(fn func()) { e.ch <- fn },
		Sizes:   []int{128, 256, 512, 1024},
		Workers: 2,
	})
	t.Cleanup(func() { e.pipe.Close(); e.drain() })
	ui.SetImagePipeline(e.pipe)
	t.Cleanup(func() { ui.SetImagePipeline(nil) })
	coll := asset.NewCollection(items)
	for i := range n {
		e.ids = append(e.ids, coll.ID(i))
	}
	e.origin = &origin{ids: e.ids}
	e.v = imageviewer.New(coll, func(id asset.ID) asset.Source { return srcs[id] })
	e.v.SetOrigin(e.origin)
	e.v.OnChange(func(id asset.ID) { e.shown = append(e.shown, id) })
	fs := imageviewer.Fullscreen(e.v)
	if configure != nil {
		fs = configure(fs)
	}
	var view gift.View = ui.ZStack(ui.Box().Background(ui.RGB(240, 240, 240)), fs)
	if wrap != nil {
		view = wrap(view)
	}
	e.h = gifttest.New(t, gifttest.Options{
		View: view,
		Size: geom.Sz(800, 480),
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}),
	})
	e.h.App().SetImages(&images{})
	e.h.Settle()
	return e
}

func (e *env) drain() int {
	n := 0
	for {
		select {
		case fn := <-e.ch:
			fn()
			n++
		default:
			return n
		}
	}
}

// load waits for the pipeline to finish what it was asked and hands the
// results to the user interface.
func (e *env) load() {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s := e.pipe.Stats()
		if e.drain() > 0 {
			e.h.Settle()
			continue
		}
		if s.QueuedVisible+s.QueuedPrefetch+s.InFlight == 0 {
			e.h.Settle()
			if e.drain() == 0 {
				return
			}
			continue
		}
		time.Sleep(time.Millisecond)
	}
	e.t.Fatal("the pipeline did not finish within ten seconds")
}

// wait lets d pass in frames.
func (e *env) wait(d time.Duration) {
	e.h.AdvanceTicks(int(d/gifttest.FrameInterval), gifttest.FrameInterval)
}

// open opens entry i and lets the transition finish.
func (e *env) open(i int) {
	e.t.Helper()
	e.v.Open(e.ids[i])
	e.h.Settle()
	e.load()
	e.wait(time.Second)
}

// pictures are the image operations of the last frame.
func (e *env) pictures() []render.Op {
	var out []render.Op
	for _, op := range e.h.Ops() {
		if op.Kind == render.OpImage && op.Bounds.Width() > 40 {
			out = append(out, op)
		}
	}
	return out
}

func near(a, b, eps float32) bool { return a-b <= eps && b-a <= eps }

// --- tests -------------------------------------------------------------------

func TestOpeningGrowsThePictureOutOfItsThumbnail(t *testing.T) {
	e := setup(t, 4, nil)
	e.v.Open(e.ids[2])
	e.h.Settle()
	e.load()
	// The first frames: the picture sits where its thumbnail was.
	e.h.Frame()
	ps := e.pictures()
	if len(ps) == 0 {
		t.Fatal("nothing is drawn in the first frame of the transition")
	}
	if b := ps[0].Bounds; !near(b.Min.X, 130, 20) || !near(b.Max.Y, 60, 30) {
		t.Errorf("the first frame draws at %v, want near the thumbnail at (130, 10)-(180, 60)", b)
	}
	if ps[0].Fit != render.ImageCover || ps[0].CornerRadius <= 0 {
		t.Errorf("the picture leaves its tile uncropped (%v) or square (%v)", ps[0].Fit, ps[0].CornerRadius)
	}
	if e.origin.concealed != e.ids[2] {
		t.Errorf("the origin conceals %q, want the opened entry", e.origin.concealed)
	}
	e.wait(time.Second)
	// At rest: 3:2 in 800 by 480 is 720 by 480, centred.
	ps = e.pictures()
	if len(ps) != 1 {
		t.Fatalf("%d pictures at rest, want the current one only", len(ps))
	}
	if b := ps[0].Bounds; !near(b.Min.X, 40, 1) || !near(b.Width(), 720, 1) || !near(b.Height(), 480, 1) {
		t.Errorf("at rest the picture is at %v, want (40, 0)-(760, 480)", b)
	}
}

func TestASwipeTurnsThePageAndTheOriginFollows(t *testing.T) {
	e := setup(t, 4, nil)
	e.open(1)
	e.h.Fling(geom.Pt(600, 240), geom.Pt(450, 240), 120*time.Millisecond)
	e.load()
	e.wait(time.Second)
	if id, _ := e.v.Current(); id != e.ids[2] {
		t.Fatalf("after a flick to the left the viewer shows %q, want %q", id, e.ids[2])
	}
	if last := e.origin.revealed[len(e.origin.revealed)-1]; last != e.ids[2] || e.origin.concealed != e.ids[2] {
		t.Errorf("the origin revealed %q and conceals %q, want the new entry", last, e.origin.concealed)
	}
	// A slow drag back of less than half a page stays.
	e.h.Fling(geom.Pt(300, 240), geom.Pt(500, 240), time.Second)
	e.wait(time.Second)
	if id, _ := e.v.Current(); id != e.ids[2] {
		t.Errorf("a slow drag of a quarter page turned to %q", id)
	}
	// Past the last page the pages rubber band and stay.
	e.v.Next()
	e.h.Settle()
	e.wait(time.Second)
	e.h.Fling(geom.Pt(700, 240), geom.Pt(100, 240), 150*time.Millisecond)
	e.wait(time.Second)
	if id, _ := e.v.Current(); id != e.ids[3] {
		t.Errorf("a flick past the last page went to %q", id)
	}
}

func TestDraggingThePictureDownCloses(t *testing.T) {
	e := setup(t, 3, nil)
	e.open(0)
	closed := false
	e.v.OnClose(func() { closed = true })
	// Not far enough and slowly: it comes back.
	e.h.Fling(geom.Pt(400, 200), geom.Pt(400, 260), time.Second)
	e.wait(time.Second)
	if !e.v.IsOpen() {
		t.Fatal("a short slow drag down closed the viewer")
	}
	e.h.Fling(geom.Pt(400, 200), geom.Pt(400, 330), 100*time.Millisecond)
	e.wait(2 * time.Second)
	if e.v.IsOpen() || !closed {
		t.Fatal("a flick down did not close the viewer")
	}
	if e.origin.concealed != "" {
		t.Errorf("the origin still conceals %q after closing", e.origin.concealed)
	}
}

func TestPinchZoomsAndBouncesBackFromTheLimit(t *testing.T) {
	e := setup(t, 2, func(f imageviewer.FullscreenView) imageviewer.FullscreenView { return f.MaxZoom(3) })
	e.open(0)
	e.h.Pinch(geom.Pt(400, 240), 100, 500, 300*time.Millisecond)
	e.h.Frame()
	stretched := e.pictures()[0].Bounds.Width()
	if stretched <= 720*3 || stretched >= 720*3*1.5 {
		t.Fatalf("a pinch of 5x shows %v wide right after the release, want past the limit of %v and held back by the rubber band", stretched, 720*3)
	}
	e.load()
	e.wait(2 * time.Second)
	if w := e.pictures()[0].Bounds.Width(); !near(w, 720*3, 2) {
		t.Errorf("after the bounce the picture is %v wide, want %v", w, 720*3)
	}
	// A double tap goes back to the fit.
	e.h.TapAt(geom.Pt(400, 240))
	e.wait(100 * time.Millisecond)
	e.h.TapAt(geom.Pt(400, 240))
	e.wait(time.Second)
	if w := e.pictures()[0].Bounds.Width(); !near(w, 720, 1) {
		t.Errorf("a double tap left the picture %v wide, want the fit", w)
	}
}

func TestPanningAZoomedPictureOnPastItsEdgeTurnsThePage(t *testing.T) {
	e := setup(t, 3, func(f imageviewer.FullscreenView) imageviewer.FullscreenView { return f.DoubleTapZoom(2) })
	e.open(0)
	e.h.TapAt(geom.Pt(400, 240))
	e.wait(100 * time.Millisecond)
	e.h.TapAt(geom.Pt(400, 240))
	e.wait(time.Second)
	// 1440 wide: 320 px of travel to the right edge, then the page.
	e.h.Fling(geom.Pt(700, 240), geom.Pt(100, 240), 200*time.Millisecond)
	e.load()
	e.wait(time.Second)
	if id, _ := e.v.Current(); id != e.ids[1] {
		t.Fatalf("panning past the edge of a zoomed picture shows %q, want the next", id)
	}
}

func TestKeysTurnZoomAndClose(t *testing.T) {
	e := setup(t, 3, nil)
	e.open(0)
	e.h.Key(gift.KeyRight)
	e.wait(time.Second)
	if id, _ := e.v.Current(); id != e.ids[1] {
		t.Fatalf("right arrow shows %q", id)
	}
	e.h.Key(gift.KeyEnd)
	e.h.Settle()
	if id, _ := e.v.Current(); id != e.ids[2] {
		t.Fatalf("end shows %q", id)
	}
	e.h.TypeRune('+')
	e.load()
	e.wait(time.Second)
	if w := e.pictures()[0].Bounds.Width(); !near(w, 1080, 2) {
		t.Errorf("plus zoomed to %v wide, want 1.5 times 720", w)
	}
	e.h.Key(gift.KeyEscape)
	e.wait(2 * time.Second)
	if e.v.IsOpen() {
		t.Error("escape did not close the viewer")
	}
}

func TestTheArrowsComeWithTheMouseAndGo(t *testing.T) {
	e := setup(t, 3, nil)
	e.open(1)
	arrows := func() int {
		n := 0
		for _, op := range e.h.Ops() {
			if op.Kind == render.OpFillRoundRect && near(op.Bounds.Width(), 40, 0.5) && op.Bounds.Min.Y > 200 {
				n++
			}
		}
		return n
	}
	if n := arrows(); n != 0 {
		t.Fatalf("%d arrows before the mouse moved", n)
	}
	e.h.MoveTo(geom.Pt(400, 300))
	e.h.MoveTo(geom.Pt(410, 300))
	e.wait(300 * time.Millisecond)
	if n := arrows(); n != 2 {
		t.Fatalf("%d arrows after the mouse moved, want both", n)
	}
	e.h.Find(gifttest.ByKey("viewer-next")).Click()
	e.wait(time.Second)
	if id, _ := e.v.Current(); id != e.ids[2] {
		t.Errorf("the next arrow shows %q", id)
	}
	e.wait(3 * time.Second)
	if n := arrows(); n != 0 {
		t.Errorf("%d arrows three seconds after the mouse rested", n)
	}
}

func TestASwipeUpRevealsTheCard(t *testing.T) {
	e := setup(t, 2, func(f imageviewer.FullscreenView) imageviewer.FullscreenView {
		return f.Card(func(id asset.ID) gift.View { return ui.Text(string(id)).Key("details") })
	})
	e.open(0)
	if b := e.h.Find(gifttest.ByKey("details")).LayoutBounds(); b.Min.Y < 470 {
		t.Fatalf("the closed card is at %v, want below the screen", b)
	}
	before := e.pictures()[0].Bounds
	e.h.Fling(geom.Pt(400, 400), geom.Pt(400, 250), 150*time.Millisecond)
	e.wait(time.Second)
	if b := e.h.Find(gifttest.ByKey("details")).LayoutBounds(); b.Min.Y > 300 {
		t.Fatalf("after a swipe up the card is at %v, want on screen", b)
	}
	// A sheet over the picture: the picture stays where it was.
	if after := e.pictures()[0].Bounds; !near(after.Min.X, before.Min.X, 0.5) || !near(after.Min.Y, before.Min.Y, 0.5) ||
		!near(after.Max.X, before.Max.X, 0.5) || !near(after.Max.Y, before.Max.Y, 0.5) {
		t.Errorf("the open card moved the picture from %v to %v", before, after)
	}
	e.h.Key(gift.KeyEscape)
	e.wait(time.Second)
	if !e.v.IsOpen() {
		t.Fatal("escape closed the viewer instead of the card")
	}
	if b := e.h.Find(gifttest.ByKey("details")).LayoutBounds(); b.Min.Y < 470 {
		t.Errorf("after escape the card is at %v, want put away", b)
	}
}
