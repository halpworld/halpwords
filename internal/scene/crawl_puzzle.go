package scene

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/pkg/maps"
	"github.com/halpworld/halpwords/pkg/puzzle"
	"github.com/halpworld/halpwords/pkg/words"
)

// lockPuzzle is a word puzzle on a sealed door or a chest. There is no
// clock: puzzles are for careful spelling.
type lockPuzzle struct {
	lock  puzzle.Lock
	at    dungeon.Point
	p     puzzle.Puzzle
	field *typing.Field // the typing field; in a crossword, the chosen word's
	// pick is the highlighted tile (Pick), row (Match), wheel (Dial) or
	// word (Grid).
	pick   int
	choice []int           // the option on each slot, for Match and Dial
	fields []*typing.Field // one per word, for Grid

	hints    int  // letters shown by hints
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
	lp.p = c.makePuzzle(lp.lock, lp.at)
	lp.pick, lp.showing, lp.hints = 0, false, 0
	lp.field, lp.choice, lp.fields = nil, nil, nil
	switch lp.p.Answer() {
	case puzzle.Foreign:
		lp.field = c.foreignField()
	case puzzle.Native:
		lp.field = typing.NewField(words.English)
	case puzzle.Match, puzzle.Dial:
		lp.choice = lp.p.(puzzle.Chooser).Start()
		lp.pick = nextWheel(lp, -1, 1)
	case puzzle.Grid:
		_, _, placed := lp.p.(puzzle.Crossworder).Layout()
		for range placed {
			lp.fields = append(lp.fields, c.foreignField())
		}
		lp.field = lp.fields[0]
	}
}

// makePuzzle makes a puzzle for the lock at a: the one a hand-made map
// sets there, or a random one. A word the lists lack, or one that cannot
// make the kind of puzzle set, gives a puzzle of that kind about another
// word.
func (c *Crawl) makePuzzle(lock puzzle.Lock, at dungeon.Point) puzzle.Puzzle {
	r, depth := c.run, c.level.Depth
	gen := r.ai.generated(r)
	plan, ok := c.level.Locks[at]
	if !ok {
		return puzzle.NewWith(lock, depth, r.deck, r.lang, r.rules(), r.rng, gen)
	}
	if plan.Word != "" {
		id := maps.FindWord(r.deck.Entries(), plan.Word)
		kinds := []puzzle.Kind{plan.Kind}
		if !plan.Set {
			kinds = nil
			for _, k := range puzzle.Kinds(lock, depth) {
				if puzzle.OneWord(k) {
					kinds = append(kinds, k)
				}
			}
			r.rng.Shuffle(len(kinds), func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })
			kinds = append(kinds, puzzle.Spell)
		}
		for _, k := range kinds {
			if p := puzzle.MakeWord(k, lock, depth, r.deck, id, r.lang, r.rules(), r.rng, gen); p != nil {
				return p
			}
		}
	}
	if plan.Set {
		return puzzle.MakeWith(plan.Kind, lock, depth, r.deck, r.lang, r.rules(), r.rng, gen)
	}
	return puzzle.NewWith(lock, depth, r.deck, r.lang, r.rules(), r.rng, gen)
}

