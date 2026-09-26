//go:build !js

package link

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// testWS is the server's end of one play connection in a test.
type testWS struct {
	t    *testing.T
	conn net.Conn
	br   *bufio.Reader
	mu   sync.Mutex
}

func (s *testWS) send(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn.Write(serverFrame(true, opText, []byte(msg)))
}

// recv returns the next message from the game, skipping pings.
func (s *testWS) recv() map[string]any {
	s.t.Helper()
	for {
		s.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		op, p, err := readClientFrame(s.br)
		if err != nil {
			s.t.Errorf("reading from the game: %v", err)
			return nil
		}
		if op == opClose {
			return map[string]any{"t": "(close)", "code": float64(binary.BigEndian.Uint16(append(p, 0, 0)))}
		}
		var m map[string]any
		if err := json.Unmarshal(p, &m); err != nil {
			s.t.Errorf("the game sent %q", p)
		}
		if m["t"] != "ping" {
			return m
		}
	}
}

func (s *testWS) expect(t string) map[string]any {
	s.t.Helper()
	m := s.recv()
	if m["t"] != t {
		s.t.Errorf("the game sent %v, want %q", m, t)
	}
	return m
}

func (s *testWS) close(code int, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn.Write(serverFrame(true, opClose, closePayload(code, reason)))
	s.conn.Close()
}

const hello = `{"t":"hello","v":1,"heartbeat":20,"presets":["good-luck","nice-one","well-done","ready","help","thanks"],"emotes":["wave","thumbs-up","clap","smile"],"modes":["lobby"],"can_host":false}`

// roomOf is a room message with the host, the player (m2) and others.
func roomOf(seq int, others ...string) string {
	members := []string{`{"id":"m1","role":"host"}`, `{"id":"m2","role":"learner","name":"Brave Otter"}`}
	members = append(members, others...)
	return `{"t":"room","seq":` + itoa(seq) + `,"room":{"code":"ABCDEF","mode":"lobby","you":"m2","ends_at":1790000000000,"members":[` + strings.Join(members, ",") + `]}}`
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// playServer makes the fake answer /api/v1/play: it checks the ticket,
// opens the WebSocket, says hello, and hands each connection to conns.
func playServer(t *testing.T, f *fake) chan *testWS {
	conns := make(chan *testWS, 8)
	f.playMu.Lock()
	defer f.playMu.Unlock()
	f.playWS = func(w http.ResponseWriter, r *http.Request) {
		ticket := r.URL.Query().Get("ticket")
		f.mu.Lock()
		ok := f.tickets[ticket]
		delete(f.tickets, ticket)
		f.agents = append(f.agents, r.Header.Get("User-Agent"))
		f.mu.Unlock()
		if !ok {
			writeErr(w, 401, "bad_ticket")
			return
		}
		if r.Header.Get("Upgrade") != "websocket" || r.Header.Get("Sec-WebSocket-Version") != "13" {
			writeErr(w, 400, "not_websocket")
			return
		}
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
			acceptKey(r.Header.Get("Sec-WebSocket-Key")) + "\r\n\r\n")
		brw.Flush()
		s := &testWS{t: t, conn: conn, br: brw.Reader}
		s.send(hello)
		conns <- s
	}
	return conns
}

func next(t *testing.T, conns chan *testWS) *testWS {
	t.Helper()
	select {
	case s := <-conns:
		t.Cleanup(func() { s.conn.Close() })
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("the game didn't connect")
		return nil
	}
}

