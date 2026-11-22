package raw

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"runtime"
	"slices"
	"sync"

	"github.com/worldiety/gift/cms"
)

// Look is how [OutputPicture] makes a picture of the light of the scene.
type Look uint8

const (
	// LookCamera makes the picture as the camera made its own: the colours
	// and tones of its style, with its contrast and saturation, the mean
	// of what its dynamic range optimiser did, and black and white where
	// the camera took the photograph so. They are measured, file by file,
	// against the preview the camera rendered from the same raw data: a
	// matrix and a tone curve that turn the scene into the preview. A
	// file without a preview, one whose preview the measurement does not
	// fit, and a picture made with a profile of [Options.Profile] are
	// made as LookNeutral.
	LookCamera Look = iota
	// LookNeutral makes the picture of the profile's colours and the tone
	// curve of this package, the same for every file and every setting of
	// the camera.
	LookNeutral
)

func (l Look) String() string {
	switch l {
	case LookCamera:
		return "camera"
	case LookNeutral:
		return "neutral"
	}
	return fmt.Sprintf("Look(%d)", uint8(l))
}

// cameraLook is the look of the camera measured against a preview: linear
// Rec. 2020 of the scene through matrix to the linear light of the
// preview's space before its tones, through the tone curve, channel by
// channel, to the linear light of the preview; gray when the preview is.
type cameraLook struct {
	matrix mat
	tone   *toneCurve
	gray   bool
	// space is the preview's colour space, to2020 its linear light to
	// linear Rec. 2020.
	space    *cms.Profile
	to2020   mat
	residual float64 // the mean ΔE of the held out cells
	neutral  float64 // that of the neutral look
}

// maxLookError is the most the measured look may miss the preview by, a
// mean ΔE 76 of the cells held out. The fits of the Alpha 7 III's files
// miss by 1.3 to 2.6, at ISO 16000 by 4.4.
const maxLookError = 10

// lookGrid is the side of the cells the scene and the preview are compared
// in, in pixels of the preview: large enough that noise, sharpening and the
// preview's alignment within a pixel do not matter.
const lookGrid = 4

// lookState holds the camera look of a file once measured.
type lookState struct {
	once sync.Once
	look *cameraLook
}

// cameraLook measures the look of f against its largest preview, once,
// on the mosaic d develops; nil when there is none or it does not fit.
func (f *File) cameraLook(d *developer) *cameraLook {
	f.looks.once.Do(func() {
		f.looks.look = f.measureLook(d)
	})
	return f.looks.look
}

func (f *File) measureLook(d *developer) *cameraLook {
	// The preview must show the crop: of its aspect, within a percent, so
	// that a thumbnail with black bars is not taken for the picture.
	p, ok := f.Largest()
	c := f.Mosaic.Crop
	if !ok || p.Width < 64*lookGrid || p.Height < 32*lookGrid || c.Dy() == 0 ||
		math.Abs(float64(p.Width*c.Dy())/float64(p.Height*c.Dx())-1) > 0.01 {
		return nil
	}
	img, err := f.DecodePreview(p)
	if err != nil {
		return nil
	}
	return f.lookOf(d, img, p.Space)
}

// lookOf measures the look of the picture img, in space, nil for sRGB,
// against the scene of the mosaic d develops.
func (f *File) lookOf(d *developer, img image.Image, space *cms.Profile) *cameraLook {
	if space == nil {
		space = cms.SRGB()
	}
	t, err := cms.NewTransform(space, cms.LinearRec2020(), cms.Options{Intent: cms.RelativeColorimetric})
	if err != nil {
		return nil
	}
	sh, ok := t.Shaper()
	if !ok {
		return nil
	}
	// The camera rendered the preview with the white balance it took the
	// photograph with, which d's may not be: its levels are scaled to it.
	wb := f.whiteBalance(Options{})
	scene, err := f.newPipe(Options{Output: OutputScene}, wb, nil)
	if err != nil {
		return nil
	}
	lo := min(wb[0], wb[1], wb[2])
	var rescale vec
	for c := range 3 {
		rescale[c] = wb[c] / lo / float64(d.level[c])
	}
	cells := lookCells(d, rescale, scene, img, sh.In)
	if len(cells) < 2000 {
		return nil
	}
	var to2020 mat
	for i := range 9 {
		to2020[i/3][i%3] = float64(sh.Matrix[i])
	}
	from2020, ok := to2020.inverse()
	if !ok {
		return nil
	}
	l := fitLook(cells, from2020)
	if l == nil {
		return nil
	}
	l.space, l.to2020 = space, to2020
	return l
}

