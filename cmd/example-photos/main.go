// Command example-photos is a photo browser in the manner of iOS's Photos: a
// gallery of thumbnails, and a full screen viewer that grows out of the tile
// that was tapped and shrinks back into it.
//
//	go run ./cmd/example-photos
//	go run ./cmd/example-photos -dir ~/Pictures -fullscreen
//
// In the viewer: swipe or use the arrow keys to page, pinch, double tap or
// wheel to zoom, drag a picture down or press escape to close it, swipe up for
// its details. With a mouse the arrows at the sides appear while it moves.
//
// With no -dir the example writes two dozen large pictures into its cache
// directory on first run — landscapes, portraits and panoramas of up to 12
// megapixels, with fine detail to zoom into — so that the staged loading has
// something to do. libjpeg-turbo is used when it is installed; it decodes the
// first stage of a camera picture from the DC coefficients of its blocks.
// Raw files of cameras, ARW and DNG, are shown as the preview the camera
// embedded in them, or with -develop as their raw data developed; see
// package raw.
//
// On Linux the touch screen is read directly, so pinching works on a
// Raspberry Pi display; see [backend.Config.Touchscreen].
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/asset/turbojpeg"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/raw"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/imageviewer"
)

var (
	dir        = flag.String("dir", os.Getenv("GIFT_PHOTOS_DIR"), "directory of JPEG, PNG and raw (ARW, DNG) pictures; empty generates samples")
	cache      = flag.String("cache", defaultCacheDir(), "persistent thumbnail cache directory; empty disables it")
	fullscreen = flag.Bool("fullscreen", false, "start full screen")
	width      = flag.Int("width", 1280, "window width")
	height     = flag.Int("height", 800, "window height")
	turbo      = flag.Bool("turbo", true, "decode JPEG with libjpeg-turbo when it is installed")
	open       = flag.Int("open", -1, "open the viewer on this picture at start, for screenshots")
	stats      = flag.Bool("stats", false, "print frames per second and the stage decodes every five seconds")
	workers    = flag.Int("workers", 0, "decode goroutines; zero is the pipeline's default")
	upload     = flag.Int("upload", 0, "picture bytes uploaded per frame, in KiB; zero is the backend's default")
	develop    = flag.Bool("develop", false, "show raw files developed from their raw data rather than as the camera's preview")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "example-photos:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := example.LoadFont(); err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if *turbo && turbojpeg.Register() {
		asset.RegisterDecoder(asset.MIMEJPEG, turbojpeg.Decoder{})
		logger.Info("JPEG through libjpeg-turbo")
	}
	space := backend.WorkingSpace(nil)
	asset.RegisterDecoder(raw.MIMEARW, raw.Decoder{Format: raw.ARW, Develop: *develop, Space: space})
	asset.RegisterDecoder(raw.MIMEDNG, raw.Decoder{Format: raw.DNG, Develop: *develop, Space: space})
	srcDir := *dir
	if srcDir == "" {
		var err error
		if srcDir, err = generateSamples(logger); err != nil {
			return err
		}
	}
	sources, err := scan(srcDir)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return fmt.Errorf("no JPEG, PNG or raw files in %s", srcDir)
	}
	items := make([]asset.Metadata, len(sources))
	byID := make(map[asset.ID]asset.Source, len(sources))
	for i, s := range sources {
		items[i] = s.Metadata()
		byID[items[i].ID] = s
	}

	var gallery *ui.Gallery
	var viewer *imageviewer.Viewer
	app := gift.New(gift.Options{Root: func(ctx *gift.Context) gift.View {
		return root(gallery, viewer)
	}})
	pipe := asset.NewPipeline(asset.Config{
		Deliver: app.Post,
		Space:   space,
		Workers: *workers,
		// Rungs up to 4096 for the viewer: a screen of 1920 asks for 2048,
		// and a zoomed picture for 4096. The large ones stay off the disk.
		Sizes:       []int{128, 256, 512, 1024, 2048, 4096},
		PixelBudget: 256 << 20,
		Disk:        asset.DiskCacheConfig{Dir: *cache, MaxSize: 512},
		Logger:      logger,
	})
	defer func() {
		pipe.Close()
		app.DrainPosts()
	}()
	ui.SetImagePipeline(pipe)

	gallery = ui.NewGallery(asset.NewCollection(items))
	resolve := func(id asset.ID) asset.Source { return byID[id] }
	gallery.SetSources(resolve)
	viewer = imageviewer.New(gallery.Collection(), resolve)
	viewer.SetOrigin(gallery)
	if *open >= 0 && *open < len(items) {
		app.Post(func() { viewer.Open(items[*open].ID) })
	}

	cfg := backend.Config{
		Title: "gift photos", Width: *width, Height: *height,
		// The two settings of a kiosk on a Raspberry Pi: draw only what
		// changed, and draw it straight into the window, which saves a full
		// screen pass per frame.
		DrawOnDemand: true, DirectToScreen: true, IdleTPS: 10,
		Logger:     logger,
		AssetStats: ui.ImagePipelineStats,
	}
	if *upload > 0 {
		cfg.Textures.UploadBytesPerFrame = int64(*upload) << 10
	}
	if *stats {
		report(&cfg, pipe)
	}
	if *fullscreen {
		eb.SetFullscreen(true)
	}
	return backend.Run(app, cfg)
}

