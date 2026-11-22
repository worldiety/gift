package raw

import "github.com/worldiety/gift/cms"

// DNGColor is how a DNG describes the colours of its camera: the input of
// the colour model of the DNG specification, which maps the camera's
// colours to XYZ by two matrices measured under two illuminants and
// interpolated for the white balance of the photograph.
//
// Matrices are row by row. A colour matrix has a row for each colour of the
// camera and three columns; a forward matrix three rows and a column for
// each colour of the camera; a calibration matrix is square.
type DNGColor struct {
	// Version is DNGVersion, such as 1, 4, 0, 0.
	Version [4]byte
	// UniqueCameraModel names the camera for finding profiles for it.
	UniqueCameraModel string
	// ColorMatrix1 and ColorMatrix2 map XYZ to the camera's colours under
	// CalibrationIlluminant1 and CalibrationIlluminant2; ColorMatrix2 and
	// the second illuminant are empty for a camera measured under one.
	ColorMatrix1, ColorMatrix2 []float64
	// ForwardMatrix1 and ForwardMatrix2 map the camera's white balanced
	// colours to XYZ under D50; empty when the file has none.
	ForwardMatrix1, ForwardMatrix2 []float64
	// CameraCalibration1 and CameraCalibration2 correct the individual
	// camera against the model's matrices; empty when the file has none.
	CameraCalibration1, CameraCalibration2 []float64
	// CalibrationIlluminant1 and CalibrationIlluminant2 are the
	// illuminants as EXIF's LightSource numbers them: 17 standard light A,
	// 21 D65, and so on.
	CalibrationIlluminant1, CalibrationIlluminant2 int
	// AnalogBalance is the gain applied to the camera's colours before
	// they were stored; empty when the file has none, which means 1.
	AnalogBalance []float64
	// AsShotNeutral is the white of the photograph in the camera's
	// colours, which [File.WhiteBalance] is the inverse of; empty when
	// the white is given as AsShotWhiteXY instead.
	AsShotNeutral []float64
	// AsShotWhiteXY is the white of the photograph as chromaticity x, y.
	AsShotWhiteXY []float64
	// BaselineExposure is the exposure in stops that the camera's model
	// needs to be shown right.
	BaselineExposure float64
	// CameraCalibrationSignature names the profile the camera
	// calibrations were measured against; empty for any.
	CameraCalibrationSignature string
}

// readDNG reads the colour tags of IFD 0 and, for the white balance and
// levels it does not have, those of the mosaic's directory, where the
// specification allows several of them too.
func (f *File) readDNG(ifd0, mosaic dir) {
	t := f.t
	c := &DNGColor{}
	f.DNG = c
	if v := t.intsOf(ifd0, 0xc612); len(v) == 4 {
		for i := range 4 {
			c.Version[i] = byte(v[i])
		}
	}
	c.UniqueCameraModel = t.strOf(ifd0, 0xc614)
	look := func(tag uint16) []float64 {
		if v := t.floatsOf(mosaic, tag); v != nil {
			return v
		}
		return t.floatsOf(ifd0, tag)
	}
	c.ColorMatrix1, c.ColorMatrix2 = look(0xc621), look(0xc622)
	c.CameraCalibration1, c.CameraCalibration2 = look(0xc623), look(0xc624)
	c.ForwardMatrix1, c.ForwardMatrix2 = look(0xc714), look(0xc715)
	c.AnalogBalance = look(0xc627)
	c.AsShotNeutral = look(0xc628)
	c.AsShotWhiteXY = look(0xc629)
	if v := look(0xc62a); len(v) > 0 {
		c.BaselineExposure = v[0]
	}
	if v := look(0xc65a); len(v) > 0 {
		c.CalibrationIlluminant1 = int(v[0])
	}
	if v := look(0xc65b); len(v) > 0 {
		c.CalibrationIlluminant2 = int(v[0])
	}
	c.CameraCalibrationSignature = t.strOf(ifd0, 0xc6f3)
	// The camera profile is in the first directory; some writers put the
	// matrices with the raw data.
	if p, err := readProfile(t, ifd0); err == nil {
		f.profile = p
	} else if p, err := readProfile(t, mosaic); err == nil {
		f.profile = p
	}
	if n := c.AsShotNeutral; len(n) == 3 && n[0] > 0 && n[1] > 0 && n[2] > 0 {
		f.WhiteBalance = [3]float64{n[1] / n[0], 1, n[1] / n[2]}
	} else if w := c.AsShotWhiteXY; len(w) == 2 && f.profile != nil && w[0] > 0 && w[1] > 0 && w[0]+w[1] < 1 {
		// A white given as a chromaticity is the camera's neutral through
		// the matrices for it.
		n := f.model(f.profile).neutralOf(cms.Chromaticity{X: w[0], Y: w[1]})
		if n[0] > 0 && n[1] > 0 && n[2] > 0 {
			f.WhiteBalance = [3]float64{n[1] / n[0], 1, n[1] / n[2]}
		}
	}
}
