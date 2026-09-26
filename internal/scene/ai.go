package scene

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/llm"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
	"github.com/halpworld/halpwords/pkg/puzzle"
	"github.com/halpworld/halpwords/pkg/words"
)

// runAI is what an adventure asks the AI for. Every request runs in the
// background and nothing waits for it: whatever has arrived is used, and
// the game carries on with its own content otherwise. Nothing here uses
// the run's random numbers, so a seed plays the same with or without AI.
type runAI struct {
	svc *llm.Service
	rng *rand.Rand // for choosing taunts, apart from the run's rng

	// scripts are the Dungeon Director's floor scripts, by depth.
	scripts   map[int]*llm.Script
	directing map[int]*llm.Job[*llm.Script]
	tries     map[int]int // requests made for each floor's script

	// work is the content request in flight: words, taunts or tips. One
	// runs at a time.
	work     *llm.Job[int]
	wordsFor int  // the floor words were last asked for
	taunted  bool // taunts have been asked for on this run
	tips     []int
	misses   map[int]int // times each word was missed on this run
	fails    int         // requests that failed in a row

	gen *puzzle.Generated // generated puzzles, when they can be used
}

// maxFails is how many requests in a row may fail before the run stops
// asking for more. maxTries is how many times a floor's script is asked
// for.
const (
	maxFails = 4
	maxTries = 2
)

func newRunAI(ctx *game.Context) *runAI {
	return &runAI{
		svc:       ctx.AI,
		rng:       rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 7)),
		scripts:   map[int]*llm.Script{},
		directing: map[int]*llm.Job[*llm.Script]{},
		tries:     map[int]int{},
		misses:    map[int]int{},
		wordsFor:  -1,
	}
}

// on reports whether the AI can be asked for things now.
func (a *runAI) on() bool { return a != nil && a.svc.Ready() && a.fails < maxFails }

// bank is the generated content for the run's language.
func (a *runAI) bank(r *run) *llm.Bank {
	if a == nil || a.svc == nil {
		return nil
	}
	return a.svc.Bank(r.lang.Code)
}

// generated returns the puzzles beyond the word lists' own words for the
// run: the lists' gap-fill sentences, and what the AI wrote when it is on.
// It is nil when there are none. Scored runs use the fixed puzzles only, so
// their scores compare.
func (a *runAI) generated(r *run) *puzzle.Generated {
	if r.hardcore() {
		return nil
	}
	if a == nil || !a.svc.Ready() {
		return r.cloze
	}
	if a.gen == nil {
		b := a.bank(r)
		g := &puzzle.Generated{Riddles: b.AllRiddles(), Cloze: map[string][]puzzle.ClozeLine{}}
		for k, cs := range b.AllCloze() {
			for _, c := range cs {
				g.Cloze[k] = append(g.Cloze[k], puzzle.ClozeLine{Text: c.Text, English: c.English})
			}
		}
		a.gen = g.Add(r.cloze)
	}
	return a.gen
}

// floorWords are the words to build a floor's content around: the ones the
// player most needs, then the ones due for practice, then the rest.
func floorWords(r *run, n int) []words.Entry {
	entries := r.deck.Entries()
	var out []words.Entry
	have := map[int]bool{}
	add := func(id int) {
		if !have[id] && len(out) < n {
			have[id] = true
			out = append(out, entries[id])
		}
	}
	for _, id := range r.deck.Missed() {
		add(id)
	}
	if mem := r.deck.Memory(); mem != nil {
		for _, id := range mem.Weakest(entries, n) {
			add(id)
		}
		for id, e := range entries {
			if mem.IsDue(e) {
				add(id)
			}
		}
	}
	for _, id := range rand.New(rand.NewPCG(uint64(r.depth), r.seed)).Perm(len(entries)) {
		add(id)
	}
	return out
}

