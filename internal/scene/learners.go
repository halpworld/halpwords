package scene

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/pkg/proc"
)

// Several learners on one computer (W2.5): the Switch learner screen
// lists them, adds one and signs one out; the sign-in screen signs a
// learner in with a login card, the class code and their pictures, or
// a grown-up's pairing code, and opens a signed-in learner's folder
// again with the same card or pictures.

// lrRow is a row of the Switch learner screen.
type lrRow struct {
	learner *profile.Learner // a learner, or nil for an action
	action  lrAction
}

type lrAction int

const (
	lrPick lrAction = iota
	lrAdd
	lrSignOut
	lrRemove
	lrBack
)

// Learners is the Switch learner screen.
type Learners struct {
	bg  *ebiten.Image
	sel int
	// confirm is the action waiting for Y or N.
	confirm lrAction
	// signingOut reports when a sign-out has finished.
	signingOut func() bool
	msg        string
	msgCol     color.RGBA
}

// NewLearners creates the Switch learner screen.
func NewLearners(ctx *game.Context) game.Scene {
	return &Learners{bg: backdrop(12, 1.2), confirm: lrPick}
}

func (s *Learners) rows(ctx *game.Context) []lrRow {
	var rows []lrRow
	for _, l := range ctx.Learners.List {
		rows = append(rows, lrRow{learner: l})
	}
	rows = append(rows, lrRow{action: lrAdd})
	if cur := ctx.Learner(); cur != nil {
		if ctx.Link.Linked() {
			rows = append(rows, lrRow{learner: cur, action: lrSignOut})
		} else if len(ctx.Learners.List) > 1 {
			rows = append(rows, lrRow{learner: cur, action: lrRemove})
		}
	}
	return append(rows, lrRow{action: lrBack})
}

func (s *Learners) say(msg string, c color.RGBA) { s.msg, s.msgCol = msg, c }

// Update implements game.Scene.
func (s *Learners) Update(ctx *game.Context) error {
	if ctx.Learners == nil {
		ctx.Replace(NewTitle(ctx))
		return nil
	}
	if s.signingOut != nil {
		if s.signingOut() {
			s.signingOut = nil
			s.sel = 0
			s.say("Signed out.", pal.Lime)
		}
		return nil
	}
	if s.confirm != lrPick {
		switch {
		case input.Pressed(ebiten.KeyY):
			ctx.Sound.Play(audio.Select)
			s.doConfirmed(ctx)
			s.confirm = lrPick
		case input.Pressed(ebiten.KeyN) || input.Back():
			ctx.Sound.Play(audio.Back)
			s.confirm = lrPick
		}
		return nil
	}
	rows := s.rows(ctx)
	n := len(rows)
	s.sel = min(s.sel, n-1)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewTitle(ctx))
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + 1) % n
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		r := rows[s.sel]
		switch r.action {
		case lrPick:
			s.pick(ctx, r.learner)
		case lrAdd:
			ctx.Sound.Play(audio.Select)
			prev := ctx.Learners.Current
			if _, err := ctx.AddLearner(""); err != nil {
				ctx.Sound.Play(audio.Wrong)
				s.say("Couldn't add a learner: "+err.Error(), pal.Rose)
				return nil
			}
			ctx.Replace(NewSignIn(ctx, prev))
		case lrSignOut, lrRemove:
			ctx.Sound.Play(audio.Select)
			s.confirm = r.action
		case lrBack:
			ctx.Sound.Play(audio.Back)
			ctx.Replace(NewTitle(ctx))
		}
	}
	return nil
}

// pick switches to a learner: at once, or after their sign-in when
// their folder is locked.
func (s *Learners) pick(ctx *game.Context, l *profile.Learner) {
	if l.ID == ctx.Learners.Current {
		ctx.Sound.Play(audio.Select)
		ctx.Replace(NewTitle(ctx))
		return
	}
	if l.Lock != nil {
		ctx.Sound.Play(audio.Select)
		ctx.Replace(newUnlock(ctx, l))
		return
	}
	switchTo(ctx, l)
}

