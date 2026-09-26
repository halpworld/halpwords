package proc

import (
	"image"
	"image/color"
)

// The picture set: 24 simple pixel pictures children tap for a picture
// password at school (halpwords-server's login cards, PLAN §7 Children):
// a sun, a cat, a boat and so on, each 8 by 8 pixels in one or two
// colours. They are drawn in code, so the game, the website and the
// printed sheets show the same pictures without an image file.
//
// A picture is known by its index in Pictures, which is what picture
// passwords store, and by its slug, which the website uses to name it.
// Never reorder or remove a picture: stored passwords name them by
// index. New pictures go at the end.

// PictureSize is a picture's width and height in pixels.
const PictureSize = 8

// PictureCount is how many pictures there are.
const PictureCount = 24

// Picture is one picture of the set.
type Picture struct {
	// Slug names it, such as "sun".
	Slug string
	// Main and Accent are its two colours.
	Main, Accent color.RGBA
	rows         [PictureSize]string // '#' main, '+' accent, '.' empty
}

// At returns the colour of the pixel at x, y (0 to PictureSize-1, from
// the top left), and false for an empty pixel.
func (p Picture) At(x, y int) (color.RGBA, bool) {
	if x < 0 || y < 0 || x >= PictureSize || y >= PictureSize {
		return color.RGBA{}, false
	}
	switch p.rows[y][x] {
	case '#':
		return p.Main, true
	case '+':
		return p.Accent, true
	}
	return color.RGBA{}, false
}

// Run is a horizontal run of pixels of one colour.
type Run struct {
	X, Y, W int
	Colour  color.RGBA
}

// Runs returns the picture's pixels as horizontal runs of one colour,
// top to bottom, so it can be drawn with few rectangles.
func (p Picture) Runs() []Run {
	var out []Run
	for y := range PictureSize {
		for x := 0; x < PictureSize; {
			c, ok := p.At(x, y)
			if !ok {
				x++
				continue
			}
			start := x
			for x < PictureSize {
				if d, ok := p.At(x, y); !ok || d != c {
					break
				}
				x++
			}
			out = append(out, Run{X: start, Y: y, W: x - start, Colour: c})
		}
	}
	return out
}

// Image draws the picture with each pixel scale by scale, on a
// transparent background.
func (p Picture) Image(scale int) *image.RGBA {
	scale = max(1, scale)
	img := image.NewRGBA(image.Rect(0, 0, PictureSize*scale, PictureSize*scale))
	for y := range PictureSize * scale {
		for x := range PictureSize * scale {
			if c, ok := p.At(x/scale, y/scale); ok {
				img.SetRGBA(x, y, c)
			}
		}
	}
	return img
}

// PictureAt returns the picture at index i of Pictures.
func PictureAt(i int) (Picture, bool) {
	if i < 0 || i >= len(pictures) {
		return Picture{}, false
	}
	return pictures[i], true
}

// Pictures returns the picture set, in its fixed order.
func Pictures() []Picture { return append([]Picture(nil), pictures...) }

func picRGB(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 255} }

// The pictures' colours: dark enough to print well in colour and to tell apart in
// picGrey.
var (
	picRed    = picRGB(0xc6, 0x28, 0x28)
	picOrange = picRGB(0xe6, 0x7e, 0x00)
	picYellow = picRGB(0xf2, 0xb7, 0x05)
	picGreen  = picRGB(0x2e, 0x7d, 0x32)
	picDkgrn  = picRGB(0x1b, 0x4d, 0x1e)
	picBlue   = picRGB(0x15, 0x65, 0xc0)
	picNavy   = picRGB(0x0d, 0x2c, 0x6b)
	picPurple = picRGB(0x6a, 0x1b, 0x9a)
	picPink   = picRGB(0xd8, 0x1b, 0x60)
	picBrown  = picRGB(0x6d, 0x4c, 0x41)
	picTan    = picRGB(0xc8, 0xa2, 0x6e)
	picGrey   = picRGB(0x61, 0x61, 0x61)
	picBlack  = picRGB(0x21, 0x21, 0x21)
	picWhite  = picRGB(0xe0, 0xe0, 0xe0)
)

