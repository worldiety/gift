package raw

import "math"

// hsvMap is a table of a camera profile that corrects colours by their hue,
// saturation and value: ProfileHueSatMap or ProfileLookTable. Each entry is
// a shift of hue in degrees and factors of saturation and value; the table
// is stored with value outermost, then hue, saturation innermost. Hue spans
// the circle in hue divisions, saturation and value 0 to 1 in one step less
// than their divisions; one value division leaves value out.
type hsvMap struct {
	hues, sats, vals int
	data             []float32
	// srgb indexes value by its sRGB encoding rather than linearly.
	srgb bool
}

// newHSVMap makes a map of the given divisions over data, nil when they do
// not fit.
func newHSVMap(hues, sats, vals int, data []float32, srgb bool) *hsvMap {
	vals = max(vals, 1)
	if hues < 1 || sats < 2 || hues*sats*vals > 1<<20 || len(data) != 3*hues*sats*vals {
		return nil
	}
	for _, v := range data {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil
		}
	}
	// The sRGB encoding of value only means something to a table that has
	// value.
	return &hsvMap{hues: hues, sats: sats, vals: vals, data: data, srgb: srgb && vals > 1}
}

// blendHSV is the map of p's divisions over g of a and 1-g of b.
func blendHSV(dims [3]int, srgb bool, a, b []float32, g float64) *hsvMap {
	if b == nil || g >= 1 {
		return newHSVMap(dims[0], dims[1], dims[2], a, srgb)
	}
	d := make([]float32, len(a))
	for i := range d {
		d[i] = float32(g)*a[i] + float32(1-g)*b[i]
	}
	return newHSVMap(dims[0], dims[1], dims[2], d, srgb)
}

// entry is the entry at the divisions h, s, v.
func (m *hsvMap) entry(h, s, v int) (float32, float32, float32) {
	i := 3 * ((v*m.hues+h)*m.sats + s)
	return m.data[i], m.data[i+1], m.data[i+2]
}

// apply maps linear r, g, b, which are not negative, through the table:
// to hue, saturation and value as the hexcone model has them, the entry
// for them interpolated between the nearest divisions, and back. The
// tables of a camera profile work in linear ProPhoto RGB.
func (m *hsvMap) apply(r, g, b float32) (float32, float32, float32) {
	v := max(r, g, b)
	if !(v > 0) {
		return r, g, b
	}
	lo := min(r, g, b)
	delta := v - lo
	s := delta / v
	var h float32
	if delta > 0 {
		switch v {
		case r:
			h = (g - b) / delta
			if h < 0 {
				h += 6
			}
		case g:
			h = 2 + (b-r)/delta
		default:
			h = 4 + (r-g)/delta
		}
	}

	hf := h * float32(m.hues) / 6
	h0 := int(hf)
	ht := hf - float32(h0)
	h0 %= m.hues
	h1 := (h0 + 1) % m.hues

	sf := s * float32(m.sats-1)
	s0 := min(int(sf), m.sats-2)
	st := min(sf-float32(s0), 1)

	vi := v
	if m.srgb {
		vi = srgbEncode(v)
	}
	var v0 int
	var vt float32
	if m.vals > 1 {
		vf := min(vi, 1) * float32(m.vals-1)
		v0 = min(int(vf), m.vals-2)
		vt = min(vf-float32(v0), 1)
	}

	var shift, sat, val float32
	add := func(w float32, hh, ss, vv int) {
		if w == 0 {
			return
		}
		a, b, c := m.entry(hh, ss, vv)
		shift += w * a
		sat += w * b
		val += w * c
	}
	weights, n := [2]float32{1 - vt, vt}, 2
	if m.vals == 1 {
		weights, n = [2]float32{1, 0}, 1
	}
	for k := range n {
		vv, wv := v0+k, weights[k]
		add(wv*(1-ht)*(1-st), h0, s0, vv)
		add(wv*ht*(1-st), h1, s0, vv)
		add(wv*(1-ht)*st, h0, s0+1, vv)
		add(wv*ht*st, h1, s0+1, vv)
	}

	h += shift / 60
	for h < 0 {
		h += 6
	}
	for h >= 6 {
		h -= 6
	}
	// Saturation and value are clipped at 1, as the specification has it;
	// a value above 1, a highlight the scene has beyond white, is only
	// ever lowered.
	s = min(max(s*sat, 0), 1)
	switch {
	case m.srgb && v <= 1:
		if val != 1 {
			v = srgbDecode(min(vi*val, 1))
		}
	case m.srgb:
		v *= srgbDecode(min(val, 1))
	case v <= 1:
		v = min(v*val, 1)
	default:
		v *= min(val, 1)
	}
	return hsvToRGB(h, s, v)
}

func hsvToRGB(h, s, v float32) (float32, float32, float32) {
	i := int(h)
	f := h - float32(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	switch i {
	case 0:
		return v, t, p
	case 1:
		return q, v, p
	case 2:
		return p, v, t
	case 3:
		return p, q, v
	case 4:
		return t, p, v
	}
	return v, p, q
}

func srgbEncode(v float32) float32 {
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return float32(1.055*math.Pow(float64(v), 1/2.4) - 0.055)
}

func srgbDecode(v float32) float32 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return float32(math.Pow((float64(v)+0.055)/1.055, 2.4))
}
