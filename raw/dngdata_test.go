package raw

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"image"
	"math/bits"
	"slices"
	"testing"
)

// bitWriter writes the entropy coded data of a JPEG, stuffing a 0 after
// every 0xff.
type bitWriter struct {
	out []byte
	acc uint32
	n   uint
}

func (w *bitWriter) put(v uint32, k uint) {
	for i := int(k) - 1; i >= 0; i-- {
		w.acc = w.acc<<1 | v>>uint(i)&1
		w.n++
		if w.n == 8 {
			w.byte(byte(w.acc))
			w.acc, w.n = 0, 0
		}
	}
}

func (w *bitWriter) byte(b byte) {
	w.out = append(w.out, b)
	if b == 0xff {
		w.out = append(w.out, 0)
	}
}

// flush pads the last byte with ones.
func (w *bitWriter) flush() {
	for w.n != 0 {
		w.put(1, 1)
	}
}

// testTable is a Huffman table for the encoder: counts of codes of each
// length and the categories they code.
type testTable struct {
	counts [16]int
	vals   []byte
}

func (tt testTable) codes() map[int][2]uint32 {
	m := map[int][2]uint32{}
	code, k := uint32(0), 0
	for l := 1; l <= 16; l++ {
		for range tt.counts[l-1] {
			m[int(tt.vals[k])] = [2]uint32{code, uint32(l)}
			code++
			k++
		}
		code <<= 1
	}
	return m
}

var (
	// Every category in five bits.
	flatTable = testTable{counts: [16]int{4: 17}, vals: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}
	// Codes of 3 to 6 bits, the short ones for the small categories.
	skewedTable = testTable{counts: [16]int{2: 4, 3: 4, 4: 4, 5: 5}, vals: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}
	// Codes of 1 to 16 bits, longer than the decoder looks up at once.
	longTable = testTable{counts: [16]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, vals: []byte{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0}}
)

// encodeLJPEG writes s, w by h pixels of c components, as a lossless JPEG of
// the given precision and predictor, as T.81 has it: a table per component,
// a restart every restart rows when not 0.
func encodeLJPEG(s []uint16, w, h, c, precision, pred, restartRows int, tables []testTable) []byte {
	var b []byte
	seg := func(m byte, data []byte) {
		b = append(b, 0xff, m)
		b = binary.BigEndian.AppendUint16(b, uint16(len(data)+2))
		b = append(b, data...)
	}
	b = append(b, 0xff, 0xd8)
	sof := []byte{byte(precision)}
	sof = binary.BigEndian.AppendUint16(sof, uint16(h))
	sof = binary.BigEndian.AppendUint16(sof, uint16(w))
	sof = append(sof, byte(c))
	for k := range c {
		sof = append(sof, byte(k+1), 0x11, 0)
	}
	seg(0xc3, sof)
	for id, tt := range tables {
		d := []byte{byte(id)}
		for _, n := range tt.counts {
			d = append(d, byte(n))
		}
		seg(0xc4, append(d, tt.vals...))
	}
	if restartRows > 0 {
		seg(0xdd, binary.BigEndian.AppendUint16(nil, uint16(restartRows*w)))
	}
	sos := []byte{byte(c)}
	for k := range c {
		sos = append(sos, byte(k+1), byte(k%len(tables))<<4)
	}
	seg(0xda, append(sos, byte(pred), 0, 0))

	codes := make([]map[int][2]uint32, c)
	for k := range c {
		codes[k] = tables[k%len(tables)].codes()
	}
	bw := &bitWriter{}
	stride := w * c
	interval := h
	if restartRows > 0 {
		interval = restartRows
	}
	rst := 0
	for y := range h {
		top := y%interval == 0
		if top && y > 0 {
			bw.flush()
			bw.out = append(bw.out, 0xff, byte(0xd0+rst%8))
			rst++
		}
		for x := range w {
			for k := range c {
				i := y*stride + x*c + k
				var p int
				switch {
				case top && x == 0:
					p = 1 << (precision - 1)
				case top:
					p = int(s[i-c])
				case x == 0:
					p = int(s[i-stride])
				default:
					ra, rb, rc := int(s[i-c]), int(s[i-stride]), int(s[i-stride-c])
					p = [8]int{0, ra, rb, rc, ra + rb - rc, ra + (rb-rc)>>1, rb + (ra-rc)>>1, (ra + rb) >> 1}[pred]
				}
				d := int(int16(uint16(int(s[i]) - p)))
				var t uint
				var v uint32
				switch {
				case d == -32768:
					t = 16
				case d > 0:
					t = uint(bits.Len(uint(d)))
					v = uint32(d)
				case d < 0:
					t = uint(bits.Len(uint(-d)))
					v = uint32(d + 1<<t - 1)
				}
				code := codes[k][int(t)]
				bw.put(code[0], uint(code[1]))
				if t > 0 && t < 16 {
					bw.put(v, t)
				}
			}
		}
	}
	bw.flush()
	b = append(b, bw.out...)
	return append(b, 0xff, 0xd9)
}

