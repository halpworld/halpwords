package scene

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/words"
)

// fewWords is how many words a run can have before the checklist warns
// that they will come round often. Fewer are allowed.
const fewWords = 5

// ListPick is the checklist of a language's word lists before an
// Adventure or a Practice (#89): the lists ticked last are ticked, so
// Enter plays the same as last time.
type ListPick struct {
	bg    *ebiten.Image
	lang  *words.Language
	about string // what the lists are for, such as "Adventure"
	rows  []*listRow
	sel   int
	top   int // the first row shown
	// langs, when set, are the languages ←/→ go through (Practice).
	langs  []*words.Language
	locked bool // an assignment locks the lists
	msg    string
	start  func(ctx *game.Context, lang *words.Language, pool listPool) game.Scene
	back   func(ctx *game.Context) game.Scene
}

// newListPick makes the checklist for lang. start makes the scene the
// ticked lists are played in; back is where Esc goes.
func newListPick(ctx *game.Context, lang *words.Language, about string,
	start func(*game.Context, *words.Language, listPool) game.Scene, back func(*game.Context) game.Scene) *ListPick {
	p := &ListPick{bg: backdrop(3, 1.4), about: about, start: start, back: back}
	p.setLanguage(ctx, lang)
	return p
}

func (p *ListPick) setLanguage(ctx *game.Context, lang *words.Language) {
	p.lang, p.sel, p.top, p.msg = lang, 0, 0, ""
	p.rows = listRows(ctx, lang)
	p.locked = false
	for _, r := range p.rows {
		p.locked = p.locked || r.locked
	}
}

// pickNeeded reports whether a run set up as setup in lang asks which
// lists to play: not for an assignment, a Daily or Hardcore run, or a
// quest that names its lists.
func pickNeeded(ctx *game.Context, lang *words.Language, setup runSetup) bool {
	return setup.assign == nil && !setup.mode.Scored() && !questNamesLists(ctx, lang, setup.quest)
}

// adventureLists is the checklist before a run in lang set up as setup;
// Enter goes on to the hero.
func adventureLists(ctx *game.Context, lang *words.Language, setup runSetup) *ListPick {
	setup.pool, setup.picked = listPool{}, false
	about := setupText(setup)
	start := func(_ *game.Context, lang *words.Language, pool listPool) game.Scene {
		s := setup
		s.pool, s.picked = pool, true
		return NewClassPick(lang, s)
	}
	back := func(ctx *game.Context) game.Scene {
		if setup.quest != nil && setup.quest.Language != "" {
			return NewQuests(ctx)
		}
		return NewAdventure(ctx, setup)
	}
	return newListPick(ctx, lang, about, start, back)
}

// NewPracticeLists is the checklist before Practice. ←/→ change the
// language.
func NewPracticeLists(ctx *game.Context) game.Scene {
	var langs []*words.Language
	for _, l := range words.Languages {
		if len(ctx.ListsFor(l.Code)) > 0 {
			langs = append(langs, l)
		}
	}
	if len(langs) == 0 {
		return NewPractice(ctx) // as before the checklist
	}
	start := func(ctx *game.Context, lang *words.Language, pool listPool) game.Scene {
		return newPracticeIn(ctx, lang, pool)
	}
	p := newListPick(ctx, langs[0], "Practice", start, NewTitle)
	p.langs = langs
	return p
}

// ticked returns the keys of the ticked lists, in the game's order, and
// how many different words they hold.
func (p *ListPick) ticked() (keys []string, n int) {
	seen := map[string]bool{}
	for _, r := range p.rows {
		if !r.ticked {
			continue
		}
		keys = append(keys, r.key)
		for _, e := range r.list.Entries {
			seen[words.Key(e)] = true
		}
	}
	return keys, len(seen)
}

// Update implements game.Scene.
func (p *ListPick) Update(ctx *game.Context) error {
	n := len(p.rows)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Replace(p.back(ctx))
	case len(p.langs) > 1 && (input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyArrowRight)):
		ctx.Sound.Play(audio.Blip)
		i := 0
		for k, l := range p.langs {
			if l == p.lang {
				i = k
			}
		}
		d := 1
		if input.Repeat(ebiten.KeyArrowLeft) {
			d = len(p.langs) - 1
		}
		p.setLanguage(ctx, p.langs[(i+d)%len(p.langs)])
	case n == 0:
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		p.sel = (p.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		p.sel = (p.sel + 1) % n
	case input.Pressed(ebiten.KeySpace):
		p.toggle(ctx, func(r *listRow) bool { return r == p.rows[p.sel] }, !p.rows[p.sel].ticked)
	case input.Pressed(ebiten.KeyA):
		all := true
		for _, r := range p.rows {
			all = all && r.ticked
		}
		p.toggle(ctx, func(*listRow) bool { return true }, !all)
	case input.Confirm():
		p.begin(ctx)
	}
	return nil
}

// toggle ticks (or unticks) the rows which says, unless the lists are
// locked.
func (p *ListPick) toggle(ctx *game.Context, which func(*listRow) bool, tick bool) {
	if p.locked {
		ctx.Sound.Play(audio.Wrong)
		p.msg = "Your teacher locked these lists for an assignment."
		return
	}
	ctx.Sound.Play(audio.Blip)
	p.msg = ""
	for _, r := range p.rows {
		if which(r) {
			r.ticked = tick
		}
	}
}