// report prints, every five seconds, the frames drawn and the decodes.
func report(cfg *backend.Config, pipe *asset.Pipeline) {
	var r *backend.Renderer
	var last backend.RendererStats
	lastAt := time.Now()
	cfg.OnRenderer = func(rr *backend.Renderer) { r = rr }
	cfg.OnUpdate = func() error {
		if time.Since(lastAt) < 5*time.Second {
			return nil
		}
		s, p := r.Stats(), pipe.Stats()
		fmt.Printf("%.1f frames/s, %d decodes, %d disk hits\n",
			float64(s.Frames-last.Frames)/time.Since(lastAt).Seconds(), p.Decodes, p.DiskHits)
		last, lastAt = s, time.Now()
		return nil
	}
}

// root is the gallery with the viewer over it.
func root(g *ui.Gallery, v *imageviewer.Viewer) gift.View {
	return ui.ZStack(
		ui.ImageGallery(g).
			Layout(ui.Justified().RowHeight(180).Gap(4)).
			Tile(ui.TileStyle{CornerRadius: 6}).
			Padding(4).
			OnOpen(func(id asset.ID) { v.Open(id) }).
			OnActivate(func(id asset.ID) { v.Open(id) }).
			Key("gallery"),
		imageviewer.Fullscreen(v).Card(details(v)).Key("viewer"),
	).Background(ui.RGB(16, 16, 18))
}

// details is the card below a picture: what the catalogue knows about it.
func details(v *imageviewer.Viewer) func(asset.ID) gift.View {
	return func(id asset.ID) gift.View {
		m := asset.Metadata{ID: id}
		if i, ok := v.Collection().Find(id); ok {
			m = v.Collection().At(i)
		}
		name := filepath.Base(string(id))
		size := "unknown size"
		if m.Width > 0 {
			size = fmt.Sprintf("%d × %d pixels, %.1f megapixels", m.Width, m.Height, float64(m.Width)*float64(m.Height)/1e6)
		}
		return ui.VStack(
			ui.Text(name).FontSize(20).Foreground(ui.RGB(255, 255, 255)),
			ui.Text(size).Foreground(ui.RGB(200, 200, 205)),
			ui.Text("Swipe down to put this card away.").Foreground(ui.RGB(150, 150, 155)),
		).Gap(8).Padding(24).Align(geom.TopLeading)
	}
}

// scan collects the pictures of a directory, sorted by name.
func scan(dir string) ([]asset.Source, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".jpg", ".jpeg", ".png", ".arw", ".dng":
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	out := make([]asset.Source, 0, len(names))
	for _, n := range names {
		out = append(out, asset.File(filepath.Join(dir, n)))
	}
	return out, nil
}

func defaultCacheDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "gift", "example-photos")
}