// waitFor waits until the play state is as ok says.
func waitFor(t *testing.T, p *Play, what string, ok func(PlayState) bool) PlayState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s := p.State()
		if ok(s) {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s: %+v", what, s)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// playing sets up a linked game and a fake that answers play, with
// short timings. The game's connections are gone when the test ends.
func playing(t *testing.T) (*fake, *Client, chan *testWS) {
	old, oldChat, oldLeave := rejoinWait, chatEvery, leaveWait
	rejoinWait = func(int) time.Duration { return 10 * time.Millisecond }
	leaveWait = 50 * time.Millisecond
	t.Cleanup(func() { rejoinWait, chatEvery, leaveWait = old, oldChat, oldLeave })
	f, c, _, _ := linked(t)
	conns := playServer(t, f)
	t.Cleanup(func() {
		p := c.Play()
		p.Leave()
		p.wg.Wait()
	})
	return f, c, conns
}

func joinRoom(t *testing.T, f *fake, c *Client, conns chan *testWS) *testWS {
	t.Helper()
	c.Play().Join("abc-def")
	s := next(t, conns)
	if m := s.expect("join"); m["code"] != "ABCDEF" {
		t.Fatalf("joined %v", m)
	}
	s.send(roomOf(4, `{"id":"m3","role":"learner","name":"Quiet Fox"}`))
	waitFor(t, c.Play(), "the room", func(s PlayState) bool { return s.Phase == PlayInRoom })
	return s
}

func TestPlayJoinAndChat(t *testing.T) {
	f, c, conns := playing(t)
	p := c.Play()
	s := joinRoom(t, f, c, conns)
	st := p.State()
	if st.Room.Code != "ABCDEF" || st.Room.You != "m2" || len(st.Room.Members) != 3 || len(st.Presets) != 6 {
		t.Fatalf("room %+v", st)
	}
	if ua := f.agents[len(f.agents)-1]; !strings.HasPrefix(ua, "Halpwords/") {
		t.Fatalf("the socket's User-Agent is %q", ua)
	}

	s.send(`{"t":"joined","seq":5,"member":{"id":"m4","role":"adult"}}`)
	s.send(`{"t":"said","seq":6,"id":"m3","preset":"good-luck"}`)
	s.send(`{"t":"away","seq":7,"id":"m3"}`)
	s.send(`{"t":"something-new","seq":0}`) // a later version's message
	st = waitFor(t, p, "three events", func(s PlayState) bool { return len(s.Events) == 3 })
	if e := st.Events[1]; e.Kind != "said" || e.Member.Name != "Quiet Fox" || e.Preset != "good-luck" || e.Mine {
		t.Fatalf("said: %+v", e)
	}
	if len(st.Room.Members) != 4 || !st.Room.Members[2].Away {
		t.Fatalf("members %+v", st.Room.Members)
	}

	// Only presets and emotes the server offers, and not too fast.
	if p.Say("meet me at the park") || p.Emote("wink") {
		t.Fatal("sent something not on the lists")
	}
	if !p.Say("well-done") {
		t.Fatal("didn't say well-done")
	}
	if p.Emote("wave") {
		t.Fatal("sent twice in a second")
	}
	if m := s.expect("say"); m["preset"] != "well-done" || len(m) != 2 {
		t.Fatalf("said %v", m)
	}
	chatEvery = 0
	if !p.Emote("wave") {
		t.Fatal("didn't wave")
	}
	if m := s.expect("emote"); m["emote"] != "wave" {
		t.Fatalf("emoted %v", m)
	}
	s.send(`{"t":"emoted","seq":8,"id":"m2","emote":"wave"}`)
	st = waitFor(t, p, "my wave", func(s PlayState) bool { return len(s.Events) == 4 })
	if !st.Events[3].Mine {
		t.Fatalf("my wave: %+v", st.Events[3])
	}

	s.send(`{"t":"notice","notice":"shutdown","in":5}`)
	waitFor(t, p, "the notice", func(s PlayState) bool { return s.Notice != "" })
	s.send(`{"t":"ended","seq":9,"reason":"closed"}`)
	st = waitFor(t, p, "the lobby", func(s PlayState) bool { return s.Phase == PlayOff })
	if st.Problem != "The host ended the room." {
		t.Fatalf("problem %q", st.Problem)
	}
	if p.Say("thanks") {
		t.Fatal("said something in no room")
	}
}

func TestPlayResyncsAfterAGap(t *testing.T) {
	f, c, conns := playing(t)
	p := c.Play()
	s := joinRoom(t, f, c, conns)
	s.send(`{"t":"joined","seq":6,"member":{"id":"m9","role":"learner","name":"Lost One"}}`) // 5 is missing
	if m := s.expect("join"); m["code"] != "ABCDEF" {
		t.Fatalf("resync: %v", m)
	}
	s.send(`{"t":"said","seq":7,"id":"m3","preset":"ready"}`) // before the room: ignored
	s.send(roomOf(7, `{"id":"m5","role":"learner","name":"Green Owl"}`, `{"id":"m9","role":"learner","name":"Lost One"}`))
	s.send(`{"t":"said","seq":8,"id":"m9","preset":"help"}`)
	st := waitFor(t, p, "the event after", func(s PlayState) bool { return len(s.Events) == 1 })
	if len(st.Room.Members) != 4 || st.Events[0].Member.Name != "Lost One" {
		t.Fatalf("after resync: %+v", st)
	}
}

func TestPlayRejoinsAfterADrop(t *testing.T) {
	f, c, conns := playing(t)
	p := c.Play()
	s := joinRoom(t, f, c, conns)
	s.conn.Close() // dropped, without a close
	waitFor(t, p, "rejoining", func(s PlayState) bool { return s.Phase == PlayRejoining })
	s2 := next(t, conns)
	if m := s2.expect("join"); m["code"] != "ABCDEF" {
		t.Fatalf("rejoined %v", m)
	}
	s2.send(roomOf(6))
	st := waitFor(t, p, "back in", func(s PlayState) bool { return s.Phase == PlayInRoom })
	if len(st.Room.Members) != 2 {
		t.Fatalf("members %+v", st.Room.Members)
	}
	// The room ended while the game was away.
	s2.close(1001, "gone")
	s3 := next(t, conns)
	s3.expect("join")
	s3.send(`{"t":"error","code":"not-found"}`)
	st = waitFor(t, p, "the lobby", func(s PlayState) bool { return s.Phase == PlayOff })
	if st.Problem != "The room has ended." {
		t.Fatalf("problem %q", st.Problem)
	}
}

func TestPlayDoesNotRejoin(t *testing.T) {
	for _, tc := range []struct {
		name    string
		end     func(*testWS)
		problem string
	}{
		{"shutdown", func(s *testWS) { s.close(1012, "shutdown") }, "The server is restarting. Try again in a minute."},
		{"bad message", func(s *testWS) { s.close(1008, "bad-message") }, "The server closed the connection."},
		{"kicked", func(s *testWS) { s.send(`{"t":"left","seq":5,"id":"m2","reason":"kicked"}`) }, "The host removed you from the room."},
		{"replaced", func(s *testWS) { s.send(`{"t":"ended","reason":"replaced"}`) }, "You joined from somewhere else."},
		{"binary", func(s *testWS) { s.conn.Write(serverFrame(true, opBinary, []byte("x"))) }, "The server sent something the game doesn't understand."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, conns := playing(t)
			s := joinRoom(t, f, c, conns)
			tc.end(s)
			st := waitFor(t, c.Play(), "the lobby", func(s PlayState) bool { return s.Phase == PlayOff })
			if st.Problem != tc.problem {
				t.Fatalf("problem %q", st.Problem)
			}
			select {
			case <-conns:
				t.Fatal("the game connected again")
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}

func TestPlayJoinRefused(t *testing.T) {
	f, c, conns := playing(t)
	p := c.Play()
	p.Join("zzzzzz")
	s := next(t, conns)
	s.expect("join")
	s.send(`{"t":"error","code":"not-found"}`)
	st := waitFor(t, p, "the lobby", func(s PlayState) bool { return s.Phase == PlayOff })
	if !strings.Contains(st.Problem, "No room has that code") {
		t.Fatalf("problem %q", st.Problem)
	}

	// Not a code: nothing is sent.
	for _, code := range []string{"", "abc", "abcdefg", "ab!def", "ABCDÉF"} {
		p.Join(code)
		if st := p.State(); st.Phase != PlayOff || st.Problem == "" {
			t.Fatalf("%q: %+v", code, st)
		}
	}
	if NormaliseCode("abc-def") != "ABCDEF" || NormaliseCode(" o1l-i2 3") != "0111"+"23" {
		t.Fatalf("normalised %q", NormaliseCode(" o1l-i2 3"))
	}

	// The server refuses the socket: too many connections.
	f.playMu.Lock()
	f.playWS = func(w http.ResponseWriter, r *http.Request) { writeErr(w, 429, "too_many_connections") }
	f.playMu.Unlock()
	p.Join("ABCDEF")
	st = waitFor(t, p, "the lobby", func(s PlayState) bool { return s.Phase == PlayOff && s.Problem != "" })
	if !strings.Contains(st.Problem, "two devices") {
		t.Fatalf("problem %q", st.Problem)
	}
}

func TestPlayNeedsALink(t *testing.T) {
	_, c, _, _ := setup(t)
	c.Play().Join("ABCDEF")
	if st := c.Play().State(); st.Phase != PlayOff || !strings.Contains(st.Problem, "Link") {
		t.Fatalf("%+v", st)
	}
}

func TestPlayLeaveTellsTheServer(t *testing.T) {
	f, c, conns := playing(t)
	s := joinRoom(t, f, c, conns)
	c.Play().Leave()
	if st := c.Play().State(); st.Phase != PlayOff || st.Problem != "" {
		t.Fatalf("%+v", c.Play().State())
	}
	s.expect("leave")
	if m := s.recv(); m["t"] != "(close)" || m["code"] != float64(1000) {
		t.Fatalf("after leave: %v", m)
	}
	// Unlinking leaves too.
	s = joinRoom(t, f, c, conns)
	c.Unlink()
	s.expect("leave")
	if st := c.Play().State(); st.Phase != PlayOff {
		t.Fatalf("%+v", st)
	}
}

func TestDialChecksTheHandshake(t *testing.T) {
	f, c, _ := playing(t)
	f.playMu.Lock()
	f.playWS = func(w http.ResponseWriter, r *http.Request) {
		conn, brw, _ := http.NewResponseController(w).Hijack()
		defer conn.Close()
		brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: wrong\r\n\r\n")
		brw.Flush()
	}
	f.playMu.Unlock()
	c.Play().Join("ABCDEF")
	st := waitFor(t, c.Play(), "the lobby", func(s PlayState) bool { return s.Phase == PlayOff && s.Problem != "" })
	if st.Problem != "Can't reach the server." {
		t.Fatalf("problem %q", st.Problem)
	}
}
