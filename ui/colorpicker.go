package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var (
	colorPickerType   = gift.RegisterType("ui.ColorPicker")
	colorGridType     = gift.RegisterType("ui.colorGrid")
	colorSpectrumType = gift.RegisterType("ui.colorSpectrum")
	colorSliderType   = gift.RegisterType("ui.colorSlider")
	colorSwatchType   = gift.RegisterType("ui.colorSwatch")
)

// Metrics of a colour picker: those of UIColorPickerViewController, scaled
// to a panel 320 points wide.
const (
	pickerWidth   = float32(320)
	pickerArea    = float32(266) // the height of the grid and the spectrum
	pickerRadius  = float32(8)
	pickerTrack   = float32(30)
	pickerColumns = 12
	pickerRows    = 10
)

// --- colour arithmetic ---------------------------------------------------------

// straight returns the colour channels of c without its premultiplied alpha,
// and the alpha.
func straight(c Color) (r, g, b, a float32) {
	if c.A <= 0 {
		return 0, 0, 0, 0
	}
	return min(c.R/c.A, 1), min(c.G/c.A, 1), min(c.B/c.A, 1), c.A
}

// fromStraight is the premultiplied colour of straight channels.
func fromStraight(r, g, b, a float32) Color {
	a = max(0, min(a, 1))
	return Color{R: max(0, min(r, 1)) * a, G: max(0, min(g, 1)) * a, B: max(0, min(b, 1)) * a, A: a}
}

// hsb is the hue in degrees, the saturation and the brightness of straight
// channels.
func hsb(r, g, b float32) (h, s, v float32) {
	mx, mn := max(r, g, b), min(r, g, b)
	v = mx
	if mx > 0 {
		s = (mx - mn) / mx
	}
	d := mx - mn
	switch {
	case d == 0:
		h = 0
	case mx == r:
		h = 60 * float32(math.Mod(float64((g-b)/d), 6))
	case mx == g:
		h = 60 * ((b-r)/d + 2)
	default:
		h = 60 * ((r-g)/d + 4)
	}
	if h < 0 {
		h += 360
	}
	return h, s, v
}

// fromHSB is the inverse of [hsb].
func fromHSB(h, s, v float32) (r, g, b float32) {
	h = float32(math.Mod(float64(h), 360))
	if h < 0 {
		h += 360
	}
	c := v * s
	x := c * (1 - float32(math.Abs(math.Mod(float64(h/60), 2)-1)))
	m := v - c
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}

func byte8(f float32) int { return int(math.Round(float64(max(0, min(f, 1)) * 255))) }

// HexColor is c as "#RRGGBB", or "#RRGGBBAA" when it is not opaque, in
// straight alpha.
func HexColor(c Color) string {
	r, g, b, a := straight(c)
	s := "#"
	for _, f := range []float32{r, g, b} {
		s += hex2(byte8(f))
	}
	if byte8(a) < 255 {
		s += hex2(byte8(a))
	}
	return s
}

func hex2(v int) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[v>>4], digits[v&15]})
}

