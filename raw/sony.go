package raw

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Sony is what a Sony camera wrote about its settings into an ARW: in the
// MakerNote, and in the SR2 block, which the camera encrypts.
//
// The settings are those of the camera's own pictures, its JPEGs and the
// previews in the ARW. A raw converter that is to develop the raw data the
// way the camera did reads them here; Lightroom and most others read only
// the white balance and leave the rest.
type Sony struct {
	// ModelID is Sony's number for the camera, 363 for the ILCE-7M3.
	ModelID int
	// CreativeStyle is the style as the camera names it: "Standard",
	// "Vivid", "Neutral", "Clear", "Deep", "Light", "Portrait",
	// "Landscape", "Sunset", "Nightview", "Autumn", "BW" or "Sepia".
	CreativeStyle string
	// Contrast, Saturation and Sharpness are the levels of the style the
	// photograph was taken with, -3 to +3. A black and white style has no
	// saturation; the camera writes 0.
	Contrast, Saturation, Sharpness int
	// DRO is the setting of the dynamic range optimiser.
	DRO DRO
	// WhiteBalance is the white balance setting, which [File.WhiteBalance]
	// has the factors of.
	WhiteBalance WhiteBalance
	// ColorTemperature is the temperature in kelvin the white balance was
	// set to, 0 when it was not set to one.
	ColorTemperature int
	// WhiteBalanceShift is the fine tuning of the white balance, along
	// amber to blue and green to magenta.
	WhiteBalanceShift [2]int
	// Presets are the white balance factors of the camera's presets, as
	// [File.WhiteBalance] has those it took the photograph with: what the
	// camera would have set under daylight, tungsten and the others, which
	// tells of its sensor under these lights.
	Presets []WhiteBalancePreset
}

// WhiteBalancePreset is the white balance of one of the camera's presets.
type WhiteBalancePreset struct {
	// Name is the preset: "Daylight", "Cloudy", "Tungsten", "Flash",
	// "Shade", the fluorescent lights "Fluorescent", "FluorescentP1",
	// "FluorescentP2" and "FluorescentM1", or a temperature such as
	// "4500K".
	Name string
	// Factors are those of red, green and blue, green 1.
	Factors [3]float64
}

// DRO is the setting of Sony's dynamic range optimiser, which lifts the
// shadows of the camera's pictures where it finds them, more or less
// strongly.
type DRO uint32

const (
	DROOff          DRO = 0
	DROStandard     DRO = 1
	DROAdvancedAuto DRO = 2
	DROAuto         DRO = 3
	// DROAdvanced1 to DROAdvanced1+4 are the levels of the advanced
	// optimiser of older cameras.
	DROAdvanced1 DRO = 8
	// DROLevel1 to DROLevel1+4 are the levels Lv1 to Lv5.
	DROLevel1 DRO = 16
)

// Level is the level 1 to 5 of a fixed setting, 0 for off and the
// automatic ones.
func (d DRO) Level() int {
	switch {
	case d >= DROLevel1 && d < DROLevel1+5:
		return int(d-DROLevel1) + 1
	case d >= DROAdvanced1 && d < DROAdvanced1+5:
		return int(d-DROAdvanced1) + 1
	}
	return 0
}

func (d DRO) String() string {
	switch d {
	case DROOff:
		return "off"
	case DROStandard:
		return "standard"
	case DROAdvancedAuto:
		return "advanced auto"
	case DROAuto:
		return "auto"
	}
	if l := d.Level(); l > 0 {
		if d >= DROLevel1 {
			return fmt.Sprintf("Lv%d", l)
		}
		return fmt.Sprintf("advanced Lv%d", l)
	}
	return fmt.Sprintf("DRO %d", uint32(d))
}

// WhiteBalance is the white balance setting of a Sony camera.
type WhiteBalance uint32

const (
	WhiteBalanceAuto        WhiteBalance = 0x00
	WhiteBalanceTemperature WhiteBalance = 0x01
	WhiteBalanceDaylight    WhiteBalance = 0x10
	WhiteBalanceCloudy      WhiteBalance = 0x20
	WhiteBalanceShade       WhiteBalance = 0x30
	WhiteBalanceTungsten    WhiteBalance = 0x40
	WhiteBalanceFlash       WhiteBalance = 0x50
	WhiteBalanceFluorescent WhiteBalance = 0x60
	WhiteBalanceCustom      WhiteBalance = 0x70
	WhiteBalanceUnderwater  WhiteBalance = 0x80
)

func (w WhiteBalance) String() string {
	switch w {
	case WhiteBalanceAuto:
		return "auto"
	case WhiteBalanceTemperature:
		return "colour temperature"
	case WhiteBalanceDaylight:
		return "daylight"
	case WhiteBalanceCloudy:
		return "cloudy"
	case WhiteBalanceShade:
		return "shade"
	case WhiteBalanceTungsten:
		return "tungsten"
	case WhiteBalanceFlash:
		return "flash"
	case WhiteBalanceFluorescent:
		return "fluorescent"
	case WhiteBalanceCustom:
		return "custom"
	case WhiteBalanceUnderwater:
		return "underwater"
	}
	return fmt.Sprintf("white balance %#x", uint32(w))
}

// The presets in the SR2 block, from tag 0x7820 on.
var presetNames = [...]string{
	"Daylight", "Cloudy", "Tungsten", "Flash", "4500K", "Shade",
	"Fluorescent", "FluorescentP1", "FluorescentP2", "FluorescentM1",
	"8500K", "6000K", "3200K", "2500K",
}

