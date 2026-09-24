package scene

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/proc"
	"github.com/halpworld/halpwords/internal/raycast"
)

// props are the sprites for shrines, campfires and merchants.
type props struct {
	shrine   [2]*proc.Indexed
	fire     [4]*proc.Indexed
	embers   *proc.Indexed
	merchant [2]*proc.Indexed
}

func newProps() props {
	var p props
	for f := range p.shrine {
		p.shrine[f] = proc.ShrineSprite(f)
		p.merchant[f] = proc.MerchantSprite(f)
	}
	for f := range p.fire {
		p.fire[f] = proc.CampfireSprite(f, false)
	}
	p.embers = proc.CampfireSprite(0, true)
	return p
}

// sprite returns the sprite for feature ft standing at (x, y).
func (p *props) sprite(ft *dungeon.Feature, x, y float64, tick uint64) raycast.Sprite {
	s := raycast.Sprite{X: x, Y: y}
	switch ft.Kind {
	case dungeon.Shrine:
		s.Img, s.Size = p.shrine[tick/30%2], 0.55
	case dungeon.Campfire:
		s.Img, s.Size = p.fire[tick/6%4], 0.4
		if ft.Used {
			s.Img = p.embers
		}
	default:
		s.Img, s.Size = p.merchant[tick/40%2], 0.62
	}
	return s
}

// useFeature uses the shrine, campfire or merchant in front of the hero.
func (c *Crawl) useFeature(ctx *game.Context, ft *dungeon.Feature) {
	c.feature = ft
	switch ft.Kind {
	case dungeon.Shrine:
		c.play(audio.Select)
		c.mode = modeShrine
	case dungeon.Campfire:
		c.rest(ft)
	case dungeon.Merchant:
		c.openShop(ctx)
	}
}

// updateShrine asks whether to pray at the shrine.
func (c *Crawl) updateShrine(ctx *game.Context) {
	switch {
	case input.Pressed(ebiten.KeyY) || input.Confirm():
		c.pray(ctx)
		c.mode = modeExplore
	case input.Pressed(ebiten.KeyN) || input.Back():
		c.mode = modeExplore
	}
}

// pray saves the adventure at the shrine. Falling later brings the hero
// back here.
func (c *Crawl) pray(ctx *game.Context) {
	r := c.run
	r.shrine = r.here(c.level, c.pos, c.facing)
	if !c.writeSave(ctx, false) {
		c.play(audio.Wrong)
		return
	}
	c.play(audio.Pray)
	ctx.Notify("Game saved")
	c.showBanner("SAVED", "The shrine will remember you")
	r.say("You pray at the shrine. If you fall, you will wake here.", pal.Cyan)
}

// wakeText says where a fallen hero will wake up.
func (c *Crawl) wakeText() string {
	if d := c.run.shrine.Depth; d > 1 || c.run.shrine.Floor != nil {
		return fmt.Sprintf("Wake at the shrine on floor %d?", d)
	}
	return "Wake at the dungeon's entrance?"
}

// maxWeakest is how many words a campfire shows.
const maxWeakest = 5

// rest restores HP and MP at a campfire, once, and shows the words the
// hero has been missing.
func (c *Crawl) rest(ft *dungeon.Feature) {
	r, h := c.run, &c.run.hero
	if ft.Used {
		r.info("The campfire has burnt down to embers.")
		c.bump(c.facing)
		return
	}
	ft.Used = true
	hp, mp := h.Heal(h.MaxHP()), h.Restore(h.MaxMP())
	c.play(audio.Rest)
	msg := "You rest by the campfire."
	if hp > 0 || mp > 0 {
		msg += fmt.Sprintf(" +%d HP, +%d MP.", hp, mp)
	}
	r.say(msg, pal.Lime)
	c.weakest = c.weakest[:0]
	entries := r.deck.Entries()
	for _, id := range r.deck.Missed() {
		if len(c.weakest) == maxWeakest {
			break
		}
		e := entries[id]
		c.weakest = append(c.weakest, logLine{e.Prompt + " = " + e.Answers[0], pal.White})
	}
	c.mode = modeCampfire
}

// drawCampfire shows what resting did, and the words to practise.
func (c *Crawl) drawCampfire(view *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	w, h := vw-40, 60+18*max(1, len(c.weakest))+30
	x, y := viewX+20, viewY+(vh-22-h)/2
	gfx.Window(view, x, y, w, h)
	f.DrawCentered(view, "You rest by the fire", x+w/2, y+10, 1, pal.Yellow)
	f.DrawCentered(view, "HP and MP restored.", x+w/2, y+28, 1, pal.Lime)
	if len(c.weakest) == 0 {
		f.DrawCentered(view, "No missed words to practise. Well done!", x+w/2, y+54, 1, pal.Ice)
	} else {
		f.DrawCentered(view, "The flames show words you missed:", x+w/2, y+50, 1, pal.Tan)
		for i, l := range c.weakest {
			f.DrawCentered(view, l.text, x+w/2, y+70+i*18, 1, l.col)
		}
	}
	c.drawHint(view, ctx, "Enter continue")
}
