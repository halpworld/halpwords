package scene

import (
	"math"
	"runtime"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
)

// Title is the title screen and main menu.
type Title struct {
	bg      *ebiten.Image
	torches []*gfx.Torch
	sel     int
	items   []titleItem
	saved   string // describes the saved adventure, if there is one
	hasSave bool
	// assigns are the assignment quests a grown-up set, when the game is
	// linked.
	assigns []link.Quest
}

// titleItem is an entry in the main menu.
type titleItem int

const (
	titleContinue titleItem = iota
	titleNew
	titlePractice
	titleTogether
	titleGrimoire
	titleFame
	titleWordLists
	titleSettings
	titleAI
	titleAccount
	titleQuit
	titleAssignments
)

var titleLabels = [...]string{"Continue", "New Adventure", "Practice", "Play Together", "Grimoire", "Hall of Fame", "Word Lists", "Settings", "AI Helper", "Account", "Quit", "Assignments"}

// NewTitle creates the title screen.
func NewTitle(ctx *game.Context) game.Scene {
	ctx.EndSession()
	t := &Title{
		bg:      backdrop(1, 1.1),
		torches: []*gfx.Torch{gfx.NewTorch(96, 150, 1), gfx.NewTorch(game.ScreenW-96, 150, 2)},
	}
	t.saved, t.hasSave = saveSummary()
	t.build(ctx)
	return t
}

// build makes the menu: Assignments comes first when there are
// assignment quests, after Continue. The same entry stays chosen.
func (t *Title) build(ctx *game.Context) {
	var was titleItem = -1
	if t.sel < len(t.items) {
		was = t.items[t.sel]
	}
	t.assigns = ctx.Link.Quests()
	t.items = t.items[:0]
	if t.hasSave {
		t.items = append(t.items, titleContinue)
	}
	if len(t.assigns) > 0 {
		t.items = append(t.items, titleAssignments)
	}
	t.items = append(t.items, titleNew, titlePractice, titleTogether, titleGrimoire, titleFame, titleWordLists, titleSettings, titleAI, titleAccount)
	if runtime.GOOS != "js" {
		t.items = append(t.items, titleQuit) // a web page is closed, not quit
	}
	t.sel = 0
	for i, it := range t.items {
		if it == was {
			t.sel = i
		}
	}
}

// rows is how many items are in each of the menu's two columns.
func (t *Title) rows() int { return (len(t.items) + 1) / 2 }

// Update implements game.Scene.
func (t *Title) Update(ctx *game.Context) error {
	for _, tr := range t.torches {
		tr.Update()
	}
	if d := droppedQuests(); len(d) > 0 {
		ctx.Replace(NewQuests(ctx, d...))
		return nil
	}
	if qs := ctx.Link.Quests(); !slices.EqualFunc(qs, t.assigns, func(a, b link.Quest) bool { return a == b }) {
		t.build(ctx) // a sync brought new assignments
	}
	n := len(t.items)
	switch {
	case input.Pressed(ebiten.KeyTab) && ctx.Learners != nil:
		ctx.Sound.Play(audio.Select)
		ctx.Replace(NewLearners(ctx))
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		t.sel = (t.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		t.sel = (t.sel + 1) % n
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyA):
		if t.sel >= t.rows() {
			ctx.Sound.Play(audio.Blip)
			t.sel -= t.rows()
		}
	case input.Repeat(ebiten.KeyArrowRight) || input.Repeat(ebiten.KeyD):
		if t.sel < t.rows() {
			ctx.Sound.Play(audio.Blip)
			t.sel = min(n-1, t.sel+t.rows())
		}
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		it := t.items[t.sel]
		if it == titleQuit {
			return ebiten.Termination
		}
		if it == titleContinue {
			c, err := loadCrawl(ctx)
			if err != nil {
				ctx.Sound.Play(audio.Wrong)
				ctx.Notify("Can't continue: " + err.Error())
				return nil
			}
			ctx.Sound.Play(audio.Select)
			ctx.Replace(c)
			return nil
		}
		ctx.Sound.Play(audio.Select)
		ctx.Replace(map[titleItem]func(*game.Context) game.Scene{
			titleNew:         NewNewGame,
			titlePractice:    NewPractice,
			titleTogether:    NewLobby,
			titleGrimoire:    func(ctx *game.Context) game.Scene { return NewGrimoire(ctx, nil) },
			titleFame:        NewHallOfFame,
			titleWordLists:   NewWordLists,
			titleSettings:    NewSettings,
			titleAI:          NewAISetup,
			titleAccount:     NewAccount,
			titleAssignments: NewAssignments,
		}[it](ctx))
	}
	return nil
}

