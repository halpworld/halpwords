package link

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/halpworld/halpwords/pkg/race"
)

// Playing together (halpwords-server's docs/api/play.md): a linked game
// joins a room by its code over one WebSocket, sees who is in it, and
// sends preset phrases and emotes. There is no free text: the game can
// only send the few messages below, with values from the lists the
// server sent. In a race room (W7.6) the host starts races: the game gets
// the dungeon's seed and word list, and reports how far the hero has got
// (Report). In a raid room (W7.5) the class fights a boss together: the
// server deals words and grades the answers (play_raid.go).
//
// Like the rest of the link, nothing here blocks the game loop: the
// connection runs in goroutines, and the game reads State each frame.

// PlayPhase is where the game is in playing together.
type PlayPhase int

const (
	// PlayOff is in no room and not connected: the lobby.
	PlayOff PlayPhase = iota
	// PlayJoining is connecting and joining a room.
	PlayJoining
	// PlayInRoom is in a room.
	PlayInRoom
	// PlayRejoining is getting back into the room after the connection
	// dropped.
	PlayRejoining
)

// The member roles.
const (
	RoleHost    = "host"
	RoleAdult   = "adult"
	RoleLearner = "learner"
)

// CodeLength is the length of a room code.
const CodeLength = 6

// codeAlphabet is the room codes' alphabet: Crockford's base 32.
const codeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Member is someone in a room. Learners are shown by their pseudonym
// (Name); adults only by their role.
type Member struct {
	ID   string `json:"id"`
	Role string `json:"role"`
	Name string `json:"name,omitempty"`
	Away bool   `json:"away,omitempty"`
}

// The room modes the game knows.
const (
	ModeLobby = "lobby"
	ModeRace  = "race"
)

// A racer's status (Racer.Status and Result.Status).
const (
	RacerRacing   = "racing"
	RacerFinished = "finished"
	RacerFell     = "fell"
	RacerLeft     = "left"
	// RacerNotCounted is a racer the server left out of the results: it
	// got a report the game can't have made.
	RacerNotCounted = "not-counted"
)