// testMosaic is a mosaic of 12 bits with detail and noise.
func testMosaic(w, h int) []uint16 {
	m := make([]uint16, w*h)
	seed := uint32(7)
	for y := range h {
		for x := range w {
			seed = seed*1664525 + 1013904223
			v := 600 + 30*x + 20*y + int(seed>>24)
			if (x/5+y/3)%2 == 0 {
				v += 1500
			}
			m[y*w+x] = uint16(min(v, 4095))
		}
	}
	return m
}

// The decoder decodes what T.81 encodes: every predictor, one component and
// several, codes longer than the lookup, restarts.
func TestLosslessJPEG(t *testing.T) {
	const w, h = 24, 10
	for c := 1; c <= 3; c++ {
		s := testMosaic(w*c, h)
		for pred := 1; pred <= 7; pred++ {
			for _, restart := range []int{0, 2} {
				tables := []testTable{flatTable, skewedTable, longTable}[:c]
				enc := encodeLJPEG(s, w, h, c, 12, pred, restart, tables)
				j, err := parseLJPEG(enc)
				if err != nil {
					t.Fatalf("%d components, predictor %d: %v", c, pred, err)
				}
				got, err := j.decode()
				if err != nil {
					t.Fatalf("%d components, predictor %d: %v", c, pred, err)
				}
				if !slices.Equal(got, s) {
					t.Errorf("%d components, predictor %d, restart %d: decodes otherwise", c, pred, restart)
				}
			}
		}
	}
	// A difference of 32768, the category without extra bits.
	s := []uint16{0, 32768, 0, 32768}
	j, err := parseLJPEG(encodeLJPEG(s, 4, 1, 1, 16, 1, 0, []testTable{flatTable}))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := j.decode(); err != nil || !slices.Equal(got, s) {
		t.Errorf("%v, %v", got, err)
	}
	// Not lossless, or damaged.
	for _, b := range [][]byte{nil, {0xff, 0xd8}, {0xff, 0xd8, 0xff, 0xc0, 0, 2}} {
		if _, err := parseLJPEG(b); err == nil {
			t.Errorf("%x parsed", b)
		}
	}
}

// buildDNGData makes a DNG of a w by h mosaic of RGGB whose directory has
// the entries of the raw data given, with its data written by data.
func buildDNGData(w, h int, entries func(tw *tiffWriter) []tiffEntry) []byte {
	tw := &tiffWriter{b: []byte("II*\x00\x00\x00\x00\x00")}
	es := append([]tiffEntry{
		longs(0x00fe, 0), longs(0x0100, uint32(w)), longs(0x0101, uint32(h)),
		shorts(0x0106, 32803), shorts(0x0115, 1),
		shorts(0x828d, 2, 2), bytesOf(0x828e, 0, 1, 1, 2), longs(0xc61d, 4095),
	}, entries(tw)...)
	raw := tw.dir(es...)
	ifd0 := tw.dir(ascii(0x010f, "Testmaker"), ascii(0x0110, "T2"), bytesOf(0xc612, 1, 4, 0, 0), longs(0x014a, raw))
	binary.LittleEndian.PutUint32(tw.b[4:], ifd0)
	return tw.b
}

