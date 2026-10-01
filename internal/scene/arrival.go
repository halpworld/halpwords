package scene

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/text/unicode/norm"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
)

const (
	arrivalTicks = 150 // how long the floor-arrival card stays
	arrivalFade  = 30  // the last ticks, spent fading out
)

// taglines are one short line for each world, by theme name. A world
// without one shows none.
var taglines = map[string]string{
	"The Crypt":          "Old stones keep old secrets.",
	"Mossy Cellars":      "Soft moss. Drip, drip, drip.",
	"Flooded Caves":      "Follow the echoes of the water.",
	"Ice Halls":          "Frosty halls glitter in the cold.",
	"Lava Forge":         "The forge glows warm and bright.",
	"Amethyst Vaults":    "Crystals hum a quiet tune.",
	"Clockwork Workshop": "Clocks tick. Gears turn.",
	"Sky Garden":         "Hedges, flowers and open sky.",
	"Sandstone Tomb":     "Cool sand and glowing glyphs.",
	"Whispering Library": "Books whisper. Listen closely.",
}

// arrivalHint is the key that skips the card, in its corner.
const arrivalHint = "Space"

// arrivalLine is one line of the arrival card.
type arrivalLine struct {
	text  string
	scale int
	col   color.RGBA
	wrap  bool // may take two lines before it is shrunk
}

// arrivalLines is what the card says: the floor, its headline name, the
// world's name when the headline is something else, and the world's
// tagline for a generated floor.
func arrivalLines(depth int, headline, world string, generated bool) []arrivalLine {
	lines := []arrivalLine{
		{fmt.Sprintf("Floor %d", depth), 3, pal.Yellow, false},
		{headline, 2, pal.White, true},
	}
	if headline != world && world != "" {
		lines = append(lines, arrivalLine{world, 1, pal.White, false})
	}
	if t := taglines[world]; generated && t != "" {
		lines = append(lines, arrivalLine{t, 1, pal.White, false})
	}
	return lines
}

// arrivalCard is the card for the floor the hero is on.
func (c *Crawl) arrivalCard() []arrivalLine {
	generated := c.run.questMap(c.run.depth) == nil
	return arrivalLines(c.run.depth, c.floorName(), c.theme.Name, generated)
}

// startArrival shows the arrival card for the floor. A banner already up
// is left alone.
func (c *Crawl) startArrival() {
	c.arrival = c.arrivalCard()
	c.arriveT = arrivalTicks
}

// enter changes the mode. The arrival card does not outlive explore mode.
func (c *Crawl) enter(m mode) {
	c.mode = m
	if m != modeExplore {
		c.arriveT = 0
	}
}

// skipArrival ends the card at once when Space or Enter is pressed, and
// the key does nothing else. Esc is the menu key and is left alone: it
// opens the menu, which clears the card. It reports whether it skipped.
func (c *Crawl) skipArrival() bool {
	if c.arriveT > 0 && c.mode == modeExplore && input.Pressed(ebiten.KeySpace, ebiten.KeyEnter, ebiten.KeyNumpadEnter) {
		c.arriveT = 0
		return true
	}
	return false
}

// calm reports whether the player asked for calm effects.
func (c *Crawl) calm(ctx *game.Context) bool { return ctx.Calm() }

// clusters splits s into user-visible letters: a base rune with the
// combining marks after it, so accents are never cut off.
func clusters(s string) []string {
	var out []string
	for _, r := range norm.NFC.String(s) {
		if len(out) > 0 && unicode.Is(unicode.M, r) {
			out[len(out)-1] += string(r)
			continue
		}
		out = append(out, string(r))
	}
	return out
}

