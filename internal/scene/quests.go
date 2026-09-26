package scene

import (
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// questRun is the quest an adventure was started for: it plays the
// quest's list only.
type questRun struct {
	ID    string // the assignment
	List  string // the list's server ID
	Title string // the list's title, which names the quest
}

// questNow is the clock the quest screens use; tests set it.
var questNow = time.Now

// questList finds a quest's word list (by its server ID) among the lists
// the game has: the assigned copy, or the player's own copy of a list no
// longer assigned. It is nil when the game doesn't have it (yet).
func questList(ctx *game.Context, id string) *words.List {
	if id == "" {
		return nil
	}
	var own *words.List
	for _, l := range ctx.Lists {
		if l.ID != id {
			continue
		}
		if link.IsAssigned(l) {
			return l
		}
		if own == nil {
			own = l
		}
	}
	return own
}

// questFor finds the quest with an ID, as the server last sent it.
func questFor(ctx *game.Context, id string) (link.Quest, bool) {
	for _, q := range ctx.Link.Quests() {
		if q.ID == id {
			return q, true
		}
	}
	return link.Quest{}, false
}

// Quests shows the quests a grown-up set (the assignments on the website)
// with their progress and due dates, and starts one. At a campfire it only
// shows them.
type Quests struct {
	bg     *ebiten.Image
	quests []link.Quest
	sel    int
	top    int // the first row shown
	way    int // which of the chosen quest's PlayModes is picked
	look   bool
	msg    string
}

// questRows is how many quests fit on the screen at once.
const questRows = 5

// NewQuests creates the Quests screen, opened from the title screen.
func NewQuests(ctx *game.Context) game.Scene {
	q := &Quests{bg: backdrop(12, 1.3)}
	q.refresh(ctx)
	return q
}

// NewQuestsLook creates the Quests screen for a look at them from a
// campfire: it can't start one, and Esc goes back to the campfire.
func NewQuestsLook(ctx *game.Context) game.Scene {
	q := &Quests{bg: backdrop(12, 1.3), look: true}
	q.refresh(ctx)
	return q
}

// refresh picks up quests a sync brought in, keeping the one chosen.
func (q *Quests) refresh(ctx *game.Context) {
	id := ""
	if q.sel < len(q.quests) {
		id = q.quests[q.sel].ID
	}
	q.quests = link.SortQuests(ctx.Link.Quests(), questNow())
	q.sel = min(q.sel, max(0, len(q.quests)-1))
	for i, qu := range q.quests {
		if qu.ID == id {
			q.sel = i
		}
	}
	q.way = min(q.way, len(q.ways())-1)
}

// ways are the chosen quest's ways to play.
func (q *Quests) ways() []string {
	if q.sel >= len(q.quests) {
		return []string{link.ModePractice}
	}
	return q.quests[q.sel].PlayModes()
}

func (q *Quests) move(ctx *game.Context, d int) {
	n := len(q.quests)
	if n == 0 {
		return
	}
	ctx.Sound.Play(audio.Blip)
	q.sel = (q.sel + d + n) % n
	q.way, q.msg = 0, ""
	q.top = max(min(q.top, q.sel), q.sel-questRows+1)
}

// Update implements game.Scene.
func (q *Quests) Update(ctx *game.Context) error {
	q.refresh(ctx)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		if q.look {
			ctx.Pop()
		} else {
			ctx.Replace(NewTitle(ctx))
		}
	case input.Up():
		q.move(ctx, -1)
	case input.Down():
		q.move(ctx, 1)
	case q.look || len(q.quests) == 0:
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyArrowRight):
		if len(q.ways()) > 1 {
			ctx.Sound.Play(audio.Blip)
			q.way = 1 - q.way
		}
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		if s := q.start(ctx); s != nil {
			ctx.Sound.Play(audio.Select)
			ctx.Replace(s)
			return nil
		}
		ctx.Sound.Play(audio.Wrong)
	}
	return nil
}

