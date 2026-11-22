package raw

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"strings"
)

// Format is the kind of a raw file.
type Format uint8

const (
	// FormatUnknown is a file this package does not read.
	FormatUnknown Format = iota
	// ARW is Sony's raw format, of the Alpha cameras and others.
	ARW
	// DNG is Adobe's Digital Negative, written by some cameras and by
	// converters from the raw formats of the others.
	DNG
)

// The media types of the formats, as [Format.MIME] gives them.
const (
	MIMEARW = "image/x-sony-arw"
	MIMEDNG = "image/x-adobe-dng"
)

func (f Format) String() string {
	switch f {
	case ARW:
		return "ARW"
	case DNG:
		return "DNG"
	}
	return "unknown"
}

// MIME is the media type of the format, empty for [FormatUnknown].
func (f Format) MIME() string {
	switch f {
	case ARW:
		return MIMEARW
	case DNG:
		return MIMEDNG
	}
	return ""
}

var (
	// ErrFormat is a file that is not a raw file this package reads, or
	// one that is damaged or cut short where its structure lies.
	ErrFormat = errors.New("raw: not a raw file this package reads")
	// ErrNoPreview is a raw file without a preview that can be read.
	ErrNoPreview = errors.New("raw: the file has no preview")
	// ErrUnsupported is raw data this package does not decode, such as
	// a sensor of other colours than red, green and blue.
	ErrUnsupported = errors.New("raw: data this package does not decode")
)

// Compression is how the mosaic of a raw file is stored: the value of the
// TIFF tag Compression of its directory.
type Compression uint16

const (
	Uncompressed Compression = 1
	// LosslessJPEG is the lossless JPEG of DNG, and of Sony's lossless
	// compressed raw.
	LosslessJPEG Compression = 7
	Deflate      Compression = 8
	// SonyCompressed is Sony's compressed raw, which the cameras call
	// "Compressed": rows in blocks of 16 pixels of a colour, each block an
	// 11-bit maximum and minimum and 7-bit steps between, through a curve
	// to 14 bits.
	SonyCompressed Compression = 32767
	LossyJPEG      Compression = 34892
	JPEGXL         Compression = 52546
)

func (c Compression) String() string {
	switch c {
	case Uncompressed:
		return "uncompressed"
	case LosslessJPEG:
		return "lossless JPEG"
	case Deflate:
		return "Deflate"
	case SonyCompressed:
		return "Sony compressed"
	case LossyJPEG:
		return "lossy JPEG"
	case JPEGXL:
		return "JPEG XL"
	}
	return fmt.Sprintf("compression %d", uint16(c))
}

// File is a raw file as its metadata describes it. [Open] reads it without
// its pixels.
type File struct {
	Format Format
	// Make and Model name the camera as it names itself, such as "SONY"
	// and "ILCE-7M3".
	Make, Model string
	// Orientation is the EXIF orientation of the file, 1 to 8, and 0 when
	// it does not say. It applies to the mosaic and the previews alike,
	// which are stored as the sensor saw them.
	Orientation int

	// Mosaic describes the sensor data.
	Mosaic Mosaic
	// WhiteBalance is the white balance the photograph was taken with, as
	// the factors the red, green and blue photosites are multiplied by,
	// green 1; zero when the file does not say.
	WhiteBalance [3]float64
	// Previews are the pictures the camera, or the converter that wrote
	// a DNG, rendered from the raw data and embedded, the largest first.
	// A preview whose data the file does not hold, as in a file cut
	// short, is not listed.
	Previews []Preview

	// Sony is what a Sony camera wrote about its settings, nil for a file
	// of another make.
	Sony *Sony
	// DNG is the colour description of a DNG, nil for other formats.
	DNG *DNGColor

	t *tiff
	// segs is where the data of the mosaic lies, strips or tiles in their
	// order.
	segs []segment
	// sonyCurve is the curve of Sony's compressed raw, tag 0x7010.
	sonyCurve [4]uint16
	layout    layout
}

