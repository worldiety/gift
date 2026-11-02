package tray

import (
	"image"
	"image/color"

	"github.com/worldiety/gift/ui"
)

// tinted rasterises sym at px by px device pixels and colours it c, with the
// coverage of the mask as alpha: straight, not premultiplied, alpha. It
// returns nil for a symbol that draws nothing at that size.
//
// It is the picture Windows and Linux get; macOS takes the bare mask as a
// template image and tints it itself. A platform that needs premultiplied
// pixels converts with [premultiplied].
func tinted(sym ui.Symbol, px int, c color.NRGBA) *image.NRGBA {
	m := sym.Rasterize(px)
	if m == nil {
		return nil
	}
	n := m.Rect.Dx()
	out := image.NewNRGBA(image.Rect(0, 0, n, n))
	for i, a := range m.Pix[:n*n] {
		j := i * 4
		out.Pix[j], out.Pix[j+1], out.Pix[j+2] = c.R, c.G, c.B
		out.Pix[j+3] = uint8((int(a)*int(c.A) + 127) / 255)
	}
	return out
}

// premultiplied returns the pixels of im with every colour channel multiplied
// by its alpha, in the order R, G, B, A, row by row.
func premultiplied(im *image.NRGBA) []byte {
	out := make([]byte, len(im.Pix))
	for i := 0; i+3 < len(im.Pix); i += 4 {
		a := int(im.Pix[i+3])
		out[i] = uint8((int(im.Pix[i])*a + 127) / 255)
		out[i+1] = uint8((int(im.Pix[i+1])*a + 127) / 255)
		out[i+2] = uint8((int(im.Pix[i+2])*a + 127) / 255)
		out[i+3] = uint8(a)
	}
	return out
}
