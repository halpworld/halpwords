// Package unifont parses GNU Unifont .hex bitmap fonts.
//
// Each line of a .hex file has the form "XXXX:BITMAP" where XXXX is the code
// point in hex and BITMAP is 32 hex digits (an 8×16 glyph) or 64 hex digits
// (a 16×16 glyph), one row after another, most significant bit leftmost.
package unifont

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Height is the height of every Unifont glyph in pixels.
const Height = 16

// Glyph is a single bitmap glyph.
type Glyph struct {
	Width int            // 8 or 16
	Rows  [Height]uint16 // pixel x is set if Rows[y]>>(Width-1-x)&1 == 1
}

// Set reports whether pixel (x, y) is set.
func (g *Glyph) Set(x, y int) bool {
	return g.Rows[y]>>(g.Width-1-x)&1 == 1
}

// Face is a parsed font.
type Face struct {
	glyphs map[rune]*Glyph
}

// Parse reads a .hex font.
func Parse(r io.Reader) (*Face, error) {
	f := &Face{glyphs: map[rune]*Glyph{}}
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		cp, bits, ok := strings.Cut(text, ":")
		if !ok {
			return nil, fmt.Errorf("unifont: line %d: missing ':'", line)
		}
		v, err := strconv.ParseUint(cp, 16, 32)
		if err != nil {
			return nil, fmt.Errorf("unifont: line %d: bad code point: %w", line, err)
		}
		g := &Glyph{}
		switch len(bits) {
		case 32:
			g.Width = 8
		case 64:
			g.Width = 16
		default:
			return nil, fmt.Errorf("unifont: line %d: bitmap has %d digits", line, len(bits))
		}
		step := g.Width / 4
		for y := 0; y < Height; y++ {
			row, err := strconv.ParseUint(bits[y*step:(y+1)*step], 16, 16)
			if err != nil {
				return nil, fmt.Errorf("unifont: line %d: %w", line, err)
			}
			g.Rows[y] = uint16(row)
		}
		f.glyphs[rune(v)] = g
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return f, nil
}

// ParseBytes is Parse for an in-memory font.
func ParseBytes(b []byte) (*Face, error) {
	return Parse(bytes.NewReader(b))
}

// Glyph returns the glyph for r, falling back to U+FFFD and then '?'.
func (f *Face) Glyph(r rune) *Glyph {
	if g, ok := f.glyphs[r]; ok {
		return g
	}
	if g, ok := f.glyphs[0xFFFD]; ok {
		return g
	}
	return f.glyphs['?']
}

// Has reports whether the font has a glyph for r.
func (f *Face) Has(r rune) bool {
	_, ok := f.glyphs[r]
	return ok
}

// Width returns the width of s in pixels at scale 1.
func (f *Face) Width(s string) int {
	w := 0
	for _, r := range s {
		if g := f.Glyph(r); g != nil {
			w += g.Width
		}
	}
	return w
}
