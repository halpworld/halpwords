package scene

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/internal/words"
)

// puzzleKind is what a word puzzle unlocks.
type puzzleKind int

const (
	puzzleDoor  puzzleKind = iota // a rune-sealed door
	puzzleChest                   // a locked chest; the answer is shown with gaps
)

// puzzle is a word lock on a sealed door or a chest. There is no clock:
// puzzles are for careful spelling.
type puzzle struct {
	kind   puzzleKind
	at     dungeon.Point
	word   words.Entry
	wordID int
	hint   string // the answer with some letters hidden, for chests
	field  *typing.Field

	showing  bool // showing the result
	solved   bool
	title    string
	titleCol color.RGBA
	lines    []logLine
}

func (c *Crawl) startPuzzle(at dungeon.Point, kind puzzleKind) {
	f := typing.NewField(c.run.lang)
	f.Greek = c.run.greek
	c.puzzle = &puzzle{kind: kind, at: at, field: f}
	c.mode = modePuzzle
	c.muted = true
	c.queued = actNone
	c.dealPuzzle()
	if kind == puzzleDoor {
		c.run.info("Glowing runes seal this door. Spell the word to break them.")
	} else {
		c.run.info("The chest has a letter lock. Fill in the missing letters.")
	}
}

func (c *Crawl) dealPuzzle() {
	p := c.puzzle
	p.word, p.wordID = c.run.deck.Next()
	p.hint = ""
	if p.kind == puzzleChest {
		p.hint = words.Blank(p.word.Answers[0], c.run.rng)
	}
	p.field.Reset()
	p.showing = false
}

func (c *Crawl) updatePuzzle(ctx *game.Context) {
	p := c.puzzle
	if p.showing {
		if input.Confirm() || input.Back() {
			if p.solved || input.Back() {
				c.puzzle = nil
				c.mode = modeExplore
			} else {
				c.dealPuzzle()
			}
		}
		return
	}
	if input.Back() {
		c.puzzle = nil
		c.mode = modeExplore
		return
	}
	if c.muted {
		return
	}
	if typeInto(ctx, p.field) {
		c.solvePuzzle()
	}
	c.run.greek = p.field.Greek
}

// solvePuzzle checks the answer. Accents may slip; anything worse zaps the
// hero.
func (c *Crawl) solvePuzzle() {
	p, h := c.puzzle, &c.run.hero
	typed := p.field.Text()
	l := c.run.lang
	res := words.Grade(typed, p.word, l, l.Defaults, p.field.UsedBackspace)
	c.scoreAnswer(p.wordID, res.Tier)
	p.lines = answerLines(p.word, res, typed)
	p.showing = true
	if res.Tier < words.AccentSlip {
		h.HP -= 2
		c.hurt, c.shake = 12, 6
		c.float("-2", pal.Rose)
		p.title, p.titleCol = "ZAP!", pal.Rose
		msg := "The runes flare and sting you: 2 damage."
		if p.kind == puzzleChest {
			msg = "A needle springs from the lock: 2 damage."
		}
		p.lines = append(p.lines, logLine{msg, pal.Rose})
		c.run.say(msg, pal.Rose)
		if h.HP <= 0 {
			c.die()
		}
		return
	}
	p.solved = true
	p.title, p.titleCol = res.Tier.String()+"!", tierColor[res.Tier]
	if p.kind == puzzleDoor {
		c.level.Set(p.at, dungeon.OpenDoor)
		p.lines = append(p.lines, logLine{"The runes fade and the door swings open.", pal.Lime})
		c.run.say("The seal breaks and the door opens.", pal.Lime)
		return
	}
	ch := c.level.Chests[p.at]
	ch.Open = true
	h.Gold += ch.Gold
	h.Potions += ch.Potions
	loot := fmt.Sprintf("The chest opens: %d gold", ch.Gold)
	if ch.Potions > 0 {
		loot += fmt.Sprintf(" and %d potion", ch.Potions)
		if ch.Potions > 1 {
			loot += "s"
		}
	}
	loot += "!"
	p.lines = append(p.lines, logLine{loot, pal.Yellow})
	c.run.say(loot, pal.Yellow)
	c.float(fmt.Sprintf("+%d gold", ch.Gold), pal.Yellow)
}

func (c *Crawl) drawPuzzlePanel(dst *ebiten.Image, ctx *game.Context, x, y, w int) {
	p, f := c.puzzle, ctx.Font
	cx := x + w/2
	if p.showing {
		drawResult(dst, ctx, p.title, p.titleCol, p.lines, cx, y)
		return
	}
	label := "SEALED DOOR · Spell in " + c.run.lang.Name + ":"
	if p.kind == puzzleChest {
		label = "LOCKED CHEST · Fill in the " + c.run.lang.Name + " word:"
	}
	f.DrawShadow(dst, label, x+12, y+10, 1, pal.Pink)
	prompt := p.word.Prompt
	if p.hint != "" {
		prompt += "  →  " + p.hint
	}
	sc := f.FitScale(prompt, w-40, 2)
	f.DrawCentered(dst, prompt, cx, y+28, sc, pal.White)
	drawTyped(dst, ctx, p.field.Text(), cx, y+60, min(560, w-40), 2, !c.muted)
}

func (c *Crawl) puzzleHelp() string {
	if c.puzzle.showing {
		if c.puzzle.solved {
			return "Enter continue"
		}
		return "Enter try another word · Esc leave"
	}
	if c.muted {
		return "Let go of the movement keys to start"
	}
	return "Enter check · Esc leave"
}
