package link

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// session is a recorded exchange file (testdata/session-link-qr.json):
// the server's internal/api/schema_test.go validates the same file
// against its schemas.
type session struct {
	Exchanges []struct {
		Name     string          `json:"name"`
		Method   string          `json:"method"`
		Path     string          `json:"path"`
		Request  map[string]any  `json:"request"`
		Status   int             `json:"status"`
		Response json.RawMessage `json:"response"`
	} `json:"exchanges"`
}

// TestPairRecordedSession replays the recorded session against the
// client: the requests the game makes have the recorded shape, and it
// takes each answer as it should.
func TestPairRecordedSession(t *testing.T) {
	data, err := os.ReadFile("testdata/session-link-qr.json")
	if err != nil {
		t.Fatal(err)
	}
	var s session
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if names := []string{"start", "pending", "slow_down", "tokens"}; len(s.Exchanges) != len(names) {
		t.Fatalf("%d exchanges", len(s.Exchanges))
	} else {
		for i, n := range names {
			if s.Exchanges[i].Name != n {
				t.Fatalf("exchange %d is %q, want %q", i, s.Exchanges[i].Name, n)
			}
		}
	}
	var mu sync.Mutex
	next, challenge := 0, ""
	var other []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if next >= len(s.Exchanges) {
			other = append(other, r.Method+" "+r.URL.Path)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		x := s.Exchanges[next]
		next++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if r.Method != x.Method || r.URL.Path != x.Path {
			t.Errorf("%s: %s %s, want %s %s", x.Name, r.Method, r.URL.Path, x.Method, x.Path)
		}
		// Same fields as recorded; the secrets are the game's own.
		if fieldNames(body) != fieldNames(x.Request) {
			t.Errorf("%s: sent %v, recorded %v", x.Name, fieldNames(body), fieldNames(x.Request))
		}
		switch x.Name {
		case "start":
			challenge, _ = body["code_challenge"].(string)
			if len(challenge) != 43 {
				t.Errorf("challenge %q", challenge)
			}
		default:
			v, _ := body["code_verifier"].(string)
			if body["device_code"] != x.Request["device_code"] || s256(v) != challenge || body["name"] != x.Request["name"] {
				t.Errorf("%s: sent %v", x.Name, body)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(x.Status)
		w.Write(x.Response)
	}))
	defer srv.Close()
	clk := &clock{t: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)}
	st := newMemStore()
	c := Open(Options{Store: st, Server: srv.URL, Now: clk.now, Name: "Halpwords on Windows"})
	c.sleep = func(time.Duration) {}
	ctx := context.Background()

	code, err := c.StartPair(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if code.UserCode != "MKT4-HX7R" || code.Every != 5*time.Second || !code.Expires.Equal(clk.now().Add(15*time.Minute)) ||
		code.URL != "https://halpwords.com/link" || code.Verifier != "" {
		t.Fatalf("code %+v", code)
	}
	if code.ScanURL != "" {
		t.Errorf("the QR code holds %q, an address on another host than the server's", code.ScanURL)
	}
	if err := c.PollPair(ctx); !errors.Is(err, ErrPairPending) {
		t.Fatalf("pending: %v", err)
	}
	if err := c.PollPair(ctx); !errors.Is(err, ErrPairPending) {
		t.Fatalf("slow_down: %v", err)
	}
	if p := c.PendingPair(); p == nil || p.Every != 10*time.Second {
		t.Fatalf("after slow_down: %+v", p)
	}
	if err := c.PollPair(ctx); err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if !c.Linked() || c.Way() != WayPairing || c.DeviceID() != "dev_01JQRCODE0000000000000000" || c.PendingPair() != nil {
		t.Fatalf("after the tokens: linked %v way %q", c.Linked(), c.Way())
	}
	if !c.Status().Unconfirmed {
		t.Fatal("linked by QR code without asking the child")
	}
	// Nothing syncs before the child says it is them.
	if err := c.Sync(ctx); !errors.Is(err, ErrUnconfirmed) {
		t.Fatalf("sync before confirming: %v", err)
	}
	// Kept across a restart.
	c2 := Open(Options{Store: st, Server: srv.URL, Now: clk.now})
	if !c2.Status().Unconfirmed || !c2.Linked() {
		t.Fatal("the confirmation was lost on a restart")
	}
	if err := c2.Sync(ctx); !errors.Is(err, ErrUnconfirmed) {
		t.Fatalf("sync after a restart: %v", err)
	}
	mu.Lock()
	if len(other) > 0 {
		t.Errorf("requests before confirming: %v", other)
	}
	mu.Unlock()
	c2.ConfirmPair()
	if c2.Status().Unconfirmed {
		t.Fatal("still unconfirmed")
	}
	c3 := Open(Options{Store: st, Server: srv.URL, Now: clk.now})
	if c3.Status().Unconfirmed {
		t.Fatal("the confirmation wasn't kept")
	}
}

