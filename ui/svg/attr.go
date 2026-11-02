package svg

import (
	"math"
	"strconv"
	"strings"
)

// rgba is a colour, not premultiplied, each channel in [0, 1].
type rgba [4]float64

// parseColor reads a colour of SVG: #rgb, #rgba, #rrggbb, #rrggbbaa,
// rgb(), rgba() with numbers or percentages, or one of the named colours of
// CSS. currentColor is resolved by the caller.
func parseColor(s string) (rgba, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(s, "#"):
		h := s[1:]
		if len(h) == 3 || len(h) == 4 {
			var b strings.Builder
			for _, c := range h {
				b.WriteRune(c)
				b.WriteRune(c)
			}
			h = b.String()
		}
		if len(h) != 6 && len(h) != 8 {
			return rgba{}, false
		}
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return rgba{}, false
		}
		if len(h) == 6 {
			v = v<<8 | 0xff
		}
		return rgba{float64(v>>24&0xff) / 255, float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255}, true
	case strings.HasPrefix(s, "rgb"):
		open, end := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
		if open < 0 || end < open {
			return rgba{}, false
		}
		parts := strings.FieldsFunc(s[open+1:end], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		if len(parts) < 3 {
			return rgba{}, false
		}
		c := rgba{0, 0, 0, 1}
		for i := range min(len(parts), 4) {
			p := parts[i]
			f, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
			if err != nil {
				return rgba{}, false
			}
			switch {
			case strings.HasSuffix(p, "%"):
				f /= 100
			case i < 3:
				f /= 255
			}
			c[i] = math.Min(math.Max(f, 0), 1)
		}
		return c, true
	case s == "transparent":
		return rgba{}, true
	}
	if v, ok := namedColors[s]; ok {
		return rgba{float64(v>>16) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255, 1}, true
	}
	return rgba{}, false
}

// parseLength reads a length in user units: a number with px, pt, pc, mm,
// cm, in, em or ex, or a percentage of ref.
func parseLength(s string, ref float64) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	unit := 1.0
	for _, u := range []struct {
		suffix string
		f      float64
	}{{"px", 1}, {"pt", 4.0 / 3}, {"pc", 16}, {"mm", 96 / 25.4}, {"cm", 96 / 2.54}, {"in", 96}, {"em", 16}, {"ex", 8}, {"%", -1}} {
		if strings.HasSuffix(s, u.suffix) {
			s, unit = strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), u.f
			break
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if unit < 0 {
		return f / 100 * ref, true
	}
	return f * unit, true
}

// numbers reads a list of numbers separated by commas and spaces.
func numbers(s string) []float64 {
	var out []float64
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' }) {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// matrix is an affine transform: x' = a·x + c·y + e, y' = b·x + d·y + f.
type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

// mul returns m applied after n: the transform of a child n inside a parent
// m.
func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

func (m matrix) apply(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

// invert returns the inverse of m, and false when m is singular.
func (m matrix) invert() (matrix, bool) {
	det := m[0]*m[3] - m[1]*m[2]
	if det == 0 || math.IsNaN(det) {
		return matrix{}, false
	}
	return matrix{
		m[3] / det, -m[1] / det, -m[2] / det, m[0] / det,
		(m[2]*m[5] - m[3]*m[4]) / det, (m[1]*m[4] - m[0]*m[5]) / det,
	}, true
}

// scale is how much m enlarges a length, the geometric mean of its axes:
// what a stroke width becomes.
func (m matrix) scale() float64 { return math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2])) }

