package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

func TestHexColoursReadAndWrite(t *testing.T) {
	for _, c := range []struct {
		in   string
		want ui.Color
	}{
		{"#FF8000", ui.RGB(255, 128, 0)},
		{"ff8000", ui.RGB(255, 128, 0)},
		{"#abc", ui.RGB(0xaa, 0xbb, 0xcc)},
		{"#FF000080", ui.RGBA(255, 0, 0, 128)},
	} {
		got, ok := ui.ParseHexColor(c.in)
		if !ok || got != c.want {
			t.Errorf("ParseHexColor(%q) = %v, %v, want %v", c.in, got, ok, c.want)
		}
	}
	for _, bad := range []string{"", "#12", "#GG0000", "#1234567"} {
		if _, ok := ui.ParseHexColor(bad); ok {
			t.Errorf("ParseHexColor(%q) accepted it", bad)
		}
	}
	if got := ui.HexColor(ui.RGBA(255, 0, 0, 128)); got != "#FF000080" {
		t.Errorf("HexColor of a half transparent red = %q", got)
	}
	if got := ui.HexColor(ui.RGB(0, 122, 255)); got != "#007AFF" {
		t.Errorf("HexColor of the accent of iOS = %q", got)
	}
}

// pickerScene is a colour picker bound to state.
func pickerScene(t *testing.T, start ui.Color) (*gifttest.Harness, *ui.Color) {
	t.Helper()
	got := new(ui.Color)
	root := func(ctx *gift.Context) gift.View {
		c := ctx.State("c", start)
		*got = ctx.Read(c)
		return ui.VStack(ui.ColorPicker(c.Get(), c.Set).Key("picker")).Padding(20)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(400, 700), Font: loadTestFont(t), Theme: ui.LightTheme()})
	h.Settle()
	return h, got
}

// mode taps the i-th of the three segments, Grid, Spectrum and Sliders.
func mode(h *gifttest.Harness, i int) {
	b := h.Find(gifttest.ByType("ui.SegmentedControl")).Bounds()
	h.TapAt(geom.Pt(b.Min.X+b.Width()*(float32(i)+0.5)/3, (b.Min.Y+b.Max.Y)/2))
	h.Settle()
}

func rgb8(c ui.Color) [4]int {
	var out [4]int
	r, g, b, a := c.R, c.G, c.B, c.A
	if a > 0 {
		r, g, b = r/a, g/a, b/a
	}
	for i, f := range []float32{r, g, b, a} {
		out[i] = int(f*255 + 0.5)
	}
	return out
}

func TestTheGridChoosesASwatch(t *testing.T) {
	h, got := pickerScene(t, ui.RGB(0, 122, 255))
	g := h.Find(gifttest.ByKey("color-grid")).Bounds()
	cw, ch := g.Width()/12, g.Height()/10
	h.TapAt(geom.Pt(g.Min.X+cw/2, g.Min.Y+ch/2))
	if c := rgb8(*got); c != [4]int{255, 255, 255, 255} {
		t.Fatalf("the top left swatch chose %v, want white", c)
	}
	h.Advance(time.Second)
	h.TapAt(geom.Pt(g.Max.X-cw/2, g.Min.Y+ch/2))
	if c := rgb8(*got); c != [4]int{0, 0, 0, 255} {
		t.Fatalf("the top right swatch chose %v, want black", c)
	}
}

func TestTheSpectrumRunsFromWhiteThroughTheHueToBlack(t *testing.T) {
	h, got := pickerScene(t, ui.RGB(0, 122, 255))
	mode(h, 1)
	s := h.Find(gifttest.ByKey("color-spectrum")).Bounds()
	h.TapAt(geom.Pt(s.Min.X+1, s.Min.Y+s.Height()/2))
	if c := rgb8(*got); c[0] < 250 || c[1] > 8 || c[2] > 8 {
		t.Fatalf("the left of the middle is %v, want red", c)
	}
	h.Advance(time.Second)
	h.TapAt(geom.Pt(s.Min.X+s.Width()/3, s.Min.Y+s.Height()/2))
	if c := rgb8(*got); c[1] < 250 || c[0] > 8 || c[2] > 8 {
		t.Fatalf("a third across the middle is %v, want green", c)
	}
	h.Advance(time.Second)
	h.TapAt(geom.Pt(s.Min.X+s.Width()/2, s.Min.Y+1))
	if c := rgb8(*got); c[0] < 250 || c[1] < 250 || c[2] < 250 {
		t.Fatalf("the top is %v, want white", c)
	}
	h.Advance(time.Second)
	h.TapAt(geom.Pt(s.Min.X+s.Width()/2, s.Max.Y-1))
	if c := rgb8(*got); c[0] > 4 || c[1] > 4 || c[2] > 4 {
		t.Fatalf("the bottom is %v, want black", c)
	}
}

func TestTheSlidersSetOneChannelEachAndOpacityTheAlpha(t *testing.T) {
	h, got := pickerScene(t, ui.RGB(10, 20, 30))
	mode(h, 2)
	red := h.Find(gifttest.ByKey("color-red")).Bounds()
	h.TapAt(geom.Pt(red.Max.X-2, (red.Min.Y+red.Max.Y)/2))
	if c := rgb8(*got); c != [4]int{255, 20, 30, 255} {
		t.Fatalf("the red slider at its end made %v", c)
	}
	h.Find(gifttest.ByKey("color-green")).Focus()
	h.Key(gift.KeyRight)
	if c := rgb8(*got); c[1] != 21 {
		t.Fatalf("a step of green made %v", c)
	}
	op := h.Find(gifttest.ByKey("color-opacity")).Bounds()
	h.Advance(time.Second)
	h.TapAt(geom.Pt(op.Min.X+op.Width()/2, (op.Min.Y+op.Max.Y)/2))
	if c := rgb8(*got); c[3] < 120 || c[3] > 135 || c[0] != 255 {
		t.Fatalf("opacity half way made %v, want half transparent and still red", c)
	}
	if v := h.Find(gifttest.ByKey("color-opacity")).Value(); v != "50 %" && v != "49 %" && v != "51 %" {
		t.Fatalf("the opacity slider reports %q", v)
	}
}

