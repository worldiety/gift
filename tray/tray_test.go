package tray

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"log/slog"
	"strings"
	"testing"

	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

// The portable half: the single attempt at opening, the validation, the
// numbering of the entries and the way a click reaches Post. The platform
// below is a fake.

type fakePlatform struct {
	states  []state
	removed int
}

func (p *fakePlatform) update(s state) error { p.states = append(p.states, s); return nil }
func (p *fakePlatform) remove() error        { p.removed++; return nil }

func fake(cfg Config) (*Tray, *fakePlatform, *func(uint64, uint32)) {
	p := &fakePlatform{}
	var clicked func(uint64, uint32)
	t := newTray(cfg, func(c openConfig) (platform, error) { clicked = c.clicked; return p, nil })
	return t, p, &clicked
}

func TestAFailedOpenIsTriedOnceAndEveryCallSaysUnsupported(t *testing.T) {
	var logs bytes.Buffer
	opens := 0
	tr := newTray(Config{Icon: outline.User, Logger: slog.New(slog.NewTextHandler(&logs, nil))}, func(openConfig) (platform, error) {
		opens++
		return nil, ErrUnsupported
	})
	for range 3 {
		if err := tr.Set([]Item{{Title: "a"}}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("error %v, want ErrUnsupported", err)
		}
	}
	if opens != 1 {
		t.Fatalf("opened %d times", opens)
	}
	if c := strings.Count(logs.String(), "tray unavailable"); c != 1 {
		t.Fatalf("logged the failure %d times:\n%s", c, logs.String())
	}
}

func TestNothingIsShownBeforeTheFirstSet(t *testing.T) {
	tr, p, _ := fake(Config{Icon: outline.User, Tooltip: "tip"})
	if err := tr.SetTooltip("other"); err != nil {
		t.Fatal(err)
	}
	if err := tr.SetIcon(outline.CameraPhoto); err != nil {
		t.Fatal(err)
	}
	if err := tr.Remove(); err != nil {
		t.Fatal(err)
	}
	if len(p.states) != 0 || p.removed != 0 {
		t.Fatalf("the platform saw %d states and %d removals before Set", len(p.states), p.removed)
	}
	if err := tr.Set(oneEntry); err != nil {
		t.Fatal(err)
	}
	if got := p.states[0]; got.tooltip != "other" || !bytes.Equal(got.icon.Rasterize(16).Pix, outline.CameraPhoto.Rasterize(16).Pix) {
		t.Fatalf("first state %+v", got)
	}
}

