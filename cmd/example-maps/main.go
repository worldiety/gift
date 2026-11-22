// Command example-maps shows a map of ui/maps: a self hosted PMTiles archive
// of OpenStreetMap, or the debug tiles, in a window of its own.
//
//	go run ./cmd/example-maps                       # the Oldenburg extract, see PLAN.md section 37
//	go run ./cmd/example-maps -pmtiles city.pmtiles
//	go run ./cmd/example-maps -url https://example.org/city.pmtiles
//	go run ./cmd/example-maps -debug -heading 30
//	go run ./cmd/example-maps -xyz 'https://tile.openstreetmap.org/{z}/{x}/{y}.png'  # for trying out
//	go run ./cmd/example-maps -fullscreen            # a kiosk
//	go run ./cmd/example-maps -photos 2000           # a camera roll on the map
//	go run ./cmd/example-maps -weather dwd           # the weather of the Deutscher Wetterdienst
//	go run ./cmd/example-maps -weather dwd+gfs       # and NOAA's GFS beyond Germany
//
// Pan with a finger or the mouse, zoom with two fingers, the wheel or plus
// and minus, turn with two fingers; a double tap zooms in. -dark draws the
// dark style.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"

	"github.com/worldiety/gift/asset"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/maps"
	"github.com/worldiety/gift/weather"
	"github.com/worldiety/gift/weather/dwd"
	"github.com/worldiety/gift/weather/gfs"
)

func main() {
	def := ""
	if dir, err := os.UserCacheDir(); err == nil {
		def = filepath.Join(dir, "gift", "maps", "oldenburg.pmtiles")
	}
	path := flag.String("pmtiles", def, "a PMTiles archive")
	url := flag.String("url", "", "a PMTiles archive on an HTTP server, read with range requests")
	xyz := flag.String("xyz", "", "a raster tile service by its URL template, https://tile.openstreetmap.org/{z}/{x}/{y}.png for trying out")
	attribution := flag.String("attribution", "© OpenStreetMap", "the attribution of the -xyz service")
	debug := flag.Bool("debug", false, "draw the debug tiles instead of a map")
	dark := flag.Bool("dark", false, "the dark style")
	width := flag.Int("width", 1180, "window width")
	height := flag.Int("height", 800, "window height")
	lat := flag.Float64("lat", 53.1435, "latitude of the centre")
	lon := flag.Float64("lon", 8.2146, "longitude of the centre")
	zoom := flag.Float64("zoom", 13, "zoom")
	heading := flag.Float64("heading", 0, "heading in degrees, clockwise from north")
	noLabels := flag.Bool("nolabels", false, "draw the map without its labels, for measuring")
	fullscreen := flag.Bool("fullscreen", false, "start full screen, as a kiosk does")
	photos := flag.Int("photos", 0, "show this many sample photographs around Oldenburg, clustered")
	weatherFrom := flag.String("weather", "", "the weather over the map: dwd, gfs, or dwd+gfs for the one inside Germany and the other beyond")
	reduced := flag.Bool("reduced", false, "the reduced quality of the weather, for a Raspberry Pi")
	workers := flag.Int("workers", 2, "decode and rasterising goroutines: two leave the frame loop a core of its own on a Raspberry Pi 5")
	upload := flag.Int("upload", 0, "texture upload budget per frame in KiB; 0 is two tiles a frame, which keep a pan at 60 Hz on a Raspberry Pi 5")
	flag.Parse()
	if *fullscreen {
		eb.SetFullscreen(true)
	}
	cam := maps.Camera{Center: geo.LatLon(*lat, *lon), Zoom: *zoom, Heading: *heading}
	wo := weatherOptions{from: *weatherFrom, reduced: *reduced}
	if err := run(*path, *url, *xyz, *attribution, *debug, *dark, *noLabels, *width, *height, *upload, *photos, *workers, wo, cam); err != nil {
		fmt.Fprintln(os.Stderr, "example-maps:", err)
		os.Exit(1)
	}
}

// weatherOptions is what the flags say about the weather.
type weatherOptions struct {
	from    string
	reduced bool
}

