package scene

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// memFiles keeps the link's files in memory.
type memFiles map[string][]byte

func (m memFiles) Read(name string) ([]byte, error) {
	if d, ok := m[name]; ok {
		return d, nil
	}
	return nil, fs.ErrNotExist
}
func (m memFiles) Write(name string, d []byte) error        { m[name] = d; return nil }
func (m memFiles) WritePrivate(name string, d []byte) error { m[name] = d; return nil }
func (m memFiles) Remove(name string) error                 { delete(m, name); return nil }

// linkedContext is a test context linked to a server that only links:
// everything queued stays queued.
func linkedContext(t *testing.T) *game.Context {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/link" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer",
			"access_token": "hwd_1", "expires_in": 86400, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
	}))
	t.Cleanup(srv.Close)
	ctx := testContext(t)
	ctx.Link = link.Open(link.Options{Store: memFiles{}, Server: srv.URL})
	if err := ctx.Link.LinkNow(context.Background(), "ABCD-EFGH"); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestAnswersAndSessionsGoToTheLink(t *testing.T) {
	ctx := linkedContext(t)
	fr, _ := words.Lookup("fr")
	c := hardcoreCrawl(t, ctx, 1)
	if c.run.linkMode() != "hardcore" {
		t.Fatal(c.run.linkMode())
	}
	c.kind = "attack"
	c.scoreAnswer(0, words.Result{Tier: words.Perfect}, "", false, 1)
	if n := ctx.Link.Status().Pending; n != 1 {
		t.Fatalf("%d events queued, want the day's totals", n)
	}
	// A session is sent when the player goes back to the title.
	a := newRun(ctx, fr, rpg.Knight, runSetup{mode: compete.Adventure})
	if a.linkMode() != "adventure" {
		t.Fatal(a.linkMode())
	}
	ctx.Playing("adventure", "fr", func() int { return 3 })
	ctx.EndSession() // too short to send
	if n := ctx.Link.Status().Pending; n != 1 {
		t.Fatalf("%d events queued, a short session was sent", n)
	}
}

func TestLockedSettingsCantChange(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	locked := profile.Preset(fr)
	locked.Timer = profile.Relaxed
	ctx.Profile.Settings.Locked = map[string]profile.LangSettings{"fr": locked}
	ctx.Profile.Settings.LockNote = "Set on the website by your teacher."
	s := NewSettings(ctx).(*Settings)
	for s.lang() == nil || s.lang().Code != "fr" {
		s.switchTab(ctx, 1)
	}
	if !s.locked(ctx) {
		t.Fatal("French settings aren't locked")
	}
	if ctx.Profile.Settings.For(fr) != locked {
		t.Fatal("the locked settings don't apply")
	}
	s.switchTab(ctx, 1)
	if s.lang() != nil && s.locked(ctx) {
		t.Fatalf("%s is locked too", s.lang().Name)
	}
}

func TestAssignedListsOnTheShelf(t *testing.T) {
	useTempDir(t)
	starters, _ := game.StarterLists()
	s := newShelf(starters, nil)
	l := importList(t, "title: Pets\nlanguage: fr\ndog = le chien\n")
	l.File = link.AssignedDir + "/lst_1.txt"
	s.addAssigned([]*words.List{l})
	r := s.rows[0]
	if !r.assigned || r.deletable() || undeletable(r) == undeletable(s.rows[1]) {
		t.Fatalf("assigned row %+v", r)
	}
	for _, i := range s.targets("fr") {
		if s.rows[i].assigned {
			t.Fatal("an import could go into an assigned list")
		}
	}
}

func TestAccountHelpers(t *testing.T) {
	if showCode([]rune("AB")) != "AB__-____" || showCode([]rune("ABCDEFGH")) != "ABCD-EFGH" {
		t.Fatal(showCode([]rune("AB")))
	}
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	for d, want := range map[time.Duration]string{
		10 * time.Second: "just now", 5 * time.Minute: "5 minutes ago",
		3 * time.Hour: "3 hours ago", 72 * time.Hour: "3 days ago",
	} {
		if got := ago(now, now.Add(-d)); got != want {
			t.Errorf("ago(%v) = %q", d, got)
		}
	}
	if ago(now, time.Time{}) != "not yet" {
		t.Error("zero time")
	}
	ctx := testContext(t)
	a := NewAccount(ctx).(*Account)
	if items := a.items(ctx); len(items) != 3 || items[0] != acLink || items[1] != acSchool {
		t.Fatalf("unlinked menu %v", items)
	}
	ctx = linkedContext(t)
	if items := a.items(ctx); len(items) != 3 || items[0] != acSync {
		t.Fatalf("linked menu %v", items)
	}
}
