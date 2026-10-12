package scene

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
)

// The review fixes of My choices (PLAN §14, 2026-10-12).

// waitDue runs the Account screen's check until the fetched choices are
// waiting to open.
func waitDue(t *testing.T, ctx *game.Context, a *Account) {
	t.Helper()
	holdKey(t, ebiten.KeyF24)
	for deadline := time.Now().Add(5 * time.Second); a.due == nil; {
		if time.Now().After(deadline) {
			t.Fatal("nothing due")
		}
		if a.checkChoices(ctx, ctx.Link.Status()) != nil {
			t.Fatal("opened at once")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestAccountWaitsBeforeChoices (G5e): after linking, "Linked!" stays
// until a key or a pause; the key only opens My choices, which says
// "Linked!" again on the notice.
func TestAccountWaitsBeforeChoices(t *testing.T) {
	site := &choicesSite{}
	ctx := choicesContext(t, site)
	a := NewAccount(ctx).(*Account)
	a.linked = "Linked! Your progress now goes to your grown-up's account."
	waitDue(t, ctx, a)
	for range 10 {
		if a.checkChoices(ctx, ctx.Link.Status()) != nil {
			t.Fatal("opened without a key or a pause")
		}
	}
	holdKey(t, ebiten.KeyArrowDown)
	c := a.checkChoices(ctx, ctx.Link.Status())
	holdKey(t, ebiten.KeyF24)
	if c == nil || c.step != chOnPage || c.page.kind != chChild || !strings.HasPrefix(c.msg, "Linked!") {
		t.Fatalf("on a key: %+v", c)
	}
	if a.sel != 0 {
		t.Error("the key also moved the menu")
	}
	drawChoices(t, ctx, c)

	// Without a key, it opens after the pause.
	a = NewAccount(ctx).(*Account)
	waitDue(t, ctx, a)
	n := 0
	for a.checkChoices(ctx, ctx.Link.Status()) == nil {
		if n++; n > choicesPause {
			t.Fatal("never opened")
		}
	}
	if n < choicesPause-2 {
		t.Errorf("opened after %d ticks", n)
	}
}

// TestChildNoticeEscIsNotSeen (G5e): Esc on the child notice moves on
// without recording it; it is due again.
func TestChildNoticeEscIsNotSeen(t *testing.T) {
	site := &choicesSite{}
	ctx := choicesContext(t, site)
	c := NewChoices(ctx).(*Choices)
	chSettle(t, ctx, c)
	if c.step != chOnPage || c.page.kind != chChild {
		t.Fatalf("page: %+v", c.page)
	}
	chPress(t, ctx, c, ebiten.KeyEscape)
	time.Sleep(50 * time.Millisecond)
	if c.step != chList || !ctx.Link.ChildNoticeDue() || len(site.posted()) != 0 {
		t.Errorf("Esc recorded it: step %d, posted %v", c.step, site.posted())
	}
}

// TestChoicesBusyEsc (G5a): Esc while a change is on its way doesn't
// leave; it says it is still sending.
func TestChoicesBusyEsc(t *testing.T) {
	site := &choicesSite{}
	ctx := choicesContext(t, site)
	ctx.Link.SawNotice(context.Background(), "child-link/1/en/c")
	c := NewChoices(ctx).(*Choices)
	chSettle(t, ctx, c)
	block := make(chan chResult)
	c.out, c.step = block, chBusy
	chPress(t, ctx, c, ebiten.KeyEscape)
	if c.step != chBusy || !c.still {
		t.Fatalf("Esc while busy: step %d still %v", c.step, c.still)
	}
	drawChoices(t, ctx, c)
}

// TestChoicesActedButReloadFailed (G5b): a change that worked says so,
// and the failed reload is told apart.
func TestChoicesActedButReloadFailed(t *testing.T) {
	site := &choicesSite{failAfter: 1}
	ctx := choicesContext(t, site)
	ctx.Link.SawNotice(context.Background(), "child-link/1/en/c")
	c := NewChoices(ctx).(*Choices)
	chSettle(t, ctx, c)
	c.sel = 2 // stop
	chPress(t, ctx, c, ebiten.KeyEnter)
	chPress(t, ctx, c, ebiten.KeyY)
	chSettle(t, ctx, c)
	if c.step != chFailed || !strings.HasPrefix(c.msg, "Done.") || !strings.Contains(c.msg2, "couldn't be loaded again") {
		t.Fatalf("after stopping: step %d %q / %q", c.step, c.msg, c.msg2)
	}
	if !ctx.Link.Status().Stopped {
		t.Error("not stopped")
	}
	drawChoices(t, ctx, c)
}

// TestChoicesTypedYes (G5h): Y and N are the typed letters, so a
// German keyboard's Y works and its Z (where a US keyboard has Y)
// doesn't.
func TestChoicesTypedYes(t *testing.T) {
	ctx := testContext(t)
	ctx.Input = &input.State{Chars: []rune{'z'}}
	holdKey(t, ebiten.KeyY) // the key a US keyboard has Y on
	if typedYes(ctx) {
		t.Error("Z on a German keyboard said yes")
	}
	ctx.Input.Chars = []rune{'Y'}
	holdKey(t, ebiten.KeyZ)
	if !typedYes(ctx) || typedNo(ctx) {
		t.Error("Y on a German keyboard didn't say yes")
	}
	ctx.Input.Chars = nil
	holdKey(t, ebiten.KeyN)
	if !typedNo(ctx) {
		t.Error("N without typed characters")
	}
}

// TestChoicesListScrolls (G5g, G5c): many choices scroll rather than
// run into the message; the chosen ask's hint is there and Enter is
// "change".
func TestChoicesListScrolls(t *testing.T) {
	site := &choicesSite{asks: []string{"a", "b", "c", "d", "e", "f", "g"}}
	ctx := choicesContext(t, site)
	ctx.Link.SawNotice(context.Background(), "child-link/1/en/c")
	c := NewChoices(ctx).(*Choices)
	chSettle(t, ctx, c)
	c.say("A message of two lines at the bottom of the screen, long enough to wrap onto its second line.", c.msgCol)
	info := c.infoLines(ctx.Font, ctx.Link.Status(), game.ScreenW-96)
	my := 40 + 20 + len(info)*18 + 10
	rows := max(3, (chListEnd-my)/chRow)
	if my+rows*chRow > chHintY {
		t.Errorf("list runs to %d, past the hint at %d", my+rows*chRow, chHintY)
	}
	n := len(c.items())
	if n <= rows {
		t.Fatalf("%d items fit in %d rows; nothing to scroll", n, rows)
	}
	for i := range n {
		c.sel = i
		drawChoices(t, ctx, c)
		if c.sel < c.top || c.sel >= c.top+rows {
			t.Errorf("item %d out of view (top %d, rows %d)", i, c.top, rows)
		}
	}
	if c.top == 0 {
		t.Error("never scrolled")
	}
}

// TestChoicesWhileStopped (G3, S7): no "who can see", no Sync now, and
// with no way to ask, the list says to ask a parent or guardian.
func TestChoicesWhileStopped(t *testing.T) {
	site := &choicesSite{}
	ctx := choicesContext(t, site)
	ctx.Link.SawNotice(context.Background(), "child-link/1/en/c")
	if err := ctx.Link.StopProgress(context.Background()); err != nil {
		t.Fatal(err)
	}
	site.mu.Lock()
	site.stopped = true
	site.mu.Unlock()
	a := &Account{}
	for _, it := range a.items(ctx) {
		if it == acSync {
			t.Error("Sync now while stopped")
		}
	}
	c := newChoicesWith(ctx, &link.Choices{Family: true, Progress: &link.OwnProgress{Stopped: true}}, "")
	var all []string
	for _, l := range c.infoLines(ctx.Font, ctx.Link.Status(), 500) {
		all = append(all, l.s)
	}
	text := strings.Join(all, " ")
	if strings.Contains(text, "can see your progress") || !strings.Contains(text, "ask your parent or guardian") {
		t.Errorf("stopped, no restart: %q", text)
	}
	for _, it := range c.items() {
		if it.kind == chItemRestart {
			t.Error("restart offered without a restart from the server")
		}
	}

	// A suspended account says so first.
	s, _ := syncProblem(link.Status{Linked: true, Stopped: true, Suspended: true})
	if !strings.HasPrefix(s, "Can't sync right now") {
		t.Errorf("suspended and stopped: %q", s)
	}
	s, _ = syncProblem(link.Status{Linked: true, Stopped: true, Err: link.ErrNotLinkable})
	if strings.Contains(s, "isn't being kept") {
		t.Errorf("removed and stopped: %q", s)
	}
}

// TestChoicesNothingAsked (G6): a 13-15 with nothing asked gets
// visibility with no asks and no notice, and is told, as the website
// says, that nobody outside the family can see them.
func TestChoicesNothingAsked(t *testing.T) {
	site := &choicesSite{asks: []string{}}
	ctx := choicesContext(t, site)
	ctx.Link.SawNotice(context.Background(), "child-link/1/en/c")
	site.mu.Lock()
	site.stopped, site.waiting = false, false
	site.mu.Unlock()
	c := NewChoices(ctx).(*Choices)
	chSettle(t, ctx, c)
	if c.ch == nil || c.ch.Visibility == nil || len(c.ch.Visibility.Asks) != 0 || c.ch.Visibility.Ref != "" {
		t.Fatalf("visibility with nothing asked: %+v", c.ch)
	}
	c.ch.Progress = nil // a 13-15 has no progress choices
	var all []string
	for _, l := range c.infoLines(ctx.Font, ctx.Link.Status(), 1000) {
		all = append(all, l.s)
	}
	if text := strings.Join(all, " "); !strings.Contains(text, "Nobody outside your family can see you on Halpwords. If your parent or guardian wants to change that, you decide here too.") ||
		strings.Contains(text, "your grown-up makes these choices") {
		t.Errorf("nothing asked: %q", text)
	}
}

// TestWrapHard (G5f): a long word or web address is broken so no line
// runs off.
func TestWrapHard(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	f := ctx.Font
	s := "Write to https://halpwords.example/a/very/long/address/that/never/ends/at/all/and/keeps/going/on/and/on please."
	for _, l := range wrapHard(f, s, 200) {
		if f.Width(l, 1) > 200 {
			t.Errorf("line %q is %d wide", l, f.Width(l, 1))
		}
	}
	if got := pageTitle(f, strings.Repeat("A long title ", 20), 200); len(got) != 2 {
		t.Errorf("title lines: %q", got)
	}
}
