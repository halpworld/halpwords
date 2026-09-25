package proc

import (
	"image"
	"image/color"
	"math"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/pal"
)

// IconSize is the size of the icon's art, in pixels.
const IconSize = 64

// letterArt is the letter on the icon's rune tile, as string art.
var letterArt = [...]string{
	".....##...",
	"....##....",
	"..........",
	"..######..",
	".##....##.",
	"##......##",
	"##########",
	"##........",
	"##......##",
	".##....##.",
	"..######..",
}

// Icon draws the game's icon from its own art: a Green Slime in a torch-lit
// crypt, under a stone tile carved with a golden letter, in a rounded
// frame like the game's windows.
func Icon() *image.RGBA {
	const n = IconSize
	img := BrickWall(n, n, Themes[0].Wall, 7)
	// Warm torchlight from above, then darkness at the edges.
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			d := math.Hypot(float64(x-n/2), float64(y-n/3)) / n
			k := max(0, 0.55-d) * 0.9
			k = math.Floor(k*8+bayer4[y&3][x&3]) / 8
			c := img.RGBAAt(x, y)
			img.SetRGBA(x, y, color.RGBA{
				uint8(min(255, float64(c.R)+k*140)),
				uint8(min(255, float64(c.G)+k*80)),
				uint8(min(255, float64(c.B)+k*20)), 255})
		}
	}
	Vignette(img, 0.9)

	// The rune tile: stone with a bevel, and a glowing letter.
	tx, ty, tw, th := n/2-10, 8, 20, 19
	fill(img, tx+1, ty+1, tw, th, pal.Black) // shadow
	fill(img, tx, ty, tw, th, pal.Stone)
	fill(img, tx+1, ty+1, tw-2, th-2, pal.Granite)
	fill(img, tx, ty, tw, 1, pal.Ash)
	fill(img, tx, ty, 1, th, pal.Ash)
	fill(img, tx, ty+th-1, tw, 1, pal.Slate)
	fill(img, tx+tw-1, ty, 1, th, pal.Slate)
	lx, ly := tx+(tw-len(letterArt[0]))/2, ty+(th-len(letterArt))/2
	for y, row := range letterArt {
		for x := range len(row) {
			if row[x] == '#' {
				img.SetRGBA(lx+x+1, ly+y+1, pal.Brown)
			}
		}
	}
	for y, row := range letterArt {
		for x := range len(row) {
			if row[x] == '#' {
				img.SetRGBA(lx+x, ly+y, pal.Yellow)
			}
		}
	}

	// The slime, on the floor under the tile.
	slime := MonsterSprite(dungeon.Slime, 0, 3, 0).RGBA()
	sx, sy := n/2-16, n-36
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if c := slime.RGBAAt(x, y); c.A > 0 {
				img.SetRGBA(sx+x, sy+y, c)
			}
		}
	}

	frame(img)
	return img
}

// fill paints a rectangle of img.
func fill(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			img.SetRGBA(i, j, c)
		}
	}
}

// frame rounds the icon's corners and gives it the double border of the
// game's windows.
func frame(img *image.RGBA) {
	n := img.Bounds().Dx()
	const r = 10 // corner radius
	// dist is how far (x, y) is inside the rounded square's edge.
	dist := func(x, y int) float64 {
		cx := max(r, min(n-1-r, x))
		cy := max(r, min(n-1-r, y))
		if cx == x || cy == y {
			return float64(min(x, y, n-1-x, n-1-y))
		}
		return r - math.Hypot(float64(x-cx), float64(y-cy))
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			switch d := dist(x, y); {
			case d < 0:
				img.SetRGBA(x, y, color.RGBA{})
			case d < 1:
				img.SetRGBA(x, y, pal.Black)
			case d < 3:
				img.SetRGBA(x, y, pal.Tan)
			case d < 4:
				img.SetRGBA(x, y, pal.Bronze)
			case d < 5:
				img.SetRGBA(x, y, pal.Black)
			}
		}
	}
}

// IconAt returns the icon at size×size pixels: scaled up with sharp pixels,
// or down by averaging, so small icons stay clear.
func IconAt(size int) *image.RGBA {
	src := Icon()
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	if size >= IconSize {
		k := size / IconSize
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				out.SetRGBA(x, y, src.RGBAAt(min(IconSize-1, x/k), min(IconSize-1, y/k)))
			}
		}
		return out
	}
	k := IconSize / size
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a int
			for j := 0; j < k; j++ {
				for i := 0; i < k; i++ {
					c := src.RGBAAt(x*k+i, y*k+j)
					// Average premultiplied colours, so clear corners don't darken edges.
					r, g, b, a = r+int(c.R)*int(c.A), g+int(c.G)*int(c.A), b+int(c.B)*int(c.A), a+int(c.A)
				}
			}
			if a > 0 {
				out.SetRGBA(x, y, color.RGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(a / (k * k))})
			}
		}
	}
	return out
}
