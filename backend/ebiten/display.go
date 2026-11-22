package ebiten

import (
	"log/slog"
	"runtime"
	"strings"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/cms"
)

// WorkingSpace returns the colour space [Run] paints in on this machine:
// Display P3 on macOS and iOS, sRGB everywhere else.
//
// # Why P3 on a Mac
//
// A Mac's compositor converts every window from the space its layer is
// tagged with into the display's, so a frame painted in P3 shows the colours
// of a photograph made in P3 on a display that has them, which every Mac
// since 2016 has, and on one that does not the compositor maps them as it
// maps every other application's. gift's own colours, which are sRGB, look
// as they did: the renderer converts them; see [Renderer.SetWorkingSpace].
// Elsewhere the frame is sRGB, which is what Windows and X11 show a window
// as.
//
// A person who wants the frame in sRGB on a Mac anyway, to compare numbers
// with another machine say, writes "display": "srgb" into
// ~/.wdygift/config.json.
//
// It is also the space to make thumbnails in, asset.Config.Space. log
// receives a note when the configuration chose; it may be nil.
func WorkingSpace(log *slog.Logger) *cms.Profile {
	c, path := readUserConfig()
	if runtime.GOOS != "darwin" && runtime.GOOS != "ios" {
		return cms.SRGB()
	}
	switch d := strings.TrimSpace(c.Display); {
	case strings.EqualFold(d, "srgb"):
		if log != nil {
			log.Info("gift/backend/ebiten: the display is taken for sRGB, as the user configuration says",
				slog.String("path", path))
		}
		return cms.SRGB()
	case d != "" && !strings.EqualFold(d, "auto") && log != nil:
		log.Warn("gift/backend/ebiten: display setting of the user configuration not understood, auto used",
			slog.String("path", path), slog.String("display", d))
	}
	return cms.DisplayP3()
}

// screenColorSpace is how Ebitengine tags the screen for a working space.
// Ebitengine heeds it only on macOS and iOS, where the compositor converts
// from it; elsewhere it is sRGB whatever this says.
func screenColorSpace(space *cms.Profile) eb.ColorSpace {
	if space == cms.DisplayP3() {
		return eb.ColorSpaceDisplayP3
	}
	return eb.ColorSpaceSRGB
}