// ParseHexColor reads "#RGB", "#RRGGBB" or "#RRGGBBAA", with or without the
// hash, in straight alpha, and reports whether it was one.
func ParseHexColor(s string) (Color, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 && len(s) != 8 {
		return Color{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return Color{}, false
	}
	a := uint64(255)
	if len(s) == 8 {
		a, v = v&0xff, v>>8
	}
	return RGBA(uint8(v>>16), uint8(v>>8), uint8(v), uint8(a)), true
}

// sameColor reports whether a and b are the same colour to eight bits.
func sameColor(a, b Color) bool {
	r1, g1, b1, a1 := straight(a)
	r2, g2, b2, a2 := straight(b)
	return byte8(r1) == byte8(r2) && byte8(g1) == byte8(g2) && byte8(b1) == byte8(b2) && byte8(a1) == byte8(a2)
}

// --- the picker -------------------------------------------------------------------

// ColorPickerView chooses a colour. It is created by [ColorPicker].
type ColorPickerView struct {
	value    Color
	onChange func(Color)
	noAlpha  bool
	bg       Color
	hasBg    bool
	key      string
	label    string
	flex     float32
}

// ColorPicker returns the colour picker of iOS, UIColorPickerViewController,
// as a panel 320 points wide: a grid of swatches, a spectrum of every hue and
// sliders for red, green and blue, one at a time under a segmented control,
// with an opacity slider, the colour and its hex code below them.
//
//	c := ctx.State("c", ui.RGB(0, 122, 255))
//	ui.ColorPicker(ctx.Read(c), c.Set)
//
// It is controlled like a [Slider]: it shows the colour it is given and
// reports a new one to onChange, with every move of a finger over the
// spectrum or a slider. Read the colour with [gift.Context.Read], not with
// [gift.State.Get]: only a build that read the state runs again when it
// changes, and a picker nobody builds again shows the old colour, as if it
// took no tap. The hex field takes "#RRGGBB" and "#RRGGBBAA" as they are
// typed. [ColorWell] puts the picker in a popover behind a swatch.
func ColorPicker(value Color, onChange func(Color)) ColorPickerView {
	return ColorPickerView{value: value, onChange: onChange}
}

// SupportsOpacity with false hides the opacity slider and keeps every colour
// opaque, as supportsAlpha does on iOS.
func (v ColorPickerView) SupportsOpacity(b bool) ColorPickerView { v.noAlpha = !b; return v }

// Background sets the colour the picker stands on, which rounds the corners
// of its grid and its spectrum; the default is [ColorSurface], the colour of
// a popover.
func (v ColorPickerView) Background(c Color) ColorPickerView { v.bg, v.hasBg = c, true; return v }

// Label sets the accessible name.
func (v ColorPickerView) Label(s string) ColorPickerView { v.label = s; return v }

// Key sets the reconciliation key.
func (v ColorPickerView) Key(s string) ColorPickerView { v.key = s; return v }

// Flex sets the flex factor in a stack.
func (v ColorPickerView) Flex(f float32) ColorPickerView { v.flex = f; return v }

// ViewType implements gift.View.
func (v ColorPickerView) ViewType() gift.TypeID { return colorPickerType }

// Build implements gift.View.
func (v ColorPickerView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Flex: v.flex, Label: v.label, Layouter: anchorLayout{}, Children: []gift.View{gift.Component("ui.colorPicker", v.body)}}
}

func (v ColorPickerView) set(c Color) {
	if v.noAlpha {
		r, g, b, _ := straight(c)
		c = fromStraight(r, g, b, 1)
	}
	if v.onChange != nil && !sameColor(c, v.value) {
		v.onChange(c)
	}
}

func (v ColorPickerView) body(ctx *gift.Context) gift.View {
	mode := ctx.State("mode", 0)
	ctx.Read(mode)
	bg := Color(ColorSurface)
	if v.hasBg {
		bg = v.bg
	}
	bgc := ResolveColor(bg)

	// The hex field follows the colour, unless the colour is what the field
	// says already, which is the case while somebody types into it.
	want := HexColor(v.value)
	hex := Editor(ctx, "hex", want)
	synced := ctx.State("synced", want)
	if want != synced.Get() {
		synced.Set(want)
		if c, ok := ParseHexColor(hex.Text()); !ok || !sameColor(c, v.value) {
			hex.SetText(want)
		}
	}
	typed := func(s string) {
		if c, ok := ParseHexColor(s); ok && (len(strings.TrimPrefix(s, "#")) >= 6) {
			v.set(c)
		}
	}

	var content gift.View
	switch mode.Get() {
	case 1:
		content = colorSpectrum{v: v, bg: bgc}
	case 2:
		content = VStack(
			channelRow("RED", colorSlider{v: v, channel: 0}),
			channelRow("GREEN", colorSlider{v: v, channel: 1}),
			channelRow("BLUE", colorSlider{v: v, channel: 2}),
		).Gap(14).Frame(pickerWidth, pickerArea).Align(geom.Alignment{X: 0, Y: 0})
	default:
		content = colorGrid{v: v, bg: bgc}
	}
	kids := []gift.View{
		SegmentedControl(mode.Get(), []string{"Grid", "Spectrum", "Sliders"}, mode.Set).Frame(pickerWidth, geom.Unbounded()),
		content,
	}
	if !v.noAlpha {
		kids = append(kids, channelRow("OPACITY", colorSlider{v: v, channel: 3}))
	}
	kids = append(kids, HStack(
		colorSwatch{c: v.value, size: 44},
		Spacer(),
		Text("Hex").FontSize(15).Foreground(ColorSecondaryLabel),
		TextField(hex).OnChange(typed).OnSubmit(typed).Frame(124, geom.Unbounded()).Key("color-hex"),
	).Gap(10).Align(geom.Center).Frame(pickerWidth, geom.Unbounded()))
	return VStack(kids...).Gap(16).Align(geom.Alignment{X: 0, Y: 0}).Frame(pickerWidth, geom.Unbounded())
}