func TestTheHexFieldTakesAColourAsItIsTyped(t *testing.T) {
	h, got := pickerScene(t, ui.RGB(0, 122, 255))
	h.Find(gifttest.ByKey("color-hex")).Focus()
	h.Key(gift.KeyA, gift.ShortcutModifier)
	h.SetModifiers() // the shortcut key is let go before typing
	h.TypeText("#34C759")
	h.Settle()
	if c := rgb8(*got); c != [4]int{0x34, 0xc7, 0x59, 255} {
		t.Fatalf("typing #34C759 made %v", c)
	}
}

func TestAColorWellOpensThePickerInAPopover(t *testing.T) {
	got := new(ui.Color)
	root := func(ctx *gift.Context) gift.View {
		c := ctx.State("c", ui.RGB(255, 149, 0))
		*got = ctx.Read(c)
		return ui.VStack(ui.HStack(ui.Text("Tint").Flex(1), ui.ColorWell(c.Get(), c.Set))).Padding(20)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(500, 700), Font: loadTestFont(t), Theme: ui.LightTheme()})
	h.Settle()
	if b := h.Find(gifttest.ByKey("color-well")).LayoutBounds(); b.Width() != 28 {
		t.Fatalf("the well is %v", b)
	}
	h.AssertNone(gifttest.ByKey("color-grid"))
	h.Find(gifttest.ByKey("color-well")).Tap()
	h.AdvanceTicks(30, gifttest.FrameInterval)
	g := h.Find(gifttest.ByKey("color-grid")).Bounds()
	h.TapAt(geom.Pt(g.Max.X-5, g.Min.Y+5))
	if c := rgb8(*got); c != [4]int{0, 0, 0, 255} {
		t.Fatalf("a swatch in the popover chose %v", c)
	}
	h.TapAt(geom.Pt(490, 690))
	h.AdvanceTicks(30, gifttest.FrameInterval)
	h.AssertNone(gifttest.ByKey("color-grid"))
}

// TestADragPastTheEdgeKeepsTheColourAtTheEdge drags over the spectrum and
// the red slider of a picker in a scroll view, out past their edges. The
// colour stays at the edge, and the page does not move: a move outside used
// to go up to the scroll view, which took the pointer and scrolled instead.
func TestADragPastTheEdgeKeepsTheColourAtTheEdge(t *testing.T) {
	got := new(ui.Color)
	root := func(ctx *gift.Context) gift.View {
		c := ctx.State("c", ui.RGB(10, 20, 30))
		*got = ctx.Read(c)
		return ui.VScroll(ui.ColorPicker(c.Get(), c.Set).Key("picker"), ui.ZStack().Frame(10, 900)).Padding(20)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(400, 700), Font: loadTestFont(t), Theme: ui.LightTheme()})
	h.Settle()
	mode(h, 1)
	s := h.Find(gifttest.ByKey("color-spectrum")).Bounds()
	mid := geom.Pt(s.Min.X+s.Width()/2, s.Min.Y+s.Height()/2)

	// The mouse down and out past the bottom, which is black.
	h.PressAt(mid)
	h.MoveTo(mid.Add(geom.Pt(0, 20)))
	h.MoveTo(mid.Add(geom.Pt(0, s.Height()/2+40)))
	h.MoveTo(mid.Add(geom.Pt(0, s.Height()/2+160)))
	h.Release()
	if b := h.Find(gifttest.ByKey("color-spectrum")).Bounds(); b != s {
		t.Fatalf("the mouse moved the spectrum from %v to %v", s, b)
	}
	if c := rgb8(*got); c[0] > 4 || c[1] > 4 || c[2] > 4 {
		t.Fatalf("the mouse down past the edge made %v, want black", c)
	}

	// A finger up and out past the top, which is white.
	h.Advance(time.Second)
	h.TouchDownAt(mid)
	h.TouchMoveTo(mid.Add(geom.Pt(0, -20)))
	h.TouchMoveTo(mid.Add(geom.Pt(0, -s.Height()/2-40)))
	h.TouchMoveTo(mid.Add(geom.Pt(0, -s.Height()/2-160)))
	h.TouchUpAt(mid.Add(geom.Pt(0, -s.Height()/2-160)))
	if b := h.Find(gifttest.ByKey("color-spectrum")).Bounds(); b != s {
		t.Fatalf("the finger moved the spectrum from %v to %v", s, b)
	}
	if c := rgb8(*got); c[0] < 250 || c[1] < 250 || c[2] < 250 {
		t.Fatalf("the finger up past the edge made %v, want white", c)
	}

	// The mouse along red and out past its end, and below the slider.
	h.Advance(time.Second)
	mode(h, 2)
	red := h.Find(gifttest.ByKey("color-red")).Bounds()
	at := geom.Pt(red.Min.X+red.Width()/2, (red.Min.Y+red.Max.Y)/2)
	h.PressAt(at)
	h.MoveTo(at.Add(geom.Pt(30, 0)))
	h.MoveTo(geom.Pt(red.Max.X+60, red.Max.Y+120))
	h.Release()
	if c := rgb8(*got); c[0] != 255 {
		t.Fatalf("a drag of red past its end made %v", c)
	}
}
