package scene

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/puzzle"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/internal/words"
)

// lockPuzzle is a word puzzle on a sealed door or a chest. There is no
// clock: puzzles are for careful spelling.
type lockPuzzle struct {
	lock  puzzle.Lock
	at    dungeon.Point
	p     puzzle.Puzzle
	field *typing.Field // nil for Pick puzzles
	pick  int           // the highlighted tile, for Pick puzzles

	showing  bool // showing the result
	solved   bool
	mimic    bool // the chest was a Mimic; the fight starts next
	title    string
	titleCol color.RGBA
	lines    []logLine
}

func (c *Crawl) startPuzzle(at dungeon.Point, lock puzzle.Lock) {
	c.puzzle = &lockPuzzle{lock: lock, at: at}
	c.mode = modePuzzle
	c.muted = true
	c.queued = actNone
	c.dealPuzzle()
	if lock == puzzle.Door {
		c.run.info("Glowing runes seal this door. Solve the word puzzle to break them.")
	} else {
		c.run.info("The chest has a letter lock. Solve the word puzzle to open it.")
	}
}

// dealPuzzle puts a new puzzle on the lock.
func (c *Crawl) dealPuzzle() {
	lp := c.puzzle
	lp.p = puzzle.New(lp.lock, c.run.depth, c.run.deck, c.run.lang, c.run.rng)
	lp.pick, lp.showing = 0, false
	switch lp.p.Answer() {
	case puzzle.Foreign:
		lp.field = typing.NewField(c.run.lang)
		lp.field.Greek = c.run.greek
	case puzzle.Native:
		lp.field = typing.NewField(words.English)
	default:
		lp.field = nil
	}
}

var pickKeys = [][2]ebiten.Key{
	{ebiten.KeyDigit1, ebiten.KeyNumpad1},
	{ebiten.KeyDigit2, ebiten.KeyNumpad2},
	{ebiten.KeyDigit3, ebiten.KeyNumpad3},
	{ebiten.KeyDigit4, ebiten.KeyNumpad4},
}

