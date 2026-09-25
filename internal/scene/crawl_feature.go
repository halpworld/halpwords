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
	"github.com/halpworld/halpwords/internal/raycast"
	"github.com/halpworld/halpwords/pkg/proc"
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
	c.fxRunes()
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
	c.fxHeal()
	msg := "You rest by the campfire."
	if hp > 0 || mp > 0 {
		msg += fmt.Sprintf(" +%d HP, +%d MP.", hp, mp)
	}
	r.say(msg, pal.Lime)
	c.weakest = c.weakest[:0]
	c.insight = c.insight[:0]
	ids := c.weakestWords()
	var tipped, rest []int
	for _, id := range ids {
		tip, ok := r.ai.tipFor(r, id)
		if !ok || len(tipped) == maxInsight {
			if !ok {
				r.ai.queueTipIf(id) // for the next campfire
			}
			rest = append(rest, id)
			continue
		}
		tipped = append(tipped, id)
		c.insight = append(c.insight, r.deck.Entries()[id].Answers[0]+": "+tip)
	}
	if len(tipped) > 0 {
		// The words with tips first, and fewer words to make room.
		ids = append(tipped, rest...)[:max(len(tipped), min(len(ids), maxWeakestWithTips))]
		r.say("A Scroll of Insight glows in the firelight.", pal.Cyan)
	}
	for _, id := range ids {
		e := r.deck.Entries()[id]
		c.weakest = append(c.weakest, logLine{e.Prompt + " = " + e.Answers[0], pal.White})
	}
	c.mode = modeCampfire
}

// A campfire's Scroll of Insight shows memory tips the AI wrote for up to
// maxInsight of the weakest words, and then lists fewer words.
const (
	maxInsight         = 2
	maxWeakestWithTips = 2
	maxInsightLines    = 4
)

// weakestWords are the words the hero most needs to practise: the ones
// missed on this adventure first, then the weakest in the Grimoire.
func (c *Crawl) weakestWords() []int {
	r := c.run
	ids := r.deck.Missed()
	have := map[int]bool{}
	for _, id := range ids {
		have[id] = true
	}
	if mem := r.deck.Memory(); mem != nil {
		for _, id := range mem.Weakest(r.deck.Entries(), maxWeakest*2) {
			if !have[id] {
				ids = append(ids, id)
				have[id] = true
			}
		}
	}
	return ids[:min(len(ids), maxWeakest)]
}

// drawCampfire shows what resting did, and the words to practise.
func (c *Crawl) drawCampfire(view *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	w := vw - 40
	var tips []string
	for _, t := range c.insight {
		lines := wrap(f, t, w-24)
		if len(tips)+len(lines) > maxInsightLines {
			break
		}
		tips = append(tips, lines...)
	}
	h := 60 + 18*max(1, len(c.weakest)) + 30
	if len(tips) > 0 {
		h += 22 + 16*len(tips)
	}
	x, y := viewX+20, viewY+max(0, (vh-22-h)/2)
	gfx.Window(view, x, y, w, h)
	f.DrawCentered(view, "You rest by the fire", x+w/2, y+10, 1, pal.Yellow)
	f.DrawCentered(view, "HP and MP restored.", x+w/2, y+28, 1, pal.Lime)
	if len(c.weakest) == 0 {
		f.DrawCentered(view, "No weak words to practise. Well done!", x+w/2, y+54, 1, pal.Ice)
	} else {
		f.DrawCentered(view, "The flames show your weakest words:", x+w/2, y+50, 1, pal.Tan)
		for i, l := range c.weakest {
			f.DrawCentered(view, l.text, x+w/2, y+70+i*18, 1, l.col)
		}
	}
	if len(tips) > 0 {
		ty := y + 70 + 18*len(c.weakest) + 4
		f.DrawCentered(view, "✦ Scroll of Insight ✦", x+w/2, ty, 1, pal.Cyan)
		for i, t := range tips {
			f.DrawCentered(view, t, x+w/2, ty+20+i*16, 1, pal.Sky)
		}
	}
	c.drawHint(view, ctx, "Enter continue · G Grimoire")
}
