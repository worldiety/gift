package ebiten

import (
	"testing"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// wallpaper uploads a w by h picture with the given alpha and returns its id.
func wallpaper(t *testing.T, r *Renderer, w, h int, alpha byte) render.ImageID {
	t.Helper()
	pix := make([]byte, w*h*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = byte(i/4%w)*alpha/255, 90*alpha/255, 200*alpha/255, alpha
	}
	hd, ok := r.Textures().Acquire(render.Pixels{Pix: pix, W: w, H: h, Stride: w * 4})
	if !ok {
		t.Fatal("upload refused")
	}
	id, ok := r.Textures().Resolve(hd)
	if !ok {
		t.Fatal("resolve failed")
	}
	return id
}

func addPicture(l *render.List, id render.ImageID, b geom.Rect) {
	l.Add(render.Op{Kind: render.OpImage, Image: id, Bounds: b, Color: render.Color{R: 1, G: 1, B: 1, A: 1}})
}

func paneFrame(t *testing.T, r *Renderer, build func(l *render.List)) string {
	t.Helper()
	trace := glassTrace(r)
	var l render.List
	l.Reset()
	build(&l)
	r.BeginFrame(geom.Sz(800, 600))
	r.Submit(&l)
	r.EndFrame()
	return trace()
}

var paneGlass = render.NewGlass().Quality(render.Full)

// TestAPaneOverAPictureUsesItAsItsBackdrop is the claim of backdrop.go: no
// copy, no chain per frame, no scene target – one composite.
func TestAPaneOverAPictureUsesItAsItsBackdrop(t *testing.T) {
	r, _ := sceneRenderer(t, 800, 600)
	id := wallpaper(t, r, 64, 48, 255)
	build := func(l *render.List) {
		addPicture(l, id, geom.Rc(0, 0, 800, 600))
		l.Add(render.Op{Kind: render.OpShadow, Bounds: geom.Rc(90, 90, 310, 210), Color: render.RGBA(0, 0, 0, 80), Blur: 16})
		addGlass(l, geom.Rc(100, 100, 300, 200), 16, paneGlass)
		addGlass(l, geom.Rc(400, 100, 600, 200), 16, paneGlass)
	}

	first := paneFrame(t, r, build)
	if st := r.Stats(); st.GlassStaticOps != 2 || st.GlassStaticBlurs != 1 {
		t.Fatalf("first frame: %d static panes, %d blurs; trace %s", st.GlassStaticOps, st.GlassStaticBlurs, first)
	}
	second := paneFrame(t, r, build)
	if second != "static,static" {
		t.Errorf("a frame of two panes over a picture ran %q, want two composites and nothing else", second)
	}
	if st := r.Stats(); st.GlassStaticBlurs != 1 {
		t.Errorf("the picture was blurred again: %d blurs", st.GlassStaticBlurs)
	}
}

// TestAPaneOverSomethingDrawnOnThePictureIsLive: a label on the wallpaper
// under a pane is part of its backdrop.
func TestAPaneOverSomethingDrawnOnThePictureIsLive(t *testing.T) {
	r, _ := sceneRenderer(t, 800, 600)
	id := wallpaper(t, r, 64, 48, 255)
	trace := paneFrame(t, r, func(l *render.List) {
		addPicture(l, id, geom.Rc(0, 0, 800, 600))
		addRect(l, geom.Rc(150, 150, 170, 170), render.RGB(255, 0, 0))
		addRect(l, geom.Rc(500, 500, 520, 520), render.RGB(255, 0, 0))
		addGlass(l, geom.Rc(100, 100, 300, 200), 16, paneGlass)
		addGlass(l, geom.Rc(400, 100, 600, 200), 16, paneGlass)
	})
	st := r.Stats()
	if st.GlassStaticOps != 1 || st.GlassFullOps != 1 {
		t.Errorf("want the pane over the label live and the other static, got %d static and %d live; trace %s",
			st.GlassStaticOps, st.GlassFullOps, trace)
	}
}

// TestAPaneOverATranslucentPictureIsLive: what shows through the picture is
// part of the backdrop too.
func TestAPaneOverATranslucentPictureIsLive(t *testing.T) {
	r, _ := sceneRenderer(t, 800, 600)
	id := wallpaper(t, r, 64, 48, 128)
	paneFrame(t, r, func(l *render.List) {
		addPicture(l, id, geom.Rc(0, 0, 800, 600))
		addGlass(l, geom.Rc(100, 100, 300, 200), 16, paneGlass)
	})
	if st := r.Stats(); st.GlassStaticOps != 0 || st.GlassFullOps != 1 {
		t.Errorf("a pane over a translucent picture: %d static, %d live", st.GlassStaticOps, st.GlassFullOps)
	}
}

// TestAPaneOverAPictureNeedsNoSceneTarget: the screen sized offscreen is owed
// only to live glass.
func TestAPaneOverAPictureNeedsNoSceneTarget(t *testing.T) {
	r, _ := sceneRenderer(t, 800, 600)
	id := wallpaper(t, r, 64, 48, 255)
	build := func(l *render.List) {
		addPicture(l, id, geom.Rc(0, 0, 800, 600))
		addGlass(l, geom.Rc(100, 100, 300, 200), 16, paneGlass)
	}
	paneFrame(t, r, build)
	before := r.Targets().Stats().Leases
	if trace := paneFrame(t, r, build); trace != "static" {
		t.Errorf("trace %q, want a single static composite and no scene blit", trace)
	}
	if n := r.Targets().Stats().Leases - before; n != 0 {
		t.Errorf("a frame whose only pane has a static backdrop leased %d targets", n)
	}
}

// TestAPaneInASlidingPageKeepsItsStaticBackdrop: drawn through, the page's
// wallpaper moves with it and is the same blurred picture in every frame.
func TestAPaneInASlidingPageKeepsItsStaticBackdrop(t *testing.T) {
	r, _ := sceneRenderer(t, 800, 600)
	id := wallpaper(t, r, 64, 48, 255)
	for i, dx := range []float32{0, 120, 240.5} {
		trace := paneFrame(t, r, func(l *render.List) {
			bounds := geom.Rc(0, 0, 800, 600)
			parent := l.PushXform(geom.Translate(geom.Pt(dx, 0)))
			inner := l.BeginLayer(1, bounds, parent, 0)
			l.Add(render.Op{Kind: render.OpImage, Image: id, Bounds: bounds, Color: render.Color{R: 1, G: 1, B: 1, A: 1}, Clip: l.CurrentClip(), Xform: inner})
			l.Add(render.Op{Kind: render.OpMaterial, Bounds: geom.Rc(100, 100, 300, 200), CornerRadius: 16,
				Material: l.AddMaterial(paneGlass.Material()), Clip: l.CurrentClip(), Xform: inner})
			l.EndLayer()
		})
		if i > 0 && trace != "static" {
			// The first frame blurs the picture, once.
			t.Errorf("frame %d at dx=%v ran %q, want one static composite", i, dx, trace)
		}
	}
	if st := r.Stats(); st.GlassStaticBlurs != 1 {
		t.Errorf("the wallpaper of a sliding page was blurred %d times", st.GlassStaticBlurs)
	}
}
