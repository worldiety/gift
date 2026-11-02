package svg

import (
	"image"
	"math"
	"slices"
	"testing"
)

func mustParse(t *testing.T, src string) *Image {
	t.Helper()
	im, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return im
}

// px returns the pixel at x, y, unpremultiplied to 0–255.
func px(img *image.RGBA, x, y int) [4]int {
	c := img.RGBAAt(x, y)
	if c.A == 0 {
		return [4]int{}
	}
	un := func(v uint8) int { return int(math.Round(float64(v) * 255 / float64(c.A))) }
	return [4]int{un(c.R), un(c.G), un(c.B), int(c.A)}
}

func close4(a, b [4]int, tol int) bool {
	for i := range a {
		if d := a[i] - b[i]; d > tol || d < -tol {
			return false
		}
	}
	return true
}

func TestColoursLengthsAndTransformsAreRead(t *testing.T) {
	for s, want := range map[string]rgba{
		"#f00":                 {1, 0, 0, 1},
		"#00ff0080":            {0, 1, 0, 128.0 / 255},
		"rgb(0, 0, 255)":       {0, 0, 1, 1},
		"rgba(100%,0%,0%,0.5)": {1, 0, 0, 0.5},
		"RebeccaPurple":        {0x66 / 255.0, 0x33 / 255.0, 0x99 / 255.0, 1},
	} {
		if got, ok := parseColor(s); !ok || got != want {
			t.Errorf("%q is %v %v", s, got, ok)
		}
	}
	if _, ok := parseColor("blurple"); ok {
		t.Error("an unknown name is a colour")
	}
	if l, ok := parseLength("1in", 0); !ok || l != 96 {
		t.Errorf("1in is %v", l)
	}
	if l, ok := parseLength("50%", 300); !ok || l != 150 {
		t.Errorf("50%% of 300 is %v", l)
	}
	m, ok := parseTransform("translate(10, 20) rotate(90 5 5)")
	if !ok {
		t.Fatal("transform")
	}
	if x, y := m.apply(10, 5); math.Abs(x-15) > 1e-9 || math.Abs(y-30) > 1e-9 {
		t.Errorf("rotated about 5,5 and moved: %v %v", x, y)
	}
}

func TestPresentationIsInheritedAndStyleWins(t *testing.T) {
	im := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">
		<g fill="red" stroke="blue" stroke-width="2">
			<rect width="5" height="5" style="fill: #00ff00"/>
			<circle r="2" cx="7" cy="7"/>
			<rect width="1" height="1" display="none"/>
		</g>
	</svg>`)
	if len(im.shapes) != 2 {
		t.Fatalf("%d shapes", len(im.shapes))
	}
	if c := im.shapes[0].fill.color; c != (rgba{0, 1, 0, 1}) {
		t.Errorf("the style of the rect is %v", c)
	}
	if c := im.shapes[1].fill.color; c != (rgba{1, 0, 0, 1}) || im.shapes[1].width != 2 {
		t.Errorf("the circle inherits %v %v", c, im.shapes[1].width)
	}
}

func TestUseDefsAndGradientsResolve(t *testing.T) {
	im := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 10 10">
		<defs>
			<linearGradient id="base"><stop offset="0" stop-color="red"/><stop offset="100%" stop-color="blue"/></linearGradient>
			<linearGradient id="g" xlink:href="#base" x2="0" y2="1"/>
			<rect id="r" width="2" height="2"/>
		</defs>
		<use xlink:href="#r" x="3" y="4" fill="url(#g)"/>
		<text x="1" y="1">no</text>
	</svg>`)
	if len(im.shapes) != 1 {
		t.Fatalf("%d shapes", len(im.shapes))
	}
	s := im.shapes[0]
	if s.fill.kind != paintGradient || len(s.fill.grad.stops) != 2 || s.fill.grad.y2 != 1 || s.fill.grad.x2 != 0 {
		t.Fatalf("the gradient is %+v", s.fill.grad)
	}
	if x, y := s.m.apply(0, 0); x != 3 || y != 4 {
		t.Errorf("use at %v %v", x, y)
	}
	if !slices.Contains(im.Skipped(), "text") {
		t.Errorf("skipped %v", im.Skipped())
	}
}

func TestTheSizeComesFromTheAttributesOrTheViewBox(t *testing.T) {
	for src, want := range map[string][2]float64{
		`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="32" viewBox="0 0 10 10"/>`: {64, 32},
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 30 20"/>`:                        {30, 20},
		`<svg xmlns="http://www.w3.org/2000/svg" width="60" viewBox="0 0 30 20"/>`:             {60, 40},
		`<svg xmlns="http://www.w3.org/2000/svg"/>`:                                            {300, 150},
	} {
		im := mustParse(t, src)
		if w, h := im.Size(); w != want[0] || h != want[1] {
			t.Errorf("%s is %v by %v", src, w, h)
		}
	}
	if _, err := Parse([]byte(`<html/>`)); err == nil {
		t.Error("an html document parses")
	}
}