// foreignField returns a field for typing in the language being learned.
func (c *Crawl) foreignField() *typing.Field {
	f := typing.NewField(c.run.lang)
	f.Greek = c.run.greek
	return f
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
	if input.Pressed(ebiten.KeyF4) {
		if a := lp.p.Answer(); a == puzzle.Foreign || a == puzzle.Native {
			c.hint(&lp.hints, puzzleAnswer(lp.p))
		} else {
			c.run.info("Hints only work on puzzles you type.")
		}
	}
	switch lp.p.Answer() {
	case puzzle.Pick:
		c.updatePick(lp)
		return
	case puzzle.Match:
		c.updateMatch(lp)
		return
	case puzzle.Dial:
		c.updateDial(lp)
		return
	case puzzle.Grid:
		c.updateGrid(ctx, lp)
	default:
		if typeInto(ctx, lp.field) {
			c.solvePuzzle()
		}
	}
	if lp.field != nil && lp.p.Answer() != puzzle.Native {
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

// updateMatch chooses a row with ↑/↓ and swaps meanings with ←/→, so every
// meaning stays on one row. Enter checks.
func (c *Crawl) updateMatch(lp *lockPuzzle) {
	n := len(lp.choice)
	switch {
	case input.Up():
		lp.pick = (lp.pick + n - 1) % n
		c.play(audio.Blip)
	case input.Down():
		lp.pick = (lp.pick + 1) % n
		c.play(audio.Blip)
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyA):
		c.swapMeaning(lp, n-1)
	case input.Repeat(ebiten.KeyArrowRight) || input.Repeat(ebiten.KeyD):
		c.swapMeaning(lp, 1)
	case input.Confirm():
		c.solvePuzzle()
	}
}

// swapMeaning gives the chosen row the meaning by places further down the
// list, and the row that had it this row's old meaning.
func (c *Crawl) swapMeaning(lp *lockPuzzle, by int) {
	n := len(lp.choice)
	want := (lp.choice[lp.pick] + by) % n
	for r, m := range lp.choice {
		if m == want {
			lp.choice[r] = lp.choice[lp.pick]
		}
	}
	lp.choice[lp.pick] = want
	c.play(audio.Key)
}

// updateDial chooses a wheel with ←/→ and turns it with ↑/↓. Enter checks.
func (c *Crawl) updateDial(lp *lockPuzzle) {
	opts := lp.p.(puzzle.Chooser).Options()
	turn := func(by int) {
		n := len(opts[lp.pick])
		lp.choice[lp.pick] = (lp.choice[lp.pick] + by + n) % n
		c.play(audio.Key)
	}
	switch {
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyA):
		lp.pick = nextWheel(lp, lp.pick, -1)
		c.play(audio.Blip)
	case input.Repeat(ebiten.KeyArrowRight) || input.Repeat(ebiten.KeyD):
		lp.pick = nextWheel(lp, lp.pick, 1)
		c.play(audio.Blip)
	case input.Up():
		turn(-1)
	case input.Down():
		turn(1)
	case input.Confirm():
		c.solvePuzzle()
	}
}

// nextWheel returns the next slot from i in direction dir that is not a
// fixed plate, wrapping around. For Match puzzles every row counts.
func nextWheel(lp *lockPuzzle, i, dir int) int {
	opts := lp.p.(puzzle.Chooser).Options()
	n := len(opts)
	for k := 1; k <= n; k++ {
		j := ((i+dir*k)%n + n) % n
		if len(opts[j]) > 1 {
			return j
		}
	}
	return max(0, i)
}

// updateGrid types into the chosen crossword word. ↑/↓ choose a word, and
// Enter moves to the next empty word, or checks when all are filled in.
func (c *Crawl) updateGrid(ctx *game.Context, lp *lockPuzzle) {
	n := len(lp.fields)
	choose := func(i int) {
		lp.pick = i
		lp.field = lp.fields[i]
		lp.field.Greek = c.run.greek
		c.play(audio.Blip)
	}
	switch {
	case input.Repeat(ebiten.KeyArrowUp):
		choose((lp.pick + n - 1) % n)
		return
	case input.Repeat(ebiten.KeyArrowDown):
		choose((lp.pick + 1) % n)
		return
	}
	typeInto(ctx, lp.field)
	if !input.Confirm() {
		return
	}
	for k := 0; k < n; k++ {
		if i := (lp.pick + k) % n; lp.fields[i].Len() == 0 {
			if k > 0 {
				choose(i)
			}
			return
		}
	}
	c.solvePuzzle()
}

