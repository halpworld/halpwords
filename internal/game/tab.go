package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/browser"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/internal/unifont"
)

// Starter runs the game, or in a web game open in another tab, waits for
// that tab to close first (halpwords#111). Two tabs share one local
// storage: a second one writing it would fork a suspended run and write
// one tab's old link tokens over the other's. Until this tab holds the
// lock, nothing is read or written: the screen only says why, and the
// game starts by itself once the other tab closes. Outside a browser it
// is New straight away.
type Starter struct {
	first func(*Context) Scene
	// hold tries to take the tab's lock (browser.HoldTab); start makes
	// the game (New).
	hold  func() bool
	start func(func(*Context) Scene) (*Game, error)

	g      *Game
	font   *gfx.Font
	trying chan bool // a hold on its way
	tick   int
}

// holdEvery is how often, in updates, a waiting tab tries the lock again.
const holdEvery = 60

// Start is New for first, or a Starter that waits for the other tab. A
// game whose files are kept in memory (a play-test) needs no lock.
func Start(first func(*Context) Scene) (*Starter, error) {
	return start(first, browser.HoldTab, New)
}

func start(first func(*Context) Scene, hold func() bool, mk func(func(*Context) Scene) (*Game, error)) (*Starter, error) {
	s := &Starter{first: first, hold: hold, start: mk}
	if save.InMemory() || hold() {
		g, err := mk(first)
		if err != nil {
			return nil, err
		}
		s.g = g
		return s, nil
	}
	face, err := unifont.ParseBytes(assets.UnifontHex)
	if err != nil {
		return nil, err
	}
	s.font = gfx.NewFont(face)
	return s, nil
}

// Waiting reports whether the game waits for another tab.
func (s *Starter) Waiting() bool { return s.g == nil }

// Update implements ebiten.Game.
func (s *Starter) Update() error {
	if s.g != nil {
		return s.g.Update()
	}
	if s.trying != nil {
		select {
		case held := <-s.trying:
			s.trying = nil
			if held {
				g, err := s.start(s.first)
				if err != nil {
					return err
				}
				s.g = g
				return nil
			}
		default:
		}
	}
	s.tick++
	if s.trying == nil && s.tick%holdEvery == 0 {
		out := make(chan bool, 1)
		s.trying = out
		go func() { out <- s.hold() }()
	}
	return nil
}

// tabLines are what a waiting tab says.
var tabLines = []string{
	"Halpwords is open in another tab.",
	"Play there, or close it:",
	"this tab then starts by itself.",
}

// Draw implements ebiten.Game.
func (s *Starter) Draw(screen *ebiten.Image) {
	if s.g != nil {
		s.g.Draw(screen)
		return
	}
	screen.Fill(color.Black)
	s.font.DrawCentered(screen, "Halpwords", ScreenW/2, 100, 3, pal.Yellow)
	for i, l := range tabLines {
		c := pal.Ice
		if i == 0 {
			c = pal.White
		}
		s.font.DrawCentered(screen, l, ScreenW/2, 170+i*20, 1, c)
	}
}

// Layout implements ebiten.Game.
func (s *Starter) Layout(w, h int) (int, int) { return ScreenW, ScreenH }

// DrawFinalScreen scales as Game does.
func (s *Starter) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, m ebiten.GeoM) {
	if s.g != nil {
		s.g.DrawFinalScreen(screen, offscreen, m)
		return
	}
	screen.Fill(color.Black)
	sw, sh := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())
	scale := math.Min(sw/ScreenW, sh/ScreenH)
	if scale >= 1 {
		scale = math.Floor(scale)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(math.Floor((sw-ScreenW*scale)/2), math.Floor((sh-ScreenH*scale)/2))
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(offscreen, op)
}

// Close closes the game, if it started: a tab that only waited has
// nothing to save.
func (s *Starter) Close() {
	if s.g != nil {
		s.g.Close()
	}
}