func run(path, url, xyz, attribution string, debug, dark, noLabels bool, width, height, upload, photos, workers int, wo weatherOptions, cam maps.Camera) error {
	if err := example.LoadFont(); err != nil {
		return err
	}
	var tiles maps.Provider
	var err error
	switch {
	case debug:
		tiles = maps.DebugTiles()
	case url != "":
		tiles, err = maps.PMTilesURL(url)
	case xyz != "":
		// The OSMF's servers want an application named in the User-Agent,
		// and no prefetching.
		tiles, err = maps.XYZ(xyz, maps.XYZOptions{
			Attribution: attribution,
			Header:      http.Header{"User-Agent": {"gift-example-maps/1 (+https://github.com/worldiety/gift)"}},
		})
	default:
		tiles, err = maps.PMTiles(path)
	}
	if err != nil {
		return err
	}
	light, darkStyle := maps.ProtomapsLight(), maps.ProtomapsDark()
	if noLabels {
		light.Labels, darkStyle.Labels = nil, nil
	}
	m, err := maps.New(maps.Config{
		Tiles: tiles, Style: light, DarkStyle: darkStyle,
		Camera: cam,
	})
	if err != nil {
		return err
	}
	var points *maps.Points
	if photos > 0 {
		dir, err := os.UserCacheDir()
		if err != nil {
			dir = os.TempDir()
		}
		srcs, err := showcase.MapPhotos(filepath.Join(dir, "gift", "map-photos"), photos)
		if err != nil {
			return err
		}
		pts, err := maps.LocatePhotos(context.Background(), srcs)
		if err != nil {
			return err
		}
		points = maps.NewPoints(pts, maps.Clustering{})
	}
	var sky maps.Overlay
	if wo.from != "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			dir = os.TempDir()
		}
		const agent = "gift-example-maps/1 (+https://github.com/worldiety/gift)"
		icon := dwd.ICOND2(dwd.Options{CacheDir: filepath.Join(dir, "gift", "weather", "dwd"), UserAgent: agent})
		global := gfs.Global(gfs.Options{CacheDir: filepath.Join(dir, "gift", "weather", "gfs"), UserAgent: agent})
		var p weather.Provider
		switch wo.from {
		case "dwd":
			p = icon
		case "gfs":
			p = global
		case "dwd+gfs":
			// The Deutscher Wetterdienst over Germany, the GFS beyond.
			p = weather.Combine(icon, global)
		default:
			return fmt.Errorf("no weather provider %q: dwd, gfs or dwd+gfs", wo.from)
		}
		s := weather.NewSky(p, weather.SkyConfig{Region: geo.RectOf(geo.LatLon(cam.Center.Lat-0.5, cam.Center.Lon-0.8), geo.LatLon(cam.Center.Lat+0.5, cam.Center.Lon+0.8))})
		s.Start()
		defer s.Stop()
		q := maps.WeatherFull
		if wo.reduced {
			q = maps.WeatherReduced
		}
		sky = maps.Weather(s, maps.WeatherOptions{Quality: q})
	}
	app := gift.New(gift.Options{Root: func(*gift.Context) gift.View {
		v := maps.View(m).Flex(1).Key("map")
		if sky != nil {
			v = v.Overlay(sky)
		}
		if points != nil {
			v = v.Points(points, maps.PhotoMarkers())
		}
		return ui.Window(v)
	}})
	if dark {
		ui.SetTheme(app, ui.DarkTheme())
	}
	cache := ""
	if dir, err := os.UserCacheDir(); err == nil {
		cache = filepath.Join(dir, "gift", "maps-cache")
	}
	// The disk cache keeps the tiles of a service by its caching headers,
	// which its usage policy asks for, and drawn vector tiles between runs.
	// The decoded tiles of a zoom wait in the CPU cache for their upload:
	// at density 2 a tile is 1 MiB, and the default of 64 MiB is less than a
	// screenful and its stand-ins and neighbours, so they are given four
	// times the room the 256 KiB tiles of a Raspberry Pi's display have.
	pixels := int64(0)
	if runtime.GOOS != "linux" {
		pixels = 4 * asset.DefaultPixelBudget
	}
	pipe := asset.NewPipeline(asset.Config{Deliver: app.Post, Workers: workers, PixelBudget: pixels,
		Space: backend.WorkingSpace(nil), Disk: asset.DiskCacheConfig{Dir: cache}})
	defer func() {
		pipe.Close()
		app.DrainPosts()
	}()
	ui.SetImagePipeline(pipe)
	cfg := backend.Config{
		Title: "gift maps", Width: width, Height: height,
		// The settings of a kiosk on a Raspberry Pi: draw only what changed,
		// straight into the window, and tick slowly while nothing does.
		DrawOnDemand: true, DirectToScreen: true, IdleTPS: 10,
		AssetStats: ui.ImagePipelineStats,
	}
	// Two tiles a frame: 512 KiB where the display of a Raspberry Pi has
	// density 1 and a tile is 256 pixels square, and the backend's 2 MiB on
	// the displays of density 2 elsewhere, where a tile is 512. Less there,
	// and the tiles of a zoom arrive over a second.
	if upload == 0 && runtime.GOOS == "linux" {
		upload = 512
	}
	if upload > 0 {
		cfg.Textures.UploadBytesPerFrame = int64(upload) << 10
	}
	return backend.Run(app, cfg)
}