// switchTo makes l the learner playing and goes to the title.
func switchTo(ctx *game.Context, l *profile.Learner) {
	if err := ctx.SwitchLearner(l.ID); err != nil {
		ctx.Sound.Play(audio.Wrong)
		ctx.Notify("Couldn't switch: " + err.Error())
		return
	}
	ctx.Sound.Play(audio.Perfect)
	ctx.Notify("Hello, " + l.Name + "!")
	ctx.Replace(NewTitle(ctx))
}

func (s *Learners) doConfirmed(ctx *game.Context) {
	switch s.confirm {
	case lrSignOut:
		s.signingOut = ctx.SignOut()
		s.say("Signing out…", pal.Ice)
	case lrRemove:
		if err := ctx.RemoveLearner(ctx.Learners.Current); err != nil {
			s.say("Couldn't remove: "+err.Error(), pal.Rose)
			return
		}
		s.sel = 0
		s.say("Removed.", pal.Lime)
	}
}

// Draw implements game.Scene.
func (s *Learners) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, s.bg, 0, 0)
	f.DrawCentered(dst, "Who is playing?", cx, 8, 3, pal.Yellow)
	if ctx.Learners == nil {
		return
	}
	rows := s.rows(ctx)
	const x, w = 80, game.ScreenW - 160
	const step = 20
	// Scroll so the chosen row shows.
	visible := 11
	first := max(0, min(s.sel-visible+1, len(rows)-visible))
	h := min(len(rows), visible)*step + 16
	y := 54
	gfx.Window(dst, x, y, w, h)
	for i := first; i < min(len(rows), first+visible); i++ {
		r := rows[i]
		ry := y + 10 + (i-first)*step
		c := pal.Steel
		if i == s.sel && s.confirm == lrPick && s.signingOut == nil {
			c = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+12, ry, 1, pal.Yellow)
			}
		}
		label, note := "", ""
		switch r.action {
		case lrPick:
			label = r.learner.Name
			switch {
			case r.learner.ID == ctx.Learners.Current:
				note = "playing now"
			case r.learner.Lock != nil:
				note = "sign in to play"
			}
		case lrAdd:
			label = "+ Add a learner"
		case lrSignOut:
			label = "Sign out " + r.learner.Name
		case lrRemove:
			label = "Remove " + r.learner.Name + " from this computer"
		case lrBack:
			label = "Back"
		}
		f.DrawShadow(dst, fit(f, label, w-190, 1), x+32, ry, 1, c)
		if note != "" {
			f.DrawShadow(dst, note, x+w-16-f.Width(note, 1), ry, 1, pal.Ash)
		}
	}
	if s.msg != "" {
		f.DrawCentered(dst, s.msg, cx, game.ScreenH-44, 1, s.msgCol)
	}
	f.DrawShadow(dst, "↑/↓ choose   Enter select   Esc back", 8, game.ScreenH-20, 1, pal.Ash)
	switch s.confirm {
	case lrSignOut:
		drawSignOut(dst, ctx)
	case lrRemove:
		dx, dy := dialog(dst, ctx, "REMOVE?", 500, 170)
		f.DrawShadow(dst, "This deletes this learner's hero, word memory,", dx, dy, 1, pal.Tan)
		f.DrawShadow(dst, "Hall of Fame and word lists from this computer.", dx, dy+16, 1, pal.Tan)
		f.DrawShadow(dst, "Y remove   N cancel", dx, dy+72, 1, pal.Ash)
	}
}

// siStep is what the sign-in screen is doing.
type siStep int

const (
	siChoose   siStep = iota // choosing how to sign in
	siCard                   // typing a login card's code
	siPairing                // typing a grown-up's pairing code
	siClass                  // typing the class code
	siName                   // choosing a name from the class's list
	siUsername               // typing a username
	siPictures               // tapping 3 pictures
	siPlayName               // typing a name, to play without signing in
	siWaiting                // waiting for the server
	siSSO                    // signing in with a school account on the website
)

