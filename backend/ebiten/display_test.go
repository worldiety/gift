package ebiten

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/cms"
)

func TestTheDisplaySettingIsReadAsWritten(t *testing.T) {
	for _, c := range []struct {
		file string
		want displaySetting
	}{
		{"", displaySetting{}},
		{`{"display": "auto"}`, displaySetting{}},
		{`{"display": " SRGB "}`, displaySetting{srgb: true}},
		{`{"display": "/usr/share/color/icc/monitor.icc"}`, displaySetting{profile: "/usr/share/color/icc/monitor.icc"}},
		{`{"display": "C:\\Windows\\monitor.icm"}`, displaySetting{profile: `C:\Windows\monitor.icm`}},
		{`{"display": "monitor.ICM"}`, displaySetting{profile: "monitor.ICM"}},
		{`{"display": "vivid"}`, displaySetting{bad: "vivid"}},
	} {
		withHome(t, c.file)
		if got, _ := readDisplaySetting(); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.file, got, c.want)
		}
	}
	withHome(t, `{"display": "~/calibrated.icc"}`)
	home, _ := os.UserHomeDir()
	if got, _ := readDisplaySetting(); got.profile != filepath.Join(home, "calibrated.icc") {
		t.Errorf("~ is %q", got.profile)
	}
}

// A display with a matrix profile gets its matrix and curves, one with
// tables a table; a display that is sRGB gets nothing, and neither does no
// profile.
func TestTheDisplayConversionIsTheProfile(t *testing.T) {
	if tab, _, err := bakeDisplay(nil); tab != nil || err != nil {
		t.Errorf("no profile: %v, %v", tab, err)
	}
	if tab, desc, err := bakeDisplay(cms.SRGB().Bytes()); tab != nil || err != nil {
		t.Errorf("an sRGB display (%s): %v, %v", desc, tab, err)
	}
	if _, _, err := bakeDisplay(cms.SGray().Bytes()); err == nil {
		t.Error("a gray display profile accepted")
	}
	if _, _, err := bakeDisplay([]byte("not a profile")); err == nil {
		t.Error("garbage accepted")
	}

	tab, desc, err := bakeDisplay(cms.DisplayP3().Bytes())
	if err != nil || tab == nil || !tab.shaper || desc != "Display P3" {
		t.Fatalf("a P3 display: %+v, %q, %v", tab, desc, err)
	}
	if tab.w != displayCurveSize || tab.h != 1 || len(tab.pix) != 4*displayCurveSize {
		t.Errorf("curves of %dx%d", tab.w, tab.h)
	}
	// The curves of P3 are the sRGB curve: entry i is linear (i/(L-1))².
	for _, i := range []int{0, 100, 512, displayCurveSize - 1} {
		u := float64(i) / (displayCurveSize - 1)
		want := srgbEncode(u*u) * 255
		for c := range 3 {
			if d := float64(tab.pix[4*i+c]) - want; d > 0.5 || d < -0.5 {
				t.Errorf("entry %d channel %d is %d, want %.1f", i, c, tab.pix[4*i+c], want)
			}
		}
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "cms", "testdata", "profiles", "rgb-mab.icc"))
	if err != nil {
		t.Fatal(err)
	}
	if tab, _, err := bakeDisplay(data); err != nil || tab == nil || tab.shaper || tab.w != displayLUTSize*displayLUTSize {
		t.Errorf("a display with tables: %+v, %v", tab, err)
	}
}