// tiles cuts a mosaic into tiles of tw by th, padded with zeros, each made
// into bytes by enc.
func tiles(m []uint16, w, h, tw, th int, enc func(tile []uint16) []byte) [][]byte {
	var out [][]byte
	for ty := 0; ty < h; ty += th {
		for tx := 0; tx < w; tx += tw {
			tile := make([]uint16, tw*th)
			for y := range th {
				for x := range tw {
					if tx+x < w && ty+y < h {
						tile[y*tw+x] = m[(ty+y)*w+tx+x]
					}
				}
			}
			out = append(out, enc(tile))
		}
	}
	return out
}

// tileEntries writes the tiles and gives the entries that say where they
// are.
func tileEntries(tw *tiffWriter, ts [][]byte, w, h int) []tiffEntry {
	var offs, counts []uint32
	for _, t := range ts {
		offs = append(offs, tw.blob(t))
		counts = append(counts, uint32(len(t)))
	}
	return []tiffEntry{longs(0x0142, uint32(w)), longs(0x0143, uint32(h)), longs(0x0144, offs...), longs(0x0145, counts...)}
}

func le16(s []uint16) []byte {
	b := make([]byte, 2*len(s))
	for i, v := range s {
		binary.LittleEndian.PutUint16(b[2*i:], v)
	}
	return b
}

func deflate(b []byte) []byte {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(b)
	zw.Close()
	return z.Bytes()
}

// The raw data of a DNG decodes as it was stored, in every way a DNG
// stores it.
func TestDNGDataIsDecoded(t *testing.T) {
	const w, h = 71, 37
	m := testMosaic(w, h)
	for _, c := range []struct {
		name    string
		entries func(tw *tiffWriter) []tiffEntry
		want    []uint16
	}{
		{"16 bits in strips", func(tw *tiffWriter) []tiffEntry {
			var offs, counts []uint32
			for y := 0; y < h; y += 8 {
				b := le16(m[y*w : min(y+8, h)*w])
				offs = append(offs, tw.blob(b))
				counts = append(counts, uint32(len(b)))
			}
			return []tiffEntry{shorts(0x0102, 16), shorts(0x0103, 1), longs(0x0111, offs...), longs(0x0116, 8), longs(0x0117, counts...)}
		}, m},
		{"12 bits packed, rows padded", func(tw *tiffWriter) []tiffEntry {
			var b []byte
			for y := range h {
				bw := &bitWriter{}
				for x := range w {
					bw.put(uint32(m[y*w+x]), 12)
				}
				for bw.n != 0 {
					bw.put(0, 1)
				}
				// Packed data is not stuffed: undo the writer's 0 after 0xff.
				b = append(b, bytes.ReplaceAll(bw.out, []byte{0xff, 0}, []byte{0xff})...)
			}
			return []tiffEntry{shorts(0x0102, 12), shorts(0x0103, 1), longs(0x0111, tw.blob(b)), longs(0x0117, uint32(len(b)))}
		}, m},
		{"8 bits through a table", func(tw *tiffWriter) []tiffEntry {
			b := make([]byte, w*h)
			for i, v := range m {
				b[i] = byte(v >> 4)
			}
			table := make([]uint16, 256)
			for i := range table {
				table[i] = uint16(i * 16)
			}
			return []tiffEntry{shorts(0x0102, 8), shorts(0x0103, 1), longs(0x0111, tw.blob(b)), longs(0x0117, uint32(len(b))), shorts(0xc618, table...)}
		}, func() []uint16 {
			s := make([]uint16, len(m))
			for i, v := range m {
				s[i] = v >> 4 << 4
			}
			return s
		}()},
		{"lossless JPEG in tiles", func(tw *tiffWriter) []tiffEntry {
			ts := tiles(m, w, h, 32, 16, func(tile []uint16) []byte {
				return encodeLJPEG(tile, 32, 16, 1, 12, 1, 0, []testTable{skewedTable})
			})
			return append([]tiffEntry{shorts(0x0102, 12), shorts(0x0103, 7)}, tileEntries(tw, ts, 32, 16)...)
		}, m},
		{"lossless JPEG of two components, half as wide", func(tw *tiffWriter) []tiffEntry {
			ts := tiles(m, w, h, 32, 16, func(tile []uint16) []byte {
				return encodeLJPEG(tile, 16, 16, 2, 12, 6, 4, []testTable{flatTable, longTable})
			})
			return append([]tiffEntry{shorts(0x0102, 12), shorts(0x0103, 7)}, tileEntries(tw, ts, 32, 16)...)
		}, m},
		{"Deflate, horizontal differences", func(tw *tiffWriter) []tiffEntry {
			ts := tiles(m, w, h, 32, 16, func(tile []uint16) []byte {
				d := slices.Clone(tile)
				for y := range 16 {
					for x := 31; x > 0; x-- {
						d[y*32+x] -= d[y*32+x-1]
					}
				}
				return deflate(le16(d))
			})
			return append([]tiffEntry{shorts(0x0102, 16), shorts(0x0103, 8), shorts(0x013d, 2)}, tileEntries(tw, ts, 32, 16)...)
		}, m},
		{"Deflate, differences over two", func(tw *tiffWriter) []tiffEntry {
			ts := tiles(m, w, h, 32, 16, func(tile []uint16) []byte {
				d := slices.Clone(tile)
				for y := range 16 {
					for x := 31; x > 1; x-- {
						d[y*32+x] -= d[y*32+x-2]
					}
				}
				return deflate(le16(d))
			})
			return append([]tiffEntry{shorts(0x0102, 16), shorts(0x0103, 8), longs(0x013d, 34892)}, tileEntries(tw, ts, 32, 16)...)
		}, m},
	} {
		b := buildDNGData(w, h, c.entries)
		f, err := Open(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, err := f.ReadMosaic()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !slices.Equal(got, c.want) {
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("%s: photosite (%d, %d) is %d, want %d", c.name, i%w, i/w, got[i], c.want[i])
					break
				}
			}
		}
	}
}

