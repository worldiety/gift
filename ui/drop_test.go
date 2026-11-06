package ui_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
)

// dropScene is an outer file destination of 400 by 300 with an inner text
// destination keyed "inner", and a button keyed "press" inside the outer one.
type dropScene struct {
	outer, inner []ui.DropInfo
	targeted     []bool
	pressed      int
	disabled     bool
}

func (s *dropScene) root(*gift.Context) gift.View {
	inner := ui.DropDestination(ui.Text("note").Frame(120, 40)).Key("inner").
		Accepts(gift.DropText).
		OnDrop(func(d ui.DropInfo) bool { s.inner = append(s.inner, d); return true })
	return ui.DropDestination(
		ui.VStack(
			inner,
			ui.Button(ui.Text("press"), func() { s.pressed++ }).Key("press"),
		).Gap(20).Padding(40).Frame(400, 300),
	).Key("outer").Accepts(gift.DropFile).Disabled(s.disabled).
		OnDrop(func(d ui.DropInfo) bool { s.outer = append(s.outer, d); return true }).
		OnTargeted(func(on bool) { s.targeted = append(s.targeted, on) })
}

func newDropScene(t *testing.T) (*gifttest.Harness, *dropScene) {
	s := &dropScene{}
	h := gifttest.New(t, gifttest.Options{Root: s.root, Size: geom.Sz(400, 300), Font: ui.MustFont(ui.FontQuery{Family: inter.Family})})
	return h, s
}

func TestADropDestinationFiltersByKind(t *testing.T) {
	h, s := newDropScene(t)
	outer := h.Find(gifttest.ByKey("outer"))
	press := h.Find(gifttest.ByKey("press"))

	// Text over the button: only the outer area is there, and it takes files.
	press.DragOver(gift.DropText)
	outer.AssertNotDropTargeted()
	if press.DropOnto(gift.DropTextData("x")) {
		t.Error("the file destination took text")
	}
	if len(s.outer) != 0 || len(s.targeted) != 0 {
		t.Errorf("outer %v, targeted %v", s.outer, s.targeted)
	}

	press.DragOver(gift.DropFile)
	outer.AssertDropTargeted()
	if !press.DropOnto(gift.DropFiles("/tmp/a.txt", "/tmp/b.txt")) {
		t.Fatal("the files were not taken")
	}
	outer.AssertNotDropTargeted()
	if got := s.outer[0].Files(); len(got) != 2 || got[1] != "/tmp/b.txt" {
		t.Errorf("files %v", got)
	}
	if len(s.targeted) != 2 || !s.targeted[0] || s.targeted[1] {
		t.Errorf("OnTargeted was told %v, want [true false]", s.targeted)
	}
}

func TestAFinderDropWithFilesGoesToTheOuterFileDestination(t *testing.T) {
	h, s := newDropScene(t)
	inner := h.Find(gifttest.ByKey("inner"))
	// What the Finder drags: file URLs, and the paths as text, which the
	// backend reports as files only.
	inner.DragOver(gift.DropFile)
	inner.AssertNotDropTargeted()
	h.Find(gifttest.ByKey("outer")).AssertDropTargeted()
	if !inner.DropOnto(gift.DropFiles("/tmp/a.txt")) {
		t.Fatal("not taken")
	}
	if len(s.outer) != 1 || len(s.inner) != 0 {
		t.Errorf("outer %d drops, inner %d", len(s.outer), len(s.inner))
	}

	// A drop that does carry both kinds reaches the inner text destination,
	// which gets the text and nothing else.
	mixed := gift.DropFiles("/tmp/a.txt")
	mixed.Items = append(mixed.Items, gift.DropItem{Kind: gift.DropText, Text: "hello"})
	if !inner.DropOnto(mixed) {
		t.Fatal("not taken")
	}
	if len(s.inner) != 1 || len(s.inner[0].Items) != 1 || s.inner[0].Items[0].Text != "hello" {
		t.Errorf("inner got %+v", s.inner)
	}
}

func TestADropReportsItsLocalPosition(t *testing.T) {
	h, s := newDropScene(t)
	inner := h.Find(gifttest.ByKey("inner"))
	b := inner.Bounds()
	h.DropAt(geom.Pt(b.Min.X+7, b.Min.Y+9), gift.DropTextData("x"))
	if len(s.inner) != 1 {
		t.Fatal("no drop")
	}
	if got := s.inner[0].Location; got != geom.Pt(7, 9) {
		t.Errorf("Location = %v, want (7, 9)", got)
	}
}

