package game

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/profile"
)

// crtSource is a Kage shader that makes the screen look like an old
// monitor: a slightly curved glass, scanlines, a soft glow between
// pixels and darker corners. It samples the logical screen without
// smoothing, so the pixels stay sharp.
const crtSource = `//kage:unit pixels
package main

// Strength is 0 for none to 1 for a strong effect.
var Strength float
// Scan is how dark the gaps between scanlines are. It is 0 when the screen
// is too small to show them.
var Scan float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	origin := imageSrc0Origin()
	size := imageSrc0Size()
	p := (srcPos - origin) / size

	// Bend the picture, as on curved glass.
	c := p*2 - 1
	c *= 1 + 0.035*Strength*(c.yx*c.yx)
	p = c*0.5 + 0.5
	if p.x < 0 || p.x > 1 || p.y < 0 || p.y > 1 {
		return vec4(0, 0, 0, 1)
	}

	sp := p * size
	texel := floor(sp) + 0.5
	col := imageSrc0UnsafeAt(origin + texel).rgb
	// A little light bleeds in from the pixels either side.
	l := imageSrc0At(origin + texel - vec2(1, 0)).rgb
	r := imageSrc0At(origin + texel + vec2(1, 0)).rgb
	col = mix(col, max(col, (l+r)*0.5), 0.3*Strength)

	// Scanlines: each row is brightest in its middle.
	d := abs(fract(sp.y) - 0.5) * 2
	col *= 1 - Scan*d*d

	// Darker corners, and a lift so the picture isn't dimmer overall.
	v := 16 * p.x * p.y * (1 - p.x) * (1 - p.y)
	col *= mix(1, pow(v, 0.25), Strength)
	col *= 1 + 0.12*Strength + 0.3*Scan
	return vec4(col, 1)
}
`

// crt draws the logical screen through the CRT shader.
type crt struct {
	shader *ebiten.Shader
	failed bool
}

// draw draws src over the rectangle (x, y, w, h) of dst with the effect at
// the given setting. It reports false if the shader can't be used, and the
// caller should draw without it.
func (c *crt) draw(dst ebiten.FinalScreen, src *ebiten.Image, x, y, w, h, scale float64, setting profile.CRT) bool {
	if c.failed {
		return false
	}
	if c.shader == nil {
		s, err := ebiten.NewShader([]byte(crtSource))
		if err != nil {
			log.Printf("CRT filter is off: %v", err)
			c.failed = true
			return false
		}
		c.shader = s
	}
	strength, scan := 0.5, 0.3
	if setting == profile.CRTStrong {
		strength, scan = 1, 0.5
	}
	if scale < 2 {
		scan = 0 // lines finer than a pixel just blur
	}
	sw, sh := float32(src.Bounds().Dx()), float32(src.Bounds().Dy())
	x0, y0, x1, y1 := float32(x), float32(y), float32(x+w), float32(y+h)
	vs := []ebiten.Vertex{
		{DstX: x0, DstY: y0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x1, DstY: y0, SrcX: sw, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x0, DstY: y1, SrcX: 0, SrcY: sh, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x1, DstY: y1, SrcX: sw, SrcY: sh, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	op := &ebiten.DrawTrianglesShaderOptions{Uniforms: map[string]any{"Strength": strength, "Scan": scan}}
	op.Images[0] = src
	dst.DrawTrianglesShader(vs, []uint16{0, 1, 2, 1, 2, 3}, c.shader, op)
	return true
}
