// Package qr makes QR codes (ISO/IEC 18004) for the game to show on
// screen, such as the address a grown-up scans to link the game
// (halpwords-server#153). It encodes bytes in byte mode at error
// correction level M, in versions 1 to 10 (up to 213 bytes), and picks
// the mask with the lowest penalty, as the standard says. It uses only
// the standard library and draws nothing: the screens draw Code's
// modules themselves.
package qr

import "errors"

// ErrTooLong is data that doesn't fit in a version 10 code at level M.
var ErrTooLong = errors.New("qr: too much data")

// MaxVersion is the largest version Encode makes.
const MaxVersion = 10

// Code is a QR code: a square of modules, dark or light, without the
// quiet zone around it (4 modules of light, which the screen adds).
type Code struct {
	version int
	mask    int
	size    int
	dark    []bool // size*size, row by row
}

// Size is how many modules wide (and high) the code is: 21 for version
// 1, 4 more for each version after.
func (c *Code) Size() int { return c.size }

// Version is the code's version, 1 to MaxVersion.
func (c *Code) Version() int { return c.version }

// Mask is the mask pattern the code uses, 0 to 7.
func (c *Code) Mask() int { return c.mask }

// Dark reports whether the module in column x and row y, from the top
// left, is dark. Modules outside the code (the quiet zone) are light.
func (c *Code) Dark(x, y int) bool {
	if x < 0 || y < 0 || x >= c.size || y >= c.size {
		return false
	}
	return c.dark[y*c.size+x]
}

// block is the error correction layout of a version at level M: the
// error correction codewords per block, and the blocks in its two
// groups with their data codewords (ISO/IEC 18004 table 9).
type block struct {
	ec             int
	n1, d1, n2, d2 int
}

// levelM are versions 1 to 10 at level M; index 0 is unused.
var levelM = [MaxVersion + 1]block{
	{},
	{10, 1, 16, 0, 0},
	{16, 1, 28, 0, 0},
	{26, 1, 44, 0, 0},
	{18, 2, 32, 0, 0},
	{24, 2, 43, 0, 0},
	{16, 4, 27, 0, 0},
	{18, 4, 31, 0, 0},
	{22, 2, 38, 2, 39},
	{22, 3, 36, 2, 37},
	{26, 4, 43, 1, 44},
}

// align are the centres of the alignment patterns, by version.
var align = [MaxVersion + 1][]int{
	{}, {}, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34},
	{6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50},
}

func (b block) data() int { return b.n1*b.d1 + b.n2*b.d2 }

// countBits is the length of the byte mode's character count.
func countBits(version int) int {
	if version < 10 {
		return 8
	}
	return 16
}

// Capacity is how many bytes a code of version holds at level M.
func Capacity(version int) int {
	if version < 1 || version > MaxVersion {
		return 0
	}
	return (levelM[version].data()*8 - 4 - countBits(version)) / 8
}

// Encode makes the smallest code that holds data, with the best mask.
func Encode(data []byte) (*Code, error) {
	for v := 1; v <= MaxVersion; v++ {
		if len(data) <= Capacity(v) {
			return encode(data, v, -1), nil
		}
	}
	return nil, ErrTooLong
}

// encode makes the code of version for data with mask, or with the
// best mask when mask is -1. data fits.
func encode(data []byte, version, mask int) *Code {
	words := codewords(data, version)
	c := &Code{version: version, size: 17 + 4*version}
	c.dark = make([]bool, c.size*c.size)
	fn := make([]bool, c.size*c.size) // function patterns, not data
	c.drawFunctions(fn)
	c.place(words, fn)
	if mask >= 0 {
		c.applyMask(mask, fn)
		c.drawFormat(mask)
		c.mask = mask
		return c
	}
	best, score := 0, -1
	for m := range 8 {
		c.applyMask(m, fn)
		c.drawFormat(m)
		if p := c.penalty(); score < 0 || p < score {
			best, score = m, p
		}
		c.applyMask(m, fn) // undo: masks are XOR
	}
	c.applyMask(best, fn)
	c.drawFormat(best)
	c.mask = best
	return c
}

