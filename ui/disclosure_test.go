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

// paragraph is content of a known height: three lines of 20.
func paragraph() gift.View { return ui.Box().Frame(200, 60).Key("body") }

func disclosureHarness(t *testing.T, root func(*gift.Context) gift.View) *gifttest.Harness {
	t.Helper()
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(400, 600),
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family})})
	h.Settle()
	return h
}

func groupHeight(h *gifttest.Harness) float32 {
	return h.Find(gifttest.ByKey("group")).LayoutBounds().Height()
}

func TestADisclosureGroupOpensAndClosesWithItsContent(t *testing.T) {
	h := disclosureHarness(t, func(*gift.Context) gift.View {
		return ui.VStack(ui.DisclosureGroup("Details", paragraph()).Key("group")).Frame(360, 0)
	})
	closed := groupHeight(h)
	if closed != ui.RowHeight {
		t.Fatalf("closed: %v high, want a row of %v", closed, ui.RowHeight)
	}
	h.AssertNone(gifttest.ByKey("body"))

	h.Find(label("Details")).Click()
	h.AssertExists(gifttest.ByKey("body"))
	if got := groupHeight(h); got != closed {
		t.Fatalf("the content showed before the clock moved: %v high", got)
	}
	h.Advance(ui.DisclosureDuration / 2)
	half := groupHeight(h)
	open := closed + 60 + 12
	if half <= closed || half >= open {
		t.Fatalf("half way: %v high, want between %v and %v", half, closed, open)
	}
	h.Advance(ui.DisclosureDuration)
	if got := groupHeight(h); got != open {
		t.Fatalf("open: %v high, want %v", got, open)
	}
	// The content is inset as far as the title.
	if x := h.Find(gifttest.ByKey("body")).LayoutBounds().Min.X; x != ui.RowPadding {
		t.Fatalf("content at x %v, want %v", x, ui.RowPadding)
	}

	h.Find(label("Details")).Click()
	h.Advance(ui.DisclosureDuration / 2)
	h.AssertExists(gifttest.ByKey("body"))
	if got := groupHeight(h); got <= closed || got >= open {
		t.Fatalf("closing half way: %v high", got)
	}
	h.Advance(ui.DisclosureDuration)
	if got := groupHeight(h); got != closed {
		t.Fatalf("closed again: %v high", got)
	}
	h.AssertNone(gifttest.ByKey("body"))
}