func TestOnTargetedComesInPairs(t *testing.T) {
	h, s := newDropScene(t)
	press := h.Find(gifttest.ByKey("press"))
	press.DragOver(gift.DropFile)
	h.DragOverAt(geom.Pt(390, 290), gift.DropFile)
	h.DragLeave()
	press.DragOver(gift.DropFile)
	// The drag turns to text: the file destination lets it go.
	press.DragOver(gift.DropText)
	h.DragLeave()
	want := []bool{true, false, true, false}
	if len(s.targeted) != len(want) {
		t.Fatalf("OnTargeted was told %v, want %v", s.targeted, want)
	}
	for i := range want {
		if s.targeted[i] != want[i] {
			t.Fatalf("OnTargeted was told %v, want %v", s.targeted, want)
		}
	}
}

func TestADisabledDropDestinationTakesNothingAndItsContentStillWorks(t *testing.T) {
	h, s := newDropScene(t)
	s.disabled = true
	h.App().InvalidateAll()
	h.Settle()
	press := h.Find(gifttest.ByKey("press"))
	press.DragOver(gift.DropFile)
	h.Find(gifttest.ByKey("outer")).AssertNotDropTargeted()
	if press.DropOnto(gift.DropFiles("/tmp/a")) {
		t.Error("a disabled destination took a drop")
	}
	press.Click()
	if s.pressed != 1 {
		t.Error("the button inside the destination did not work")
	}
}

func TestADropDestinationLeavesTheHoverOfItsContent(t *testing.T) {
	h, s := newDropScene(t)
	press := h.Find(gifttest.ByKey("press"))
	press.Hover()
	press.AssertHovered()
	press.Click()
	if s.pressed != 1 {
		t.Error("the click did not reach the button")
	}
}

// TestADropDestinationAroundAShowcaseChangesNothing wraps a busy demo in a
// window-wide destination: the demo must still lay out without overflow and
// answer the pointer as before.
func TestADropDestinationAroundAShowcaseChangesNothing(t *testing.T) {
	d, ok := showcase.Lookup("disclosure-group")
	if !ok {
		t.Fatal("no demo disclosure-group")
	}
	h := gifttest.New(t, gifttest.Options{
		Root: func(ctx *gift.Context) gift.View {
			return ui.DropDestination(ui.Window(ui.VStack(d.View(ctx)).Padding(20))).
				OnDrop(func(ui.DropInfo) bool { return true })
		},
		Size: geom.Sz(float32(d.Width)+40, float32(d.Height)+40),
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}),
	})
	h.Settle()
	h.AssertNoOverflow()
	if !h.App().HasDropTargets() {
		t.Error("HasDropTargets is false")
	}
	if !h.DropAt(geom.Pt(30, 30), gift.DropFiles("/tmp/a")) {
		t.Error("the window-wide destination did not take the drop")
	}
}

// TestTheDropDestinationDemoLooksTheWayItLooks pins the highlight, in both
// themes, with a text drag held over the inner destination.
//
//	GIFT_UPDATE_GOLDEN=1 go test -tags giftgpu -run DropDestination ./ui
func TestTheDropDestinationDemoLooksTheWayItLooks(t *testing.T) {
	d, ok := showcase.Lookup("drop-destination")
	if !ok {
		t.Fatal("no demo drop-destination")
	}
	for _, theme := range []struct {
		name string
		t    ui.Theme
	}{{"light", ui.LightTheme()}, {"dark", ui.DarkTheme()}} {
		h := gifttest.New(t, gifttest.Options{
			Root:  func(ctx *gift.Context) gift.View { return ui.Window(ui.VStack(d.View(ctx)).Padding(20)) },
			Size:  geom.Sz(float32(d.Width)+40, float32(d.Height)+40),
			Theme: theme.t,
			Font:  ui.MustFont(ui.FontQuery{Family: inter.Family}),
		})
		h.Settle()
		h.AssertNoOverflow()
		note := h.Find(gifttest.ByText("Drop text here"))
		note.DragOver(gift.DropText)
		note.AssertDropTargeted()
		h.AssertOpaque()
		h.Warm()
		h.AssertGolden("drop-destination-targeted-" + theme.name)
	}
}

func TestABareDropTellsOnTargetedNothing(t *testing.T) {
	h, s := newDropScene(t)
	if !h.Find(gifttest.ByKey("press")).DropOnto(gift.DropFiles("/tmp/a")) {
		t.Fatal("not taken")
	}
	if len(s.targeted) != 0 {
		t.Errorf("OnTargeted was told %v by a drop with no drag before it, as on Linux", s.targeted)
	}
}
