package qr

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"
)

// Level is the error correction level: how much of a code may be damaged or
// hidden and still be read. A higher level makes the same text a larger code.
type Level uint8

// The error correction levels of the standard, with the share of the code
// each one recovers.
const (
	// L recovers about 7 %, the smallest code for a text.
	L Level = iota
	// M recovers about 15 %, the usual choice and the default of [View].
	M
	// Q recovers about 25 %.
	Q
	// H recovers about 30 %, for a code that is printed and may get dirty.
	H
)

func (l Level) String() string {
	switch l {
	case L:
		return "L"
	case M:
		return "M"
	case Q:
		return "Q"
	case H:
		return "H"
	}
	return fmt.Sprintf("Level(%d)", uint8(l))
}

// ErrTooLong is returned by [Encode] for a text that does not fit into the
// largest code, version 40: 2953 bytes at level L, fewer at higher levels.
var ErrTooLong = errors.New("gift/qr: text too long for a QR code")

// QuietZone is the width of the light border a reader needs around a code, in
// modules, as the standard asks for it. [Code.Image] and [View] include it.
const QuietZone = 4

// Code is an encoded QR code: a square of dark and light modules, Size on
// each side. It is immutable and may be shared between goroutines.
type Code struct {
	size    int
	version int
	level   Level
	mask    int
	dark    []bool
}

// Size returns the number of modules on each side, 21 for version 1 up to 177
// for version 40, without the quiet zone.
func (c *Code) Size() int { return c.size }

// Version returns the version, 1 to 40, which is what the size follows from.
func (c *Code) Version() int { return c.version }

// Level returns the error correction level. It may be higher than the one
// asked for: [Encode] raises it as far as the code's size allows.
func (c *Code) Level() Level { return c.level }

// Dark reports whether the module in column x and row y is dark. Outside the
// code, which includes the quiet zone, every module is light.
func (c *Code) Dark(x, y int) bool {
	if x < 0 || y < 0 || x >= c.size || y >= c.size {
		return false
	}
	return c.dark[y*c.size+x]
}

// Image returns the code as a grey image, black on white, with every module
// px pixels square and the quiet zone around it: for a file, a print or a
// test, where [View] is for the screen.
func (c *Code) Image(px int) *image.Gray {
	px = max(px, 1)
	side := (c.size + 2*QuietZone) * px
	im := image.NewGray(image.Rect(0, 0, side, side))
	for i := range im.Pix {
		im.Pix[i] = 0xff
	}
	for y := range c.size {
		for x := range c.size {
			if !c.Dark(x, y) {
				continue
			}
			x0, y0 := (x+QuietZone)*px, (y+QuietZone)*px
			for yy := y0; yy < y0+px; yy++ {
				for xx := x0; xx < x0+px; xx++ {
					im.SetGray(xx, yy, color.Gray{})
				}
			}
		}
	}
	return im
}

// Encode returns text as a QR code at level or better: the smallest version
// that holds it, at the highest level that still fits that version.
//
// The text is encoded whole in one mode: numeric for digits only,
// alphanumeric for digits, upper case letters and " $%*+-./:" only, and
// bytes, its UTF-8, for everything else. A URL is usually bytes; an upper case
// one, or a user code such as "WDJB-MJHT", is the smaller alphanumeric.
// Readers take bytes as UTF-8, which is why no ECI designator is written.
//
// It panics on a level other than L, M, Q and H, a mistake at the call site.
func Encode(text string, level Level) (*Code, error) {
	return encode(text, level, -1, true)
}