var pictures = []Picture{
	{Slug: "sun", Main: picYellow, Accent: picOrange, rows: [PictureSize]string{
		"+..+..+.",
		".......+",
		"+.####..",
		".######.",
		".######+",
		"+.####..",
		"........",
		".+..+..+"}},
	{Slug: "heart", Main: picRed, Accent: picRed, rows: [PictureSize]string{
		"........",
		".##..##.",
		"########",
		"########",
		".######.",
		"..####..",
		"...##...",
		"........"}},
	{Slug: "star", Main: picYellow, Accent: picYellow, rows: [PictureSize]string{
		"...##...",
		"...##...",
		"########",
		".######.",
		"..####..",
		".######.",
		".##..##.",
		"##....##"}},
	{Slug: "moon", Main: picNavy, Accent: picNavy, rows: [PictureSize]string{
		"..####..",
		".###....",
		"###.....",
		"###.....",
		"###.....",
		"###.....",
		".###....",
		"..####.."}},
	{Slug: "tree", Main: picGreen, Accent: picBrown, rows: [PictureSize]string{
		"...##...",
		"..####..",
		".######.",
		"########",
		".######.",
		"...++...",
		"...++...",
		"..++++.."}},
	{Slug: "house", Main: picRed, Accent: picTan, rows: [PictureSize]string{
		"...##...",
		"..####..",
		".######.",
		"########",
		".++++++.",
		".++..++.",
		".++..++.",
		".++..++."}},
	{Slug: "fish", Main: picBlue, Accent: picWhite, rows: [PictureSize]string{
		"........",
		"...###..",
		"#.#####.",
		"####+###",
		"#.######",
		"...###..",
		"........",
		"........"}},
	{Slug: "apple", Main: picRed, Accent: picGreen, rows: [PictureSize]string{
		"....+...",
		"...+....",
		".##.##..",
		"#######.",
		"#######.",
		"#######.",
		".#####..",
		"..#.#..."}},
	{Slug: "cat", Main: picGrey, Accent: picGreen, rows: [PictureSize]string{
		"#.....#.",
		"##...##.",
		"#######.",
		"#+###+#.",
		"#######.",
		".#####..",
		"..###...",
		"........"}},
	{Slug: "dog", Main: picTan, Accent: picBrown, rows: [PictureSize]string{
		"++....++",
		"+######+",
		"+#.##.#+",
		".######.",
		".##++##.",
		"..####..",
		"...##...",
		"........"}},
	{Slug: "bird", Main: picBlue, Accent: picOrange, rows: [PictureSize]string{
		"..##....",
		".#.##...",
		".#####++",
		"######..",
		".#####..",
		"..###...",
		"...#....",
		"..#.#..."}},
	{Slug: "flower", Main: picPink, Accent: picYellow, rows: [PictureSize]string{
		"..#.#...",
		".#####..",
		"##+++##.",
		".#+++#..",
		"##+++##.",
		".#####..",
		"..#.#...",
		"........"}},
	{Slug: "boat", Main: picBrown, Accent: picBlue, rows: [PictureSize]string{
		"...#....",
		"...#+...",
		"...#++..",
		"...#+++.",
		"...#....",
		"########",
		".######.",
		"..####.."}},
	{Slug: "car", Main: picRed, Accent: picBlack, rows: [PictureSize]string{
		"........",
		"..####..",
		".#.##.#.",
		"########",
		"########",
		".++..++.",
		".++..++.",
		"........"}},
	{Slug: "key", Main: picOrange, Accent: picOrange, rows: [PictureSize]string{
		".###....",
		"#...#...",
		"#...#...",
		".###....",
		"..#.....",
		"..###...",
		"..#.....",
		"..##...."}},
	{Slug: "bell", Main: picYellow, Accent: picBrown, rows: [PictureSize]string{
		"...##...",
		"..####..",
		".######.",
		".######.",
		".######.",
		"########",
		"...++...",
		"........"}},
	{Slug: "hat", Main: picPurple, Accent: picYellow, rows: [PictureSize]string{
		"........",
		"...###..",
		"..####..",
		"..####..",
		"..++++..",
		"########",
		"........",
		"........"}},
	{Slug: "ball", Main: picRed, Accent: picWhite, rows: [PictureSize]string{
		"..####..",
		".##++##.",
		"##++++##",
		"########",
		"########",
		"##++++##",
		".##++##.",
		"..####.."}},
	{Slug: "crown", Main: picYellow, Accent: picRed, rows: [PictureSize]string{
		"........",
		"#..#..#.",
		"##.#.##.",
		"#######.",
		"#+#+#+#.",
		"#######.",
		"........",
		"........"}},
	{Slug: "leaf", Main: picGreen, Accent: picDkgrn, rows: [PictureSize]string{
		"......##",
		"....####",
		"..###+##",
		".##+####",
		".#+####.",
		".+####..",
		"+.##....",
		"........"}},
	{Slug: "drop", Main: picBlue, Accent: picWhite, rows: [PictureSize]string{
		"...#....",
		"...#....",
		"..###...",
		".##+##..",
		".#+###..",
		".#####..",
		"..###...",
		"........"}},
	{Slug: "rocket", Main: picGrey, Accent: picOrange, rows: [PictureSize]string{
		"...#....",
		"..###...",
		"..#.#...",
		"..###...",
		"..###...",
		".#####..",
		".#.+.#..",
		"...+...."}},
	{Slug: "umbrella", Main: picPurple, Accent: picBrown, rows: [PictureSize]string{
		"...#....",
		".#####..",
		"#######.",
		"#######.",
		"...+....",
		"...+....",
		".+.+....",
		"..+....."}},
	{Slug: "mushroom", Main: picRed, Accent: picWhite, rows: [PictureSize]string{
		"..####..",
		".#+##+#.",
		"########",
		"#+####+#",
		"...++...",
		"...++...",
		"..++++..",
		"........"}},
}