// siWay is a way to sign in, on the first step.
type siWay int

const (
	siWayCard siWay = iota
	siWayClass
	siWayPairing
	siWayJustPlay
	siWaySSO
)

var siWayLabels = [...]string{"I have a login card", "I have a class code", "A grown-up gave me a code",
	"Just play, without signing in", "I have a school Google or Microsoft account"}

// SignIn signs the learner playing in: at school with a login card or
// the class code and pictures, or at home with a grown-up's pairing code.
// It also opens a signed-in learner's folder again (unlock).
type SignIn struct {
	bg   *ebiten.Image
	step siStep
	sel  int
	// prev is the learner to go back to when a learner being added
	// gives up; "" when no learner is being added.
	prev string
	// unlock is the learner whose folder the sign-in opens, or nil.
	unlock *profile.Learner

	text []rune // what is being typed

	// The class code sign-in.
	classCode string
	class     *link.Class
	learnerID string
	username  string
	grid      []int // the 9 pictures
	picks     []int
	cursor    int // the picture chosen, 0 to 8
	scroll    int // the first name shown

	card    string        // the card's code, once sent
	pending chan siResult // an answer from the server on its way
	linking bool          // a sign-in on its way
	msg     string
	msgCol  color.RGBA

	// Signing in with a school account.
	sso      *link.SSOCode
	polling  chan error // a poll on its way
	nextPoll uint64     // the tick of the next poll
}

type siResult struct {
	class *link.Class
	sso   *link.SSOCode
	err   error
}

// NewSignIn creates the sign-in screen for the learner playing. prev is
// the learner who played before one being added, or "".
func NewSignIn(ctx *game.Context, prev string) game.Scene {
	return &SignIn{bg: backdrop(13, 1.2), prev: prev}
}

// newUnlock creates the sign-in screen that opens l's folder.
func newUnlock(ctx *game.Context, l *profile.Learner) *SignIn {
	s := &SignIn{bg: backdrop(13, 1.2), unlock: l}
	if l.Lock.Kind == profile.LockPictures && len(l.Lock.Grid) == link.Grid {
		s.step, s.grid = siPictures, l.Lock.Grid
	} else {
		s.step = siCard
	}
	return s
}

func (s *SignIn) say(msg string, c color.RGBA) { s.msg, s.msgCol = msg, c }

func (s *SignIn) ways() []siWay {
	ws := []siWay{siWayCard, siWayClass, siWaySSO, siWayPairing}
	if s.prev != "" {
		ws = append(ws, siWayJustPlay)
	}
	return ws
}

// codeLen is how many characters the code being typed has.
func (s *SignIn) codeLen() int {
	switch s.step {
	case siCard:
		return link.CardCodeLen
	case siClass:
		return link.ClassCodeLen
	}
	return link.PairingCodeLen
}

// Update implements game.Scene.
func (s *SignIn) Update(ctx *game.Context) error {
	if s.step == siSSO {
		s.updateSSO(ctx)
		return nil
	}
	if s.pending != nil {
		select {
		case r := <-s.pending:
			s.pending = nil
			s.answer(ctx, r)
		default:
		}
		return nil
	}
	if s.linking {
		if st := ctx.Link.Status(); !st.Busy {
			s.linking = false
			s.linked(ctx, st)
		}
		return nil
	}
	switch s.step {
	case siChoose:
		s.updateChoose(ctx)
	case siCard, siPairing, siClass:
		s.updateCode(ctx)
	case siName:
		s.updateName(ctx)
	case siUsername, siPlayName:
		s.updateText(ctx)
	case siPictures:
		s.updatePictures(ctx)
	}
	return nil
}