// Mosaic describes the sensor data of a raw file: what it covers and how
// its numbers are to be read.
type Mosaic struct {
	// Width and Height are those of the stored data, the margins the
	// sensor has beyond the picture included.
	Width, Height int
	// Crop is the part of the data that is the picture, within Width by
	// Height: DNG's default crop within its active area. The whole data
	// when the file does not say.
	Crop image.Rectangle
	// BitsPerSample is the depth the data is stored with.
	BitsPerSample int
	Compression   Compression
	// SamplesPerPixel is 1 for a mosaic behind a colour filter array and
	// 3 for the linear, already demosaiced data some DNGs hold.
	SamplesPerPixel int
	// CFA is the colour filter pattern, the colours of the 2 by 2
	// photosites at the origin of the data row by row, such as "RGGB";
	// empty when the pattern is not a 2 by 2 one of red, green and blue.
	CFA string
	// Black is the black level of each photosite of the 2 by 2 pattern,
	// row by row, in the units of the data.
	Black [4]float64
	// White is the level at which the data clips.
	White float64
}

// segment is a run of bytes of the file.
type segment struct{ off, n int64 }

// dirOf is a directory with the reader its fields are read with.
type dirOf struct {
	off int64
	d   dir
}

// Open reads the metadata of the raw file r of size bytes: its directories,
// the MakerNote and, of a Sony file, the encrypted SR2 block. It reads no
// pixels. r must stay valid while the File is used, which reads its
// previews through it.
//
// A file of a format this package does not read is [ErrFormat], as is a
// file whose directories are damaged. Damage elsewhere, in a field that
// does not matter or in data past the end of a file cut short, leaves that
// part out.
func Open(r io.ReaderAt, size int64) (*File, error) {
	var h [8]byte
	if n, _ := r.ReadAt(h[:], 0); n < 8 {
		return nil, ErrFormat
	}
	bo, first, ok := header(h[:])
	if !ok {
		return nil, ErrFormat
	}
	t := &tiff{r: r, end: size, bo: bo}
	ifd0, _, err := t.dir(first)
	if err != nil {
		return nil, err
	}
	f := &File{t: t, Format: formatOf(t, ifd0)}
	if f.Format == FormatUnknown {
		return nil, ErrFormat
	}
	f.Make = t.strOf(ifd0, 0x010f)
	f.Model = t.strOf(ifd0, 0x0110)
	if o, ok := t.int(ifd0, 0x0112); ok && o >= 1 && o <= 8 {
		f.Orientation = int(o)
	}

	dirs := walk(t, first)
	var mosaic dir
	for _, x := range dirs {
		if isMosaic(t, x.d) {
			if mosaic == nil || area(t, x.d) > area(t, mosaic) {
				mosaic = x.d
			}
		}
	}
	if mosaic == nil {
		return nil, fmt.Errorf("%w: no raw data", ErrFormat)
	}
	f.readMosaic(mosaic)

	var exif dir
	if fl, ok := ifd0[0x8769]; ok {
		exif, _, _ = t.dir(t.offset(fl))
	}
	switch f.Format {
	case ARW:
		f.readSony(ifd0, exif, mosaic)
	case DNG:
		f.readDNG(ifd0, mosaic)
	}
	f.readPreviews(dirs, exif)
	return f, nil
}

// Sniff reports the format of a raw file from its head, which must hold its
// first directory and the camera's make; the first few kilobytes do.
func Sniff(head []byte) Format {
	bo, off, ok := header(head)
	if !ok {
		return FormatUnknown
	}
	t := &tiff{r: bytes.NewReader(head), end: int64(len(head)), bo: bo}
	d, _, err := t.dir(off)
	if err != nil {
		return FormatUnknown
	}
	return formatOf(t, d)
}