// solvePuzzle checks the answer. Accents may slip; anything worse zaps the
// hero, or wakes a Mimic.
func (c *Crawl) solvePuzzle() {
	lp, h := c.puzzle, &c.run.hero
	a := puzzle.Attempt{Pick: lp.pick, Choice: lp.choice}
	switch {
	case lp.fields != nil:
		for _, f := range lp.fields {
			a.Texts = append(a.Texts, f.Text())
			a.UsedBackspace = a.UsedBackspace || f.UsedBackspace
		}
	case lp.field != nil:
		a.Text, a.UsedBackspace = lp.field.Text(), lp.field.UsedBackspace
	}
	shown := a.Text // what the hero answered, in words
	if lp.p.Answer() == puzzle.Dial {
		shown = dialed(lp)
	}
	res := lp.p.Check(a)
	c.scoreAnswer(lp.p.Word(), res.Result, shown, lp.hints > 0, 0)
	lp.lines = lp.lines[:0]
	for _, s := range res.Solution {
		lp.lines = append(lp.lines, logLine{s, pal.White})
	}
	verb := "You typed "
	if lp.p.Answer() == puzzle.Dial {
		verb = "The wheels showed "
	}
	switch {
	case lp.p.Answer() == puzzle.Pick && !res.Passed():
		lp.lines = append(lp.lines, logLine{"You picked " + lp.p.Tiles()[lp.pick] + ".", pal.Steel})
	case res.Tier < words.Correct && shown != "":
		lang := c.run.lang
		if lp.p.Answer() == puzzle.Native {
			lang = words.English
		}
		lp.lines = append(lp.lines, mistakeLine(verb+shown+".", shown, res.Result, lang))
	case res.Tier == words.AccentSlip:
		lp.lines = append(lp.lines, logLine{"Watch the accents!", pal.Cyan})
	}
	lp.showing = true
	if !res.Passed() {
		c.failPuzzle()
		return
	}
	lp.solved = true
	lp.title, lp.titleCol = res.Tier.String()+"!", tierColor[res.Tier]
	xp := rpg.PuzzleXP(lp.lock == puzzle.Chest, c.level.Depth)
	if lp.lock == puzzle.Door {
		c.play(audio.Unseal)
		c.fxRunes()
		c.level.Set(lp.at, dungeon.OpenDoor)
		lp.lines = append(lp.lines, logLine{fmt.Sprintf("The runes fade and the door swings open. +%d XP", xp), pal.Lime})
		c.run.say(fmt.Sprintf("The seal breaks and the door opens. +%d XP", xp), pal.Lime)
		lp.lines = append(lp.lines, c.gainXP(xp)...)
		return
	}
	c.play(audio.Chest)
	ch := c.level.Chests[lp.at]
	ch.Open = true
	c.run.tally.Chests++
	gold := h.GoldFind(ch.Gold, true)
	h.Gold += gold
	loot := fmt.Sprintf("The chest opens: %d gold! +%d XP", gold, xp)
	lp.lines = append(lp.lines, logLine{loot, pal.Yellow})
	c.run.say(loot, pal.Yellow)
	c.float(fmt.Sprintf("+%d gold", gold), pal.Yellow)
	c.fxCoins(gold)
	lp.lines = append(lp.lines, c.takeLoot(ch, "Inside: ")...)
	lp.lines = append(lp.lines, c.gainXP(xp)...)
}

// puzzleAnswer is the answer a typed puzzle wants, for hints.
func puzzleAnswer(p puzzle.Puzzle) string {
	return p.Check(puzzle.Attempt{}).Expected
}

// takeLoot gives the hero what ch holds besides gold, and describes it.
// Gear that does not fit in the bag stays in the chest.
func (c *Crawl) takeLoot(ch *dungeon.Chest, prefix string) []logLine {
	h := &c.run.hero
	var got []string
	if ch.Potions > 0 {
		h.Items[rpg.Potion] += ch.Potions
		got = append(got, rpg.Potion.Count(ch.Potions))
		ch.Potions = 0
	}
	for _, it := range ch.Items {
		h.Items[it]++
		got = append(got, aName(it.String()))
	}
	ch.Items = nil
	var lines []logLine
	say := func(msg string) {
		lines = append(lines, logLine{msg, pal.Yellow})
		c.run.say(msg, pal.Yellow)
	}
	if len(got) > 0 {
		say(prefix + andList(got) + "!")
	}
	if g := ch.Gear; g != nil {
		switch {
		case !h.Take(*g):
			say("There is " + aName(g.Name()) + " too, but your bag is full.")
			return lines
		case h.Gear[g.Slot] != nil && *h.Gear[g.Slot] == *g:
			say("You find " + aName(g.Name()) + " and put it on! " + g.About())
		default:
			say("You find " + aName(g.Name()) + ". It goes in your bag (I).")
		}
		ch.Gear = nil
	}
	return lines
}

// aName puts "a" or "an" before a name.
func aName(s string) string {
	if s != "" && strings.ContainsRune("AEIOUaeiou", rune(s[0])) {
		return "an " + s
	}
	return "a " + s
}