func TestHalfOpenContentIsClippedForInput(t *testing.T) {
	taps := 0
	h := disclosureHarness(t, func(*gift.Context) gift.View {
		return ui.VStack(ui.DisclosureGroup("Details",
			ui.Box().Frame(200, 60),
			ui.Button(ui.Text("Lower"), func() { taps++ }).Key("lower"),
		).Key("group")).Frame(360, 0)
	})
	h.Find(label("Details")).Click()
	h.Advance(ui.DisclosureDuration / 4)
	b := h.Find(gifttest.ByKey("lower")).LayoutBounds()
	g := h.Find(gifttest.ByKey("group")).LayoutBounds()
	if b.Min.Y < g.Max.Y {
		t.Fatalf("the button at %v is already inside the group at %v", b, g)
	}
	h.ClickAt(geom.Pt((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2))
	if taps != 0 {
		t.Fatal("a tap reached content that is not shown yet")
	}
	h.Advance(ui.DisclosureDuration)
	h.Find(gifttest.ByKey("lower")).Click()
	if taps != 1 {
		t.Fatal("the shown button took no tap")
	}
}

func TestAnExpandedGroupIsTheApplicationsToOpen(t *testing.T) {
	var asked []bool
	h := disclosureHarness(t, func(ctx *gift.Context) gift.View {
		open := ctx.State("open", true)
		return ui.VStack(
			ui.DisclosureGroup("Details", paragraph()).Key("group").
				Expanded(ctx.Read(open), func(v bool) { asked = append(asked, v); open.Set(v) }),
			ui.DisclosureGroup("Fixed", paragraph()).Key("fixed").Expanded(true, func(v bool) { asked = append(asked, v) }),
		).Frame(360, 0)
	})
	// Built open: at once, without a movement.
	if got := groupHeight(h); got != ui.RowHeight+72 {
		t.Fatalf("built open: %v high", got)
	}
	h.Find(label("Details")).Click()
	h.Advance(ui.DisclosureDuration)
	if got := groupHeight(h); got != ui.RowHeight {
		t.Fatalf("closed: %v high", got)
	}
	// A group whose value does not follow the callback stays as it is.
	h.Find(label("Fixed")).Click()
	h.Advance(ui.DisclosureDuration)
	if got := h.Find(gifttest.ByKey("fixed")).LayoutBounds().Height(); got != ui.RowHeight+72 {
		t.Fatalf("fixed group: %v high", got)
	}
	if len(asked) != 2 || asked[0] || asked[1] {
		t.Fatalf("asked %v, want [false false]", asked)
	}
}

func TestADisabledGroupDoesNotOpen(t *testing.T) {
	h := disclosureHarness(t, func(*gift.Context) gift.View {
		return ui.VStack(ui.DisclosureGroup("Details", paragraph()).Key("group").Disabled(true)).Frame(360, 0)
	})
	b := h.Find(gifttest.ByKey("group")).LayoutBounds()
	h.ClickAt(geom.Pt(b.Min.X+40, b.Min.Y+b.Height()/2))
	h.Advance(ui.DisclosureDuration)
	h.AssertNone(gifttest.ByKey("body"))
}

func TestContentInsetFollowsTheIcon(t *testing.T) {
	h := disclosureHarness(t, func(*gift.Context) gift.View {
		return ui.VStack(
			ui.DisclosureGroup("Details", paragraph()).Key("group").Icon(ui.NewSymbol(testGlyph, 24)).
				Expanded(true, nil),
		).Frame(360, 0)
	})
	if x := h.Find(gifttest.ByKey("body")).LayoutBounds().Min.X; x != ui.RowPadding+22+12 {
		t.Fatalf("content after an icon at x %v", x)
	}
}

// testGlyph is a symbol of one square, for a group with an icon.
var testGlyph = []byte("M2 2h20v20H2z")

func TestAnAccordionKeepsOneGroupOpen(t *testing.T) {
	h := disclosureHarness(t, func(*gift.Context) gift.View {
		return ui.Accordion(
			ui.DisclosureGroup("One", ui.Box().Frame(10, 30).Key("one")),
			ui.DisclosureGroup("Two", ui.Box().Frame(10, 30).Key("two")),
			ui.DisclosureGroup("Three", ui.Box().Frame(10, 30).Key("three")),
		).Frame(360, geom.Unbounded())
	})
	h.AssertNone(gifttest.ByKey("one"))
	h.Find(label("One")).Click()
	h.Advance(ui.DisclosureDuration)
	h.AssertExists(gifttest.ByKey("one"))
	h.Find(label("Three")).Click()
	h.Advance(ui.DisclosureDuration)
	h.AssertNone(gifttest.ByKey("one"))
	h.AssertExists(gifttest.ByKey("three"))
	// A tap on the open one closes it.
	h.Find(label("Three")).Click()
	h.Advance(ui.DisclosureDuration)
	h.AssertNone(gifttest.ByKey("three"))
}

func TestAnAccordionsOpenGroupIsTheApplicationsToChoose(t *testing.T) {
	var asked []int
	h := disclosureHarness(t, func(ctx *gift.Context) gift.View {
		open := ctx.State("open", 1)
		return ui.Accordion(
			ui.DisclosureGroup("One", ui.Box().Frame(10, 30).Key("one")),
			ui.DisclosureGroup("Two", ui.Box().Frame(10, 30).Key("two")),
		).Open(ctx.Read(open), func(i int) { asked = append(asked, i); open.Set(i) }).Frame(360, geom.Unbounded())
	})
	h.AssertExists(gifttest.ByKey("two"))
	h.Find(label("One")).Click()
	h.Advance(ui.DisclosureDuration)
	h.Find(label("One")).Click()
	h.Advance(ui.DisclosureDuration)
	if len(asked) != 2 || asked[0] != 0 || asked[1] != -1 {
		t.Fatalf("asked %v, want [0 -1]", asked)
	}
	h.AssertNone(gifttest.ByKey("one"))
	h.AssertNone(gifttest.ByKey("two"))
}

// TestTheDisclosureDemoLooksTheWayItLooks pins the picture of the component
// page in both themes.
//
//	GIFT_UPDATE_GOLDEN=1 go test -tags giftgpu -run TestTheDisclosureDemo ./ui
func TestTheDisclosureDemoLooksTheWayItLooks(t *testing.T) {
	d, ok := showcase.Lookup("disclosure-group")
	if !ok {
		t.Fatal("no demo disclosure-group")
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
		h.AssertOpaque()
		h.Warm()
		h.AssertGolden("disclosure-group-" + theme.name)
	}
}

// label selects the label of the group titled title, the button and not
// its text.
func label(title string) gifttest.Selector {
	return gifttest.ByText(title).And(gifttest.Interactive())
}