// formatOf tells the format by the first directory: DNG by its version
// tag, ARW by Sony's make and the pointer to the SR2 block that every ARW
// has.
func formatOf(t *tiff, ifd0 dir) Format {
	if _, ok := ifd0[0xc612]; ok {
		return DNG
	}
	if _, ok := ifd0[0xc634]; ok && strings.HasPrefix(strings.ToUpper(t.strOf(ifd0, 0x010f)), "SONY") {
		return ARW
	}
	return FormatUnknown
}

// maxDirs bounds the directories a file is read with. A DNG has a few, an
// ARW five.
const maxDirs = 64

// walk reads the chain of directories from first and every directory their
// SubIFDs point to, each once.
func walk(t *tiff, first int64) []dirOf {
	var (
		out  []dirOf
		seen = map[int64]bool{}
	)
	var visit func(off int64, depth int, chain bool)
	visit = func(off int64, depth int, chain bool) {
		for i := 0; off != 0 && i < 16 && !seen[off] && len(seen) < maxDirs; i++ {
			seen[off] = true
			d, next, err := t.dir(off)
			if err != nil {
				return
			}
			out = append(out, dirOf{off, d})
			if depth < 4 {
				for _, sub := range t.intsOf(d, 0x014a) {
					visit(sub, depth+1, false)
				}
			}
			if !chain {
				return
			}
			off = next
		}
	}
	visit(first, 0, true)
	return out
}

// isMosaic reports whether d holds raw data: the full picture, not a
// reduced one, of photosites behind a colour filter array or, of a DNG,
// linear samples.
func isMosaic(t *tiff, d dir) bool {
	sub, _ := t.int(d, 0x00fe)
	ph, _ := t.int(d, 0x0106)
	return sub&1 == 0 && (ph == 32803 || ph == 34892)
}

func area(t *tiff, d dir) int64 {
	w, _ := t.int(d, 0x0100)
	h, _ := t.int(d, 0x0101)
	return w * h
}

func (f *File) readMosaic(d dir) {
	t, m := f.t, &f.Mosaic
	w, _ := t.int(d, 0x0100)
	h, _ := t.int(d, 0x0101)
	m.Width, m.Height = int(w), int(h)
	if b := t.intsOf(d, 0x0102); len(b) > 0 {
		m.BitsPerSample = int(b[0])
	}
	c, ok := t.int(d, 0x0103)
	if !ok {
		c = 1
	}
	m.Compression = Compression(c)
	m.SamplesPerPixel = 1
	if s, ok := t.int(d, 0x0115); ok {
		m.SamplesPerPixel = int(s)
	}
	if m.SamplesPerPixel == 1 {
		m.CFA = cfa(t, d)
	}

	// The picture within the data: DNG's default crop, relative to its
	// active area, which ARW writes too.
	full := image.Rect(0, 0, m.Width, m.Height)
	active := full
	if a := t.intsOf(d, 0xc68d); len(a) == 4 {
		active = image.Rect(int(a[1]), int(a[0]), int(a[3]), int(a[2])).Intersect(full)
	}
	m.Crop = active
	o, s := t.floatsOf(d, 0xc61f), t.floatsOf(d, 0xc620)
	if len(o) == 2 && len(s) == 2 && s[0] > 0 && s[1] > 0 {
		x, y := active.Min.X+int(math.Round(o[0])), active.Min.Y+int(math.Round(o[1]))
		if c := image.Rect(x, y, x+int(math.Round(s[0])), y+int(math.Round(s[1]))).Intersect(active); !c.Empty() {
			m.Crop = c
		}
	}

	if v := t.floatsOf(d, 0xc61d); len(v) > 0 && v[0] > 0 {
		m.White = v[0]
	} else if m.BitsPerSample > 0 && m.BitsPerSample < 32 {
		m.White = float64(int64(1)<<m.BitsPerSample - 1)
	}
	// DNG's black level, repeated over a pattern of up to 2 by 2.
	if v := t.floatsOf(d, 0xc61a); len(v) > 0 {
		rows, cols := 1, 1
		if r := t.intsOf(d, 0xc619); len(r) == 2 && r[0] >= 1 && r[0] <= 2 && r[1] >= 1 && r[1] <= 2 {
			rows, cols = int(r[0]), int(r[1])
		}
		per := m.SamplesPerPixel
		if len(v) >= rows*cols*per {
			for y := range 2 {
				for x := range 2 {
					m.Black[2*y+x] = v[((y%rows)*cols+x%cols)*per]
				}
			}
		}
	}

	f.readLayout(d)
	offs, counts := t.intsOf(d, 0x0111), t.intsOf(d, 0x0117)
	if len(offs) == 0 {
		offs, counts = t.intsOf(d, 0x0144), t.intsOf(d, 0x0145)
	}
	if len(offs) == len(counts) {
		for i := range offs {
			f.segs = append(f.segs, segment{offs[i], counts[i]})
		}
	}
}

