package raw

import (
	"bytes"
	"cmp"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"slices"
	"sync"

	"github.com/worldiety/gift/cms"
)

// maxPreview bounds the bytes of one preview. The full size JPEG of a camera
// of a hundred megapixels is some thirty megabytes.
const maxPreview = 64 << 20

// Preview is a picture that the camera, or the converter that wrote a DNG,
// rendered from the raw data and embedded in the file: the photograph as
// the camera shows it, in its colours and with its settings. An ARW of the
// Alpha 7 III has two, of 1616 by 1080 and of 160 by 120 pixels, the
// smaller with black bars above and below.
type Preview struct {
	// Width and Height are those of the stored picture, before the
	// orientation of the file is applied.
	Width, Height int
	// JPEG reports whether the data is a JPEG. Otherwise it is RGB, three
	// bytes a pixel, row after row, as the small first picture of a DNG
	// often is.
	JPEG bool
	// Space is the colour space of the pixels as the file says, nil when
	// it does not, which means sRGB as for any picture.
	Space *cms.Profile

	segs  []segment
	model color.Model
}

// PreviewData is the data of p as it is stored, a JPEG when p.JPEG.
func (f *File) PreviewData(p Preview) ([]byte, error) {
	return f.t.readAll(p.segs)
}

// DecodePreview decodes p into an image of p.Width by p.Height.
func (f *File) DecodePreview(p Preview) (image.Image, error) {
	if p.JPEG {
		if len(p.segs) != 1 {
			return nil, ErrNoPreview
		}
		s := p.segs[0]
		return jpeg.Decode(io.NewSectionReader(f.t.r, s.off-f.t.base, s.n))
	}
	b, err := f.t.readAll(p.segs)
	if err != nil {
		return nil, err
	}
	n := p.Width * p.Height
	if len(b) < 3*n {
		return nil, fmt.Errorf("%w: a preview of %d bytes for %dx%d pixels", ErrFormat, len(b), p.Width, p.Height)
	}
	img := image.NewNRGBA(image.Rect(0, 0, p.Width, p.Height))
	for i := range n {
		copy(img.Pix[4*i:4*i+3], b[3*i:3*i+3])
		img.Pix[4*i+3] = 0xff
	}
	return img, nil
}

// Largest is the largest preview, ok false when the file has none.
func (f *File) Largest() (Preview, bool) {
	if len(f.Previews) == 0 {
		return Preview{}, false
	}
	return f.Previews[0], true
}

// readPreviews finds the previews in dirs: JPEGs where the old style tags
// JPEGInterchangeFormat point, as in an ARW, or in the strip of a reduced
// resolution directory, as in a DNG, and uncompressed RGB.
func (f *File) readPreviews(dirs []dirOf, exif dir) {
	t := f.t
	space := exifSpace(t, exif)
	var ps []Preview
	seen := map[segment]bool{}
	add := func(p Preview, ok bool) {
		if ok && !seen[p.segs[0]] {
			seen[p.segs[0]] = true
			ps = append(ps, p)
		}
	}
	for _, x := range dirs {
		d := x.d
		if isMosaic(t, d) {
			continue
		}
		sp := space
		if f.Format == DNG {
			sp = dngPreviewSpace(t, d)
		}
		if o, ok := t.int(d, 0x0201); ok {
			n, _ := t.int(d, 0x0202)
			add(f.jpegPreview([]segment{{o, n}}, sp))
			continue
		}
		segs := segmentsOf(t, d)
		switch c, _ := t.int(d, 0x0103); c {
		case 6, 7:
			add(f.jpegPreview(segs, sp))
		case 1:
			add(f.rgbPreview(d, segs, sp))
		}
	}
	slices.SortStableFunc(ps, func(a, b Preview) int {
		return cmp.Compare(b.Width*b.Height, a.Width*a.Height)
	})
	f.Previews = ps
}

// jpegPreview reads the frame header of a JPEG preview in one segment.
func (f *File) jpegPreview(segs []segment, space *cms.Profile) (Preview, bool) {
	if len(segs) != 1 || !f.t.within(segs) || segs[0].n > maxPreview {
		return Preview{}, false
	}
	s := segs[0]
	cfg, err := jpeg.DecodeConfig(io.NewSectionReader(f.t.r, s.off-f.t.base, s.n))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return Preview{}, false
	}
	if cfg.ColorModel == color.GrayModel && space == cms.SRGB() {
		space = cms.SGray()
	}
	return Preview{Width: cfg.Width, Height: cfg.Height, JPEG: true, Space: space, segs: segs, model: cfg.ColorModel}, true
}

// rgbPreview takes an uncompressed directory of 8-bit RGB as a preview.
func (f *File) rgbPreview(d dir, segs []segment, space *cms.Profile) (Preview, bool) {
	t := f.t
	ph, _ := t.int(d, 0x0106)
	spp, _ := t.int(d, 0x0115)
	planar, ok := t.int(d, 0x011c)
	if !ok {
		planar = 1
	}
	w, _ := t.int(d, 0x0100)
	h, _ := t.int(d, 0x0101)
	if ph != 2 || spp != 3 || planar != 1 || w <= 0 || h <= 0 || w*h > maxPreview/3 {
		return Preview{}, false
	}
	for _, b := range t.intsOf(d, 0x0102) {
		if b != 8 {
			return Preview{}, false
		}
	}
	var n int64
	for _, s := range segs {
		n += s.n
	}
	if n < 3*w*h || !t.within(segs) {
		return Preview{}, false
	}
	return Preview{Width: int(w), Height: int(h), Space: space, segs: segs, model: color.NRGBAModel}, true
}

// exifSpace is the colour space the EXIF of the file declares for the
// pictures the camera rendered: sRGB, or Adobe RGB as cameras say it, the
// colour space tag uncalibrated and the interoperability index R03 of the
// DCF standard, or the 2 some makers write instead. nil when it does not
// say.
func exifSpace(t *tiff, exif dir) *cms.Profile {
	if exif == nil {
		return nil
	}
	cs, _ := t.int(exif, 0xa001)
	switch cs {
	case 1:
		return cms.SRGB()
	case 2:
		return cms.AdobeRGB()
	}
	if fl, ok := exif[0xa005]; ok {
		if d, _, err := t.dir(t.offset(fl)); err == nil && t.strOf(d, 0x0001) == "R03" {
			return cms.AdobeRGB()
		}
	}
	return nil
}

// grayGamma22 is the gray of DNG's preview colour space 1.
var grayGamma22 = sync.OnceValue(func() *cms.Profile {
	p, err := cms.NewGrayProfile("Gray Gamma 2.2", cms.WhiteD65, cms.Gamma(2.2))
	if err != nil {
		panic(err)
	}
	return p
})

// dngPreviewSpace is the PreviewColorSpace of a preview directory of a DNG.
func dngPreviewSpace(t *tiff, d dir) *cms.Profile {
	v, _ := t.int(d, 0xc71a)
	switch v {
	case 1:
		return grayGamma22()
	case 2:
		return cms.SRGB()
	case 3:
		return cms.AdobeRGB()
	case 4:
		return cms.ProPhotoRGB()
	}
	return nil
}

// readerAt makes a reader of a whole file into an io.ReaderAt without
// copying when it already is one or holds the bytes.
func readerAt(r io.Reader) (io.ReaderAt, int64, error) {
	switch v := r.(type) {
	case *bytes.Buffer:
		b := v.Bytes()
		return bytes.NewReader(b), int64(len(b)), nil
	case *bytes.Reader:
		return v, v.Size(), nil
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, err
	}
	return bytes.NewReader(b), int64(len(b)), nil
}