// back goes back a step, and from the first gives up.
func (s *SignIn) back(ctx *game.Context) {
	ctx.Sound.Play(audio.Back)
	s.say("", pal.Steel)
	s.text = s.text[:0]
	switch {
	case s.unlock != nil:
		ctx.Replace(NewLearners(ctx))
	case s.step == siChoose:
		s.giveUp(ctx)
	case s.step == siName || s.step == siUsername:
		s.step = siClass
		s.text = []rune(link.NormCode(s.classCode))
	case s.step == siPictures:
		s.picks = s.picks[:0]
		if s.class != nil && s.class.NamesShown {
			s.step = siName
		} else {
			s.step = siUsername
			s.text = []rune(s.username)
		}
	default:
		s.step = siChoose
	}
}

// giveUp leaves the screen. A learner being added is removed again, and
// the one before plays.
func (s *SignIn) giveUp(ctx *game.Context) {
	if s.prev != "" && !ctx.Link.Linked() {
		if cur := ctx.Learner(); cur != nil && cur.ID != s.prev {
			ctx.RemoveLearner(cur.ID)
		}
		if ctx.Learners.Find(s.prev) != nil && ctx.Learners.Current != s.prev {
			ctx.SwitchLearner(s.prev)
		}
		ctx.Replace(NewLearners(ctx))
		return
	}
	ctx.Replace(NewAccount(ctx))
}

func (s *SignIn) updateChoose(ctx *game.Context) {
	ws := s.ways()
	n := len(ws)
	s.sel = min(s.sel, n-1)
	switch {
	case input.Back():
		s.back(ctx)
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + 1) % n
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		ctx.Sound.Play(audio.Select)
		s.text = s.text[:0]
		s.say("", pal.Steel)
		if ws[s.sel] == siWaySSO {
			s.startSSO(ctx)
			return
		}
		s.step = map[siWay]siStep{siWayCard: siCard, siWayClass: siClass, siWayPairing: siPairing, siWayJustPlay: siPlayName}[ws[s.sel]]
	}
}

// codeRune is r as a code's character, if it can be one.
func codeRune(r rune) (rune, bool) {
	r = unicode.ToUpper(r)
	return r, r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func (s *SignIn) updateCode(ctx *game.Context) {
	n := s.codeLen()
	for _, r := range ctx.Input.Chars {
		if r, ok := codeRune(r); ok && len(s.text) < n {
			s.text = append(s.text, r)
			ctx.Sound.Play(audio.Key)
		}
	}
	switch {
	case input.Repeat(ebiten.KeyBackspace) && len(s.text) > 0:
		s.text = s.text[:len(s.text)-1]
		ctx.Sound.Play(audio.Erase)
	case input.Back():
		s.back(ctx)
	case input.Confirm() && len(s.text) == n:
		ctx.Sound.Play(audio.Select)
		code := string(s.text)
		switch s.step {
		case siCard:
			if s.unlock != nil {
				s.tryUnlock(ctx, link.NormCode(code))
				return
			}
			s.card = code
			s.signIn(ctx, link.SignIn{Code: code})
		case siPairing:
			s.signIn(ctx, link.SignIn{Code: code})
		case siClass:
			s.classCode = code
			s.ask(ctx, "", "")
		}
	case input.Confirm():
		ctx.Sound.Play(audio.Wrong)
		s.say(fmt.Sprintf("The code has %d letters and numbers.", n), pal.Rose)
	}
}

// ask asks the server for the class, and the pictures of a child when
// learnerID or username names one.
func (s *SignIn) ask(ctx *game.Context, learnerID, username string) {
	s.pending = make(chan siResult, 1)
	s.say("Asking the website…", pal.Ice)
	lc, code, out := ctx.Link, s.classCode, s.pending
	go func() {
		c, cancel := context.WithTimeout(context.Background(), askTimeout)
		defer cancel()
		cl, err := lc.FindClass(c, code, learnerID, username)
		out <- siResult{class: cl, err: err}
	}()
}

// askTimeout limits a question to the server.
const askTimeout = 30e9 // 30 seconds

// answer takes the server's answer about the class.
func (s *SignIn) answer(ctx *game.Context, r siResult) {
	if r.err != nil {
		ctx.Sound.Play(audio.Wrong)
		s.say(upperFirst(explainSignIn(r.err))+".", pal.Rose)
		return
	}
	if r.sso != nil {
		s.showSSO(ctx, r.sso)
		return
	}
	s.say("", pal.Steel)
	s.class = r.class
	if len(r.class.Pictures) == link.Grid {
		s.grid, s.picks, s.cursor = r.class.Pictures, s.picks[:0], 0
		s.step = siPictures
		return
	}
	s.text = s.text[:0]
	if r.class.NamesShown && len(r.class.Learners) > 0 {
		s.step, s.sel, s.scroll = siName, 0, 0
		return
	}
	s.step = siUsername
}

// explainSignIn says what went wrong with a sign-in.
func explainSignIn(err error) string {
	if errors.Is(err, profile.ErrLocked) {
		return err.Error()
	}
	return link.Explain(err)
}

// namesShown is how many names the list shows at once.
const namesShown = 10

func (s *SignIn) updateName(ctx *game.Context) {
	names := s.class.Learners
	n := len(names)
	for _, r := range ctx.Input.Chars {
		// A letter jumps to the next name starting with it.
		r = unicode.ToLower(r)
		for k := 1; k <= n; k++ {
			i := (s.sel + k) % n
			if first := []rune(strings.ToLower(names[i].DisplayName)); len(first) > 0 && first[0] == r {
				s.sel = i
				ctx.Sound.Play(audio.Blip)
				break
			}
		}
	}
	switch {
	case input.Back():
		s.back(ctx)
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + 1) % n
	case input.Confirm():
		ctx.Sound.Play(audio.Select)
		s.learnerID, s.username = names[s.sel].ID, ""
		s.ask(ctx, s.learnerID, "")
	}
	s.scroll = max(0, min(s.scroll, s.sel), s.sel-namesShown+1)
}