// cfa reads the colour filter pattern of d, as "RGGB" and the like.
func cfa(t *tiff, d dir) string {
	dims, pat := t.intsOf(d, 0x828d), t.intsOf(d, 0x828e)
	if len(dims) != 2 || dims[0] != 2 || dims[1] != 2 || len(pat) != 4 {
		return ""
	}
	// DNG may say which colour each number of the pattern is; the default
	// and every camera of red, green and blue say 0, 1, 2.
	planes := []int64{0, 1, 2}
	if p := t.intsOf(d, 0xc616); len(p) > 0 {
		planes = p
	}
	var s [4]byte
	for i, c := range pat {
		if c < 0 || int(c) >= len(planes) {
			return ""
		}
		switch planes[c] {
		case 0:
			s[i] = 'R'
		case 1:
			s[i] = 'G'
		case 2:
			s[i] = 'B'
		default:
			return ""
		}
	}
	return string(s[:])
}

// byColour orders four values given for red, green, green and blue as the
// photosites of the pattern: the first green of the pattern gets the first
// green value.
func byColour(cfa string, rggb [4]float64) ([4]float64, bool) {
	if len(cfa) != 4 {
		return [4]float64{}, false
	}
	var out [4]float64
	greens := 0
	for i := range 4 {
		switch cfa[i] {
		case 'R':
			out[i] = rggb[0]
		case 'G':
			out[i] = rggb[1+min(greens, 1)]
			greens++
		case 'B':
			out[i] = rggb[3]
		}
	}
	return out, true
}

// segments are the strips or tiles of d, or the JPEG its old style tags
// point to.
func segmentsOf(t *tiff, d dir) []segment {
	offs, counts := t.intsOf(d, 0x0111), t.intsOf(d, 0x0117)
	if len(offs) == 0 || len(offs) != len(counts) {
		return nil
	}
	s := make([]segment, len(offs))
	for i := range offs {
		s[i] = segment{offs[i], counts[i]}
	}
	return s
}

// readAll reads segments one after the other into one slice.
func (t *tiff) readAll(segs []segment) ([]byte, error) {
	var n int64
	for _, s := range segs {
		if s.n < 0 || s.off < 0 {
			return nil, ErrFormat
		}
		n += s.n
	}
	if n > maxPreview {
		return nil, fmt.Errorf("%w: %d bytes of preview", ErrFormat, n)
	}
	out := make([]byte, 0, n)
	for _, s := range segs {
		if s.off > t.end-s.n {
			return nil, fmt.Errorf("%w: data past the end of the file", ErrFormat)
		}
		b := make([]byte, s.n)
		k, err := t.r.ReadAt(b, s.off-t.base)
		if int64(k) != s.n {
			if err == nil || err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

// within reports whether every segment lies in the file.
func (t *tiff) within(segs []segment) bool {
	for _, s := range segs {
		if s.off < t.base || s.n <= 0 || s.off > t.end-s.n {
			return false
		}
	}
	return len(segs) > 0
}
