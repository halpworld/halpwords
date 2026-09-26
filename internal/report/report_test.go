package report

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// The fixtures in testdata are the exact JSON this package sends. The
// server keeps copies (halpwords-server internal/web/testdata/
// game_reports) and posts them in its tests, so the two can't disagree
// without a test failing on one side.
func TestFixtures(t *testing.T) {
	g := Game{Version: "1.2.0", Platform: "linux/amd64"}
	crash := "Halpwords 1.2.0 crashed.\npanic: runtime error: index out of range [3] with length 3\n\ngoroutine 1 [running]:\nmain.main()\n"
	bug := Bug(g, "81234", "The dungeon froze after the third door.", crash, true)
	bug.ID = "0b6f6f0e-3c1a-4c1e-9d55-2a7c3e1f9a10"
	upset := Upset(Game{Version: "1.2.0", Platform: "windows/amd64"}, "player", "  A player in the race had a rude name.\x07 ")
	upset.ID = "5d2c9a47-8e61-4b0f-a3d2-71c4f09e6b38"
	for name, r := range map[string]Report{"bug": bug, "upset": upset} {
		if err := r.Check(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		got, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile("testdata/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(append(got, '\n'), want) {
			t.Errorf("%s:\n%s\nwant\n%s", name, got, want)
		}
	}
}

func TestCrashOnlyWhenTicked(t *testing.T) {
	g := Game{Version: "dev", Platform: "js/wasm"}
	r := Bug(g, "1", "", "panic: oh no", false)
	b, _ := json.Marshal(r)
	if r.Crash != "" || strings.Contains(string(b), "crash") || strings.Contains(string(b), "oh no") {
		t.Errorf("unticked crash sent: %s", b)
	}
	if r := Bug(g, "1", "", "panic: oh no", true); r.Crash != "panic: oh no" {
		t.Errorf("ticked crash: %q", r.Crash)
	}
	big := strings.Repeat("é", MaxCrash)
	if r := Bug(g, "1", "", big, true); len(r.Crash) > MaxCrash || r.Check() != nil {
		t.Errorf("big crash: %d bytes, %v", len(r.Crash), r.Check())
	}
}

func TestCheck(t *testing.T) {
	g := Game{Version: "1", Platform: "x"}
	ok := Upset(g, "name", strings.Repeat("ó", 600))
	if ok.Check() != nil || len([]rune(ok.Text)) != MaxText {
		t.Errorf("long text isn't cut to %d letters", MaxText)
	}
	for name, r := range map[string]Report{
		"no about":    Upset(g, "", ""),
		"bad about":   Upset(g, "weather", ""),
		"bad kind":    {ID: "a", Kind: "abuse", Game: g},
		"no id":       {Kind: KindBug, Game: g},
		"bug about":   {ID: "a", Kind: KindBug, About: "name", Game: g},
		"upset crash": {ID: "a", Kind: KindUpset, About: "name", Crash: "x", Game: g},
	} {
		if r.Check() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if a, b := NewID(), NewID(); a == b || len(a) != 36 || a[14] != '4' {
		t.Errorf("IDs %s %s", a, b)
	}
}

func TestServerURL(t *testing.T) {
	for env, want := range map[string]string{
		"":                               DefaultServer,
		"https://staging.halpwords.com/": "https://staging.halpwords.com",
		"http://localhost:8080":          "http://localhost:8080",
		"http://evil.example":            DefaultServer,
		"https://u:p@x.example":          DefaultServer,
		"not a url":                      DefaultServer,
	} {
		t.Setenv("HALPWORDS_SERVER", env)
		if got := ServerURL(); got != want {
			t.Errorf("HALPWORDS_SERVER=%q: %q, want %q", env, got, want)
		}
	}
}

// memStore is a Store in memory.
type memStore struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (m *memStore) Read(name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return b, nil
}

func (m *memStore) Write(name string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[name] = append([]byte(nil), data...)
	return nil
}

// fakeServer answers POST /api/v1/reports with status and records what
// it got.
type fakeServer struct {
	mu     sync.Mutex
	status int
	got    []Report
	auth   []string
	ua     []string
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method != http.MethodPost || r.URL.Path != "/api/v1/reports" || r.Header.Get("Content-Type") != "application/json" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var rep Report
	b, _ := io.ReadAll(r.Body)
	if json.Unmarshal(b, &rep) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.got = append(f.got, rep)
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	f.ua = append(f.ua, r.Header.Get("User-Agent"))
	w.WriteHeader(f.status)
	w.Write([]byte(`{}`))
}

func (f *fakeServer) set(status int) {
	f.mu.Lock()
	f.status = status
	f.mu.Unlock()
}

func newOutbox(url string, token func() string) (*Outbox, *memStore) {
	st := &memStore{files: map[string][]byte{}}
	return &Outbox{Queue: NewQueue(st), Sender: &Sender{Server: url, Token: token, UserAgent: "Halpwords/1.2.0 (linux; amd64)"}}, st
}

func TestQueuedOfflineThenSent(t *testing.T) {
	fk := &fakeServer{status: http.StatusCreated}
	srv := httptest.NewServer(fk)
	defer srv.Close()
	ctx := context.Background()

	// Offline: nothing answers at this address.
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	o, st := newOutbox(deadURL, nil)
	g := Game{Version: "1.2.0", Platform: "linux/amd64"}
	for _, r := range []Report{Bug(g, "7", "froze", "", false), Upset(g, "list", "")} {
		if err := o.Queue.Add(r); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := o.Flush(ctx); n != 0 || err == nil || o.Waiting() != 2 {
		t.Fatalf("offline: sent %d, %v, %d waiting", n, err, o.Waiting())
	}
	// The queue is in the user's folder, so it survives a restart.
	o2 := &Outbox{Queue: NewQueue(st), Sender: &Sender{Server: srv.URL}}
	if n, err := o2.Flush(ctx); n != 2 || err != nil || o2.Waiting() != 0 {
		t.Fatalf("online: sent %d, %v, %d waiting", n, err, o2.Waiting())
	}
	if len(fk.got) != 2 || fk.got[0].Kind != KindBug || fk.got[1].About != "list" {
		t.Errorf("server got %+v", fk.got)
	}
	// Unlinked: no token, so the report is anonymous.
	for _, a := range fk.auth {
		if a != "" {
			t.Errorf("an unlinked game sent %q", a)
		}
	}
}

func TestLinkedGameSendsItsToken(t *testing.T) {
	fk := &fakeServer{status: http.StatusCreated}
	srv := httptest.NewServer(fk)
	defer srv.Close()
	token := ""
	o, _ := newOutbox(srv.URL, func() string { return token })
	g := Game{Version: "1.2.0", Platform: "linux/amd64"}
	o.Queue.Add(Upset(g, "name", ""))
	o.Flush(context.Background())
	token = "hwd_abc"
	o.Queue.Add(Upset(g, "name", ""))
	o.Flush(context.Background())
	if len(fk.auth) != 2 || fk.auth[0] != "" || fk.auth[1] != "Bearer hwd_abc" || fk.ua[1] != "Halpwords/1.2.0 (linux; amd64)" {
		t.Errorf("auth %q ua %q", fk.auth, fk.ua)
	}
}

func TestServerAnswers(t *testing.T) {
	fk := &fakeServer{}
	srv := httptest.NewServer(fk)
	defer srv.Close()
	o, _ := newOutbox(srv.URL, nil)
	g := Game{Version: "1", Platform: "x"}
	for _, c := range []struct {
		status  int
		waiting int
	}{
		{http.StatusTooManyRequests, 1},
		{http.StatusInternalServerError, 1},
		{http.StatusUnauthorized, 1},
		{http.StatusBadRequest, 0}, // never accepted: dropped
		{http.StatusOK, 0},         // a resend the server already had
	} {
		fk.set(c.status)
		o.Queue.Add(Upset(g, "other", ""))
		o.Flush(context.Background())
		if w := o.Waiting(); w != c.waiting {
			t.Errorf("after %d: %d waiting, want %d", c.status, w, c.waiting)
		}
		o.Queue = NewQueue(&memStore{files: map[string][]byte{}})
	}
}

func TestQueueLimits(t *testing.T) {
	st := &memStore{files: map[string][]byte{}}
	q := NewQueue(st)
	g := Game{Version: "1", Platform: "x"}
	var last string
	for i := 0; i < MaxQueued+5; i++ {
		r := Upset(g, "other", "")
		last = r.ID
		if err := q.Add(r); err != nil {
			t.Fatal(err)
		}
	}
	rs, _ := q.Pending()
	if len(rs) != MaxQueued || rs[len(rs)-1].ID != last {
		t.Errorf("%d queued", len(rs))
	}
	if q.Add(Report{ID: "x", Kind: "nope"}) == nil {
		t.Error("an invalid report was queued")
	}
	st.files[QueueFile] = []byte("{damaged")
	if rs, err := q.Pending(); err != nil || len(rs) != 0 {
		t.Errorf("damaged queue: %d, %v", len(rs), err)
	}
}