// channelRow is a slider under its name, with its value beside it.
func channelRow(name string, s colorSlider) gift.View {
	return VStack(
		Text(name).FontSize(12).Foreground(ColorSecondaryLabel),
		HStack(s.Key("color-"+strings.ToLower(name)), Text(s.valueText()).FontSize(15).MaxLines(1).Frame(44, geom.Unbounded())).
			Gap(10).Align(geom.Center),
	).Gap(6).Align(geom.Alignment{X: 0, Y: 0}).Frame(pickerWidth, geom.Unbounded())
}

// paintCornerMask covers what lies outside the rounded rectangle r with the
// colour bg: a ring wide enough to reach past the corners, whose inner
// contour is r with its radius. It is how a grid of square cells or a pile of
// gradients gets round corners without a rounded clip.
func paintCornerMask(ctx *gift.PaintContext, r geom.Rect, radius float32, bg Color) {
	w := radius + 2
	ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: r.Outset(geom.InsetsAll(w)), Color: bg, CornerRadius: radius + w, StrokeWidth: w})
}

// --- grid -------------------------------------------------------------------------

// gridColour is the swatch in column col and row row of the grid of iOS:
// greys from white to black in the first row, then for each of twelve hues
// from dark to pale.
func gridColour(col, row int) Color {
	if row == 0 {
		v := 1 - float32(col)/(pickerColumns-1)
		return fromStraight(v, v, v, 1)
	}
	hues := [pickerColumns]float32{196, 214, 232, 262, 292, 330, 2, 16, 30, 44, 54, 80}
	shades := [pickerRows - 1][2]float32{
		{1, 0.30}, {1, 0.45}, {1, 0.62}, {1, 0.80}, {0.92, 1}, {0.72, 1}, {0.52, 1}, {0.34, 1}, {0.18, 1},
	}
	sv := shades[row-1]
	r, g, b := fromHSB(hues[col], sv[0], sv[1])
	return fromStraight(r, g, b, 1)
}

type colorGrid struct {
	v  ColorPickerView
	bg Color
}

func (colorGrid) ViewType() gift.TypeID { return colorGridType }

func (g colorGrid) Build(*gift.BuildContext) gift.Element {
	n := &colorGridNode{g: g}
	return gift.Element{Key: "color-grid", Layouter: n, Painter: n, Interactor: n}
}

type colorGridNode struct{ g colorGrid }

func (n *colorGridNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(geom.Sz(pickerWidth, pickerArea))
}

func (n *colorGridNode) cell(b geom.Rect, col, row int) geom.Rect {
	cw, ch := b.Width()/pickerColumns, b.Height()/pickerRows
	return geom.Rc(b.Min.X+float32(col)*cw, b.Min.Y+float32(row)*ch, b.Min.X+float32(col+1)*cw, b.Min.Y+float32(row+1)*ch)
}

func (n *colorGridNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	_, _, _, a := straight(n.g.v.value)
	sel := -1
	for row := range pickerRows {
		for col := range pickerColumns {
			c := gridColour(col, row)
			// A cell overlaps its neighbours by a hair, so that no seam of
			// the background shows between two swatches.
			ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: n.cell(b, col, row).Outset(geom.InsetsAll(0.25)), Color: c})
			r, gg, bb, _ := straight(c)
			if sameColor(fromStraight(r, gg, bb, a), n.g.v.value) {
				sel = row*pickerColumns + col
			}
		}
	}
	paintCornerMask(ctx, b, pickerRadius, n.g.bg)
	if sel >= 0 {
		c := n.cell(b, sel%pickerColumns, sel/pickerColumns)
		ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: c.Outset(geom.InsetsAll(1)), Color: RGBA(0, 0, 0, 90), CornerRadius: 4, StrokeWidth: 1})
		ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: c, Color: RGB(255, 255, 255), CornerRadius: 3, StrokeWidth: 3})
	}
}

