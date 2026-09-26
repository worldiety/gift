package asset_test

import (
	"image"
	"image/jpeg"
	"io"
	"sync"
	"testing"

	xdraw "golang.org/x/image/draw"

	"github.com/worldiety/gift/asset"
)

// scalingJPEG is a [asset.ScaledDecoder] for JPEG that records what the
// pipeline asked of it. It decodes with image/jpeg and scales to twice the
// minimum, which is what a real decoder with fixed factors does too: more than
// asked, less than stored.
type scalingJPEG struct {
	asset.Decoder // the built in one, for DecodeConfig and MemoryFactor

	mu               sync.Mutex
	scaledCalls      int
	fullCalls        int
	minW, minH       int
	memW, memH       int
	gotBuffer        bool
	outW, outH       int
	reservationBytes int64
}

func (d *scalingJPEG) Sniff(h []byte) bool {
	return len(h) >= 3 && h[0] == 0xFF && h[1] == 0xD8 && h[2] == 0xFF
}

func (d *scalingJPEG) Decode(r io.Reader) (image.Image, error) {
	d.mu.Lock()
	d.fullCalls++
	d.mu.Unlock()
	return jpeg.Decode(r)
}

func (d *scalingJPEG) ScaledMemory(header []byte, w, h, minW, minH int) int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.memW, d.memH = minW, minH
	return d.reservationBytes
}

func (d *scalingJPEG) DecodeScaled(r io.Reader, minW, minH int) (image.Image, error) {
	_, isBuffer := r.(interface{ Bytes() []byte })
	full, err := jpeg.Decode(r)
	if err != nil {
		return nil, err
	}
	b := full.Bounds()
	w, h := min(b.Dx(), 2*minW), min(b.Dy(), 2*minH)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.NearestNeighbor.Scale(out, out.Bounds(), full, b, xdraw.Src, nil)

	d.mu.Lock()
	defer d.mu.Unlock()
	d.scaledCalls++
	d.minW, d.minH = minW, minH
	d.gotBuffer = isBuffer
	d.outW, d.outH = w, h
	return out, nil
}

// useDecoder registers d for JPEG for the duration of the test. The registry
// is process wide, which is safe here only because this package runs no test
// in parallel; see the project plan, section 13.
func useDecoder(t *testing.T, d asset.Decoder) {
	t.Helper()
	asset.RegisterDecoder(asset.MIMEJPEG, d)
	t.Cleanup(func() { asset.RegisterDecoder(asset.MIMEJPEG, asset.JPEGDecoderForTest()) })
}

// The scaled path is the one taken for a ScaledDecoder, it is asked for the
// thumbnail's size in *stored* space, its own memory figure is what is
// reserved, and the smaller picture it returns still makes a correctly sized
// and oriented thumbnail.
func TestScaledDecoderIsAskedForTheStoredTarget(t *testing.T) {
	const reservation = 12345
	d := &scalingJPEG{Decoder: asset.JPEGDecoderForTest(), reservationBytes: reservation}
	useDecoder(t, d)

	dir := t.TempDir()
	// Stored 800 by 400, orientation 6: shown as 400 by 800. Asked for 128,
	// the thumbnail is 64 by 128 as shown and therefore 128 by 64 stored.
	raw := withEXIFOrientation(t, jpegBytes(t, 800, 400), asset.OrientationRightTop)
	path := writeFile(t, dir, "rot.jpg", raw)

	c := newCollector()
	p := asset.NewPipeline(baseConfig(c))
	defer p.Close()
	p.Request(asset.Request{Source: asset.File(path), Size: 128,
		Priority: asset.Visible, OnResult: c.onResult})
	r := c.waitFor(t, 1)[0]
	if r.Err() != nil {
		t.Fatal(r.Err())
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.scaledCalls != 1 || d.fullCalls != 0 {
		t.Fatalf("DecodeScaled called %d times, Decode %d; want the scaled path once",
			d.scaledCalls, d.fullCalls)
	}
	if d.minW != 128 || d.minH != 64 {
		t.Errorf("DecodeScaled asked for %dx%d, want the stored-space 128x64", d.minW, d.minH)
	}
	if d.memW != d.minW || d.memH != d.minH {
		t.Errorf("ScaledMemory asked about %dx%d, DecodeScaled for %dx%d",
			d.memW, d.memH, d.minW, d.minH)
	}
	if !d.gotBuffer {
		t.Error("the reader has no Bytes method; a C decoder would have to copy the picture")
	}
	if r.Image.Width() != 64 || r.Image.Height() != 128 {
		t.Errorf("thumbnail = %dx%d, want a portrait 64x128", r.Image.Width(), r.Image.Height())
	}

	st := p.Stats()
	if st.Decode.Peak != reservation {
		t.Errorf("decode budget peak = %d, want the decoder's own %d", st.Decode.Peak, reservation)
	}
	if want := uint64(d.outW * d.outH); st.DecodedPixels != want {
		t.Errorf("DecodedPixels = %d, want the %d the decoder produced", st.DecodedPixels, want)
	}
}

// A ScaledDecoder that reports no memory must not escape the budget: the
// reservation falls back to the unscaled figure.
func TestScaledDecoderWithoutAFigureIsStillBudgeted(t *testing.T) {
	d := &scalingJPEG{Decoder: asset.JPEGDecoderForTest()}
	useDecoder(t, d)

	dir := t.TempDir()
	path := writeFile(t, dir, "a.jpg", jpegBytes(t, 400, 200))
	c := newCollector()
	p := asset.NewPipeline(baseConfig(c))
	defer p.Close()
	p.Request(asset.Request{Source: asset.File(path), Size: 64,
		Priority: asset.Visible, OnResult: c.onResult})
	if r := c.waitFor(t, 1)[0]; r.Err() != nil {
		t.Fatal(r.Err())
	}
	want := int64(400 * 200 * d.MemoryFactor())
	if got := p.Stats().Decode.Peak; got != want {
		t.Errorf("decode budget peak = %d, want the unscaled %d", got, want)
	}
}