// encode is [Encode] with the mask fixed, for mask 0 to 7, and without
// raising the level, for the tests that compare it with another encoder.
func encode(text string, level Level, mask int, boost bool) (*Code, error) {
	if level > H {
		panic(fmt.Sprintf("gift/qr: %v is not an error correction level", level))
	}
	seg := segmentOf(text)
	version := 0
	for v := 1; v <= 40; v++ {
		if seg.bits(v) <= dataCodewords(v, level)*8 {
			version = v
			break
		}
	}
	if version == 0 {
		return nil, ErrTooLong
	}
	if boost {
		for l := level + 1; l <= H; l++ {
			if seg.bits(version) <= dataCodewords(version, l)*8 {
				level = l
			}
		}
	}

	// The data: mode, count, payload, terminator, padding to the byte and
	// then the alternating pad bytes up to the capacity.
	capacity := dataCodewords(version, level) * 8
	var b bitBuffer
	b.write(uint(seg.mode), 4)
	b.write(uint(seg.count), seg.countBits(version))
	seg.encode(&b)
	b.write(0, min(4, capacity-b.n))
	b.write(0, (8-b.n%8)%8)
	for pad := uint(0xec); b.n < capacity; pad ^= 0xec ^ 0x11 {
		b.write(pad, 8)
	}

	c := &Code{size: version*4 + 17, version: version, level: level}
	c.dark = make([]bool, c.size*c.size)
	fn := make([]bool, c.size*c.size)
	c.drawFunctions(fn)
	c.drawCodewords(fn, interleave(b.bytes, version, level))

	if mask < 0 {
		best := 1 << 30
		for m := range 8 {
			c.applyMask(fn, m)
			c.drawFormat(fn, m)
			if p := c.penalty(); p < best {
				best, mask = p, m
			}
			c.applyMask(fn, m)
		}
	}
	c.mask = mask
	c.applyMask(fn, mask)
	c.drawFormat(fn, mask)
	return c, nil
}

