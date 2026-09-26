package scene

import (
	"errors"
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/race"
	"github.com/halpworld/halpwords/pkg/words"
)

// Races (W7.6): in a race room the host starts a race, and every racer's
// game builds the same dungeon from the race's seed and word list. A race
// run is played by Hardcore rules (one life, so the floors never change),
// as the first class, with only the race list's words: no helpers, no AI,
// no saves, and nothing written to the player's memory or the grown-up's
// account. The game reports its floor, monsters beaten and the hero's cell
// (link.Play.Report); the others show as small markers on the map. The
// first to reach the goal floor wins.

// raceRun is the race a run is part of.
type raceRun struct {
	play   *link.Play
	number int // the race's link.Race.Number
	goal   int
	you    string // the player's member ID
	// monsters counts the monsters beaten in the race; fell is set when
	// the hero fell or gave up, done when they reached the goal.
	monsters   int
	fell, done bool
}

// errRaceList is a race list the game can't play.
var errRaceList = errors.New("this game can't read the race's word list")

// newRaceRun makes the run for race rc, as member you.
func newRaceRun(ctx *game.Context, rc *link.Race, you string) (*run, error) {
	l, err := words.Parse(strings.NewReader(rc.List), "race")
	if err != nil || len(l.Entries) == 0 {
		return nil, errRaceList
	}
	lang, ok := words.Lookup(l.Language)
	if !ok {
		return nil, errRaceList
	}
	r := startRunWith(ctx, lang, rpg.Classes[0], rc.Seed, []*words.List{l})
	r.setMode(ctx, compete.Hardcore)
	r.race = &raceRun{play: ctx.Link.Play(), number: rc.Number, goal: rc.Goal, you: you}
	return r, nil
}

// startRace begins the race run r: its first floor, with a banner.
func startRace(r *run) *Crawl {
	c := newCrawl(r)
	c.showBanner("RACE!", fmt.Sprintf("First to floor %d wins", r.race.goal))
	r.say(fmt.Sprintf("The race is on! Find the stairs down to floor %d.", r.race.goal), pal.Yellow)
	return c
}

// report is how far the hero has got, for the room.
func (rr *raceRun) report(r *run, at dungeon.Point) race.Report {
	return race.Report{Floor: min(r.depth, rr.goal), Monsters: rr.monsters, X: at.X, Y: at.Y, Fell: rr.fell && !rr.done}
}

// over reports whether the race is over for the room: it ended, or the
// game is out of the room.
func (rr *raceRun) over(st link.PlayState) bool {
	return st.Phase == link.PlayOff || st.Race == nil || st.Race.Number != rr.number
}

// updateRace reports the hero's progress, and ends the run when the race
// is over. It reports whether the crawl was replaced.
func (c *Crawl) updateRace(ctx *game.Context) bool {
	rr := c.run.race
	st := rr.play.State()
	if rr.over(st) {
		ctx.Replace(newRaceEnd(ctx, c.run, c.pos))
		return true
	}
	rr.play.Report(rr.report(c.run, c.pos))
	return false
}

// raceTitle is the side panel's title in a race: the floor and the time
// left.
func (c *Crawl) raceTitle(ctx *game.Context) string {
	rr := c.run.race
	s := fmt.Sprintf("Race · Floor %d of %d", c.run.depth, rr.goal)
	if rc := rr.play.State().Race; rc != nil {
		if left := time.Until(rc.EndsAt); left > 0 {
			s += fmt.Sprintf(" · %d:%02d", int(left.Minutes()), int(left.Seconds())%60)
		}
	}
	return s
}

// racerColours tell the other racers apart on the map.
var racerColours = []color.RGBA{pal.Cyan, pal.Orange, pal.Pink, pal.Lime}

// rival is another racer on the player's floor.
type rival struct {
	name string
	at   dungeon.Point
	col  color.RGBA
}

// rivals are the other racers on the hero's floor, still in the race.
func (c *Crawl) rivals() []rival {
	rr := c.run.race
	if rr == nil {
		return nil
	}
	st := rr.play.State()
	if st.Race == nil {
		return nil
	}
	var out []rival
	n := 0
	for _, r := range st.Race.Racers {
		if r.ID == rr.you {
			continue
		}
		col := racerColours[n%len(racerColours)]
		n++
		if r.Floor != c.run.depth || r.Status != link.RacerRacing {
			continue
		}
		out = append(out, rival{name: racerName(st.Room, r.ID), at: dungeon.Point{X: r.X, Y: r.Y}, col: col})
	}
	return out
}

// racerName is a racer's pseudonym.
func racerName(room link.Room, id string) string {
	for _, m := range room.Members {
		if m.ID == id && m.Name != "" {
			return m.Name
		}
	}
	return "A racer"
}

// finishRace is the hero reaching the goal floor.
func (c *Crawl) finishRace(ctx *game.Context) {
	rr := c.run.race
	rr.done = true
	c.play(audio.Stairs)
	ctx.Replace(newRaceEnd(ctx, c.run, dungeon.Point{}))
}

// RaceEnd is the race's end for the player: it waits for the others, and
// shows the results when the race is over.
type RaceEnd struct {
	bg   *ebiten.Image
	rr   *raceRun
	last race.Report // the player's final report, sent until it goes
}

func newRaceEnd(ctx *game.Context, r *run, at dungeon.Point) *RaceEnd {
	ctx.EndSession()
	rr := r.race
	last := rr.report(r, at)
	if rr.done {
		last.X, last.Y = 0, 0 // a new floor: any cell on it will do
	}
	return &RaceEnd{bg: backdrop(9, 1.2), rr: rr, last: last}
}