func TestFillsAreAntialiasedAndInPlace(t *testing.T) {
	im := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20">
		<rect x="5" y="5" width="10" height="10" fill="#ff0000"/>
		<circle cx="10" cy="10" r="3" fill="#0000ff" fill-opacity="0.5"/>
	</svg>`)
	img := im.Rasterize(20, 20)
	if got := px(img, 6, 6); !close4(got, [4]int{255, 0, 0, 255}, 1) {
		t.Errorf("inside the rect %v", got)
	}
	if got := px(img, 2, 2); got[3] != 0 {
		t.Errorf("outside %v", got)
	}
	// Half blue over red.
	if got := px(img, 10, 10); !close4(got, [4]int{128, 0, 127, 255}, 2) {
		t.Errorf("the middle %v", got)
	}
	// A circle's edge is partly covered.
	edge := img.RGBAAt(12, 7).B
	if edge == 0 || edge >= 127 {
		t.Errorf("the rim of the circle has blue %v", edge)
	}
}

func TestStrokesHaveTheirWidthAndJoins(t *testing.T) {
	im := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40">
		<polyline points="5,35 20,5 35,35" fill="none" stroke="black" stroke-width="4" stroke-linejoin="miter"/>
	</svg>`)
	img := im.Rasterize(40, 40)
	if a := img.RGBAAt(20, 6).A; a < 250 {
		t.Errorf("the corner has %v", a)
	}
	// A miter reaches above the corner; the stroke is 4 wide.
	if a := img.RGBAAt(20, 2).A; a == 0 {
		t.Error("no miter tip above the corner")
	}
	if a := img.RGBAAt(20, 20).A; a != 0 {
		t.Errorf("inside the open polyline %v", a)
	}
	// Overlapping stamps do not add up: a half transparent stroke stays half.
	im = mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40">
		<polyline points="5,20 20,20 20,35" fill="none" stroke="black" stroke-opacity="0.5" stroke-width="6" stroke-linejoin="round"/>
	</svg>`)
	img = im.Rasterize(40, 40)
	if a := img.RGBAAt(20, 20).A; a < 120 || a > 135 {
		t.Errorf("a half transparent corner has %v", a)
	}
}

func TestAnEvenOddHoleStaysEmpty(t *testing.T) {
	im := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 30 30">
		<path fill-rule="evenodd" d="M2 2h26v26h-26z M10 10h10v10h-10z"/>
	</svg>`)
	img := im.Rasterize(30, 30)
	if a := img.RGBAAt(15, 15).A; a != 0 {
		t.Errorf("the hole has %v", a)
	}
	if a := img.RGBAAt(5, 5).A; a != 255 {
		t.Errorf("the frame has %v", a)
	}
	if len(im.Skipped()) != 0 {
		t.Errorf("skipped %v", im.Skipped())
	}
}

func TestGradientsAndClipsPaint(t *testing.T) {
	im := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 10">
		<defs>
			<linearGradient id="g"><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient>
			<clipPath id="c"><rect width="50" height="10"/></clipPath>
		</defs>
		<rect width="100" height="10" fill="url(#g)" clip-path="url(#c)"/>
	</svg>`)
	img := im.Rasterize(100, 10)
	if got := px(img, 25, 5); !close4(got, [4]int{64, 64, 64, 255}, 3) {
		t.Errorf("a quarter of the way %v", got)
	}
	if a := img.RGBAAt(75, 5).A; a != 0 {
		t.Errorf("beyond the clip %v", a)
	}
	im = mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20">
		<radialGradient id="r"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></radialGradient>
		<rect width="20" height="20" fill="url(#r)"/>
	</svg>`)
	img = im.Rasterize(20, 20)
	if c, e := px(img, 10, 10), px(img, 10, 1); c[0] < 230 || e[0] > 40 {
		t.Errorf("the radial gradient is %v in the middle and %v at the edge", c, e)
	}
}

func TestTheViewBoxIsFittedAsTheDocumentSays(t *testing.T) {
	im := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`)
	img := im.Rasterize(40, 20)
	if img.RGBAAt(5, 10).A != 0 || img.RGBAAt(20, 10).A != 255 || img.RGBAAt(35, 10).A != 0 {
		t.Error("a square viewBox is not centred in a wide picture")
	}
	im = mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" preserveAspectRatio="none"><rect width="10" height="10"/></svg>`)
	if img := im.Rasterize(40, 20); img.RGBAAt(2, 10).A != 255 {
		t.Error("none does not stretch")
	}
}

func TestATintKeepsOnlyTheOpacity(t *testing.T) {
	im := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">
		<rect width="5" height="10" fill="red"/><rect x="5" width="5" height="10" fill="lime" fill-opacity="0.5"/>
	</svg>`)
	img := im.RasterizeTinted(10, 10, 0, 0, 1, 1)
	if got := px(img, 2, 5); !close4(got, [4]int{0, 0, 255, 255}, 1) {
		t.Errorf("tinted red %v", got)
	}
	if got := px(img, 7, 5); !close4(got, [4]int{0, 0, 255, 128}, 2) {
		t.Errorf("tinted half lime %v", got)
	}
}

func TestDashesLeaveGaps(t *testing.T) {
	im := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 10">
		<line x1="0" y1="5" x2="40" y2="5" stroke="black" stroke-width="2" stroke-dasharray="10 10"/>
	</svg>`)
	img := im.Rasterize(40, 10)
	if img.RGBAAt(5, 5).A == 0 || img.RGBAAt(15, 5).A != 0 || img.RGBAAt(25, 5).A == 0 {
		t.Error("the dashes are not where they belong")
	}
}

func TestBrokenInputDoesNotBreakTheDrawing(t *testing.T) {
	im := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">
		<path d="M1 1 L9 1 L9 9 Z L x"/>
		<rect width="1e300" height="5" transform="rotate(nonsense)"/>
		<circle r="-3"/>
	</svg>`)
	img := im.Rasterize(10, 10)
	if img.RGBAAt(8, 3).A == 0 {
		t.Error("the well formed beginning of a path is not drawn")
	}
	if !slices.Contains(im.Skipped(), "malformed path data") || !slices.Contains(im.Skipped(), "transform") {
		t.Errorf("skipped %v", im.Skipped())
	}
	if b := im.Rasterize(1e9, -5).Bounds(); b.Dx() != MaxExtent || b.Dy() != 1 {
		t.Errorf("a picture of %v", b)
	}
}
