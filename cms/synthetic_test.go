package cms

import (
	"encoding/binary"
	"flag"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// The synthetic profiles exercise the parts of the ICC specification the
// built-in profiles do not: lut8 and lut16 tables, the v2 encoding of Lab,
// mAB and mBA, a printer with paper white and an ink limit, a gray scanner
// with a white of its own. They model nothing real; they only have to be
// valid, smooth and different per intent, so that a wrong choice of table
// or a wrong encoding shows against LittleCMS.
//
//	go test ./cms -run TestWriteSyntheticProfiles -cms.write-profiles
var writeProfiles = flag.Bool("cms.write-profiles", false, "write the profiles of testdata/profiles")

func TestWriteSyntheticProfiles(t *testing.T) {
	if !*writeProfiles {
		t.Skip("only with -cms.write-profiles")
	}
	dir := filepath.Join("testdata", "profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, p := range syntheticProfiles(t) {
		if err := os.WriteFile(filepath.Join(dir, name+".icc"), p, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func syntheticProfiles(t *testing.T) map[string][]byte {
	t.Helper()
	m := map[string][]byte{
		"srgb":        SRGB().Bytes(),
		"displayp3":   DisplayP3().Bytes(),
		"adobergb":    AdobeRGB().Bytes(),
		"prophoto":    ProPhotoRGB().Bytes(),
		"rec2020":     Rec2020().Bytes(),
		"linear-srgb": LinearSRGB().Bytes(),
		"sgray":       SGray().Bytes(),
		"cmyk-lut16":  cmykPrinter(),
		"rgb-mab":     rgbModular(),
		"rgb-lut8":    rgbLut8(),
		"gray-xyz":    grayScanner(),
	}
	for name, b := range m {
		if _, err := ParseProfile(b); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	return m
}

// srgbLinear is the matrix from linear sRGB to XYZ relative to D50.
func srgbLinear() mat3 {
	m, _ := rgbToXYZ(WhiteD65, primariesSRGB)
	return adaptation(WhiteD65.XYZ(), D50).mul(m)
}

func clamp01(v float64) float64 { return min(max(v, 0), 1) }

// table16 samples f over a grid of g points per input channel, first
// channel slowest, as 16 bit words.
func table16(in, out, g int, f func(v []float64) []float64) []uint16 {
	var r []uint16
	v := make([]float64, in)
	var walk func(c int)
	walk = func(c int) {
		if c == in {
			for _, o := range f(v)[:out] {
				r = append(r, uint16(math.Round(clamp01(o)*65535)))
			}
			return
		}
		for i := range g {
			v[c] = float64(i) / float64(g-1)
			walk(c + 1)
		}
	}
	walk(0)
	return r
}

func curve16(n int, f func(float64) float64) []uint16 {
	r := make([]uint16, n)
	for i := range r {
		r[i] = uint16(math.Round(clamp01(f(float64(i)/float64(n-1))) * 65535))
	}
	return r
}

func ident(x float64) float64 { return x }

// mft builds a lut8Type or lut16Type from its curves and table.
func mft(is16 bool, in, out, g int, ins, outs [][]uint16, clut []uint16) []byte {
	be := binary.BigEndian
	b := sigBytes(typeMft1)
	if is16 {
		b = sigBytes(typeMft2)
	}
	b = append(b, 0, 0, 0, 0, byte(in), byte(out), byte(g), 0)
	for i := range 9 {
		v := 0.0
		if i%4 == 0 {
			v = 1
		}
		b = append(b, s15f16Bytes(v)...)
	}
	put := func(t []uint16) {
		for _, v := range t {
			if is16 {
				b = be.AppendUint16(b, v)
			} else {
				b = append(b, byte(math.Round(float64(v)/257)))
			}
		}
	}
	if is16 {
		b = be.AppendUint16(b, uint16(len(ins[0])))
		b = be.AppendUint16(b, uint16(len(outs[0])))
	}
	for _, c := range ins {
		put(c)
	}
	put(clut)
	for _, c := range outs {
		put(c)
	}
	return b
}

// labV2Encode and labV2Decode are the lut16 encoding of Lab.
func labV2Encode(l, a, b float64) []float64 {
	return []float64{l / 100 / labV2, (a + 128) / 255 / labV2, (b + 128) / 255 / labV2}
}

func labV2Decode(v []float64) (l, a, b float64) {
	return v[0] * labV2 * 100, v[1]*labV2*255 - 128, v[2]*labV2*255 - 128
}

// cmykPrinter is a v2 printer profile on Lab with lut16 tables: a paper
// whiter than nothing but darker and yellower than D50, inks with dot gain,
// a black that is not black, and an ink limit. A2B2 is missing, so that
// the saturation intent falls back to the perceptual table.
func cmykPrinter() []byte {
	m := srgbLinear()
	toLab := func(perceptual bool) func(v []float64) []float64 {
		return func(v []float64) []float64 {
			c, mg, y, k := math.Pow(v[0], 0.8), math.Pow(v[1], 0.8), math.Pow(v[2], 0.8), math.Pow(v[3], 0.9)
			r := (1 - 0.93*c) * (1 - 0.94*k)
			g := (1 - 0.93*mg) * (1 - 0.94*k)
			bl := (1 - 0.93*y) * (1 - 0.94*k)
			x, yy, z := m.apply(r, g, bl)
			l, a, b := xyzToLab(D50, x, yy, z)
			if perceptual {
				l = 4 + l*0.96
			}
			return labV2Encode(l, a, b)
		}
	}
	inv, _ := m.inverse()
	toCMYK := func(perceptual bool) func(v []float64) []float64 {
		return func(v []float64) []float64 {
			l, a, b := labV2Decode(v)
			if perceptual {
				l = (l - 4) / 0.96
			}
			x, y, z := labToXYZ(D50, l, a, b)
			r, g, bl := inv.apply(x, y, z)
			r, g, bl = clamp01(r), clamp01(g), clamp01(bl)
			k := min(1-r, 1-g, 1-bl) * 0.6
			kk := 1 - 0.94*math.Pow(k, 0.9)
			ink := func(o float64) float64 {
				return math.Pow(clamp01((1-o/kk)/0.93), 1/0.8)
			}
			cmyk := []float64{ink(r), ink(g), ink(bl), k}
			if sum := cmyk[0] + cmyk[1] + cmyk[2] + cmyk[3]; sum > 3 {
				for i := range cmyk {
					cmyk[i] *= 3 / sum
				}
			}
			return cmyk
		}
	}
	shape := [][]uint16{curve16(256, func(x float64) float64 { return math.Pow(x, 1.05) })}
	ins4 := [][]uint16{shape[0], shape[0], shape[0], shape[0]}
	lin := curve16(2, ident)
	outs3 := [][]uint16{lin, lin, lin}
	ins3 := [][]uint16{lin, lin, lin}
	outs4 := [][]uint16{lin, lin, lin, lin}
	tags := []tag{
		{tagDesc, descV2("Synthetic CMYK printer")},
		{tagWtpt, xyzTag(XYZ{0.92, 0.95, 0.75})},
		{tagA2B0, mft(true, 4, 3, 9, ins4, outs3, table16(4, 3, 9, toLab(true)))},
		{tagA2B1, mft(true, 4, 3, 9, ins4, outs3, table16(4, 3, 9, toLab(false)))},
		{tagB2A0, mft(true, 3, 4, 17, ins3, outs4, table16(3, 4, 17, toCMYK(true)))},
		{tagB2A1, mft(true, 3, 4, 17, ins3, outs4, table16(3, 4, 17, toCMYK(false)))},
	}
	return writeProfileVersion(0x02100000, ClassOutput, SpaceCMYK, SpaceLab, tags)
}

// descV2 is a textDescriptionType, the v2 form of a description.
func descV2(s string) []byte {
	b := append(sigBytes(typeDesc), 0, 0, 0, 0)
	b = binary.BigEndian.AppendUint32(b, uint32(len(s)+1))
	b = append(b, s...)
	b = append(b, 0)
	return append(b, make([]byte, 4+4+2+1+67)...)
}

// rgbModular is a v4 display profile on XYZ with mAB and mBA tables that
// use every element: A curves, a table that mixes the channels a little,
// M curves, a matrix with an offset, B curves.
func rgbModular() []byte {
	m := srgbLinear()
	inv, _ := m.inverse()
	be := binary.BigEndian
	para := func(kind int, p ...float64) []byte {
		b, _ := curveTag(Parametric(kind, p...))
		return b
	}
	table := func(t []uint16) []byte {
		b := append(sigBytes(typeCurv), 0, 0, 0, 0)
		b = be.AppendUint32(b, uint32(len(t)))
		for _, v := range t {
			b = be.AppendUint16(b, v)
		}
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
		return b
	}
	clutBytes := func(g int, t []uint16) []byte {
		b := make([]byte, 20)
		for i := range 3 {
			b[i] = byte(g)
		}
		b[16] = 2
		for _, v := range t {
			b = be.AppendUint16(b, v)
		}
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
		return b
	}
	matrix := func(mm mat3, scale float64, off [3]float64) []byte {
		var b []byte
		for _, r := range mm {
			for _, v := range r {
				b = append(b, s15f16Bytes(v*scale)...)
			}
		}
		for _, v := range off {
			b = append(b, s15f16Bytes(v)...)
		}
		return b
	}
	// The table mixes 5 % of each neighbour channel in, and back out.
	mix := mat3{{0.9, 0.05, 0.05}, {0.05, 0.9, 0.05}, {0.05, 0.05, 0.9}}
	unmix, _ := mix.inverse()
	build := func(aToB bool) []byte {
		var parts [5][]byte // B, matrix, M, CLUT, A
		lin := para(0, 1)
		parts[0] = append(append(append([]byte{}, lin...), lin...), lin...)
		gm := para(0, 1.1)
		parts[2] = append(append(append([]byte{}, gm...), gm...), gm...)
		if aToB {
			parts[1] = matrix(m, 1/maxXYZ, [3]float64{0.001, 0.001, 0.001})
			parts[3] = clutBytes(9, table16(3, 3, 9, func(v []float64) []float64 {
				x, y, z := mix.apply(v[0], v[1], v[2])
				return []float64{math.Pow(x, 1/1.1), math.Pow(y, 1/1.1), math.Pow(z, 1/1.1)}
			}))
			a := table(curve16(1024, func(x float64) float64 { return math.Pow(x, 2.2) }))
			parts[4] = append(append(append([]byte{}, a...), a...), a...)
		} else {
			// mBA reverses the order: B, matrix, M, CLUT, A from PCS to
			// device, and the matrix takes normalized XYZ.
			parts[1] = matrix(inv, maxXYZ, [3]float64{-0.001, -0.001, -0.001})
			parts[3] = clutBytes(9, table16(3, 3, 9, func(v []float64) []float64 {
				x, y, z := math.Pow(v[0], 1/1.1), math.Pow(v[1], 1/1.1), math.Pow(v[2], 1/1.1)
				r, g, b := unmix.apply(x, y, z)
				return []float64{r, g, b}
			}))
			a := table(curve16(1024, func(x float64) float64 { return math.Pow(x, 1/2.2) }))
			parts[4] = append(append(append([]byte{}, a...), a...), a...)
		}
		sig := uint32(typeMAB)
		if !aToB {
			sig = typeMBA
		}
		b := append(sigBytes(sig), 0, 0, 0, 0, 3, 3, 0, 0)
		b = append(b, make([]byte, 20)...)
		for i, part := range parts {
			be.PutUint32(b[12+4*i:], uint32(len(b)))
			b = append(b, part...)
		}
		return b
	}
	tags := []tag{
		{tagDesc, mluc("Synthetic RGB, mAB and mBA")},
		{tagWtpt, xyzTag(D50)},
		{tagA2B0, build(true)},
		{tagB2A0, build(false)},
	}
	return writeProfileVersion(0x04300000, ClassDisplay, SpaceRGB, SpaceXYZ, tags)
}

// rgbLut8 is a v2 scanner profile on Lab with lut8 tables in both
// directions, the colorimetric one less saturated than the perceptual.
func rgbLut8() []byte {
	m := srgbLinear()
	inv, _ := m.inverse()
	dec := curve16(256, func(x float64) float64 { return SRGBCurve().mustCompile().eval(x) })
	enc := curve16(256, func(x float64) float64 { return SRGBCurve().mustCompile().inverse(x) })
	lin := curve16(256, ident)
	toLab := func(sat float64) func(v []float64) []float64 {
		return func(v []float64) []float64 {
			x, y, z := m.apply(v[0], v[1], v[2])
			l, a, b := xyzToLab(D50, x, y, z)
			return []float64{l / 100, (a*sat + 128) / 255, (b*sat + 128) / 255}
		}
	}
	toRGB := func(sat float64) func(v []float64) []float64 {
		return func(v []float64) []float64 {
			x, y, z := labToXYZ(D50, v[0]*100, (v[1]*255-128)/sat, (v[2]*255-128)/sat)
			r, g, b := inv.apply(x, y, z)
			return []float64{r, g, b}
		}
	}
	three := func(c []uint16) [][]uint16 { return [][]uint16{c, c, c} }
	tags := []tag{
		{tagDesc, descV2("Synthetic RGB, lut8")},
		{tagWtpt, xyzTag(D50)},
		{tagA2B0, mft(false, 3, 3, 9, three(dec), three(lin), table16(3, 3, 9, toLab(1)))},
		{tagA2B1, mft(false, 3, 3, 9, three(dec), three(lin), table16(3, 3, 9, toLab(0.9)))},
		{tagB2A0, mft(false, 3, 3, 9, three(lin), three(enc), table16(3, 3, 9, toRGB(1)))},
		{tagB2A1, mft(false, 3, 3, 9, three(lin), three(enc), table16(3, 3, 9, toRGB(0.9)))},
	}
	return writeProfileVersion(0x02100000, ClassInput, SpaceRGB, SpaceLab, tags)
}

// grayScanner is a v2 input profile of a gray device with a gamma of 1.8
// and a white of its own, which the absolute intent keeps.
func grayScanner() []byte {
	c := append(sigBytes(typeCurv), 0, 0, 0, 0, 0, 0, 0, 1)
	c = binary.BigEndian.AppendUint16(c, uint16(math.Round(1.8*256)))
	tags := []tag{
		{tagDesc, descV2("Synthetic gray scanner")},
		{tagWtpt, xyzTag(XYZ{0.93, 0.97, 0.85})},
		{tagKTRC, c},
	}
	return writeProfileVersion(0x02100000, ClassInput, SpaceGray, SpaceXYZ, tags)
}

func (c Curve) mustCompile() curve {
	k, err := c.compile()
	if err != nil {
		panic(err)
	}
	return k
}