func TestSetRefusesWhatNoSystemCanDisplay(t *testing.T) {
	tr, p, _ := fake(Config{Icon: outline.User})
	deep := []Item{{Title: "x", OnClick: func() {}}}
	for range MaxDepth {
		deep = []Item{{Title: "sub", Items: deep}}
	}
	many := make([]Item, MaxItems+1)
	for i := range many {
		many[i] = Separator()
	}
	for name, bad := range map[string][]Item{
		"no title":          {{}},
		"invalid UTF-8":     {{Title: "a\xffb"}},
		"NUL":               {{Title: "a\x00b"}},
		"line break":        {{Title: "a\nb"}},
		"submenu and click": {{Title: "s", OnClick: func() {}, Items: []Item{{Title: "x"}}}},
		"checked submenu":   {{Title: "s", Checked: true, Items: []Item{{Title: "x"}}}},
		"too deep":          deep,
		"too many":          many,
		"bad child":         {{Title: "s", Items: []Item{{}}}},
	} {
		if err := tr.Set(bad); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if len(p.states) != 0 {
		t.Fatalf("%d refused menus reached the platform", len(p.states))
	}
	if err := New(Config{}).Set(oneEntry); err == nil {
		t.Error("a tray without an icon was accepted")
	}
	if err := tr.SetIcon(ui.Symbol{}); err == nil {
		t.Error("the zero Symbol was accepted as the icon")
	}
	if err := tr.SetTooltip("a\nb"); err == nil {
		t.Error("a tooltip with a line break was accepted")
	}
}

func TestClickableEntriesAreNumberedInOrderAcrossSubmenus(t *testing.T) {
	tr, p, _ := fake(Config{Icon: outline.User})
	f := func() {}
	if err := tr.Set([]Item{
		{Title: "a", OnClick: f},
		Separator(),
		{Title: "label"},
		{Title: "sub", Items: []Item{{Title: "b", OnClick: f}, {Title: "c", OnClick: f, Checked: true}}},
		{Title: "d", OnClick: f, Disabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	m := p.states[0].menu
	got := []uint32{m[0].id, m[1].id, m[2].id, m[3].id, m[3].children[0].id, m[3].children[1].id, m[4].id}
	want := []uint32{1, 0, 0, 0, 2, 3, 4}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids %v, want %v", got, want)
		}
	}
	if !m[1].separator || !m[3].children[1].checked || !m[4].disabled {
		t.Fatalf("flags lost: %+v", m)
	}
}

func TestAClickIsPostedOnlyForTheMenuShownNow(t *testing.T) {
	var posted []func()
	tr, p, clicked := fake(Config{Icon: outline.User, Post: func(fn func()) { posted = append(posted, fn) }})
	var hits []string
	if err := tr.Set([]Item{{Title: "a", OnClick: func() { hits = append(hits, "a") }}}); err != nil {
		t.Fatal(err)
	}
	old := p.states[0].gen
	(*clicked)(old, 1)
	(*clicked)(old, 2) // no such entry
	(*clicked)(old, 0)
	if err := tr.Set([]Item{{Title: "b", OnClick: func() { hits = append(hits, "b") }}}); err != nil {
		t.Fatal(err)
	}
	(*clicked)(old, 1) // made in the replaced menu
	(*clicked)(p.states[1].gen, 1)
	if err := tr.SetTooltip("t"); err != nil {
		t.Fatal(err)
	}
	(*clicked)(p.states[2].gen, 1) // a tooltip does not change the menu
	if err := tr.Remove(); err != nil {
		t.Fatal(err)
	}
	(*clicked)(p.states[2].gen, 1)
	for _, fn := range posted {
		fn()
	}
	if strings.Join(hits, ",") != "a,b,b" {
		t.Fatalf("ran %v", hits)
	}
}

func TestANilPostDropsTheClickAndSaysSoOnce(t *testing.T) {
	var logs bytes.Buffer
	tr, p, clicked := fake(Config{Icon: outline.User, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if err := tr.Set([]Item{{Title: "a", OnClick: func() { t.Fatal("ran without Post") }}}); err != nil {
		t.Fatal(err)
	}
	(*clicked)(p.states[0].gen, 1)
	(*clicked)(p.states[0].gen, 1)
	if c := strings.Count(logs.String(), "Config.Post is nil"); c != 1 {
		t.Fatalf("warned %d times", c)
	}
}

func TestRemoveAndSetAgainShowTheIconAgain(t *testing.T) {
	tr, p, _ := fake(Config{Icon: outline.User})
	if err := tr.Set(oneEntry); err != nil {
		t.Fatal(err)
	}
	if err := tr.Remove(); err != nil {
		t.Fatal(err)
	}
	if err := tr.Remove(); err != nil {
		t.Fatal(err)
	}
	if p.removed != 1 {
		t.Fatalf("removed %d times", p.removed)
	}
	if err := tr.Set(oneEntry); err != nil {
		t.Fatal(err)
	}
	if len(p.states) != 2 || p.states[1].gen <= p.states[0].gen {
		t.Fatalf("states %+v", p.states)
	}
}

func TestLongTextIsCutAtARuneBoundary(t *testing.T) {
	tr, p, _ := fake(Config{Icon: outline.User})
	title := strings.Repeat("a", MaxTitleBytes-1) + "ä" + "tail"
	if err := tr.Set([]Item{{Title: title}}); err != nil {
		t.Fatal(err)
	}
	if got := p.states[0].menu[0].title; got != strings.Repeat("a", MaxTitleBytes-1) {
		t.Fatalf("kept %d bytes", len(got))
	}
}

func TestTintedKeepsTheCoverageAsAlpha(t *testing.T) {
	im := tinted(outline.User, 32, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	if im == nil || im.Rect.Dx() != 32 {
		t.Fatalf("got %v", im)
	}
	ink := 0
	for i := 0; i < len(im.Pix); i += 4 {
		if im.Pix[i] != 10 || im.Pix[i+1] != 20 || im.Pix[i+2] != 30 {
			t.Fatalf("colour at %d is %v", i/4, im.Pix[i:i+3])
		}
		if im.Pix[i+3] != 0 {
			ink++
		}
	}
	if ink == 0 {
		t.Fatal("no ink")
	}
	pm := premultiplied(im)
	for i := 0; i < len(pm); i += 4 {
		if pm[i] > pm[i+3] || pm[i+2] > pm[i+3] {
			t.Fatalf("pixel %d is not premultiplied: %v", i/4, pm[i:i+4])
		}
	}
	if tinted(ui.Symbol{}, 32, color.NRGBA{A: 255}) != nil {
		t.Fatal("the zero Symbol produced a picture")
	}
}

func TestSetIconAndSetTooltipHandOnTheMenuOfTheLastSet(t *testing.T) {
	tr, p, _ := fake(Config{Icon: outline.User})
	f := func() {}
	for range 2 {
		if err := tr.Set([]Item{{Title: "a", OnClick: f}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tr.SetTooltip("t"); err != nil {
		t.Fatal(err)
	}
	if err := tr.SetIcon(outline.CameraPhoto); err != nil {
		t.Fatal(err)
	}
	s := p.states
	same := func(a, b []entry) bool { return &a[0] == &b[0] }
	if same(s[0].menu, s[1].menu) {
		t.Fatal("two Sets share a menu slice")
	}
	if !same(s[1].menu, s[2].menu) || !same(s[1].menu, s[3].menu) {
		t.Fatal("SetTooltip or SetIcon replaced the menu slice")
	}
}

// notYet answers every update with "not now" until ready is set.
type notYet struct {
	fakePlatform
	ready bool
}

func (p *notYet) update(s state) error {
	p.states = append(p.states, s)
	if !p.ready {
		return fmt.Errorf("%w: not yet", ErrUnsupported)
	}
	return nil
}

func TestNotNowIsLoggedOncePerOutage(t *testing.T) {
	var logs bytes.Buffer
	p := &notYet{}
	tr := newTray(Config{Icon: outline.User, Logger: slog.New(slog.NewTextHandler(&logs, nil))},
		func(openConfig) (platform, error) { return p, nil })
	for range 3 {
		if err := tr.Set(oneEntry); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("error %v", err)
		}
	}
	p.ready = true
	if err := tr.Set(oneEntry); err != nil {
		t.Fatal(err)
	}
	p.ready = false
	_ = tr.Set(oneEntry)
	out := logs.String()
	if c := strings.Count(out, "kept until the system can show it"); c != 2 {
		t.Fatalf("logged %d times:\n%s", c, out)
	}
	if strings.Contains(out, "level=ERROR") {
		t.Fatalf("a wait was logged as an error:\n%s", out)
	}
}

// oneEntry is the smallest menu Set accepts.
var oneEntry = []Item{{Title: "Quit"}}

func TestAMenuWithoutAnEntryIsRefused(t *testing.T) {
	tr, p, _ := fake(Config{Icon: outline.User})
	for _, items := range [][]Item{nil, {}, {Separator(), Separator()}} {
		if err := tr.Set(items); err == nil {
			t.Errorf("Set(%v) was accepted", items)
		}
	}
	if len(p.states) != 0 {
		t.Fatalf("the platform saw %d states", len(p.states))
	}
	if err := tr.Set([]Item{Separator(), {Title: "Disabled", Disabled: true}}); err != nil {
		t.Fatal(err)
	}
}

func TestTheSameMenuIsTheSliceOfOneSet(t *testing.T) {
	a := []entry{{id: 1, title: "a"}}
	b := []entry{{id: 1, title: "a"}}
	if !sameMenu(a, a) || !sameMenu(a[:1], a) || !sameMenu(nil, []entry{}) {
		t.Error("a slice is not the same as itself")
	}
	if sameMenu(a, b) {
		t.Error("two equal menus of two Sets are the same")
	}
}
