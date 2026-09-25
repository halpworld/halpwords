package scene

import (
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

// Settings changes how answers are graded and timed, for each language.
type Settings struct {
	bg    *ebiten.Image
	langs []*words.Language
	li    int
	sel   int // 0 is the language, then the settings, then Reset
	note  string
}

// NewSettings creates the Settings screen.
func NewSettings(ctx *game.Context) game.Scene {
	return &Settings{bg: backdrop(5, 1.3), langs: words.Languages}
}

func (s *Settings) lang() *words.Language { return s.langs[s.li] }

// rows is the number of lines that can be chosen.
func (s *Settings) rows() int { return len(settingsFor(s.lang())) + 2 }

// Update implements game.Scene.
func (s *Settings) Update(ctx *game.Context) error {
	n := s.rows()
	step := 0
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewTitle(ctx))
		return nil
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + 1) % n
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyA):
		step = -1
	case input.Repeat(ebiten.KeyArrowRight) || input.Repeat(ebiten.KeyD):
		step = 1
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		if s.sel == n-1 {
			s.change(ctx, func(ls *profile.LangSettings) { *ls = profile.Preset(s.lang()) })
			s.note = s.lang().Name + " is back to the standard settings."
			return nil
		}
		step = 1
	}
	if step == 0 {
		return nil
	}
	switch {
	case s.sel == 0:
		ctx.Sound.Play(audio.Blip)
		s.li = (s.li + step + len(s.langs)) % len(s.langs)
		s.sel, s.note = 0, ""
	case s.sel < n-1:
		st := settingsFor(s.lang())[s.sel-1]
		s.change(ctx, func(ls *profile.LangSettings) {
			st.set(ls, (st.get(ls)+step+len(st.choices))%len(st.choices))
		})
		s.note = ""
	}
	return nil
}

// change edits the settings for the language shown, and saves them.
func (s *Settings) change(ctx *game.Context, edit func(*profile.LangSettings)) {
	ls := ctx.Profile.Settings.For(s.lang())
	edit(&ls)
	ctx.Profile.Settings.Set(s.lang(), ls)
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
	f.DrawCentered(dst, "Settings", cx, 12, 3, pal.Yellow)

	const x, w, rowH, valX = 24, game.ScreenW - 48, 24, 244
	y := 70
	list := settingsFor(s.lang())
	gfx.Window(dst, x, y, w, rowH*(len(list)+2)+24)
	ls := ctx.Profile.Settings.For(s.lang())
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
	row(0, "Language", func(ry int, sel bool) {
		col := pal.Yellow
		f.DrawShadow(dst, "◄ "+s.lang().Name+" ►", x+valX, ry, 1, col)
	})
	for i, st := range list {
		cur := st.get(&ls)
		row(i+1, st.name, func(ry int, sel bool) {
			vx := x + valX
			for k, ch := range st.choices {
				col := pal.Stone
				if k == cur {
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
	row(len(list)+1, "Reset "+s.lang().Name+" to the standard settings", nil)

	about := "Choose a language with ←/→."
	switch {
	case s.sel > 0 && s.sel <= len(list):
		about = list[s.sel-1].about
	case s.sel == len(list)+1:
		about = "The standard settings suit most classes."
	}
	by := game.ScreenH - 74
	f.DrawCentered(dst, about, cx, by, 1, pal.Ice)
	if s.note != "" {
		f.DrawCentered(dst, s.note, cx, by+18, 1, pal.Lime)
	} else {
		f.DrawCentered(dst, "Hardcore runs always use the standard settings, so scores compare.", cx, by+18, 1, pal.Tan)
	}
	f.DrawShadow(dst, "↑/↓ choose   ←/→ change   Enter reset   Esc back", 8, game.ScreenH-20, 1, pal.Ash)
}