func (n *colorGridNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	handled, follow := pickerPointer(ctx, e)
	if follow {
		b := ctx.DeviceBounds()
		col := int((e.Pos.X - b.Min.X) / b.Width() * pickerColumns)
		row := int((e.Pos.Y - b.Min.Y) / b.Height() * pickerRows)
		col, row = max(0, min(col, pickerColumns-1)), max(0, min(row, pickerRows-1))
		r, g, bb, _ := straight(gridColour(col, row))
		_, _, _, a := straight(n.g.v.value)
		if a == 0 {
			a = 1
		}
		n.g.v.set(fromStraight(r, g, bb, a))
	}
	return handled
}

// pickerPointer is the pointer half of the grid, the spectrum and a channel
// slider. It reports whether e belongs to a press the area took, and whether
// the colour follows e.Pos: on the press and on every move until the release,
// wherever the pointer has wandered. The press is kept in the control state,
// as [Slider] keeps it, and not read from Interaction().Pressed, which clears
// when the pointer leaves the area: the move then went up to an enclosing
// scroll view, which took the pointer, so that a drag past the edge of the
// spectrum scrolled the page instead of holding the colour at the edge.
func pickerPointer(ctx *gift.EventContext, e gift.Event) (handled, follow bool) {
	st := ctx.ControlState()
	switch e.Kind {
	case gift.EventPointerDown:
		st.Grabbed = true
		ctx.SetControlState(st)
		return true, true
	case gift.EventPointerMove:
		if !st.Grabbed {
			return false, false
		}
		ctx.StealPointer()
		return true, true
	case gift.EventPointerUp, gift.EventPointerCancel:
		if !st.Grabbed {
			return false, false
		}
		st.Grabbed = false
		ctx.SetControlState(st)
		return true, false
	}
	return false, false
}

// --- spectrum ---------------------------------------------------------------------

type colorSpectrum struct {
	v  ColorPickerView
	bg Color
}

func (colorSpectrum) ViewType() gift.TypeID { return colorSpectrumType }

func (s colorSpectrum) Build(*gift.BuildContext) gift.Element {
	n := &colorSpectrumNode{s: s}
	return gift.Element{Key: "color-spectrum", Layouter: n, Painter: n, Interactor: n}
}

type colorSpectrumNode struct{ s colorSpectrum }

func (n *colorSpectrumNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(geom.Sz(pickerWidth, pickerArea))
}

// spectrumAt is the colour at the fractions fx across and fy down the
// spectrum: the hue across, and down it from white through the full colour
// in the middle to black.
func spectrumAt(fx, fy float32) (r, g, b float32) {
	r, g, b = fromHSB(360*fx, 1, 1)
	if fy < 0.5 {
		t := fy * 2
		return 1 - t + r*t, 1 - t + g*t, 1 - t + b*t
	}
	t := (fy - 0.5) * 2
	return r * (1 - t), g * (1 - t), b * (1 - t)
}

// spectrumPos is where c shows on the spectrum, as fractions: exactly for the
// colours it has, those of full brightness or full saturation, and by its
// hue and the nearer of the two for every other.
func spectrumPos(c Color) (fx, fy float32) {
	r, g, b, _ := straight(c)
	h, s, v := hsb(r, g, b)
	if v >= s {
		return h / 360, s / 2
	}
	return h / 360, 0.5 + (1-v)/2
}

