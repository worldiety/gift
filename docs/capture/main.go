// Command capture renders the screenshots of the documentation site.
//
// Every screenshot is a framebuffer capture of a real window, taken through
// the automation interface of the giftauto build tag, never by hand and never
// offscreen: the same route docs/screenshots documents for the README.
//
//	go run ./docs/capture                 # every component demo and every example
//	go run ./docs/capture -only button,card  # only these demos and examples
//	go run ./docs/capture -examples=false # only the component demos
//
// Run it from the repository root on a machine with a display. For each demo
// of docs/gallery it starts the gallery with -demo, takes one screenshot in the
// light and one in the dark theme, crops them to what the demo drew and writes
// them as screenshot-light.png and screenshot-dark.png into the page bundle of
// the component below docs/gift.dev/content/docs/components.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	only     = flag.String("only", "", "comma separated names of the demos and examples to capture, empty for all")
	examples = flag.Bool("examples", true, "also capture the example programs in cmd")
	site     = flag.String("site", "docs/gift.dev", "the Hugo site the screenshots are written into")
	headful  = flag.Bool("keep", false, "keep each window open until enter is pressed, to debug a demo")
)

// demo mirrors the JSON of `gallery -list`.
type demo struct {
	Name       string `json:"name"`
	Group      string `json:"group"`
	Input      string `json:"input"`
	Settle     int    `json:"settle"`
	FullWindow bool   `json:"fullWindow"`
}

// exampleShot is a screenshot of one of the example programs in cmd.
type exampleShot struct {
	// Name is the directory below cmd and the page bundle below
	// content/docs/examples.
	Name string
	// Args are passed to the program.
	Args []string
	// Input, if not empty, is posted to /input before the screenshots.
	Input string
	// Settle is the number of frames to wait for pictures to arrive.
	Settle int
}

var exampleShots = []exampleShot{
	// Three taps on "+", so the shot shows a count, both enabled buttons and
	// the marker moved down the list; then the pointer leaves the buttons.
	{Name: "example-counter", Input: `{"steps":[{"op":"waitFrames","frames":2},{"op":"tap","x":231,"y":201},{"op":"waitFrames","frames":2},{"op":"tap","x":231,"y":201},{"op":"waitFrames","frames":2},{"op":"tap","x":231,"y":201},{"op":"waitFrames","frames":2},{"op":"pointerMove","x":5,"y":5},{"op":"waitFrames","frames":4}]}`},
	{Name: "example-kitchensink", Settle: 30},
	// The click selects a tile after the pictures have arrived, so both shots show the selection and a toolbar
	// whose counters have caught up with the decoded thumbnails.
	{Name: "example-gallery", Settle: 30, Input: `{"steps":[
		{"op":"waitFrames","frames":90},
		{"op":"click","x":470,"y":180},
		{"op":"waitFrames","frames":4}
	]}`},
	{Name: "example-effects", Args: []string{"-quality=full"}, Settle: 30},
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "capture:", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat("go.mod"); err != nil {
		return errors.New("run from the repository root")
	}
	tmp, err := os.MkdirTemp("", "gift-capture-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	gallery := filepath.Join(tmp, "gallery")
	if err := goBuild(gallery, "./docs/gallery"); err != nil {
		return err
	}
	out, err := exec.Command(gallery, "-list").Output()
	if err != nil {
		return fmt.Errorf("gallery -list: %w", err)
	}
	var demos []demo
	if err := json.Unmarshal(out, &demos); err != nil {
		return fmt.Errorf("gallery -list: %w", err)
	}

	var failed []string
	for _, d := range demos {
		if !selected(d.Name) {
			continue
		}
		dir := filepath.Join(*site, "content", "docs", "components", d.Group, d.Name)
		err := capture(gallery, []string{"-demo", d.Name}, d.Input, d.Settle, !d.FullWindow, dir)
		report(d.Name, err, &failed)
	}

	if *examples {
		for _, e := range exampleShots {
			if !selected(e.Name) {
				continue
			}
			bin := filepath.Join(tmp, e.Name)
			if err := goBuild(bin, "./cmd/"+e.Name); err != nil {
				return err
			}
			dir := filepath.Join(*site, "content", "docs", "examples", e.Name)
			err := capture(bin, e.Args, e.Input, e.Settle, false, dir)
			report(e.Name, err, &failed)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d failed: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

func selected(name string) bool {
	return *only == "" || slices.Contains(strings.Split(*only, ","), name)
}

func report(name string, err error, failed *[]string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", name, err)
		*failed = append(*failed, name)
		return
	}
	fmt.Println("ok  ", name)
}

func goBuild(out, pkg string) error {
	cmd := exec.Command("go", "build", "-tags", "giftauto", "-o", out, pkg)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build %s: %w", pkg, err)
	}
	return nil
}

// capture starts bin, takes a light and a dark screenshot and writes them
// into dir.
func capture(bin string, args []string, input string, settle int, crop bool, dir string) error {
	addr, err := freeAddr()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "GIFT_AUTO_ADDR="+addr)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	base := "http://" + addr
	if err := waitHealthy(base, 20*time.Second); err != nil {
		return err
	}
	if input != "" {
		if _, err := post(base+"/input", input); err != nil {
			return fmt.Errorf("input: %w", err)
		}
	}
	if settle <= 0 {
		settle = 8
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, mode := range []string{"light", "dark"} {
		if _, err := post(base+"/theme", `{"mode":"`+mode+`"}`); err != nil {
			return fmt.Errorf("theme %s: %w", mode, err)
		}
		img, density, err := screenshot(base, settle)
		if err != nil {
			return fmt.Errorf("screenshot %s: %w", mode, err)
		}
		if crop {
			img = cropToContent(img, int(24*density))
		}
		if err := writePNG(filepath.Join(dir, "screenshot-"+mode+".png"), img); err != nil {
			return err
		}
	}
	if *headful {
		fmt.Print("press enter to close the window ")
		_, _ = fmt.Scanln()
	}
	return nil
}

func freeAddr() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().String(), nil
}

func waitHealthy(base string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("no automation interface on %s after %s", base, timeout)
}

func post(url, body string) ([]byte, error) {
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", resp.Status, bytes.TrimSpace(b))
	}
	return b, err
}

func screenshot(base string, settle int) (image.Image, float64, error) {
	resp, err := http.Get(base + "/screenshot?settle=" + strconv.Itoa(settle))
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("%s: %s", resp.Status, bytes.TrimSpace(b))
	}
	density, err := strconv.ParseFloat(resp.Header.Get("X-Gift-Density"), 64)
	if err != nil || density <= 0 {
		density = 1
	}
	img, err := png.Decode(resp.Body)
	return img, density, err
}

// cropToContent cuts img down to the bounding box of every pixel that differs
// from the window background, which is taken from the top left corner, plus
// pad pixels on every side.
func cropToContent(img image.Image, pad int) image.Image {
	b := img.Bounds()
	bg := img.At(b.Min.X, b.Min.Y)
	br, bgc, bb, _ := bg.RGBA()
	box := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if near(r, br) && near(g, bgc) && near(bl, bb) {
				continue
			}
			box = box.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	if box.Empty() {
		return img
	}
	box = box.Inset(-pad).Intersect(b)
	out := image.NewRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	draw.Draw(out, out.Bounds(), img, box.Min, draw.Src)
	return out
}

// near tolerates the one or two steps of difference that dithering and
// blending leave in an otherwise flat background.
func near(a, b uint32) bool {
	d := int(a>>8) - int(b>>8)
	return d >= -2 && d <= 2
}

func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
