// Command gallery shows one component demo of the documentation per window.
//
// It exists only to render the component screenshots of the documentation
// site in docs/gift.dev and is not one of the examples. The demos live in
// internal/showcase, a file each, which the kitchen sink example shows as
// well; the part of a view function between the markers
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
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
)

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
		out := showcase.All()
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	d, ok := showcase.Lookup(*name)
	if !ok {
		return fmt.Errorf("unknown demo %q, see -list", *name)
	}
	if err := example.LoadFont(); err != nil {
		return err
	}

	app = gift.New(gift.Options{Root: func(ctx *gift.Context) gift.View {
		if d.FullWindow {
			return ui.Window(d.View(ctx))
		}
		return ui.Window(ui.VStack(d.View(ctx)).Padding(24)).Align(geom.Alignment{X: 0.5, Y: 0.5})
	}})

	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	if err := showcase.Samples(filepath.Join(base, "gift", "docs-gallery")); err != nil {
		return err
	}
	pipe := asset.NewPipeline(asset.Config{Deliver: app.Post, Space: backend.WorkingSpace(nil)})
	defer func() {
		pipe.Close()
		app.DrainPosts()
	}()
	ui.SetImagePipeline(pipe)

	// docs/capture sets GIFT_CAPTURE, so that the operator's cursor resting
	// over the window does not hover anything in the screenshots.
	return backend.Run(app, backend.Config{
		Title: "gift gallery: " + d.Name, Width: d.Width, Height: d.Height,
		NoMouse: os.Getenv("GIFT_CAPTURE") != "",
	})
}
