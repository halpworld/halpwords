package game

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/pal"
)

const fadeStep = 1.0 / 10 // ten ticks to fade out, ten to fade in

// manager is the scene stack with fade-to-black transitions.
type manager struct {
	stack   []Scene
	pending func()
	fade    float64 // 0 = clear, 1 = black
	dir     int     // +1 fading out, -1 fading in, 0 idle
}

func (m *manager) transition(op func()) {
	if m.pending != nil {
		return // one transition at a time
	}
	m.pending = op
	m.dir = 1
}

func (m *manager) top() Scene { return m.stack[len(m.stack)-1] }

func (m *manager) update(ctx *Context) error {
	switch m.dir {
	case 1:
		m.fade += fadeStep
		if m.fade >= 1 {
			m.fade = 1
			m.pending()
			m.pending = nil
			m.dir = -1
		}
		return nil
	case -1:
		m.fade -= fadeStep
		if m.fade <= 0 {
			m.fade = 0
			m.dir = 0
		}
	}
	return m.top().Update(ctx)
}

func (m *manager) draw(dst *ebiten.Image, ctx *Context) {
	m.top().Draw(dst, ctx)
	if m.fade > 0 {
		// Step the fade in quarters for a retro look.
		a := float64(int(m.fade*4+0.5)) / 4
		gfx.FillRect(dst, 0, 0, ScreenW, ScreenH, pal.Fade(pal.Black, a))
	}
}
