package raw

import (
	"fmt"
	"image"
	"runtime"
	"sync"
)

// Options says how [File.Develop] develops the raw data.
type Options struct {
	// Half develops at half the width and height, each 2 by 2 block of
	// photosites one pixel, without demosaicing: a fourth of the work, and
	// as sharp as a picture of half the size can be.
	Half bool
	// WhiteBalance replaces the white balance the photograph was taken
	// with, as the factors of red, green and blue; zero keeps the
	// photograph's own.
	WhiteBalance [3]float64
	// Highlights is what becomes of photosites that clipped.
	Highlights Highlights
	// Workers bounds the goroutines that develop tiles; zero is GOMAXPROCS.
	Workers int
}

// Highlights is what [File.Develop] does with photosites that clipped.
//
// White balance multiplies red and blue by more than green, so that where
// the sensor clipped, green stops at a lower level than the others and a
// white sky turns magenta. Both modes prevent that.
type Highlights uint8

const (
	// HighlightsReconstruct estimates a channel that clipped from those
	// that did not around it, by the colour the pixels next to the
	// clipped areas have, and makes what clipped in all three white at
	// the level of the brightest channel: blown highlights stay brighter
	// than what surrounds them, and their edges keep their colour.
	HighlightsReconstruct Highlights = iota
	// HighlightsClip clips every channel at the level the first one
	// clips, which is white, as most raw converters' simplest mode does.
	// Bright saturated colours above it lose some of their saturation.
	HighlightsClip
)

// Tile is a part of the developed picture: linear light in the colours of
// the camera, white balanced, black at 0 and the level at which the first
// channel clips, which is white, at 1. Brighter colours, and highlights
// that were reconstructed, are above 1.
type Tile struct {
	// Rect is where the tile lies in the developed picture, whose origin
	// is the top left corner of the crop.
	Rect image.Rectangle
	// R, G and B are the three channels, row after row, Stride values
	// from one row to the next.
	R, G, B []float32
	Stride  int
}

// tileSize is the side of the tiles develop works in, in output pixels,
// and tileBorder the photosites around a tile that demosaicing reads.
const (
	tileSize   = 256
	tileBorder = 12
)

// Develop develops the raw data of f into the picture its crop shows:
// levels, white balance, highlights and demosaicing, in tiles of float32
// that are handed to sink as they are done. sink is called from several
// goroutines at once, and a tile's planes are reused once it returns.
//
// The developed picture is Mosaic.Crop's size, or half that with
// [Options.Half], before the orientation of the file is applied; [Bounds]
// tells it.
func (f *File) Develop(opt Options, sink func(Tile)) error {
	raw, err := f.ReadMosaic()
	if err != nil {
		return err
	}
	d, err := newDeveloper(f, raw, opt)
	if err != nil {
		return err
	}
	return d.run(sink)
}

// Bounds is the size of the picture [File.Develop] makes with opt.
func (f *File) Bounds(opt Options) image.Rectangle {
	c := f.Mosaic.Crop
	if opt.Half {
		return image.Rect(0, 0, c.Dx()/2, c.Dy()/2)
	}
	return image.Rect(0, 0, c.Dx(), c.Dy())
}

// Linear is a developed picture held whole: three planes of float32, as
// [Tile] describes them.
type Linear struct {
	Rect    image.Rectangle
	R, G, B []float32
}