func (n *colorSpectrumNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	// Six bands of hue, each a straight line in RGB between two of the
	// primaries and secondaries, which is exactly what hue is.
	w := b.Width() / 6
	for i := range 6 {
		r0, g0, b0 := fromHSB(float32(i)*60, 1, 1)
		r1, g1, b1 := fromHSB(float32(i+1)*60, 1, 1)
		from, to := fromStraight(r0, g0, b0, 1), fromStraight(r1, g1, b1, 1)
		x0 := b.Min.X + float32(i)*w
		ctx.Add(render.Op{
			Kind: render.OpFillRect, Bounds: geom.Rc(x0-0.25, b.Min.Y, x0+w+0.25, b.Max.Y), Color: from,
			Material: ctx.AddMaterial(render.HorizontalGradient(from, to).Material()),
		})
	}
	mid := (b.Min.Y + b.Max.Y) / 2
	white := RGB(255, 255, 255)
	black := RGB(0, 0, 0)
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(b.Min.X, b.Min.Y, b.Max.X, mid), Color: white,
		Material: ctx.AddMaterial(render.LinearGradient(white, Color{}).Material())})
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(b.Min.X, mid, b.Max.X, b.Max.Y), Color: Color{},
		Material: ctx.AddMaterial(render.LinearGradient(Color{}, black).Material())})
	paintCornerMask(ctx, b, pickerRadius, n.s.bg)

	// The loupe: the colour in a white ring, with a shadow to lift it.
	fx, fy := spectrumPos(n.s.v.value)
	p := geom.Pt(b.Min.X+fx*b.Width(), b.Min.Y+fy*b.Height())
	const k = 14
	disc := geom.Rc(p.X-k, p.Y-k, p.X+k, p.Y+k)
	r, g, bb, _ := straight(n.s.v.value)
	ctx.Add(render.Op{Kind: render.OpShadow, Bounds: disc.Translate(geom.Pt(0, 1)), Color: RGBA(0, 0, 0, 70), CornerRadius: k, Blur: 6})
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: disc, Color: fromStraight(r, g, bb, 1), CornerRadius: k})
	ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: disc, Color: white, CornerRadius: k, StrokeWidth: 3})
}

func (n *colorSpectrumNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	handled, follow := pickerPointer(ctx, e)
	if follow {
		b := ctx.DeviceBounds()
		fx := max(0, min((e.Pos.X-b.Min.X)/b.Width(), 0.9999))
		fy := max(0, min((e.Pos.Y-b.Min.Y)/b.Height(), 1))
		r, g, bb := spectrumAt(fx, fy)
		_, _, _, a := straight(n.s.v.value)
		if a == 0 {
			a = 1
		}
		n.s.v.set(fromStraight(r, g, bb, a))
	}
	return handled
}

// --- channel slider -----------------------------------------------------------------

// colorSlider sets one channel of the colour, red, green, blue or alpha, on
// a track that shows the colour with that channel from none to full.
type colorSlider struct {
	v       ColorPickerView
	channel int
	key     string
}

func (colorSlider) ViewType() gift.TypeID { return colorSliderType }

func (s colorSlider) Key(k string) colorSlider { s.key = k; return s }

// Flex reports that a channel slider fills its row.
func (colorSlider) baseFlex() float32 { return 1 }

func (s colorSlider) channelValue() float32 {
	r, g, b, a := straight(s.v.value)
	return [4]float32{r, g, b, a}[s.channel]
}

func (s colorSlider) valueText() string {
	f := s.channelValue()
	if s.channel == 3 {
		return strconv.Itoa(int(math.Round(float64(f*100)))) + " %"
	}
	return strconv.Itoa(byte8(f))
}

// with is the colour with the channel set to f.
func (s colorSlider) with(f float32) Color {
	r, g, b, a := straight(s.v.value)
	ch := [4]float32{r, g, b, a}
	if s.v.value.A <= 0 && s.channel != 3 {
		ch[3] = 1
	}
	ch[s.channel] = max(0, min(f, 1))
	return fromStraight(ch[0], ch[1], ch[2], ch[3])
}

func (s colorSlider) Build(*gift.BuildContext) gift.Element {
	n := &colorSliderNode{s: s, knob: ResolveColor(defaultSliderKnob), border: resolveBorder(defaultSliderKnobBorder),
		focusRing: resolveBorder(defaultSliderFocusRing)}
	return gift.Element{Key: s.key, Flex: 1, Layouter: n, Painter: n, Interactor: n, Focusable: true}
}

type colorSliderNode struct {
	s                 colorSlider
	knob              Color
	border, focusRing Border
}

func (n *colorSliderNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	w := c.Max.W
	if !isFinite(w) {
		w = 240
	}
	return c.Constrain(geom.Sz(w, pickerTrack))
}

