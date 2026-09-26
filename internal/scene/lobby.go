package scene

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/race"
)

// Lobby is Play Together: the player types the code of a room a grown-up
// opened on the website, joins it, and sees who is in it. Players can
// only send the preset phrases and emotes below: there is no free text.
// In a race room, a race the host starts begins from here after a
// countdown (race.go). Boss Raid (W7.5) starts from here later.
type Lobby struct {
	bg   *ebiten.Image
	code []rune // the room code being typed
	// chat is the chosen preset or emote in the room.
	chat int
	// leaving is set while asking whether to leave the room.
	leaving bool
	// seen is how many changes of the play state the screen has seen, and
	// last the last event it made a sound for.
	seen int
	last int
	msg  string
	// raced is the number of the last race the player ran (or can't
	// run), so it doesn't start again; next is the run for the race
	// about to start, and bad a race whose list the game can't play.
	raced int
	next  *run
	bad   int
}

// NewLobby creates the Play Together screen.
func NewLobby(ctx *game.Context) game.Scene {
	return &Lobby{bg: backdrop(7, 1.15)}
}

// presetText is what the game shows for each preset phrase; only the IDs
// travel.
var presetText = map[string]string{
	"good-luck": "Good luck!",
	"nice-one":  "Nice one!",
	"well-done": "Well done!",
	"ready":     "Ready!",
	"help":      "Help!",
	"thanks":    "Thanks!",
}

// emoteText is what the game shows for each emote, after the name.
var emoteText = map[string]string{
	"wave":      "waves",
	"thumbs-up": "gives a thumbs up",
	"clap":      "claps",
	"smile":     "smiles",
}

// emoteYou is emoteText for the player's own emotes.
var emoteYou = map[string]string{
	"wave":      "wave",
	"thumbs-up": "give a thumbs up",
	"clap":      "clap",
	"smile":     "smile",
}

// chatChoice is a preset or an emote the player can send.
type chatChoice struct {
	emote bool
	id    string
}

func (c chatChoice) label() string {
	if c.emote {
		return "*" + emoteText[c.id] + "*"
	}
	return presetText[c.id]
}

// chatChoices are the presets and emotes the server offers that the game
// knows how to show.
func chatChoices(st link.PlayState) []chatChoice {
	var out []chatChoice
	for _, p := range st.Presets {
		if _, ok := presetText[p]; ok {
			out = append(out, chatChoice{id: p})
		}
	}
	for _, e := range st.Emotes {
		if _, ok := emoteText[e]; ok {
			out = append(out, chatChoice{emote: true, id: e})
		}
	}
	return out
}

// memberName is how the lobby names a member: learners by their
// pseudonym, adults by their role.
func memberName(m link.Member, you string) string {
	var s string
	switch m.Role {
	case link.RoleHost:
		s = "Host"
	case link.RoleAdult:
		s = "Grown-up"
	default:
		s = m.Name
		if s == "" {
			s = "A player"
		}
	}
	if m.ID == you {
		s += " (you)"
	}
	return s
}

// eventText says what happened, in a line of the room's log.
func eventText(e link.PlayEvent, you string) string {
	who := memberName(e.Member, you)
	if e.Mine {
		who = "You"
	}
	switch e.Kind {
	case "joined":
		return who + " joined."
	case "left":
		switch e.Reason {
		case "kicked":
			return who + " was removed by the host."
		case "timeout":
			return who + " didn't come back."
		}
		return who + " left."
	case "away":
		return who + " lost the connection."
	case "back":
		return who + " is back."
	case "said":
		if t, ok := presetText[e.Preset]; ok {
			return who + ": " + t
		}
	case "emoted":
		if t, ok := emoteText[e.Emote]; ok {
			if e.Mine {
				t = emoteYou[e.Emote]
			}
			return who + " " + t + "."
		}
	}
	return ""
}

// showRoomCode puts a dash in a room code: ABC-DEF.
func showRoomCode(code string) string {
	code += strings.Repeat("_", max(0, link.CodeLength-len(code)))
	return code[:3] + "-" + code[3:]
}

// roomCodeRune reports whether r can be typed in a room code, upper
// case.
func roomCodeRune(r rune) (rune, bool) {
	r = unicode.ToUpper(r)
	if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
		return r, true
	}
	return r, false
}

// sortedMembers puts the host first, then grown-ups, then the player,
// then the others in the order they came.
func sortedMembers(r link.Room) []link.Member {
	rank := func(m link.Member) int {
		switch {
		case m.Role == link.RoleHost:
			return 0
		case m.Role == link.RoleAdult:
			return 1
		case m.ID == r.You:
			return 2
		}
		return 3
	}
	out := slices.Clone(r.Members)
	slices.SortStableFunc(out, func(a, b link.Member) int { return rank(a) - rank(b) })
	return out
}

