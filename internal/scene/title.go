package scene

import (
	"math"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
)

// Title is the title screen and main menu.
type Title struct {
	bg      *ebiten.Image
	torches []*gfx.Torch
	sel     int
	items   []titleItem
	saved   string // describes the saved adventure, if there is one
}

// titleItem is an entry in the main menu.
type titleItem int

const (
	titleContinue titleItem = iota
	titleNew
	titlePractice
	titleGrimoire
	titleFame
	titleWordLists
	titleSettings
	titleAI
	titleQuit
)

var titleLabels = [...]string{"Continue", "New Adventure", "Practice", "Grimoire", "Hall of Fame", "Word Lists", "Settings", "AI Helper", "Quit"}

// NewTitle creates the title screen.
func NewTitle(ctx *game.Context) game.Scene {
	t := &Title{
		bg:      backdrop(1, 1.1),
		torches: []*gfx.Torch{gfx.NewTorch(96, 150, 1), gfx.NewTorch(game.ScreenW-96, 150, 2)},
		items:   []titleItem{titleNew, titlePractice, titleGrimoire, titleFame, titleWordLists, titleSettings, titleAI},
	}
	if runtime.GOOS != "js" {
		t.items = append(t.items, titleQuit) // a web page is closed, not quit
	}
	if s, ok := saveSummary(); ok {
		t.saved = s
		t.items = append([]titleItem{titleContinue}, t.items...)
	}
	return t
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
	n := len(t.items)
	switch {
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
			titleNew:       NewNewGame,
			titlePractice:  NewPractice,
			titleGrimoire:  func(ctx *game.Context) game.Scene { return NewGrimoire(ctx, nil) },
			titleFame:      NewHallOfFame,
			titleWordLists: NewWordLists,
			titleSettings:  NewSettings,
			titleAI:        NewAISetup,
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
	if rows > 4 {
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

	f.DrawShadow(dst, "Arrows choose   Enter select", 8, game.ScreenH-20, 1, pal.Ash)
	v := game.VersionText()
	f.DrawShadow(dst, v, game.ScreenW-8-f.Width(v, 1), game.ScreenH-20, 1, pal.Ash)
	if ctx.AI.Ready() {
		on := "✦ AI on"
		f.DrawShadow(dst, on, game.ScreenW/2-f.Width(on, 1)/2, game.ScreenH-20, 1, pal.Lime)
	}
}