// A DNG that claims more photosites than its data can hold is refused
// before the mosaic is allocated.
func TestDNGDataThatCannotBe(t *testing.T) {
	for _, c := range []struct {
		name    string
		entries func(tw *tiffWriter) []tiffEntry
	}{
		{"uncompressed", func(tw *tiffWriter) []tiffEntry {
			return []tiffEntry{shorts(0x0102, 16), shorts(0x0103, 1), longs(0x0111, tw.blob(make([]byte, 64))), longs(0x0117, 64)}
		}},
		{"lossless JPEG", func(tw *tiffWriter) []tiffEntry {
			return []tiffEntry{shorts(0x0102, 16), shorts(0x0103, 7), longs(0x0111, tw.blob(make([]byte, 64))), longs(0x0117, 64)}
		}},
	} {
		b := buildDNGData(20000, 20000, c.entries)
		f, err := Open(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.ReadMosaic(); err == nil {
			t.Errorf("%s: 400 megapixels in 64 bytes", c.name)
		}
	}
}

// DNG's black level of each column and row adds to that of the pattern.
func TestDNGBlackLevelDeltas(t *testing.T) {
	const w, h = 40, 30
	m := make([]uint16, w*h)
	for i := range m {
		m[i] = 1000
	}
	deltaH := make([]int32, 2*w)
	for x := range w {
		deltaH[2*x], deltaH[2*x+1] = int32(x), 1
	}
	deltaV := make([]int32, 2*h)
	for y := range h {
		deltaV[2*y], deltaV[2*y+1] = 100, 1
	}
	b := buildDNGData(w, h, func(tw *tiffWriter) []tiffEntry {
		d := le16(m)
		return []tiffEntry{shorts(0x0102, 16), shorts(0x0103, 1), longs(0x0111, tw.blob(d)), longs(0x0117, uint32(len(d))),
			srationals(0xc61b, deltaH...), srationals(0xc61c, deltaV...)}
	})
	f, err := Open(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	f.WhiteBalance = [3]float64{1, 1, 1}
	l, err := f.DevelopPlanes(Options{Half: true, Output: OutputCamera})
	if err != nil {
		t.Fatal(err)
	}
	if l.Rect != image.Rect(0, 0, 20, 15) {
		t.Fatalf("%v", l.Rect)
	}
	// Block x averages columns 2x and 2x+1 and their neighbours; the
	// black there is about 100 + 2x.
	for _, x := range []int{3, 10, 16} {
		got := float64(l.G[7*20+x])
		want := (1000 - 100 - float64(2*x) - 0.5) / 4095
		if d := got - want; d > 0.001 || d < -0.001 {
			t.Errorf("block %d: %.4f, want %.4f", x, got, want)
		}
	}
	_ = fmt.Sprint
}