// Update implements game.Scene.
func (l *Lobby) Update(ctx *game.Context) error {
	p := ctx.Link.Play()
	st := p.State()
	if st.Changes != l.seen {
		l.seen = st.Changes
		l.msg = ""
		if n := len(st.Events); n > 0 && n != l.last && !st.Events[n-1].Mine {
			ctx.Sound.Play(audio.Blip)
		}
		l.last = len(st.Events)
		if st.Phase == link.PlayOff && st.Problem != "" {
			ctx.Sound.Play(audio.Wrong)
		}
	}
	switch st.Phase {
	case link.PlayOff:
		l.leaving = false
		l.updateCode(ctx, p)
	case link.PlayJoining, link.PlayRejoining:
		if input.Back() {
			ctx.Sound.Play(audio.Back)
			p.Leave()
		}
	case link.PlayInRoom:
		l.updateRoom(ctx, p, st)
	}
	return nil
}

func (l *Lobby) updateCode(ctx *game.Context, p *link.Play) {
	if !ctx.Link.Linked() {
		if input.Back() || input.Confirm() {
			l.leave(ctx)
		}
		return
	}
	for _, r := range ctx.Input.Chars {
		if r, ok := roomCodeRune(r); ok && len(l.code) < link.CodeLength {
			l.code = append(l.code, r)
			ctx.Sound.Play(audio.Key)
		}
	}
	switch {
	case input.Repeat(ebiten.KeyBackspace) && len(l.code) > 0:
		l.code = l.code[:len(l.code)-1]
		ctx.Sound.Play(audio.Erase)
	case input.Back():
		l.leave(ctx)
	case input.Confirm() && len(l.code) == link.CodeLength:
		ctx.Sound.Play(audio.Select)
		p.Join(string(l.code))
		l.chat = 0
	case input.Confirm():
		ctx.Sound.Play(audio.Wrong)
		l.msg = fmt.Sprintf("A room code has %d letters and numbers.", link.CodeLength)
	}
}

func (l *Lobby) updateRoom(ctx *game.Context, p *link.Play, st link.PlayState) {
	if l.updateRace(ctx, p, st) {
		return
	}
	if l.leaving {
		switch {
		case input.Pressed(ebiten.KeyY):
			ctx.Sound.Play(audio.Back)
			p.Leave()
			l.leaving = false
		case input.Pressed(ebiten.KeyN) || input.Back():
			ctx.Sound.Play(audio.Back)
			l.leaving = false
		}
		return
	}
	choices := chatChoices(st)
	n := len(choices)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Select)
		l.leaving = true
	case n == 0:
	case input.Repeat(ebiten.KeyArrowLeft) || input.Up():
		ctx.Sound.Play(audio.Blip)
		l.chat = (l.chat + n - 1) % n
	case input.Repeat(ebiten.KeyArrowRight) || input.Down():
		ctx.Sound.Play(audio.Blip)
		l.chat = (l.chat + 1) % n
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		c := choices[min(l.chat, n-1)]
		sent := false
		if c.emote {
			sent = p.Emote(c.id)
		} else {
			sent = p.Say(c.id)
		}
		if sent {
			ctx.Sound.Play(audio.Select)
		} else {
			ctx.Sound.Play(audio.Wrong)
		}
	}
}

// updateRace gets the run ready for a race the player is in, and starts
// it when the countdown is over. It reports whether the race started.
func (l *Lobby) updateRace(ctx *game.Context, p *link.Play, st link.PlayState) bool {
	rc := st.Race
	if rc == nil || rc.Number == l.raced {
		l.next = nil
		return false
	}
	if me, ok := rc.Racer(st.Room.You); !ok || me.Status != link.RacerRacing {
		return false
	}
	if l.bad != rc.Number && (l.next == nil || l.next.race.number != rc.Number) {
		r, err := newRaceRun(ctx, rc, st.Room.You)
		if err != nil {
			l.bad, l.next = rc.Number, nil
		} else {
			l.next = r
		}
	}
	if time.Now().Before(rc.StartsAt) {
		return false
	}
	if l.bad == rc.Number {
		// Out of the race at once, so the others needn't wait.
		if p.Report(race.Report{Floor: 1, Fell: true}) {
			l.raced = rc.Number
		}
		return false
	}
	l.raced = rc.Number
	ctx.Sound.Play(audio.Select)
	ctx.Replace(startRace(l.next))
	return true
}

