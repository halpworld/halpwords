package scene

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/words"
)

// setting is one line of the Settings screen: a name, its choices, which
// is set, and how to change it.
type setting struct {
	name    string
	about   string
	choices []string
	get     func(ls *profile.LangSettings) int
	set     func(ls *profile.LangSettings, i int)
}

// strictness is the order Settings offers strictness in.
var strictness = []words.Strictness{words.Strict, words.Reduced, words.Ignore}

func strictChoice(s words.Strictness) int {
	for i, v := range strictness {
		if v == s {
			return i
		}
	}
	return 0
}

// markName is what a language calls its marks.
func markName(lang *words.Language) string {
	switch lang.Code {
	case "la":
		return "Macrons"
	case "ga":
		return "Fadas"
	}
	return "Accents"
}

// settingsFor lists the settings that apply to lang.
func settingsFor(lang *words.Language) []setting {
	marks := markName(lang)
	list := []setting{{
		name:    marks,
		about:   "Strict: a wrong or missing mark is a miss. Reduced: it still hits, for less.",
		choices: []string{"strict", "reduced credit", "ignore"},
		get:     func(ls *profile.LangSettings) int { return strictChoice(ls.Rules.Accents) },
		set:     func(ls *profile.LangSettings, i int) { ls.Rules.Accents = strictness[i] },
	}}
	if lang.Script == words.ScriptGreek {
		list = append(list, setting{
			name:    "Breathings",
			about:   "Smooth and rough breathings, such as ἀ and ἁ.",
			choices: []string{"strict", "reduced credit", "ignore"},
			get:     func(ls *profile.LangSettings) int { return strictChoice(ls.Rules.Breathings) },
			set:     func(ls *profile.LangSettings, i int) { ls.Rules.Breathings = strictness[i] },
		})
	}
	if len(lang.Articles) > 0 {
		list = append(list, setting{
			name:    "Articles",
			about:   "Required: \"le chien\", not just \"chien\". Good practice for genders.",
			choices: []string{"required", "optional"},
			get:     func(ls *profile.LangSettings) int { return b2i(!ls.Rules.ArticlesRequired) },
			set:     func(ls *profile.LangSettings, i int) { ls.Rules.ArticlesRequired = i == 0 },
		})
	}
	return append(list,
		setting{
			name:    "Capitals",
			about:   "Strict: capital letters must match the word list.",
			choices: []string{"strict", "ignore"},
			get:     func(ls *profile.LangSettings) int { return b2i(!ls.Rules.CaseSensitive) },
			set:     func(ls *profile.LangSettings, i int) { ls.Rules.CaseSensitive = i == 0 },
		},
		setting{
			name:    "Live typo highlighting",
			about:   "Letters turn red from the first typo, like a Rune of Clarity.",
			choices: []string{"on", "off"},
			get:     func(ls *profile.LangSettings) int { return b2i(!ls.Highlight) },
			set:     func(ls *profile.LangSettings, i int) { ls.Highlight = i == 0 },
		},
		setting{
			name:    "Timer speed",
			about:   "How long battles give you to type. Relaxed gives half as much time again.",
			choices: []string{"relaxed", "normal", "fast"},
			get: func(ls *profile.LangSettings) int {
				for i, t := range profile.Timers {
					if t == ls.Timer {
						return i
					}
				}
				return 1
			},
			set: func(ls *profile.LangSettings, i int) { ls.Timer = profile.Timers[i] },
		},
	)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Settings changes the game's sound and screen, and how answers are
// graded and timed for each language. Its first tab is the game's; then
// there is one per language.
type Settings struct {
	bg    *ebiten.Image
	langs []*words.Language
	tab   int // 0 is Sound & Screen, then the languages
	sel   int // 0 is the tab, then the settings, then Reset
	note  string
	// pushed is set when Settings was opened over an adventure. Then it
	// only has the Sound & Screen tab, since the adventure's grading rules
	// are fixed when it starts, and Esc goes back to it.
	pushed bool
}

// NewSettings creates the Settings screen.
func NewSettings(ctx *game.Context) game.Scene {
	return &Settings{bg: backdrop(5, 1.3), langs: words.Languages}
}

// newOptions creates the Settings screen with only its Sound & Screen tab,
// to open over an adventure.
func newOptions(ctx *game.Context) game.Scene {
	return &Settings{bg: backdrop(5, 1.3), pushed: true}
}

// lang is the language whose tab is showing, or nil on the game's tab.
func (s *Settings) lang() *words.Language {
	if s.tab == 0 {
		return nil
	}
	return s.langs[s.tab-1]
}

func (s *Settings) tabName(i int) string {
	if i == 0 {
		return "Sound & Screen"
	}
	return s.langs[i-1].Name
}

// option is a line of the Sound & Screen tab.
type option struct {
	name, about string
	choices     []string
	volume      bool // drawn as a bar of MaxVolume steps
	get         func(o *profile.Options) int
	set         func(o *profile.Options, i int)
}

func onOff(b bool) int { return b2i(!b) }

var options = []option{
	{
		name: "Music", about: "How loud the music is. F3 turns all sound off and on.", volume: true,
		get: func(o *profile.Options) int { return o.Music },
		set: func(o *profile.Options, i int) { o.Music = i },
	},
	{
		name: "Sound effects", about: "How loud the sound effects are.", volume: true,
		get: func(o *profile.Options) int { return o.Effects },
		set: func(o *profile.Options, i int) { o.Effects = i },
	},
	{
		name: "CRT filter", about: "Makes the screen look like an old monitor, with scanlines.",
		choices: []string{"off", "soft", "strong"},
		get:     func(o *profile.Options) int { return int(o.CRT) },
		set:     func(o *profile.Options, i int) { o.CRT = profile.CRT(i) },
	},
	{
		name: "Full screen", about: "F11 or Alt+Enter switches at any time.",
		choices: []string{"on", "off"},
		get:     func(o *profile.Options) int { return onOff(o.Fullscreen) },
		set:     func(o *profile.Options, i int) { o.Fullscreen = i == 0 },
	},
	{
		name: "Screen shake", about: "The view shakes when the hero is hit or a boss rages.",
		choices: []string{"on", "off"},
		get:     func(o *profile.Options) int { return onOff(o.Shake) },
		set:     func(o *profile.Options, i int) { o.Shake = i == 0 },
	},
}

// line is one line of the tab showing, ready to draw or change.
type line struct {
	name, about string
	choices     []string
	volume      bool
	cur         int
	change      func(i int)
}

// lines lists the settings on the tab showing.
func (s *Settings) lines(ctx *game.Context) []line {
	var out []line
	if lang := s.lang(); lang != nil {
		ls := ctx.Profile.Settings.For(lang)
		for _, st := range settingsFor(lang) {
			out = append(out, line{name: st.name, about: st.about, choices: st.choices, cur: st.get(&ls), change: func(i int) {
				s.saveLang(ctx, func(ls *profile.LangSettings) { st.set(ls, i) })
			}})
		}
		return out
	}
	o := ctx.Profile.Settings.Options()
	for _, op := range options {
		n := len(op.choices)
		if op.volume {
			n = profile.MaxVolume + 1
		}
		out = append(out, line{name: op.name, about: op.about, choices: op.choices, volume: op.volume, cur: op.get(&o), change: func(i int) {
			s.saveOptions(ctx, func(o *profile.Options) { op.set(o, max(0, min(n-1, i))) })
		}})
	}
	return out
}

// Update implements game.Scene.
func (s *Settings) Update(ctx *game.Context) error {
	list := s.lines(ctx)
	n := len(list) + 2
	step, fresh := 0, false // fresh: a new press, not a key held down
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		if s.pushed {
			ctx.Pop()
		} else {
			ctx.Replace(NewTitle(ctx))
		}
		return nil
	case input.Pressed(ebiten.KeyTab):
		s.switchTab(ctx, 1)
		return nil
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + 1) % n
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyA):
		step = -1
		fresh = input.Pressed(ebiten.KeyArrowLeft, ebiten.KeyA)
	case input.Repeat(ebiten.KeyArrowRight) || input.Repeat(ebiten.KeyD):
		step = 1
		fresh = input.Pressed(ebiten.KeyArrowRight, ebiten.KeyD)
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		if s.sel == n-1 {
			s.reset(ctx)
			return nil
		}
		step, fresh = 1, true
	}
	if step == 0 {
		return nil
	}
	switch {
	case s.sel == 0:
		s.switchTab(ctx, step)
	case s.sel < n-1:
		l := list[s.sel-1]
		switch {
		case l.volume:
			l.change(l.cur + step) // volumes stop at the ends
		case !fresh:
			// Holding a key only slides volumes; flicking a choice back and
			// forth, like full screen, helps no one.
		default:
			l.change((l.cur + step + len(l.choices)) % len(l.choices))
		}
		s.note = ""
	}
	return nil
}

