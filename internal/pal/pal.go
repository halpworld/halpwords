// Package pal defines the game's 32-colour palette (DawnBringer 32).
// Every procedural graphic picks its colours from here, which keeps the
// 8-bit look consistent.
package pal

import "image/color"

var (
	Black    = rgb(0x000000)
	Night    = rgb(0x222034)
	Plum     = rgb(0x45283c)
	Mahogany = rgb(0x663931)
	Brown    = rgb(0x8f563b)
	Orange   = rgb(0xdf7126)
	Tan      = rgb(0xd9a066)
	Skin     = rgb(0xeec39a)
	Yellow   = rgb(0xfbf236)
	Lime     = rgb(0x99e550)
	Green    = rgb(0x6abe30)
	Teal     = rgb(0x37946e)
	Forest   = rgb(0x4b692f)
	Olive    = rgb(0x524b24)
	Slate    = rgb(0x323c39)
	Indigo   = rgb(0x3f3f74)
	Navy     = rgb(0x306082)
	Blue     = rgb(0x5b6ee1)
	Sky      = rgb(0x639bff)
	Cyan     = rgb(0x5fcde4)
	Ice      = rgb(0xcbdbfc)
	White    = rgb(0xffffff)
	Steel    = rgb(0x9badb7)
	Ash      = rgb(0x847e87)
	Stone    = rgb(0x696a6a)
	Granite  = rgb(0x595652)
	Purple   = rgb(0x76428a)
	Red      = rgb(0xac3232)
	Rose     = rgb(0xd95763)
	Pink     = rgb(0xd77bba)
	Moss     = rgb(0x8f974a)
	Bronze   = rgb(0x8a6f30)
)

func rgb(v uint32) color.RGBA {
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
}

// Fade returns c with alpha a (0..1), premultiplied as Ebitengine expects.
func Fade(c color.RGBA, a float64) color.RGBA {
	if a < 0 {
		a = 0
	} else if a > 1 {
		a = 1
	}
	return color.RGBA{uint8(float64(c.R) * a), uint8(float64(c.G) * a), uint8(float64(c.B) * a), uint8(255 * a)}
}