// direct asks the Dungeon Director for the script of floor depth, unless it
// has one or is working on it.
func (a *runAI) direct(r *run, depth int) {
	if !a.on() || r.quest != nil || a.scripts[depth] != nil || a.directing[depth] != nil || a.tries[depth] >= maxTries {
		return
	}
	a.tries[depth]++
	l := r.floor(depth)
	f := llm.Floor{Lang: r.lang, Depth: depth, Words: floorWords(r, 12)}
	for _, t := range proc.Themes {
		f.Themes = append(f.Themes, t.Name)
	}
	seen := map[string]bool{}
	for _, m := range l.Monsters {
		if !m.Kind.Boss() && !seen[m.Kind.Name] {
			seen[m.Kind.Name] = true
			f.Monsters = append(f.Monsters, m.Kind.Name)
		}
	}
	if b := l.Boss(); b != nil {
		f.Boss = b.Kind.Name
	}
	svc := a.svc
	a.directing[depth] = llm.Start(func() (*llm.Script, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		return svc.Direct(ctx, f)
	})
}

// startFloor asks for what the floor the hero has just reached needs, and
// the next floor's script, so it is ready on arrival.
func (a *runAI) startFloor(r *run) {
	if !a.on() {
		return
	}
	a.direct(r, r.depth)
	a.direct(r, r.depth+1)
	for _, id := range r.deck.Missed() {
		a.queueTip(id)
	}
}

// queueTipIf asks for a memory tip for word id, if the AI is on.
func (a *runAI) queueTipIf(id int) {
	if a != nil {
		a.queueTip(id)
	}
}

func (a *runAI) queueTip(id int) {
	for _, t := range a.tips {
		if t == id {
			return
		}
	}
	a.tips = append(a.tips, id)
}

// missed notes a word the hero got wrong. A word missed twice gets a memory
// tip for the next campfire.
func (a *runAI) missed(id int) {
	if a == nil || id < 0 {
		return
	}
	a.misses[id]++
	if a.misses[id] == 2 {
		a.queueTip(id)
	}
}

// poll collects finished requests and starts the next one. The crawl calls
// it every tick.
func (a *runAI) poll(c *Crawl) {
	if a == nil {
		return
	}
	r := c.run
	for depth, j := range a.directing {
		if !j.Done() || (depth == r.depth && c.mode == modeBattle) {
			continue
		}
		delete(a.directing, depth)
		sc, err := j.Result()
		a.note(err)
		if err == nil && sc != nil {
			a.scripts[depth] = sc
			if depth == r.depth {
				c.lateScript(sc)
			}
		}
	}
	if a.work.Done() {
		_, err := a.work.Result()
		a.work = nil
		a.note(err)
		a.gen = nil // take in any new puzzles
	}
	if !a.on() {
		return
	}
	// Ask again for a script that failed, while it can still be used.
	a.direct(r, r.depth)
	a.direct(r, r.depth+1)
	if a.work != nil {
		return
	}
	svc, lang := a.svc, r.lang
	start := func(fn func(context.Context) (int, error)) {
		a.work = llm.Start(func() (int, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			return fn(ctx)
		})
	}
	switch {
	case a.wordsFor != r.depth && !r.hardcore():
		a.wordsFor = r.depth
		ws := floorWords(r, 16)
		start(func(ctx context.Context) (int, error) { return svc.FillWords(ctx, lang, ws) })
	case !a.taunted && a.bank(r).TauntCount() < 16:
		a.taunted = true
		ws := floorWords(r, 20)
		start(func(ctx context.Context) (int, error) { return svc.FillTaunts(ctx, lang, ws) })
	case len(a.tips) > 0:
		var ws []words.Entry
		for _, id := range a.tips {
			ws = append(ws, r.deck.Entries()[id])
		}
		a.tips = nil
		start(func(ctx context.Context) (int, error) { return svc.FillTips(ctx, lang, ws) })
	}
}

// note counts failed requests in a row, so a run stops asking when the AI
// keeps failing, such as with no internet.
func (a *runAI) note(err error) {
	if err == nil {
		a.fails = 0
	} else {
		a.fails++
	}
}