// lookCell is a cell of the grid: the scene in it, linear Rec. 2020, and
// the preview, linear in its space.
type lookCell struct {
	scene, preview vec
}

// lookCells averages the scene and the preview over the cells of the grid,
// leaving out those where the sensor clipped.
func lookCells(d *developer, rescale vec, scene *pipe, img image.Image, in [3][]float32) []lookCell {
	pb := img.Bounds()
	gw, gh := pb.Dx()/lookGrid, pb.Dy()/lookGrid
	cw, ch := d.crop.Dx(), d.crop.Dy()
	rows := make([][]lookCell, gh)
	var wg sync.WaitGroup
	next := make(chan int)
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for gy := range next {
				var out []lookCell
				for gx := range gw {
					// The cell in the crop, in whole 2 by 2 blocks so that
					// each colour is counted alike.
					x0, x1 := gx*cw/gw&^1, (gx+1)*cw/gw&^1
					y0, y1 := gy*ch/gh&^1, (gy+1)*ch/gh&^1
					var sum [3]float64
					var n [3]int
					clipped := false
					for y := y0; y < y1 && !clipped; y++ {
						sy := d.crop.Min.Y + y
						for x := x0; x < x1; x++ {
							sx := d.crop.Min.X + x
							if d.clipped(sx, sy) {
								clipped = true
								break
							}
							c := d.colourAt(sx, sy)
							sum[c] += float64(d.value(sx, sy))
							n[c]++
						}
					}
					if clipped || n[0] == 0 || n[1] == 0 || n[2] == 0 {
						continue
					}
					r := float32(rescale[0] * sum[0] / float64(n[0]))
					g := float32(rescale[1] * sum[1] / float64(n[1]))
					b := float32(rescale[2] * sum[2] / float64(n[2]))
					tr, tg, tb := []float32{r}, []float32{g}, []float32{b}
					scene.run(Tile{Rect: image.Rect(0, 0, 1, 1), R: tr, G: tg, B: tb, Stride: 1})
					c := lookCell{scene: vec{float64(tr[0]), float64(tg[0]), float64(tb[0])}}
					for py := gy * lookGrid; py < (gy+1)*lookGrid; py++ {
						for px := gx * lookGrid; px < (gx+1)*lookGrid; px++ {
							pr, pg, pbl := rgb8(img, pb.Min.X+px, pb.Min.Y+py)
							c.preview[0] += float64(linearOf(in[0], pr))
							c.preview[1] += float64(linearOf(in[1], pg))
							c.preview[2] += float64(linearOf(in[2], pbl))
						}
					}
					for k := range 3 {
						c.preview[k] /= lookGrid * lookGrid
					}
					out = append(out, c)
				}
				rows[gy] = out
			}
		}()
	}
	for gy := range gh {
		next <- gy
	}
	close(next)
	wg.Wait()
	var cells []lookCell
	for _, r := range rows {
		cells = append(cells, r...)
	}
	return cells
}