// start is the scene that plays the chosen quest in the chosen way, or
// nil when it can't be played (q.msg says why).
func (q *Quests) start(ctx *game.Context) game.Scene {
	qu := q.quests[q.sel]
	l := questList(ctx, qu.List.ID)
	if l == nil || len(l.Entries) == 0 {
		q.msg = "Its word list hasn't arrived yet. Try Sync now in Account."
		return nil
	}
	lang, ok := words.Lookup(l.Language)
	if !ok {
		q.msg = "This game can't play that list's language."
		return nil
	}
	if q.ways()[q.way] == link.ModeAdventure {
		return NewClassPick(lang, runSetup{mode: compete.Adventure,
			quest: &questRun{ID: qu.ID, List: qu.List.ID, Title: questName(qu)}})
	}
	return NewQuestPractice(ctx, qu, l)
}

// questName is what a quest is called: its list's title.
func questName(q link.Quest) string {
	if q.List.Title != "" {
		return q.List.Title
	}
	return "A word list"
}

// Draw implements game.Scene.
func (q *Quests) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, q.bg, 0, 0)
	f.DrawCentered(dst, "Quests", cx, 12, 3, pal.Yellow)
	f.DrawCentered(dst, "Word lists a grown-up gave you, with how far you've got.", cx, 64, 1, pal.Tan)

	help := "↑/↓ choose   Enter play   Esc back"
	switch {
	case q.look:
		help = "↑/↓ choose   Esc back to the fire"
	case len(q.ways()) > 1:
		help = "↑/↓ choose   ←/→ Practice or Adventure   Enter play   Esc back"
	}
	f.DrawShadow(dst, help, 8, game.ScreenH-20, 1, pal.Ash)

	const x, y, w = 32, 86, game.ScreenW - 64
	if len(q.quests) == 0 {
		gfx.Window(dst, x, y, w, 80)
		lines := [2]string{"No quests right now.", "When a grown-up gives you a word list on the website, it shows here."}
		if !ctx.Link.Linked() {
			lines = [2]string{"This game isn't linked to a grown-up's account.", "Link it in Account on the title screen to get quests."}
		}
		f.DrawCentered(dst, lines[0], cx, y+20, 1, pal.White)
		f.DrawCentered(dst, lines[1], cx, y+44, 1, pal.Ice)
		return
	}
	const rowH = 46
	shown := min(questRows, len(q.quests)-q.top)
	gfx.Window(dst, x, y, w, rowH*shown+12)
	now := questNow()
	for i := range shown {
		k := q.top + i
		qu := q.quests[k]
		ry := y + 6 + i*rowH
		sel := k == q.sel
		if sel {
			gfx.FillRect(dst, x+4, ry, w-8, rowH-2, pal.Fade(pal.Indigo, 0.8))
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+10, ry+4, 1, pal.Yellow)
			}
		}
		way := ""
		if sel && !q.look {
			way = q.wayText()
		}
		drawQuest(dst, ctx, qu, x+26, ry+4, w-40, now, sel, way)
	}
	if q.top > 0 {
		f.DrawCentered(dst, "▲", cx, y-14, 1, pal.Steel)
	}
	if q.top+shown < len(q.quests) {
		f.DrawCentered(dst, "▼", cx, y+rowH*shown+14, 1, pal.Steel)
	}
	if q.msg != "" {
		f.DrawCentered(dst, q.msg, cx, game.ScreenH-40, 1, pal.Rose)
	}
}

// wayText shows the way the chosen quest will be played, with arrows when
// there is a choice.
func (q *Quests) wayText() string {
	ways := q.ways()
	name := map[string]string{link.ModePractice: "Practice", link.ModeAdventure: "Adventure"}[ways[q.way]]
	if len(ways) > 1 {
		return "◄ " + name + " ►"
	}
	return name
}