// tabs is how many tabs there are.
func (s *Settings) tabs() int { return len(s.langs) + 1 }

func (s *Settings) switchTab(ctx *game.Context, step int) {
	if s.tabs() == 1 {
		return
	}
	ctx.Sound.Play(audio.Blip)
	s.tab = (s.tab + step + s.tabs()) % s.tabs()
	s.sel, s.note = 0, ""
}

// reset puts the tab showing back to the standard settings.
func (s *Settings) reset(ctx *game.Context) {
	if lang := s.lang(); lang != nil {
		s.saveLang(ctx, func(ls *profile.LangSettings) { *ls = profile.Preset(lang) })
		s.note = lang.Name + " is back to the standard settings."
		return
	}
	s.saveOptions(ctx, func(o *profile.Options) {
		full := o.Fullscreen // leave the window as it is
		*o = profile.DefaultOptions()
		o.Fullscreen = full
	})
	s.note = "Sound and screen are back to the standard settings."
}

// saveLang edits the settings for the language shown, and saves them.
func (s *Settings) saveLang(ctx *game.Context, edit func(*profile.LangSettings)) {
	ls := ctx.Profile.Settings.For(s.lang())
	edit(&ls)
	ctx.Profile.Settings.Set(s.lang(), ls)
	s.saved(ctx)
}