// rgb8 is the 8-bit colour of img at (x, y).
func rgb8(img image.Image, x, y int) (uint8, uint8, uint8) {
	switch m := img.(type) {
	case *image.YCbCr:
		yi, ci := m.YOffset(x, y), m.COffset(x, y)
		return color.YCbCrToRGB(m.Y[yi], m.Cb[ci], m.Cr[ci])
	case *image.Gray:
		v := m.Pix[m.PixOffset(x, y)]
		return v, v, v
	case *image.RGBA:
		i := m.PixOffset(x, y)
		return m.Pix[i], m.Pix[i+1], m.Pix[i+2]
	}
	r, g, b, _ := img.At(x, y).RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

// linearOf is the linear light of the 8-bit value v through a curve sampled
// evenly from 0 to 1.
func linearOf(curve []float32, v uint8) float32 {
	f := float32(v) / 255 * float32(len(curve)-1)
	i := int(f)
	if i >= len(curve)-1 {
		return curve[len(curve)-1]
	}
	w := f - float32(i)
	return curve[i] + w*(curve[i+1]-curve[i])
}

// The tone curve of a look is measured in bins of an eighth of a stop of
// the light before it, from 2^lookLow to 2^lookHigh.
const (
	lookLow   = -14.0
	lookHigh  = 4.0
	lookSteps = 8
	lookBins  = int((lookHigh - lookLow) * lookSteps)
)

// fitLook fits the matrix and the tone curve that turn the scene of the
// cells into their preview: alternately the curve for the matrix, as the
// median of the preview in each bin of the light, and the matrix for the
// curve, by least squares of the relative error against what the curve
// needs to make the preview. Every other cell is held out and judges the
// fit against the neutral look.
func fitLook(cells []lookCell, from2020 mat) *cameraLook {
	// A quarter of the cells fit, another quarter judge: some 25000 each
	// for the preview of an Alpha 7 III.
	var fit, held []lookCell
	for i, c := range cells {
		switch i % 4 {
		case 0:
			fit = append(fit, c)
		case 2:
			held = append(held, c)
		}
	}
	gray := isGray(cells)
	m := from2020
	var curve []float64
	for range 3 {
		curve = fitCurve(fit, m)
		if curve == nil {
			return nil
		}
		next, ok := fitMatrix(fit, curve, m, gray)
		if !ok {
			return nil
		}
		m = next
	}
	curve = fitCurve(fit, m)
	if curve == nil {
		return nil
	}
	l := &cameraLook{matrix: m, gray: gray, tone: curveTable(curve)}
	l.residual = lookError(held, func(s vec) vec { return l.apply(s) })
	neutral := defaultTone()
	l.neutral = lookError(held, func(s vec) vec {
		var v vec
		for c := range 3 {
			v[c] = float64(neutral.at(float32(s[c])))
		}
		return from2020.apply(v)
	})
	// A fit that is not better than the neutral look, or that misses the
	// preview by far, is not of the same picture.
	if !(l.residual < l.neutral) || l.residual > maxLookError {
		return nil
	}
	return l
}

// isGray reports whether the preview is black and white: its cells
// differ between their channels by less than a percent of their light.
func isGray(cells []lookCell) bool {
	var off int
	for _, c := range cells {
		p := c.preview
		hi := max(p[0], p[1], p[2])
		if hi > 0.02 && (hi-min(p[0], p[1], p[2]))/hi > 0.03 {
			off++
		}
	}
	return off < len(cells)/100
}

// fitCurve measures the tone curve for the matrix m: in each bin, the
// median of the preview's channels whose light before the curve falls in
// it, made rising, and 1 a stop and a half above the last bin.
func fitCurve(cells []lookCell, m mat) []float64 {
	bins := make([][]float64, lookBins)
	for _, c := range cells {
		u := m.apply(c.scene)
		for k := range 3 {
			if !(u[k] > 0) {
				continue
			}
			b := int((math.Log2(u[k]) - lookLow) * lookSteps)
			if b >= 0 && b < lookBins {
				bins[b] = append(bins[b], c.preview[k])
			}
		}
	}
	y := make([]float64, lookBins)
	w := make([]float64, lookBins)
	first, last := -1, -1
	for b, v := range bins {
		if len(v) < 12 {
			continue
		}
		slices.Sort(v)
		y[b], w[b] = v[len(v)/2], float64(len(v))
		if first < 0 {
			first = b
		}
		last = b
	}
	if first < 0 || last-first < 3*lookSteps {
		return nil
	}
	// Bins without enough cells between first and last are interpolated.
	for b := first; b <= last; b++ {
		if w[b] > 0 {
			continue
		}
		n := b + 1
		for w[n] == 0 {
			n++
		}
		p := b - 1
		y[b] = y[p] + (y[n]-y[p])*float64(b-p)/float64(n-p)
		w[b] = 1
	}
	pav(y[first:last+1], w[first:last+1])
	// Below the first bin the curve falls in proportion to the light, and
	// above the last it rises to 1 within a stop and a half.
	for b := range first {
		y[b] = y[first] * math.Exp2(float64(b-first)/lookSteps)
	}
	top := min(last+lookSteps*3/2, lookBins-1)
	for b := last + 1; b < lookBins; b++ {
		if b >= top {
			y[b] = 1
			continue
		}
		t := float64(b-last) / float64(top-last)
		y[b] = y[last] + (1-y[last])*t
	}
	for b := range y {
		y[b] = min(max(y[b], 0), 1)
	}
	return y
}

// pav makes y non-decreasing by pooling adjacent violators, weighted by w.
func pav(y, w []float64) {
	type block struct {
		v, w float64
		n    int
	}
	var s []block
	for i := range y {
		s = append(s, block{y[i], w[i], 1})
		for len(s) > 1 && s[len(s)-2].v > s[len(s)-1].v {
			a, b := s[len(s)-2], s[len(s)-1]
			tw := a.w + b.w
			s = append(s[:len(s)-2], block{(a.v*a.w + b.v*b.w) / tw, tw, a.n + b.n})
		}
	}
	i := 0
	for _, b := range s {
		for range b.n {
			y[i] = b.v
			i++
		}
	}
}

// curveAt is the measured curve at the light u.
func curveAt(curve []float64, u float64) float64 {
	if !(u > 0) {
		return 0
	}
	f := (math.Log2(u)-lookLow)*lookSteps - 0.5
	if f < 0 {
		return curve[0] * u / math.Exp2(lookLow+0.5/lookSteps)
	}
	i := int(f)
	if i >= len(curve)-1 {
		return curve[len(curve)-1]
	}
	t := f - float64(i)
	return curve[i] + t*(curve[i+1]-curve[i])
}

// curveInverse is the light the measured curve makes y of, false where the
// curve is flat at y or does not reach it.
func curveInverse(curve []float64, y float64) (float64, bool) {
	i, _ := slices.BinarySearch(curve, y)
	if i == 0 || i >= len(curve) || curve[i] == curve[i-1] {
		return 0, false
	}
	t := (y - curve[i-1]) / (curve[i] - curve[i-1])
	f := float64(i-1) + t + 0.5
	return math.Exp2(lookLow + f/lookSteps), true
}

// curveTable tabulates the measured curve for the pipe.
func curveTable(curve []float64) *toneCurve {
	return newToneCurve(func(u float64) float64 { return curveAt(curve, u) }, math.Exp2(lookHigh))
}

// fitMatrix fits the matrix that takes the scene of the cells to the light
// the curve turns into their preview, by least squares of the relative
// error, twice, the second time without the tenth of the cells that fit
// worst. For a gray preview its rows are one.
func fitMatrix(cells []lookCell, curve []float64, prev mat, gray bool) (mat, bool) {
	type pair struct {
		s vec
		t vec
	}
	var pairs []pair
	for _, c := range cells {
		var t vec
		ok := true
		for k := range 3 {
			p := c.preview[k]
			if p < 0.005 || p > 0.95 {
				ok = false
				break
			}
			if t[k], ok = curveInverse(curve, p); !ok {
				break
			}
		}
		if ok {
			pairs = append(pairs, pair{c.scene, t})
		}
	}
	if len(pairs) < 500 {
		return prev, len(pairs) > 0
	}
	solve := func(ps []pair) (mat, bool) {
		var m mat
		for k := range 3 {
			var a mat
			var b vec
			for _, p := range ps {
				t := p.t[k]
				if gray {
					t = (p.t[0] + p.t[1] + p.t[2]) / 3
				}
				x := vec{p.s[0] / t, p.s[1] / t, p.s[2] / t}
				for i := range 3 {
					for j := range 3 {
						a[i][j] += x[i] * x[j]
					}
					b[i] += x[i]
				}
			}
			inv, ok := a.inverse()
			if !ok {
				return mat{}, false
			}
			m[k] = inv.apply(b)
		}
		return m, true
	}
	m, ok := solve(pairs)
	if !ok {
		return prev, false
	}
	errs := make([]float64, len(pairs))
	for i, p := range pairs {
		u := m.apply(p.s)
		for k := range 3 {
			errs[i] += math.Abs(u[k]/p.t[k] - 1)
		}
	}
	limit := slices.Clone(errs)
	slices.Sort(limit)
	worst := limit[len(limit)*9/10]
	kept := pairs[:0]
	for i, p := range pairs {
		if errs[i] < worst {
			kept = append(kept, p)
		}
	}
	m, ok = solve(kept)
	if !ok {
		return prev, false
	}
	if gray {
		for k := 1; k < 3; k++ {
			m[k] = m[0]
		}
	}
	return m, true
}

// apply is the look of the scene s: the linear light of the preview's
// space.
func (l *cameraLook) apply(s vec) vec {
	u := l.matrix.apply(s)
	var v vec
	for c := range 3 {
		v[c] = float64(l.tone.at(float32(u[c])))
	}
	return v
}

// lookError is the mean ΔE 76 between the preview of the cells and what
// look makes of their scene, both taken as sRGB-like light for Lab.
func lookError(cells []lookCell, look func(vec) vec) float64 {
	var sum float64
	for _, c := range cells {
		a := labD50(look(c.scene))
		b := labD50(c.preview)
		sum += math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
	}
	return sum / float64(len(cells))
}

// labD50 is a CIELAB of linear RGB of sRGB's primaries, good enough to
// compare two looks by.
func labD50(v vec) vec {
	x := 0.4124*v[0] + 0.3576*v[1] + 0.1805*v[2]
	y := 0.2126*v[0] + 0.7152*v[1] + 0.0722*v[2]
	z := 0.0193*v[0] + 0.1192*v[1] + 0.9505*v[2]
	f := func(t float64) float64 {
		if t > 216.0/24389 {
			return math.Cbrt(t)
		}
		return (24389.0/27*t + 16) / 116
	}
	fx, fy, fz := f(x/0.95047), f(y), f(z/1.08883)
	return vec{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}
