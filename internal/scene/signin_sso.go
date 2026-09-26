package scene

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/browser"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
)

// Signing in with a school Google or Microsoft account (W5.4): the game
// gets a code from the server, the pupil signs in on the website (the
// game opens it; the web game leaves for it and comes back), and the
// game asks the server every few seconds whether they have.

// openPage opens a web page; tests replace it.
var openPage = browser.Open

// resumeSSO is the sign-in screen going on with a sign-in with a school
// account the game started before, when the web game comes back from the
// website. It is nil when there is none.
func resumeSSO(ctx *game.Context) game.Scene {
	code := ctx.Link.PendingSSO()
	if code == nil {
		return nil
	}
	s := &SignIn{bg: backdrop(13, 1.2)}
	s.step, s.sso = siSSO, code
	s.say("Checking…", pal.Ice)
	return s
}

// startSSO asks the server for a code.
func (s *SignIn) startSSO(ctx *game.Context) {
	s.pending = make(chan siResult, 1)
	s.say("Asking the website…", pal.Ice)
	lc, out := ctx.Link, s.pending
	go func() {
		c, cancel := context.WithTimeout(context.Background(), askTimeout)
		defer cancel()
		code, err := lc.StartSSO(c, browser.Here())
		out <- siResult{sso: code, err: err}
	}()
}

// showSSO shows the code and opens the website's page.
func (s *SignIn) showSSO(ctx *game.Context, code *link.SSOCode) {
	s.step, s.sso, s.nextPoll = siSSO, code, ctx.Tick+ticks(code.Every)
	if err := openPage(code.VerifyURL); err != nil {
		s.say("Open the address below in a web browser.", pal.Ice)
		return
	}
	s.say("Sign in on the page that opened.", pal.Ice)
}

// ticks is d in game ticks.
func ticks(d time.Duration) uint64 {
	return uint64(d.Seconds() * float64(ebiten.TPS()))
}

func (s *SignIn) updateSSO(ctx *game.Context) {
	if s.polling != nil {
		select {
		case err := <-s.polling:
			s.polling = nil
			s.polled(ctx, err)
		default:
		}
	}
	if s.step != siSSO {
		return
	}
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Link.CancelSSO()
		s.sso, s.polling = nil, nil
		s.say("", pal.Steel)
		s.step = siChoose
		return
	case input.Confirm():
		ctx.Sound.Play(audio.Select)
		if err := openPage(s.sso.VerifyURL); err != nil {
			s.say("Open the address below in a web browser.", pal.Ice)
		}
	}
	if s.polling == nil && ctx.Tick >= s.nextPoll {
		s.polling = make(chan error, 1)
		lc, out := ctx.Link, s.polling
		go func() {
			c, cancel := context.WithTimeout(context.Background(), askTimeout)
			defer cancel()
			out <- lc.PollSSO(c)
		}()
	}
}

// polled takes the answer to a poll.
func (s *SignIn) polled(ctx *game.Context, err error) {
	switch {
	case err == nil:
		s.sso = nil
		ctx.SignedIn(nil)
		ctx.Link.SyncNow()
		ctx.Sound.Play(audio.Perfect)
		ctx.Notify("Signed in!")
		ctx.Replace(NewTitle(ctx))
	case errors.Is(err, link.ErrSSOPending):
		if s.msg == "Checking…" {
			s.say("Sign in on the website, then come back here.", pal.Ice)
		}
		s.nextPoll = ctx.Tick + ticks(s.sso.Every)
	case ctx.Link.PendingSSO() != nil:
		// The server couldn't be reached: the code still works.
		s.say(upperFirst(explainSignIn(err))+".", pal.Rose)
		s.nextPoll = ctx.Tick + ticks(3*s.sso.Every)
	default:
		ctx.Sound.Play(audio.Wrong)
		s.say(upperFirst(explainSignIn(err))+".", pal.Rose)
		s.sso = nil
		s.step = siChoose
	}
}

// drawSSO draws the code and the address, and returns the key hint.
func (s *SignIn) drawSSO(dst *ebiten.Image, ctx *game.Context, x, y, w int) string {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.Window(dst, x, y, w, 170)
	f.DrawShadow(dst, "Sign in with your school account at:", x+20, y+16, 1, pal.Ice)
	addr := strings.TrimPrefix(strings.TrimPrefix(s.sso.URL, "https://"), "http://")
	f.DrawCentered(dst, fit(f, addr, w-40, 2), cx, y+40, 2, pal.White)
	f.DrawShadow(dst, "and type this code:", x+20, y+80, 1, pal.Ice)
	f.DrawCentered(dst, s.sso.UserCode, cx, y+104, 3, pal.Yellow)
	f.DrawShadow(dst, "The code runs out after a few minutes.", x+20, y+146, 1, pal.Steel)
	return "Enter open the page again   Esc back"
}