// taunt returns a battle cry for a monster to shout, half the time.
func (a *runAI) taunt(r *run) (llm.Taunt, bool) {
	if a == nil || !a.svc.Ready() || a.rng.IntN(2) == 0 {
		return llm.Taunt{}, false
	}
	return a.bank(r).Taunt(a.rng)
}

// tipFor returns the memory tip for word id, if the AI wrote one.
func (a *runAI) tipFor(r *run, id int) (string, bool) {
	if a == nil || a.svc == nil {
		return "", false
	}
	return a.bank(r).Tip(r.deck.Entries()[id])
}

// script returns the Dungeon Director's script for the floor the run is
// on, or nil.
func (r *run) script() *llm.Script {
	if r.ai == nil || r.quest != nil {
		return nil
	}
	return r.ai.scripts[r.depth]
}

// themeFor is the look of the run's current floor: the Director's choice,
// or the usual one for the depth.
func (r *run) themeFor() *proc.Theme {
	if m := r.questMap(r.depth); m != nil {
		if i := m.ThemeIndex(); i >= 0 && i < len(proc.Themes) {
			return &proc.Themes[i]
		}
		return proc.ThemeFor(m.Level())
	}
	if sc := r.script(); sc != nil && sc.Theme >= 0 && sc.Theme < len(proc.Themes) {
		return &proc.Themes[sc.Theme]
	}
	return proc.ThemeFor(r.depth)
}

// floorName is the name the floor's banner shows.
func (c *Crawl) floorName() string {
	if m := c.run.questMap(c.run.depth); m != nil {
		return m.Title
	}
	if sc := c.run.script(); sc != nil {
		return sc.Name
	}
	return c.theme.Name
}

// dress gives the floor's monsters the names in the Director's script.
func dress(l *dungeon.Level, sc *llm.Script) {
	if sc == nil {
		return
	}
	for _, m := range l.Monsters {
		switch {
		case m.Kind.Boss() && sc.Boss != "":
			m.Title = sc.Boss
		case sc.Names[m.Kind.Name] != "":
			m.Title = sc.Names[m.Kind.Name]
		}
	}
}

// arrive tells the hero about the floor, with the Director's words if there
// is a script.
func (c *Crawl) arrive() {
	r := c.run
	sc := r.script()
	if r.quest != nil {
		r.say(fmt.Sprintf("Floor %d of %d: %s. Find the stairs down!", r.depth, len(r.quest.Maps), c.floorName()), pal.Yellow)
		c.lore = nil
		c.readNote()
		return
	}
	r.say(fmt.Sprintf("Floor %d: %s. Find the stairs down!", r.depth, c.floorName()), pal.Yellow)
	if sc != nil && sc.Intro != "" {
		r.say(sc.Intro, pal.Cyan)
	}
	c.lore = nil
	if sc != nil {
		c.lore = append(c.lore, sc.Lore...)
	}
}

// lateScript uses a floor script that arrived after the hero did: the
// monsters get their names and the floor its name and notes. The floor
// keeps its look.
func (c *Crawl) lateScript(sc *llm.Script) {
	dress(c.level, sc)
	c.lore = append([]string(nil), sc.Lore...)
	c.showBanner(fmt.Sprintf("Floor %d", c.run.depth), sc.Name)
	c.run.say("The dungeon stirs. This is "+sc.Name+".", pal.Yellow)
	if sc.Intro != "" {
		c.run.say(sc.Intro, pal.Cyan)
	}
}

// loreEvery is how many steps pass between notes found on the walls.
const loreEvery = 30

// walked counts a step, and now and then finds a note on the wall.
func (c *Crawl) walked() {
	c.steps++
	if len(c.lore) > 0 && c.steps%loreEvery == 0 {
		c.run.say("Scratched on the wall: “"+c.lore[0]+"”", pal.Tan)
		c.lore = c.lore[1:]
	}
}
