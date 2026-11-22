package raw

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"runtime"
	"sync"
)

// layout is how the raw data of a DNG is stored, beyond what [Mosaic]
// says: in tiles or strips, with which predictor, through which table.
type layout struct {
	// tileW and tileH are the size of the tiles, zero for strips of
	// rowsPerStrip rows.
	tileW, tileH int
	rowsPerStrip int
	predictor    int
	sampleFormat int
	// linear is the LinearizationTable, nil for none.
	linear []uint16
	// blackH and blackV are BlackLevelDeltaH and V, the black level of
	// each column and row above Mosaic.Black; nil for none.
	blackH, blackV []float32
}

// readLayout reads the layout of the directory of the raw data.
func (f *File) readLayout(d dir) {
	t, l := f.t, &f.layout
	if w, ok := t.int(d, 0x0142); ok {
		h, _ := t.int(d, 0x0143)
		l.tileW, l.tileH = int(w), int(h)
	}
	l.rowsPerStrip = f.Mosaic.Height
	if r, ok := t.int(d, 0x0116); ok && r > 0 && r < int64(l.rowsPerStrip) {
		l.rowsPerStrip = int(r)
	}
	l.predictor = 1
	if p, ok := t.int(d, 0x013d); ok {
		l.predictor = int(p)
	}
	l.sampleFormat = 1
	if s, ok := t.int(d, 0x0153); ok {
		l.sampleFormat = int(s)
	}
	if v := t.intsOf(d, 0xc618); len(v) > 0 {
		l.linear = make([]uint16, len(v))
		for i, x := range v {
			l.linear[i] = uint16(min(max(x, 0), 0xffff))
		}
	}
	m := f.Mosaic
	if v := t.floatsOf(d, 0xc61b); len(v) == m.Width {
		l.blackH = make([]float32, len(v))
		for i, x := range v {
			l.blackH[i] = float32(x)
		}
	}
	if v := t.floatsOf(d, 0xc61c); len(v) == m.Height {
		l.blackV = make([]float32, len(v))
		for i, x := range v {
			l.blackV[i] = float32(x)
		}
	}
}

// readDNGMosaic decodes the raw data of a DNG: uncompressed of 8 to 16 bits,
// lossless JPEG or Deflate, in tiles or strips, which are decoded on all
// cores, then through the LinearizationTable.
func (f *File) readDNGMosaic() ([]uint16, error) {
	m, l := f.Mosaic, f.layout
	if l.sampleFormat != 1 {
		return nil, fmt.Errorf("%w: DNG data of floating point", ErrUnsupported)
	}
	switch m.Compression {
	case Uncompressed, LosslessJPEG, Deflate:
	default:
		return nil, fmt.Errorf("%w: DNG data in %v", ErrUnsupported, m.Compression)
	}
	if m.BitsPerSample < 1 || m.BitsPerSample > 16 {
		return nil, fmt.Errorf("%w: DNG data of %d bits", ErrUnsupported, m.BitsPerSample)
	}
	// Where each segment goes, and the width its rows have.
	type part struct {
		x, y, w, h int
		seg        segment
	}
	var parts []part
	if l.tileW > 0 {
		if l.tileH <= 0 || l.tileW > 1<<16 || l.tileH > 1<<16 {
			return nil, fmt.Errorf("%w: tiles of %dx%d", ErrFormat, l.tileW, l.tileH)
		}
		across := (m.Width + l.tileW - 1) / l.tileW
		down := (m.Height + l.tileH - 1) / l.tileH
		if len(f.segs) < across*down {
			return nil, fmt.Errorf("%w: %d tiles for %dx%d", ErrFormat, len(f.segs), across, down)
		}
		for i := range across * down {
			parts = append(parts, part{i % across * l.tileW, i / across * l.tileH, l.tileW, l.tileH, f.segs[i]})
		}
	} else {
		down := (m.Height + l.rowsPerStrip - 1) / l.rowsPerStrip
		if len(f.segs) < down {
			return nil, fmt.Errorf("%w: %d strips for %d", ErrFormat, len(f.segs), down)
		}
		for i := range down {
			parts = append(parts, part{0, i * l.rowsPerStrip, m.Width, min(l.rowsPerStrip, m.Height-i*l.rowsPerStrip), f.segs[i]})
		}
	}
	var total int64
	for _, p := range parts {
		if s := p.seg; s.off < f.t.base || s.n < 0 || s.off > f.t.end-s.n {
			return nil, fmt.Errorf("%w: raw data past the end of the file", ErrFormat)
		}
		total += p.seg.n
	}
	// Compressed data can be made to inflate to any size; the parts are
	// bounded by their place, the whole by what that many bytes of lossless
	// JPEG or Deflate can hold, a generous thousand photosites a byte.
	n := int64(m.Width) * int64(m.Height)
	if m.Compression == Uncompressed && total < n*int64(m.BitsPerSample)/8 ||
		m.Compression != Uncompressed && n > 1000*total+1<<20 {
		return nil, fmt.Errorf("%w: %d bytes for %dx%d photosites", ErrFormat, total, m.Width, m.Height)
	}

	out := make([]uint16, m.Width*m.Height)
	var (
		mu    sync.Mutex
		first error
		wg    sync.WaitGroup
		next  = make(chan part)
	)
	for range min(runtime.GOMAXPROCS(0), len(parts)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range next {
				if err := f.decodePart(out, p.x, p.y, p.w, p.h, p.seg); err != nil {
					mu.Lock()
					first = cmpErr(first, err)
					mu.Unlock()
				}
			}
		}()
	}
	for _, p := range parts {
		next <- p
	}
	close(next)
	wg.Wait()
	if first != nil {
		return nil, first
	}
	if t := l.linear; t != nil {
		last := uint16(len(t) - 1)
		for i, v := range out {
			out[i] = t[min(v, last)]
		}
	}
	return out, nil
}

func cmpErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// decodePart decodes the tile or strip in s, w by h photosites at (x, y),
// into out, leaving what lies outside the mosaic.
func (f *File) decodePart(out []uint16, x, y, w, h int, s segment) error {
	m, l := f.Mosaic, f.layout
	data := make([]byte, s.n)
	if k, _ := f.t.r.ReadAt(data, s.off-f.t.base); int64(k) != s.n {
		return fmt.Errorf("%w: raw data cut short", ErrFormat)
	}
	var samples []uint16
	switch m.Compression {
	case LosslessJPEG:
		j, err := parseLJPEG(data)
		if err != nil {
			return err
		}
		if int64(j.w)*int64(j.h)*int64(j.comps) > int64(w)*int64(h)+int64(w)*int64(j.comps) {
			return fmt.Errorf("%w: a lossless JPEG of %dx%dx%d for a tile of %dx%d", ErrFormat, j.w, j.h, j.comps, w, h)
		}
		if samples, err = j.decode(); err != nil {
			return err
		}
	case Deflate:
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("%w: Deflate: %v", ErrFormat, err)
		}
		limit := int64(w*h*((m.BitsPerSample+7)/8)) + 1
		raw, err := io.ReadAll(io.LimitReader(zr, limit))
		if err != nil {
			return fmt.Errorf("%w: Deflate: %v", ErrFormat, err)
		}
		if samples, err = f.unpack(raw, w, h); err != nil {
			return err
		}
		if err := undoPredictor(samples, w, l.predictor); err != nil {
			return err
		}
	default:
		var err error
		if samples, err = f.unpack(data, w, h); err != nil {
			return err
		}
	}
	// The samples fill the rows of the part one after the other: a lossless
	// JPEG of two components and half the width has a row of the part in
	// each of its rows.
	rows := len(samples) / w
	for r := range min(rows, h) {
		yy := y + r
		if yy >= m.Height {
			break
		}
		n := min(w, m.Width-x)
		copy(out[yy*m.Width+x:yy*m.Width+x+n], samples[r*w:r*w+n])
	}
	return nil
}

// unpack reads w by h samples of Mosaic.BitsPerSample bits: 8 as bytes, 16
// in the byte order of the file, and the others packed, the most
// significant bit first, every row from a byte boundary.
func (f *File) unpack(b []byte, w, h int) ([]uint16, error) {
	bits := f.Mosaic.BitsPerSample
	rowBytes := (w*bits + 7) / 8
	if len(b) < rowBytes*h {
		return nil, fmt.Errorf("%w: %d bytes for %dx%d samples of %d bits", ErrFormat, len(b), w, h, bits)
	}
	out := make([]uint16, w*h)
	switch bits {
	case 8:
		for i := range out {
			out[i] = uint16(b[i])
		}
	case 16:
		for i := range out {
			out[i] = f.t.bo.Uint16(b[2*i:])
		}
	default:
		for y := range h {
			row := b[y*rowBytes : (y+1)*rowBytes]
			var acc uint32
			var n, k int
			for x := range w {
				for n < bits {
					acc = acc<<8 | uint32(row[k])
					k++
					n += 8
				}
				n -= bits
				out[y*w+x] = uint16(acc >> n & (1<<bits - 1))
			}
		}
	}
	return out, nil
}

// undoPredictor undoes the horizontal differencing of TIFF (2) and of DNG
// over two and four samples (34892, 34893), row by row.
func undoPredictor(s []uint16, w, p int) error {
	var step int
	switch p {
	case 1:
		return nil
	case 2:
		step = 1
	case 34892:
		step = 2
	case 34893:
		step = 4
	default:
		return fmt.Errorf("%w: predictor %d", ErrUnsupported, p)
	}
	for y := 0; y+w <= len(s); y += w {
		row := s[y : y+w]
		for x := step; x < w; x++ {
			row[x] += row[x-step]
		}
	}
	return nil
}