func (n *colorSliderNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	r := b.Height() / 2
	if n.s.channel == 3 {
		paintChecker(ctx, b, r)
	}
	from, to := n.s.with(0), n.s.with(1)
	if n.s.channel != 3 {
		// The colour channels on an opaque track, so that a translucent
		// colour does not make them hard to read.
		r0, g0, b0, _ := straight(from)
		r1, g1, b1, _ := straight(to)
		from, to = fromStraight(r0, g0, b0, 1), fromStraight(r1, g1, b1, 1)
	}
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: from, CornerRadius: r,
		Material: ctx.AddMaterial(render.HorizontalGradient(from, to).Material())})
	// The knob of a slider, inside the track.
	k := r - 3
	x := b.Min.X + r + n.s.channelValue()*(b.Width()-2*r)
	knob := geom.Rc(x-k, b.Min.Y+3, x+k, b.Max.Y-3)
	ctx.Add(render.Op{Kind: render.OpShadow, Bounds: knob.Translate(geom.Pt(0, 1)), Color: RGBA(0, 0, 0, 50), CornerRadius: k, Blur: 4})
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: knob, Color: n.knob, CornerRadius: k})
	paintBorder(ctx, styleSpec{border: n.border, radius: k}, knob)
	if ia := ctx.Interaction(); ia.FocusVisible {
		paintBorder(ctx, styleSpec{border: n.focusRing, radius: r}, b)
	}
}

// paintChecker fills b with the grey checkerboard that shows transparency:
// squares between the two round ends and a plain grey in the ends, which the
// gradient then covers.
func paintChecker(ctx *gift.PaintContext, b geom.Rect, r float32) {
	light, dark := RGB(255, 255, 255), RGB(204, 204, 204)
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: light, CornerRadius: r})
	const sq = float32(7.5)
	for y, row := b.Min.Y, 0; y < b.Max.Y-0.5; y, row = y+sq, row+1 {
		for x, col := b.Min.X+r, 0; x < b.Max.X-r-0.5; x, col = x+sq, col+1 {
			if (row+col)%2 == 1 {
				ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(x, y, min(x+sq, b.Max.X-r), min(y+sq, b.Max.Y)), Color: dark})
			}
		}
	}
}

func (n *colorSliderNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	set := func(f float32) { n.s.v.set(n.s.with(f)) }
	if handled, follow := pickerPointer(ctx, e); handled {
		if e.Kind == gift.EventPointerDown {
			ctx.RequestFocus()
		}
		if follow {
			b := ctx.DeviceBounds()
			r := b.Height() / 2
			set((e.Pos.X - b.Min.X - r) / (b.Width() - 2*r))
		}
		return true
	}
	switch e.Kind {
	case gift.EventKeyDown:
		step := float32(1) / 255
		if n.s.channel == 3 {
			step = 0.01
		}
		switch e.Key {
		case gift.KeyLeft, gift.KeyDown:
			set(n.s.channelValue() - step)
		case gift.KeyRight, gift.KeyUp:
			set(n.s.channelValue() + step)
		case gift.KeyHome:
			set(0)
		case gift.KeyEnd:
			set(1)
		default:
			return false
		}
		return true
	}
	return false
}

// AccessibleValue implements [gift.ValueReporter]: the value beside the
// slider.
func (n *colorSliderNode) AccessibleValue(gift.ControlState) string { return n.s.valueText() }

// --- swatch -----------------------------------------------------------------------

// colorSwatch shows a colour in a rounded square, over the checkerboard when
// it is translucent.
type colorSwatch struct {
	c    Color
	size float32
}

func (colorSwatch) ViewType() gift.TypeID { return colorSwatchType }

func (s colorSwatch) Build(*gift.BuildContext) gift.Element {
	n := &colorSwatchNode{s: s, edge: ResolveColor(ColorSeparator)}
	return gift.Element{Key: "color-swatch", Layouter: n, Painter: n}
}

type colorSwatchNode struct {
	s    colorSwatch
	edge Color
}

func (n *colorSwatchNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(geom.Sz(n.s.size, n.s.size))
}

func (n *colorSwatchNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	r := float32(8)
	if !n.s.c.IsOpaque() {
		paintChecker(ctx, b, r)
	}
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: n.s.c, CornerRadius: r})
	ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: b, Color: n.edge, CornerRadius: r, StrokeWidth: 1})
}

// --- well -------------------------------------------------------------------------

var (
	colorWellType       = gift.RegisterType("ui.ColorWell")
	colorWellButtonType = gift.RegisterType("ui.colorWellButton")
)