// DevelopLinear develops f into one [Linear], twelve bytes a pixel: some
// 290 megabytes for 24 megapixels, which is what [File.Develop] avoids.
func (f *File) DevelopLinear(opt Options) (*Linear, error) {
	r := f.Bounds(opt)
	n := r.Dx() * r.Dy()
	l := &Linear{Rect: r, R: make([]float32, n), G: make([]float32, n), B: make([]float32, n)}
	err := f.Develop(opt, func(t Tile) {
		for y := range t.Rect.Dy() {
			o := (t.Rect.Min.Y+y)*r.Dx() + t.Rect.Min.X
			s := y * t.Stride
			w := t.Rect.Dx()
			copy(l.R[o:o+w], t.R[s:s+w])
			copy(l.G[o:o+w], t.G[s:s+w])
			copy(l.B[o:o+w], t.B[s:s+w])
		}
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// developer holds what every tile is developed with.
type developer struct {
	opt  Options
	raw  []uint16
	w, h int
	crop image.Rectangle
	// colour is the colour, 0 red, 1 green, 2 blue, of the photosite at
	// (x, y) of the stored data, by (y&1)*2 + x&1.
	colour [4]int
	// black and scale make a stored value v linear light, white
	// balanced: (v-black)*scale; clipAt is the stored value from which a
	// photosite counts as clipped.
	black, scale, clipAt [4]float32
	// level is the value each colour clips at after white balance, the
	// lowest 1; top is the highest.
	level [3]float32
	top   float32
	// clip marks, for each 2 by 2 block of the stored data, the colours
	// that clipped in it (bits 1, 2, 4), and bit 8 for a block next to
	// one that clipped; nil when nothing clipped.
	clip   []uint8
	bw, bh int
	ratio  [3]float32
	// blackH and blackV are DNG's black level of each column and row
	// above black, nil for none.
	blackH, blackV []float32
}

func newDeveloper(f *File, raw []uint16, opt Options) (*developer, error) {
	m := f.Mosaic
	d := &developer{opt: opt, raw: raw, w: m.Width, h: m.Height, crop: m.Crop,
		blackH: f.layout.blackH, blackV: f.layout.blackV}
	if d.crop.Empty() || !d.crop.In(image.Rect(0, 0, d.w, d.h)) {
		return nil, fmt.Errorf("%w: crop %v of %dx%d", ErrFormat, d.crop, d.w, d.h)
	}
	for p := range 4 {
		switch m.CFA[p] {
		case 'R':
			d.colour[p] = 0
		case 'G':
			d.colour[p] = 1
		case 'B':
			d.colour[p] = 2
		}
	}
	wb := opt.WhiteBalance
	if wb == ([3]float64{}) {
		wb = f.WhiteBalance
	}
	if wb[0] <= 0 || wb[1] <= 0 || wb[2] <= 0 {
		wb = [3]float64{1, 1, 1}
	}
	lo := min(wb[0], wb[1], wb[2])
	for c := range 3 {
		d.level[c] = float32(wb[c] / lo)
	}
	d.top = max(d.level[0], d.level[1], d.level[2])
	white := m.White
	for p := range 4 {
		b := m.Black[p]
		if white <= b {
			return nil, fmt.Errorf("%w: white %v at or below black %v", ErrFormat, white, b)
		}
		d.black[p] = float32(b)
		d.scale[p] = d.level[d.colour[p]] / float32(white-b)
		d.clipAt[p] = float32(b + clipFraction*(white-b))
	}
	if opt.Highlights == HighlightsClip {
		d.top = 1
		return d, nil
	}
	d.markClipped()
	if d.clip != nil {
		d.ratio = d.borderRatios()
	}
	return d, nil
}

// clipFraction is how close to white a photosite counts as clipped. Sony's
// compressed raw steps by up to 3 percent near white, so a photosite that
// clipped may be decoded a little below it.
const clipFraction = 0.97

// value is the photosite at (x, y) of the stored data, linear and white
// balanced, mirrored at the edges of the data so that it keeps its colour.
func (d *developer) value(x, y int) float32 {
	x, y = mirror(x, d.w), mirror(y, d.h)
	p := (y&1)*2 + x&1
	b := d.black[p]
	if d.blackH != nil {
		b += d.blackH[x]
	}
	if d.blackV != nil {
		b += d.blackV[y]
	}
	return max(0, (float32(d.raw[y*d.w+x])-b)*d.scale[p])
}

func (d *developer) clipped(x, y int) bool {
	x, y = mirror(x, d.w), mirror(y, d.h)
	return float32(d.raw[y*d.w+x]) >= d.clipAt[(y&1)*2+x&1]
}

// mirror reflects i into [0, n) by an even amount.
func mirror(i, n int) int {
	for i < 0 || i >= n {
		if i < 0 {
			i = -i
		}
		if i >= n {
			i = 2*(n-1) - i
		}
	}
	return i
}

// markClipped makes the map of the blocks that clipped, and of those next
// to them, or leaves it nil when nothing did.
func (d *developer) markClipped() {
	d.bw, d.bh = (d.w+1)/2, (d.h+1)/2
	var mask []uint8
	for by := range d.bh {
		for bx := range d.bw {
			var bits uint8
			for p := range 4 {
				x, y := 2*bx+p&1, 2*by+p>>1
				if x < d.w && y < d.h && d.clipped(x, y) {
					bits |= 1 << d.colour[(y&1)*2+x&1]
				}
			}
			if bits != 0 {
				if mask == nil {
					mask = make([]uint8, d.bw*d.bh)
				}
				mask[by*d.bw+bx] = bits
			}
		}
	}
	if mask == nil {
		return
	}
	near := make([]uint8, len(mask))
	for by := range d.bh {
		for bx := range d.bw {
			if mask[by*d.bw+bx]&7 == 0 {
				continue
			}
			for y := max(by-1, 0); y <= min(by+1, d.bh-1); y++ {
				for x := max(bx-1, 0); x <= min(bx+1, d.bw-1); x++ {
					near[y*d.bw+x] = 8
				}
			}
		}
	}
	for i := range mask {
		mask[i] |= near[i]
	}
	d.clip = mask
}

// block is the clip marks of the block of (x, y).
func (d *developer) block(x, y int) uint8 {
	if d.clip == nil {
		return 0
	}
	x, y = mirror(x, d.w), mirror(y, d.h)
	return d.clip[(y/2)*d.bw+x/2]
}

// borderRatios measures the colour of the pixels around clipped areas: for
// each colour, its ratio to the mean of the other two, over the blocks that
// did not clip but lie within two of one that did. A clipped channel is
// estimated as that ratio times the mean of the others.
func (d *developer) borderRatios() [3]float32 {
	var sum [3]float64
	var n int
	for by := range d.bh {
		for bx := range d.bw {
			if d.clip[by*d.bw+bx]&7 != 0 || !d.nearClipped(bx, by, 2) {
				continue
			}
			v := d.blockRGB(bx, by)
			for c := range 3 {
				o := (v[(c+1)%3] + v[(c+2)%3]) / 2
				if o <= 0.05 {
					goto next
				}
				sum[c] += float64(min(max(v[c]/o, 0.25), 4))
			}
			n++
		next:
		}
	}
	r := [3]float32{1, 1, 1}
	if n > 0 {
		for c := range 3 {
			r[c] = float32(sum[c] / float64(n))
		}
	}
	return r
}

func (d *developer) nearClipped(bx, by, dist int) bool {
	for y := max(by-dist, 0); y <= min(by+dist, d.bh-1); y++ {
		for x := max(bx-dist, 0); x <= min(bx+dist, d.bw-1); x++ {
			if d.clip[y*d.bw+x]&7 != 0 {
				return true
			}
		}
	}
	return false
}

// blockRGB is the block's red, the mean of its greens and its blue.
func (d *developer) blockRGB(bx, by int) [3]float32 {
	var v [3]float32
	for p := range 4 {
		x, y := 2*bx+p&1, 2*by+p>>1
		c := d.colour[(mirror(y, d.h)&1)*2+mirror(x, d.w)&1]
		if c == 1 {
			v[1] += d.value(x, y) / 2
		} else {
			v[c] += d.value(x, y)
		}
	}
	return v
}

// highlight is the photosite at (x, y) with the highlights treated.
func (d *developer) highlight(x, y int) float32 {
	v := d.value(x, y)
	if d.opt.Highlights == HighlightsClip {
		return min(v, 1)
	}
	if d.block(x, y) == 0 || !d.clipped(x, y) {
		return v
	}
	// The other colours around: the mean of each over the 3 by 3
	// photosites, and whether they clipped too.
	c := d.colourAt(x, y)
	var sum [3]float32
	var n [3]int
	all := true
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			k := d.colourAt(x+dx, y+dy)
			if k == c {
				continue
			}
			sum[k] += d.value(x+dx, y+dy)
			n[k]++
			if !d.clipped(x+dx, y+dy) {
				all = false
			}
		}
	}
	if all {
		return d.top
	}
	var o float32
	var k int
	for i := range 3 {
		if n[i] > 0 {
			o += sum[i] / float32(n[i])
			k++
		}
	}
	o /= float32(k)
	return min(max(v, o*d.ratio[c]), d.top)
}

func (d *developer) colourAt(x, y int) int {
	return d.colour[(mirror(y, d.h)&1)*2+mirror(x, d.w)&1]
}

// run develops the tiles of the picture on the workers.
func (d *developer) run(sink func(Tile)) error {
	out := image.Rect(0, 0, d.crop.Dx(), d.crop.Dy())
	if d.opt.Half {
		out = image.Rect(0, 0, d.crop.Dx()/2, d.crop.Dy()/2)
	}
	var tiles []image.Rectangle
	for y := out.Min.Y; y < out.Max.Y; y += tileSize {
		for x := out.Min.X; x < out.Max.X; x += tileSize {
			tiles = append(tiles, image.Rect(x, y, x+tileSize, y+tileSize).Intersect(out))
		}
	}
	workers := d.opt.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	workers = min(workers, len(tiles))
	next := make(chan image.Rectangle)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var buf tileBuffers
			for r := range next {
				if d.opt.Half {
					sink(d.half(r, &buf))
				} else {
					sink(d.full(r, &buf))
				}
			}
		}()
	}
	for _, r := range tiles {
		next <- r
	}
	close(next)
	wg.Wait()
	return nil
}

