package scene

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// maxName is the longest name the Hall of Fame keeps.
const maxName = 12

// GameOver ends a Hardcore run: it adds up the score, gives the share code
// and puts a good run in the Hall of Fame.
type GameOver struct {
	bg     *ebiten.Image
	lang   *words.Language
	mode   compete.Mode
	class  string
	floor  int
	tally  compete.Tally
	score  int
	seed   uint64
	code   string // the share code
	best   int    // the best score before this run
	gaveUp bool

	place   int    // where the run would go in the Hall of Fame, or 0
	name    []rune // the name being typed for the Hall of Fame
	entered bool   // the run is in the Hall of Fame
}

// newGameOver ends run r, which fell or was given up.
func newGameOver(ctx *game.Context, r *run, gaveUp bool) *GameOver {
	ctx.EndSession()
	g := &GameOver{
		bg: backdrop(9, 1.2), lang: r.lang, mode: r.mode, class: r.hero.Class.String(),
		floor: r.depth, tally: r.tally, score: r.score(), seed: r.seed, gaveUp: gaveUp,
	}
	share := compete.Share{Lang: r.lang.Code, Seed: r.seed, Floor: r.depth, Score: g.score}
	if r.mode == compete.Daily {
		if d, err := time.Parse(time.DateOnly, r.day); err == nil {
			share = compete.Share{Lang: r.lang.Code, Daily: true, Month: d.Month(), Day: d.Day(), Floor: r.depth, Score: g.score}
		}
	}
	g.code = share.Code()
	key := compete.TableKey(r.mode, r.lang.Code)
	g.best = ctx.Profile.Fame.Best(key)
	g.place = ctx.Profile.Fame.Rank(key, g.score)
	if g.score == 0 {
		g.place = 0
	}
	g.name = []rune(ctx.Profile.Name)
	r.remember()
	return g
}

// record puts the run in the Hall of Fame under the typed name.
func (g *GameOver) record(ctx *game.Context) {
	name := strings.TrimSpace(string(g.name))
	if name == "" {
		name = "Hero"
	}
	p := ctx.Profile
	p.Name = name
	day := today()
	g.place = p.Fame.Add(compete.TableKey(g.mode, g.lang.Code), compete.Fame{
		Name: name, Class: g.class, Floor: g.floor, Score: g.score, Date: day, Code: g.code,
	})
	if err := p.SaveFame(); err != nil {
		ctx.Notify("Could not save the Hall of Fame")
	}
	g.entered = true
}

// Update implements game.Scene.
func (g *GameOver) Update(ctx *game.Context) error {
	if g.place > 0 && !g.entered {
		for _, r := range ctx.Input.Chars {
			if (unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '\'') && len(g.name) < maxName {
				g.name = append(g.name, r)
				ctx.Sound.Play(audio.Key)
			}
		}
		if input.Repeat(ebiten.KeyBackspace) && len(g.name) > 0 {
			g.name = g.name[:len(g.name)-1]
			ctx.Sound.Play(audio.Erase)
		}
		if input.Confirm() {
			ctx.Sound.Play(audio.LevelUp)
			g.record(ctx)
		}
		return nil
	}
	switch {
	case input.Pressed(ebiten.KeyH):
		ctx.Sound.Play(audio.Select)
		ctx.Replace(hallAt(g.mode, g.lang, g.place))
	case input.Confirm() || input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewTitle(ctx))
	}
	return nil
}