// andList joins items as "a, b and c".
func andList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// dialed returns the word the tumbler's wheels show.
func dialed(lp *lockPuzzle) string {
	var b strings.Builder
	for i, w := range lp.p.(puzzle.Chooser).Options() {
		b.WriteString(w[lp.choice[i]])
	}
	return b.String()
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
	switch lp.p.Answer() {
	case puzzle.Pick:
		c.drawPicks(dst, ctx, tiles, x+12, y+32, w-24)
		return
	case puzzle.Match:
		c.drawPairs(dst, ctx, cx, y+28)
		return
	case puzzle.Dial:
		f.DrawShadow(dst, lp.p.Clue(), x+12+f.Width(label+lp.p.Ask(), 1)+8, y+10, 1, pal.White)
		c.drawWheels(dst, ctx, cx, y+28)
		return
	case puzzle.Grid:
		c.drawClues(dst, ctx, x+12, y+30)
		return
	}
	switch clue := lp.p.Clue(); {
	case f.Width(clue, 1) > w-40:
		// A riddle: a sentence or two at the normal size.
		for i, line := range wrap(f, clue, w-40) {
			f.DrawCentered(dst, line, cx, y+26+i*16, 1, pal.White)
		}
		c.drawPuzzleTyped(dst, ctx, cx, y+62, min(560, w-40))
	case len(tiles) > 0:
		f.DrawCentered(dst, lp.p.Clue(), cx, y+26, f.FitScale(lp.p.Clue(), w-40, 1), pal.White)
		drawTiles(dst, ctx, tiles, cx, y+44)
		c.drawPuzzleTyped(dst, ctx, cx, y+64, min(560, w-40))
	default:
		f.DrawCentered(dst, lp.p.Clue(), cx, y+28, f.FitScale(lp.p.Clue(), w-40, 2), pal.White)
		c.drawPuzzleTyped(dst, ctx, cx, y+60, min(560, w-40))
	}
}

// drawPuzzleTyped draws what the hero has typed, with typos marked when
// live highlighting is on.
func (c *Crawl) drawPuzzleTyped(dst *ebiten.Image, ctx *game.Context, cx, y, width int) {
	lp := c.puzzle
	good := -1
	if c.run.settings.Highlight {
		lang := c.run.lang
		if lp.p.Answer() == puzzle.Native {
			lang = words.English
		}
		good = goodPrefix(lp.field.Text(), []string{puzzleAnswer(lp.p)}, lang)
	}
	drawTypedMarked(dst, ctx, lp.field.Text(), good, cx, y, width, 2, !c.muted)
}

// wrap splits s into lines no wider than width at scale 1.
func wrap(f *gfx.Font, s string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && f.Width(line+" "+word, 1) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	return append(lines, line)
}

// drawPairs draws the words to match, each with the meaning it is set to,
// in rows centred on cx. The chosen row is highlighted.
func (c *Crawl) drawPairs(dst *ebiten.Image, ctx *game.Context, cx, y int) {
	lp, f := c.puzzle, ctx.Font
	meanings := lp.p.(puzzle.Chooser).Options()
	for i, word := range lp.p.Tiles() {
		ry := y + i*17
		meaning, col := meanings[i][lp.choice[i]], pal.Ice
		if i == lp.pick && !c.muted {
			gfx.FillRect(dst, cx-200, ry, 400, 16, pal.Indigo)
			col = pal.White
			f.Draw(dst, "←", cx+14, ry, 1, pal.Yellow)
			f.Draw(dst, "→", cx+32+f.Width(meaning, 1), ry, 1, pal.Yellow)
		}
		f.DrawShadow(dst, word, cx-24-f.Width(word, 1), ry, 1, pal.White)
		f.Draw(dst, "=", cx-4, ry, 1, pal.Ash)
		f.DrawShadow(dst, meaning, cx+26, ry, 1, col)
	}
}