// codewords are the data and error correction codewords of data in
// version, interleaved in the order they are placed.
func codewords(data []byte, version int) []byte {
	b := levelM[version]
	// The bit stream: mode, count, the bytes, the terminator, padding.
	var bits bitBuf
	bits.add(0b0100, 4)
	bits.add(uint(len(data)), countBits(version))
	for _, d := range data {
		bits.add(uint(d), 8)
	}
	capBits := b.data() * 8
	bits.add(0, min(4, capBits-bits.n))
	bits.add(0, (8-bits.n%8)%8)
	for pad := uint(0xEC); bits.n < capBits; pad ^= 0xEC ^ 0x11 {
		bits.add(pad, 8)
	}
	stream := bits.b

	// Split into blocks, each with its error correction.
	gen := rsGenerator(b.ec)
	var blocks, ecs [][]byte
	for i := range b.n1 + b.n2 {
		n := b.d1
		if i >= b.n1 {
			n = b.d2
		}
		blocks = append(blocks, stream[:n])
		ecs = append(ecs, rsRemainder(stream[:n], gen))
		stream = stream[n:]
	}
	// Interleave: the data a codeword from each block at a time (the
	// short blocks run out first), then the error correction.
	var out []byte
	for i := range max(b.d1, b.d2) {
		for _, blk := range blocks {
			if i < len(blk) {
				out = append(out, blk[i])
			}
		}
	}
	for i := range b.ec {
		for _, e := range ecs {
			out = append(out, e[i])
		}
	}
	return out
}

// bitBuf is a bit stream, most significant bit first.
type bitBuf struct {
	b []byte
	n int // bits used
}

func (w *bitBuf) add(v uint, bits int) {
	for i := bits - 1; i >= 0; i-- {
		if w.n%8 == 0 {
			w.b = append(w.b, 0)
		}
		if v>>i&1 == 1 {
			w.b[w.n/8] |= 0x80 >> (w.n % 8)
		}
		w.n++
	}
}

// set sets a module of a function pattern.
func (c *Code) set(fn []bool, x, y int, dark bool) {
	c.dark[y*c.size+x] = dark
	fn[y*c.size+x] = true
}

