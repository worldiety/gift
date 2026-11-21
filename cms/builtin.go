package cms

import "sync"

// The primaries of the RGB spaces built in.
var (
	primariesSRGB     = [3]Chromaticity{{0.64, 0.33}, {0.30, 0.60}, {0.15, 0.06}}
	primariesP3       = [3]Chromaticity{{0.680, 0.320}, {0.265, 0.690}, {0.150, 0.060}}
	primariesAdobe    = [3]Chromaticity{{0.64, 0.33}, {0.21, 0.71}, {0.15, 0.06}}
	primariesProPhoto = [3]Chromaticity{{0.7347, 0.2653}, {0.1596, 0.8404}, {0.0366, 0.0001}}
	primaries2020     = [3]Chromaticity{{0.708, 0.292}, {0.170, 0.797}, {0.131, 0.046}}
)

func builtin(desc string, white Chromaticity, p [3]Chromaticity, c Curve) func() *Profile {
	return sync.OnceValue(func() *Profile {
		pr, err := NewRGBProfile(desc, white, p, c)
		if err != nil {
			panic(err)
		}
		return pr
	})
}

// The profiles built in. Each is made once, on first use, and shared.
var (
	// SRGB is sRGB, IEC 61966-2-1: the colour space of the web, of most
	// photographs and of every colour gift paints.
	SRGB = builtin("sRGB", WhiteD65, primariesSRGB, SRGBCurve())
	// DisplayP3 is the space of Apple's displays: the primaries of DCI-P3
	// with the white and the curve of sRGB.
	DisplayP3 = builtin("Display P3", WhiteD65, primariesP3, SRGBCurve())
	// AdobeRGB is Adobe RGB (1998), with its gamma of 563/256.
	AdobeRGB = builtin("Adobe RGB (1998)", WhiteD65, primariesAdobe, Gamma(563.0/256))
	// ProPhotoRGB is ProPhoto RGB, ROMM RGB as Adobe's profile has it, a
	// pure gamma of 1.8 on D50. Lightroom works in its primaries.
	ProPhotoRGB = builtin("ProPhoto RGB", WhiteD50, primariesProPhoto, Gamma(1.8))
	// Rec2020 is ITU-R BT.2020 with the curve of BT.709.
	Rec2020 = builtin("Rec. 2020", WhiteD65, primaries2020, Rec709Curve())
	// LinearSRGB is sRGB without its curve: the primaries of sRGB on
	// linear light.
	LinearSRGB = builtin("Linear sRGB", WhiteD65, primariesSRGB, Linear())
	// LinearRec2020 is Rec. 2020 on linear light, a working space wide
	// enough for the colours of a camera.
	LinearRec2020 = builtin("Linear Rec. 2020", WhiteD65, primaries2020, Linear())
	// SGray is gray with the curve of sRGB, what a gray picture without a
	// profile is taken to be.
	SGray = sync.OnceValue(func() *Profile {
		p, err := NewGrayProfile("sGray", WhiteD65, SRGBCurve())
		if err != nil {
			panic(err)
		}
		return p
	})
)
