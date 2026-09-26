package scene

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/pkg/words"
)

// rankContext is a test context linked to a server that has put the
// learner on a French class board and an Irish world board.
func rankContext(t *testing.T) *game.Context {
	t.Helper()
	useTempDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/link":
			json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer",
				"access_token": "hwd_1", "expires_in": 86400, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
		case "/api/v1/lists":
			json.NewEncoder(w).Encode(map[string]any{"lists": []any{}})
		case "/api/v1/me":
			json.NewEncoder(w).Encode(map[string]any{"learner": map[string]any{"id": "lrn_1", "display_name": "Aoife"},
				"settings": map[string]any{}, "accommodations": map[string]any{}, "seen_by": []string{"guardian"},
				"game": map[string]any{"version": "1.0.0", "min_version": "1.0.0", "supported": true}})
		case "/api/v1/memory":
			json.NewEncoder(w).Encode(map[string]any{"lang": r.URL.Query().Get("lang"), "answers": 0, "memory": words.NewMemory()})
		case "/api/v1/assignments":
			w.Write([]byte(`{"assignments":[]}`))
		case "/api/v1/ranks":
			w.Write([]byte(`{"boards":[
 {"scope":"class","kind":"hardcore","lang":"fr","season":"week","period":"2026-W39","entries":[
  {"place":1,"name":"Brave Otter","avatar":"otter","score":5400,"floor":9},
  {"place":2,"name":"Quiet Fox","avatar":"fox","score":3100,"floor":6,"you":true}]},
 {"scope":"world","kind":"daily","lang":"ga","season":"day","period":"2026-09-26","entries":[]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	ctx := testContext(t)
	ctx.Link = link.Open(link.Options{Store: memFiles{}, Server: srv.URL})
	if err := ctx.Link.LinkNow(context.Background(), "ABCD-EFGH"); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Link.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	return ctx
}

// The Hall of Fame has a Rankings tab only once the learner is on a
// board, and it shows the boards for the language chosen.
func TestHallOfFameRankings(t *testing.T) {
	ctx := testContext(t)
	h := NewHallOfFame(ctx).(*HallOfFame)
	if h.tabs(ctx) != len(fameModes) {
		t.Error("a Rankings tab without a board")
	}

	ctx = rankContext(t)
	withFont(t, ctx)
	h = NewHallOfFame(ctx).(*HallOfFame)
	if h.tabs(ctx) != len(fameModes)+1 {
		t.Fatal("no Rankings tab")
	}
	h.mi = len(fameModes)
	if !h.ranking() {
		t.Fatal("not on the Rankings tab")
	}
	for i, l := range words.Languages {
		if l.Code == "fr" {
			h.li = i
		}
	}
	bs := h.boards(ctx)
	if len(bs) != 1 || bs[0].Scope != "class" || len(bs[0].Entries) != 2 || !bs[0].Entries[1].You {
		t.Fatalf("French boards: %+v", bs)
	}
	if got := boardTitle(bs[0]); !strings.Contains(got, "Class") || !strings.Contains(got, "this week") {
		t.Errorf("title %q", got)
	}
	dst := ebiten.NewImage(game.ScreenW, game.ScreenH)
	h.Draw(dst, ctx)
}
