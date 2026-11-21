package cms

import "fmt"

// Space is the colour space of the data on one side of a profile, as the
// ICC signature in the profile header.
type Space uint32

// The colour spaces this package works with. A profile describes a device
// in RGB, Gray or CMYK and connects it to XYZ or Lab, its profile
// connection space.
const (
	SpaceXYZ  Space = 'X'<<24 | 'Y'<<16 | 'Z'<<8 | ' '
	SpaceLab  Space = 'L'<<24 | 'a'<<16 | 'b'<<8 | ' '
	SpaceRGB  Space = 'R'<<24 | 'G'<<16 | 'B'<<8 | ' '
	SpaceGray Space = 'G'<<24 | 'R'<<16 | 'A'<<8 | 'Y'
	SpaceCMYK Space = 'C'<<24 | 'M'<<16 | 'Y'<<8 | 'K'
)

// Channels is the number of channels of a colour in the space, or 0 for a
// space this package does not handle.
func (s Space) Channels() int {
	switch s {
	case SpaceGray:
		return 1
	case SpaceRGB, SpaceXYZ, SpaceLab:
		return 3
	case SpaceCMYK:
		return 4
	}
	return 0
}

func (s Space) String() string { return sigString(uint32(s)) }

// Class is the device class of a profile.
type Class uint32

// The device classes of the ICC specification. This package transforms
// with input, display, output and colour space profiles.
const (
	ClassInput      Class = 's'<<24 | 'c'<<16 | 'n'<<8 | 'r'
	ClassDisplay    Class = 'm'<<24 | 'n'<<16 | 't'<<8 | 'r'
	ClassOutput     Class = 'p'<<24 | 'r'<<16 | 't'<<8 | 'r'
	ClassLink       Class = 'l'<<24 | 'i'<<16 | 'n'<<8 | 'k'
	ClassColorSpace Class = 's'<<24 | 'p'<<16 | 'a'<<8 | 'c'
	ClassAbstract   Class = 'a'<<24 | 'b'<<16 | 's'<<8 | 't'
	ClassNamedColor Class = 'n'<<24 | 'm'<<16 | 'c'<<8 | 'l'
)

func (c Class) String() string { return sigString(uint32(c)) }

// Intent is the rendering intent: what a transform does with the colours
// the destination cannot show.
type Intent uint8

// The four rendering intents of the ICC specification.
const (
	// Perceptual compresses the whole source gamut into the destination,
	// as the profile's maker saw fit. The usual intent for photographs.
	// Profiles without perceptual tables, matrix profiles like sRGB among
	// them, treat it as RelativeColorimetric.
	Perceptual Intent = iota
	// RelativeColorimetric keeps every colour the destination can show and
	// clips the others, with white mapped to white.
	RelativeColorimetric
	// Saturation keeps colours vivid at the expense of accuracy; meant for
	// business graphics.
	Saturation
	// AbsoluteColorimetric is RelativeColorimetric without mapping white to
	// white: a proof shows the paper colour of the destination.
	AbsoluteColorimetric
)

func (i Intent) String() string {
	switch i {
	case Perceptual:
		return "Perceptual"
	case RelativeColorimetric:
		return "RelativeColorimetric"
	case Saturation:
		return "Saturation"
	case AbsoluteColorimetric:
		return "AbsoluteColorimetric"
	}
	return fmt.Sprintf("Intent(%d)", uint8(i))
}

// sigString is a signature as its four characters, trailing blanks
// removed, or as hexadecimal where they are not printable.
func sigString(s uint32) string {
	b := []byte{byte(s >> 24), byte(s >> 16), byte(s >> 8), byte(s)}
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return fmt.Sprintf("0x%08x", s)
		}
	}
	for len(b) > 0 && b[len(b)-1] == ' ' {
		b = b[:len(b)-1]
	}
	return string(b)
}