func fieldNames(m map[string]any) string {
	return strings.Join(slices.Sorted(maps.Keys(m)), ",")
}

// pairServer is a pretend server for linking by QR code.
type pairServer struct {
	mu sync.Mutex
	// start answers start with this status and code, when set; tokenStatus
	// and tokenCode the polls.
	startStatus int
	startCode   string
	startRaw    string // a body that isn't JSON
	tokenStatus int
	tokenCode   string
	approved    bool
	starts      int
	polls       int
	challenge   string
	paths       []string
	verifyURL   string
}

func (f *pairServer) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths = append(f.paths, r.Method+" "+r.URL.Path)
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	switch r.URL.Path {
	case "/api/v1/link/start":
		f.starts++
		switch {
		case f.startRaw != "":
			w.WriteHeader(f.startStatus)
			w.Write([]byte(f.startRaw))
			return
		case f.startStatus != 0:
			writeErr(w, f.startStatus, f.startCode)
			return
		}
		f.challenge, _ = body["code_challenge"].(string)
		writeJSON(w, http.StatusOK, map[string]any{"device_code": "hwlr_abc", "user_code": "MKT4-HX7R",
			"verification_uri": "https://halpwords.com/link", "verification_uri_complete": f.verifyURL,
			"expires_in": 900, "interval": 5})
	case "/api/v1/link/token":
		f.polls++
		v, _ := body["code_verifier"].(string)
		switch {
		case f.tokenStatus != 0:
			writeErr(w, f.tokenStatus, f.tokenCode)
		case body["device_code"] != "hwlr_abc" || s256(v) != f.challenge:
			writeErr(w, http.StatusBadRequest, codeExpiredToken)
		case !f.approved:
			writeErr(w, http.StatusBadRequest, codeAuthorizationPending)
		default:
			writeJSON(w, http.StatusOK, map[string]any{"device_id": "dev_1", "token_type": "Bearer", "access_token": "hwd_1",
				"expires_in": 3600, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
		}
	case "/api/v1/me":
		writeJSON(w, http.StatusOK, map[string]any{"learner": map[string]any{"id": "lrn_1", "display_name": "Aoife"}})
	case "/api/v1/unlink":
		w.WriteHeader(http.StatusNoContent)
	case "/api/v1/choices":
		writeJSON(w, http.StatusOK, map[string]any{"child_notice": map[string]any{"ref": "child-v1", "title": "T", "body": []string{"B"}}})
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func pairSetup(t *testing.T) (*pairServer, *Client, *memStore, *clock, string) {
	t.Helper()
	f := &pairServer{}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.verifyURL = srv.URL + "/link?c=MKT4HX7R"
	st := newMemStore()
	clk := &clock{t: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)}
	c := Open(Options{Store: st, Server: srv.URL, Now: clk.now})
	c.sleep = func(time.Duration) {}
	return f, c, st, clk, srv.URL
}

func TestPairFlow(t *testing.T) {
	f, c, st, _, url := pairSetup(t)
	ctx := context.Background()
	code, err := c.StartPair(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if code.ScanURL != url+"/link?c=MKT4HX7R" {
		t.Errorf("ScanURL %q", code.ScanURL)
	}
	// A web game that reloads goes on with the code in link.json, and
	// the verifier never leaves the client.
	c2 := Open(Options{Store: st, Server: url, Now: c.now})
	c2.sleep = func(time.Duration) {}
	if p := c2.PendingPair(); p == nil || p.DeviceCode != "hwlr_abc" || p.Verifier != "" {
		t.Fatalf("pending after a reload: %+v", p)
	}
	if err := c2.PollPair(ctx); !errors.Is(err, ErrPairPending) {
		t.Fatal(err)
	}
	f.approved = true
	if err := c2.PollPair(ctx); err != nil {
		t.Fatal(err)
	}
	if name, err := c2.PairedLearner(ctx); err != nil || name != "Aoife" {
		t.Fatalf("learner %q, %v", name, err)
	}
	if _, err := c2.StartPair(ctx); !errors.Is(err, ErrLinked) {
		t.Errorf("start when linked: %v", err)
	}
	// "That's not me": unlinked, nothing sent but the unlink.
	c2.Unlink()
	c2.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.paths {
		switch p {
		case "POST /api/v1/link/start", "POST /api/v1/link/token", "GET /api/v1/me", "POST /api/v1/unlink":
		default:
			t.Errorf("sent %s before the child confirmed", p)
		}
	}
	if c2.Linked() {
		t.Error("still linked after no")
	}
}

// TestPairOff: an older server has no /link/start. Any 404 or 405 falls
// back to the typed code, and the game doesn't ask again until it
// restarts.
func TestPairOff(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
		raw    string
	}{
		{"404 not JSON", http.StatusNotFound, "", "404 page not found\n"},
		{"404 not_found", http.StatusNotFound, "not_found", ""},
		{"404 link_off", http.StatusNotFound, codeLinkOff, ""},
		{"405", http.StatusMethodNotAllowed, "", "Method Not Allowed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, _, _, url := pairSetup(t)
			f.startStatus, f.startCode, f.startRaw = tc.status, tc.code, tc.raw
			ctx := context.Background()
			if _, err := c.StartPair(ctx); !errors.Is(err, ErrPairOff) {
				t.Fatalf("start: %v", err)
			}
			if _, err := c.StartPair(ctx); !errors.Is(err, ErrPairOff) || f.starts != 1 {
				t.Fatalf("again: %v, %d starts", err, f.starts)
			}
			// A restart asks again.
			c2 := Open(Options{Server: url})
			f.startStatus, f.startCode, f.startRaw = 0, "", ""
			if _, err := c2.StartPair(ctx); err != nil || f.starts != 2 {
				t.Fatalf("after a restart: %v", err)
			}
		})
	}
}

// TestPairServerError: no answer, or a server error, keeps the code
// until it runs out; a start that fails says so (not ErrPairOff).
func TestPairServerError(t *testing.T) {
	f, c, _, clk, _ := pairSetup(t)
	ctx := context.Background()
	f.startStatus, f.startCode = http.StatusServiceUnavailable, "internal"
	if _, err := c.StartPair(ctx); err == nil || errors.Is(err, ErrPairOff) {
		t.Fatalf("start on a 503: %v", err)
	}
	f.startStatus, f.startCode = 0, ""
	if _, err := c.StartPair(ctx); err != nil {
		t.Fatal(err)
	}
	f.tokenStatus, f.tokenCode = http.StatusInternalServerError, "internal"
	if err := c.PollPair(ctx); err == nil || c.PendingPair() == nil {
		t.Fatalf("a 500 dropped the code: %v", err)
	}
	f.tokenStatus, f.tokenCode = http.StatusTooManyRequests, codeRateLimited
	var e *Error
	if err := c.PollPair(ctx); !errors.Is(err, ErrSSOBusy) || !errors.As(err, &e) || c.PendingPair() == nil {
		t.Fatalf("a 429: %v", err)
	}
	clk.add(16 * time.Minute)
	polls := f.polls
	if err := c.PollPair(ctx); !errors.Is(err, ErrPairExpired) || f.polls != polls || c.PendingPair() != nil {
		t.Fatalf("expired: %v, %d polls", err, f.polls-polls)
	}
}

// TestPairOver: answers that end the link forget the code.
func TestPairOver(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   error
	}{
		{http.StatusBadRequest, codeExpiredToken, ErrPairExpired},
		{http.StatusForbidden, codeAccessDenied, ErrPairRefused},
		{http.StatusForbidden, codeNotLinkable, ErrNotLinkable},
		{http.StatusForbidden, codeAccountSuspended, ErrSuspended},
	} {
		t.Run(tc.code, func(t *testing.T) {
			f, c, st, _, _ := pairSetup(t)
			ctx := context.Background()
			if _, err := c.StartPair(ctx); err != nil {
				t.Fatal(err)
			}
			f.tokenStatus, f.tokenCode = tc.status, tc.code
			if err := c.PollPair(ctx); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if c.PendingPair() != nil || c.Linked() || st.has(stateFile) {
				t.Fatal("the code is still kept")
			}
			if Explain(tc.want) == Explain(errors.New("x")) {
				t.Errorf("Explain(%v) says nothing", tc.want)
			}
		})
	}
}

