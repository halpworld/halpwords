package playtest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

const page = "https://play.example/halpwords/"

func pageWith(raw string) string { return page + "?" + Param + "=" + url.QueryEscape(raw) }

// Only an address on the game's own origin is accepted.
func TestFromPage(t *testing.T) {
	good := map[string]string{
		"https://play.example/playtest/o/q/1?exp=1&sig=x": "https://play.example/playtest/o/q/1?exp=1&sig=x",
		"/playtest/o/q/1?exp=1&sig=x":                     "https://play.example/playtest/o/q/1?exp=1&sig=x",
		"quest.hwquest":                                   "https://play.example/halpwords/quest.hwquest",
		"HTTPS://PLAY.EXAMPLE/q#frag":                     "https://play.example/q",
	}
	for raw, want := range good {
		u, err := FromPage(pageWith(raw))
		if err != nil || u == nil || u.String() != want {
			t.Errorf("%q: got %v, %v; want %s", raw, u, err, want)
		}
	}
	bad := []string{
		"https://evil.example/q.hwquest",
		"//evil.example/q.hwquest",
		"http://play.example/q.hwquest",       // another scheme
		"https://play.example:8443/q.hwquest", // another port
		"https://play.example.evil.example/q",
		"https://user:pw@play.example/q",
		"https://play.example@evil.example/q",
		"/\\evil.example/q",
		"\\\\evil.example/q",
		"/\tq",
		"javascript:alert(1)",
		"data:application/json,{}",
		"file:///etc/passwd",
		"ftp://play.example/q",
		"https:q",
	}
	for _, raw := range bad {
		if u, err := FromPage(pageWith(raw)); err == nil {
			t.Errorf("%q: accepted as %v", raw, u)
		}
	}
	if _, err := FromPage(pageWith("https://evil.example/q")); !errors.Is(err, ErrCrossOrigin) {
		t.Errorf("another website: %v", err)
	}
	if u, err := FromPage(page); u != nil || err != nil {
		t.Errorf("no parameter: got %v, %v", u, err)
	}
	if _, err := FromPage("file:///index.html?quest=/q"); err == nil {
		t.Error("a page that isn't on the web")
	}
}

func testQuest(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../assets/quests/scribes-cellars.hwquest")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A quest is fetched, checked and returned; errors say what went wrong.
func TestFetch(t *testing.T) {
	quest := testQuest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			if r.Method != http.MethodGet || r.Header.Get("Cookie") != "" {
				t.Errorf("%s with cookie %q", r.Method, r.Header.Get("Cookie"))
			}
			w.Write(quest)
		case "/gone":
			http.NotFound(w, r)
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/big":
			w.Write([]byte(strings.Repeat(" ", MaxFile+1)))
		case "/bad":
			w.Write([]byte(`{"format":"hwquest","version":1,"title":"Empty","maps":[]}`))
		default:
			w.Write([]byte("not json"))
		}
	}))
	defer srv.Close()
	u := func(p string) *url.URL { v, _ := url.Parse(srv.URL + p); return v }
	q, err := Fetch(context.Background(), srv.Client(), u("/ok"))
	if err != nil || q.Title == "" || len(q.Maps) == 0 {
		t.Fatalf("got %v, %v", q, err)
	}
	for _, p := range []string{"/gone", "/redirect", "/big", "/bad", "/junk"} {
		if q, err := Fetch(context.Background(), srv.Client(), u(p)); err == nil {
			t.Errorf("%s: got %q", p, q.Title)
		}
	}
	if _, err := Fetch(context.Background(), srv.Client(), u("/gone")); err == nil || !strings.Contains(err.Error(), "Play-test again") {
		t.Errorf("a run-out link says %v", err)
	}
}