// drawFunctions draws the finder, timing and alignment patterns, the
// dark module and the version information, and reserves the format
// information's modules.
func (c *Code) drawFunctions(fn []bool) {
	n := c.size
	for i := range n {
		c.set(fn, 6, i, i%2 == 0)
		c.set(fn, i, 6, i%2 == 0)
	}
	for _, p := range [][2]int{{3, 3}, {n - 4, 3}, {3, n - 4}} {
		// The finder and its light separator, cut off at the edges.
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
	ac := align[c.version]
	for i, ax := range ac {
		for j, ay := range ac {
			// Not over the finders.
			if i == 0 && j == 0 || i == 0 && j == len(ac)-1 || i == len(ac)-1 && j == 0 {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					c.set(fn, ax+dx, ay+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	c.drawFormatIn(fn, 0) // reserved; drawn again with the mask
	c.set(fn, 8, n-8, true)
	if c.version >= 7 {
		bits := versionBits(c.version)
		for i := range 18 {
			d := bits>>i&1 == 1
			a, b := n-11+i%3, i/3
			c.set(fn, a, b, d)
			c.set(fn, b, a, d)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// formatBits are the 15 format bits for level M with mask: the 5 data
// bits (level M is 00), their BCH(15,5) remainder, and the XOR mask.
func formatBits(mask int) uint {
	data := uint(0b00<<3 | mask)
	rem := data
	for range 10 {
		rem = rem<<1 ^ (rem>>9)*0x537
	}
	return (data<<10 | rem&0x3FF) ^ 0x5412
}

// versionBits are the 18 version bits: the version and its BCH(18,6)
// remainder.
func versionBits(version int) uint {
	rem := uint(version)
	for range 12 {
		rem = rem<<1 ^ (rem>>11)*0x1F25
	}
	return uint(version)<<12 | rem&0xFFF
}

// drawFormat draws the format information for mask.
func (c *Code) drawFormat(mask int) {
	fn := make([]bool, len(c.dark)) // throwaway: already reserved
	c.drawFormatIn(fn, mask)
}

func (c *Code) drawFormatIn(fn []bool, mask int) {
	bits := formatBits(mask)
	bit := func(i int) bool { return bits>>i&1 == 1 }
	n := c.size
	// Around the top left finder.
	for i := range 6 {
		c.set(fn, 8, i, bit(i))
	}
	c.set(fn, 8, 7, bit(6))
	c.set(fn, 8, 8, bit(7))
	c.set(fn, 7, 8, bit(8))
	for i := 9; i < 15; i++ {
		c.set(fn, 14-i, 8, bit(i))
	}
	// Under the top right and beside the bottom left finder.
	for i := range 8 {
		c.set(fn, n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		c.set(fn, 8, n-15+i, bit(i))
	}
}

// place puts the codewords in the modules that aren't function
// patterns, two columns at a time in a zigzag from the bottom right.
// The modules left over (the remainder bits) stay light.
func (c *Code) place(words []byte, fn []bool) {
	n := c.size
	i := 0
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5 // skip the vertical timing pattern
		}
		up := (right+1)&2 == 0
		for v := range n {
			y := v
			if up {
				y = n - 1 - v
			}
			for j := range 2 {
				x := right - j
				if fn[y*n+x] || i >= len(words)*8 {
					continue
				}
				c.dark[y*n+x] = words[i/8]>>(7-i%8)&1 == 1
				i++
			}
		}
	}
}

// applyMask flips the data modules where mask says to. Applying it
// twice undoes it.
func (c *Code) applyMask(mask int, fn []bool) {
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

// penalty scores the code as ISO/IEC 18004 section 7.8.3 does: runs of
// five or more modules of one colour in a row or column, 2x2 blocks of
// one colour, patterns like a finder's (1:1:3:1:1 with four light
// modules on one side), and how far dark is from half.
func (c *Code) penalty() int {
	n := c.size
	at := func(x, y int) bool { return c.dark[y*n+x] }
	p := 0
	// Rule 1: runs.
	for i := range n {
		for _, row := range []bool{true, false} {
			run, last := 0, false
			for j := range n {
				d := at(j, i)
				if !row {
					d = at(i, j)
				}
				if j > 0 && d == last {
					run++
					continue
				}
				if run >= 5 {
					p += run - 2
				}
				run, last = 1, d
			}
			if run >= 5 {
				p += run - 2
			}
		}
	}
	// Rule 2: 2x2 blocks.
	for y := range n - 1 {
		for x := range n - 1 {
			d := at(x, y)
			if at(x+1, y) == d && at(x, y+1) == d && at(x+1, y+1) == d {
				p += 3
			}
		}
	}
	// Rule 3: finder-like patterns, in rows and columns.
	p1 := [11]bool{true, false, true, true, true, false, true, false, false, false, false}
	p2 := [11]bool{false, false, false, false, true, false, true, true, true, false, true}
	for i := range n {
		for j := 0; j+11 <= n; j++ {
			r1, r2, c1, c2 := true, true, true, true
			for k := range 11 {
				r, col := at(j+k, i), at(i, j+k)
				r1 = r1 && r == p1[k]
				r2 = r2 && r == p2[k]
				c1 = c1 && col == p1[k]
				c2 = c2 && col == p2[k]
			}
			if r1 || r2 {
				p += 40
			}
			if c1 || c2 {
				p += 40
			}
		}
	}
	// Rule 4: balance, 10 for each 5% from half.
	dark := 0
	for _, d := range c.dark {
		if d {
			dark++
		}
	}
	total := n * n
	p += abs(20*dark-10*total) / total * 10
	return p
}