// drawWheels draws the tumbler lock: a column per wheel with the letter it
// is on large in the middle, and the letters before and after it above and
// below. Fixed plates show their text. The chosen wheel is outlined.
func (c *Crawl) drawWheels(dst *ebiten.Image, ctx *game.Context, cx, y int) {
	const gap, h = 4, 64
	lp, f := c.puzzle, ctx.Font
	opts := lp.p.(puzzle.Chooser).Options()
	width := func(o []string) int {
		if len(o) == 1 {
			return f.Width(o[0], 2) + 4
		}
		return 28
	}
	total := -gap
	for _, o := range opts {
		total += width(o) + gap
	}
	x := cx - total/2
	for i, o := range opts {
		bw := width(o)
		if len(o) == 1 {
			f.DrawShadow(dst, o[0], x+2, y+h/2-16, 2, pal.Tan)
			x += bw + gap
			continue
		}
		edge := pal.Stone
		if i == lp.pick && !c.muted {
			edge = pal.Yellow
		}
		gfx.FillRect(dst, x, y, bw, h, edge)
		gfx.FillRect(dst, x+2, y+2, bw-4, h-4, pal.Night)
		gfx.FillRect(dst, x+2, y+h/2-17, bw-4, 34, pal.Granite)
		n, k := len(o), lp.choice[i]
		f.DrawCentered(dst, o[(k+n-1)%n], x+bw/2, y+2, 1, pal.Slate)
		f.DrawCentered(dst, o[k], x+bw/2, y+h/2-16, 2, pal.White)
		f.DrawCentered(dst, o[(k+1)%n], x+bw/2, y+h-18, 1, pal.Slate)
		x += bw + gap
	}
}

// drawClues lists the crossword's words, with what has been typed for
// each. The chosen word is marked.
func (c *Crawl) drawClues(dst *ebiten.Image, ctx *game.Context, x, y int) {
	lp, f := c.puzzle, ctx.Font
	_, _, placed := lp.p.(puzzle.Crossworder).Layout()
	for i, pl := range placed {
		ry := y + i*18
		dir, col := "Across", pal.Ice
		if pl.Down {
			dir = "Down"
		}
		if i == lp.pick {
			f.Draw(dst, "▶", x, ry, 1, pal.Yellow)
			col = pal.White
		}
		f.DrawShadow(dst, fmt.Sprintf("%s: %s (%d letters)", dir, pl.Clue, pl.Len), x+16, ry, 1, col)
		text := lp.fields[i].Text()
		f.DrawShadow(dst, text, x+340, ry, 1, pal.Ice)
		if i == lp.pick && !c.muted && ctx.Tick/16%2 == 0 {
			gfx.FillRect(dst, x+340+f.Width(text, 1)+1, ry+1, 3, 14, pal.Yellow)
		}
	}
}

// drawCrossword draws the crossword grid over the 3D view, filled with
// what has been typed. The chosen word's cells are outlined.
func (c *Crawl) drawCrossword(view *ebiten.Image, ctx *game.Context) {
	const cell = 22
	lp, f := c.puzzle, ctx.Font
	w, h, placed := lp.p.(puzzle.Crossworder).Layout()
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	gx, gy := viewX+(vw-w*cell)/2, viewY+(vh-22-h*cell)/2
	gfx.FillRect(view, gx-8, gy-8, w*cell+16, h*cell+16, pal.Fade(pal.Black, 0.7))

	// Each cell shows the chosen word's letter if it has one there, or
	// else another word's.
	type square struct {
		letter string
		mine   bool
	}
	grid := map[[2]int]*square{}
	for i, pl := range placed {
		typed := []rune(lp.fields[i].Text())
		for k := 0; k < pl.Len; k++ {
			x, y := pl.Cell(k)
			sq := grid[[2]int{x, y}]
			if sq == nil {
				sq = &square{}
				grid[[2]int{x, y}] = sq
			}
			if k < len(typed) && (sq.letter == "" || i == lp.pick) {
				sq.letter = string(typed[k])
			}
			sq.mine = sq.mine || i == lp.pick
		}
	}
	for at, sq := range grid {
		x, y := gx+at[0]*cell, gy+at[1]*cell
		edge := pal.Stone
		if sq.mine && !lp.showing {
			edge = pal.Yellow
		}
		gfx.FillRect(view, x, y, cell, cell, edge)
		gfx.FillRect(view, x+2, y+2, cell-4, cell-4, pal.Night)
		f.DrawCentered(view, sq.letter, x+cell/2, y+3, 1, pal.White)
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
	case lp.p.Answer() == puzzle.Foreign || lp.p.Answer() == puzzle.Native:
		return "Enter check · F4 hint · Esc leave"
	}
	switch lp.p.Answer() {
	case puzzle.Pick:
		return "1-4 or ←/→ and Enter pick · Esc leave"
	case puzzle.Match:
		return "↑↓ word · ←→ swap meaning · Enter check"
	case puzzle.Dial:
		return "←→ wheel · ↑↓ turn · Enter check · Esc leave"
	case puzzle.Grid:
		return "↑↓ word · Enter next word or check · Esc leave"
	}
	return "Enter check · Esc leave"
}