// usernameRune reports whether r can be in a username (or a player's
// name, which may also have spaces and any letter).
func (s *SignIn) textRune(r rune) bool {
	if s.step == siPlayName {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '\''
	}
	return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_')
}

// maxText is the longest username or player's name.
const maxText = 20

func (s *SignIn) updateText(ctx *game.Context) {
	for _, r := range ctx.Input.Chars {
		if s.textRune(r) && len(s.text) < maxText {
			s.text = append(s.text, r)
			ctx.Sound.Play(audio.Key)
		}
	}
	text := strings.TrimSpace(string(s.text))
	switch {
	case input.Repeat(ebiten.KeyBackspace) && len(s.text) > 0:
		s.text = s.text[:len(s.text)-1]
		ctx.Sound.Play(audio.Erase)
	case input.Back():
		s.back(ctx)
	case input.Confirm() && text == "":
		ctx.Sound.Play(audio.Wrong)
	case input.Confirm() && s.step == siUsername:
		ctx.Sound.Play(audio.Select)
		s.learnerID, s.username = "", text
		s.ask(ctx, "", text)
	case input.Confirm():
		ctx.Sound.Play(audio.Perfect)
		if l := ctx.Learner(); l != nil {
			l.Name = text
			ctx.SignedIn(nil)
		}
		ctx.Notify("Hello, " + text + "!")
		ctx.Replace(NewTitle(ctx))
	}
}

// pictureKeys pick a picture of the grid directly: 1 to 9, top left to
// bottom right, on the number row or the keypad.
var pictureKeys = [link.Grid][2]ebiten.Key{
	{ebiten.Key1, ebiten.KeyNumpad7}, {ebiten.Key2, ebiten.KeyNumpad8}, {ebiten.Key3, ebiten.KeyNumpad9},
	{ebiten.Key4, ebiten.KeyNumpad4}, {ebiten.Key5, ebiten.KeyNumpad5}, {ebiten.Key6, ebiten.KeyNumpad6},
	{ebiten.Key7, ebiten.KeyNumpad1}, {ebiten.Key8, ebiten.KeyNumpad2}, {ebiten.Key9, ebiten.KeyNumpad3},
}

