package turbojpeg_test

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/asset/turbojpeg"
)

// photo is a synthetic camera picture: a gradient with sensor-like noise, so
// that it compresses like a photograph rather than like a test card. A flat
// gradient encodes to a tenth of the bytes and decodes in a fraction of the
// time, which would flatter both decoders.
func photo(w, h int) []byte {
	img := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	var seed uint32 = 7
	for y := range h {
		for x := range w {
			seed = seed*1664525 + 1013904223
			img.Y[y*img.YStride+x] = uint8(x*180/w+y*60/h) + uint8(seed>>27)
		}
	}
	for y := range h / 2 {
		for x := range w / 2 {
			img.Cb[y*img.CStride+x] = uint8(96 + x*64/(w/2))
			img.Cr[y*img.CStride+x] = uint8(160 - y*64/(h/2))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// The real decoder, registered the way an application does, produces the same
// thumbnail through the pipeline as image/jpeg does, and decodes less to do
// it.
func TestPipelineWithTurboJPEG(t *testing.T) {
	if !turbojpeg.Register() {
		t.Skip("libturbojpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "p.jpg")
	if err := os.WriteFile(path, photo(1600, 1200), 0o600); err != nil {
		t.Fatal(err)
	}

	thumb := func() (*image.RGBA, asset.Stats) {
		var (
			mu  sync.Mutex
			got []asset.Result
		)
		done := make(chan struct{})
		p := asset.NewPipeline(asset.Config{Workers: 1, Sizes: []int{256},
			Deliver: func(fn func()) { fn() }})
		defer p.Close()
		p.Request(asset.Request{Source: asset.File(path), Size: 256, Priority: asset.Visible,
			OnResult: func(r asset.Result) {
				mu.Lock()
				got = append(got, r)
				mu.Unlock()
				close(done)
			}})
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("no result")
		}
		if err := got[0].Err(); err != nil {
			t.Fatal(err)
		}
		// Copied out, because Close gives the thumbnail's pixels back.
		view := got[0].Image.RGBA()
		cp := image.NewRGBA(view.Rect)
		copy(cp.Pix, view.Pix)
		return cp, p.Stats()
	}

	want, goStats := thumb()
	// Process wide and not undone: this test binary has no other test
	// that decodes JPEG through the registry, and it runs last.
	asset.RegisterDecoder(asset.MIMEJPEG, turbojpeg.Decoder{})
	got, tjStats := thumb()

	if got.Rect.Dx() != 256 || got.Rect.Dy() != 192 || want.Rect.Dx() != 256 {
		t.Fatalf("thumbnails %dx%d and %dx%d, want 256x192",
			got.Rect.Dx(), got.Rect.Dy(), want.Rect.Dx(), want.Rect.Dy())
	}
	// 1600x1200 to 256x192 is a factor 6.25, so the decoder takes 1/4 —
	// 400x300 — and not 1/8, which would be 200x150 and too small.
	if tjStats.DecodedPixels != 400*300 {
		t.Errorf("turbojpeg decoded %d pixels, want the 400x300 of a quarter", tjStats.DecodedPixels)
	}
	if goStats.DecodedPixels != 1600*1200 {
		t.Errorf("image/jpeg decoded %d pixels, want all of them", goStats.DecodedPixels)
	}
	if tjStats.Decode.Peak >= goStats.Decode.Peak {
		t.Errorf("turbojpeg reserved %d bytes, image/jpeg %d; scaling should reserve less",
			tjStats.Decode.Peak, goStats.Decode.Peak)
	}

	var sum, n float64
	for y := range got.Rect.Dy() {
		for x := range got.Rect.Dx() {
			a, b := got.RGBAAt(x, y), want.RGBAAt(x, y)
			for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B)} {
				sum += float64(max(d, -d))
				n++
			}
		}
	}
	// The two differ in where they average — the IDCT or the resampler —
	// and the noise is the high frequency they average differently.
	if e := sum / n; e > 6 {
		t.Errorf("thumbnails differ by %.2f on average", e)
	} else {
		t.Logf("thumbnails differ by %.2f on average", e)
	}
}

// --- benchmarks -------------------------------------------------------------

var (
	benchOnce sync.Once
	benchData []byte
)

// benchPhoto is 12 megapixels, the smallest camera the photo booth sees, or
// the file GIFT_BENCH_JPEG names, to measure a real camera's output.
func benchPhoto(b *testing.B) []byte {
	benchOnce.Do(func() {
		if name := os.Getenv("GIFT_BENCH_JPEG"); name != "" {
			data, err := os.ReadFile(name)
			if err != nil {
				panic(err)
			}
			benchData = data
			return
		}
		benchData = photo(4000, 3000)
	})
	return benchData
}

// BenchmarkDecode compares the decoders on the work the pipeline does for a
// gallery tile: decode, then scale to the rung with the pipeline's default
// ApproxBiLinear. The full size decodes are there for scale.
func BenchmarkDecode(b *testing.B) {
	data := benchPhoto(b)
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("encoded picture: %dx%d, %.1f MB", cfg.Width, cfg.Height, float64(len(data))/1e6)

	b.Run("imagejpeg/full", func(b *testing.B) {
		for b.Loop() {
			if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("turbojpeg/full", func(b *testing.B) {
		if !turbojpeg.Available() {
			b.Skip("libturbojpeg not installed")
		}
		for b.Loop() {
			if _, err := turbojpeg.Decode(data); err != nil {
				b.Fatal(err)
			}
		}
	})
	for _, rung := range []int{256, 1024} {
		tw, th := rung, rung*cfg.Height/cfg.Width
		if cfg.Height > cfg.Width {
			tw, th = rung*cfg.Width/cfg.Height, rung
		}
		dst := image.NewRGBA(image.Rect(0, 0, tw, th))
		b.Run("imagejpeg/tile"+itoa(rung), func(b *testing.B) {
			for b.Loop() {
				img, err := jpeg.Decode(bytes.NewReader(data))
				if err != nil {
					b.Fatal(err)
				}
				xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Src, nil)
			}
		})
		b.Run("turbojpeg/tile"+itoa(rung), func(b *testing.B) {
			if !turbojpeg.Available() {
				b.Skip("libturbojpeg not installed")
			}
			for b.Loop() {
				img, err := turbojpeg.DecodeScaled(data, dst.Rect.Dx(), dst.Rect.Dy())
				if err != nil {
					b.Fatal(err)
				}
				xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Src, nil)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