func TestPairCancel(t *testing.T) {
	f, c, st, _, _ := pairSetup(t)
	ctx := context.Background()
	if _, err := c.StartPair(ctx); err != nil {
		t.Fatal(err)
	}
	c.CancelPair()
	if c.PendingPair() != nil || st.has(stateFile) {
		t.Fatal("cancel kept the code")
	}
	if err := c.PollPair(ctx); !errors.Is(err, ErrPairExpired) || f.polls != 0 {
		t.Fatalf("poll after cancel: %v, %d polls", err, f.polls)
	}
}

func TestSameHost(t *testing.T) {
	for _, tc := range []struct {
		addr, server string
		want         bool
	}{
		{"https://halpwords.com/link?c=ABCDEFGH", "https://halpwords.com", true},
		{"https://HALPWORDS.com/link?c=ABCDEFGH", "https://halpwords.com/", true},
		{"https://test.halpwords.com/link?c=X", "https://test.halpwords.com", true},
		{"http://localhost:8080/link?c=X", "http://localhost:8080", true},
		{"https://evil.example/link?c=X", "https://halpwords.com", false},
		{"https://halpwords.com.evil.example/link", "https://halpwords.com", false},
		{"https://user@halpwords.com/link", "https://halpwords.com", false},
		{"javascript:alert(1)", "https://halpwords.com", false},
		{"", "https://halpwords.com", false},
		{"https://halpwords.com/link", "", false},
	} {
		if got := sameHost(tc.addr, tc.server); got != tc.want {
			t.Errorf("sameHost(%q, %q) = %v", tc.addr, tc.server, got)
		}
	}
}