// Racer is someone in a race, and how far they have got.
type Racer struct {
	ID       string `json:"id"`
	Floor    int    `json:"floor"`
	Monsters int    `json:"monsters"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Status   string `json:"status"`
}

// Race is a race in the room: every racer's game builds the same dungeon
// from Seed and List.
type Race struct {
	// Number counts the races the game has seen, so the game can tell a
	// new race from the one it is running.
	Number int
	Seed   uint64
	// Goal is the floor that wins.
	Goal int
	// List is the word list in the game's text format.
	List string
	// StartsAt is when the race clock starts, by the game's clock;
	// EndsAt is the time limit.
	StartsAt, EndsAt time.Time
	Racers           []Racer
}

// Racer returns the racer with the member ID, and whether there is one.
func (r *Race) Racer(id string) (Racer, bool) {
	if r == nil {
		return Racer{}, false
	}
	i := slices.IndexFunc(r.Racers, func(x Racer) bool { return x.ID == id })
	if i < 0 {
		return Racer{}, false
	}
	return r.Racers[i], true
}

// Result is a racer's place in a race that has ended. Place is 0 for a
// racer who left or wasn't counted.
type Result struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Place    int           `json:"place"`
	Floor    int           `json:"floor"`
	Monsters int           `json:"monsters"`
	Time     time.Duration `json:"-"`
	Status   string        `json:"status"`
}

// Room is the room the game is in.
type Room struct {
	Code string
	Mode string
	// You is the player's member ID.
	You     string
	EndsAt  time.Time
	Members []Member
}

// PlayEvent is something that happened in the room, for the lobby's log.
type PlayEvent struct {
	// Kind is joined, left, away, back, said or emoted.
	Kind string
	// Member is who it was about, as they were then.
	Member Member
	// Mine is set when it was the player.
	Mine bool
	// Preset or Emote is what they sent; Reason why they left.
	Preset, Emote, Reason string
}

// PlayState is what the lobby shows.
type PlayState struct {
	Phase PlayPhase
	// Room is the room (in PlayInRoom, and as it was in PlayRejoining).
	Room Room
	// Events are the last things that happened in the room, oldest first.
	Events []PlayEvent
	// Presets and Emotes are what the player may send.
	Presets, Emotes []string
	// Problem says why the game is back in the lobby, or that a request
	// was refused; empty when all is well.
	Problem string
	// Notice is a notice from the server, such as that it will restart.
	Notice string
	// Race is the race under way in the room, or nil; Results are the
	// last race's.
	Race    *Race
	Results []Result
	// Raid is the raid under way in the room, or nil; Finale is how the
	// last one ended, and Summary what the player did in it.
	Raid    *Raid
	Finale  *RaidFinale
	Summary *RaidTally
	// Changes counts changes, so the game can tell something happened.
	Changes int
}

// Timings. They are variables so tests can shorten them.
var (
	// rejoinWindow is how long after a drop the game tries to get back
	// into the room; the server keeps the place for 60 seconds.
	rejoinWindow = 55 * time.Second
	// rejoinWait is the wait before rejoin try n (from 1).
	rejoinWait = func(n int) time.Duration { return min(time.Duration(1<<min(n-1, 3))*time.Second, 8*time.Second) }
	// defaultHeartbeat is how often the game pings until hello says.
	defaultHeartbeat = 20 * time.Second
	// chatEvery is the least time between two presets or emotes. The
	// server allows 1 a second (bursts of 3); the game keeps under it.
	chatEvery = time.Second
	// leaveWait is how long leaving waits to tell the server.
	leaveWait = 2 * time.Second
)

// maxEvents is how many events PlayState keeps.
const maxEvents = 12

// maxPlayMessage is the largest message the game takes from the server.
const maxPlayMessage = 64 << 10

// Play is the game's side of playing together. Get it with Client.Play.
type Play struct {
	c *Client

	mu sync.Mutex
	st PlayState
	// run counts joins and leaves; a connection of an older run changes
	// nothing.
	run    int
	cancel context.CancelFunc
	out    chan []byte   // the messages to send on this run's connection
	code   string        // the code of the room being joined or in
	seq    int64         // the room's last event
	resync bool          // waiting for a new room after a gap
	chat   time.Time     // when the last preset or emote went
	races  int           // the races seen, for Race.Number
	raids  int           // the raids seen, for Raid.Number
	report race.Reporter // when to send the race's next progress

	wg sync.WaitGroup // the runs' goroutines
}

// Play returns the game's playing together. It is nil when c is.
func (c *Client) Play() *Play {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.play == nil {
		c.play = &Play{c: c}
	}
	return c.play
}

// State is what the lobby shows now.
func (p *Play) State() PlayState {
	if p == nil {
		return PlayState{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.st
	s.Room.Members = slices.Clone(s.Room.Members)
	s.Events = slices.Clone(s.Events)
	s.Presets = slices.Clone(s.Presets)
	s.Emotes = slices.Clone(s.Emotes)
	s.Results = slices.Clone(s.Results)
	if s.Race != nil {
		r := *s.Race
		r.Racers = slices.Clone(r.Racers)
		s.Race = &r
	}
	s.Raid = s.Raid.clone()
	if s.Finale != nil {
		f := *s.Finale
		s.Finale = &f
	}
	if s.Summary != nil {
		t := *s.Summary
		s.Summary = &t
	}
	return s
}

// NormaliseCode turns a typed room code into its canonical form: upper
// case, without spaces or dashes, with O read as 0 and I and L as 1. It
// returns "" when that is not a room code.
func NormaliseCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch r {
		case ' ', '-':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		if r > 0x7f || !strings.ContainsRune(codeAlphabet, r) {
			return ""
		}
		b.WriteRune(r)
	}
	if b.Len() != CodeLength {
		return ""
	}
	return b.String()
}

// Join joins the room with the code, in the background. The game must be
// linked; a game in a room leaves it first.
func (p *Play) Join(code string) {
	if p == nil {
		return
	}
	code = NormaliseCode(code)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
	p.st = PlayState{Changes: p.st.Changes + 1, Notice: p.st.Notice}
	switch {
	case code == "":
		p.st.Problem = "A room code has 6 letters and numbers."
		return
	case !p.c.Linked():
		p.st.Problem = "Link this game to play together."
		return
	}
	p.st.Notice = ""
	p.st.Phase = PlayJoining
	p.code, p.seq, p.resync = code, 0, false
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	run := p.run
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.loop(ctx, run)
	}()
}

// Leave leaves the room, or stops joining one, and goes back to the
// lobby. It tells the server in the background.
func (p *Play) Leave() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
	p.st.Phase = PlayOff
	p.st.Room = Room{}
	p.st.Events = nil
	p.st.Problem = ""
	p.st.Race, p.st.Results = nil, nil
	p.st.Raid, p.st.Finale, p.st.Summary = nil, nil, nil
	p.st.Changes++
}

// stopLocked ends the current run: its connection says leave and
// closes, and it changes nothing any more. p.mu is held.
func (p *Play) stopLocked() {
	p.run++
	if p.out != nil {
		select {
		case p.out <- bye:
		default:
		}
		p.out = nil
	}
	if p.cancel != nil {
		// The writer closes the connection after saying leave; this is
		// in case it can't.
		time.AfterFunc(leaveWait, p.cancel)
		p.cancel = nil
	}
}

// bye is the leave message; the writer closes the connection after it.
var bye = []byte(`{"t":"leave"}`)

// Say sends a preset phrase to the room. It reports false when the game
// isn't in a room, the preset isn't one the server offers, or the last
// one went less than a second ago.
func (p *Play) Say(preset string) bool {
	return p.chatSend("say", "preset", preset, func(s *PlayState) []string { return s.Presets })
}

// Emote sends an emote to the room, like Say.
func (p *Play) Emote(emote string) bool {
	return p.chatSend("emote", "emote", emote, func(s *PlayState) []string { return s.Emotes })
}

func (p *Play) chatSend(t, field, value string, allowed func(*PlayState) []string) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if p.st.Phase != PlayInRoom || p.out == nil || !slices.Contains(allowed(&p.st), value) ||
		now.Sub(p.chat) < chatEvery {
		return false
	}
	msg, _ := json.Marshal(map[string]string{"t": t, field: value})
	select {
	case p.out <- msg:
		p.chat = now
		return true
	default:
		return false
	}
}

// Report tells the room how far the hero has got in the race. Call it
// every frame while racing: it sends r only when it changed, at most every
// race.ReportEvery, and at once for a new floor or a fall
// (race.Reporter). It reports whether r went. Nothing goes before the
// race starts, after it ends, or when the player isn't racing in it.
func (p *Play) Report(r race.Report) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	rc := p.st.Race
	if p.st.Phase != PlayInRoom || p.out == nil || rc == nil || now.Before(rc.StartsAt) {
		return false
	}
	if me, ok := rc.Racer(p.st.Room.You); !ok || me.Status != RacerRacing {
		return false
	}
	if !p.report.Due(r, now) {
		return false
	}
	msg, _ := json.Marshal(struct {
		T string `json:"t"`
		race.Report
	}{"progress", r})
	select {
	case p.out <- msg:
		return true
	default:
		// Try again next frame.
		p.report = race.Reporter{}
		return false
	}
}

// playEnd ends a run: the game is back in the lobby, with a problem to
// show (or none, when the player left).
type playEnd struct{ problem string }

func (e playEnd) Error() string { return "link: play ended: " + e.problem }

// loop joins the room and, while the player is in it, gets back in
// after a drop.
func (p *Play) loop(ctx context.Context, run int) {
	var (
		inRoom  bool      // this run got into the room
		dropped time.Time // when the connection dropped
		tries   int
	)
	for {
		got, err := p.session(ctx, run)
		inRoom = inRoom || got
		if ctx.Err() != nil {
			return
		}
		problem, again := p.explain(err)
		if got {
			dropped, tries = time.Now(), 0
		}
		if again && inRoom && time.Since(dropped) < rejoinWindow {
			tries++
			if !p.update(run, func() { p.st.Phase = PlayRejoining }) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(rejoinWait(tries)):
			}
			continue
		}
		if inRoom && again {
			problem = "The connection dropped for too long, so you left the room."
		}
		p.mu.Lock()
		if p.run == run {
			p.stopLocked()
			p.st.Phase = PlayOff
			p.st.Room = Room{}
			p.st.Problem = problem
			p.st.Race, p.st.Results = nil, nil
			p.st.Raid, p.st.Finale, p.st.Summary = nil, nil, nil
			p.st.Changes++
		}
		p.mu.Unlock()
		return
	}
}

// explain says why a run's connection ended, and whether trying again
// may work.
func (p *Play) explain(err error) (problem string, again bool) {
	var (
		end playEnd
		ce  *CloseError
		e   *Error
	)
	switch {
	case errors.As(err, &end):
		return end.problem, false
	case errors.Is(err, ErrNotLinked), errors.Is(err, ErrUnlinked):
		return "Link this game to play together.", false
	case errors.As(err, &ce):
		switch ce.Code {
		case 1001, 1006, 1011, 1013:
			return "Lost the connection to the server.", true
		case 1012:
			return "The server is restarting. Try again in a minute.", false
		case 1000:
			if ce.Reason == "replaced" {
				return "You joined from somewhere else.", false
			}
			return "You left the room.", false
		}
		return "The server closed the connection.", false
	case errors.As(err, &e):
		switch {
		case e.Status == http.StatusTooManyRequests:
			return "You are already playing together on two devices.", false
		case e.Status == http.StatusUnauthorized && e.Code == "bad_ticket":
			return "Lost the connection to the server.", true
		case e.Status >= 500:
			return "The server is busy. Try again in a minute.", true
		}
		return Explain(err), false
	case errors.Is(err, errProtocol), errors.Is(err, errTooBig):
		return "The server sent something the game doesn't understand.", false
	}
	return "Can't reach the server.", true
}

// update runs fn with p.mu held if run is still the current run, and
// reports whether it was.
func (p *Play) update(run int, fn func()) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.run != run {
		return false
	}
	fn()
	p.st.Changes++
	return true
}

// ticketReply is the answer of POST /api/v1/play/tickets.
type ticketReply struct {
	Ticket    string `json:"ticket"`
	URL       string `json:"url"`
	ExpiresIn int    `json:"expires_in"`
}

// session gets a ticket, connects, joins the room and plays until the
// connection ends. It reports whether it got into the room.
func (p *Play) session(ctx context.Context, run int) (bool, error) {
	c := p.c
	c.mu.Lock()
	gen := c.gen
	c.mu.Unlock()
	// Whatever talks to the server holds syncMu, so the tokens aren't
	// refreshed twice at once; a sync running now gets a moment.
	for !c.syncMu.TryLock() {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	var tr ticketReply
	_, _, err := c.authed(ctx, gen, http.MethodPost, "/api/v1/play/tickets", nil, nil, &tr)
	c.syncMu.Unlock()
	if err != nil {
		return false, err
	}
	u, err := url.Parse(c.server)
	if err != nil {
		return false, err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return false, errors.New("link: the server's address isn't http or https")
	}
	if !strings.HasPrefix(tr.URL, "/") || strings.HasPrefix(tr.URL, "//") || tr.Ticket == "" {
		return false, errors.New("link: a ticket without a place to use it")
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + tr.URL
	u.RawQuery = url.Values{"ticket": {tr.Ticket}}.Encode()
	hdr := http.Header{}
	if inBrowser {
		hdr.Set(clientHeader, c.clientVersion())
	} else {
		hdr.Set("User-Agent", c.userAgent())
	}
	dctx, cancel := context.WithTimeout(ctx, requestTimeout)
	sock, err := dialSocket(dctx, u, hdr, maxPlayMessage)
	cancel()
	if err != nil {
		return false, err
	}
	defer sock.Close(1000, "")
	stop := context.AfterFunc(ctx, func() { sock.Close(1000, "") })
	defer stop()

	out := make(chan []byte, 32)
	var heard atomic.Int64 // when the last message came, Unix nanoseconds
	heard.Store(time.Now().UnixNano())
	beat := make(chan time.Duration, 1)
	if !p.update(run, func() { p.out = out }) {
		return false, ctx.Err()
	}
	done := make(chan struct{})
	defer close(done)
	go p.write(sock, out, beat, &heard, done)

	inRoom := false
	first := true
	for {
		msg, err := sock.Read()
		if err != nil {
			return inRoom, err
		}
		heard.Store(time.Now().UnixNano())
		var m inMessage
		if err := json.Unmarshal(msg, &m); err != nil {
			sock.Close(1002, "")
			return inRoom, errProtocol
		}
		if first {
			if m.T != "hello" {
				sock.Close(1002, "")
				return false, errProtocol
			}
			first = false
			if m.Heartbeat > 0 {
				beat <- time.Duration(m.Heartbeat) * time.Second
			}
			var join []byte
			if !p.update(run, func() {
				p.st.Presets, p.st.Emotes = m.Presets, m.Emotes
				p.resync = false
				join, _ = json.Marshal(map[string]string{"t": "join", "code": p.code})
			}) {
				return inRoom, ctx.Err()
			}
			out <- join
			continue
		}
		var end error
		if !p.update(run, func() { end = p.handle(m, out, &inRoom) }) {
			return inRoom, ctx.Err()
		}
		if end != nil {
			return inRoom, end
		}
	}
}

// write sends the run's messages and a ping every heartbeat, and closes
// the connection when the server has said nothing for two heartbeats.
func (p *Play) write(sock socket, out chan []byte, beat chan time.Duration, heard *atomic.Int64, done chan struct{}) {
	every := defaultHeartbeat
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case d := <-beat:
			every = d
			tick.Reset(every)
		case msg := <-out:
			if err := sock.Write(msg); err != nil {
				sock.Close(1000, "")
				return
			}
			if &msg[0] == &bye[0] {
				sock.Close(1000, "")
				return
			}
		case <-tick.C:
			if time.Since(time.Unix(0, heard.Load())) > 2*every+5*time.Second {
				sock.Close(1001, "")
				return
			}
			if err := sock.Write([]byte(`{"t":"ping"}`)); err != nil {
				sock.Close(1000, "")
				return
			}
		}
	}
}

// raceMessage is a race as the server sends it.
type raceMessage struct {
	Seed     uint64  `json:"seed"`
	Goal     int     `json:"goal"`
	List     string  `json:"list"`
	StartsIn int64   `json:"starts_in"`
	EndsIn   int64   `json:"ends_in"`
	Racers   []Racer `json:"racers"`
}

// resultMessage is a result as the server sends it.
type resultMessage struct {
	Result
	Time int64 `json:"time"`
}

// inMessage is every field of a server message the game reads.
type inMessage struct {
	T         string   `json:"t"`
	Seq       int64    `json:"seq"`
	Heartbeat int      `json:"heartbeat"`
	Presets   []string `json:"presets"`
	Emotes    []string `json:"emotes"`
	Room      *struct {
		Code    string          `json:"code"`
		Mode    string          `json:"mode"`
		You     string          `json:"you"`
		EndsAt  int64           `json:"ends_at"`
		Members []Member        `json:"members"`
		Race    *raceMessage    `json:"race"`
		Results []resultMessage `json:"results"`
		Raid    *raidMessage    `json:"raid"`
		Finale  *finaleMessage  `json:"finale"`
	} `json:"room"`
	Race     *raceMessage    `json:"race"`
	Progress *Racer          `json:"progress"`
	Results  []resultMessage `json:"results"`
	Member   *Member         `json:"member"`
	ID       string          `json:"id"`
	Reason   string          `json:"reason"`
	Preset   string          `json:"preset"`
	Emote    string          `json:"emote"`
	Notice   string          `json:"notice"`
	In       int             `json:"in"`
	Code     string          `json:"code"`
	Raid     *raidMessage    `json:"raid"`
	Word     *wordMessage    `json:"word"`
	Graded   *gradedMessage  `json:"graded"`
	Damage   int             `json:"damage"`
	Boss     *bossMessage    `json:"boss"`
	Attack   *attackMessage  `json:"attack"`
	Finale   *finaleMessage  `json:"finale"`
	Summary  *RaidTally      `json:"summary"`
}

// The words for why a room ended, and for refused requests.
var (
	endedText = map[string]string{
		"host-left": "The host left, so the room ended.",
		"closed":    "The host ended the room.",
		"time":      "The room's time is up.",
		"shutdown":  "The server is restarting. Try again in a minute.",
		"replaced":  "You joined from somewhere else.",
	}
	joinErrorText = map[string]string{
		"not-found":     "No room has that code. Check it with your grown-up.",
		"full":          "That room is full.",
		"not-allowed":   "You can't join that room.",
		"slow-down":     "Too many tries. Wait a little and try again.",
		"busy":          "The server is busy. Try again in a minute.",
		"shutting-down": "The server is restarting. Try again in a minute.",
	}
)

// handle acts on a message from the server. p.mu is held. It returns a
// playEnd when the player is out of the room.
func (p *Play) handle(m inMessage, out chan []byte, inRoom *bool) error {
	switch m.T {
	case "room":
		if m.Room == nil {
			return nil
		}
		p.st.Room = Room{Code: m.Room.Code, Mode: m.Room.Mode, You: m.Room.You,
			EndsAt: time.UnixMilli(m.Room.EndsAt), Members: m.Room.Members}
		p.st.Phase = PlayInRoom
		p.st.Problem = ""
		p.seq, p.resync = m.Seq, false
		*inRoom = true
		p.setRace(m.Room.Race)
		p.st.Results = results(m.Room.Results)
		// Say where the hero is again, in case a report was lost.
		p.report = race.Reporter{}
		p.setRaid(m.Room.Raid, false)
		p.st.Finale = m.Room.Finale.finale()
		return nil
	case "notice":
		if m.Notice == "shutdown" {
			p.st.Notice = "The server is restarting soon."
		}
		return nil
	case "error":
		if !*inRoom {
			if p.st.Phase == PlayRejoining {
				switch m.Code {
				case "not-found":
					return playEnd{"The room has ended."}
				case "not-allowed":
					return playEnd{"The host removed you from the room."}
				}
			}
			if t, ok := joinErrorText[m.Code]; ok {
				return playEnd{t}
			}
			return playEnd{"You can't join that room."}
		}
		if m.Code == "slow-down" {
			p.st.Problem = "Slow down a little."
		}
		return nil
	case "ended":
		t := endedText[m.Reason]
		if t == "" {
			t = "The room ended."
		}
		return playEnd{t}
	case "word", "graded", "summary":
		// A raider's own: not room events.
		if *inRoom {
			p.handleRaid(m)
		}
		return nil
	case "joined", "left", "away", "back", "said", "emoted", "race", "progress", "results",
		"raid", "hit", "boss", "attack", "attacked", "finale":
	default:
		if m.Seq == 0 {
			// pong, and messages of later versions the game doesn't
			// know.
			return nil
		}
		// A room event of a later version: it still counts in the
		// sequence.
	}
	if !*inRoom || p.resync || m.Seq <= p.seq {
		return nil
	}
	if m.Seq != p.seq+1 {
		// Something was missed: joining the same room again sends it
		// whole.
		p.resync = true
		join, _ := json.Marshal(map[string]string{"t": "join", "code": p.code})
		select {
		case out <- join:
		default:
		}
		return nil
	}
	p.seq = m.Seq
	switch m.T {
	case "race":
		p.setRace(m.Race)
		p.st.Results = nil
		return nil
	case "progress":
		if rc := p.st.Race; rc != nil && m.Progress != nil {
			if i := slices.IndexFunc(rc.Racers, func(x Racer) bool { return x.ID == m.ID }); i >= 0 {
				pr := *m.Progress
				pr.ID = m.ID
				rc.Racers[i] = pr
			}
		}
		return nil
	case "results":
		p.st.Race = nil
		p.st.Results = results(m.Results)
		return nil
	case "raid", "hit", "boss", "attack", "attacked", "finale":
		p.raidEvent(m)
		return nil
	case "joined", "left", "away", "back", "said", "emoted":
	default:
		return nil
	}
	r := &p.st.Room
	id := m.ID
	if m.Member != nil {
		id = m.Member.ID
	}
	i := slices.IndexFunc(r.Members, func(x Member) bool { return x.ID == id })
	ev := PlayEvent{Kind: m.T, Mine: id == r.You, Preset: m.Preset, Emote: m.Emote, Reason: m.Reason}
	if i >= 0 {
		ev.Member = r.Members[i]
	}
	switch m.T {
	case "joined":
		if m.Member == nil {
			return nil
		}
		ev.Member = *m.Member
		if i >= 0 {
			r.Members[i] = *m.Member
		} else {
			r.Members = append(r.Members, *m.Member)
		}
	case "left":
		if i >= 0 {
			r.Members = slices.Delete(r.Members, i, i+1)
		}
		if ev.Mine {
			switch m.Reason {
			case "kicked":
				return playEnd{"The host removed you from the room."}
			case "timeout":
				return playEnd{"You were away too long, so you left the room."}
			}
			return playEnd{""}
		}
	case "away", "back":
		if i >= 0 {
			r.Members[i].Away = m.T == "away"
		}
	}
	p.st.Events = append(p.st.Events, ev)
	if n := len(p.st.Events) - maxEvents; n > 0 {
		p.st.Events = slices.Delete(p.st.Events, 0, n)
	}
	return nil
}

// setRace takes a race from the server (nil for none). The same race
// again, in a room sent whole after a rejoin, keeps its number. p.mu is
// held.
func (p *Play) setRace(m *raceMessage) {
	if m == nil {
		p.st.Race = nil
		return
	}
	now := time.Now()
	goal := m.Goal
	if goal < 1 {
		goal = race.Goal
	}
	rc := &Race{Seed: m.Seed, Goal: goal, List: m.List,
		StartsAt: now.Add(time.Duration(m.StartsIn) * time.Millisecond),
		EndsAt:   now.Add(time.Duration(m.EndsIn) * time.Millisecond),
		Racers:   m.Racers}
	if old := p.st.Race; old != nil && old.Seed == rc.Seed && old.List == rc.List {
		rc.Number = old.Number
		rc.StartsAt = old.StartsAt // a rejoin doesn't move the start
	} else {
		p.races++
		rc.Number = p.races
		p.report = race.Reporter{}
	}
	p.st.Race = rc
}

// results turns the server's results into the game's.
func results(ms []resultMessage) []Result {
	if len(ms) == 0 {
		return nil
	}
	out := make([]Result, len(ms))
	for i, m := range ms {
		out[i] = m.Result
		out[i].Time = time.Duration(m.Time) * time.Millisecond
	}
	return out
}
