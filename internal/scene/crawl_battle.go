package scene

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/combat"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/llm"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/raycast"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/pkg/words"
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
	warned bool    // the dodge timer has sounded its warning

	title    string
	titleCol color.RGBA
	lines    []logLine
	shown    uint64 // tick the result appeared
	wait     bool   // the result waits for Enter, so a mistake can be studied

	flash, lunge, dying int // monster animation ticks

	// taunt is what the monster shouted as the battle began, shown for
	// tauntT more ticks.
	taunt  llm.Taunt
	tauntT int

	slow    bool // an Hourglass gives half as much time again
	clarity bool // a Rune of Clarity shows typos as they are typed
	hints   int  // letters shown by hints for this word
}

// tauntTicks is how long a monster's taunt stays over the view.
const tauntTicks = 240

// hourglassTime is how much longer an Hourglass gives to type.
const hourglassTime = 1.5

// timeScale is how much longer than usual the hero has to type an attack.
func (c *Crawl) timeScale() float64 {
	k := c.run.hero.TimeBonus() * c.run.settings.Timer.Scale()
	if c.battle != nil && c.battle.slow {
		k *= hourglassTime
	}
	return k
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
	h := &c.run.hero
	c.battle = &battle{m: m, field: f, slow: h.Hourglass, clarity: h.Clarity}
	if h.Hourglass {
		c.run.say("Your hourglass turns. Time slows around you.", pal.Cyan)
	}
	if h.Clarity {
		c.run.say("Your Rune of Clarity glows.", pal.Cyan)
	}
	h.Hourglass, h.Clarity = false, false
	c.play(audio.Alert)
	first := phaseAttack
	if m.Kind.Boss() {
		c.play(audio.Rage)
		c.showBanner(m.Name(), "guards the stairs!")
	}
	if ambush {
		c.run.say(fmt.Sprintf("A %s attacks!", m.Name()), pal.Orange)
		first = phaseDefend
	} else {
		c.run.say(fmt.Sprintf("You fight the %s!", m.Name()), pal.Yellow)
	}
	if t, ok := c.run.ai.taunt(c.run); ok {
		c.battle.taunt, c.battle.tauntT = t, tauntTicks
		c.run.say(fmt.Sprintf("The %s shouts: “%s” (%s)", m.Name(), t.Text, t.English), pal.Pink)
	}
	// The first time the hero meets a trait, explain it before the fight.
	if fresh := m.Traits &^ c.run.seenTraits; fresh != 0 {
		c.run.seenTraits |= m.Traits
		var lines []logLine
		for _, t := range dungeon.Traits {
			if fresh&t != 0 {
				lines = append(lines, logLine{t.String() + ": " + t.Hint(), pal.Ice})
			}
		}
		c.showResult(ctx, fmt.Sprintf("The %s is %s!", m.Kind.Name, traitWords(fresh)), pal.Cyan, lines, true, first)
		return
	}
	if first == phaseDefend {
		c.beginDefend(ctx)
	} else {
		c.beginAttack(ctx)
	}
}

// traitWords describes traits in a sentence, such as "armored and swift".
func traitWords(t dungeon.Trait) string {
	s := strings.ToLower(t.String())
	if i := strings.LastIndex(s, ", "); i >= 0 {
		s = s[:i] + " and " + s[i+2:]
	}
	return s
}

// deal puts a new word in front of the hero.
func (c *Crawl) deal(ctx *game.Context, p phase) {
	b := c.battle
	ok := false
	target := combat.WordTarget(c.level.Depth, b.m.Kind.Boss())
	if b.m.Phase > 0 {
		// An angry boss asks for longer words.
		b.word, b.wordID, ok = c.run.deck.NextNear(target, func(e words.Entry) bool { return runes(e.Answers[0]) >= 6 })
	}
	if !ok {
		b.word, b.wordID, _ = c.run.deck.NextNear(target, nil)
	}
	b.hints = 0
	b.field.Reset()
	b.phase, b.start = p, ctx.Tick
	b.title, b.lines = "", nil
	b.warned = false
}

func (c *Crawl) beginAttack(ctx *game.Context) { c.deal(ctx, phaseAttack) }