// parseTransform reads the transform attribute: matrix, translate, scale,
// rotate, skewX and skewY, applied from left to right.
func parseTransform(s string) (matrix, bool) {
	m := identity
	s = strings.TrimSpace(s)
	for s != "" {
		open := strings.IndexByte(s, '(')
		end := strings.IndexByte(s, ')')
		if open < 0 || end < open {
			return identity, false
		}
		name := strings.TrimSpace(strings.Trim(s[:open], ", "))
		v := numbers(s[open+1 : end])
		s = strings.TrimLeft(s[end+1:], ", \t\n\r")
		var t matrix
		switch {
		case name == "matrix" && len(v) == 6:
			t = matrix{v[0], v[1], v[2], v[3], v[4], v[5]}
		case name == "translate" && len(v) == 1:
			t = matrix{1, 0, 0, 1, v[0], 0}
		case name == "translate" && len(v) == 2:
			t = matrix{1, 0, 0, 1, v[0], v[1]}
		case name == "scale" && len(v) == 1:
			t = matrix{v[0], 0, 0, v[0], 0, 0}
		case name == "scale" && len(v) == 2:
			t = matrix{v[0], 0, 0, v[1], 0, 0}
		case name == "rotate" && (len(v) == 1 || len(v) == 3):
			a := v[0] * math.Pi / 180
			sin, cos := math.Sin(a), math.Cos(a)
			t = matrix{cos, sin, -sin, cos, 0, 0}
			if len(v) == 3 {
				t = matrix{1, 0, 0, 1, v[1], v[2]}.mul(t).mul(matrix{1, 0, 0, 1, -v[1], -v[2]})
			}
		case name == "skewX" && len(v) == 1:
			t = matrix{1, 0, math.Tan(v[0] * math.Pi / 180), 1, 0, 0}
		case name == "skewY" && len(v) == 1:
			t = matrix{1, math.Tan(v[0] * math.Pi / 180), 0, 1, 0, 0}
		default:
			return identity, false
		}
		m = m.mul(t)
	}
	return m, true
}

// parseStyle reads the declarations of a style attribute.
func parseStyle(s string, into map[string]string) {
	for _, decl := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important"))
		into[strings.TrimSpace(strings.ToLower(k))] = v
	}
}

// urlRef returns the id of url(#id), and false for anything else.
func urlRef(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "url(") {
		return "", false
	}
	end := strings.IndexByte(s, ')')
	if end < 0 {
		return "", false
	}
	ref := strings.Trim(strings.TrimSpace(s[4:end]), `"'`)
	return strings.TrimPrefix(ref, "#"), strings.HasPrefix(ref, "#")
}