func (s *SignIn) updatePictures(ctx *game.Context) {
	tapped := -1
	for i, keys := range pictureKeys {
		if input.Pressed(keys[0], keys[1]) {
			s.cursor, tapped = i, i
		}
	}
	switch {
	case input.Back():
		s.back(ctx)
		return
	case input.Repeat(ebiten.KeyBackspace) && len(s.picks) > 0:
		s.picks = s.picks[:len(s.picks)-1]
		ctx.Sound.Play(audio.Erase)
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		s.cursor = (s.cursor + 6) % 9
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		s.cursor = (s.cursor + 3) % 9
	case input.Repeat(ebiten.KeyArrowLeft):
		ctx.Sound.Play(audio.Blip)
		s.cursor = s.cursor/3*3 + (s.cursor%3+2)%3
	case input.Repeat(ebiten.KeyArrowRight):
		ctx.Sound.Play(audio.Blip)
		s.cursor = s.cursor/3*3 + (s.cursor%3+1)%3
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		tapped = s.cursor
	}
	if tapped < 0 || len(s.picks) >= link.Picks {
		return
	}
	ctx.Sound.Play(audio.Key)
	s.picks = append(s.picks, s.grid[tapped])
	if len(s.picks) < link.Picks {
		return
	}
	picks := append([]int(nil), s.picks...)
	s.picks = s.picks[:0]
	if s.unlock != nil {
		s.tryUnlock(ctx, profile.PicturesSecret(picks))
		return
	}
	s.signIn(ctx, link.SignIn{ClassCode: s.classCode, LearnerID: s.learnerID, Username: s.username, Pictures: picks})
	s.picks = picks // kept for the lock, once signed in
}

// signIn sends the sign-in to the server.
func (s *SignIn) signIn(ctx *game.Context, si link.SignIn) {
	ctx.Link.SignIn(si)
	s.linking = true
	s.step = siWaiting
	s.say("Signing in…", pal.Ice)
}

// linked finishes a sign-in: on success it locks the learner's folder
// with the same sign-in.
func (s *SignIn) linked(ctx *game.Context, st link.Status) {
	if !st.Linked {
		ctx.Sound.Play(audio.Wrong)
		s.say(upperFirst(explainSignIn(st.Err))+".", pal.Rose)
		s.text = s.text[:0]
		switch {
		case s.card != "":
			s.step = siCard
		case s.classCode != "":
			s.picks = s.picks[:0]
			s.step = siPictures
		default:
			s.step = siPairing
		}
		return
	}
	var lock *profile.Lock
	var err error
	switch {
	case s.card != "":
		lock, err = profile.NewLock(profile.LockCard, link.NormCode(s.card))
	case s.classCode != "":
		if lock, err = profile.NewLock(profile.LockPictures, profile.PicturesSecret(s.picks)); lock != nil {
			lock.Grid = append([]int(nil), s.grid...)
			if s.class != nil {
				lock.ClassName = s.class.Class.Name
			}
		}
	}
	if err != nil {
		ctx.Notify("Signed in, but couldn't lock your progress here")
	}
	ctx.SignedIn(lock)
	ctx.Sound.Play(audio.Perfect)
	ctx.Notify("Signed in!")
	ctx.Replace(NewTitle(ctx))
}

// tryUnlock opens the learner's folder with secret.
func (s *SignIn) tryUnlock(ctx *game.Context, secret string) {
	ok, err := ctx.Learners.Unlock(s.unlock.ID, secret)
	switch {
	case ok:
		switchTo(ctx, s.unlock)
	case errors.Is(err, profile.ErrLocked):
		ctx.Sound.Play(audio.Wrong)
		s.say("Too many wrong tries. Wait a few minutes, then try again.", pal.Rose)
		s.text = s.text[:0]
	default:
		ctx.Sound.Play(audio.Wrong)
		s.say("That's not right. Try again.", pal.Rose)
		s.text = s.text[:0]
	}
}