// TestPairCodeJSON: the verifier and the device code are kept in
// link.json, and ScanURL is not.
func TestPairCodeJSON(t *testing.T) {
	p := PairCode{DeviceCode: "hwlr_x", Verifier: "v", ScanURL: "https://halpwords.com/link"}
	data, _ := json.Marshal(p)
	var back PairCode
	json.Unmarshal(data, &back)
	want := p
	want.ScanURL = ""
	if !reflect.DeepEqual(back, want) {
		t.Errorf("round trip %+v", back)
	}
}

// TestPairUnconfirmed: until the child says the learner is them, the
// game may only read who it is and My choices (for the child-link
// notice). Nothing is sent, no token is handed out, and Close uploads
// nothing; ConfirmPair then lets the sync go.
func TestPairUnconfirmed(t *testing.T) {
	f, c, _, _, _ := pairSetup(t)
	ctx := context.Background()
	if _, err := c.StartPair(ctx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.approved = true
	f.mu.Unlock()
	if err := c.PollPair(ctx); err != nil {
		t.Fatal(err)
	}
	if !c.Status().Unconfirmed || c.AccessToken() != "" {
		t.Fatal("tokens handed out before the child confirmed")
	}
	if _, err := c.Choices(ctx); err != nil {
		t.Fatalf("choices: %v", err)
	}
	if err := c.SawNotice(ctx, "child-v1"); !errors.Is(err, ErrUnconfirmed) {
		t.Errorf("notice sent before the child confirmed: %v", err)
	}
	if err := c.Sync(ctx); !errors.Is(err, ErrUnconfirmed) {
		t.Errorf("sync: %v", err)
	}
	f.mu.Lock()
	for _, p := range f.paths {
		switch p {
		case "POST /api/v1/link/start", "POST /api/v1/link/token", "GET /api/v1/choices":
		default:
			t.Errorf("sent %s before the child confirmed", p)
		}
	}
	f.mu.Unlock()
	if !c.ConfirmPair() || c.ConfirmPair() {
		t.Error("ConfirmPair reports whether the game waited for it")
	}
	if c.Status().Unconfirmed || c.AccessToken() != "hwd_1" {
		t.Error("still held after ConfirmPair")
	}
	c.Close()
}
