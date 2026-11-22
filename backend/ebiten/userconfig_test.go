package ebiten

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/cms"
)

// withHome makes dir the home directory of the test, with a user
// configuration of content unless that is empty.
func withHome(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if content == "" {
		return
	}
	if err := os.MkdirAll(filepath.Join(dir, ".wdygift"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".wdygift", "config.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTheUserScaleReplacesTheDensityOfTheApplication(t *testing.T) {
	for _, c := range []struct {
		name, file string
		want       float64
		warns      bool
	}{
		{"no file", "", 3, false},
		{"a scale", `{"scale": 2}`, 2, false},
		{"a fraction", `{"scale": 1.75}`, 1.75, false},
		{"one on a display that says two", `{"scale": 1}`, 1, false},
		{"no scale", `{}`, 3, false},
		{"other settings", `{"theme": "dark"}`, 3, false},
		{"broken", `{"scale": 2`, 3, true},
		{"a string", `{"scale": "2"}`, 3, true},
		{"below one", `{"scale": 0.5}`, 3, true},
		{"above the most", `{"scale": 8}`, 3, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			withHome(t, c.file)
			var buf bytes.Buffer
			cfg := Config{Density: 3, Logger: slog.New(slog.NewTextHandler(&buf, nil))}
			if got := cfg.density(); got != c.want {
				t.Errorf("density %v, want %v", got, c.want)
			}
			if warned := strings.Contains(buf.String(), "level=WARN"); warned != c.warns {
				t.Errorf("warned %v, want %v; log:\n%s", warned, c.warns, buf.String())
			}
		})
	}
	// Without a logger a broken file is as quiet as a missing one.
	withHome(t, `{"scale": -1}`)
	if got := (Config{}).density(); got != 0 {
		t.Errorf("density %v without a logger, want 0", got)
	}
}

func TestAWindowOpensAtTheDensity(t *testing.T) {
	for _, c := range []struct {
		name            string
		density, factor float64
		wantW, wantH    int
	}{
		{"no density of its own", 0, 1, 1280, 720},
		{"an unknown factor", 2, 0, 1280, 720},
		{"as the desktop says", 2, 2, 1280, 720},
		{"2 where the desktop says 1", 2, 1, 2560, 1440},
		{"1 where the desktop says 2", 1, 2, 640, 360},
		{"1.5 where the desktop says 1", 1.5, 1, 1920, 1080},
	} {
		t.Run(c.name, func(t *testing.T) {
			if w, h := windowSize(1280, 720, c.density, c.factor); w != c.wantW || h != c.wantH {
				t.Errorf("windowSize = %dx%d, want %dx%d", w, h, c.wantW, c.wantH)
			}
		})
	}
}

func TestTheStartScaleIsGLFWs(t *testing.T) {
	if s := (&game{}).startScale(); s != 0 {
		t.Errorf("without a reading %v, want 0", s)
	}
	// No Xft.dpi is 96 to GLFW.
	if s := (&game{systemScale: func() float64 { return 0 }}).startScale(); s != 1 {
		t.Errorf("without Xft.dpi %v, want 1", s)
	}
	if s := (&game{systemScale: func() float64 { return 2 }}).startScale(); s != 2 {
		t.Errorf("at Xft.dpi 192 %v, want 2", s)
	}
}

// TestLayoutFKeepsTheDensityOfItsOwn covers a display whose desktop reports 1
// for a 2x panel and a density of 2 from the user's configuration: the window
// keeps its pixels and lays out half as many logical ones in them, also once
// the desktop corrects itself.
func TestLayoutFKeepsTheDensityOfItsOwn(t *testing.T) {
	for _, c := range []struct {
		name        string
		raw, system float64
		outW, outH  float64
	}{
		{"the desktop says 1", 1, 1, 1600, 1200},
		{"no Xft.dpi", 1, 0, 1600, 1200},
		{"the desktop says 2 after the start", 1, 2, 1600, 1200},
	} {
		t.Run(c.name, func(t *testing.T) {
			app := gift.New(gift.Options{
				Root: func(*gift.Context) gift.View { return recorderView{app: &keyboardApp{}} },
			})
			g := &game{app: app, density: 2,
				scale:       func() float64 { return c.raw },
				systemScale: func() float64 { return c.system },
			}
			if w, h := g.LayoutF(c.outW, c.outH); w != 1600 || h != 1200 {
				t.Errorf("LayoutF(%v, %v) = (%v, %v), want the 1600x1200 pixels of the window", c.outW, c.outH, w, h)
			}
			if g.w != 800 || g.h != 600 {
				t.Errorf("viewport %dx%d, want 800x600", g.w, g.h)
			}
			if got := app.Density(); got != 2 {
				t.Errorf("density %v, want 2", got)
			}
		})
	}
}

// TestTheScreenIsTaggedWithTheWorkingSpace pins the colour space of the
// screen: unset, Ebitengine tags it Display P3 on macOS whatever the
// renderer paints, and sRGB numbers came out too saturated.
func TestTheScreenIsTaggedWithTheWorkingSpace(t *testing.T) {
	if cs := runOptions(Config{}, cms.SRGB()).ColorSpace; cs != eb.ColorSpaceSRGB {
		t.Errorf("colour space %v for sRGB", cs)
	}
	if cs := runOptions(Config{}, cms.DisplayP3()).ColorSpace; cs != eb.ColorSpaceDisplayP3 {
		t.Errorf("colour space %v for Display P3", cs)
	}
}

func TestTheWorkingSpaceFollowsTheDisplaySetting(t *testing.T) {
	wide := runtime.GOOS == "darwin" || runtime.GOOS == "ios"
	for _, c := range []struct {
		name, file string
		p3, warns  bool
	}{
		{"no file", "", wide, false},
		{"auto", `{"display": "auto"}`, wide, false},
		{"sRGB", `{"display": "sRGB"}`, false, false},
		{"nonsense", `{"display": "vivid"}`, wide, wide},
		{"broken file", `{"display": "srgb"`, wide, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			withHome(t, c.file)
			var buf bytes.Buffer
			got := WorkingSpace(slog.New(slog.NewTextHandler(&buf, nil)))
			if (got == cms.DisplayP3()) != c.p3 {
				t.Errorf("working space %s", got.Description())
			}
			if got != cms.DisplayP3() && got != cms.SRGB() {
				t.Errorf("working space %s is neither", got.Description())
			}
			if warned := strings.Contains(buf.String(), "level=WARN"); warned != c.warns {
				t.Errorf("warned %v, want %v; log:\n%s", warned, c.warns, buf.String())
			}
		})
	}
}