// The table is sRGB through the display profile, laid out as display.kage
// reads it.
func TestTheDisplayTableIsLaidOutForTheShader(t *testing.T) {
	tr, err := cms.NewTransform(cms.SRGB(), cms.DisplayP3(), cms.Options{Intent: cms.Perceptual})
	if err != nil {
		t.Fatal(err)
	}
	lut, changes, err := bakeDisplayLUT(tr)
	if err != nil || !changes {
		t.Fatalf("%v, %v", changes, err)
	}
	const n = displayLUTSize
	if len(lut) != n*n*n*4 {
		t.Fatalf("%d bytes", len(lut))
	}
	at := func(r, g, b int) []byte {
		i := 4 * (g*n*n + b*n + r)
		return lut[i : i+4]
	}
	for _, p := range [][3]int{{n - 1, 0, 0}, {0, n - 1, 0}, {0, 0, n - 1}, {8, 16, 24}, {n - 1, n - 1, n - 1}, {0, 0, 0}} {
		sq := func(i int) float64 { u := float64(i) / (n - 1); return u * u }
		want := tr.Color(sq(p[0]), sq(p[1]), sq(p[2]))
		got := at(p[0], p[1], p[2])
		for k := range 3 {
			if d := float64(got[k]) - want[k]*255; d > 1 || d < -1 {
				t.Errorf("%v: %v, want %.1f", p, got[:3], []float64{want[0] * 255, want[1] * 255, want[2] * 255})
				break
			}
		}
		if got[3] != 255 {
			t.Errorf("%v: alpha %d", p, got[3])
		}
	}
}

// The profile is read when it is time to, and a table made only when it
// changed; the frame that shows a change is asked for once.
func TestTheDisplayIsPolled(t *testing.T) {
	var (
		data  []byte
		err   error
		reads int
	)
	var buf bytes.Buffer
	d := displayPassFor(displaySetting{}, "", slog.New(slog.NewTextHandler(&buf, nil)),
		func(*slog.Logger) displayReader {
			return func() ([]byte, string, error) { reads++; return data, "test", err }
		})
	now := time.Unix(1000, 0)
	poll := func() { d.poll(now); now = now.Add(displayPoll) }

	poll()
	if d.active() || d.takeChanged() || reads != 1 {
		t.Fatalf("no profile: active %v, %d reads", d.active(), reads)
	}
	d.poll(now.Add(-time.Second))
	if reads != 1 {
		t.Fatal("read before it was time")
	}

	data = cms.DisplayP3().Bytes()
	poll()
	if !d.active() || !d.takeChanged() || d.takeChanged() {
		t.Fatal("a wide display: no conversion, or the change not reported once")
	}
	poll()
	if d.takeChanged() {
		t.Error("the same profile again changed something")
	}

	err = errors.New("the X server is gone")
	poll()
	poll()
	if !d.active() || strings.Count(buf.String(), "the X server is gone") != 1 {
		t.Errorf("a failed read: active %v, log:\n%s", d.active(), buf.String())
	}
	err = nil

	data = cms.SRGB().Bytes()
	poll()
	if d.active() || !d.takeChanged() {
		t.Error("an sRGB display still converts")
	}
}

func TestTheDisplaySettingDecidesThePass(t *testing.T) {
	system := func(*slog.Logger) displayReader {
		return func() ([]byte, string, error) { return nil, "system", nil }
	}
	none := func(*slog.Logger) displayReader { return nil }
	if d := displayPassFor(displaySetting{srgb: true}, "", nil, system); d != nil {
		t.Error("srgb converts")
	}
	if d := displayPassFor(displaySetting{}, "", nil, none); d != nil {
		t.Error("a pass with nothing to read")
	}
	if d := displayPassFor(displaySetting{bad: "vivid"}, "", nil, system); d == nil {
		t.Error("a setting not understood switched the system's profile off")
	}

	file := filepath.Join(t.TempDir(), "wide.icc")
	if err := os.WriteFile(file, cms.DisplayP3().Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	d := displayPassFor(displaySetting{profile: file}, "", nil, none)
	if d == nil {
		t.Fatal("the profile of the user is not used")
	}
	d.poll(time.Now())
	if !d.active() {
		t.Error("the profile of the user is not converted into")
	}
	if d := displayPassFor(displaySetting{profile: file + ".missing"}, "", nil, system); d == nil {
		t.Error("a missing file switched the system's profile off")
	}
}

func TestTheDisplayShadersCompile(t *testing.T) {
	for _, src := range [][]byte{displayShaderSrc, displayShaperSrc} {
		if _, err := eb.NewShader(src); err != nil {
			t.Fatal(err)
		}
	}
}