// wrapText breaks s into lines at most w wide at scale, at spaces where it
// can and between letters where a word is too long.
func wrapText(f *gfx.Font, s string, scale, w int) []string {
	var lines []string
	cur := ""
	add := func(word string) {
		try := word
		if cur != "" {
			try = cur + " " + word
		}
		if f.Width(try, scale) <= w {
			cur = try
			return
		}
		if cur != "" {
			lines = append(lines, cur)
			cur = ""
		}
		for _, cl := range clusters(word) {
			if cur != "" && f.Width(cur+cl, scale) > w {
				lines = append(lines, cur)
				cur = ""
			}
			cur += cl
		}
	}
	for _, word := range strings.Fields(s) {
		add(word)
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// ellipsis cuts s to fit w at scale and ends it with "…".
func ellipsis(f *gfx.Font, s string, scale, w int) string {
	cl := clusters(s)
	for len(cl) > 0 && f.Width(strings.Join(cl, "")+"…", scale) > w {
		cl = cl[:len(cl)-1]
	}
	return strings.Join(cl, "") + "…"
}

// fitLines sets l in at most maxLines lines of at most w: first at its own
// scale, then smaller, and at last cut short with an ellipsis.
func fitLines(f *gfx.Font, l arrivalLine, w int) []arrivalLine {
	maxLines := 1
	if l.wrap {
		maxLines = 2
	}
	var parts []string
	for sc := l.scale; sc >= 1; sc-- {
		l.scale = sc
		if parts = wrapText(f, l.text, sc, w); len(parts) <= maxLines {
			break
		}
	}
	if len(parts) > maxLines {
		parts[maxLines-1] = ellipsis(f, strings.Join(parts[maxLines-1:], " "), l.scale, w)
		parts = parts[:maxLines]
	}
	out := make([]arrivalLine, len(parts))
	for i, p := range parts {
		l.text = p
		out[i] = l
	}
	return out
}

// placedLine is a line of the card with its place, in screen pixels.
type placedLine struct {
	arrivalLine
	x, y int
	hint bool // the key hint in the corner, not centred
}

// layoutArrival centres the lines in a view vw by vh at (viewX, viewY),
// sized by measuring the text, and returns them with the box behind them.
func layoutArrival(f *gfx.Font, lines []arrivalLine, vw, vh int) ([]placedLine, image.Rectangle) {
	const pad, gap = 8, 4
	maxW := vw - 2*pad - 8
	var placed []placedLine
	w, h := 0, 0
	for _, l := range lines {
		for _, p := range fitLines(f, l, maxW) {
			placed = append(placed, placedLine{arrivalLine: p})
			w = max(w, f.Width(p.text, p.scale))
			h += gfx.LineHeight * p.scale
			if len(placed) > 1 {
				h += gap
			}
		}
	}
	hw := f.Width(arrivalHint, 1)
	w = max(w, hw)
	h += gap + gfx.LineHeight
	box := image.Rect(0, 0, w+2*pad, h+2*pad)
	box = box.Add(image.Pt(viewX+(vw-box.Dx())/2, viewY+min(40, max(0, (vh-box.Dy())/3))))
	y := box.Min.Y + pad
	for i := range placed {
		l := &placed[i]
		l.x = viewX + vw/2 - f.Width(l.text, l.scale)/2
		l.y = y
		y += gfx.LineHeight*l.scale + gap
	}
	placed = append(placed, placedLine{
		arrivalLine: arrivalLine{arrivalHint, 1, pal.Ash, false},
		x:           box.Max.X - pad - hw, y: box.Max.Y - pad - gfx.LineHeight, hint: true,
	})
	return placed, box
}

// drawArrival draws the card over the 3D view, fading at the end.
func (c *Crawl) drawArrival(view *ebiten.Image, ctx *game.Context) {
	if c.arriveT <= 0 || c.mode != modeExplore || len(c.arrival) == 0 {
		return
	}
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	a := min(1, float64(c.arriveT)/arrivalFade)
	placed, box := layoutArrival(ctx.Font, c.arrival, vw, vh)
	gfx.FillRect(view, box.Min.X, box.Min.Y, box.Dx(), box.Dy(), pal.Fade(pal.Black, 0.7*a))
	for _, l := range placed {
		ctx.Font.DrawOutline(view, l.text, l.x, l.y, l.scale, pal.Fade(l.col, a), pal.Fade(pal.Black, a))
	}
}

// raidTheme is the look of the boss's lair: always the Mossy Cellars,
// whatever order the themes come in.
func raidTheme() *proc.Theme {
	for i := range proc.Themes {
		if proc.Themes[i].Name == "Mossy Cellars" {
			return &proc.Themes[i]
		}
	}
	return proc.ThemeFor(3)
}
