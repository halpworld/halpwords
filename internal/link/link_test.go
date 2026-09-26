package link

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/pkg/words"
)

// memStore keeps files in memory.
type memStore struct {
	mu      sync.Mutex
	files   map[string][]byte
	private map[string]bool
}

func newMemStore() *memStore {
	return &memStore{files: map[string][]byte{}, private: map[string]bool{}}
}

func (m *memStore) Read(name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return d, nil
}

func (m *memStore) Write(name string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[name] = append([]byte(nil), data...)
	delete(m.private, name)
	return nil
}

func (m *memStore) WritePrivate(name string, data []byte) error {
	m.Write(name, data)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.private[name] = true
	return nil
}

func (m *memStore) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, name)
	return nil
}

func (m *memStore) has(name string) bool {
	_, err := m.Read(name)
	return err == nil
}

// The patterns of docs/api/events.request.schema.json.
var (
	timeRE = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2})$`)
	modeRE = regexp.MustCompile(`^[a-z][a-z_]{0,19}(:[a-z][a-z_]{0,19})?$`)
	sessRE = regexp.MustCompile(`^[a-z][a-z_]{0,19}$`)
	dayRE  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	uaRE   = regexp.MustCompile(`^Halpwords/(dev|[0-9]+\.[0-9]+\.[0-9]+\S*) \(`)
)

// event is an event as the fake server keeps it.
type event struct {
	Kind string
	Seq  int64
	Raw  map[string]any
}

// fake is a pretend halpwords-server: the game API of docs/api, with
// ways to make it fail.
type fake struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	code     string // the pairing code that works
	linkedAs string
	access   string
	refresh  string
	used     map[string]bool // refresh tokens already swapped
	n        int             // tokens handed out
	expires  int             // expires_in of access tokens
	lists    []wireList
	etag     string
	memory   map[string]*words.Memory
	me       map[string]any
	events   map[int64]event
	lastSeq  int64
	batches  int
	calls    map[string]int
	notMod   int              // 304 answers
	fail     map[string][]int // statuses to answer on the next calls to a path
	refuse   map[int64]string
	tooLarge int // answer 413 to batches bigger than this; 0 never
	agents   []string
	clients  []string // X-Halpwords-Client headers
	unlinked int      // devices unlinked by the game
}

func newFake(t *testing.T) *fake {
	f := &fake{t: t, code: "ABCD-EFGH", used: map[string]bool{}, expires: 86400,
		memory: map[string]*words.Memory{}, events: map[int64]event{}, calls: map[string]int{},
		fail: map[string][]int{}, refuse: map[int64]string{}}
	f.me = map[string]any{
		"learner":        map[string]any{"id": "lrn_1", "display_name": "Aoife", "avatar": map[string]any{"class": "scribe", "colour": "#aabbcc"}, "languages": []string{"fr"}},
		"settings":       map[string]any{},
		"accommodations": map[string]any{},
		"seen_by":        []string{"guardian", "teacher"},
		"game":           map[string]any{"version": "1.2.0", "min_version": "1.1.0", "supported": true},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) setLists(lists ...wireList) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = lists
	var ids []string
	for _, l := range lists {
		ids = append(ids, fmt.Sprintf("%s@%d", l.ID, l.Version))
	}
	f.etag = `"` + strings.Join(ids, ",") + `"`
}

func (f *fake) failNext(path string, statuses ...int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[path] = append(f.fail[path], statuses...)
}

func (f *fake) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[path]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": "fake " + code}})
}

func (f *fake) tokens() map[string]any {
	f.n++
	f.access = fmt.Sprintf("hwd_%d", f.n)
	f.refresh = fmt.Sprintf("hwr_%d", f.n)
	return map[string]any{"device_id": "dev_1", "token_type": "Bearer", "access_token": f.access,
		"expires_in": f.expires, "refresh_token": f.refresh, "refresh_expires_in": 90 * 86400}
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := r.URL.Path
	f.calls[path]++
	f.agents = append(f.agents, r.Header.Get("User-Agent"))
	f.clients = append(f.clients, r.Header.Get("X-Halpwords-Client"))
	if q := f.fail[path]; len(q) > 0 {
		f.fail[path] = q[1:]
		if q[0] == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "1")
		}
		writeErr(w, q[0], map[int]string{401: "unauthenticated", 429: "rate_limited", 500: "internal", 503: "internal", 413: "too_large"}[q[0]])
		return
	}
	body, _ := io.ReadAll(r.Body)
	auth := func() bool {
		if r.Header.Get("Authorization") != "Bearer "+f.access || f.access == "" {
			writeErr(w, 401, "unauthenticated")
			return false
		}
		return true
	}
	switch {
	case r.Method == "POST" && path == "/api/v1/link":
		var req struct{ Code, Name string }
		json.Unmarshal(body, &req)
		norm := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(req.Code))
		if norm != strings.ReplaceAll(f.code, "-", "") {
			writeErr(w, 400, "invalid_code")
			return
		}
		f.code = "USED-USED"
		f.linkedAs = req.Name
		writeJSON(w, 200, f.tokens())
	case r.Method == "POST" && path == "/api/v1/token":
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		json.Unmarshal(body, &req)
		switch {
		case f.used[req.RefreshToken]:
			f.access, f.refresh = "", ""
			writeErr(w, 401, "token_reused")
		case req.RefreshToken != f.refresh || f.refresh == "":
			writeErr(w, 401, "invalid_token")
		default:
			f.used[req.RefreshToken] = true
			writeJSON(w, 200, f.tokens())
		}
	case r.Method == "POST" && path == "/api/v1/unlink":
		if auth() {
			f.access, f.refresh = "", ""
			f.unlinked++
			w.WriteHeader(http.StatusNoContent)
		}
	case r.Method == "GET" && path == "/api/v1/me":
		if auth() {
			writeJSON(w, 200, f.me)
		}
	case r.Method == "GET" && path == "/api/v1/lists":
		if !auth() {
			return
		}
		w.Header().Set("ETag", f.etag)
		if r.Header.Get("If-None-Match") == f.etag && f.etag != "" {
			f.notMod++
			w.WriteHeader(304)
			return
		}
		lists := f.lists
		if lists == nil {
			lists = []wireList{}
		}
		writeJSON(w, 200, map[string]any{"lists": lists})
	case r.Method == "GET" && path == "/api/v1/assignments":
		if auth() {
			writeJSON(w, 200, map[string]any{"assignments": []any{}})
		}
	case r.Method == "GET" && path == "/api/v1/memory":
		if !auth() {
			return
		}
		lang := r.URL.Query().Get("lang")
		m := f.memory[lang]
		if m == nil {
			m = words.NewMemory()
		}
		writeJSON(w, 200, map[string]any{"lang": lang, "answers": 0, "built_at": nil, "memory": m})
	case r.Method == "POST" && path == "/api/v1/events":
		if !auth() {
			return
		}
		f.postEvents(w, body)
	default:
		writeErr(w, 404, "not_found")
	}
}

// postEvents checks a batch as docs/api/events.request.schema.json and
// the server's gamesync do, and stores it.
func (f *fake) postEvents(w http.ResponseWriter, body []byte) {
	var b map[string][]map[string]any
	if err := json.Unmarshal(body, &b); err != nil {
		writeErr(w, 400, "invalid_request")
		return
	}
	n := len(b["answers"]) + len(b["sessions"]) + len(b["totals"])
	if f.tooLarge > 0 && n > f.tooLarge {
		writeErr(w, 413, "too_large")
		return
	}
	if n == 0 || n > MaxBatch {
		f.t.Errorf("a batch of %d events", n)
		writeErr(w, 400, "invalid_request")
		return
	}
	for k := range b {
		if k != "answers" && k != "sessions" && k != "totals" {
			f.t.Errorf("unknown array %q", k)
		}
	}
	seen := map[int64]bool{}
	var details []map[string]string
	check := func(kind string, i int, e map[string]any, required ...string) {
		for _, r := range required {
			if _, ok := e[r]; !ok {
				f.t.Errorf("%s %d: no %s: %v", kind, i, r, e)
			}
		}
		seq := int64(e["seq"].(float64))
		if seq < 1 || seen[seq] {
			f.t.Errorf("%s %d: seq %d reused or < 1", kind, i, seq)
		}
		seen[seq] = true
		if at, ok := e["at"].(string); ok && !timeRE.MatchString(at) {
			f.t.Errorf("%s %d: at %q", kind, i, at)
		}
		if bad, ok := e["bad"]; ok && bad == true {
			details = append(details, map[string]string{"path": fmt.Sprintf("/%s/%d/bad", kind, i), "message": "no"})
		}
	}
	for i, a := range b["answers"] {
		check("answers", i, a, "seq", "at", "list_id", "list_version", "word", "mode", "tier")
		if !modeRE.MatchString(a["mode"].(string)) {
			f.t.Errorf("answer mode %q", a["mode"])
		}
		if t := a["tier"].(float64); t < 0 || t > 4 {
			f.t.Errorf("tier %v", t)
		}
	}
	for i, s := range b["sessions"] {
		check("sessions", i, s, "seq", "at", "mode", "lang", "secs")
		if !sessRE.MatchString(s["mode"].(string)) {
			f.t.Errorf("session mode %q", s["mode"])
		}
	}
	for i, t := range b["totals"] {
		check("totals", i, t, "seq", "day", "lang", "answers", "right", "secs")
		if !dayRE.MatchString(t["day"].(string)) {
			f.t.Errorf("day %q", t["day"])
		}
		if t["right"].(float64) > t["answers"].(float64) {
			f.t.Errorf("right > answers: %v", t)
		}
	}
	if len(details) > 0 {
		writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "invalid_request", "message": "bad", "details": details}})
		return
	}
	f.batches++
	refused := []map[string]any{}
	stored, dups := 0, 0
	for kind, list := range b {
		for _, e := range list {
			seq := int64(e["seq"].(float64))
			f.lastSeq = max(f.lastSeq, seq)
			if code := f.refuse[seq]; code != "" {
				refused = append(refused, map[string]any{"seq": seq, "code": code, "message": "no"})
				continue
			}
			if _, ok := f.events[seq]; ok {
				dups++
				continue
			}
			f.events[seq] = event{Kind: kind, Seq: seq, Raw: e}
			stored++
		}
	}
	writeJSON(w, 200, map[string]any{"last_seq": f.lastSeq, "stored": stored, "duplicates": dups, "refused": refused})
}

func (f *fake) eventsOf(kind string) []event {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []event
	for _, e := range f.events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// clock is a test clock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

const animals = "title: Animals\nlanguage: fr\nid: lst_animals\nversion: 3\n\ndog = le chien\ncat = le chat\n"

var (
	dog = words.Entry{Prompt: "dog", Answers: []string{"le chien"}}
	cat = words.Entry{Prompt: "cat", Answers: []string{"le chat"}}
	own = words.Entry{Prompt: "house", Answers: []string{"la maison"}}
)

// setup returns a fake server and a client for it, not yet linked.
func setup(t *testing.T) (*fake, *Client, *memStore, *clock) {
	t.Helper()
	f := newFake(t)
	st := newMemStore()
	clk := &clock{t: time.Date(2026, 9, 26, 14, 5, 9, 0, time.FixedZone("CEST", 2*3600))}
	c := Open(Options{Store: st, Server: f.srv.URL, Version: "v1.2.0-3-gabcdef", OwnDir: "words", Now: clk.now})
	c.sleep = func(time.Duration) {}
	return f, c, st, clk
}

// linked returns a linked client with the Animals list assigned and
// synced.
func linked(t *testing.T) (*fake, *Client, *memStore, *clock) {
	t.Helper()
	f, c, st, clk := setup(t)
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Words: 2, Text: animals})
	if err := c.LinkNow(context.Background(), "abcd efgh"); err != nil {
		t.Fatal(err)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f, c, st, clk
}

func TestLink(t *testing.T) {
	f, c, st, _ := setup(t)
	if c.Linked() || c.AccessToken() != "" {
		t.Fatal("linked before linking")
	}
	if err := c.LinkNow(context.Background(), "WXYZ-WXYZ"); !errors.Is(err, ErrBadCode) {
		t.Fatalf("wrong code: %v", err)
	}
	if err := c.LinkNow(context.Background(), "abc"); !errors.Is(err, ErrBadCode) {
		t.Fatalf("short code: %v", err)
	}
	if err := c.LinkNow(context.Background(), "abcd-efgh"); err != nil {
		t.Fatal(err)
	}
	if !c.Linked() || c.AccessToken() != "hwd_1" {
		t.Fatalf("not linked: %+v", c.Status())
	}
	if !st.private[stateFile] {
		t.Error("link.json is not private")
	}
	if strings.Contains(string(st.files[stateFile]), "hwr_1") == false {
		t.Error("the refresh token was not kept")
	}
	if f.linkedAs == "" {
		t.Error("the game sent no name")
	}
	if err := c.LinkNow(context.Background(), "abcd-efgh"); !errors.Is(err, ErrLinked) {
		t.Errorf("linking twice: %v", err)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := c.Status()
	if s.Learner != "Aoife" || s.LastSync.IsZero() || s.Err != nil || s.Busy {
		t.Errorf("status: %+v", s)
	}
	if got := SeenByText(s.SeenBy); got != "Your parent and your teacher can see your progress." {
		t.Errorf("seen by: %q", got)
	}
	for _, ua := range f.agents {
		if !uaRE.MatchString(ua) || !strings.HasPrefix(ua, "Halpwords/1.2.0-3-gabcdef (") {
			t.Errorf("User-Agent %q", ua)
		}
	}
	// A new client reads the link from the store.
	c2 := Open(Options{Store: st, Server: f.srv.URL})
	if !c2.Linked() || c2.Status().Learner != "Aoife" {
		t.Error("the link didn't survive a restart")
	}
}

func TestLinkAsync(t *testing.T) {
	f, c, _, _ := setup(t)
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Words: 2, Text: animals})
	c.Link("ABCDEFGH")
	deadline := time.Now().Add(5 * time.Second)
	for c.Status().Busy || !c.Linked() || len(c.Lists()) == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("still linking: %+v", c.Status())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSyncSendsAnswersSessionsAndTotals(t *testing.T) {
	f, c, st, clk := linked(t)
	lists := c.Lists()
	if len(lists) != 1 || lists[0].ID != "lst_animals" || !IsAssigned(lists[0]) || !st.has("assigned/lst_animals.txt") {
		t.Fatalf("lists: %v", lists)
	}
	c.Answer("fr", dog, "adventure:attack", words.Answer{Tier: words.Perfect, Timed: true, Secs: 2.5})
	c.Answer("fr", cat, "Practice", words.Answer{Tier: words.Graze, Mistake: words.LetterMistake})
	c.Answer("fr", own, "adventure:dodge", words.Answer{Tier: words.Correct, Timed: true, Secs: 3})
	c.Answer("fr", own, "adventure:dodge", words.Answer{Tier: words.Miss, Timed: true, Secs: 4})
	clk.add(time.Minute)
	c.Session(Session{Start: clk.now().Add(-10 * time.Minute), Mode: "adventure", Lang: "fr", Secs: 600, Floor: 3})
	c.Session(Session{Start: clk.now(), Mode: "practice", Lang: "fr", Secs: 2}) // too short
	if s := c.Status(); s.Pending != 4 {
		t.Fatalf("pending %d, want 2 answers, 1 totals and 1 session", s.Pending)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := c.Status(); s.Pending != 0 {
		t.Fatalf("still pending: %d", s.Pending)
	}
	answers := f.eventsOf("answers")
	if len(answers) != 2 {
		t.Fatalf("answers sent: %v", answers)
	}
	for _, a := range answers {
		r := a.Raw
		if r["list_id"] != "lst_animals" || r["list_version"] != 3.0 {
			t.Errorf("answer names %v", r)
		}
		switch r["word"] {
		case "dog = le chien":
			if r["mode"] != "adventure:attack" || r["tier"] != 4.0 || r["secs"] != 2.5 || r["mistake"] != nil {
				t.Errorf("dog: %v", r)
			}
		case "cat = le chat":
			if r["mode"] != "practice" || r["tier"] != 1.0 || r["secs"] != nil || r["mistake"] != float64(words.LetterMistake) {
				t.Errorf("cat: %v", r)
			}
		default:
			t.Errorf("word %v", r["word"])
		}
		if r["at"] != "2026-09-26T14:05:09+02:00" {
			t.Errorf("at %v", r["at"])
		}
	}
	totals := f.eventsOf("totals")
	if len(totals) != 1 || totals[0].Raw["answers"] != 2.0 || totals[0].Raw["right"] != 1.0 ||
		totals[0].Raw["secs"] != 7.0 || totals[0].Raw["day"] != "2026-09-26" || totals[0].Raw["lang"] != "fr" {
		t.Errorf("totals: %v", totals)
	}
	sessions := f.eventsOf("sessions")
	if len(sessions) != 1 || sessions[0].Raw["floor"] != 3.0 || sessions[0].Raw["secs"] != 600.0 {
		t.Errorf("sessions: %v", sessions)
	}
	// Sequence numbers keep rising after a restart.
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.Save()
	c2 := Open(Options{Store: st, Server: f.srv.URL, Now: clk.now})
	c2.sleep = func(time.Duration) {}
	if c2.Status().Pending != 1 {
		t.Fatal("the queue didn't survive a restart")
	}
	c2.Answer("fr", cat, "practice", words.Answer{Tier: words.Perfect})
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(f.eventsOf("answers")); n != 4 {
		t.Errorf("%d answers stored, want 4 (no sequence number reused)", n)
	}
}

func TestResendIsSafe(t *testing.T) {
	f, c, _, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	// The server stores the batch but its answer is lost.
	c.mu.Lock()
	b, _ := c.takeBatch(MaxBatch)
	c.q.sending = nil
	c.mu.Unlock()
	data, _ := json.Marshal(b)
	req, _ := http.NewRequest("POST", f.srv.URL+"/api/v1/events", strings.NewReader(string(data)))
	req.Header.Set("Authorization", "Bearer "+c.AccessToken())
	if resp, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(f.eventsOf("answers")); n != 1 || c.Status().Pending != 0 {
		t.Errorf("%d answers, %d pending", n, c.Status().Pending)
	}
}

func TestListsETagAndUnassigning(t *testing.T) {
	f, c, st, _ := linked(t)
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.notMod != 1 {
		t.Errorf("unchanged lists were sent again (%d 304s)", f.notMod)
	}
	// A new version and a licensed list.
	v4 := strings.Replace(animals, "version: 3", "version: 4", 1) + "bird = l'oiseau\n"
	pub := "title: Verbs\nlanguage: fr\nid: lst_pub\nversion: 1\nlicence: licensed\n\nto be = être\n"
	f.setLists(wireList{ID: "lst_animals", Version: 4, Title: "Animals", Language: "fr", Text: v4},
		wireList{ID: "lst_pub", Version: 1, Title: "Verbs", Language: "fr", Text: pub},
		wireList{ID: "lst_bad", Version: 1, Title: "Bad", Language: "fr", Text: "nonsense"})
	before := c.Changes()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Changes() == before {
		t.Error("no change counted")
	}
	lists := c.Lists()
	if len(lists) != 2 || lists[0].Version != 4 || len(lists[0].Entries) != 3 {
		t.Fatalf("lists: %+v", lists)
	}
	c.Answer("fr", words.Entry{Prompt: "bird", Answers: []string{"l'oiseau"}}, "practice", words.Answer{Tier: words.Perfect})
	c.mu.Lock()
	if len(c.q.Answers) != 1 || c.q.Answers[0].ListVersion != 4 {
		t.Errorf("answer to the new version: %+v", c.q.Answers)
	}
	c.mu.Unlock()
	// Both are unassigned: Animals becomes the player's own, the licensed
	// list goes.
	f.setLists()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(c.Lists()) != 0 || st.has("assigned/lst_animals.txt") || st.has("assigned/lst_pub.txt") {
		t.Error("unassigned lists are still assigned")
	}
	if !strings.Contains(string(st.files["words/animals.txt"]), "bird = l'oiseau") {
		t.Errorf("the unassigned list wasn't kept: %v", keys(st))
	}
	if st.has("words/verbs.txt") {
		t.Error("a licensed list was kept")
	}
}

func keys(st *memStore) []string {
	var out []string
	for k := range st.files {
		out = append(out, k)
	}
	return out
}

func TestRefresh(t *testing.T) {
	f, c, st, clk := linked(t)
	// The access token runs out: it is refreshed first.
	clk.add(24 * time.Hour)
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.AccessToken() != "hwd_2" || !strings.Contains(string(st.files[stateFile]), "hwr_2") {
		t.Fatalf("not refreshed: %s", c.AccessToken())
	}
	// The server says the token is no good (revoked, say): refresh and
	// try again.
	f.failNext("/api/v1/me", 401)
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.AccessToken() != "hwd_3" {
		t.Errorf("not refreshed after a 401: %s", c.AccessToken())
	}
}

func TestTokenReusedUnlinks(t *testing.T) {
	f, c, st, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	// Someone copied the tokens and refreshed: the server unlinks.
	f.mu.Lock()
	f.used[f.refresh] = true
	f.mu.Unlock()
	f.failNext("/api/v1/me", 401)
	err := c.Sync(context.Background())
	if !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	s := c.Status()
	if s.Linked || s.Note == "" || s.Err != nil || s.Pending != 0 {
		t.Errorf("status: %+v", s)
	}
	if !strings.Contains(string(st.files["words/animals.txt"]), "dog = le chien") {
		t.Error("the assigned list wasn't kept")
	}
	if c.AccessToken() != "" {
		t.Error("tokens kept")
	}
}

func TestRetries(t *testing.T) {
	f, c, _, _ := linked(t)
	var waits []time.Duration
	c.sleep = func(d time.Duration) { waits = append(waits, d) }
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	f.failNext("/api/v1/events", 503, 429)
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.eventsOf("answers")) != 1 || len(waits) != 2 || waits[0] != time.Second || waits[1] != 3*time.Second {
		t.Errorf("waits %v", waits)
	}
	// Three failures in a row: the sync gives up, the answer stays.
	c.Answer("fr", cat, "practice", words.Answer{Tier: words.Perfect})
	f.failNext("/api/v1/events", 500, 500, 500)
	if err := c.Sync(context.Background()); err == nil {
		t.Fatal("no error")
	}
	s := c.Status()
	if s.Pending != 1 || s.Err == nil || s.Offline {
		t.Errorf("after one failed sync: %+v", s)
	}
	if Explain(s.Err) != "the server has a problem; the game will try again later" {
		t.Errorf("explained as %q", Explain(s.Err))
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Status().Pending != 0 || c.Status().Err != nil {
		t.Error("the answer wasn't sent later")
	}
	// A refresh is never tried twice.
	c.mu.Lock()
	c.st.AccessExp = time.Time{}
	c.mu.Unlock()
	f.failNext("/api/v1/token", 503)
	before := f.count("/api/v1/token")
	c.Sync(context.Background())
	if n := f.count("/api/v1/token") - before; n != 1 {
		t.Errorf("token asked %d times", n)
	}
	if !c.Linked() {
		t.Error("a failed refresh unlinked the game")
	}
}

func TestServerDown(t *testing.T) {
	f, c, st, _ := linked(t)
	f.srv.Close()
	for range 3 {
		start := time.Now()
		c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
		c.Answer("fr", own, "practice", words.Answer{Tier: words.Perfect})
		if time.Since(start) > 50*time.Millisecond {
			t.Error("Answer waited")
		}
		if err := c.Sync(context.Background()); err == nil {
			t.Fatal("synced with the server down")
		}
	}
	s := c.Status()
	if !s.Offline || !s.Linked || s.Pending != 4 {
		t.Errorf("status: %+v", s)
	}
	if Explain(s.Err) != "can't reach the server; the game will try again later" {
		t.Errorf("explained as %q", Explain(s.Err))
	}
	if c.wait() <= SyncEvery {
		t.Error("no backoff")
	}
	if len(c.Lists()) != 1 || !st.has(queueFile) {
		t.Error("lists or queue lost")
	}
	// Linking with the server down fails quietly.
	_, c2, _, _ := setup(t)
	c2.server = f.srv.URL
	if err := c2.LinkNow(context.Background(), "ABCD-EFGH"); err == nil || c2.Linked() {
		t.Error("linked with the server down")
	}
}

func TestQueueCapFolding(t *testing.T) {
	_, c, _, clk := linked(t)
	c.o.QueueCap = 100
	for i := range 250 {
		if i == 120 {
			clk.add(24 * time.Hour)
		}
		tier := words.Perfect
		if i%5 == 0 {
			tier = words.Miss
		}
		c.Answer("fr", dog, "practice", words.Answer{Tier: tier, Timed: true, Secs: 2})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if n := c.q.len(); n > 100 {
		t.Fatalf("queue holds %d", n)
	}
	answers, right, secs := len(c.q.Answers), 0, 0.0
	for _, a := range c.q.Answers {
		right += b2i(a.Tier >= int(words.Correct))
		secs += *a.Secs
	}
	days := map[string]bool{}
	for _, tt := range c.q.Totals {
		answers += tt.Answers
		right += tt.Right
		secs += tt.Secs
		days[tt.Day] = true
	}
	if answers != 250 || right != 200 || secs != 500 {
		t.Errorf("after folding: %d answers, %d right, %.0f secs; want 250, 200, 500", answers, right, secs)
	}
	if len(days) != 2 || c.q.Folded == 0 {
		t.Errorf("folded into days %v (%d folded)", days, c.q.Folded)
	}
	// The newest answers are kept as they are.
	last := c.q.Answers[len(c.q.Answers)-1]
	if last.Seq != c.st.NextSeq-1 && c.q.Totals[len(c.q.Totals)-1].Seq != c.st.NextSeq-1 {
		t.Error("the newest event was folded")
	}
}

func TestQueueCapDropsSessions(t *testing.T) {
	_, c, _, clk := linked(t)
	c.o.QueueCap = 10
	for range 30 {
		c.Session(Session{Start: clk.now(), Mode: "adventure", Lang: "fr", Secs: 60})
	}
	if n := c.Status().Pending; n > 10 {
		t.Errorf("queue holds %d", n)
	}
}

func TestTotalsNotChangedWhileSending(t *testing.T) {
	_, c, _, _ := linked(t)
	c.Answer("fr", own, "practice", words.Answer{Tier: words.Perfect})
	c.mu.Lock()
	_, seqs := c.takeBatch(MaxBatch)
	c.mu.Unlock()
	c.Answer("fr", own, "practice", words.Answer{Tier: words.Perfect})
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.q.Totals) != 2 || c.q.Totals[0].Answers != 1 {
		t.Errorf("an answer was added to totals being sent: %+v (sending %v)", c.q.Totals, seqs)
	}
}

func TestBadBatchesDontStick(t *testing.T) {
	f, c, _, _ := linked(t)
	for range 3 {
		c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	}
	// The server refuses one event of a batch: it is dropped with the
	// rest.
	c.mu.Lock()
	f.refuse[c.q.Answers[0].Seq] = "unknown_word"
	c.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.eventsOf("answers")) != 2 || c.Status().Pending != 0 {
		t.Errorf("refused: %d stored, %d pending", len(f.eventsOf("answers")), c.Status().Pending)
	}
	// Too large: smaller batches.
	for range 10 {
		c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	}
	f.tooLarge = 3
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.eventsOf("answers")) != 12 || c.Status().Pending != 0 {
		t.Errorf("too large: %d stored, %d pending", len(f.eventsOf("answers")), c.Status().Pending)
	}
	// invalid_request naming an event: that one is dropped.
	f.tooLarge = 0
	e := &Error{Status: 400, Code: codeInvalidRequest}
	e.Details = append(e.Details, struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	}{Path: "/sessions/1/secs"})
	b := wireBatch{Answers: make([]wireAnswer, 2), Sessions: make([]wireSession, 2)}
	if got := badEvents(b, []int64{5, 6, 7, 8}, e); len(got) != 1 || !got[8] {
		t.Errorf("bad events %v", got)
	}
	e.Details = nil
	if got := badEvents(b, []int64{5, 6, 7, 8}, e); len(got) != 4 {
		t.Errorf("bad events %v", got)
	}
}

func TestMergeRules(t *testing.T) {
	f, c, _, _ := linked(t)
	local := map[string]*words.Memory{"fr": words.NewMemory()}
	m := local["fr"]
	// The player's own word, and dog answered here before (and sent).
	for range 3 {
		m.Record(own, words.Answer{Tier: words.Perfect})
	}
	m.Record(dog, words.Answer{Tier: words.Perfect})
	// The server knows more about dog (from another computer), and cat.
	srv := words.NewMemory()
	for range 4 {
		srv.Record(dog, words.Answer{Tier: words.Perfect})
	}
	srv.Record(cat, words.Answer{Tier: words.Miss})
	f.mu.Lock()
	f.memory["fr"] = srv
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	// An answer given after the sync, not yet sent.
	m.Record(dog, words.Answer{Tier: words.Miss})
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Miss})
	ownCard := *m.Cards[words.Key(own)]
	clock := m.Clock
	if !c.MergeMemory(local) {
		t.Fatal("nothing merged")
	}
	if c.MergeMemory(local) {
		t.Error("merged twice")
	}
	d := m.Cards[words.Key(dog)]
	// dog: the server's 4 perfect answers, then the queued miss.
	if d.Seen != 5 || d.Perfect != 4 || d.Misses != 1 || d.Box != 1 {
		t.Errorf("dog: %+v", d)
	}
	if ct := m.Cards[words.Key(cat)]; ct == nil || ct.Misses != 1 {
		t.Errorf("cat: %+v", ct)
	}
	if *m.Cards[words.Key(own)] != ownCard || m.Clock != clock {
		t.Errorf("own card or clock changed: %+v, %d → %d", m.Cards[words.Key(own)], clock, m.Clock)
	}
	// Due times follow the local clock: cat (box 1) is due 3 answers
	// after the merge's base, not at the server's clock.
	if due := m.Cards[words.Key(cat)].Due; due < m.Clock-2 || due > m.Clock+3 {
		t.Errorf("cat due at %d, clock %d", due, m.Clock)
	}
}

func TestMergeSkippedAfterUpload(t *testing.T) {
	f, c, _, _ := linked(t)
	srv := words.NewMemory()
	srv.Record(dog, words.Answer{Tier: words.Perfect})
	f.mu.Lock()
	f.memory["fr"] = srv
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Answers are sent before the game loop merges: the memory is out of
	// date and waits for the next sync.
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Miss})
	c.mu.Lock()
	gen := c.gen
	c.mu.Unlock()
	if err := c.upload(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	local := map[string]*words.Memory{}
	if c.MergeMemory(local) {
		t.Error("merged a memory older than an upload")
	}
}

func TestUnlink(t *testing.T) {
	f, c, st, _ := linked(t)
	pub := "title: Verbs\nlanguage: fr\nid: lst_pub\nversion: 1\nlicence: licensed\n\nto be = être\n"
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Text: animals},
		wireList{ID: "lst_pub", Version: 1, Title: "Verbs", Language: "fr", Text: pub})
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.Save()
	seq := c.st.NextSeq
	c.Unlink()
	s := c.Status()
	if s.Linked || s.Pending != 0 || c.AccessToken() != "" || len(c.Lists()) != 0 {
		t.Errorf("after unlinking: %+v", s)
	}
	if st.has(queueFile) || st.has("assigned/lst_animals.txt") || st.has("assigned/lst_pub.txt") {
		t.Errorf("files left: %v", keys(st))
	}
	if !st.has("words/animals.txt") || st.has("words/verbs.txt") {
		t.Errorf("kept lists: %v", keys(st))
	}
	if strings.Contains(string(st.files[stateFile]), "hwr_") {
		t.Error("tokens kept on disk")
	}
	// Nothing is queued once unlinked.
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.Session(Session{Start: time.Now(), Mode: "practice", Lang: "fr", Secs: 60})
	if c.Status().Pending != 0 {
		t.Error("queued while unlinked")
	}
	if err := c.Sync(context.Background()); !errors.Is(err, ErrNotLinked) {
		t.Errorf("sync: %v", err)
	}
	// Linking again goes on from the same sequence number.
	f.mu.Lock()
	f.code = "QRST-VWXY"
	f.mu.Unlock()
	if err := c.LinkNow(context.Background(), "QRSTVWXY"); err != nil {
		t.Fatal(err)
	}
	if c.st.NextSeq < seq {
		t.Error("sequence numbers went back")
	}
	// Unlinking keeps an identical copy only once.
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.Unlink()
	if st.has("words/animals-2.txt") {
		t.Error("the same list kept twice")
	}
}

// TestUnlinkTellsTheServer: unlinking asks the server to unlink the
// device too, refreshing an access token that ran out first.
func TestUnlinkTellsTheServer(t *testing.T) {
	f, c, _, clk := linked(t)
	c.Unlink()
	c.bg.Wait()
	if f.count("/api/v1/unlink") != 1 || f.unlinked != 1 {
		t.Errorf("unlink calls %d, unlinked %d", f.count("/api/v1/unlink"), f.unlinked)
	}
	// Not linked: nothing to tell.
	c.Unlink()
	c.bg.Wait()
	if f.count("/api/v1/unlink") != 1 {
		t.Error("told the server twice")
	}

	f.mu.Lock()
	f.code = "QRST-VWXY"
	f.mu.Unlock()
	if err := c.LinkNow(context.Background(), "QRSTVWXY"); err != nil {
		t.Fatal(err)
	}
	clk.add(2 * 24 * time.Hour)
	before := f.count("/api/v1/token")
	c.Unlink()
	c.bg.Wait()
	if f.count("/api/v1/token") != before+1 || f.unlinked != 2 {
		t.Errorf("with an old access token: token calls %d, unlinked %d", f.count("/api/v1/token")-before, f.unlinked)
	}
	if c.Linked() {
		t.Error("the refresh relinked the game")
	}
}

// TestUnlinkOffline: the game unlinks itself even when the server can't
// be told.
func TestUnlinkOffline(t *testing.T) {
	f, c, _, _ := linked(t)
	f.srv.Close()
	c.Unlink()
	if c.Linked() || c.AccessToken() != "" {
		t.Error("still linked")
	}
	c.bg.Wait()
}

// TestWebBuildSendsClientHeader: in a browser the game sends its version
// in X-Halpwords-Client, not User-Agent.
func TestWebBuildSendsClientHeader(t *testing.T) {
	inBrowser = true
	t.Cleanup(func() { inBrowser = false })
	f, _, _, _ := linked(t)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.clients) == 0 {
		t.Fatal("no requests")
	}
	for i, h := range f.clients {
		if !strings.HasPrefix(h, "halpwords/") || !uaRE.MatchString("Halpwords/"+strings.TrimPrefix(h, "halpwords/")) {
			t.Errorf("X-Halpwords-Client %q", h)
		}
		if strings.HasPrefix(f.agents[i], "Halpwords/") {
			t.Errorf("User-Agent %q set in a browser", f.agents[i])
		}
	}
}

func TestUnlinkDuringSync(t *testing.T) {
	f, c, _, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	f.mu.Lock() // hold the server until the game has unlinked
	done := make(chan error)
	go func() { done <- c.Sync(context.Background()) }()
	time.Sleep(20 * time.Millisecond)
	c.Unlink()
	f.mu.Unlock()
	<-done
	if c.Linked() || len(c.Lists()) != 0 || c.Status().Pending != 0 {
		t.Errorf("a sync undid the unlink: %+v", c.Status())
	}
}

func TestLocked(t *testing.T) {
	f, c, _, _ := linked(t)
	fr, _ := words.Lookup("fr")
	own := profile.LangSettings{Rules: fr.Defaults, Highlight: true}
	if _, locked := c.Locked(fr, own); locked {
		t.Error("locked with nothing set")
	}
	f.mu.Lock()
	f.me["accommodations"] = map[string]any{"relaxed_timers": true, "ignore_accents": true}
	f.mu.Unlock()
	before := c.Changes()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Changes() == before {
		t.Error("new settings not counted as a change")
	}
	ls, locked := c.Locked(fr, own)
	if !locked || ls.Timer != profile.Relaxed || ls.Rules.Accents != words.Ignore || !ls.Highlight {
		t.Errorf("accommodations: %+v %v", ls, locked)
	}
	f.mu.Lock()
	f.me["accommodations"] = map[string]any{}
	f.me["settings"] = map[string]any{"Langs": map[string]any{"fr": map[string]any{"Rules": map[string]any{"Accents": 2, "ArticlesRequired": true}, "Timer": 2}}}
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	ls, locked = c.Locked(fr, own)
	if !locked || ls.Timer != profile.Fast || !ls.Rules.ArticlesRequired || ls.Highlight {
		t.Errorf("settings: %+v %v", ls, locked)
	}
	la, _ := words.Lookup("la")
	if _, locked := c.Locked(la, own); locked {
		t.Error("Latin locked")
	}
	c.Unlink()
	if _, locked := c.Locked(fr, own); locked {
		t.Error("locked after unlinking")
	}
}

func TestStartAndClose(t *testing.T) {
	f, c, st, _ := linked(t)
	old := SyncEvery
	SyncEvery = time.Hour
	defer func() { SyncEvery = old }()
	c.Start()
	c.Start() // twice is fine
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.SyncNow()
	deadline := time.Now().Add(5 * time.Second)
	for len(f.eventsOf("answers")) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("SyncNow didn't sync")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Quitting sends what is left.
	for c.Status().Busy {
		time.Sleep(time.Millisecond)
	}
	c.Answer("fr", cat, "practice", words.Answer{Tier: words.Perfect})
	c.Close()
	if len(f.eventsOf("answers")) != 2 || st.has(queueFile) {
		t.Errorf("close: %d answers sent", len(f.eventsOf("answers")))
	}
}

func TestNilClient(t *testing.T) {
	var c *Client
	c.Answer("fr", dog, "practice", words.Answer{})
	c.Session(Session{Secs: 100})
	c.SyncNow()
	c.Close()
	c.Unlink()
	if c.Linked() || c.AccessToken() != "" || c.Lists() != nil || c.MergeMemory(nil) || c.Changes() != 0 || c.Me() != nil {
		t.Error("a nil client did something")
	}
	if _, locked := c.Locked(words.Languages[0], profile.LangSettings{}); locked {
		t.Error("nil client locked settings")
	}
}

func TestDamagedFiles(t *testing.T) {
	st := newMemStore()
	st.Write(stateFile, []byte("{nope"))
	st.Write(queueFile, []byte("[1,2"))
	c := Open(Options{Store: st, Server: "http://127.0.0.1:1"})
	if c.Linked() || c.Status().Pending != 0 {
		t.Error("damaged files read as something")
	}
}

func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{
		"adventure:attack": "adventure:attack", "Practice": "practice", "": "practice",
		"hard core:Dodge!": "hardcore:dodge", "a_b:": "a_b",
	} {
		if got := cleanMode(in, true); got != want || !modeRE.MatchString(got) {
			t.Errorf("cleanMode(%q) = %q, want %q", in, got, want)
		}
	}
	if cleanMode("adventure:attack", false) != "adventure" {
		t.Error("session mode keeps the kind")
	}
	for _, id := range []string{"lst_1", "../../etc", strings.Repeat("x", 100), ""} {
		f := fileFor(id)
		if strings.ContainsAny(f, "/\\") || !strings.HasSuffix(f, ".txt") {
			t.Errorf("fileFor(%q) = %q", id, f)
		}
	}
	for roles, want := range map[string]string{
		"":                                    "Only the grown-up who linked this game can see your progress.",
		"teacher":                             "Your teacher can see your progress.",
		"guardian,co-guardian":                "Your parents can see your progress.",
		"teacher,teaching-assistant":          "Your teacher and a teaching assistant can see your progress.",
		"guardian,teacher,teaching-assistant": "Your parent, your teacher and a teaching assistant can see your progress.",
	} {
		var r []string
		if roles != "" {
			r = strings.Split(roles, ",")
		}
		if got := SeenByText(r); got != want {
			t.Errorf("SeenByText(%v) = %q", r, got)
		}
	}
	for roles, want := range map[string]string{
		"":                             "Set on the website by a grown-up.",
		"teaching-assistant":           "Set on the website by a grown-up.",
		"guardian":                     "Set on the website by your parent.",
		"guardian,teacher":             "Set on the website by your parent or your teacher.",
		"guardian,co-guardian,teacher": "Set on the website by your parents or your teacher.",
	} {
		var r []string
		if roles != "" {
			r = strings.Split(roles, ",")
		}
		if got := SetByText(r); got != want {
			t.Errorf("SetByText(%v) = %q", r, got)
		}
	}
	e := words.Entry{Prompt: "to be", Answers: []string{"être"}}
	if words.Key(entryFor(words.Key(e))) != words.Key(e) {
		t.Error("entryFor doesn't round-trip")
	}
	c := &Client{o: Options{Version: "dev"}}
	if !uaRE.MatchString(c.userAgent()) || !strings.HasPrefix(c.userAgent(), "Halpwords/dev (") {
		t.Errorf("UA %q", c.userAgent())
	}
	t.Setenv("HALPWORDS_SERVER", "https://staging.halpwords.com/")
	if ServerURL() != "https://staging.halpwords.com" {
		t.Error(ServerURL())
	}
	t.Setenv("HALPWORDS_SERVER", "")
	if ServerURL() != DefaultServer {
		t.Error(ServerURL())
	}
}

func TestTrySave(t *testing.T) {
	_, c, st, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.mu.Lock()
	err := c.TrySave()
	c.mu.Unlock()
	if !errors.Is(err, errBusy) || st.has(queueFile) {
		t.Fatalf("saved while busy: %v", err)
	}
	if err := c.TrySave(); err != nil || !st.has(queueFile) {
		t.Fatalf("not saved: %v", err)
	}
	var nilClient *Client
	if nilClient.TrySave() != nil {
		t.Error("nil client")
	}
}