// drawQuest draws one quest in two lines at x, y, w wide: its name, goal
// and due date, then its bar and progress, and way (how Enter plays it)
// on the right when not "".
func drawQuest(dst *ebiten.Image, ctx *game.Context, qu link.Quest, x, y, w int, now time.Time, sel bool, way string) {
	f := ctx.Font
	nameCol, goalCol := pal.Steel, pal.Ash
	if sel {
		nameCol, goalCol = pal.White, pal.Ice
	}
	when := qu.WhenText(now)
	whenCol := pal.Ash
	switch {
	case qu.Late(now):
		whenCol = pal.Rose
	case qu.Due().Sub(now) < 48*time.Hour && !qu.Complete() && !qu.Due().IsZero():
		whenCol = pal.Orange
	case !qu.Started(now):
		whenCol = pal.Sky
	}
	ww := f.Width(when, 1)
	f.DrawShadow(dst, when, x+w-ww, y, 1, whenCol)
	name := fit(f, questName(qu), w/2-20, 1)
	f.DrawShadow(dst, name, x, y, 1, nameCol)
	nx := x + f.Width(name, 1)
	f.DrawShadow(dst, fit(f, " · "+qu.GoalText(), x+w-ww-12-nx, 1), nx, y, 1, goalCol)

	by := y + 20
	progCol := pal.Ice
	if qu.Complete() {
		progCol = pal.Lime
	}
	px := x
	if qu.HasBar() {
		fg := questBarColor(qu)
		bar(dst, x, by+3, 180, 10, qu.Fraction(), fg, pal.Night)
		px = x + 190
	}
	f.DrawShadow(dst, qu.ProgressText(), px, by, 1, progCol)
	if way != "" {
		f.DrawShadow(dst, way, x+w-f.Width(way, 1), by, 1, pal.Yellow)
	}
}

func questBarColor(qu link.Quest) color.RGBA {
	if qu.Complete() {
		return pal.Lime
	}
	return pal.Sky
}

// drawQuestBanner draws the quest to do next at the top of the title
// screen, or a word that they are all done.
func drawQuestBanner(dst *ebiten.Image, ctx *game.Context, qs []link.Quest) {
	if len(qs) == 0 {
		return
	}
	f := ctx.Font
	now := questNow()
	qu, ok := link.Current(qs, now)
	if !ok {
		text := "✦ Every quest done. Well done! ✦"
		w := f.Width(text, 1) + 24
		gfx.Window(dst, game.ScreenW/2-w/2, 4, w, 26)
		f.DrawCentered(dst, text, game.ScreenW/2, 9, 1, pal.Lime)
		return
	}
	head := "✦ Quest: "
	name := fit(f, questName(qu), 150, 1)
	goal := " · " + qu.GoalText()
	prog := qu.ProgressText()
	when := qu.WhenText(now)
	if when != "" {
		when = " · " + when
	}
	barW := 0
	if qu.HasBar() {
		barW = 70
	}
	w := f.Width(head+name+goal, 1) + barW + f.Width(prog+when, 1) + 32
	if w > game.ScreenW-16 {
		goal = ""
		w = f.Width(head+name, 1) + barW + f.Width(prog+when, 1) + 32
	}
	x := game.ScreenW/2 - w/2
	gfx.Window(dst, x, 4, w, 26)
	tx := x + 12
	f.DrawShadow(dst, head, tx, 9, 1, pal.Yellow)
	tx += f.Width(head, 1)
	f.DrawShadow(dst, name, tx, 9, 1, pal.White)
	tx += f.Width(name, 1)
	f.DrawShadow(dst, goal, tx, 9, 1, pal.Ice)
	tx += f.Width(goal, 1) + 8
	if barW > 0 {
		bar(dst, tx, 12, barW-8, 10, qu.Fraction(), questBarColor(qu), pal.Night)
		tx += barW
	}
	f.DrawShadow(dst, prog, tx, 9, 1, pal.Ice)
	tx += f.Width(prog, 1)
	whenCol := pal.Ash
	if qu.Late(now) {
		whenCol = pal.Rose
	}
	f.DrawShadow(dst, when, tx, 9, 1, whenCol)
}
