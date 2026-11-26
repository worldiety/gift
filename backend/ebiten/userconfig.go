package ebiten

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"

	"github.com/worldiety/gift"
)

// userConfig is the configuration a person keeps for every gift application
// they run, in .wdygift/config.json in their home directory:
//
//	{"scale": 2, "display": "srgb"}
//
// # Why it exists
//
// A desktop can report a wrong scale, and then nothing an application or
// gift does about it reaches the person in front of it. GNOME 46 on Wayland
// with the native scaling of XWayland clients is such a desktop: the Xft.dpi
// of a 2x display does not arrive, so Ebitengine and [newSystemScale] both
// read 1 and every gift window is half its size. The file is the way around
// that until the desktop gets it right, and it names no application, because
// the display is wrong for all of them.
//
// Android and iOS have no such file, and it is not read there: an Android
// application has no home directory, and the one Go falls back to is shared
// storage, where a stray file would change the density of a phone.
type userConfig struct {
	// Scale, when set, replaces the density of the monitor and
	// [Config.Density]. It is a factor from 1 to [gift.MaxDensity] and is
	// rounded as every density is; see [gift.App.SetDensity].
	Scale float64 `json:"scale"`

	// Display says how the colours of the display are handled: "auto", the
	// default, "srgb" to take the display for an sRGB one, or the path of
	// the display's ICC profile where the system does not say or says
	// wrong, on X11, Wayland and Windows. See [WorkingSpace] and displayPass.
	Display string `json:"display"`
}

// readUserConfig reads the [userConfig], the zero one where there is none
// or it cannot be read. It reports nothing: [userScale] reads the file first
// and logs what is wrong with it.
func readUserConfig() (userConfig, string) {
	path, err := userConfigPath()
	if err != nil {
		return userConfig{}, ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return userConfig{}, path
	}
	var c userConfig
	if json.Unmarshal(data, &c) != nil {
		return userConfig{}, path
	}
	return c, path
}

// userConfigPath is the path of the [userConfig] file, and
// [errors.ErrUnsupported] on Android and iOS, which have none.
func userConfigPath() (string, error) {
	if mobilePlatform {
		return "", errors.ErrUnsupported
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".wdygift", "config.json"), nil
}

// density is the density that replaces the monitor's: the scale of the
// [userConfig], else [Config.Density], else 0 for the monitor's own.
func (cfg Config) density() float64 {
	if s := userScale(cfg.Logger); s > 0 {
		return s
	}
	return cfg.Density
}

// userScale reads the scale of the [userConfig], or 0 where there is none. A
// file that cannot be read, or a scale out of range, is logged and ignored:
// a broken setting is no reason not to start.
func userScale(log *slog.Logger) float64 {
	path, err := userConfigPath()
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) && log != nil {
			log.Warn("gift/backend/ebiten: user configuration not read", slog.String("path", path), slog.Any("err", err))
		}
		return 0
	}
	var c userConfig
	if err := json.Unmarshal(data, &c); err != nil {
		if log != nil {
			log.Warn("gift/backend/ebiten: user configuration not read", slog.String("path", path), slog.Any("err", err))
		}
		return 0
	}
	switch {
	case c.Scale == 0:
		return 0
	case !(c.Scale >= 1 && c.Scale <= gift.MaxDensity):
		if log != nil {
			log.Warn("gift/backend/ebiten: scale of the user configuration out of range",
				slog.String("path", path), slog.Float64("scale", c.Scale), slog.Int("max", gift.MaxDensity))
		}
		return 0
	}
	if log != nil {
		log.Info("gift/backend/ebiten: scale of the user configuration",
			slog.String("path", path), slog.Float64("scale", c.Scale))
	}
	return c.Scale
}

// windowSize is the size to open a window of w by h logical pixels at, in
// Ebitengine's units, which are pixels divided by factor, the scale
// Ebitengine will report. A density that differs from it gives the window
// w·density by h·density pixels, as it would have on a desktop that reported
// the density itself. An unknown factor, 0, keeps the size.
func windowSize(w, h int, density, factor float64) (int, int) {
	if density <= 0 || factor <= 0 || density == factor {
		return w, h
	}
	return int(math.Round(float64(w) * density / factor)), int(math.Round(float64(h) * density / factor))
}

// startScale is the scale Ebitengine will report for the window it opens,
// where it is known before the window is open, or 0.
//
// On X11 it is: GLFW takes the Xft.dpi of the moment it starts, 96 where
// there is none, which is what [game.systemScale] reads now. Elsewhere
// Ebitengine's monitor is nil until the loop runs, so it is not.
func (g *game) startScale() float64 {
	if g.systemScale == nil {
		return 0
	}
	if s := g.systemScale(); s > 0 {
		return s
	}
	return 1
}
