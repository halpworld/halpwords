package game

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/report"
)

// mapStore keeps a link's files in memory.
type mapStore map[string][]byte

func (m mapStore) Read(name string) ([]byte, error) {
	if d, ok := m[name]; ok {
		return d, nil
	}
	return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
}
func (m mapStore) Write(name string, data []byte) error        { m[name] = data; return nil }
func (m mapStore) WritePrivate(name string, data []byte) error { m[name] = data; return nil }
func (m mapStore) Remove(name string) error                    { delete(m, name); return nil }

// linkedTo opens a link that is linked to server as device dev, with
// access token tok.
func linkedTo(t *testing.T, server, dev, tok string) *link.Client {
	t.Helper()
	st, _ := json.Marshal(map[string]any{"Server": server, "DeviceID": dev, "Access": tok,
		"AccessExp": time.Now().Add(time.Hour), "Refresh": "hwr_1", "RefreshExp": time.Now().Add(time.Hour), "NextSeq": 1})
	return link.Open(link.Options{Store: mapStore{"link.json": st}, Server: server})
}

// reportServer records the Authorization header of each report.
func reportServer(t *testing.T) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := got
		got = nil
		return out
	}
}

// made is a report made now in ctx, tagged as Outbox.Submit tags it.
func made(o *report.Outbox) report.Report {
	r := report.Bug(report.Game{Version: "dev", Platform: "js/wasm"}, "", "It froze", "", false)
	r.From = o.From()
	return r
}

// TestReportsGoAsTheLinkedGame: a linked game's reports carry its access
// token, so the server knows which game sent them (F-U3-06); an
// unlinked game's, or one linked to another server, don't.
func TestReportsGoAsTheLinkedGame(t *testing.T) {
	srv, got := reportServer(t)
	for _, tc := range []struct {
		name string
		link *link.Client
		want string
	}{
		{"linked", linkedTo(t, srv.URL, "dev_a", "hwd_a"), "Bearer hwd_a"},
		{"linked elsewhere", linkedTo(t, "https://staging.example", "dev_a", "hwd_a"), ""},
		{"not linked", link.Open(link.Options{Store: mapStore{}, Server: srv.URL}), ""},
	} {
		ctx := &Context{Link: tc.link}
		o := &report.Outbox{Sender: &report.Sender{Server: srv.URL}}
		ctx.SendReportsAsLink(o)
		if out, err := o.Sender.Send(context.Background(), made(o)); out != report.Sent {
			t.Fatalf("%s: %v %v", tc.name, out, err)
		}
		if a := got(); len(a) != 1 || a[0] != tc.want {
			t.Errorf("%s: Authorization %q, want %q", tc.name, a, tc.want)
		}
		if l := o.Linked(); l != (tc.want != "") {
			t.Errorf("%s: Linked %v", tc.name, l)
		}
	}
}

// A report one learner made, still queued when another learner plays,
// goes anonymously, never as the other learner; the other learner's own
// reports go as them.
func TestReportsNeverGoAsAnotherLearner(t *testing.T) {
	srv, got := reportServer(t)
	ctx := &Context{Link: linkedTo(t, srv.URL, "dev_a", "hwd_a")}
	o := &report.Outbox{Sender: &report.Sender{Server: srv.URL}}
	ctx.SendReportsAsLink(o)
	fromA := made(o)
	ctx.Link = linkedTo(t, srv.URL, "dev_b", "hwd_b") // switch learners
	ctx.pollLink()
	fromB := made(o)
	for _, r := range []report.Report{fromA, fromB} {
		if out, err := o.Sender.Send(context.Background(), r); out != report.Sent {
			t.Fatalf("%v %v", out, err)
		}
	}
	if a := got(); len(a) != 2 || a[0] != "" || a[1] != "Bearer hwd_b" {
		t.Errorf("Authorization %q, want [\"\" \"Bearer hwd_b\"]", a)
	}
	// Back to the first learner: their report goes as them again.
	ctx.Link = linkedTo(t, srv.URL, "dev_a", "hwd_a")
	ctx.pollLink()
	o.Sender.Send(context.Background(), fromA)
	if a := got(); len(a) != 1 || a[0] != "Bearer hwd_a" {
		t.Errorf("back as A: Authorization %q", a)
	}
}