// tileBuffers are the planes a worker develops its tiles in, kept from
// tile to tile.
type tileBuffers struct {
	out [3][]float32
	rcd rcd
}

func grow(b []float32, n int) []float32 {
	if cap(b) < n {
		return make([]float32, n)
	}
	return b[:n]
}

// half develops the output tile r at half size: each pixel the 2 by 2 block
// of photosites at twice its place in the crop. Red and blue lie half a
// photosite off the centre of the block, on opposite sides, which would
// fringe every edge with colour; each is interpolated to the centre from
// the four nearest of its colour, 9, 3, 3 and 1 sixteenths. The two greens
// lie on either side of it, and their mean is there.
func (d *developer) half(r image.Rectangle, buf *tileBuffers) Tile {
	n := r.Dx() * r.Dy()
	for c := range 3 {
		buf.out[c] = grow(buf.out[c], n)
	}
	i := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			sx, sy := d.crop.Min.X+2*x, d.crop.Min.Y+2*y
			var v [3]float32
			for p := range 4 {
				ox, oy := p&1, p>>1
				px, py := sx+ox, sy+oy
				c := d.colourAt(px, py)
				if c == 1 {
					v[1] += d.highlight(px, py) / 2
					continue
				}
				dx, dy := 2-4*ox, 2-4*oy
				v[c] = (9*d.highlight(px, py) + 3*d.highlight(px+dx, py) +
					3*d.highlight(px, py+dy) + d.highlight(px+dx, py+dy)) / 16
			}
			buf.out[0][i], buf.out[1][i], buf.out[2][i] = v[0], v[1], v[2]
			i++
		}
	}
	return Tile{Rect: r, R: buf.out[0], G: buf.out[1], B: buf.out[2], Stride: r.Dx()}
}