// String draws the code in text, two characters per module, for a test
// failure or a terminal.
func (c *Code) String() string {
	var sb strings.Builder
	for y := -1; y <= c.size; y++ {
		for x := -1; x <= c.size; x++ {
			if c.Dark(x, y) {
				sb.WriteString("██")
			} else {
				sb.WriteString("  ")
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// The modes of a segment, as the four bits that announce them.
const (
	modeNumeric      = 1
	modeAlphanumeric = 2
	modeBytes        = 4
)

const alphanumeric = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

// segment is the text in the one mode it is encoded in.
type segment struct {
	mode  int
	text  string
	count int // characters, or bytes in byte mode
}

func segmentOf(text string) segment {
	numeric, alnum := true, true
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if ch < '0' || ch > '9' {
			numeric = false
		}
		if strings.IndexByte(alphanumeric, ch) < 0 {
			alnum = false
		}
	}
	switch {
	case numeric && text != "":
		return segment{modeNumeric, text, len(text)}
	case alnum && text != "":
		return segment{modeAlphanumeric, text, len(text)}
	}
	return segment{modeBytes, text, len(text)}
}

// countBits is the width of the character count, which grows with the
// version.
func (s segment) countBits(version int) int {
	class := 0
	if version >= 27 {
		class = 2
	} else if version >= 10 {
		class = 1
	}
	switch s.mode {
	case modeNumeric:
		return [3]int{10, 12, 14}[class]
	case modeAlphanumeric:
		return [3]int{9, 11, 13}[class]
	}
	return [3]int{8, 16, 16}[class]
}

// bits is the length of the segment with its header.
func (s segment) bits(version int) int {
	n := 4 + s.countBits(version)
	if s.count >= 1<<s.countBits(version) {
		return 1 << 30
	}
	switch s.mode {
	case modeNumeric:
		n += s.count / 3 * 10
		n += [3]int{0, 4, 7}[s.count%3]
	case modeAlphanumeric:
		n += s.count/2*11 + s.count%2*6
	default:
		n += s.count * 8
	}
	return n
}

func (s segment) encode(b *bitBuffer) {
	switch s.mode {
	case modeNumeric:
		for i := 0; i < len(s.text); i += 3 {
			j := min(i+3, len(s.text))
			v := uint(0)
			for _, ch := range s.text[i:j] {
				v = v*10 + uint(ch-'0')
			}
			b.write(v, [4]int{0, 4, 7, 10}[j-i])
		}
	case modeAlphanumeric:
		for i := 0; i < len(s.text); i += 2 {
			v := uint(strings.IndexByte(alphanumeric, s.text[i]))
			if i+1 < len(s.text) {
				v = v*45 + uint(strings.IndexByte(alphanumeric, s.text[i+1]))
				b.write(v, 11)
			} else {
				b.write(v, 6)
			}
		}
	default:
		for i := 0; i < len(s.text); i++ {
			b.write(uint(s.text[i]), 8)
		}
	}
}

// bitBuffer collects bits, the most significant first.
type bitBuffer struct {
	bytes []byte
	n     int
}

func (b *bitBuffer) write(v uint, bits int) {
	for i := bits - 1; i >= 0; i-- {
		if b.n%8 == 0 {
			b.bytes = append(b.bytes, 0)
		}
		if v>>uint(i)&1 != 0 {
			b.bytes[b.n/8] |= 0x80 >> uint(b.n%8)
		}
		b.n++
	}
}

// eccPerBlock is the number of error correction codewords in each block, by
// level and version; index 0 is unused.
var eccPerBlock = [4][41]uint8{
	{0, 7, 10, 15, 20, 26, 18, 20, 24, 30, 18, 20, 24, 26, 30, 22, 24, 28, 30, 28, 28, 28, 28, 30, 30, 26, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28},
	{0, 13, 22, 18, 26, 18, 24, 18, 22, 20, 24, 28, 26, 24, 20, 30, 24, 28, 28, 26, 30, 28, 30, 30, 30, 30, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{0, 17, 28, 22, 16, 22, 28, 26, 26, 24, 28, 24, 28, 22, 24, 24, 30, 28, 28, 26, 28, 30, 24, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
}

// blocks is the number of blocks the codewords are split into, by level and
// version; index 0 is unused.
var blocks = [4][41]uint8{
	{0, 1, 1, 1, 1, 1, 2, 2, 2, 2, 4, 4, 4, 4, 4, 6, 6, 6, 6, 7, 8, 8, 9, 9, 10, 12, 12, 12, 13, 14, 15, 16, 17, 18, 19, 19, 20, 21, 22, 24, 25},
	{0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49},
	{0, 1, 1, 2, 2, 4, 4, 6, 6, 8, 8, 8, 10, 12, 16, 12, 17, 16, 18, 21, 20, 23, 23, 25, 27, 29, 34, 34, 35, 38, 40, 43, 45, 48, 51, 53, 56, 59, 62, 65, 68},
	{0, 1, 1, 2, 4, 4, 4, 5, 6, 8, 8, 11, 11, 16, 16, 18, 16, 19, 21, 25, 25, 25, 34, 30, 32, 35, 37, 40, 42, 45, 48, 51, 54, 57, 60, 63, 66, 70, 74, 77, 81},
}

// rawModules is the number of modules of a version that carry codewords:
// all of them less the function patterns and the format and version
// information.
func rawModules(version int) int {
	n := (16*version+128)*version + 64
	if version >= 2 {
		align := version/7 + 2
		n -= (25*align-10)*align - 55
		if version >= 7 {
			n -= 36
		}
	}
	return n
}

// dataCodewords is the number of data codewords of a version at a level.
func dataCodewords(version int, l Level) int {
	return rawModules(version)/8 - int(eccPerBlock[l][version])*int(blocks[l][version])
}

// interleave splits the data into its blocks, appends to each its error
// correction and interleaves the blocks codeword by codeword.
func interleave(data []byte, version int, l Level) []byte {
	nblocks := int(blocks[l][version])
	ecc := int(eccPerBlock[l][version])
	raw := rawModules(version) / 8
	short := nblocks - raw%nblocks
	shortLen := raw / nblocks
	div := rsDivisor(ecc)

	out := make([]byte, 0, raw)
	type block struct{ data, ecc []byte }
	bs := make([]block, nblocks)
	k := 0
	for i := range bs {
		n := shortLen - ecc
		if i >= short {
			n++
		}
		bs[i] = block{data[k : k+n], rsRemainder(data[k:k+n], div)}
		k += n
	}
	for i := range shortLen - ecc + 1 {
		for _, b := range bs {
			if i < len(b.data) {
				out = append(out, b.data[i])
			}
		}
	}
	for i := range ecc {
		for _, b := range bs {
			out = append(out, b.ecc[i])
		}
	}
	return out
}

// gfMul multiplies in GF(2⁸) modulo x⁸+x⁴+x³+x²+1.
func gfMul(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = z<<1 ^ z>>7*0x11d
		z ^= int(y>>uint(i)&1) * int(x)
	}
	return byte(z)
}

// rsDivisor is the generator polynomial of the Reed-Solomon code of a
// degree, its coefficients from the highest power down, without the leading
// one.
func rsDivisor(degree int) []byte {
	d := make([]byte, degree)
	d[degree-1] = 1
	root := byte(1)
	for range degree {
		for j := range d {
			d[j] = gfMul(d[j], root)
			if j+1 < len(d) {
				d[j] ^= d[j+1]
			}
		}
		root = gfMul(root, 2)
	}
	return d
}

// rsRemainder is the error correction of data: the remainder of its
// division by the divisor.
func rsRemainder(data, div []byte) []byte {
	r := make([]byte, len(div))
	for _, b := range data {
		f := b ^ r[0]
		copy(r, r[1:])
		r[len(r)-1] = 0
		for i, c := range div {
			r[i] ^= gfMul(c, f)
		}
	}
	return r
}

func (c *Code) set(fn []bool, x, y int, dark bool) {
	c.dark[y*c.size+x] = dark
	fn[y*c.size+x] = true
}

// drawFunctions draws the patterns that are the same in every code of the
// version and marks them in fn, together with the room of the format
// information, which comes last.
func (c *Code) drawFunctions(fn []bool) {
	n := c.size
	for i := range n {
		c.set(fn, 6, i, i%2 == 0)
		c.set(fn, i, 6, i%2 == 0)
	}
	for _, p := range [][2]int{{3, 3}, {n - 4, 3}, {3, n - 4}} {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := p[0]+dx, p[1]+dy
				if x < 0 || y < 0 || x >= n || y >= n {
					continue
				}
				d := max(abs(dx), abs(dy))
				c.set(fn, x, y, d != 2 && d != 4)
			}
		}
	}
	pos := alignment(c.version)
	for i, x := range pos {
		for j, y := range pos {
			if i == 0 && j == 0 || i == 0 && j == len(pos)-1 || i == len(pos)-1 && j == 0 {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					c.set(fn, x+dx, y+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	c.drawFormat(fn, 0)
	if c.version >= 7 {
		r := c.version
		for range 12 {
			r = r<<1 ^ r>>11*0x1f25
		}
		bits := c.version<<12 | r
		for i := range 18 {
			dark := bits>>uint(i)&1 != 0
			a, b := n-11+i%3, i/3
			c.set(fn, a, b, dark)
			c.set(fn, b, a, dark)
		}
	}
}

// alignment returns the centres of the alignment patterns along one axis.
func alignment(version int) []int {
	if version == 1 {
		return nil
	}
	num := version/7 + 2
	step := (version*8 + num*3 + 5) / (num*4 - 4) * 2
	pos := make([]int, num)
	pos[0] = 6
	for i, p := num-1, version*4+17-7; i >= 1; i, p = i-1, p-step {
		pos[i] = p
	}
	return pos
}

// drawFormat draws both copies of the format information, the level and the
// mask, and the one module that is always dark.
func (c *Code) drawFormat(fn []bool, mask int) {
	data := [4]int{1, 0, 3, 2}[c.level]<<3 | mask
	r := data
	for range 10 {
		r = r<<1 ^ r>>9*0x537
	}
	bits := (data<<10 | r) ^ 0x5412
	bit := func(i int) bool { return bits>>uint(i)&1 != 0 }
	n := c.size
	for i := range 6 {
		c.set(fn, 8, i, bit(i))
	}
	c.set(fn, 8, 7, bit(6))
	c.set(fn, 8, 8, bit(7))
	c.set(fn, 7, 8, bit(8))
	for i := 9; i < 15; i++ {
		c.set(fn, 14-i, 8, bit(i))
	}
	for i := range 8 {
		c.set(fn, n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		c.set(fn, 8, n-15+i, bit(i))
	}
	c.set(fn, 8, n-8, true)
}

// drawCodewords places the codewords in the zigzag of two columns, from the
// bottom right corner up and down again, around the function patterns.
func (c *Code) drawCodewords(fn []bool, data []byte) {
	n := c.size
	i := 0
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := range n {
			for j := range 2 {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = n - 1 - vert
				}
				if fn[y*n+x] || i >= len(data)*8 {
					continue
				}
				c.dark[y*n+x] = data[i>>3]>>uint(7-i&7)&1 != 0
				i++
			}
		}
	}
}

// applyMask flips the modules outside the function patterns where the mask
// says so; applying it twice undoes it.
func (c *Code) applyMask(fn []bool, mask int) {
	n := c.size
	for y := range n {
		for x := range n {
			if fn[y*n+x] {
				continue
			}
			var flip bool
			switch mask {
			case 0:
				flip = (x+y)%2 == 0
			case 1:
				flip = y%2 == 0
			case 2:
				flip = x%3 == 0
			case 3:
				flip = (x+y)%3 == 0
			case 4:
				flip = (x/3+y/2)%2 == 0
			case 5:
				flip = x*y%2+x*y%3 == 0
			case 6:
				flip = (x*y%2+x*y%3)%2 == 0
			case 7:
				flip = ((x+y)%2+x*y%3)%2 == 0
			}
			if flip {
				c.dark[y*n+x] = !c.dark[y*n+x]
			}
		}
	}
}

// penalty rates a masked code by the four rules of the standard: long runs,
// two by two blocks, patterns that look like a finder, and an unbalanced
// share of dark modules. The mask with the lowest wins.
func (c *Code) penalty() int {
	n := c.size
	p := 0
	line := make([]bool, n)
	for y := range n {
		copy(line, c.dark[y*n:(y+1)*n])
		p += linePenalty(line)
	}
	for x := range n {
		for y := range n {
			line[y] = c.dark[y*n+x]
		}
		p += linePenalty(line)
	}
	dark := 0
	for y := range n {
		for x := range n {
			d := c.dark[y*n+x]
			if d {
				dark++
			}
			if x+1 < n && y+1 < n && d == c.dark[y*n+x+1] && d == c.dark[(y+1)*n+x] && d == c.dark[(y+1)*n+x+1] {
				p += 3
			}
		}
	}
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	return p + k*10
}

// finderLike is the dark-light-dark ratio 1:1:3:1:1 of a finder pattern.
var finderLike = [7]bool{true, false, true, true, true, false, true}

func linePenalty(line []bool) int {
	n := len(line)
	p := 0
	run := 1
	for i := 1; i <= n; i++ {
		if i < n && line[i] == line[i-1] {
			run++
			continue
		}
		if run >= 5 {
			p += 3 + run - 5
		}
		run = 1
	}
	at := func(i int) bool { return i >= 0 && i < n && line[i] }
	for i := 0; i+7 <= n; i++ {
		match := true
		for j, d := range finderLike {
			if line[i+j] != d {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		if !at(i-1) && !at(i-2) && !at(i-3) && !at(i-4) {
			p += 40
		}
		if !at(i+7) && !at(i+8) && !at(i+9) && !at(i+10) {
			p += 40
		}
	}
	return p
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
