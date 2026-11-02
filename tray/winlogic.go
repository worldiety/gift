package tray

import (
	"crypto/sha1"
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// This file holds the parts of the Windows half that are plain computation:
// the GUID of the icon, the cut of the tooltip, the text of a menu entry, the
// pixel formats GDI wants and the delays of a retry. They are in a file without a build tag so that
// their tests run on every machine, also on the Mac this package is mostly
// written on; nothing outside platform_windows.go calls them.

// winTipUnits is the number of UTF-16 units of a tooltip that
// NOTIFYICONDATAW.szTip holds, its 128 WCHARs less the terminating NUL.
const winTipUnits = 127

// winTrayGUID derives the guidItem of the notification icon from the AppID,
// as the bytes of a name-based UUID in the layout of RFC 9562, version 5:
// the first 16 bytes of the SHA-1 of a namespace string and the name, with
// the version and variant bits set. The namespace is a fixed prefix rather
// than a UUID because only the stability matters here, the same way
// gift/notify derives its activator's class id.
//
// Windows remembers the user's choice to always show an icon, or to hide it
// in the overflow, under this GUID, and only under it: an icon added without
// a GUID is identified by its window and uID, which change with every run, so
// the choice is forgotten and the icon falls back into the overflow each
// time. That is why the GUID is derived from the AppID instead of being
// random, and why an empty AppID gets no GUID at all rather than one shared
// by every application built with this package.
//
// n numbers the trays of one AppID within the process, from 1: a second icon
// of the same application needs a GUID of its own, or its NIM_ADD fails.
func winTrayGUID(appID string, n int) [16]byte {
	ns := "github.com/worldiety/gift/tray"
	if n > 1 {
		// In the namespace, not after the AppID, which may contain anything.
		ns += "#" + strconv.Itoa(n)
	}
	name := ns + ":" + appID
	sum := sha1.Sum([]byte(name))
	var u [16]byte
	copy(u[:], sum[:16])
	u[6] = u[6]&0x0f | 0x50
	u[8] = u[8]&0x3f | 0x80
	return u
}

// winCutUTF16 encodes s as UTF-16 and keeps at most limit units, without the
// terminating NUL and without splitting a surrogate pair: a lone high
// surrogate at the end would be drawn as a box. It reports whether it cut.
func winCutUTF16(s string, limit int) ([]uint16, bool) {
	u := utf16.Encode([]rune(s))
	if len(u) <= limit {
		return u, false
	}
	u = u[:limit]
	if n := len(u); n > 0 && utf16.IsSurrogate(rune(u[n-1])) && u[n-1] < 0xDC00 {
		u = u[:n-1]
	}
	return u, true
}

// winMenuText is the title of an entry as a Win32 menu reads it. An
// ampersand marks the next character as the mnemonic and is not drawn, so a
// literal one is doubled; a tab separates the title from the text of an
// accelerator, which this package does not have, so it becomes a space.
func winMenuText(title string) string {
	if !strings.ContainsAny(title, "&\t") {
		return title
	}
	return strings.NewReplacer("&", "&&", "\t", " ").Replace(title)
}

// winBGRA returns the pixels of im in the byte order of a 32-bit DIB, blue,
// green, red, alpha, row by row from the top. A menu bitmap is blended with
// premultiplied alpha, the colour bitmap of an icon carries straight alpha;
// premultiply chooses.
func winBGRA(im *image.NRGBA, premultiply bool) []byte {
	var px []byte
	if premultiply {
		px = premultiplied(im)
	} else {
		px = append([]byte(nil), im.Pix...)
	}
	for i := 0; i+3 < len(px); i += 4 {
		px[i], px[i+2] = px[i+2], px[i]
	}
	return px
}

// winANDMask returns the monochrome AND mask of an icon for im: one bit per
// pixel, most significant bit first, rows padded to 16 bits as CreateBitmap
// wants them, and a set bit where the pixel is transparent. Windows draws an
// icon with an alpha channel by its alpha and uses the mask only where alpha
// is not available, but CreateIconIndirect insists on one.
func winANDMask(im *image.NRGBA) []byte {
	w, h := im.Rect.Dx(), im.Rect.Dy()
	stride := (w + 15) / 16 * 2
	out := make([]byte, stride*h)
	for y := range h {
		for x := range w {
			if im.Pix[y*im.Stride+x*4+3] < 0x80 {
				out[y*stride+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	return out
}

// winColorRef turns a COLORREF, 0x00BBGGRR, into an opaque colour.
func winColorRef(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c), G: uint8(c >> 8), B: uint8(c >> 16), A: 0xff}
}

// winTaskbarInk is the colour of the icon on a taskbar that is light or not:
// black on light, white on dark, the way the icons of Windows itself are
// drawn.
func winTaskbarInk(light bool) color.NRGBA {
	if light {
		return color.NRGBA{A: 0xff}
	}
	return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
}

// winRetryFirst and winRetryMax bound the delay before a failed
// Shell_NotifyIconW is repeated.
const (
	winRetryFirst = 500 * time.Millisecond
	winRetryMax   = 30 * time.Second
)

// winNextRetry is the delay after d: winRetryFirst after none, then twice
// the last, never more than winRetryMax. A shell that is busy for a moment is
// asked again soon, one that refuses for good about twice a minute.
func winNextRetry(d time.Duration) time.Duration {
	return min(max(2*d, winRetryFirst), winRetryMax)
}