func (c *Crawl) updatePuzzle(ctx *game.Context) {
	lp := c.puzzle
	if lp.showing {
		if !input.Confirm() && !input.Back() {
			return
		}
		switch {
		case lp.mimic:
			m := c.level.WakeMimic(lp.at, c.run.rng.Uint64())
			c.puzzle = nil
			c.startBattle(ctx, m, true)
		case lp.solved || input.Back():
			c.puzzle = nil
			c.mode = modeExplore
		default:
			c.dealPuzzle()
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
	if lp.field == nil {
		c.updatePick(lp)
		return
	}
	if typeInto(ctx, lp.field) {
		c.solvePuzzle()
	}
	if lp.p.Answer() == puzzle.Foreign {
		c.run.greek = lp.field.Greek
	}
}

// updatePick moves the highlight with the arrow keys and picks with Enter
// or a number key.
func (c *Crawl) updatePick(lp *lockPuzzle) {
	n := len(lp.p.Tiles())
	for i, keys := range pickKeys[:min(n, len(pickKeys))] {
		if input.Pressed(keys[0], keys[1]) {
			lp.pick = i
			c.solvePuzzle()
			return
		}
	}
	switch {
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyA):
		lp.pick = (lp.pick + n - 1) % n
		c.play(audio.Blip)
	case input.Repeat(ebiten.KeyArrowRight) || input.Repeat(ebiten.KeyD):
		lp.pick = (lp.pick + 1) % n
		c.play(audio.Blip)
	case input.Confirm():
		c.solvePuzzle()
	}
}

// solvePuzzle checks the answer. Accents may slip; anything worse zaps the
// hero, or wakes a Mimic.
func (c *Crawl) solvePuzzle() {
	lp, h := c.puzzle, &c.run.hero
	a := puzzle.Attempt{Pick: lp.pick}
	if lp.field != nil {
		a.Text, a.UsedBackspace = lp.field.Text(), lp.field.UsedBackspace
	}
	res := lp.p.Check(a)
	c.scoreAnswer(lp.p.Word(), res.Tier)
	lp.lines = lp.lines[:0]
	for _, s := range res.Solution {
		lp.lines = append(lp.lines, logLine{s, pal.White})
	}
	switch {
	case lp.field == nil && !res.Passed():
		lp.lines = append(lp.lines, logLine{"You picked " + lp.p.Tiles()[lp.pick] + ".", pal.Steel})
	case res.Tier == words.AccentSlip:
		lp.lines = append(lp.lines, logLine{"You typed " + a.Text + ". Watch the accents!", pal.Cyan})
	case !res.Passed() && a.Text != "":
		lp.lines = append(lp.lines, logLine{"You typed " + a.Text + ".", pal.Steel})
	}
	lp.showing = true
	if !res.Passed() {
		c.failPuzzle()
		return
	}
	lp.solved = true
	lp.title, lp.titleCol = res.Tier.String()+"!", tierColor[res.Tier]
	if lp.lock == puzzle.Door {
		c.play(audio.Unseal)
		c.level.Set(lp.at, dungeon.OpenDoor)
		lp.lines = append(lp.lines, logLine{"The runes fade and the door swings open.", pal.Lime})
		c.run.say("The seal breaks and the door opens.", pal.Lime)
		return
	}
	c.play(audio.Chest)
	ch := c.level.Chests[lp.at]
	ch.Open = true
	h.Gold += ch.Gold
	h.Potions += ch.Potions
	loot := fmt.Sprintf("The chest opens: %d gold", ch.Gold)
	if ch.Potions > 0 {
		loot += " and " + potions(ch.Potions)
	}
	loot += "!"
	lp.lines = append(lp.lines, logLine{loot, pal.Yellow})
	c.run.say(loot, pal.Yellow)
	c.float(fmt.Sprintf("+%d gold", ch.Gold), pal.Yellow)
}

// failPuzzle punishes a wrong answer: the runes or a needle sting the hero,
// and a Mimic chest wakes up.
func (c *Crawl) failPuzzle() {
	lp, h := c.puzzle, &c.run.hero
	if lp.lock == puzzle.Chest && c.level.Chests[lp.at].Mimic {
		c.play(audio.Chomp)
		c.shake = 8
		lp.mimic = true
		lp.title, lp.titleCol = "MIMIC!", pal.Rose
		msg := "The chest bares its teeth. It's a Mimic!"
		lp.lines = append(lp.lines, logLine{msg, pal.Rose})
		c.run.say(msg, pal.Rose)
		return
	}
	c.play(audio.Zap)
	h.HP -= 2
	c.hurt, c.shake = 12, 6
	c.float("-2", pal.Rose)
	lp.title, lp.titleCol = "ZAP!", pal.Rose
	msg := "The runes flare and sting you: 2 damage."
	if lp.lock == puzzle.Chest {
		msg = "A needle springs from the lock: 2 damage."
	}
	lp.lines = append(lp.lines, logLine{msg, pal.Rose})
	c.run.say(msg, pal.Rose)
	if h.HP <= 0 {
		c.die()
	}
}

// potions says how many potions there are, such as "1 potion".
func potions(n int) string {
	if n == 1 {
		return "1 potion"
	}
	return fmt.Sprintf("%d potions", n)
}

func (c *Crawl) drawPuzzlePanel(dst *ebiten.Image, ctx *game.Context, x, y, w int) {
	lp, f := c.puzzle, ctx.Font
	cx := x + w/2
	if lp.showing {
		drawResult(dst, ctx, lp.title, lp.titleCol, lp.lines, cx, y)
		return
	}
	label := "SEALED DOOR · "
	if lp.lock == puzzle.Chest {
		label = "LOCKED CHEST · "
	}
	f.DrawShadow(dst, label+lp.p.Ask(), x+12, y+10, 1, pal.Pink)
	tiles := lp.p.Tiles()
	switch {
	case lp.field == nil:
		c.drawPicks(dst, ctx, tiles, x+12, y+32, w-24)
	case len(tiles) > 0:
		f.DrawCentered(dst, lp.p.Clue(), cx, y+26, f.FitScale(lp.p.Clue(), w-40, 1), pal.White)
		drawTiles(dst, ctx, tiles, cx, y+44)
		drawTyped(dst, ctx, lp.field.Text(), cx, y+64, min(560, w-40), 2, !c.muted)
	default:
		f.DrawCentered(dst, lp.p.Clue(), cx, y+28, f.FitScale(lp.p.Clue(), w-40, 2), pal.White)
		drawTyped(dst, ctx, lp.field.Text(), cx, y+60, min(560, w-40), 2, !c.muted)
	}
}

// drawTiles draws a row of letter tiles centred on cx.
func drawTiles(dst *ebiten.Image, ctx *game.Context, tiles []string, cx, y int) {
	const size, gap = 20, 4
	x := cx - (len(tiles)*(size+gap)-gap)/2
	for i, t := range tiles {
		tx := x + i*(size+gap)
		gfx.FillRect(dst, tx, y, size, size, pal.Stone)
		gfx.FillRect(dst, tx+1, y+1, size-2, size-2, pal.Granite)
		gfx.FillRect(dst, tx+1, y+size-3, size-2, 2, pal.Slate)
		ctx.Font.DrawCentered(dst, t, tx+size/2, y+1, 1, pal.White)
	}
}

// drawPicks draws the words to pick from in a row of numbered boxes, with
// the highlighted one outlined.
func (c *Crawl) drawPicks(dst *ebiten.Image, ctx *game.Context, tiles []string, x, y, w int) {
	const gap, h = 8, 44
	f := ctx.Font
	bw := (w - gap*(len(tiles)-1)) / len(tiles)
	sc := 2 // one size for every word, so none looks like the answer
	for _, t := range tiles {
		sc = min(sc, f.FitScale(t, bw-24, 2))
	}
	for i, t := range tiles {
		bx := x + i*(bw+gap)
		edge := pal.Indigo
		if i == c.puzzle.pick && !c.muted {
			edge = pal.Yellow
		}
		gfx.FillRect(dst, bx, y, bw, h, edge)
		gfx.FillRect(dst, bx+2, y+2, bw-4, h-4, pal.Night)
		f.Draw(dst, fmt.Sprint(i+1), bx+5, y+3, 1, pal.Ash)
		f.DrawShadow(dst, t, bx+(bw-f.Width(t, sc))/2, y+h/2-8*sc, sc, pal.Ice)
	}
}

func (c *Crawl) puzzleHelp() string {
	lp := c.puzzle
	switch {
	case lp.showing && lp.mimic:
		return "Enter fight!"
	case lp.showing && lp.solved:
		return "Enter continue"
	case lp.showing:
		return "Enter try another puzzle · Esc leave"
	case c.muted:
		return "Let go of the movement keys to start"
	case lp.field == nil:
		return "1-4 or ←/→ and Enter pick · Esc leave"
	}
	return "Enter check · Esc leave"
}