// saveOptions edits the game settings, puts them into effect and saves
// them.
func (s *Settings) saveOptions(ctx *game.Context, edit func(*profile.Options)) {
	o := ctx.Profile.Settings.Options()
	edit(&o)
	ctx.Profile.Settings.SetOptions(o)
	ctx.ApplyOptions()
	s.saved(ctx)
}

func (s *Settings) saved(ctx *game.Context) {
	if err := ctx.Profile.SaveSettings(); err != nil {
		ctx.Sound.Play(audio.Wrong)
		ctx.Notify("Could not save the settings")
		return
	}
	ctx.Sound.Play(audio.Accent)
}

// Draw implements game.Scene.
func (s *Settings) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, s.bg, 0, 0)
	f.DrawCentered(dst, "Settings", cx, 4, 3, pal.Yellow)

	// The tabs, in a row under the title.
	tw := 0
	for i := 0; i < s.tabs(); i++ {
		tw += f.Width(s.tabName(i), 1) + 20
	}
	tx := cx - tw/2
	for i := 0; i < s.tabs(); i++ {
		name := s.tabName(i)
		w := f.Width(name, 1) + 12
		col, bg := pal.Stone, pal.Fade(pal.Black, 0.5)
		if i == s.tab {
			col, bg = pal.Yellow, pal.Fade(pal.Indigo, 0.9)
			if s.sel == 0 {
				col = pal.White
			}
		}
		gfx.FillRect(dst, tx, 54, w, 20, bg)
		if i == s.tab {
			gfx.FillRect(dst, tx, 72, w, 2, col)
		}
		f.DrawShadow(dst, name, tx+6, 56, 1, col)
		tx += w + 8
	}

	const x, w, rowH, valX = 24, game.ScreenW - 48, 24, 244
	y := 80
	list := s.lines(ctx)
	gfx.Window(dst, x, y, w, rowH*(len(list)+2)+24)
	row := func(i int, name string, draw func(y int, sel bool)) {
		ry := y + 12 + i*rowH
		if i == len(list)+1 {
			ry += 8
		}
		sel := i == s.sel
		col := pal.Steel
		if sel {
			gfx.FillRect(dst, x+6, ry-3, w-12, rowH-2, pal.Fade(pal.Indigo, 0.8))
			col = pal.White
		}
		f.DrawShadow(dst, name, x+20, ry, 1, col)
		if draw != nil {
			draw(ry, sel)
		}
	}
	show := "◄ " + s.tabName(s.tab) + " ►"
	if s.tabs() == 1 {
		show = s.tabName(s.tab)
	}
	row(0, "Show", func(ry int, sel bool) {
		f.DrawShadow(dst, show, x+valX, ry, 1, pal.Yellow)
	})
	for i, l := range list {
		row(i+1, l.name, func(ry int, sel bool) {
			vx := x + valX
			if l.volume {
				for k := 1; k <= profile.MaxVolume; k++ {
					col := pal.Night
					if k <= l.cur {
						col = pal.Lime
						if sel {
							col = pal.Yellow
						}
					}
					h := 4 + k
					gfx.FillRect(dst, vx+(k-1)*10, ry+14-h, 7, h, col)
				}
				label := "off"
				if l.cur > 0 {
					label = fmt.Sprintf("%d0%%", l.cur)
				}
				f.DrawShadow(dst, label, vx+110, ry, 1, pal.Steel)
				return
			}
			for k, ch := range l.choices {
				col := pal.Stone
				if k == l.cur {
					col = pal.Lime
					if sel {
						col = pal.Yellow
					}
					gfx.FillRect(dst, vx-4, ry+15, f.Width(ch, 1)+8, 1, col)
				}
				f.DrawShadow(dst, ch, vx, ry, 1, col)
				vx += f.Width(ch, 1) + 24
			}
		})
	}
	what := "sound and screen"
	if lang := s.lang(); lang != nil {
		what = lang.Name
	}
	row(len(list)+1, "Reset "+what+" to the standard settings", nil)

	about := "Choose with ←/→ or Tab."
	switch {
	case s.sel > 0 && s.sel <= len(list):
		about = list[s.sel-1].about
	case s.sel == len(list)+1:
		about = "The standard settings suit most classes."
	case s.tabs() == 1:
		about = "The grading rules can be changed from the title screen."
	}
	by := game.ScreenH - 66
	f.DrawCentered(dst, about, cx, by, 1, pal.Ice)
	switch {
	case s.note != "":
		f.DrawCentered(dst, s.note, cx, by+18, 1, pal.Lime)
	case s.lang() != nil:
		f.DrawCentered(dst, "Hardcore runs always use the standard settings, so scores compare.", cx, by+18, 1, pal.Tan)
	}
	help := "↑/↓ choose   ←/→ change   Tab next tab   Esc back"
	if s.pushed {
		help = "↑/↓ choose   ←/→ change   Esc back to the adventure"
	}
	f.DrawShadow(dst, help, 8, game.ScreenH-20, 1, pal.Ash)
}
