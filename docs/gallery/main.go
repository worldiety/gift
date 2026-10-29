// Command gallery shows one component demo of the documentation per window.
//
// It exists only to render the component screenshots of the documentation
// site in docs/gift.dev and is not one of the examples. Every demo lives in a
// file of its own and registers itself with [register]; the part of its view
// function between the markers
//
//	// snippet:begin
//	// snippet:end
//
// is what the component page shows next to the screenshot, so the code on the
// page is the code that drew the picture.
//
//	go run ./docs/gallery -list                  # all demos as JSON
//	go run ./docs/gallery -demo button           # look at one demo
//	go run -tags giftauto ./docs/gallery -demo button
//
// docs/capture builds this program with the giftauto tag and takes the
// screenshots through the automation interface; see its README.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/ui"
)

// Demo is one component demo.
type Demo struct {
	// Name identifies the demo on the command line and names its page bundle.
	Name string `json:"name"`
	// Group is the section of the component pages the demo belongs to, such
	// as "basic" or "layout".
	Group string `json:"group"`
	// Width and Height are the window size in logical pixels. The screenshot
	// is cropped to what the demo draws, so they only have to be large enough.
	Width  int `json:"width"`
	Height int `json:"height"`
	// Input, if not empty, is the body of a POST to the /input endpoint of
	// the automation interface, sent before the screenshots are taken.
	Input string `json:"input,omitempty"`
	// Settle is the number of frames the screenshot waits for, so that
	// pictures and icons have arrived. Zero selects 8.
	Settle int `json:"settle,omitempty"`
	// FullWindow keeps the whole window instead of cropping to the content,
	// for demos that fill the window, such as a sheet or a tab bar.
	FullWindow bool `json:"fullWindow,omitempty"`

	view func(ctx *gift.Context) gift.View
}

var demos = map[string]Demo{}

// register adds a demo. It is called from the init function of each demo file.
func register(d Demo, view func(ctx *gift.Context) gift.View) {
	if _, dup := demos[d.Name]; dup {
		panic("gallery: demo registered twice: " + d.Name)
	}
	if d.Width == 0 {
		d.Width = 640
	}
	if d.Height == 0 {
		d.Height = 360
	}
	d.view = view
	demos[d.Name] = d
}

// app is the handle the demos which switch the theme or the keyboard need.
var app *gift.App

var (
	list = flag.Bool("list", false, "print all demos as JSON and exit")
	name = flag.String("demo", "", "the demo to show")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gallery:", err)
		os.Exit(1)
	}
}

func run() error {
	if *list {
		out := make([]Demo, 0, len(demos))
		for _, d := range demos {
			out = append(out, d)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	d, ok := demos[*name]
	if !ok {
		return fmt.Errorf("unknown demo %q, see -list", *name)
	}
	if err := example.LoadFont(); err != nil {
		return err
	}

	app = gift.New(gift.Options{Root: func(ctx *gift.Context) gift.View {
		if d.FullWindow {
			return ui.Window(d.view(ctx))
		}
		return ui.Window(ui.VStack(d.view(ctx)).Padding(24)).Align(geom.Alignment{X: 0.5, Y: 0.5})
	}})

	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	if err := samples(filepath.Join(base, "gift", "docs-gallery")); err != nil {
		return err
	}
	pipe := asset.NewPipeline(asset.Config{Deliver: app.Post})
	defer func() {
		pipe.Close()
		app.DrainPosts()
	}()
	ui.SetImagePipeline(pipe)

	return backend.Run(app, backend.Config{
		Title: "gift gallery: " + d.Name, Width: d.Width, Height: d.Height,
	})
}
