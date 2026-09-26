package scene

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/playtest"
)

// playtestServer serves a quest file at /quest and nothing else.
func playtestServer(t *testing.T, quest []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(quest)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A quest named in the page's address is fetched and started.
func TestPlaytestStartsTheQuest(t *testing.T) {
	ctx := testContext(t)
	// The built-in quest is in any language; this one is in French.
	data := []byte(strings.Replace(string(mustRead(t)), `"language": ""`, `"language": "fr"`, 1))
	srv := playtestServer(t, data)
	u, err := playtest.FromPage(srv.URL + "/?quest=%2Fquest")
	if err != nil || u == nil {
		t.Fatalf("got %v, %v", u, err)
	}
	p := NewPlaytest(srv.Client(), u)(ctx).(*Playtest)
	next := p.fetched(ctx, true)
	pick, ok := next.(*ClassPick)
	if !ok {
		t.Fatalf("started %T (%q)", next, p.msg)
	}
	if pick.lang.Code != "fr" || pick.setup.quest == nil || pick.setup.quest.Title == "" {
		t.Errorf("picking a class in %s for %v", pick.lang.Code, pick.setup.quest)
	}

	// A quest in any language goes to the language picker.
	srv = playtestServer(t, mustRead(t))
	u, _ = playtest.FromPage(srv.URL + "/?quest=%2Fquest")
	p = NewPlaytest(srv.Client(), u)(ctx).(*Playtest)
	if next := p.fetched(ctx, true); next == nil {
		t.Fatalf("didn't start: %q", p.msg)
	} else if a, ok := next.(*Adventure); !ok || a.setup.quest == nil {
		t.Errorf("started %T", next)
	}
}

func mustRead(t *testing.T) []byte {
	t.Helper()
	data, err := fs.ReadFile(assets.Quests, "quests/scribes-cellars.hwquest")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A link that ran out, or a quest the game can't play, is explained.
func TestPlaytestFails(t *testing.T) {
	ctx := testContext(t)
	srv := playtestServer(t, mustRead(t))
	u, _ := url.Parse(srv.URL + "/gone")
	p := NewPlaytest(srv.Client(), u)(ctx).(*Playtest)
	if next := p.fetched(ctx, true); next != nil || !strings.Contains(p.msg, "Play-test again") {
		t.Errorf("got %T, %q", next, p.msg)
	}
	p = PlaytestError(errors.New("nope"))(ctx).(*Playtest)
	if p.msg != "nope" || p.done != nil {
		t.Errorf("%q", p.msg)
	}
}
