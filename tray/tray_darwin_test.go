//go:build darwin && !ios

package tray

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/worldiety/gift/icon/outline"
)

// init keeps the main goroutine on the main thread, for TestMain.
func init() { runtime.LockOSThread() }

// TestMain runs an NSApplication on the main thread while the real menu bar
// is under test, which is what an Ebitengine application has and a test
// binary has not: without NSApp this package refuses to open, and without
// the run loop of the main thread nothing queued for it would ever run.
//
// The application is an accessory, the activation policy of a menu bar
// extra: no Dock icon and no menu bar of its own, only the status item.
func TestMain(m *testing.M) {
	if os.Getenv("GIFT_TRAY_MACOS") != "1" {
		os.Exit(m.Run())
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	const nsApplicationActivationPolicyAccessory = 1
	app.Send(objc.RegisterName("setActivationPolicy:"), nsApplicationActivationPolicyAccessory)
	go func() { os.Exit(m.Run()) }()
	// run never returns; the tests end the process with os.Exit.
	app.Send(objc.RegisterName("run"))
}

func integration() bool { return os.Getenv("GIFT_TRAY_MACOS") == "1" }

// TestNoApplicationIsUnsupported is the reason for the NSApp check: a plain
// `go test` binary has no NSApplication, a status item made in it would never
// appear, and the package says so instead of pretending. The state is kept
// all the same, for an application whose NSApp comes later.
func TestNoApplicationIsUnsupported(t *testing.T) {
	if integration() {
		t.Skip("TestMain runs an NSApplication")
	}
	var logs bytes.Buffer
	tr := New(Config{Icon: outline.User, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if err := tr.Set([]Item{{Title: "a", OnClick: func() {}}}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error %v, want ErrUnsupported", err)
	}
	if !strings.Contains(logs.String(), "NSApplication") {
		t.Fatalf("the log does not say what is missing:\n%s", logs.String())
	}
	if mac == nil {
		t.Fatalf("the frameworks did not open: %v", macErr)
	}
	if mac.hasApp() {
		t.Fatal("asking for NSApp created it")
	}
	// Set before backend.Run: the tray is open, the state is queued for the
	// main thread, and a later Set is tried again rather than refused for
	// good.
	p, ok := tr.p.(*macTray)
	if !ok {
		t.Fatalf("the platform did not open: %T", tr.p)
	}
	if err := tr.SetTooltip("later"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("SetTooltip: error %v, want ErrUnsupported", err)
	}
	p.want.mu.Lock()
	w, queued := p.want.want, p.want.queued
	p.want.mu.Unlock()
	if w == nil || w.tooltip != "later" || !queued {
		t.Fatalf("wanted %+v, queued %v; want the latest state, queued once", w, queued)
	}
}

func TestSetIconAndSetTooltipKeepTheMenuOfTheSet(t *testing.T) {
	var g menuGens
	set1 := []entry{{id: 1, title: "a"}}
	set2 := []entry{{id: 1, title: "a"}} // equal, but another Set
	if got := g.note(state{gen: 1, menu: set1}); got != 1 {
		t.Fatalf("first Set: menu generation %d", got)
	}
	low1, _ := splitTag(makeTag(1, 1))
	for gen := uint64(2); gen <= 4; gen++ { // SetTooltip, SetIcon, SetTooltip
		if got := g.note(state{gen: gen, menu: set1}); got != 1 {
			t.Fatalf("gen %d: menu generation %d, want the Set's 1", gen, got)
		}
		if got := g.resolve(low1); got != gen {
			t.Fatalf("gen %d: a click in the open menu was reported as %d", gen, got)
		}
	}
	if got := g.note(state{gen: 5, menu: set2}); got != 5 {
		t.Fatalf("second Set: menu generation %d", got)
	}
	if got := g.resolve(low1); got != 1 {
		t.Fatalf("a click in the replaced menu was reported as %d, want its own 1", got)
	}
	low5, _ := splitTag(makeTag(5, 1))
	if got := g.resolve(low5); got != 5 {
		t.Fatalf("a click in the new menu was reported as %d", got)
	}
	if !sameMenu(nil, []entry{}) || sameMenu(set1, set2) || !sameMenu(set1, set1) {
		t.Fatal("sameMenu")
	}
}

// TestTheABIStructsHaveTheirSizes checks the structs this file passes to and
// reads from AppKit against the sizes of NSSize and NSRect on a 64 bit Mac,
// and the tag against NSInteger.
func TestTheABIStructsHaveTheirSizes(t *testing.T) {
	if s := unsafe.Sizeof(nsSize{}); s != 16 {
		t.Errorf("nsSize is %d bytes, NSSize is 16", s)
	}
	if s := unsafe.Sizeof(nsRect{}); s != 32 {
		t.Errorf("nsRect is %d bytes, NSRect is 32", s)
	}
	if s := unsafe.Sizeof(int(0)); s != 8 {
		t.Errorf("int is %d bytes, NSInteger is 8", s)
	}
}

func TestATagCarriesTheGenerationAndTheID(t *testing.T) {
	const span = 1 << genBits
	for _, gen := range []uint64{1, 2, span - 1, span, span + 5, 3*span + 7, 1<<40 + 3, 1<<63 + 9} {
		for _, id := range []uint32{1, 2, MaxItems, 1<<32 - 1} {
			tag := makeTag(gen, id)
			if tag <= 0 {
				t.Fatalf("gen %d id %d: tag %d is not a positive NSInteger", gen, id, tag)
			}
			low, gotID := splitTag(tag)
			if gotID != id {
				t.Fatalf("gen %d: id %d came back as %d", gen, id, gotID)
			}
			for _, ahead := range []uint64{0, 1, 1000, span - 1} {
				if gen+ahead < gen {
					continue
				}
				if got := fullGen(gen+ahead, low); got != gen {
					t.Fatalf("gen %d, latest %d: recovered %d", gen, gen+ahead, got)
				}
			}
		}
	}
}

func TestUpdatesBeforeTheMainThreadRunsCollapseIntoOne(t *testing.T) {
	var q pending
	a, b := &prepared{tooltip: "a"}, &prepared{tooltip: "b"}
	if !q.set(a) {
		t.Fatal("the first set did not ask to be queued")
	}
	if q.set(b) || q.set(nil) || q.set(b) {
		t.Fatal("a set while queued asked to be queued again")
	}
	if p, changed := q.take(); p != b || !changed {
		t.Fatalf("took %v %v, want the latest", p, changed)
	}
	if _, changed := q.take(); changed {
		t.Fatal("a second take without set reported a change")
	}
	if !q.set(nil) {
		t.Fatal("a set after take did not ask to be queued")
	}
	if p, changed := q.take(); p != nil || !changed {
		t.Fatalf("took %v %v, want the removal", p, changed)
	}
	// No NSApplication on the main thread yet: the state stays wanted and
	// the next set queues again.
	if !q.set(a) {
		t.Fatal("a set after take did not ask to be queued")
	}
	q.unqueue()
	if !q.set(b) {
		t.Fatal("a set after unqueue did not ask to be queued")
	}
	if p, changed := q.take(); p != b || !changed {
		t.Fatalf("took %v %v after unqueue, want the latest", p, changed)
	}
}

func TestTheTemplateIsBlackWithTheCoverageAsAlpha(t *testing.T) {
	m := outline.User.Rasterize(36)
	pic := templatePNG(m)
	im, err := png.Decode(bytes.NewReader(pic))
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds() != image.Rect(0, 0, 36, 36) {
		t.Fatalf("bounds %v", im.Bounds())
	}
	ink := 0
	for y := range 36 {
		for x := range 36 {
			r, g, b, a := im.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 {
				t.Fatalf("pixel %d,%d is not black", x, y)
			}
			if want := uint32(m.Pix[y*m.Stride+x]); a>>8 != want {
				t.Fatalf("pixel %d,%d has alpha %d, coverage %d", x, y, a>>8, want)
			}
			if a != 0 {
				ink++
			}
		}
	}
	if ink == 0 {
		t.Fatal("no ink")
	}
	if templatePNG(nil) != nil {
		t.Fatal("no mask made a picture")
	}
}

func TestPointsArePixelsAtTheScale(t *testing.T) {
	for _, c := range []struct {
		pts, scale float64
		want       int
	}{{18, 2, 36}, {16, 2, 32}, {18, 1, 18}, {16, 1.5, 24}, {18, 3, 54}, {18, 1.25, 23}} {
		if got := pixels(c.pts, c.scale); got != c.want {
			t.Errorf("%v points at %v: %d pixels, want %d", c.pts, c.scale, got, c.want)
		}
	}
}

func TestPrepareTagsClickableEntriesAndDrawsTheirSymbols(t *testing.T) {
	st := state{gen: 7, icon: outline.User, tooltip: "tip", menu: []entry{
		{id: 1, title: "a", symbol: outline.CameraPhoto},
		{separator: true},
		{title: "label"},
		{title: "sub", children: []entry{{id: 2, title: "b", checked: true}, {id: 3, title: "c", disabled: true}}},
	}}
	p := prepare(st, 7, 2)
	if len(p.icon) == 0 || p.tooltip != "tip" || p.scale != 2 || p.menuGen != 7 {
		t.Fatalf("prepared %+v", p)
	}
	m := p.menu
	if m[0].tag != makeTag(7, 1) || len(m[0].picture) == 0 {
		t.Fatalf("entry a: %+v", m[0])
	}
	if !m[1].separator || m[1].tag != 0 || m[2].tag != 0 || m[3].tag != 0 || m[2].picture != nil {
		t.Fatalf("not clickable entries: %+v", m[1:])
	}
	c := m[3].children
	if len(c) != 2 || c[0].tag != makeTag(7, 2) || !c[0].checked || c[1].tag != makeTag(7, 3) || !c[1].disabled {
		t.Fatalf("submenu: %+v", c)
	}
}

// onMain runs f on the main thread and waits for it.
func onMain(t *testing.T, f func()) {
	t.Helper()
	done := make(chan struct{})
	mac.async(func() { f(); close(done) })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the main thread did not run the function within 5s")
	}
}

// TestMacOSMenuBar puts a real icon into the menu bar and is off unless
// GIFT_TRAY_MACOS=1:
//
//	GIFT_TRAY_MACOS=1 go test ./tray/ -run MacOS -v
//
// It needs a logged in GUI session but no app bundle. The icon stays for
// GIFT_TRAY_WAIT (default 3s) after the first Set, long enough to look at it
// and open the menu; the clicks the test checks are made by AppKit's own
// performActionForItemAtIndex:, which runs the action of the entry exactly as
// a click does.
func TestMacOSMenuBar(t *testing.T) {
	if !integration() {
		t.Skip("set GIFT_TRAY_MACOS=1 in a GUI session")
	}
	r := objc.RegisterName
	var (
		selButton, selFrame, selWindow = r("button"), r("frame"), r("window")
		selMenu, selCount, selItemAt   = r("menu"), r("numberOfItems"), r("itemAtIndex:")
		selState, selEnabled           = r("state"), r("isEnabled")
		selSubmenu, selPerform         = r("submenu"), r("performActionForItemAtIndex:")
		selImage, selTemplate          = r("image"), r("isTemplate")
		selRetain, selRelease          = r("retain"), r("release")
		selSeparator                   = r("isSeparatorItem")
		selToolTip, selEqual           = r("toolTip"), r("isEqualToString:")
	)

	clicks := make(chan string, 16)
	click := func(s string) func() { return func() { clicks <- s } }
	var logs bytes.Buffer
	tr := New(Config{
		Icon:    outline.User,
		Tooltip: "gift/tray test",
		Post:    func(fn func()) { fn() },
		Logger:  slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	defer func() { t.Logf("log:\n%s", logs.String()) }()
	if err := tr.Set([]Item{
		{Title: "First", Symbol: outline.CameraPhoto, OnClick: click("first")},
		Separator(),
		{Title: "Checked", Checked: true, OnClick: click("checked")},
		{Title: "Disabled", Disabled: true, OnClick: click("disabled")},
		{Title: "Submenu", Items: []Item{
			{Title: "Inner", OnClick: click("inner")},
			{Title: "Deeper", Items: []Item{{Title: "Deepest", Items: []Item{{Title: "Leaf", OnClick: click("leaf")}}}}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	p := tr.p.(*macTray)

	// waitFor polls the main thread until cond holds there.
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			ok := false
			onMain(t, func() { ok = cond() })
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s did not happen within 5s", what)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	expect := func(want string) {
		t.Helper()
		select {
		case got := <-clicks:
			if got != want {
				t.Fatalf("click on %q, want %q", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no click on %q arrived", want)
		}
	}
	expectNone := func() {
		t.Helper()
		select {
		case got := <-clicks:
			t.Fatalf("click on %q, want none", got)
		case <-time.After(300 * time.Millisecond):
		}
	}

	waitFor("the status item", func() bool { return p.item != 0 })
	var first, oldMenu objc.ID
	onMain(t, func() {
		first = p.item
		btn := p.item.Send(selButton)
		frame := objc.Send[nsRect](btn, selFrame)
		win := btn.Send(selWindow)
		im := btn.Send(selImage)
		t.Logf("button frame %+v, window %#x, template image %v", frame, win, objc.Send[bool](im, selTemplate))
		if frame.Width <= 0 || frame.Height <= 0 || win == 0 {
			t.Errorf("the button is not on screen: frame %+v, window %#x", frame, win)
		}
		if im == 0 || !objc.Send[bool](im, selTemplate) {
			t.Error("the icon is not a template image")
		}
		m := p.item.Send(selMenu)
		if n := objc.Send[int](m, selCount); n != 5 {
			t.Fatalf("the menu has %d items, want 5", n)
		}
		item := func(menu objc.ID, i int) objc.ID { return menu.Send(selItemAt, i) }
		if item(m, 0).Send(selImage) == 0 {
			t.Error("the first entry has no image")
		}
		if !objc.Send[bool](item(m, 1), selSeparator) {
			t.Error("the second entry is not a separator")
		}
		if objc.Send[int](item(m, 2), selState) != nsControlStateValueOn {
			t.Error("the checked entry has no check mark")
		}
		if objc.Send[bool](item(m, 3), selEnabled) {
			t.Error("the disabled entry is enabled")
		}
		sub := item(m, 4).Send(selSubmenu)
		if objc.Send[int](sub, selCount) != 2 {
			t.Error("the submenu does not have 2 entries")
		}
		leafMenu := item(item(item(sub, 1).Send(selSubmenu), 0).Send(selSubmenu), 0)
		if leafMenu == 0 {
			t.Error("the fourth level is missing")
		}
		m.Send(selPerform, 0)
		sub.Send(selPerform, 0)
		oldMenu = m.Send(selRetain)
	})
	expect("first")
	expect("inner")

	wait := 3 * time.Second
	if d, err := time.ParseDuration(os.Getenv("GIFT_TRAY_WAIT")); err == nil {
		wait = d
	}
	t.Logf("the icon is in the menu bar for %v", wait)
	time.Sleep(wait)

	// A tooltip that counts and a new icon while the menu is open: the menu
	// stays the same NSMenu, and a click in it still arrives.
	for i := range 5 {
		if err := tr.SetTooltip(fmt.Sprintf("gift/tray test %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tr.SetIcon(outline.CameraPhoto); err != nil {
		t.Fatal(err)
	}
	waitFor("the counted tooltip", func() bool {
		want := mac.newString("gift/tray test 4")
		defer want.Send(selRelease)
		tip := p.item.Send(selButton).Send(selToolTip)
		return tip != 0 && objc.Send[bool](tip, selEqual, want)
	})
	onMain(t, func() {
		if p.item.Send(selMenu) != oldMenu {
			t.Error("SetTooltip or SetIcon rebuilt the menu")
		}
		oldMenu.Send(selPerform, 0)
	})
	expect("first")

	if err := tr.SetTooltip("gift/tray test, updated"); err != nil {
		t.Fatal(err)
	}
	for i := range 20 { // a burst, which the main thread applies once
		if err := tr.Set([]Item{{Title: "Replaced", OnClick: click("replaced")}, {Title: "Burst", Checked: i%2 == 0}}); err != nil {
			t.Fatal(err)
		}
	}
	waitFor("the replaced menu", func() bool { return objc.Send[int](p.item.Send(selMenu), selCount) == 2 })
	onMain(t, func() {
		if p.item != first {
			t.Error("an update made a new status item instead of changing the one in the bar")
		}
		if objc.Send[int](p.item.Send(selMenu).Send(selItemAt, 1), selState) == nsControlStateValueOn {
			t.Error("the burst did not end with its last state")
		}
		oldMenu.Send(selPerform, 0) // a click in the replaced menu, still open
		p.item.Send(selMenu).Send(selPerform, 0)
		oldMenu.Send(selRelease)
	})
	expect("replaced")
	expectNone()

	if err := tr.Remove(); err != nil {
		t.Fatal(err)
	}
	waitFor("the removal", func() bool { return p.item == 0 })
	if err := tr.Set([]Item{{Title: "Back", OnClick: click("back")}}); err != nil {
		t.Fatal(err)
	}
	waitFor("the icon to come back", func() bool { return p.item != 0 })
	onMain(t, func() { p.item.Send(selMenu).Send(selPerform, 0) })
	expect("back")
	if err := tr.Remove(); err != nil {
		t.Fatal(err)
	}
	waitFor("the final removal", func() bool { return p.item == 0 })
}
