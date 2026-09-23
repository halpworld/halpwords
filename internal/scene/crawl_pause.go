package scene

import (
	"bytes"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/save"
)

// pauseItem is an entry in the pause menu.
type pauseItem int

const (
	pauseResume pauseItem = iota
	pauseFlee
	pauseSave
	pauseSaveQuit
	pauseQuit
)

var pauseLabels = [...]string{"Resume", "Flee", "Save game", "Save and quit", "Quit to title"}

// pause stops the game and opens the pause menu. Everything stands still,
// including the battle clock.
func (c *Crawl) pause(ctx *game.Context) {
	c.resume, c.mode = c.mode, modePause
	c.pausedAt = ctx.Tick
	c.menuSel = 0
	c.unsaved = c.hasUnsaved()
	c.play(audio.Select)
}

// unpause goes back to what the hero was doing, moving the battle clock on
// by the time spent paused so no time is lost.
func (c *Crawl) unpause(ctx *game.Context) {
	c.mode = c.resume
	if b := c.battle; b != nil {
		d := ctx.Tick - c.pausedAt
		b.start += d
		b.shown += d
	}
}

// pauseItems lists the pause menu. Fleeing is only for battles.
func (c *Crawl) pauseItems() []pauseItem {
	if c.resume == modeBattle {
		return []pauseItem{pauseResume, pauseFlee, pauseSave, pauseSaveQuit, pauseQuit}
	}
	return []pauseItem{pauseResume, pauseSave, pauseSaveQuit, pauseQuit}
}

// canChoose reports whether a pause menu item can be used now. The hero can
// only flee on their own turn, and can't save in the middle of a battle.
func (c *Crawl) canChoose(it pauseItem) bool {
	switch it {
	case pauseFlee:
		return c.battle != nil && c.battle.phase == phaseAttack
	case pauseSave, pauseSaveQuit:
		return c.resume == modeExplore
	}
	return true
}

func (c *Crawl) updatePause(ctx *game.Context) {
	items := c.pauseItems()
	move := func(step int) {
		c.play(audio.Blip)
		for range items {
			c.menuSel = (c.menuSel + step + len(items)) % len(items)
			if c.canChoose(items[c.menuSel]) {
				return
			}
		}
	}
	switch {
	case input.Back():
		c.play(audio.Back)
		c.unpause(ctx)
	case input.Up():
		move(-1)
	case input.Down():
		move(1)
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		c.choose(ctx, items[c.menuSel])
	}
}

func (c *Crawl) choose(ctx *game.Context, it pauseItem) {
	switch it {
	case pauseResume:
		c.play(audio.Select)
		c.unpause(ctx)
	case pauseFlee:
		c.unpause(ctx)
		c.flee(ctx)
	case pauseSave:
		if c.saveGame(ctx) {
			c.menuSel = 0
		}
	case pauseSaveQuit:
		if c.saveGame(ctx) {
			ctx.Replace(NewTitle(ctx))
		}
	case pauseQuit:
		if c.unsaved {
			c.mode = modeQuit
			return
		}
		c.play(audio.Back)
		ctx.Replace(NewTitle(ctx))
	}
}

// updateQuit asks whether to quit without saving.
func (c *Crawl) updateQuit(ctx *game.Context) {
	switch {
	case input.Pressed(ebiten.KeyY) || input.Confirm():
		c.play(audio.Back)
		ctx.Replace(NewTitle(ctx))
	case input.Pressed(ebiten.KeyN) || input.Back():
		c.mode = modePause
	}
}

// saveGame writes the adventure to the save slot, replacing any older save.
func (c *Crawl) saveGame(ctx *game.Context) bool {
	data, err := encodeSave(c.run, c.level, c.pos, c.facing)
	if err == nil {
		err = save.Write(saveName, data)
	}
	if err != nil {
		c.play(audio.Wrong)
		ctx.Notify("Could not save")
		c.run.say("Could not save the game: "+err.Error(), pal.Rose)
		return false
	}
	c.play(audio.Select)
	ctx.Notify("Game saved")
	c.lastSave, c.unsaved = data, false
	return true
}

// hasUnsaved reports whether anything has happened since the game was last
// saved or loaded.
func (c *Crawl) hasUnsaved() bool {
	if c.lastSave == nil {
		return true
	}
	data, err := encodeSave(c.run, c.level, c.pos, c.facing)
	return err != nil || !bytes.Equal(data, c.lastSave)
}

// drawPause draws the pause menu over the view.
func (c *Crawl) drawPause(view *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	gfx.FillRect(view, viewX, viewY, vw, vh, pal.Fade(pal.Black, 0.5))

	items := c.pauseItems()
	w, h := 300, 84+len(items)*20
	x, y := viewX+vw/2-w/2, viewY+(vh-22)/2-h/2
	gfx.Window(view, x, y, w, h)
	f.DrawCentered(view, "PAUSED", x+w/2, y+10, 2, pal.Yellow)
	for i, it := range items {
		iy := y + 48 + i*20
		col := pal.Steel
		switch {
		case !c.canChoose(it):
			col = pal.Stone
		case i == c.menuSel:
			col = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(view, "►", x+76, iy, 1, pal.Yellow)
			}
		}
		f.DrawShadow(view, pauseLabels[it], x+96, iy, 1, col)
	}

	note, ncol := "Your progress is saved.", pal.Lime
	switch {
	case c.resume != modeExplore:
		note, ncol = "Win or flee the battle to save.", pal.Tan
	case c.lastSave == nil:
		note, ncol = "This adventure is not saved yet.", pal.Tan
	case c.unsaved:
		note, ncol = "You have unsaved progress.", pal.Tan
	}
	f.DrawCentered(view, note, x+w/2, y+h-26, 1, ncol)
	c.drawHint(view, ctx, "↑/↓ choose · Enter select · Esc resume")
}
