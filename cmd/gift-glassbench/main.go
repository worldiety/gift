// Command gift-glassbench measures what glass costs on the target hardware.
//
// It draws a scene shaped like a photo booth home screen – a photo
// wallpaper, ten floating panels of the sizes a real design uses, text and a
// few thumbnails on them – and redraws it every frame, so that the frame
// intervals measure the steady cost of the scene. Build it with the
// giftmetrics tag and run it with GIFT_METRICS=1 to get the report:
//
//	go build -tags giftmetrics ./cmd/gift-glassbench
//	GIFT_METRICS=1 ./gift-glassbench -scene full -duration 20s -fullscreen
//
// The scenes differ only in how the panels are filled:
//
//	blank    a single full screen colour: the floor of a frame
//	none     the wallpaper only, panels are not drawn at all
//	frosted  a translucent rounded rectangle, no backdrop read
//	reduced  ui.Glass at the Reduced level (backdrop copy, no blur)
//	full     ui.Glass at the Full level (backdrop copy and dual-Kawase blur)
//	cached   each panel draws its part of a wallpaper that was blurred once
//	         on the CPU, as a rounded picture: what a static backdrop cache
//	         would cost per frame
//
// -move slides the panel layer back and forth by changing its padding, which
// rebuilds and lays out the scene every frame. -slide instead switches
// between two copies of the scene with the page transition of a navigation
// container, back and forth without a pause, which is what the layer cache
// is for; -nolayers turns the cache off for the comparison. -novsync draws as fast as the machine
// can, so that the frame interval is the cost of a frame rather than the
// refresh rate of the display.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// panel is one floating surface, in fractions of the screen.
type panel struct {
	x, y, w, h float32
	radius     float32
	title      string
	thumbs     int
}

// panels is the layout of the photo booth home screen mockup: four status
// capsules, a greeting card, three widgets and a dock. About 38 % of the
// screen is covered by glass.
var panels = []panel{
	{0.019, 0.022, 0.13, 0.05, 0.025, "20:25", 0},
	{0.60, 0.022, 0.11, 0.05, 0.025, "45 Blatt", 0},
	{0.72, 0.022, 0.10, 0.05, 0.025, "Upload", 0},
	{0.83, 0.022, 0.15, 0.05, 0.025, "Heimnetz", 0},
	{0.70, 0.117, 0.267, 0.217, 0.042, "Kiosk-Modus", 0},
	{0.034, 0.364, 0.438, 0.414, 0.042, "Eingang", 4},
	{0.488, 0.364, 0.195, 0.414, 0.042, "Drucker", 0},
	{0.698, 0.364, 0.267, 0.414, 0.042, "Vom Handy senden", 1},
	{0.27, 0.86, 0.46, 0.12, 0.047, "", 6},
	{0.034, 0.12, 0.30, 0.18, 0.042, "20:25", 0},
}