// namedColors are the named colours of CSS.
var namedColors = map[string]uint32{
	"aliceblue": 0xf0f8ff, "antiquewhite": 0xfaebd7, "aqua": 0x00ffff, "aquamarine": 0x7fffd4, "azure": 0xf0ffff,
	"beige": 0xf5f5dc, "bisque": 0xffe4c4, "black": 0x000000, "blanchedalmond": 0xffebcd, "blue": 0x0000ff,
	"blueviolet": 0x8a2be2, "brown": 0xa52a2a, "burlywood": 0xdeb887, "cadetblue": 0x5f9ea0, "chartreuse": 0x7fff00,
	"chocolate": 0xd2691e, "coral": 0xff7f50, "cornflowerblue": 0x6495ed, "cornsilk": 0xfff8dc, "crimson": 0xdc143c,
	"cyan": 0x00ffff, "darkblue": 0x00008b, "darkcyan": 0x008b8b, "darkgoldenrod": 0xb8860b, "darkgray": 0xa9a9a9,
	"darkgreen": 0x006400, "darkgrey": 0xa9a9a9, "darkkhaki": 0xbdb76b, "darkmagenta": 0x8b008b, "darkolivegreen": 0x556b2f,
	"darkorange": 0xff8c00, "darkorchid": 0x9932cc, "darkred": 0x8b0000, "darksalmon": 0xe9967a, "darkseagreen": 0x8fbc8f,
	"darkslateblue": 0x483d8b, "darkslategray": 0x2f4f4f, "darkslategrey": 0x2f4f4f, "darkturquoise": 0x00ced1,
	"darkviolet": 0x9400d3, "deeppink": 0xff1493, "deepskyblue": 0x00bfff, "dimgray": 0x696969, "dimgrey": 0x696969,
	"dodgerblue": 0x1e90ff, "firebrick": 0xb22222, "floralwhite": 0xfffaf0, "forestgreen": 0x228b22, "fuchsia": 0xff00ff,
	"gainsboro": 0xdcdcdc, "ghostwhite": 0xf8f8ff, "gold": 0xffd700, "goldenrod": 0xdaa520, "gray": 0x808080,
	"green": 0x008000, "greenyellow": 0xadff2f, "grey": 0x808080, "honeydew": 0xf0fff0, "hotpink": 0xff69b4,
	"indianred": 0xcd5c5c, "indigo": 0x4b0082, "ivory": 0xfffff0, "khaki": 0xf0e68c, "lavender": 0xe6e6fa,
	"lavenderblush": 0xfff0f5, "lawngreen": 0x7cfc00, "lemonchiffon": 0xfffacd, "lightblue": 0xadd8e6,
	"lightcoral": 0xf08080, "lightcyan": 0xe0ffff, "lightgoldenrodyellow": 0xfafad2, "lightgray": 0xd3d3d3,
	"lightgreen": 0x90ee90, "lightgrey": 0xd3d3d3, "lightpink": 0xffb6c1, "lightsalmon": 0xffa07a,
	"lightseagreen": 0x20b2aa, "lightskyblue": 0x87cefa, "lightslategray": 0x778899, "lightslategrey": 0x778899,
	"lightsteelblue": 0xb0c4de, "lightyellow": 0xffffe0, "lime": 0x00ff00, "limegreen": 0x32cd32, "linen": 0xfaf0e6,
	"magenta": 0xff00ff, "maroon": 0x800000, "mediumaquamarine": 0x66cdaa, "mediumblue": 0x0000cd,
	"mediumorchid": 0xba55d3, "mediumpurple": 0x9370db, "mediumseagreen": 0x3cb371, "mediumslateblue": 0x7b68ee,
	"mediumspringgreen": 0x00fa9a, "mediumturquoise": 0x48d1cc, "mediumvioletred": 0xc71585, "midnightblue": 0x191970,
	"mintcream": 0xf5fffa, "mistyrose": 0xffe4e1, "moccasin": 0xffe4b5, "navajowhite": 0xffdead, "navy": 0x000080,
	"oldlace": 0xfdf5e6, "olive": 0x808000, "olivedrab": 0x6b8e23, "orange": 0xffa500, "orangered": 0xff4500,
	"orchid": 0xda70d6, "palegoldenrod": 0xeee8aa, "palegreen": 0x98fb98, "paleturquoise": 0xafeeee,
	"palevioletred": 0xdb7093, "papayawhip": 0xffefd5, "peachpuff": 0xffdab9, "peru": 0xcd853f, "pink": 0xffc0cb,
	"plum": 0xdda0dd, "powderblue": 0xb0e0e6, "purple": 0x800080, "rebeccapurple": 0x663399, "red": 0xff0000,
	"rosybrown": 0xbc8f8f, "royalblue": 0x4169e1, "saddlebrown": 0x8b4513, "salmon": 0xfa8072, "sandybrown": 0xf4a460,
	"seagreen": 0x2e8b57, "seashell": 0xfff5ee, "sienna": 0xa0522d, "silver": 0xc0c0c0, "skyblue": 0x87ceeb,
	"slateblue": 0x6a5acd, "slategray": 0x708090, "slategrey": 0x708090, "snow": 0xfffafa, "springgreen": 0x00ff7f,
	"steelblue": 0x4682b4, "tan": 0xd2b48c, "teal": 0x008080, "thistle": 0xd8bfd8, "tomato": 0xff6347,
	"turquoise": 0x40e0d0, "violet": 0xee82ee, "wheat": 0xf5deb3, "white": 0xffffff, "whitesmoke": 0xf5f5f5,
	"yellow": 0xffff00, "yellowgreen": 0x9acd32,
}