// ColorWellView is a swatch that opens a colour picker. It is created by
// [ColorWell].
type ColorWellView struct {
	value    Color
	onChange func(Color)
	noAlpha  bool
	label    string
	key      string
	flex     float32
}

// ColorWell returns the control of SwiftUI's ColorPicker: the colour in a
// ring of every hue, 28 points across, that opens a [ColorPicker] in a
// [Popover] when it is tapped. Put a label next to it, as a form row does.
//
//	c := ctx.State("tint", ui.RGB(255, 149, 0))
//	ui.HStack(ui.Text("Tint").Flex(1), ui.ColorWell(ctx.Read(c), c.Set))
func ColorWell(value Color, onChange func(Color)) ColorWellView {
	return ColorWellView{value: value, onChange: onChange}
}

// SupportsOpacity with false keeps the colour opaque; see
// [ColorPickerView.SupportsOpacity].
func (v ColorWellView) SupportsOpacity(b bool) ColorWellView { v.noAlpha = !b; return v }

// Label sets the accessible name.
func (v ColorWellView) Label(s string) ColorWellView { v.label = s; return v }

// Key sets the reconciliation key.
func (v ColorWellView) Key(s string) ColorWellView { v.key = s; return v }

// Flex sets the flex factor in a stack.
func (v ColorWellView) Flex(f float32) ColorWellView { v.flex = f; return v }

// ViewType implements gift.View.
func (v ColorWellView) ViewType() gift.TypeID { return colorWellType }

// Build implements gift.View.
func (v ColorWellView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Flex: v.flex, Layouter: anchorLayout{}, Children: []gift.View{gift.Component("ui.colorWell", v.body)}}
}

func (v ColorWellView) body(ctx *gift.Context) gift.View {
	open := ctx.State("open", false)
	return Popover(
		colorWellButton{c: v.value, label: v.label, open: func() { open.Set(true) }},
		ColorPicker(v.value, v.onChange).SupportsOpacity(!v.noAlpha),
	).Presented(ctx.Read(open)).OnDismiss(func() { open.Set(false) }).MaxWidth(pickerWidth).Padding(16)
}

type colorWellButton struct {
	c     Color
	label string
	open  func()
}

func (colorWellButton) ViewType() gift.TypeID { return colorWellButtonType }

func (b colorWellButton) Build(*gift.BuildContext) gift.Element {
	n := &colorWellNode{b: b, gap: ResolveColor(ColorBackground), focusRing: resolveBorder(defaultFieldFocusRing)}
	label := b.label
	if label == "" {
		label = "Colour"
	}
	return gift.Element{Key: "color-well", Label: label, Layouter: n, Painter: n, Interactor: n, Focusable: true}
}

type colorWellNode struct {
	b         colorWellButton
	gap       Color
	focusRing Border
}

func (n *colorWellNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(geom.Sz(28, 28))
}

// Paint draws the ring of hues out of 24 arcs, each in the hue of its middle,
// and the colour inside it.
func (n *colorWellNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	c := geom.Pt((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2)
	r := min(b.Width(), b.Height()) / 2
	const k = 24
	for i := range k {
		h := float32(i) * 360 / k
		rr, g, bb := fromHSB(h+360/k/2, 1, 1)
		a := float32(i) * 2 * math.Pi / k
		paintArc(ctx, c, r, 3, a, 2*math.Pi/k+0.02, fromStraight(rr, g, bb, 1))
	}
	in := r - 5
	disc := geom.Rc(c.X-in, c.Y-in, c.X+in, c.Y+in)
	if !n.b.c.IsOpaque() {
		paintChecker(ctx, disc, in)
	}
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: disc, Color: n.b.c, CornerRadius: in})
	if ia := ctx.Interaction(); ia.Pressed {
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: RGBA(0, 0, 0, 40), CornerRadius: r})
	} else if ia.FocusVisible {
		paintBorder(ctx, styleSpec{border: n.focusRing, radius: r + 3}, b.Outset(geom.InsetsAll(3)))
	}
}

func (n *colorWellNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventPointerDown:
		return true
	case gift.EventPointerUp:
		if e.Inside && !e.Dragged {
			n.b.open()
		}
		return true
	case gift.EventKeyDown:
		if e.Key == gift.KeyEnter || e.Key == gift.KeySpace {
			n.b.open()
			return true
		}
	}
	return false
}