func main() {
	var (
		scene      = flag.String("scene", "full", "none, frosted, reduced, full or cached")
		width      = flag.Int("width", 1920, "screen width; the wallpaper is generated at this size")
		height     = flag.Int("height", 1080, "screen height")
		duration   = flag.Duration("duration", 20*time.Second, "exit after this long")
		fullscreen = flag.Bool("fullscreen", false, "run fullscreen")
		move       = flag.Bool("move", false, "slide the panels back and forth")
		novsync    = flag.Bool("novsync", false, "draw as fast as possible, so that the interval measures cost and not the display")
		direct     = flag.Bool("direct", false, "draw straight into the final screen (backend.Config.DirectToScreen)")
		slide      = flag.Bool("slide", false, "switch between two copies of the scene with a page transition, without a pause")
		nolayers   = flag.Bool("nolayers", false, "turn the layer cache off (Renderer.SetLayerCache)")
	)
	flag.Parse()

	dir, err := os.MkdirTemp("", "gift-glassbench")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(dir)

	W, H := *width, *height
	wall := wallpaper(W, H)
	wallPath := save(dir, "wall.png", wall)
	thumbPath := save(dir, "thumb.png", thumbnail())

	var cropPaths []string
	if *scene == "cached" {
		blurred := blurCPU(wall)
		for i, p := range panels {
			r := image.Rect(int(p.x*float32(W)), int(p.y*float32(H)), int((p.x+p.w)*float32(W)), int((p.y+p.h)*float32(H)))
			cropPaths = append(cropPaths, save(dir, fmt.Sprintf("crop%d.png", i), crop(blurred, r)))
		}
	}

	ui.SetDefaultFont(ui.MustFont(ui.FontQuery{Family: inter.Family}))

	var shift float64
	front := 0
	var page func() gift.View
	root := func(ctx *gift.Context) gift.View {
		tick := ctx.State("tick", 0)
		if *move {
			ctx.Read(tick)
		}
		if *slide {
			return ui.ZStack(
				slideView{key: "a", hidden: front != 0, parked: geom.Pt(-1, 0), child: page()},
				slideView{key: "b", hidden: front != 1, parked: geom.Pt(1, 0), child: page()},
			)
		}
		return page()
	}
	page = func() gift.View {
		if *scene == "blank" {
			return ui.Box().Frame(float32(W), float32(H)).Background(ui.RGB(40, 90, 160))
		}

		layers := []gift.View{ui.Image(asset.File(wallPath)).Frame(float32(W), float32(H))}
		for i, p := range panels {
			if *scene == "none" {
				continue
			}

			x := p.x*float32(W) + float32(shift)
			y := p.y * float32(H)
			w, h := p.w*float32(W), p.h*float32(H)
			r := p.radius * float32(H)

			var content []gift.View
			if p.title != "" {
				content = append(content, ui.Text(p.title).FontSize(float32(H)*0.02).Foreground(ui.RGB(20, 20, 26)))
			}

			if p.thumbs > 0 {
				row := []gift.View{}
				for range p.thumbs {
					t := h * 0.4
					row = append(row, ui.Image(asset.File(thumbPath)).Frame(t, t).CornerRadius(t/8).Clip(true))
				}
				content = append(content, ui.HStack(row...).Gap(10))
			}

			body := ui.VStack(content...).Gap(8).Padding(h*0.12).Frame(w, h)

			var surface gift.View
			switch *scene {
			case "frosted":
				surface = body.Background(ui.RGBA(255, 255, 255, 90)).CornerRadius(r).
					Border(ui.Border{Width: 1, Color: ui.RGBA(255, 255, 255, 150)})
			case "reduced":
				surface = body.Background(ui.Glass().Quality(ui.Reduced)).CornerRadius(r)
			case "full":
				surface = body.Background(ui.Glass().Quality(ui.Full)).CornerRadius(r)
			case "cached":
				surface = ui.ZStack(
					ui.Image(asset.File(cropPaths[i])).Frame(w, h).CornerRadius(r).Clip(true),
					body.Background(ui.RGBA(255, 255, 255, 50)).CornerRadius(r).
						Border(ui.Border{Width: 1, Color: ui.RGBA(255, 255, 255, 150)}),
				)
			}

			layers = append(layers, ui.VStack(surface).PaddingInsets(geom.Insets{Left: x, Top: y}))
		}

		return ui.ZStack(layers...).Align(geom.TopLeading)
	}

	app := gift.New(gift.Options{Root: root})

	pipe := asset.NewPipeline(asset.Config{Deliver: app.Post})
	defer func() {
		pipe.Close()
		app.DrainPosts()
	}()
	ui.SetImagePipeline(pipe)

	if *fullscreen {
		eb.SetFullscreen(true)
	}

	if *novsync {
		eb.SetVsyncEnabled(false)
	}

	cfg := backend.Config{Title: "gift glassbench", Width: W, Height: H, DirectToScreen: *direct}
	if *nolayers {
		cfg.OnRenderer = func(r *backend.Renderer) { r.SetLayerCache(false) }
	}
	switch *scene {
	case "full":
		cfg.GlassQuality = render.Full
	case "reduced":
		cfg.GlassQuality = render.Reduced
	}

	start := time.Now()
	switched := start
	cfg.OnUpdate = func() error {
		if time.Since(start) > *duration {
			return backend.Terminate
		}

		if *slide && time.Since(switched) >= slideDuration {
			switched = time.Now()
			front = 1 - front
			app.Invalidate()
		}

		if *move {
			// Only rightwards: a panel at the left edge must not get a
			// negative offset, which gift rejects.
			shift = 60 + 60*math.Sin(time.Since(start).Seconds()*2)
			app.Invalidate()
		}

		return nil
	}

	fmt.Printf("gift-glassbench scene=%s size=%dx%d move=%v slide=%v layers=%v duration=%v\n",
		*scene, W, H, *move, *slide, !*nolayers, *duration)

	if err := backend.Run(app, cfg); err != nil {
		fail(err)
	}
}