// showLongCode puts hyphens in a code every 4 characters, with blanks
// for what is still to type: ABCD-EF__.
func showLongCode(code []rune, n int) string {
	s := string(code) + strings.Repeat("_", n-len(code))
	var parts []string
	for len(s) > 4 {
		parts = append(parts, s[:4])
		s = s[4:]
	}
	return strings.Join(append(parts, s), "-")
}

// pictureImages are the picture set, drawn once.
var pictureImages []*ebiten.Image

// pictureSize is how big a picture is drawn: 8 pixels, each 5 by 5.
const pictureScale = 5

func pictureImage(i int) *ebiten.Image {
	if pictureImages == nil {
		for _, p := range proc.Pictures() {
			pictureImages = append(pictureImages, gfx.Upload(p.Image(pictureScale)))
		}
	}
	if i < 0 || i >= len(pictureImages) {
		return nil
	}
	return pictureImages[i]
}

// Draw implements game.Scene.
func (s *SignIn) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, s.bg, 0, 0)
	title := "Sign in"
	if s.unlock != nil {
		title = "Is it you, " + s.unlock.Name + "?"
	}
	f.DrawCentered(dst, fit(f, title, game.ScreenW-40, 3), cx, 8, 3, pal.Yellow)
	const x, w = 60, game.ScreenW - 120
	y := 56
	hint := "Enter next   Esc back"
	switch s.step {
	case siSSO:
		hint = s.drawSSO(dst, ctx, x, y, w)
	case siChoose:
		gfx.Window(dst, x, y, w, 30+len(s.ways())*24)
		for i, wy := range s.ways() {
			c := pal.Steel
			ry := y + 14 + i*24
			if i == s.sel {
				c = pal.White
				if ctx.Tick/20%2 == 0 {
					f.Draw(dst, "►", x+14, ry, 1, pal.Yellow)
				}
			}
			f.DrawShadow(dst, siWayLabels[wy], x+36, ry, 1, c)
		}
		f.DrawCentered(dst, "At school, your teacher gives you a card or a class code.", cx, y+174, 1, pal.Ice)
		f.DrawCentered(dst, "At home, a grown-up gets a code on the Halpwords website.", cx, y+192, 1, pal.Ice)
		hint = "↑/↓ choose   Enter select   Esc back"
	case siCard, siPairing, siClass:
		gfx.Window(dst, x, y, w, 150)
		prompt := map[siStep]string{
			siCard:    "Type the code on your login card:",
			siPairing: "Type the code from the website:",
			siClass:   "Type your class code:",
		}[s.step]
		f.DrawShadow(dst, prompt, x+20, y+16, 1, pal.Ice)
		f.DrawCentered(dst, showLongCode(s.text, s.codeLen()), cx, y+50, 3, pal.White)
		switch s.step {
		case siPairing:
			f.DrawShadow(dst, "A code works once, for 15 minutes.", x+20, y+110, 1, pal.Steel)
		case siClass:
			f.DrawShadow(dst, "It is on the board, or ask your teacher.", x+20, y+110, 1, pal.Steel)
		}
	case siName:
		s.drawNames(dst, ctx, x, y, w)
		hint = "↑/↓ or a letter choose   Enter select   Esc back"
	case siUsername, siPlayName:
		gfx.Window(dst, x, y, w, 120)
		prompt := "Type your username:"
		if s.step == siPlayName {
			prompt = "Type your name:"
		}
		f.DrawShadow(dst, prompt, x+20, y+16, 1, pal.Ice)
		text := string(s.text)
		f.DrawShadow(dst, text, x+20, y+50, 2, pal.White)
		if ctx.Tick/16%2 == 0 {
			gfx.FillRect(dst, x+20+f.Width(text, 2)+2, y+52, 10, 26, pal.Yellow)
		}
	case siPictures:
		s.drawPictures(dst, ctx)
		hint = "Arrows or 1-9 choose   Enter tap   Backspace undo   Esc back"
	case siWaiting:
		f.DrawCentered(dst, "Signing in…", cx, 150, 2, pal.Ice)
		hint = ""
	}
	if s.pending != nil {
		f.DrawCentered(dst, "Asking the website…", cx, game.ScreenH-44, 1, pal.Ice)
	} else if s.msg != "" && s.step != siWaiting {
		f.DrawCentered(dst, fit(f, s.msg, game.ScreenW-20, 1), cx, game.ScreenH-44, 1, s.msgCol)
	}
	if hint != "" {
		f.DrawShadow(dst, hint, 8, game.ScreenH-20, 1, pal.Ash)
	}
}

