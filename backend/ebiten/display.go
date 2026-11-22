package ebiten

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
// Elsewhere the frame is sRGB, and on Windows and X11 converted into the
// colours of the display on its way to the screen; see displayPass.
//
// A person who wants the frame in sRGB on a Mac anyway, to compare numbers
// with another machine say, writes "display": "srgb" into
// ~/.wdygift/config.json.
//
// It is also the space to make thumbnails in, asset.Config.Space. log
// receives a note when the configuration chose; it may be nil.
func WorkingSpace(log *slog.Logger) *cms.Profile {
	if runtime.GOOS != "darwin" && runtime.GOOS != "ios" {
		return cms.SRGB()
	}
	set, path := readDisplaySetting()
	switch {
	case set.srgb:
		if log != nil {
			log.Info("gift/backend/ebiten: the display is taken for sRGB, as the user configuration says",
				slog.String("path", path))
		}
		return cms.SRGB()
	case set.profile != "" && log != nil:
		log.Warn("gift/backend/ebiten: a display profile of the user configuration is not used on macOS, "+
			"which converts into the display itself", slog.String("path", path), slog.String("display", set.profile))
	case set.bad != "" && log != nil:
		log.Warn("gift/backend/ebiten: display setting of the user configuration not understood, auto used",
			slog.String("path", path), slog.String("display", set.bad))
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

// displaySetting is the "display" entry of the [userConfig]: "auto", the
// default, "srgb", or the path of an ICC profile.
type displaySetting struct {
	srgb    bool
	profile string
	// bad is a value that is none of them.
	bad string
}

// readDisplaySetting reads the display entry of the [userConfig] and the
// path of the file it is in.
func readDisplaySetting() (displaySetting, string) {
	c, path := readUserConfig()
	d := strings.TrimSpace(c.Display)
	switch {
	case d == "" || strings.EqualFold(d, "auto"):
		return displaySetting{}, path
	case strings.EqualFold(d, "srgb"):
		return displaySetting{srgb: true}, path
	case strings.ContainsAny(d, `/\`) || strings.HasSuffix(strings.ToLower(d), ".icc") ||
		strings.HasSuffix(strings.ToLower(d), ".icm"):
		if rest, ok := strings.CutPrefix(d, "~/"); ok {
			if home, err := os.UserHomeDir(); err == nil {
				d = filepath.Join(home, rest)
			}
		}
		return displaySetting{profile: d}, path
	}
	return displaySetting{bad: d}, path
}

// displayReader reads the ICC profile of the display the window is on, nil
// for none, and names where it came from for the log. It is called on the
// main thread, from [game.LayoutF].
type displayReader func() (data []byte, source string, err error)

// displayPass converts the frame into the colours of the display, in the copy
// onto the screen that every frame has.
//
// # Why
//
// Windows without Advanced Color and X11 show a window's numbers as the
// display's own: on a display wider than sRGB every colour of gift comes out
// more saturated, on a calibrated one the calibration does not reach it. An
// application that wants its colours right converts into the display's
// profile itself, as browsers and photo editors do there. macOS converts in
// its compositor, a Windows with Advanced Color in its own, and a Wayland
// compositor is the one to ask; nothing is done there.
//
// # How
//
// The frame is drawn in sRGB as everywhere, and [directGame.DrawFinalScreen]
// copies it onto the screen through a shader that converts every pixel, made
// with package cms whenever the profile changes. A profile that changes no 8
// bit colour, a display that is sRGB, makes none and the copy stays a copy.
//
// Nearly every display profile is a matrix with a curve per channel, and
// displayshaper.kage evaluates it as package cms does: the sRGB curve, the
// matrix, and the inverse curves of the display from a table spaced by the
// square root of linear light, within a step of package cms. A table could
// not do that: an inverse curve has no finite slope at black, and a
// saturated colour whose channel the display shows near zero lands there
// from anywhere in the cube. A profile made of tables is looked up in a table
// of 33 points along each axis, closer together in the shadows, by
// display.kage, interpolated in tetrahedra.
//
// The profile is read every two seconds, from the X root window or from
// Windows, because it changes when the window moves to another display and
// when the person calibrates one. The person's configuration can name a
// file instead, or switch it all off; see [userConfig].
//
// Under [Config.DirectToScreen] a frame that converts goes through the
// offscreen: the conversion is a pass of its own.
type displayPass struct {
	read  displayReader
	log   *slog.Logger
	every time.Duration
	next  time.Time

	sum     [sha256.Size]byte
	have    bool
	lastErr string

	// pending is the conversion waiting to be uploaded, nil to switch it
	// off; dirty says there is one.
	pending *displayTable
	dirty   bool
	// changed is set with every new conversion and taken by the frame that
	// shows it; see [displayPass.takeChanged].
	changed bool

	// cur is the conversion on the GPU: its table in tex, its shader.
	cur     *displayTable
	tex     *eb.Image
	shaders [2]*eb.Shader
	verts   [4]eb.Vertex
}

// displayTable is a conversion into a display: the texture its shader reads,
// w by h texels, and the shader's uniforms.
type displayTable struct {
	pix      []byte
	w, h     int
	shaper   bool
	uniforms map[string]any
}

// displayLUTSize is the number of points of the table of a profile made of
// tables, along each axis: every eighth 8 bit level and the ends, as colour
// management systems take for a display.
const displayLUTSize = 33

// displayCurveSize is the number of entries of each inverse curve of a
// matrix profile.
const displayCurveSize = 1025

// displayPoll is how often the profile of the display is read.
const displayPoll = 2 * time.Second

var (
	//go:embed display.kage
	displayShaderSrc []byte
	//go:embed displayshaper.kage
	displayShaperSrc []byte
)

// newDisplayPass returns the conversion into the display for this machine,
// or nil where there is none: on macOS and iOS, under "display": "srgb", and
// where no profile of the display can be read and the person named none.
func newDisplayPass(log *slog.Logger) *displayPass {
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		return nil
	}
	set, path := readDisplaySetting()
	return displayPassFor(set, path, log, newDisplayReader)
}

// displayPassFor is [newDisplayPass] for a setting read from path, with the
// reader of the system made by system.
func displayPassFor(set displaySetting, path string, log *slog.Logger, system func(*slog.Logger) displayReader) *displayPass {
	switch {
	case set.srgb:
		if log != nil {
			log.Info("gift/backend/ebiten: the display is taken for sRGB, as the user configuration says",
				slog.String("path", path))
		}
		return nil
	case set.profile != "":
		data, err := os.ReadFile(set.profile)
		if err == nil {
			return &displayPass{log: log, every: displayPoll,
				read: func() ([]byte, string, error) { return data, set.profile, nil }}
		}
		if log != nil {
			log.Warn("gift/backend/ebiten: display profile of the user configuration not read, the system's used",
				slog.String("path", path), slog.String("profile", set.profile), slog.Any("err", err))
		}
	case set.bad != "":
		if log != nil {
			log.Warn("gift/backend/ebiten: display setting of the user configuration not understood, auto used",
				slog.String("path", path), slog.String("display", set.bad))
		}
	}
	r := system(log)
	if r == nil {
		return nil
	}
	return &displayPass{read: r, log: log, every: displayPoll}
}

// poll reads the profile of the display when it is time to, and makes a new
// conversion when it changed.
func (d *displayPass) poll(now time.Time) {
	if d == nil || now.Before(d.next) {
		return
	}
	d.next = now.Add(d.every)
	data, source, err := d.read()
	if err != nil {
		if msg := err.Error(); msg != d.lastErr && d.log != nil {
			d.log.Warn("gift/backend/ebiten: display profile not read", slog.String("source", source), slog.Any("err", err))
			d.lastErr = msg
		}
		return
	}
	d.lastErr = ""
	sum := sha256.Sum256(data)
	if d.have && sum == d.sum {
		return
	}
	d.sum, d.have = sum, true
	tab, desc, err := bakeDisplay(data)
	if d.log != nil {
		switch {
		case err != nil:
			d.log.Warn("gift/backend/ebiten: display profile not used", slog.String("source", source), slog.Any("err", err))
		case len(data) == 0:
			d.log.Info("gift/backend/ebiten: no display profile, the display is taken for sRGB", slog.String("source", source))
		case tab == nil:
			d.log.Info("gift/backend/ebiten: the display profile is sRGB, nothing to convert",
				slog.String("source", source), slog.String("profile", desc))
		default:
			d.log.Info("gift/backend/ebiten: converting into the display profile",
				slog.String("source", source), slog.String("profile", desc), slog.Bool("matrix", tab.shaper))
		}
	}
	if tab == nil && d.cur == nil && !d.dirty {
		return
	}
	d.pending, d.dirty, d.changed = tab, true, true
}

// bakeDisplay makes the conversion of sRGB into the display profile data. It
// returns nil for no profile and for one that changes no 8 bit colour, with
// the description of the profile.
func bakeDisplay(data []byte) (*displayTable, string, error) {
	if len(data) == 0 {
		return nil, "", nil
	}
	p, err := cms.ParseProfile(data)
	if err != nil {
		return nil, "", err
	}
	desc := p.Description()
	if p.Space() != cms.SpaceRGB {
		return nil, desc, fmt.Errorf("gift/backend/ebiten: a display profile of %s", p.Space())
	}
	t, err := cms.NewTransform(cms.SRGB(), p, cms.Options{Intent: cms.Perceptual})
	if err != nil {
		return nil, desc, err
	}
	if t.Identity() {
		return nil, desc, nil
	}
	lut, changes, err := bakeDisplayLUT(t)
	if err != nil || !changes {
		return nil, desc, err
	}
	if s, ok := t.Shaper(); ok {
		return shaperTable(s), desc, nil
	}
	return &displayTable{pix: lut, w: displayLUTSize * displayLUTSize, h: displayLUTSize,
		uniforms: map[string]any{"N": float32(displayLUTSize)}}, desc, nil
}

// bakeDisplayLUT makes the table of t: displayLUTSize slices of
// displayLUTSize² texels side by side, red along x within a slice, green
// along y, blue from slice to slice, at the colours (i/(displayLUTSize-1))².
// It reports whether any of them is not the 8 bit value it came from.
func bakeDisplayLUT(t *cms.Transform) ([]byte, bool, error) {
	const n = displayLUTSize
	src := make([]byte, 0, n*n*n*6)
	// The points are squares, close together in the shadows; display.kage
	// takes the square root of a colour to find its place.
	level := func(i int) uint16 {
		u := float64(i) / (n - 1)
		return uint16(u*u*65535 + 0.5)
	}
	for g := range n {
		for b := range n {
			for r := range n {
				for _, v := range [3]uint16{level(r), level(g), level(b)} {
					src = append(src, byte(v>>8), byte(v))
				}
			}
		}
	}
	lut := make([]byte, n*n*n*4)
	if err := t.Pixels(lut, cms.RGBA8, src, cms.RGB16); err != nil {
		return nil, false, err
	}
	for i := 0; i < len(lut); i += 4 {
		for k := range 3 {
			want := (int(src[i/4*6+2*k])<<8 | int(src[i/4*6+2*k+1])) * 255
			if int(lut[i+k]) != (want+32767)/65535 {
				return lut, true, nil
			}
		}
	}
	return lut, false, nil
}

// shaperTable is the conversion of a matrix profile: the inverse curves of
// red, green and blue in the channels of one row of displayCurveSize texels,
// entry i at linear (i/(displayCurveSize-1))², and the matrix and offset as
// uniforms.
func shaperTable(s cms.Shaper) *displayTable {
	pix := make([]byte, displayCurveSize*4)
	for i := range displayCurveSize {
		u := float64(i) / (displayCurveSize - 1)
		for c := range 3 {
			tab := s.Out[c]
			x := u * float64(len(tab)-1)
			j := min(int(x), len(tab)-2)
			v := float64(tab[j]) + (x-float64(j))*float64(tab[j+1]-tab[j])
			pix[4*i+c] = byte(min(max(v, 0), 1)*255 + 0.5)
		}
		pix[4*i+3] = 255
	}
	m := s.Matrix
	return &displayTable{pix: pix, w: displayCurveSize, h: 1, shaper: true, uniforms: map[string]any{
		"ToR":    []float32{m[0], m[1], m[2]},
		"ToG":    []float32{m[3], m[4], m[5]},
		"ToB":    []float32{m[6], m[7], m[8]},
		"Offset": []float32{s.Offset[0], s.Offset[1], s.Offset[2]},
		"L":      float32(displayCurveSize),
	}}
}

// active reports whether frames are converted.
func (d *displayPass) active() bool {
	if d == nil {
		return false
	}
	if d.dirty {
		return d.pending != nil
	}
	return d.cur != nil
}

// takeChanged reports, once, that the conversion changed since the last
// frame, which needs a frame drawn even when nothing else does.
func (d *displayPass) takeChanged() bool {
	if d == nil || !d.changed {
		return false
	}
	d.changed = false
	return true
}

// upload puts a new conversion on the GPU, or takes the old one off. It is
// called at the start of Draw, so that the final screen only draws.
func (d *displayPass) upload() {
	if d == nil || !d.dirty {
		return
	}
	d.dirty = false
	next := d.pending
	d.pending = nil
	if d.tex != nil && (next == nil || d.tex.Bounds().Dx() != next.w || d.tex.Bounds().Dy() != next.h) {
		d.tex.Deallocate()
		d.tex = nil
	}
	d.cur = next
	if next == nil {
		return
	}
	if d.tex == nil {
		d.tex = eb.NewImage(next.w, next.h)
	}
	d.tex.WritePixels(next.pix)
	next.pix = nil
}

// draw copies offscreen onto screen through the conversion, and reports
// false where there is none, for the caller to copy it as it is.
func (d *displayPass) draw(screen eb.FinalScreen, offscreen *eb.Image, geoM eb.GeoM) bool {
	if d == nil || d.cur == nil {
		return false
	}
	kind, src := 0, displayShaderSrc
	if d.cur.shaper {
		kind, src = 1, displayShaperSrc
	}
	if d.shaders[kind] == nil {
		s, err := eb.NewShader(src)
		if err != nil {
			// Not expected on any driver; the frame is shown unconverted
			// rather than not at all, and the profile is not read again.
			if d.log != nil {
				d.log.Warn("gift/backend/ebiten: the display conversion did not compile", slog.Any("err", err))
			}
			d.cur, d.read = nil, func() ([]byte, string, error) { return nil, "", nil }
			return false
		}
		d.shaders[kind] = s
	}
	b := offscreen.Bounds()
	for i, c := range [4][2]float32{
		{float32(b.Min.X), float32(b.Min.Y)}, {float32(b.Max.X), float32(b.Min.Y)},
		{float32(b.Min.X), float32(b.Max.Y)}, {float32(b.Max.X), float32(b.Max.Y)},
	} {
		x, y := geoM.Apply(float64(c[0]), float64(c[1]))
		d.verts[i] = eb.Vertex{DstX: float32(x), DstY: float32(y), SrcX: c[0], SrcY: c[1],
			ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	}
	op := &eb.DrawTrianglesShaderOptions{Uniforms: d.cur.uniforms}
	op.Images[0], op.Images[1] = offscreen, d.tex
	screen.DrawTrianglesShader(d.verts[:], []uint16{0, 1, 2, 1, 2, 3}, d.shaders[kind], op)
	return true
}