// slideDuration is the length of one page movement in -slide mode, and also
// the time between two of them, so that the pages never rest.
const slideDuration = 400 * time.Millisecond

var slideType = gift.RegisterType("glassbench.slide")

// slideView is a full size page whose change of visibility is a movement,
// like the layer of a navigation container.
type slideView struct {
	key    string
	hidden bool
	parked geom.Point
	child  gift.View
}

func (s slideView) ViewType() gift.TypeID { return slideType }

func (s slideView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{
		Key:        s.key,
		Layouter:   fillLayout{},
		Children:   []gift.View{s.child},
		Hidden:     s.hidden,
		Transition: gift.TransitionSpec{Parked: s.parked, Duration: slideDuration},
	}
}

type fillLayout struct{}

func (fillLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	ctx.Measure(0, geom.Tight(c.Max))
	ctx.Place(0, geom.Point{})
	return c.Max
}

// wallpaper is a colourful picture with sharp detail, so that a blur has
// something to do and a missing blur is visible.
func wallpaper(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)
			r := 0.5 + 0.5*math.Sin(6*fx+1)
			g := 0.5 + 0.5*math.Sin(5*fy+2*fx)
			b := 0.5 + 0.5*math.Cos(4*fx*fy+3)
			if (x/40+y/40)%2 == 0 {
				r, g, b = r*0.85, g*0.85, b*0.85
			}
			img.SetRGBA(x, y, color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255})
		}
	}
	return img
}

func thumbnail() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := range 256 {
		for x := range 256 {
			img.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 180, 255})
		}
	}
	return img
}

// blurCPU blurs by shrinking to an eighth, box blurring three times and
// growing back bilinearly: the one-time cost of a backdrop cache.
func blurCPU(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx()/8, b.Dy()/8
	small := make([][4]float64, sw*sh)
	for y := range sh {
		for x := range sw {
			var acc [4]float64
			for dy := range 8 {
				for dx := range 8 {
					c := src.RGBAAt(x*8+dx, y*8+dy)
					acc[0] += float64(c.R)
					acc[1] += float64(c.G)
					acc[2] += float64(c.B)
				}
			}
			small[y*sw+x] = [4]float64{acc[0] / 64, acc[1] / 64, acc[2] / 64, 255}
		}
	}

	for range 3 {
		next := make([][4]float64, len(small))
		for y := range sh {
			for x := range sw {
				var acc [4]float64
				n := 0.0
				for dy := -3; dy <= 3; dy++ {
					for dx := -3; dx <= 3; dx++ {
						xx, yy := x+dx, y+dy
						if xx < 0 || yy < 0 || xx >= sw || yy >= sh {
							continue
						}
						c := small[yy*sw+xx]
						acc[0] += c[0]
						acc[1] += c[1]
						acc[2] += c[2]
						n++
					}
				}
				next[y*sw+x] = [4]float64{acc[0] / n, acc[1] / n, acc[2] / n, 255}
			}
		}
		small = next
	}

	out := image.NewRGBA(b)
	for y := range b.Dy() {
		for x := range b.Dx() {
			fx := math.Min(float64(x)/8, float64(sw-1))
			fy := math.Min(float64(y)/8, float64(sh-1))
			x0, y0 := int(fx), int(fy)
			x1, y1 := min(x0+1, sw-1), min(y0+1, sh-1)
			tx, ty := fx-float64(x0), fy-float64(y0)
			var c [3]float64
			for k := range 3 {
				top := small[y0*sw+x0][k]*(1-tx) + small[y0*sw+x1][k]*tx
				bot := small[y1*sw+x0][k]*(1-tx) + small[y1*sw+x1][k]*tx
				c[k] = top*(1-ty) + bot*ty
			}
			out.SetRGBA(x, y, color.RGBA{uint8(c[0]), uint8(c[1]), uint8(c[2]), 255})
		}
	}
	return out
}

func crop(src *image.RGBA, r image.Rectangle) *image.RGBA {
	r = r.Intersect(src.Bounds())
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := range r.Dy() {
		copy(out.Pix[y*out.Stride:y*out.Stride+r.Dx()*4], src.Pix[(r.Min.Y+y)*src.Stride+r.Min.X*4:])
	}
	return out
}

func save(dir, name string, img image.Image) string {
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fail(err)
	}
	return p
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gift-glassbench:", err)
	os.Exit(1)
}
