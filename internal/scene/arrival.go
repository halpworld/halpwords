package scene

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

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
	"Sky Garden":         "Climb high above the clouds.",
	"Sandstone Tomb":     "Sunlit sand and secret doors.",
	"Whispering Library": "Books whisper. Listen closely.",
}

// arrivalLine is one line of the arrival card.
type arrivalLine struct {
	text  string
	scale int
	col   color.RGBA
}

// arrivalLines is what the card says: the floor, its headline name, the
// world's name when the headline is something else, and the world's
// tagline for a generated floor.
func arrivalLines(depth int, headline, world string, generated bool) []arrivalLine {
	lines := []arrivalLine{
		{fmt.Sprintf("Floor %d", depth), 3, pal.Yellow},
		{headline, 2, pal.White},
	}
	if headline != world && world != "" {
		lines = append(lines, arrivalLine{world, 1, pal.Tan})
	}
	if t := taglines[world]; generated && t != "" {
		lines = append(lines, arrivalLine{t, 1, pal.Tan})
	}
	return lines
}

// arrivalHeadline is the name the card leads with: the quest map's title,
// else the Director's name for the floor, else the world's name.
func (c *Crawl) arrivalHeadline() string { return c.floorName() }

// startArrival shows the arrival card for the floor.
func (c *Crawl) startArrival() {
	c.banner, c.sub, c.bannerT = "", "", 0
	generated := c.run.questMap(c.run.depth) == nil
	c.arrival = arrivalLines(c.run.depth, c.arrivalHeadline(), c.theme.Name, generated)
	c.arriveT = arrivalTicks
}

// skipArrival ends the card at once, when a key to skip it was pressed.
// The key does nothing else. It reports whether it did.
func (c *Crawl) skipArrival() bool {
	if c.arriveT > 0 && c.mode == modeExplore && input.Pressed(ebiten.KeySpace, ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeyEscape) {
		c.arriveT = 0
		return true
	}
	return false
}

// calm reports whether the player asked for calm effects.
func (c *Crawl) calm(ctx *game.Context) bool { return ctx.Calm() }

// fitText shortens s with an ellipsis until it is at most w wide.
func fitText(f *gfx.Font, s string, scale, w int) string {
	r := []rune(s)
	for len(r) > 1 && f.Width(string(r), scale) > w {
		r = r[:len(r)-1]
		s = string(r) + "…"
		if f.Width(s, scale) <= w {
			return s
		}
	}
	return string(r)
}

// placedLine is a line of the card with its place, in screen pixels.
type placedLine struct {
	arrivalLine
	x, y int
}

// layoutArrival centres the lines in a view vw by vh at (viewX, viewY),
// sized by measuring the text, and returns them with the box behind them.
func layoutArrival(f *gfx.Font, lines []arrivalLine, vw, vh int) ([]placedLine, image.Rectangle) {
	const pad, gap = 8, 4
	maxW := vw - 2*pad - 8
	placed := make([]placedLine, len(lines))
	w, h := 0, 0
	for i, l := range lines {
		l.scale = f.FitScale(l.text, maxW, l.scale)
		l.text = fitText(f, l.text, l.scale, maxW)
		placed[i].arrivalLine = l
		w = max(w, f.Width(l.text, l.scale))
		h += gfx.LineHeight * l.scale
		if i > 0 {
			h += gap
		}
	}
	box := image.Rect(0, 0, w+2*pad, h+2*pad)
	box = box.Add(image.Pt(viewX+(vw-box.Dx())/2, viewY+min(40, max(0, (vh-box.Dy())/3))))
	y := box.Min.Y + pad
	for i := range placed {
		l := &placed[i]
		l.x = viewX + vw/2 - f.Width(l.text, l.scale)/2
		l.y = y
		y += gfx.LineHeight*l.scale + gap
	}
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
	gfx.FillRect(view, box.Min.X, box.Min.Y, box.Dx(), box.Dy(), pal.Fade(pal.Black, 0.55*a))
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