// full develops the output tile r at full size, demosaiced by RCD over the
// photosites of the tile and a border around it.
func (d *developer) full(r image.Rectangle, buf *tileBuffers) Tile {
	x0, y0 := d.crop.Min.X+r.Min.X-tileBorder, d.crop.Min.Y+r.Min.Y-tileBorder
	bw, bh := r.Dx()+2*tileBorder, r.Dy()+2*tileBorder
	rc := &buf.rcd
	rc.resize(bw, bh)
	// RCD works on values from 0 to 1.
	inv := 1 / d.top
	for y := range bh {
		for x := range bw {
			rc.cfa[y*bw+x] = d.highlight(x0+x, y0+y) * inv
		}
	}
	for p := range 4 {
		rc.colour[p] = d.colourAt(x0+p&1, y0+p>>1)
	}
	rc.demosaic()

	n := r.Dx() * r.Dy()
	for c := range 3 {
		buf.out[c] = grow(buf.out[c], n)
		o := buf.out[c]
		src := rc.rgb[c]
		for y := range r.Dy() {
			row := src[(y+tileBorder)*bw+tileBorder:]
			for x := range r.Dx() {
				o[y*r.Dx()+x] = row[x] * d.top
			}
		}
	}
	return Tile{Rect: r, R: buf.out[0], G: buf.out[1], B: buf.out[2], Stride: r.Dx()}
}