func (s *SignIn) drawNames(dst *ebiten.Image, ctx *game.Context, x, y, w int) {
	f := ctx.Font
	names := s.class.Learners
	gfx.Window(dst, x, y, w, 30+namesShown*20)
	f.DrawShadow(dst, fit(f, s.class.Class.Name+": who are you?", w-40, 1), x+20, y+10, 1, pal.Ice)
	for i := s.scroll; i < min(len(names), s.scroll+namesShown); i++ {
		ry := y + 32 + (i-s.scroll)*20
		c := pal.Steel
		if i == s.sel {
			c = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+14, ry, 1, pal.Yellow)
			}
		}
		f.DrawShadow(dst, fit(f, names[i].DisplayName, w-60, 1), x+36, ry, 1, c)
	}
	if s.scroll > 0 {
		f.DrawShadow(dst, "▲", x+w-24, y+32, 1, pal.Ash)
	}
	if s.scroll+namesShown < len(names) {
		f.DrawShadow(dst, "▼", x+w-24, y+12+namesShown*20, 1, pal.Ash)
	}
}

func (s *SignIn) drawPictures(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	const cell = proc.PictureSize*pictureScale + 16
	gx, gy := cx-3*cell/2, 76
	f.DrawCentered(dst, "Tap your 3 pictures, in order:", cx, 52, 1, pal.Ice)
	gfx.Window(dst, gx-8, gy-8, 3*cell+16, 3*cell+16)
	for i, p := range s.grid {
		px, py := gx+i%3*cell, gy+i/3*cell
		if i == s.cursor {
			gfx.FillRect(dst, px+2, py+2, cell-4, cell-4, pal.Indigo)
		}
		if img := pictureImage(p); img != nil {
			gfx.DrawArt(dst, img, px+8, py+8)
		}
		f.Draw(dst, fmt.Sprint(i+1), px+3, py+2, 1, pal.Ash)
	}
	// Only how many are tapped shows, never which.
	dots := strings.Repeat("● ", len(s.picks)) + strings.Repeat("○ ", link.Picks-len(s.picks))
	f.DrawCentered(dst, strings.TrimSpace(dots), cx, gy+3*cell+14, 2, pal.Yellow)
}

// drawSignOut asks whether to sign out, and says what happens to the
// learner's progress on this computer.
func drawSignOut(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	dx, dy := dialog(dst, ctx, "SIGN OUT?", 500, 170)
	if ctx.Link.KeepOnSignOut() {
		f.DrawShadow(dst, "Your progress stays on this computer. To play", dx, dy, 1, pal.Ice)
		f.DrawShadow(dst, "again, sign in with the same card or pictures.", dx, dy+16, 1, pal.Ice)
	} else {
		f.DrawShadow(dst, "Your progress on this computer will be deleted.", dx, dy, 1, pal.Tan)
		f.DrawShadow(dst, "What you sent to the website stays there.", dx, dy+16, 1, pal.Ice)
	}
	f.DrawShadow(dst, "Y sign out   N cancel", dx, dy+72, 1, pal.Ash)
}