// begin plays the ticked lists, and remembers them for next time.
func (p *ListPick) begin(ctx *game.Context) {
	keys, n := p.ticked()
	if n == 0 {
		ctx.Sound.Play(audio.Wrong)
		p.msg = "Tick at least one list to start."
		return
	}
	ctx.Sound.Play(audio.Select)
	pool := listPool{keys: keys}
	if !p.locked && ctx.Profile != nil {
		if len(keys) == len(p.rows) {
			keys = nil // every list, and lists added later too
		}
		ctx.Profile.Settings.Lists.Pick(p.lang.Code, keys)
		ctx.Profile.SaveSettings()
	}
	ctx.Replace(p.start(ctx, p.lang, pool))
}

// The checklist's layout.
const (
	lpX, lpY, lpW = 40, 74, game.ScreenW - 80
	lpRowH        = 18
	lpRows        = 10
)

// Draw implements game.Scene.
func (p *ListPick) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, p.bg, 0, 0)
	f.DrawCentered(dst, "Choose your word lists", cx, 10, 2, pal.Yellow)
	head := p.lang.Name + " · " + p.about
	if len(p.langs) > 1 {
		head = "◄ " + p.lang.Name + " ► · " + p.about
	}
	f.DrawCentered(dst, fit(f, head, game.ScreenW-40, 1), cx, 38, 1, pal.Ice)
	if note, col := p.note(); note != "" {
		f.DrawCentered(dst, fit(f, note, game.ScreenW-40, 1), cx, 55, 1, col)
	}

	h := lpRows*lpRowH + 14
	gfx.Window(dst, lpX, lpY, lpW, h)
	p.top = max(0, min(p.top, p.sel, len(p.rows)-lpRows))
	if p.sel >= p.top+lpRows {
		p.top = p.sel - lpRows + 1
	}
	for i := p.top; i < min(len(p.rows), p.top+lpRows); i++ {
		p.drawRow(dst, ctx, i, lpY+8+(i-p.top)*lpRowH)
	}
	if p.top > 0 {
		f.Draw(dst, "▲", cx-4, lpY-1, 1, pal.Tan)
	}
	if p.top+lpRows < len(p.rows) {
		f.Draw(dst, "▼", cx-4, lpY+h-15, 1, pal.Tan)
	}

	_, n := p.ticked()
	y := lpY + h + 6
	total := plural(n, "word") + " selected"
	switch {
	case n == 0:
		f.DrawShadow(dst, total, lpX+4, y, 1, pal.Rose)
		f.DrawShadow(dst, "Tick a list to start", lpX+lpW-4-f.Width("Tick a list to start", 1), y, 1, pal.Ash)
	default:
		f.DrawShadow(dst, total, lpX+4, y, 1, pal.White)
		start := "Enter: start ►"
		f.DrawShadow(dst, start, lpX+lpW-4-f.Width(start, 1), y, 1, pal.Lime)
	}
	switch {
	case p.msg != "":
		f.DrawShadow(dst, fit(f, p.msg, lpW, 1), lpX+4, y+18, 1, pal.Rose)
	case n > 0 && n < fewWords:
		f.DrawShadow(dst, "Only a few words: they will come round often.", lpX+4, y+18, 1, pal.Orange)
	}
	keys := "↑/↓ choose   Space tick   A all/none   Enter start   Esc back"
	if p.locked {
		keys = "↑/↓ look   Enter start   Esc back"
	}
	if len(p.langs) > 1 {
		keys = "←/→ language   " + keys
	}
	f.DrawShadow(dst, fit(f, keys, game.ScreenW-16, 1), 8, game.ScreenH-20, 1, pal.Ash)
}

// note is the line under the heading: the lists new to the learner, or
// the lock.
func (p *ListPick) note() (string, color.RGBA) {
	var titles []string
	for _, r := range p.rows {
		if r.isNew {
			titles = append(titles, r.list.Title)
		}
	}
	if len(titles) > 0 {
		return "New: " + strings.Join(titles, ", "), pal.Lime
	}
	if p.locked {
		return "An assignment locks these lists. Your teacher chose them.", pal.Sky
	}
	return "", pal.Ice
}

func (p *ListPick) drawRow(dst *ebiten.Image, ctx *game.Context, i, y int) {
	f := ctx.Font
	r := p.rows[i]
	col := pal.Steel
	if i == p.sel {
		gfx.FillRect(dst, lpX+6, y-1, lpW-12, lpRowH, pal.Indigo)
		col = pal.White
	}
	box, boxCol := "□", pal.Ash
	if r.ticked {
		box, boxCol = "■", pal.Lime
	}
	if r.locked {
		boxCol = pal.Sky
	}
	f.Draw(dst, box, lpX+12, y, 1, boxCol)
	right := fmt.Sprintf("%d words", len(r.list.Entries))
	tag, tagCol := "", pal.Ash
	switch {
	case r.locked:
		tag, tagCol = "locked", pal.Sky
	case r.isNew:
		tag, tagCol = "new", pal.Lime
	case r.sent:
		tag, tagCol = "sent", pal.Tan
	case r.assign:
		tag, tagCol = "assigned", pal.Tan
	}
	rw := f.Width(right, 1)
	f.DrawShadow(dst, right, lpX+lpW-12-rw, y, 1, pal.Ash)
	tx := lpX + lpW - 12 - rw - 80
	if tag != "" {
		f.DrawShadow(dst, tag, tx, y, 1, tagCol)
	}
	f.DrawShadow(dst, fit(f, r.list.Title, tx-lpX-40, 1), lpX+30, y, 1, col)
}
