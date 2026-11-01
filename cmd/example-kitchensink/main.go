// Command example-kitchensink shows every component of gift: a list grouped
// like the documentation, and beside it, or after it on a narrow screen, the
// component with the demo of its documentation page, which is live and can be
// played with.
//
//	go run ./cmd/example-kitchensink
//	go run ./cmd/example-kitchensink -show color-picker
//	go run ./cmd/example-kitchensink -width 800 -height 480 -keyboard   # a five inch kiosk
//
// The demos are those of internal/showcase, the very code the screenshots of
// the documentation are drawn from, so a component that is documented is in
// here, and in here as the page shows it. A section of its own after them
// shows what is no component: the notifications of the operating system,
// see notifications.go. Above the list sit a search field,
// a switch for the on-screen keyboard of a kiosk and one for the dark
// appearance, which switches the whole window without a colour being passed
// down anywhere.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
)

func main() {
	show := flag.String("show", "button", "the component shown first, by the name of its page, for example color-picker")
	width := flag.Int("width", 1180, "window width")
	height := flag.Int("height", 800, "window height")
	keyboard := flag.Bool("keyboard", false, "turn the on-screen keyboard on, as a kiosk without a hardware keyboard does")
	flag.Parse()
	if err := run(*show, *width, *height, *keyboard); err != nil {
		fmt.Fprintln(os.Stderr, "example-kitchensink:", err)
		os.Exit(1)
	}
}

func run(show string, width, height int, keyboard bool) error {
	if _, ok := lookup(show); !ok {
		return fmt.Errorf("no component %q", show)
	}
	initialDemo = show
	if err := example.LoadFont(); err != nil {
		return err
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	if err := showcase.Samples(filepath.Join(base, "gift", "kitchensink")); err != nil {
		return err
	}
	ui.SetOnScreenKeyboard(nil, keyboard)

	App = gift.New(gift.Options{Root: Screen})
	pipe := asset.NewPipeline(asset.Config{Deliver: App.Post})
	defer func() {
		pipe.Close()
		App.DrainPosts()
	}()
	ui.SetImagePipeline(pipe)

	// docs/capture sets GIFT_CAPTURE, so that the operator's cursor resting
	// over the window does not hover anything in the screenshot.
	return backend.Run(App, backend.Config{
		Title: "gift kitchen sink", Width: width, Height: height,
		NoMouse: os.Getenv("GIFT_CAPTURE") != "",
	})
}