// Update implements game.Scene.
func (e *RaceEnd) Update(ctx *game.Context) error {
	st := e.rr.play.State()
	if !e.rr.over(st) {
		e.rr.play.Report(e.last)
	}
	// Back to the room once the server has the result, so the lobby
	// can't start the same race again.
	if (input.Confirm() || input.Back()) && e.settled(st) {
		ctx.Sound.Play(audio.Select)
		if st.Phase == link.PlayOff {
			ctx.Replace(NewLobby(ctx))
			return nil
		}
		ctx.Replace(&Lobby{bg: backdrop(7, 1.15), raced: e.rr.number})
	}
	return nil
}

// settled reports whether the player's race is over as the room sees it.
func (e *RaceEnd) settled(st link.PlayState) bool {
	if e.rr.over(st) {
		return true
	}
	me, ok := st.Race.Racer(e.rr.you)
	return !ok || me.Status != link.RacerRacing
}

// Draw implements game.Scene.
func (e *RaceEnd) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, e.bg, 0, 0)
	st := e.rr.play.State()
	title, col := "YOU FELL", pal.Rose
	if e.rr.done {
		title, col = fmt.Sprintf("FLOOR %d!", e.rr.goal), pal.Lime
	}
	f.DrawCentered(dst, title, cx, 8, 3, col)
	sub := fmt.Sprintf("You reached floor %d and beat %s.", e.last.Floor, plural(e.rr.monsters, "monster"))
	f.DrawCentered(dst, sub, cx, 44, 1, pal.Ice)

	const x, y, w, h = 70, 66, game.ScreenW - 140, 230
	gfx.Window(dst, x, y, w, h)
	hint := ""
	switch {
	case st.Phase == link.PlayOff:
		f.DrawCentered(dst, "You are out of the room.", cx, y+40, 1, pal.Tan)
		if st.Problem != "" {
			f.DrawCentered(dst, fit(f, st.Problem, w-24, 1), cx, y+60, 1, pal.Tan)
		}
		hint = "Enter continue"
	case st.Race == nil || st.Race.Number != e.rr.number:
		f.DrawShadow(dst, "Results", x+12, y+10, 1, pal.Yellow)
		e.drawResults(dst, ctx, st, x+12, y+32, w-24)
		hint = "Enter back to the room"
	default:
		f.DrawShadow(dst, "Waiting for the others…", x+12, y+10, 1, pal.Yellow)
		e.drawRacers(dst, ctx, st, x+12, y+32, w-24)
		if e.settled(st) {
			hint = "Enter back to the room"
		}
	}
	f.DrawShadow(dst, hint, 8, game.ScreenH-20, 1, pal.Ash)
}

// drawRacers lists the racers as they are now.
func (e *RaceEnd) drawRacers(dst *ebiten.Image, ctx *game.Context, st link.PlayState, x, y, w int) {
	f := ctx.Font
	for i, r := range st.Race.Racers {
		name := racerName(st.Room, r.ID)
		c := pal.Ice
		if r.ID == e.rr.you {
			name += " (you)"
			c = pal.White
		}
		ry := y + i*20
		f.DrawShadow(dst, fit(f, name, w/2, 1), x, ry, 1, c)
		f.DrawShadow(dst, fmt.Sprintf("floor %d · %s", r.Floor, statusText(r.Status)), x+w/2, ry, 1, pal.Steel)
	}
}

// drawResults lists the race's results by place.
func (e *RaceEnd) drawResults(dst *ebiten.Image, ctx *game.Context, st link.PlayState, x, y, w int) {
	f := ctx.Font
	if len(st.Results) == 0 {
		f.DrawShadow(dst, "The race ended.", x, y, 1, pal.Ice)
		return
	}
	for i, r := range st.Results {
		drawRaceResult(dst, ctx, r, r.ID == e.rr.you, x, y+i*20, w)
	}
}

// drawRaceResult draws one line of a race's results.
func drawRaceResult(dst *ebiten.Image, ctx *game.Context, r link.Result, you bool, x, y, w int) {
	f := ctx.Font
	place := "–"
	if r.Place > 0 {
		place = fmt.Sprintf("%d.", r.Place)
	}
	name := r.Name
	if name == "" {
		name = "A racer"
	}
	c := pal.Ice
	switch {
	case you:
		name += " (you)"
		c = pal.White
	case r.Place == 0:
		c = pal.Steel
	}
	if r.Place == 1 {
		c = pal.Yellow
	}
	f.DrawShadow(dst, place, x, y, 1, c)
	f.DrawShadow(dst, fit(f, name, w/2-24, 1), x+24, y, 1, c)
	f.DrawShadow(dst, fit(f, resultText(r), w/2, 1), x+w/2, y, 1, pal.Steel)
}

// resultText says how far a racer got.
func resultText(r link.Result) string {
	switch r.Status {
	case link.RacerFinished:
		return fmt.Sprintf("floor %d in %d:%02d", r.Floor, int(r.Time.Minutes()), int(r.Time.Seconds())%60)
	case link.RacerLeft, link.RacerNotCounted:
		return statusText(r.Status)
	}
	return fmt.Sprintf("floor %d · %s · %s", r.Floor, plural(r.Monsters, "monster"), statusText(r.Status))
}

// statusText is a racer's status in words.
func statusText(s string) string {
	switch s {
	case link.RacerRacing:
		return "still going"
	case link.RacerFinished:
		return "finished"
	case link.RacerFell:
		return "fell"
	case link.RacerLeft:
		return "left"
	case link.RacerNotCounted:
		return "not counted"
	}
	return ""
}
