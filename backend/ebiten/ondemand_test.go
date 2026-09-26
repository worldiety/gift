package ebiten

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// TestDeferredUploadsAskForAnotherFrame: a picture the per frame budget turned
// away is a placeholder on the screen. With DrawOnDemand nothing else would
// ever paint it again, so the cache has to say so until a frame gets it in.
func TestDeferredUploadsAskForAnotherFrame(t *testing.T) {
	tc := NewTextureCache(TextureConfig{UploadsPerFrame: 1, UploadBytesPerFrame: -1})

	tc.BeginFrame()
	tc.Acquire(pixels(8))
	tc.Acquire(pixels(8))
	if d := tc.Deferred(); d != 1 {
		t.Fatalf("Deferred after an over budget frame = %d, want 1", d)
	}

	// Between two frames the count stands: that is when the backend asks.
	if d := tc.Deferred(); d != 1 {
		t.Fatalf("Deferred between frames = %d, want 1", d)
	}

	tc.BeginFrame()
	if _, ok := tc.Acquire(pixels(8)); !ok {
		t.Fatal("the deferred picture did not get in the next frame")
	}

	if d := tc.Deferred(); d != 0 {
		t.Fatalf("Deferred after a frame within budget = %d, want 0", d)
	}
}

// TestMustDrawSkipsOnlyUnchangedFrames is the decision of DrawOnDemand, case
// by case.
func TestMustDrawSkipsOnlyUnchangedFrames(t *testing.T) {
	app := gift.New(gift.Options{
		Root: func(*gift.Context) gift.View { return recorderView{app: &keyboardApp{}} },
	})
	g := &game{app: app, r: mustRenderer(t), onDemand: true}
	size := geom.Sz(800, 480)

	if err := app.Update(size); err != nil {
		t.Fatal(err)
	}

	if !g.mustDraw(size) {
		t.Fatal("the first frame must be drawn")
	}

	app.Paint()
	g.drawn, g.drawnSize = 2, size

	if g.mustDraw(size) {
		t.Fatal("an unchanged tree after two frames must not be drawn again")
	}

	if !g.mustDraw(geom.Sz(1024, 600)) {
		t.Fatal("a resized screen has undefined content and must be drawn")
	}

	app.Invalidate()
	if err := app.Update(size); err != nil {
		t.Fatal(err)
	}

	if !g.mustDraw(size) {
		t.Fatal("a changed tree must be drawn")
	}

	app.Paint()
	g.r.textures.frameDeferred = 2
	if !g.mustDraw(size) {
		t.Fatal("deferred uploads leave placeholders and must be drawn again")
	}

	g.r.textures.frameDeferred = 0
	if g.mustDraw(size) {
		t.Fatal("nothing left to do, yet a frame was asked for")
	}
}

// TestSkippedFramesAreNotSlowFrames: the glass policy judges intervals
// between drawn frames. A pause with nothing to draw is not a stall.
func TestSkippedFramesAreNotSlowFrames(t *testing.T) {
	r := mustRenderer(t)
	r.lastDraw = time.Now().Add(-3 * time.Second)
	r.SkipFrame()
	if !r.lastDraw.IsZero() {
		t.Fatal("SkipFrame must forget the last draw, or the next interval spans the pause")
	}
}