// readSony reads the MakerNote and the SR2 block of an ARW, and the levels
// and white balance the directory of the mosaic has in the clear in recent
// cameras.
func (f *File) readSony(ifd0, exif, mosaic dir) {
	t := f.t
	s := &Sony{}
	f.Sony = s

	if mn := makerNote(t, exif); mn != nil {
		s.CreativeStyle = t.strOf(mn, 0xb020)
		get := func(tag uint16) int {
			v, _ := t.int(mn, tag)
			return int(v)
		}
		s.ModelID = get(0xb001)
		s.Contrast, s.Saturation, s.Sharpness = get(0x2004), get(0x2005), get(0x2006)
		s.DRO = DRO(get(0xb025))
		s.WhiteBalance = WhiteBalance(get(0x0115))
		s.ColorTemperature = get(0xb021)
		if v := t.intsOf(mn, 0x2014); len(v) == 2 {
			s.WhiteBalanceShift = [2]int{int(v[0]), int(v[1])}
		}
	}

	sr2 := sr2Block(t, ifd0)
	m := &f.Mosaic
	// Black level and white balance: in the directory of the mosaic when
	// it has them, else in the SR2 block, where older cameras keep them.
	for _, src := range []struct {
		t *tiff
		d dir
	}{{t, mosaic}, {sr2.t, sr2.d}} {
		if src.d == nil {
			continue
		}
		if v := src.t.floatsOf(src.d, 0x7310); len(v) == 4 && m.Black == ([4]float64{}) {
			if b, ok := byColour(m.CFA, [4]float64(v)); ok {
				m.Black = b
			}
		}
		if f.WhiteBalance == ([3]float64{}) {
			f.WhiteBalance = sonyBalance(src.t, src.d)
		}
	}
	if sr2.d == nil {
		return
	}
	if _, ok := mosaic[0xc61d]; !ok {
		if v := sr2.t.floatsOf(sr2.d, 0x787f); len(v) > 0 && v[0] > 0 {
			m.White = v[0]
		}
	}
	for i, name := range presetNames {
		v := sr2.t.floatsOf(sr2.d, 0x7820+uint16(i))
		if len(v) == 3 && v[1] > 0 {
			s.Presets = append(s.Presets, WhiteBalancePreset{name, [3]float64{v[0] / v[1], 1, v[2] / v[1]}})
		}
	}
}

// sonyBalance reads the white balance as levels of red, green, green and
// blue (0x7313), or of green, red, blue and green (0x7303).
func sonyBalance(t *tiff, d dir) [3]float64 {
	if v := t.floatsOf(d, 0x7313); len(v) == 4 && v[1]+v[2] > 0 {
		g := (v[1] + v[2]) / 2
		return [3]float64{v[0] / g, 1, v[3] / g}
	}
	if v := t.floatsOf(d, 0x7303); len(v) == 4 && v[0]+v[3] > 0 {
		g := (v[0] + v[3]) / 2
		return [3]float64{v[1] / g, 1, v[2] / g}
	}
	return [3]float64{}
}

// makerNote reads Sony's MakerNote, a directory whose offsets are those of
// the file. The cameras of the Alpha series write it bare; compact cameras
// put a name of twelve bytes before it.
func makerNote(t *tiff, exif dir) dir {
	fl, ok := exif[0x927c]
	if exif == nil || !ok || fl.count < 14 {
		return nil
	}
	off := t.offset(fl)
	head, err := t.read(off, 12)
	if err != nil {
		return nil
	}
	if bytes.HasPrefix(head, []byte("SONY")) || bytes.HasPrefix(head, []byte("\x00\x00SONY")) {
		off += 12
	}
	d, _, err := t.dir(off)
	if err != nil {
		return nil
	}
	return d
}

// sr2 is the decrypted SR2 block and its directory.
type sr2 struct {
	t *tiff
	d dir
}

// sr2Block finds the SR2 block through the private directory IFD 0 points
// to (tag 0xc634, DNGPrivateData in a DNG), and decrypts it.
func sr2Block(t *tiff, ifd0 dir) sr2 {
	fl, ok := ifd0[0xc634]
	if !ok {
		return sr2{}
	}
	priv, _, err := t.dir(t.offset(fl))
	if err != nil {
		return sr2{}
	}
	off, ok1 := t.int(priv, 0x7200)
	n, ok2 := t.int(priv, 0x7201)
	keyField, ok3 := priv[0x7221]
	if !ok1 || !ok2 || !ok3 || n < 6 || n > maxValue {
		return sr2{}
	}
	b, err := t.read(off, int(n))
	if err != nil {
		return sr2{}
	}
	decryptSR2(b, t.bo.Uint32(keyField.value[:]))
	st := &tiff{r: bytes.NewReader(b), base: off, end: off + n, bo: t.bo}
	d, _, err := st.dir(off)
	if err != nil {
		return sr2{}
	}
	return sr2{st, d}
}

// decryptSR2 undoes Sony's encryption of the SR2 block in place: the words
// of the block, big endian, are each XORed with the next of a sequence that
// the key seeds, as dcraw's sony_decrypt has it (see NOTICE).
//
// The key makes the first four numbers of a linear congruential generator;
// a shift register makes 123 more of them; from there each number is the
// XOR of the ones 127 and 63 before it.
func decryptSR2(b []byte, key uint32) {
	var pad [128]uint32
	for p := range 4 {
		key = key*48828125 + 1
		pad[p] = key
	}
	pad[3] = pad[3]<<1 | (pad[0]^pad[2])>>31
	for p := 4; p < 127; p++ {
		pad[p] = (pad[p-4]^pad[p-2])<<1 | (pad[p-3]^pad[p-1])>>31
	}
	for i, p := 0, 127; i+4 <= len(b); i, p = i+4, p+1 {
		pad[p&127] = pad[(p+1)&127] ^ pad[(p+65)&127]
		binary.BigEndian.PutUint32(b[i:], binary.BigEndian.Uint32(b[i:])^pad[p&127])
	}
}
