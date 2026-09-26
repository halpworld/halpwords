package scene

import (
	"context"
	"net/http"
	"net/url"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/playtest"
	"github.com/halpworld/halpwords/pkg/maps"
)

// Playtest fetches a quest made in the server's map editor and starts it
// (the web game's play-test mode, internal/playtest). The web build
// starts with it when the page's address names a quest; files are then
// kept in memory, so nothing the play-test does is saved.
type Playtest struct {
	bg     *ebiten.Image
	done   chan playtestResult
	msg    string // why the quest can't be played, once it's known
	notice bool   // the "nothing is saved" notice has been shown
}

type playtestResult struct {
	q   *maps.Quest
	err error
}

// NewPlaytest returns a scene that fetches the quest at u with client and
// starts it.
func NewPlaytest(client *http.Client, u *url.URL) func(*game.Context) game.Scene {
	return func(*game.Context) game.Scene {
		p := &Playtest{bg: backdrop(4, 1.4), done: make(chan playtestResult, 1)}
		go func() {
			q, err := playtest.Fetch(context.Background(), client, u)
			p.done <- playtestResult{q, err}
		}()
		return p
	}
}

// PlaytestError returns a scene that says why a play-test can't start,
// such as a quest address on another website.
func PlaytestError(err error) func(*game.Context) game.Scene {
	return func(*game.Context) game.Scene {
		return &Playtest{bg: backdrop(4, 1.4), msg: err.Error()}
	}
}

// Update implements game.Scene.
func (p *Playtest) Update(ctx *game.Context) error {
	if !p.notice {
		p.notice = true
		ctx.Notify("Play-test: nothing is saved")
	}
	if p.done != nil {
		if next := p.fetched(ctx, false); next != nil {
			ctx.Sound.Play(audio.Select)
			ctx.Replace(next)
		}
		return nil
	}
	if input.Back() || input.Confirm() {
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewTitle(ctx))
	}
	return nil
}

// fetched returns the scene that starts the quest once it has been
// fetched, waiting for it if wait is set. It returns nil while the quest
// is on its way, and when it can't be played, which it then says.
func (p *Playtest) fetched(ctx *game.Context, wait bool) game.Scene {
	var res playtestResult
	if wait {
		res = <-p.done
	} else {
		select {
		case res = <-p.done:
		default:
			return nil
		}
	}
	p.done = nil
	if res.err != nil {
		ctx.Sound.Play(audio.Wrong)
		p.msg = res.err.Error()
		return nil
	}
	next, msg := questStart(ctx, res.q)
	if next == nil {
		ctx.Sound.Play(audio.Wrong)
		p.msg = msg
	}
	return next
}

// Draw implements game.Scene.
func (p *Playtest) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, p.bg, 0, 0)
	f.DrawCentered(dst, "Play-test", cx, 24, 3, pal.Yellow)
	f.DrawCentered(dst, "Trying out a quest. Nothing you do here is saved.", cx, 72, 1, pal.Tan)
	if p.done != nil {
		f.DrawCentered(dst, "Fetching the quest…", cx, 140, 2, pal.Ice)
		return
	}
	for i, line := range wrap(f, p.msg, game.ScreenW-80) {
		f.DrawCentered(dst, line, cx, 130+i*16, 1, pal.Rose)
	}
	f.DrawShadow(dst, "Enter or Esc to the title", 8, game.ScreenH-20, 1, pal.Ash)
}