func (l *Lobby) leave(ctx *game.Context) {
	ctx.Sound.Play(audio.Back)
	ctx.Replace(NewTitle(ctx))
}

// Draw implements game.Scene.
func (l *Lobby) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, l.bg, 0, 0)
	f.DrawCentered(dst, "Play Together", cx, 4, 3, pal.Yellow)
	st := ctx.Link.Play().State()
	hint := ""
	switch st.Phase {
	case link.PlayOff:
		hint = l.drawCode(dst, ctx, st)
	case link.PlayJoining, link.PlayRejoining:
		const x, w = 80, game.ScreenW - 160
		gfx.Window(dst, x, 120, w, 90)
		code := showRoomCode(st.Room.Code)
		text := "Joining the room…"
		if st.Phase == link.PlayRejoining {
			text = "Lost the connection. Getting back in…"
		} else if len(l.code) == link.CodeLength {
			code = showRoomCode(string(l.code))
		}
		f.DrawCentered(dst, text, cx, 136, 1, pal.Ice)
		f.DrawCentered(dst, code, cx, 162, 2, pal.White)
		hint = "Esc cancel"
	case link.PlayInRoom:
		hint = l.drawRoom(dst, ctx, st)
	}
	if st.Notice != "" {
		f.DrawCentered(dst, st.Notice, cx, 32, 1, pal.Yellow)
	}
	f.DrawShadow(dst, hint, 8, game.ScreenH-20, 1, pal.Ash)
}

func (l *Lobby) drawCode(dst *ebiten.Image, ctx *game.Context, st link.PlayState) string {
	f := ctx.Font
	cx := game.ScreenW / 2
	const x, w = 60, game.ScreenW - 120
	gfx.Window(dst, x, 56, w, 200)
	if !ctx.Link.Linked() {
		f.DrawCentered(dst, "To play together, link this game to", cx, 100, 1, pal.Ice)
		f.DrawCentered(dst, "a grown-up's account first, in Account.", cx, 118, 1, pal.Ice)
		f.DrawCentered(dst, "Then a parent or teacher can open a room", cx, 150, 1, pal.Steel)
		f.DrawCentered(dst, "and give you its code.", cx, 168, 1, pal.Steel)
		return "Esc back"
	}
	f.DrawCentered(dst, "Type the room code your grown-up gives you:", cx, 76, 1, pal.Ice)
	f.DrawCentered(dst, showRoomCode(string(l.code)), cx, 108, 4, pal.White)
	f.DrawCentered(dst, "In a room you can send friendly phrases", cx, 174, 1, pal.Steel)
	f.DrawCentered(dst, "and emotes. There is no chat.", cx, 192, 1, pal.Steel)
	msg := l.msg
	if msg == "" {
		msg = st.Problem
	}
	if msg != "" {
		f.DrawCentered(dst, fit(f, msg, w-24, 1), cx, 224, 1, pal.Rose)
	}
	return "Enter join   Esc back"
}

// drawRace shows the race under way: the countdown, or how far the
// racers have got.
func (l *Lobby) drawRace(dst *ebiten.Image, ctx *game.Context, st link.PlayState, x, y, w int) {
	f := ctx.Font
	rc := st.Race
	_, racing := rc.Racer(st.Room.You)
	switch left := time.Until(rc.StartsAt); {
	case left > 0 && racing:
		f.DrawShadow(dst, "Get ready to race!", x, y, 1, pal.Yellow)
		f.DrawCentered(dst, fmt.Sprint(int(left.Seconds())+1), x+w/2, y+26, 4, pal.White)
		f.DrawShadow(dst, fmt.Sprintf("First to floor %d wins.", rc.Goal), x, y+80, 1, pal.Ice)
		if l.bad == rc.Number {
			f.DrawShadow(dst, fit(f, "This game can't read the race's words.", w, 1), x, y+100, 1, pal.Rose)
		}
		return
	case left > 0:
		f.DrawShadow(dst, "A race is about to start.", x, y, 1, pal.Yellow)
	default:
		f.DrawShadow(dst, fmt.Sprintf("A race to floor %d is on.", rc.Goal), x, y, 1, pal.Yellow)
	}
	for i, r := range rc.Racers {
		ry := y + 24 + i*18
		name := racerName(st.Room, r.ID)
		if r.ID == st.Room.You {
			name += " (you)"
		}
		f.DrawShadow(dst, fit(f, name, w/2, 1), x, ry, 1, pal.Ice)
		f.DrawShadow(dst, fmt.Sprintf("floor %d · %s", r.Floor, statusText(r.Status)), x+w/2, ry, 1, pal.Steel)
	}
}