func (c *Crawl) beginDefend(ctx *game.Context) {
	c.deal(ctx, phaseDefend)
	b := c.battle
	b.limit = combat.DefendTime(runes(b.word.Answers[0])) * c.run.hero.DodgeBonus() * c.run.settings.Timer.Scale()
	if b.slow {
		b.limit *= hourglassTime
	}
	if b.m.Has(dungeon.Swift) {
		b.limit *= combat.SwiftTime
	}
}

func (c *Crawl) updateBattle(ctx *game.Context) {
	b := c.battle
	if input.Back() {
		c.pause(ctx)
		return
	}
	if b.flash > 0 {
		b.flash--
	}
	if b.lunge > 0 {
		b.lunge--
	}
	if b.dying > 0 && b.dying < dyingTicks {
		b.dying++
	}
	if b.tauntT > 0 {
		b.tauntT--
	}
	if (b.phase == phaseAttack || b.phase == phaseDefend) && !c.muted && input.Pressed(ebiten.KeyF4) {
		c.hint(&b.hints, b.word.Answers[0])
	}
	switch b.phase {
	case phaseAttack:
		if input.Pressed(ebiten.KeyF1) {
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
		left := b.limit - secs(ctx.Tick-b.start)
		if !b.warned && left < b.limit/3 {
			b.warned = true
			c.play(audio.Warn)
		}
		if left <= 0 {
			c.dodge(ctx, words.Result{Tier: words.Miss, Expected: b.word.Answers[0]}, "", b.limit)
			return
		}
		if typeInto(ctx, b.field) {
			typed := b.field.Text()
			c.dodge(ctx, c.grade(typed), typed, secs(ctx.Tick-b.start))
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
	return words.Grade(typed, c.battle.word, c.run.lang, c.run.rules(), c.battle.field.UsedBackspace)
}

// scoreAnswer records an answer for spaced practice, the combo streak and
// the score. An accent slip neither builds nor breaks the streak. An id
// below 0 is an answer that was not about one word, such as an odd-one-out
// pick. A word answered with a hint comes back for practice, however it
// was spelled. secs is how long a timed answer took, or 0. The first
// perfect spelling of each word is worth XP.
func (c *Crawl) scoreAnswer(id int, res words.Result, typed string, hinted bool, secs float64) {
	r, h := c.run, &c.run.hero
	t := res.Tier
	if id >= 0 {
		a := words.Answer{Tier: t, Hinted: hinted, Secs: secs, Timed: secs > 0}
		if t < words.Correct && typed != "" {
			a.Mistake = words.Classify(typed, res.Expected, r.lang)
		}
		r.deck.Answer(id, a)
		r.remember()
		mode := r.linkMode()
		if c.kind != "" {
			mode += ":" + c.kind
		}
		r.link.Answer(r.lang.Code, r.deck.Entries()[id], mode, a)
	}
	switch {
	case t >= words.Correct:
		h.Streak++
		r.tally.BestCombo = max(r.tally.BestCombo, h.Streak)
	case t < words.AccentSlip:
		h.Streak = 0
		r.ai.missed(id)
	}
	if t == words.Miss {
		r.tally.Misses++
	}
	if t == words.Perfect && id >= 0 && !hinted {
		r.tally.Perfect++
	}
	if t == words.Perfect && id >= 0 && !hinted && !r.perfect[id] {
		r.perfect[id] = true
		e := r.deck.Entries()[id]
		r.say(fmt.Sprintf("First perfect %s! +%d XP", e.Answers[0], rpg.PerfectXP), pal.Lime)
		c.gainXP(rpg.PerfectXP)
	}
}

// gainXP gives the hero XP and celebrates any level gained. It returns
// lines describing the level ups.
func (c *Crawl) gainXP(xp int) []logLine {
	h := &c.run.hero
	var lines []logLine
	for _, up := range h.GainXP(xp) {
		lines = append(lines, logLine{fmt.Sprintf("LEVEL UP! Level %d: %s", up.Level, up), pal.Lime})
		c.run.say(fmt.Sprintf("Level up! You are now level %d. %s.", up.Level, up), pal.Lime)
		c.showBanner("LEVEL UP!", fmt.Sprintf("Level %d", up.Level))
		c.run.sound.PlayLater(audio.LevelUp, 45)
		c.fxLevelUp()
	}
	return lines
}

// hint shows one more letter of answer, for MP or a Hint Scroll. shown
// counts the letters shown so far.
func (c *Crawl) hint(shown *int, answer string) {
	h := &c.run.hero
	if *shown >= letters(answer) {
		c.run.info("The whole word is showing!")
		return
	}
	switch cost := h.HintCost(); {
	case h.MP >= cost:
		h.MP -= cost
		c.run.info(fmt.Sprintf("A hint shows a letter. -%d MP", cost))
	case h.Items[rpg.HintScroll] > 0:
		h.Items[rpg.HintScroll]--
		c.run.info("You read a Hint Scroll. It shows a letter.")
	default:
		c.play(audio.Bump)
		c.run.info(fmt.Sprintf("A hint needs %d MP or a Hint Scroll.", cost))
		return
	}
	*shown++
	c.play(audio.Hint)
}

// letters counts the letters in s.
func letters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

// hintText shows the first n letters of answer, with the rest as blanks.
// Spaces and punctuation always show.
func hintText(answer string, n int) string {
	var b strings.Builder
	for _, r := range answer {
		switch {
		case !unicode.IsLetter(r):
			b.WriteRune(r)
		case n > 0:
			b.WriteRune(r)
			n--
		default:
			b.WriteRune('_')
		}
		b.WriteRune(' ')
	}
	return strings.TrimSpace(b.String())
}

// answerLines explains an answer: the right spelling, and what was typed if
// it was wrong, with a tip about the mistake.
func answerLines(e words.Entry, res words.Result, typed string, lang *words.Language) []logLine {
	lines := []logLine{{e.Prompt + " = " + res.Expected, pal.White}}
	if res.Tier < words.Correct && typed != "" {
		lines = append(lines, mistakeLine("You typed "+typed+".", typed, res, lang))
	}
	return lines
}

// mistakeLine is what the hero typed, with a tip about their mistake.
func mistakeLine(said, typed string, res words.Result, lang *words.Language) logLine {
	col := pal.Steel
	if res.Tier == words.AccentSlip {
		col = pal.Cyan
	}
	if tip := words.Classify(typed, res.Expected, lang).Tip(); tip != "" {
		said += " " + tip
	}
	return logLine{said, col}
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
	speed := combat.Speed(runes(res.Expected), secs(ctx.Tick-b.start)/c.timeScale())
	hinted := b.hints > 0
	dmg, crit := combat.Damage(h.ATK(), res.Tier, h.SpeedDamage(speed), h.Streak)
	lucky := false
	if !crit && !hinted && res.Tier >= words.Correct && c.run.rng.Float64() < h.LuckyCrit() {
		crit, lucky = true, true
		dmg = int(float64(dmg)*1.5 + 0.5)
	}
	if crit && hinted {
		crit = false
		dmg = int(float64(dmg)/1.5 + 0.5)
	}
	combo := combat.Combo(h.Streak)
	c.kind = "attack"
	c.scoreAnswer(b.wordID, res, typed, hinted, secs(ctx.Tick-b.start))
	lines := answerLines(b.word, res, typed, c.run.lang)
	if res.Tier == words.Perfect && !hinted && h.Restore(1) > 0 {
		lines = append(lines, logLine{"Perfect spelling restores 1 MP.", pal.Sky})
	}

	title, col := res.Tier.String()+"!", tierColor[res.Tier]
	switch {
	case res.Tier > words.Miss && m.Has(dungeon.Armored) && combat.ArmorBlocks(res.Tier):
		title, col = "CLANG!", pal.Steel
		b.flash = 4
		c.play(audio.Clang)
		c.fxClang()
		lines = append(lines, logLine{"Its armor turns the blow. Only exact spelling gets through!", pal.Steel})
		c.run.say(fmt.Sprintf("Your blow glances off the %s's armor.", m.Name()), pal.Steel)
	case res.Tier == words.Miss || dmg == 0:
		title = "MISS!"
		c.play(audio.Fumble)
		h.HP--
		c.hurt = 12
		c.float("-1", pal.Rose)
		lines = append(lines, logLine{"You fumble and nick yourself: 1 damage.", pal.Rose})
		c.run.say(fmt.Sprintf("You miss the %s.", m.Name()), pal.Rose)
	default:
		c.run.tally.Damage += min(dmg, m.HP)
		m.HP -= dmg
		b.flash = 10
		switch {
		case crit:
			title, col = "CRITICAL!", pal.Yellow
			if lucky {
				title = "LUCKY HIT!"
			}
			c.play(audio.Crit)
		case res.Tier >= words.Correct:
			c.play(audio.Hit)
		default:
			c.play(audio.Weak)
		}
		c.fxHit(crit, res.Tier < words.Correct)
		c.floats = append(c.floats, floater{text: fmt.Sprint(dmg), col: col, big: crit})
		info := fmt.Sprintf("%d damage   speed ×%.1f", dmg, speed)
		if combo > 1 {
			info += fmt.Sprintf("   combo ×%.1f", combo)
		}
		lines = append(lines, logLine{info, pal.Ice})
		c.run.say(fmt.Sprintf("You hit the %s for %d.", m.Name(), dmg), pal.Ice)
		lines = append(lines, c.enrage(m)...)
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

// dodge grades a defence typed in taken seconds. A good answer dodges; a
// graze halves the blow. Nothing typed means time ran out.
func (c *Crawl) dodge(ctx *game.Context, res words.Result, typed string, taken float64) {
	b, h, m := c.battle, &c.run.hero, c.battle.m
	timeout := typed == ""
	c.kind = "dodge"
	c.scoreAnswer(b.wordID, res, typed, b.hints > 0, taken)
	lines := answerLines(b.word, res, typed, c.run.lang)
	hit := h.Hit(max(1, m.ATK+c.run.rng.IntN(3)-1))
	dmg := int(math.Round(float64(hit) * combat.Block(res.Tier)))
	b.lunge = 16

	var title string
	var col color.RGBA
	switch {
	case dmg == 0:
		title, col = "DODGED!", pal.Lime
		c.play(audio.Dodge)
		lines = append(lines, logLine{fmt.Sprintf("You dodge the %s.", m.Name()), pal.Lime})
	case timeout:
		title, col = "TOO SLOW!", pal.Rose
	default:
		title, col = "HIT!", pal.Orange
	}
	if dmg > 0 {
		h.HP -= dmg
		c.play(audio.Hurt)
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
	xp, gold := m.XP(), h.GoldFind(m.Gold(), false)
	h.Gold += gold
	c.run.sound.PlayLater(audio.Defeat, 12)
	c.fxDefeat()
	c.fxCoins(gold)
	lines = append(lines, logLine{fmt.Sprintf("The %s is defeated! +%d XP, +%d gold.", m.Name(), xp, gold), pal.Yellow})
	c.run.say(fmt.Sprintf("You defeat the %s. +%d XP, +%d gold.", m.Name(), xp, gold), pal.Yellow)
	if rr := c.run.race; rr != nil {
		rr.monsters++
	}
	loot := m.Loot
	switch {
	case m.Kind.Boss():
		c.run.tally.Bosses++
	case m.Kind.Family == dungeon.Mimic:
		c.run.tally.Chests++ // the chest it was guarding
	}
	if m.Kind.Boss() {
		// Bosses always drop gear, a tier better than the floor's.
		g := rpg.RandomGear(c.level.Depth+2, c.run.rng)
		loot = &dungeon.Chest{Gear: &g, Potions: 1}
		c.showBanner("VICTORY!", "The stairs are free")
		c.run.say("The way down is open!", pal.Lime)
	}
	if loot != nil {
		lines = append(lines, c.takeLoot(loot, "It was guarding ")...)
	}
	lines = append(lines, c.gainXP(xp)...)
	b.title, b.titleCol, b.lines = title, col, lines
	b.phase, b.shown, b.dying = phaseWon, ctx.Tick, 1
}

// enrage moves a boss into its next phase as it weakens: at two thirds of
// its HP it turns swift, and at one third its words turn backwards too.
func (c *Crawl) enrage(m *dungeon.Monster) []logLine {
	if !m.Kind.Boss() || m.HP <= 0 {
		return nil
	}
	phase := combat.BossPhase(m.HP, m.MaxHP)
	var lines []logLine
	for m.Phase < phase {
		m.Phase++
		t := dungeon.Swift
		if m.Phase == 2 {
			t = dungeon.Mirrored
			if m.Has(t) {
				t = dungeon.Ghostly
			}
		}
		m.Traits |= t
		msg := fmt.Sprintf("The %s is enraged! It turns %s.", m.Name(), strings.ToLower(t.String()))
		lines = append(lines, logLine{msg, pal.Orange})
		c.run.say(msg, pal.Orange)
		c.play(audio.Rage)
		c.shake = 16
	}
	return lines
}

// flee tries to escape. It works half the time, and the monster is left
// dazed for a few turns; otherwise it gets a free attack.
func (c *Crawl) flee(ctx *game.Context) {
	m := c.battle.m
	if c.run.rng.IntN(2) == 0 {
		m.Stun = 4
		c.play(audio.Flee)
		c.run.say(fmt.Sprintf("You slip away from the %s!", m.Name()), pal.Ice)
		c.battle = nil
		c.mode = modeExplore
		c.run.hero.Streak = 0
		return
	}
	c.run.say("You can't get away!", pal.Orange)
	c.play(audio.Bump)
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
			// The bar drains at a steady pace until the speed bonus is gone.
			// Marks show where critical hits and then full damage run out.
			t /= c.timeScale()
			n := runes(b.word.Answers[0])
			speed := combat.Speed(n, t)
			bc := pal.Lime
			switch {
			case speed < 1:
				bc = pal.Orange
			case speed < 1.5:
				bc = pal.Yellow
			}
			window := combat.SpeedWindow(n)
			f.DrawShadow(dst, fmt.Sprintf("×%.1f", speed), bx-44, y+10, 1, pal.Steel)
			bar(dst, bx, y+13, bw, 10, 1-t/window, bc, pal.Night)
			for _, s := range []float64{1.5, 1} {
				mx := bx + 1 + int(float64(bw-2)*(1-combat.TargetTime(n)/s/window))
				gfx.FillRect(dst, mx, y+11, 1, 14, pal.White)
			}
		} else {
			left := max(0, b.limit-t)
			bc := pal.Lime
			if left < b.limit/3 {
				bc = pal.Rose
			}
			f.DrawShadow(dst, fmt.Sprintf("%.1fs", left), bx-44, y+10, 1, pal.Steel)
			bar(dst, bx, y+13, bw, 10, left/b.limit, bc, pal.Night)
		}
		prompt, pcol := b.word.Prompt, pal.White
		if b.m.Has(dungeon.Mirrored) {
			prompt = combat.Mirror(prompt)
		}
		if b.m.Has(dungeon.Ghostly) {
			if v := combat.Visibility(t); v > 0 {
				pcol = pal.Fade(pal.Ice, v)
			} else {
				prompt, pcol = "· · ·", pal.Ash
			}
		}
		sc := f.FitScale(prompt, w-40, 2)
		f.DrawCentered(dst, prompt, cx, y+28, sc, pcol)
		good := -1
		if b.clarity || c.run.settings.Highlight {
			good = goodPrefix(b.field.Text(), b.word.Answers, c.run.lang)
		}
		drawTypedMarked(dst, ctx, b.field.Text(), good, cx, y+60, min(560, w-40), 2, !c.muted)
	default:
		drawResult(dst, ctx, b.title, b.titleCol, b.lines, cx, y)
	}
}

// battleHelp is the key help under the view during a battle.
func (c *Crawl) battleHelp() string {
	b := c.battle
	switch b.phase {
	case phaseAttack:
		if c.muted {
			return "Let go of the movement keys to start"
		}
		return "Enter strike · F1 potion · F4 hint · Esc menu"
	case phaseDefend:
		if c.muted {
			return "Let go of the movement keys to start"
		}
		return "Type fast to dodge! · F4 hint · Esc pause"
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