// Draw implements game.Scene.
func (g *GameOver) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, g.bg, 0, 0)
	title := "YOU HAVE FALLEN"
	if g.gaveUp {
		title = "THE RUN IS OVER"
	}
	f.DrawOutline(dst, title, cx-f.Width(title, 3)/2, 10, 3, pal.Rose, pal.Black)
	sub := fmt.Sprintf("%s · %s · %s · seed %s", g.mode, g.lang.Name, g.class, compete.SeedCode(g.seed))
	f.DrawCentered(dst, sub, cx, 62, 1, pal.Tan)

	// The score, part by part.
	const x, w = 24, 290
	y := 84
	gfx.Window(dst, x, y, w, 196)
	t := g.tally
	parts := []struct {
		name string
		pts  int
	}{
		{fmt.Sprintf("Floor %d", g.floor), g.floor * compete.FloorPoints},
		{"Damage dealt", t.Damage},
		{count(t.Perfect, "perfect word", "perfect words"), t.Perfect * compete.PerfectPoints},
		{fmt.Sprintf("Best combo %d", t.BestCombo), t.BestCombo * compete.ComboPoints},
		{count(t.Bosses, "boss", "bosses"), t.Bosses * compete.BossPoints},
		{count(t.Chests, "chest", "chests"), t.Chests * compete.ChestPoints},
		{count(t.Misses, "miss", "misses"), -t.Misses * compete.MissPoints},
	}
	for i, p := range parts {
		py := y + 10 + i*18
		f.DrawShadow(dst, p.name, x+14, py, 1, pal.Steel)
		v := groupDigits(p.pts)
		if p.pts < 0 {
			v = "-" + groupDigits(-p.pts)
		}
		f.DrawShadow(dst, v, x+w-14-f.Width(v, 1), py, 1, pal.Ice)
	}
	gfx.FillRect(dst, x+10, y+140, w-20, 1, pal.Indigo)
	f.DrawShadow(dst, "Score", x+14, y+150, 2, pal.Yellow)
	sc := groupDigits(g.score)
	f.DrawShadow(dst, sc, x+w-14-f.Width(sc, 2), y+150, 2, pal.Yellow)

	// The Hall of Fame and the share code.
	rx, rw := x+w+12, game.ScreenW-24-w-12
	gfx.Window(dst, rx, y, rw, 196)
	rcx := rx + rw/2
	switch {
	case g.score > g.best && g.best > 0:
		f.DrawCentered(dst, "★ NEW PERSONAL BEST! ★", rcx, y+10, 1, pal.Yellow)
	case g.best > 0:
		f.DrawCentered(dst, "Your best: "+groupDigits(g.best), rcx, y+10, 1, pal.Tan)
	default:
		f.DrawCentered(dst, "Your first "+g.mode.String()+" score!", rcx, y+10, 1, pal.Yellow)
	}
	switch {
	case g.place > 0 && !g.entered:
		f.DrawCentered(dst, fmt.Sprintf("Place %d in the Hall of Fame!", g.place), rcx, y+34, 1, pal.Lime)
		f.DrawCentered(dst, "Type your name:", rcx, y+52, 1, pal.Tan)
		drawTyped(dst, ctx, string(g.name), rcx, y+68, rw-40, 2, true)
	case g.entered:
		f.DrawCentered(dst, fmt.Sprintf("%s is number %d in the", string(g.name), g.place), rcx, y+34, 1, pal.Lime)
		f.DrawCentered(dst, "Hall of Fame!", rcx, y+52, 2, pal.Lime)
	default:
		f.DrawCentered(dst, "Not in the Hall of Fame this time.", rcx, y+34, 1, pal.Steel)
		f.DrawCentered(dst, "Keep practising!", rcx, y+52, 1, pal.Steel)
	}
	f.DrawCentered(dst, "Share code for your friends:", rcx, y+118, 1, pal.Tan)
	f.DrawCentered(dst, g.code, rcx, y+136, f.FitScale(g.code, rw-20, 2), pal.White)
	f.DrawCentered(dst, "Check codes in the Hall of Fame.", rcx, y+172, 1, pal.Ash)

	help := "Enter title screen · H Hall of Fame"
	if g.place > 0 && !g.entered {
		help = "Type your name · Enter save"
	}
	f.DrawShadow(dst, help, 8, game.ScreenH-20, 1, pal.Ash)
}

// count says how many of something there are: "1 boss", "2 bosses".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