// Draw implements game.Scene.
func (t *Title) Draw(dst *ebiten.Image, ctx *game.Context) {
	gfx.DrawArt(dst, t.bg, 0, 0)
	for _, tr := range t.torches {
		tr.Draw(dst)
	}

	// Logo: each letter bobs on its own phase.
	const logo = "HALPWORDS"
	const scale = 5
	f := ctx.Font
	x := game.ScreenW/2 - f.Width(logo, scale)/2
	for i, r := range logo {
		s := string(r)
		dy := int(math.Round(math.Sin(float64(ctx.Tick)*0.05+float64(i)*0.6) * 4))
		f.DrawOutline(dst, s, x+scale, 40+dy+scale, scale, pal.Mahogany, pal.Mahogany) // drop shadow
		f.DrawOutline(dst, s, x, 40+dy, scale, pal.Yellow, pal.Black)
		x += f.Width(s, scale)
	}
	f.DrawCentered(dst, "~ A Dungeon of Words ~", game.ScreenW/2, 132, 2, pal.Tan)
	// Menu, in two columns. Five rows sit a little closer and higher.
	rows := t.rows()
	step, my, savedY := 26, 196, 172
	switch {
	case rows > 5:
		step, my, savedY = 21, 180, 162
	case rows > 4:
		step, my, savedY = 24, 186, 166
	}
	if t.saved != "" {
		f.DrawCentered(dst, "Saved: "+t.saved, game.ScreenW/2, savedY, 1, pal.Ice)
	}
	colW := 0
	for _, it := range t.items {
		colW = max(colW, f.Width(titleLabels[it], 2))
	}
	colW += 60
	mw, mh := colW*2+16, step*rows+20
	mx := game.ScreenW/2 - mw/2
	gfx.Window(dst, mx, my, mw, mh)
	for i, it := range t.items {
		x, y := mx+8+(i/rows)*colW, my+10+(i%rows)*step
		c := pal.Steel
		if i == t.sel {
			c = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+10, y, 2, pal.Yellow)
			}
		}
		f.DrawShadow(dst, titleLabels[it], x+36, y, 2, c)
	}

	drawAssignBanner(dst, ctx, t.assigns)
	drawLearnerBadge(dst, ctx)

	f.DrawShadow(dst, "Arrows choose   Enter select", 8, game.ScreenH-20, 1, pal.Ash)
	v := game.VersionText()
	f.DrawShadow(dst, v, game.ScreenW-8-f.Width(v, 1), game.ScreenH-20, 1, pal.Ash)
	if ctx.AI.Ready() {
		on := "✦ AI on"
		f.DrawShadow(dst, on, game.ScreenW/2-f.Width(on, 1)/2, game.ScreenH-20, 1, pal.Lime)
	}
}

// drawLearnerBadge shows who is playing, top left, and how to switch
// (W2.5): on a shared computer the next child must see it isn't them.
func drawLearnerBadge(dst *ebiten.Image, ctx *game.Context) {
	l := ctx.Learner()
	if l == nil {
		return
	}
	f := ctx.Font
	f.DrawShadow(dst, fit(f, l.Name, 150, 1), 8, 6, 1, pal.White)
	f.DrawShadow(dst, "Not you? Press Tab", 8, 20, 1, pal.Ash)
}
