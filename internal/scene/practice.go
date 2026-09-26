package scene

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/combat"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/pkg/proc"
	"github.com/halpworld/halpwords/pkg/words"
)

// Practice is a stand-alone spelling drill. Words are dealt by spaced
// repetition, and every answer goes in the Grimoire.
type Practice struct {
	bg    *ebiten.Image
	rng   *rand.Rand
	langs []*words.Language // languages that have at least one list
	li    int
	// assign is the assignment quest being practised, and list its words;
	// otherwise every list of the language is practised.
	assign *link.Quest
	list   *words.List

	deck     *words.Deck
	settings profile.LangSettings
	// setBy says who set the assignment's settings, when they apply.
	setBy   string
	word    words.Entry
	cur     int
	field   *typing.Field
	started uint64 // tick the current word appeared

	showing bool // showing the result of the last answer
	result  words.Result
	mistake words.Mistake
	typed   string
	taken   float64
	streak  int
	best    int
}

// NewPractice creates the practice screen.
func NewPractice(ctx *game.Context) game.Scene {
	p := &Practice{bg: backdrop(7, 1.6), rng: proc.NewRand(ctx.Tick + 1)}
	for _, l := range words.Languages {
		if len(ctx.ListsFor(l.Code)) > 0 {
			p.langs = append(p.langs, l)
		}
	}
	p.setLanguage(ctx, 0)
	return p
}

// NewAssignPractice creates the practice screen for an assignment quest: it
// practises the assigned list l only.
func NewAssignPractice(ctx *game.Context, q link.Quest, l *words.List) game.Scene {
	lang, _ := words.Lookup(l.Language)
	p := &Practice{bg: backdrop(7, 1.6), rng: proc.NewRand(ctx.Tick + 1), langs: []*words.Language{lang}, assign: &q, list: l}
	p.setLanguage(ctx, 0)
	return p
}

func (p *Practice) lang() *words.Language { return p.langs[p.li] }

func (p *Practice) setLanguage(ctx *game.Context, i int) {
	p.li = (i + len(p.langs)) % len(p.langs)
	entries := entriesFor(ctx, p.lang())
	if p.list != nil {
		entries = p.list.Entries
	}
	p.deck = words.NewDeck(entries, p.rng)
	p.deck.SetMemory(ctx.Profile.MemoryFor(p.lang().Code))
	p.settings, p.setBy = ctx.SettingsFor(p.lang(), p.assign)
	p.field = typing.NewField(p.lang())
	p.streak = 0
	p.next(ctx)
}

func (p *Practice) next(ctx *game.Context) {
	p.word, p.cur = p.deck.Next()
	p.field.Reset()
	p.showing = false
	p.started = ctx.Tick
}

// Update implements game.Scene.
func (p *Practice) Update(ctx *game.Context) error {
	if p.deck != nil {
		ctx.Playing("practice", p.lang().Code, nil)
	}
	if input.Back() {
		ctx.Sound.Play(audio.Back)
		if p.assign != nil {
			ctx.EndSession()
			ctx.Link.SyncNow() // for the assignment's progress
			ctx.Replace(NewAssignments(ctx))
			return nil
		}
		ctx.Replace(NewTitle(ctx))
		return nil
	}
	if p.showing {
		if input.Confirm() {
			ctx.Sound.Play(audio.Blip)
			p.next(ctx)
		}
		return nil
	}
	switch {
	case p.assign != nil:
	case input.Pressed(ebiten.KeyArrowLeft):
		ctx.Sound.Play(audio.Blip)
		p.setLanguage(ctx, p.li-1)
		return nil
	case input.Pressed(ebiten.KeyArrowRight):
		ctx.Sound.Play(audio.Blip)
		p.setLanguage(ctx, p.li+1)
		return nil
	}
	if typeInto(ctx, p.field) {
		p.typed = p.field.Text()
		p.taken = float64(ctx.Tick-p.started) / float64(ebiten.TPS())
		p.result = words.Grade(p.typed, p.word, p.lang(), p.settings.Rules, p.field.UsedBackspace)
		p.mistake = words.NoMistake
		if p.result.Tier < words.Correct {
			p.mistake = words.Classify(p.typed, p.result.Expected, p.lang())
		}
		p.deck.Answer(p.cur, words.Answer{Tier: p.result.Tier, Timed: true, Secs: p.taken, Mistake: p.mistake})
		ctx.Profile.SaveMemory()
		ctx.Link.Answer(p.lang().Code, p.word, "practice", words.Answer{Tier: p.result.Tier, Timed: true, Secs: p.taken, Mistake: p.mistake})
		p.showing = true
		ctx.Sound.Play(tierSound[p.result.Tier])
		if p.result.Tier >= words.Correct {
			p.streak++
			p.best = max(p.best, p.streak)
		} else {
			p.streak = 0
		}
	}
	return nil
}

// tierSound is the jingle for each grade.
var tierSound = map[words.Tier]audio.ID{
	words.Perfect:    audio.Perfect,
	words.Correct:    audio.Correct,
	words.AccentSlip: audio.Slip,
	words.Graze:      audio.Graze,
	words.Miss:       audio.Wrong,
}

var tierColor = map[words.Tier]color.RGBA{
	words.Perfect:    pal.Lime,
	words.Correct:    pal.Green,
	words.AccentSlip: pal.Cyan,
	words.Graze:      pal.Orange,
	words.Miss:       pal.Rose,
}

