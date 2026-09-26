package link

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/gameai"
)

func TestAskAI(t *testing.T) {
	f, c, _, clk := linked(t)
	if c.AIAvailable() {
		t.Fatal("AI available from a server that doesn't offer it")
	}
	if err := c.AskAI(context.Background(), gameai.Taunts, gameai.WordsRequest{}, nil); !errors.Is(err, ErrNoAI) {
		t.Fatalf("AskAI without AI: %v", err)
	}
	f.mu.Lock()
	f.me["ai"] = map[string]any{"available": true, "tasks": gameai.Tasks}
	f.ai = func(task string, body []byte) (int, any) {
		var req gameai.WordsRequest
		if err := json.Unmarshal(body, &req); err != nil || req.Language != "fr" {
			t.Errorf("request %s", body)
		}
		switch task {
		case gameai.Taunts:
			return 200, map[string]any{"taunts": []any{map[string]any{"text": "Ton pain est à moi !", "english": "Your bread is mine!"}}, "written_with_ai": true}
		case gameai.Cloze:
			return 429, CodeAIBudget
		}
		return 403, CodeAIOff
	}
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !c.AIAvailable() {
		t.Fatal("AI not available")
	}
	req := gameai.WordsRequest{Language: "fr", Words: []gameai.Word{{English: "bread", Answers: []string{"le pain"}}}}
	var out gameai.TauntsReply
	if err := c.AskAI(context.Background(), gameai.Taunts, req, &out); err != nil || len(out.Taunts) != 1 {
		t.Fatalf("AskAI: %+v, %v", out, err)
	}
	// Errors are not tried again.
	var e *Error
	if err := c.AskAI(context.Background(), gameai.Cloze, req, &out); !errors.As(err, &e) || e.Code != CodeAIBudget {
		t.Fatalf("budget: %v", err)
	}
	if n := f.count("/api/v1/ai/cloze"); n != 1 {
		t.Fatalf("tried %d times", n)
	}
	// A token that ran out is refreshed first; one the server refuses is
	// refreshed and the request made again.
	clk.add(24 * time.Hour)
	if err := c.AskAI(context.Background(), gameai.Taunts, req, &out); err != nil || c.AccessToken() == "hwd_1" {
		t.Fatalf("after the token ran out: %v", err)
	}
	f.failNext("/api/v1/ai/taunts", 401)
	before := c.AccessToken()
	if err := c.AskAI(context.Background(), gameai.Taunts, req, &out); err != nil || c.AccessToken() == before {
		t.Fatalf("after a 401: %v", err)
	}
	// Turned off on the server: the game stops asking until it syncs.
	if err := c.AskAI(context.Background(), gameai.Riddles, req, &out); !errors.As(err, &e) || e.Code != CodeAIOff {
		t.Fatalf("off: %v", err)
	}
	if c.AIAvailable() {
		t.Fatal("still available after ai_off")
	}
	f.mu.Lock()
	f.me["ai"] = map[string]any{"available": false}
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.AIAvailable() {
		t.Fatal("available when the server says not")
	}
}

func TestListRiddles(t *testing.T) {
	f, c, _, _ := linked(t)
	if r := c.Riddles(c.Lists()); r != nil {
		t.Fatalf("riddles from a server without them: %v", r)
	}
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Words: 2, Text: animals,
		Riddles: []ListRiddle{
			{English: "Dog", Riddle: "I wag my tail at the postman."},
			{English: "cat", Riddle: "I am a cat."},      // gives it away
			{English: "horse", Riddle: "I gallop away."}, // not in the list
		}})
	f.mu.Lock()
	f.etag = `"v2"`
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	r := c.Riddles(c.Lists())
	if len(r) != 1 || len(r["dog"]) != 1 || r["dog"][0] != "I wag my tail at the postman." {
		t.Fatalf("riddles %v", r)
	}
	// They are kept with the link.
	c2 := Open(Options{Store: c.o.Store, Server: f.srv.URL})
	if len(c2.Riddles(c2.Lists())["dog"]) != 1 {
		t.Fatal("riddles not kept")
	}
}