func (l *Lobby) drawRoom(dst *ebiten.Image, ctx *game.Context, st link.PlayState) string {
	f := ctx.Font
	r := st.Room

	// Who is in the room, in two columns.
	const mx, my, mw, mh = 8, 40, 300, 244
	gfx.Window(dst, mx, my, mw, mh)
	head := "Room " + showRoomCode(r.Code)
	if r.Mode == link.ModeRace {
		head = "Race room " + showRoomCode(r.Code)
	}
	if left := time.Until(r.EndsAt); !r.EndsAt.IsZero() && left > 0 {
		head += fmt.Sprintf("  ·  %d min left", int(left.Minutes())+1)
	}
	f.DrawShadow(dst, head, mx+10, my+8, 1, pal.Yellow)
	f.DrawShadow(dst, plural(len(r.Members), "player"), mx+10, my+24, 1, pal.Steel)
	members := sortedMembers(r)
	const rows, colW = 11, 140
	shown := min(len(members), 2*rows)
	if len(members) > 2*rows {
		shown = 2*rows - 1
	}
	for i, m := range members[:shown] {
		x, y := mx+10+(i/rows)*colW, my+46+(i%rows)*17
		c := pal.Ice
		switch {
		case m.Away:
			c = pal.Ash
		case m.ID == r.You:
			c = pal.White
		case m.Role != link.RoleLearner:
			c = pal.Lime
		}
		name := memberName(m, r.You)
		if m.Away {
			name += " (away)"
		}
		f.DrawShadow(dst, fit(f, name, colW-8, 1), x, y, 1, c)
	}
	if shown < len(members) {
		f.DrawShadow(dst, fmt.Sprintf("and %d more", len(members)-shown), mx+10+colW, my+46+(rows-1)*17, 1, pal.Steel)
	}

	// What happened.
	const ex, ew = 316, game.ScreenW - 316 - 8
	gfx.Window(dst, ex, my, ew, mh)
	y := my + 30
	shown = 11
	switch {
	case st.Race != nil:
		l.drawRace(dst, ctx, st, ex+10, my+8, ew-20)
		shown = 0
	case r.Mode == link.ModeRace && len(st.Results) > 0:
		f.DrawShadow(dst, "Last race", ex+10, my+8, 1, pal.Yellow)
		for _, res := range st.Results {
			drawRaceResult(dst, ctx, res, res.ID == r.You, ex+10, y, ew-20)
			y += 18
		}
		f.DrawShadow(dst, "What's happening", ex+10, y+6, 1, pal.Yellow)
		y += 28
		shown = max(0, 11-len(st.Results)-2)
	case r.Mode == link.ModeRace:
		f.DrawShadow(dst, "What's happening", ex+10, my+8, 1, pal.Yellow)
		f.DrawShadow(dst, "Waiting for the host to start a race.", ex+10, y, 1, pal.Lime)
		y += 18
		shown = 10
	default:
		f.DrawShadow(dst, "What's happening", ex+10, my+8, 1, pal.Yellow)
	}
	for _, e := range st.Events[max(0, len(st.Events)-shown):] {
		c := pal.Ice
		if e.Mine {
			c = pal.White
		}
		if e.Kind == "left" || e.Kind == "away" {
			c = pal.Steel
		}
		if t := eventText(e, r.You); t != "" {
			f.DrawShadow(dst, fit(f, t, ew-20, 1), ex+10, y, 1, c)
			y += 18
		}
	}

	// What to send.
	cy := my + mh + 8
	choices := chatChoices(st)
	gfx.Window(dst, mx, cy, game.ScreenW-16, 34)
	if len(choices) > 0 {
		c := choices[min(l.chat, len(choices)-1)]
		label := "◄  " + c.label() + "  ►"
		f.DrawCentered(dst, label, game.ScreenW/2, cy+9, 2, pal.White)
	}
	if st.Problem != "" {
		f.DrawShadow(dst, st.Problem, game.ScreenW-8-f.Width(st.Problem, 1), game.ScreenH-20, 1, pal.Tan)
	}

	if l.leaving {
		dx, dy := dialog(dst, ctx, "LEAVE THE ROOM?", 380, 130)
		f.DrawShadow(dst, "You can join again with the code", dx, dy, 1, pal.Ice)
		f.DrawShadow(dst, "while the room is open.", dx, dy+16, 1, pal.Ice)
		f.DrawShadow(dst, "Y leave   N stay", dx, dy+48, 1, pal.Ash)
		return ""
	}
	return "←/→ choose   Enter send   Esc leave"
}
