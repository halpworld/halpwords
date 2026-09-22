package scene

import (
	"fmt"
	"image/color"
	"math"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/combat"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/raycast"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/internal/words"
)

// phase is a step in a battle.
type phase int

const (
	phaseAttack phase = iota // the hero types a word to strike
	phaseDefend              // the hero types a word before the monster hits
	phaseResult              // showing how the last answer went
	phaseWon
	phaseLost
)

// dyingTicks is how long a defeated monster takes to shrink away.
const dyingTicks = 30

// battle is a fight with one monster, shown in the dungeon view.
type battle struct {
	m      *dungeon.Monster
	phase  phase
	next   phase // where phaseResult goes when it ends
	field  *typing.Field
	word   words.Entry
	wordID int
	start  uint64  // tick the word appeared
	limit  float64 // seconds to defend

	title    string
	titleCol color.RGBA
	lines    []logLine
	shown    uint64 // tick the result appeared
	wait     bool   // the result waits for Enter, so a mistake can be studied

	flash, lunge, dying int // monster animation ticks
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// startBattle fights m. After an ambush the monster strikes first.
func (c *Crawl) startBattle(ctx *game.Context, m *dungeon.Monster, ambush bool) {
	m.Awake = true
	c.mode = modeBattle
	c.muted = true
	c.queued = actNone
	f := typing.NewField(c.run.lang)
	f.Greek = c.run.greek
	c.battle = &battle{m: m, field: f}
	if ambush {
		c.run.say(fmt.Sprintf("A %s attacks!", m.Name()), pal.Orange)
		c.beginDefend(ctx)
	} else {
		c.run.say(fmt.Sprintf("You fight the %s!", m.Name()), pal.Yellow)
		c.beginAttack(ctx)
	}
}

// deal puts a new word in front of the hero.
func (c *Crawl) deal(ctx *game.Context, p phase) {
	b := c.battle
	b.word, b.wordID = c.run.deck.Next()
	b.field.Reset()
	b.phase, b.start = p, ctx.Tick
	b.title, b.lines = "", nil
}

func (c *Crawl) beginAttack(ctx *game.Context) { c.deal(ctx, phaseAttack) }

func (c *Crawl) beginDefend(ctx *game.Context) {
	c.deal(ctx, phaseDefend)
	b := c.battle
	b.limit = combat.DefendTime(runes(b.word.Answers[0]))
}

func (c *Crawl) updateBattle(ctx *game.Context) {
	b := c.battle
	if b.flash > 0 {
		b.flash--
	}
	if b.lunge > 0 {
		b.lunge--
	}
	if b.dying > 0 && b.dying < dyingTicks {
		b.dying++
	}
	switch b.phase {
	case phaseAttack:
		switch {
		case input.Back():
			c.flee(ctx)
			return
		case input.Pressed(ebiten.KeyF1):
			if c.drinkPotion() {
				c.beginDefend(ctx) // drinking takes a turn
			}
			return
		}
		if c.muted {
			return
		}
		if typeInto(ctx, b.field) {
			c.strike(ctx)
		}
		c.run.greek = b.field.Greek
	case phaseDefend:
		if c.muted {
			return
		}
		if secs(ctx.Tick-b.start) >= b.limit {
			c.dodge(ctx, words.Result{Tier: words.Miss, Expected: b.word.Answers[0]}, "", true)
			return
		}
		if typeInto(ctx, b.field) {
			typed := b.field.Text()
			c.dodge(ctx, c.grade(typed), typed, false)
		}
		c.run.greek = b.field.Greek
	case phaseResult:
		t := secs(ctx.Tick - b.shown)
		if t >= 0.35 && (input.Confirm() || (!b.wait && t >= 1.4)) {
			switch b.next {
			case phaseAttack:
				c.beginAttack(ctx)
			case phaseDefend:
				c.beginDefend(ctx)
			case phaseLost:
				c.die()
			}
		}
	case phaseWon:
		if secs(ctx.Tick-b.shown) >= 0.35 && input.Confirm() {
			c.level.Remove(b.m)
			c.battle = nil
			c.mode = modeExplore
		}
	}
}

func (c *Crawl) grade(typed string) words.Result {
	l := c.run.lang
	return words.Grade(typed, c.battle.word, l, l.Defaults, c.battle.field.UsedBackspace)
}

// scoreAnswer records an answer for spaced practice and the combo streak.
// An accent slip neither builds nor breaks the streak.
func (c *Crawl) scoreAnswer(id int, t words.Tier) {
	c.run.deck.Mark(id, t >= words.Correct)
	h := &c.run.hero
	switch {
	case t >= words.Correct:
		h.Streak++
	case t < words.AccentSlip:
		h.Streak = 0
	}
}

// answerLines explains an answer: the right spelling, and what was typed if
// it was wrong.
func answerLines(e words.Entry, res words.Result, typed string) []logLine {
	lines := []logLine{{e.Prompt + " = " + res.Expected, pal.White}}
	switch {
	case res.Tier == words.AccentSlip:
		lines = append(lines, logLine{"You typed " + typed + ". Watch the accents!", pal.Cyan})
	case res.Tier < words.AccentSlip && typed != "":
		lines = append(lines, logLine{"You typed " + typed + ".", pal.Steel})
	}
	return lines
}

func (c *Crawl) showResult(ctx *game.Context, title string, col color.RGBA, lines []logLine, wait bool, next phase) {
	b := c.battle
	b.title, b.titleCol, b.lines = title, col, lines
	b.wait, b.next = wait, next
	b.phase, b.shown = phaseResult, ctx.Tick
}

// strike grades an attack. Fast, correct spelling hits hardest; a miss
// fumbles.
func (c *Crawl) strike(ctx *game.Context) {
	b, h, m := c.battle, &c.run.hero, c.battle.m
	typed := b.field.Text()
	res := c.grade(typed)
	speed := combat.Speed(runes(res.Expected), secs(ctx.Tick-b.start))
	dmg, crit := combat.Damage(h.ATK, res.Tier, speed, h.Streak)
	combo := combat.Combo(h.Streak)
	c.scoreAnswer(b.wordID, res.Tier)
	lines := answerLines(b.word, res, typed)

	title, col := res.Tier.String()+"!", tierColor[res.Tier]
	if res.Tier == words.Miss || dmg == 0 {
		title = "MISS!"
		h.HP--
		c.hurt = 12
		c.float("-1", pal.Rose)
		lines = append(lines, logLine{"You fumble and nick yourself: 1 damage.", pal.Rose})
		c.run.say(fmt.Sprintf("You miss the %s.", m.Name()), pal.Rose)
	} else {
		m.HP -= dmg
		b.flash = 10
		if crit {
			title, col = "CRITICAL!", pal.Yellow
		}
		c.float(fmt.Sprint(dmg), col)
		info := fmt.Sprintf("%d damage   speed ×%.1f", dmg, speed)
		if combo > 1 {
			info += fmt.Sprintf("   combo ×%.1f", combo)
		}
		lines = append(lines, logLine{info, pal.Ice})
		c.run.say(fmt.Sprintf("You hit the %s for %d.", m.Name(), dmg), pal.Ice)
	}

	switch {
	case m.HP <= 0:
		c.win(ctx, title, col, lines)
	case h.HP <= 0:
		c.showResult(ctx, title, col, lines, true, phaseLost)
	default:
		c.showResult(ctx, title, col, lines, res.Tier < words.Correct, phaseDefend)
	}
}

// dodge grades a defence. A good answer dodges; a graze halves the blow.
func (c *Crawl) dodge(ctx *game.Context, res words.Result, typed string, timeout bool) {
	b, h, m := c.battle, &c.run.hero, c.battle.m
	c.scoreAnswer(b.wordID, res.Tier)
	lines := answerLines(b.word, res, typed)
	hit := max(1, m.ATK+c.run.rng.IntN(3)-1)
	dmg := int(math.Round(float64(hit) * combat.Block(res.Tier)))
	b.lunge = 16

	var title string
	var col color.RGBA
	switch {
	case dmg == 0:
		title, col = "DODGED!", pal.Lime
		lines = append(lines, logLine{fmt.Sprintf("You dodge the %s.", m.Name()), pal.Lime})
	case timeout:
		title, col = "TOO SLOW!", pal.Rose
	default:
		title, col = "HIT!", pal.Orange
	}
	if dmg > 0 {
		h.HP -= dmg
		c.shake, c.hurt = 12, 12
		c.float(fmt.Sprint(-dmg), pal.Rose)
		msg := fmt.Sprintf("The %s hits you: %d damage.", m.Name(), dmg)
		if dmg < hit {
			msg = fmt.Sprintf("You half dodge the %s: %d damage.", m.Name(), dmg)
		}
		lines = append(lines, logLine{msg, pal.Rose})
		c.run.say(msg, pal.Rose)
	}
	next := phaseAttack
	if h.HP <= 0 {
		next = phaseLost
	}
	c.showResult(ctx, title, col, lines, res.Tier < words.Correct, next)
}

// win hands out the rewards for defeating the monster.
func (c *Crawl) win(ctx *game.Context, title string, col color.RGBA, lines []logLine) {
	b, h, m := c.battle, &c.run.hero, c.battle.m
	xp, gold := m.XP(), m.Gold()
	h.Gold += gold
	lines = append(lines, logLine{fmt.Sprintf("The %s is defeated! +%d XP, +%d gold.", m.Name(), xp, gold), pal.Yellow})
	c.run.say(fmt.Sprintf("You defeat the %s. +%d XP, +%d gold.", m.Name(), xp, gold), pal.Yellow)
	if n := h.GainXP(xp); n > 0 {
		lines = append(lines, logLine{fmt.Sprintf("LEVEL UP! You are level %d.", h.Level), pal.Lime})
		c.run.say(fmt.Sprintf("Level up! You are now level %d. HP %d, ATK %d.", h.Level, h.MaxHP, h.ATK), pal.Lime)
		c.showBanner("LEVEL UP!", fmt.Sprintf("Level %d", h.Level))
	}
	b.title, b.titleCol, b.lines = title, col, lines
	b.phase, b.shown, b.dying = phaseWon, ctx.Tick, 1
}

// flee tries to escape. It works half the time, and the monster is left
// dazed for a few turns; otherwise it gets a free attack.
func (c *Crawl) flee(ctx *game.Context) {
	m := c.battle.m
	if c.run.rng.IntN(2) == 0 {
		m.Stun = 4
		c.run.say(fmt.Sprintf("You slip away from the %s!", m.Name()), pal.Ice)
		c.battle = nil
		c.mode = modeExplore
		c.run.hero.Streak = 0
		return
	}
	c.run.say("You can't get away!", pal.Orange)
	c.beginDefend(ctx)
	c.battle.title, c.battle.titleCol = "Can't escape!", pal.Orange
}

// dress animates the monster being fought: it flashes when hit, lunges when
// it attacks and shrinks away when defeated.
func (b *battle) dress(s *raycast.Sprite, c *Crawl) {
	if b.flash > 0 && b.flash/2%2 == 0 {
		s.Flash = true
	}
	if b.lunge > 0 {
		f := math.Sin(math.Pi*float64(b.lunge)/16) * 0.35
		hx, hy := center(c.pos)
		s.X += (hx - s.X) * f
		s.Y += (hy - s.Y) * f
	}
	if b.dying > 0 {
		k := 1 - float64(b.dying)/dyingTicks
		s.Size *= max(0.02, k)
		s.Flash = b.dying%4 < 2
	}
}

// drawBattlePanel draws the word to type and the result of the last answer.
func (c *Crawl) drawBattlePanel(dst *ebiten.Image, ctx *game.Context, x, y, w int) {
	b, f := c.battle, ctx.Font
	cx := x + w/2
	switch b.phase {
	case phaseAttack, phaseDefend:
		t := secs(ctx.Tick - b.start)
		if c.muted {
			t = 0
		}
		label, lcol := "ATTACK! Translate into "+c.run.lang.Name+":", pal.Yellow
		if b.phase == phaseDefend {
			label, lcol = "DODGE! Translate before it strikes:", pal.Orange
		}
		if b.title != "" {
			label = b.title + "  " + label
		}
		f.DrawShadow(dst, label, x+12, y+10, 1, lcol)
		const bw = 180
		bx := x + w - bw - 14
		if b.phase == phaseAttack {
			speed := combat.Speed(runes(b.word.Answers[0]), t)
			bc := pal.Lime
			switch {
			case speed < 1:
				bc = pal.Orange
			case speed < 1.5:
				bc = pal.Yellow
			}
			f.DrawShadow(dst, "speed", bx-44, y+10, 1, pal.Steel)
			bar(dst, bx, y+13, bw, 10, (speed-0.5)/1.5, bc, pal.Night)
		} else {
			left := max(0, b.limit-t)
			bc := pal.Lime
			if left < b.limit/3 {
				bc = pal.Rose
			}
			f.DrawShadow(dst, fmt.Sprintf("%.1fs", left), bx-44, y+10, 1, pal.Steel)
			bar(dst, bx, y+13, bw, 10, left/b.limit, bc, pal.Night)
		}
		sc := f.FitScale(b.word.Prompt, w-40, 2)
		f.DrawCentered(dst, b.word.Prompt, cx, y+28, sc, pal.White)
		drawTyped(dst, ctx, b.field.Text(), cx, y+60, min(560, w-40), 2, !c.muted)
	default:
		drawResult(dst, ctx, b.title, b.titleCol, b.lines, cx, y)
	}
}

// battleHelp is the key help under the view during a battle.
func (c *Crawl) battleHelp() string {
	b := c.battle
	switch b.phase {
	case phaseAttack:
		return "Enter strike · F1 potion · Esc flee"
	case phaseDefend:
		return "Type fast to dodge!"
	case phaseResult:
		if b.wait || b.next == phaseLost {
			return "Enter continue"
		}
		return ""
	case phaseWon:
		return "Enter continue"
	}
	return ""
}

// drawResult draws a result title with its explanation lines under it. With
// many lines the title shrinks to make room.
func drawResult(dst *ebiten.Image, ctx *game.Context, title string, col color.RGBA, lines []logLine, cx, y int) {
	f := ctx.Font
	ly := y + 44
	if len(lines) > 3 {
		f.DrawCentered(dst, title, cx, y+8, 1, col)
		ly = y + 28
		lines = lines[:min(len(lines), 4)]
	} else {
		f.DrawCentered(dst, title, cx, y+8, 2, col)
	}
	for i, l := range lines {
		f.DrawCentered(dst, l.text, cx, ly+i*16, 1, l.col)
	}
}