// The sample pictures: three shapes at camera sizes.
var shapes = [...][2]int{{4000, 3000}, {2400, 3200}, {4800, 1800}, {3000, 2000}}

// generateSamples writes the samples once and returns their directory.
func generateSamples(logger *slog.Logger) (string, error) {
	base := defaultCacheDir()
	if base == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "samples-v1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	const n = 24
	for i := range n {
		name := filepath.Join(dir, fmt.Sprintf("photo-%02d.jpg", i+1))
		if _, err := os.Stat(name); err == nil {
			continue
		}
		if i == 0 {
			logger.Info("writing sample pictures, once", slog.String("dir", dir))
		}
		s := shapes[i%len(shapes)]
		img := sample(i, s[0], s[1])
		f, err := os.Create(name + ".tmp")
		if err != nil {
			return "", err
		}
		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 88})
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return "", err
		}
		if err := os.Rename(name+".tmp", name); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// sample is a picture with something to look at at every zoom: a landscape of
// coloured bands, a fine grid that only a full decode resolves, and its number
// in large digits.
func sample(i, w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	hue := float64(i) * 0.137
	for y := range h {
		fy := float64(y) / float64(h)
		for x := range w {
			fx := float64(x) / float64(w)
			wave := 0.5 + 0.5*math.Sin(fx*9+math.Sin(fy*5+hue*6)*2+hue*10)
			r := 0.45 + 0.4*math.Sin(2*math.Pi*(hue+fy*0.3)+wave)
			g := 0.45 + 0.4*math.Sin(2*math.Pi*(hue+0.33+fx*0.2)+wave)
			b := 0.45 + 0.4*math.Sin(2*math.Pi*(hue+0.66-fy*0.2))
			// A fine grid every 16 pixels and a finer one every 4: the
			// detail a preview cannot show and a zoomed full decode must.
			if x%16 == 0 || y%16 == 0 {
				r, g, b = r*0.7, g*0.7, b*0.7
			} else if (x/4+y/4)%2 == 0 {
				r, g, b = r*0.94, g*0.94, b*0.94
			}
			o := img.PixOffset(x, y)
			img.Pix[o] = uint8(255 * clamp01(r))
			img.Pix[o+1] = uint8(255 * clamp01(g))
			img.Pix[o+2] = uint8(255 * clamp01(b))
			img.Pix[o+3] = 255
		}
	}
	digits(img, i+1, w/10, h/10, min(w, h)/4)
	return img
}

func clamp01(f float64) float64 { return math.Max(0, math.Min(1, f)) }

// digits draws n with seven segment digits of height size at (x, y).
func digits(img *image.RGBA, n, x, y, size int) {
	seg := [10]uint8{0x3f, 0x06, 0x5b, 0x4f, 0x66, 0x6d, 0x7d, 0x07, 0x7f, 0x6f}
	s := fmt.Sprint(n)
	t := max(4, size/8)
	w := size / 2
	white := color.RGBA{255, 255, 255, 255}
	rect := func(x0, y0, x1, y1 int) {
		for yy := y0; yy < y1; yy++ {
			for xx := x0; xx < x1; xx++ {
				if image.Pt(xx, yy).In(img.Rect) {
					img.SetRGBA(xx, yy, white)
				}
			}
		}
	}
	for _, c := range s {
		m := seg[c-'0']
		h := size / 2
		if m&0x01 != 0 {
			rect(x, y, x+w, y+t)
		}
		if m&0x02 != 0 {
			rect(x+w-t, y, x+w, y+h)
		}
		if m&0x04 != 0 {
			rect(x+w-t, y+h, x+w, y+size)
		}
		if m&0x08 != 0 {
			rect(x, y+size-t, x+w, y+size)
		}
		if m&0x10 != 0 {
			rect(x, y+h, x+t, y+size)
		}
		if m&0x20 != 0 {
			rect(x, y, x+t, y+h)
		}
		if m&0x40 != 0 {
			rect(x, y+h-t/2, x+w, y+h+t/2)
		}
		x += w + t*2
	}
}
