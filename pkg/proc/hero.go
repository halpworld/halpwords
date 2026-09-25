package proc

import (
	"image/color"

	"github.com/halpworld/halpwords/internal/pal"
)

// heroArt is the hero, as string art. Each letter is a part that the class
// colours: h head covering, f skin, k eyes, c cape, b body, T emblem,
// t belt, w blade, g hilt, l legs, d boots.
var heroArt = [...]string{
	"......hhhh......",
	".....hhhhhh.....",
	".....hffffh.....",
	".....fkffkf.....",
	"......ffff......",
	".....cbbbbc.....",
	"....cbbbbbbc..w.",
	"...ccbbTbbbcc.w.",
	"...f.bbbbbb.f.w.",
	"...f.bbbbbb.fgg.",
	".....bbttbb..f..",
	".....bb..bb.....",
	".....ll..ll.....",
	".....ll..ll.....",
	"....ddd..ddd....",
	"................",
}

// heroColours are the colours of each part, for each class in the order
// of rpg.Classes: Knight, Scribe, Rogue.
var heroColours = []map[byte]color.RGBA{
	{'h': pal.Steel, 'c': pal.Red, 'b': pal.Ash, 'T': pal.Yellow, 't': pal.Brown, 'w': pal.Ice, 'g': pal.Bronze, 'l': pal.Stone, 'd': pal.Mahogany},
	{'h': pal.Purple, 'c': pal.Plum, 'b': pal.Blue, 'T': pal.Yellow, 't': pal.Tan, 'w': pal.White, 'g': pal.Sky, 'l': pal.Indigo, 'd': pal.Brown},
	{'h': pal.Forest, 'c': pal.Olive, 'b': pal.Brown, 'T': pal.Lime, 't': pal.Mahogany, 'w': pal.Ice, 'g': pal.Granite, 'l': pal.Slate, 'd': pal.Night},
}

// HeroSprite draws the hero of class c (0 Knight, 1 Scribe, 2 Rogue).
func HeroSprite(c int) *Indexed {
	cols := heroColours[c%len(heroColours)]
	m := NewIndexed(16, 16)
	for y, row := range heroArt {
		for x := 0; x < len(row); x++ {
			switch p := row[x]; p {
			case '.':
			case 'f':
				m.Set(x, y, pal.Skin)
			case 'k':
				m.Set(x, y, pal.Black)
			default:
				m.Set(x, y, cols[p])
			}
		}
	}
	outline(m)
	return m
}