// Draw implements game.Scene.
func (p *Practice) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, p.bg, 0, 0)

	// Header: language selector and streak.
	if q := p.assign; q != nil {
		head := "Assignment: " + assignName(*q)
		f.DrawCentered(dst, fit(f, head, 360, 2), cx, 12, 2, pal.Yellow)
		goal := q.GoalText()
		f.DrawShadow(dst, goal, game.ScreenW-12-f.Width(goal, 1), 12, 1, pal.Ice)
		prog := q.ProgressText()
		f.DrawShadow(dst, prog, game.ScreenW-12-f.Width(prog, 1), 30, 1, pal.Steel)
		if p.setBy != "" {
			f.DrawCentered(dst, fit(f, "Assignment settings. "+p.setBy, 200, 1), cx, 32, 1, pal.Tan)
		}
	} else {
		f.DrawCentered(dst, "◄ "+p.lang().Name+" ►", cx, 12, 2, pal.Yellow)
	}
	f.DrawShadow(dst, fmt.Sprintf("Streak %d", p.streak), 12, 12, 1, pal.Ice)
	f.DrawShadow(dst, fmt.Sprintf("Best %d", p.best), 12, 30, 1, pal.Steel)
	if len(ctx.ListErrors) > 0 {
		f.DrawShadow(dst, "Word list error: "+ctx.ListErrors[0], 12, game.ScreenH-38, 1, pal.Rose)
	}

	// Main window.
	const wx, wy, ww, wh = 40, 48, game.ScreenW - 80, 204
	gfx.Window(dst, wx, wy, ww, wh)
	e := p.word
	f.DrawCentered(dst, "Translate into "+p.lang().Name+":", cx, wy+14, 1, pal.Steel)
	box := "new word"
	if b := p.deck.Memory().Box(e); b == words.Boxes {
		box = "mastered"
	} else if b > 0 {
		box = fmt.Sprintf("box %d of %d", b, words.Boxes)
	}
	f.DrawShadow(dst, box, wx+ww-12-f.Width(box, 1), wy+8, 1, boxColors[p.deck.Memory().Box(e)])
	sc := f.FitScale(e.Prompt, ww-40, 3)
	f.DrawCentered(dst, e.Prompt, cx, wy+34, sc, pal.White)

	// Input line.
	text := p.field.Text()
	if p.showing {
		text = p.typed
	}
	good := -1
	if p.settings.Highlight && !p.showing {
		good = goodPrefix(text, e.Answers, p.lang())
	}
	drawTypedMarked(dst, ctx, text, good, cx, wy+88, ww-40, 3, !p.showing)

	if p.showing {
		r := p.result
		f.DrawCentered(dst, r.Tier.String()+"!", cx, wy+138, 2, tierColor[r.Tier])
		info := fmt.Sprintf("Answer: %s", r.Expected)
		if r.Tier >= words.Graze {
			speed := combat.Speed(utf8.RuneCountInString(r.Expected), p.taken)
			power := combat.Accuracy(r.Tier) * speed
			info += fmt.Sprintf("    %.1fs   speed ×%.1f   power %d%%", p.taken, speed, int(power*100))
		}
		f.DrawCentered(dst, info, cx, wy+186, 1, pal.Ice)
		if tip := p.mistake.Tip(); tip != "" && p.result.Tier < words.Correct {
			f.DrawCentered(dst, tip, cx, wy+168, 1, pal.Orange)
		}
	} else {
		secs := float64(ctx.Tick-p.started) / float64(ebiten.TPS())
		f.DrawCentered(dst, fmt.Sprintf("%.1fs", secs), cx, wy+160, 1, pal.Ash)
	}

	p.drawHelp(dst, ctx)
}

func (p *Practice) drawHelp(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	y := 262
	if p.showing {
		f.DrawCentered(dst, "Enter: next word    Esc: "+p.escTo(), game.ScreenW/2, y+40, 1, pal.Steel)
		return
	}
	if p.field.Greek {
		gfx.Window(dst, 40, y-6, game.ScreenW-80, 98)
		var row1, row2 strings.Builder
		for i, k := range typing.BetaCodeChart {
			b := &row1
			if i >= 12 {
				b = &row2
			}
			fmt.Fprintf(b, "%c %c  ", k.Key, k.Greek)
		}
		f.DrawCentered(dst, row1.String(), game.ScreenW/2, y+4, 1, pal.Ice)
		f.DrawCentered(dst, row2.String(), game.ScreenW/2, y+20, 1, pal.Ice)
		f.DrawCentered(dst, "after a vowel:  ) ἀ   ( ἁ   / ά   \\ ὰ   = ᾶ   | ᾳ   + ϊ", game.ScreenW/2, y+38, 1, pal.Tan)
		f.DrawCentered(dst, "Tab: cycle accent   F2: Greek keys off", game.ScreenW/2, y+54, 1, pal.Tan)
		f.DrawCentered(dst, "Enter: check   "+p.langKeys()+"Esc: "+p.escTo(), game.ScreenW/2, y+70, 1, pal.Steel)
		return
	}
	help := "Enter: check   Backspace: fix   " + p.langKeys() + "Esc: " + p.escTo()
	if cycle, _, ok := p.lang().AccentCycle('e'); ok {
		hint := strings.Join(strings.Split(string(cycle), ""), " → ")
		f.DrawCentered(dst, "Tab after a letter adds an accent:  "+hint, game.ScreenW/2, y+22, 1, pal.Tan)
	}
	if p.lang().Script == words.ScriptGreek {
		help = "F2: Greek keys on   " + help
	}
	f.DrawCentered(dst, help, game.ScreenW/2, y+40, 1, pal.Steel)
}

// langKeys is the key hint for changing language, when the arrows do.
func (p *Practice) langKeys() string {
	if p.assign != nil {
		return ""
	}
	return "←/→: language   "
}

// escTo names where Esc goes.
func (p *Practice) escTo() string {
	if p.assign != nil {
		return "assignments"
	}
	return "menu"
}
