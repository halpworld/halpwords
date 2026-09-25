package scene

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/llm"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/proc"
	"github.com/halpworld/halpwords/pkg/puzzle"
	"github.com/halpworld/halpwords/pkg/words"
)

// aiFake is a pretend OpenAI that writes whatever the game asks for.
type aiFake struct {
	mu    sync.Mutex
	asked map[string]int // requests by kind
}

var (
	wordLine    = regexp.MustCompile(`(?m)^(\d+)\. (.+) = (.+)$`)
	monsterLine = regexp.MustCompile(`Monsters on this floor: (.*)\.`)
)

func (f *aiFake) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, "/models") {
		io.WriteString(w, `{"data":[{"id":"gpt-6-luna"}]}`)
		return
	}
	var body struct {
		Messages []struct{ Content string }
	}
	json.NewDecoder(r.Body).Decode(&body)
	prompt := body.Messages[len(body.Messages)-1].Content
	var reply any
	kind := ""
	switch {
	case strings.Contains(prompt, "Dungeon Director"):
		kind = "director"
		names := map[string]string{}
		if m := monsterLine.FindStringSubmatch(prompt); m != nil {
			for _, k := range strings.Split(m[1], ", ") {
				names[k] = "Test " + strings.Fields(k)[len(strings.Fields(k))-1]
			}
		}
		reply = map[string]any{"name": "The Test Pantry", "theme": 3, "intro": "Crumbs cover the floor.",
			"lore": []string{"Note one.", "Note two."}, "monsters": names, "boss": "The Crust King"}
	case strings.Contains(prompt, `"cloze"`):
		kind = "words"
		var items []map[string]any
		for _, m := range wordLine.FindAllStringSubmatch(prompt, -1) {
			items = append(items, map[string]any{"n": atoi(m[1]), "cloze": "Voici ___ ici.",
				"cloze_en": "Here is the " + m[2] + " here.", "riddle": "Guess what I am, brave hero."})
		}
		reply = map[string]any{"items": items}
	case strings.Contains(prompt, "battle cries"):
		kind = "taunts"
		reply = map[string]any{"taunts": []map[string]string{{"text": "Je suis le roi !", "english": "I am the king!"}}}
	case strings.Contains(prompt, "memory tip"):
		kind = "tips"
		var tips []map[string]any
		for _, m := range wordLine.FindAllStringSubmatch(prompt, -1) {
			tips = append(tips, map[string]any{"n": atoi(m[1]), "tip": "Think of " + m[3] + " twice."})
		}
		reply = map[string]any{"tips": tips}
	}
	f.mu.Lock()
	f.asked[kind]++
	f.mu.Unlock()
	text, _ := json.Marshal(reply)
	content, _ := json.Marshal(string(text))
	fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}],"usage":{"prompt_tokens":500,"completion_tokens":300}}`, content)
}

func atoi(s string) int { var n int; fmt.Sscan(s, &n); return n }

func (f *aiFake) count(kind string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked[kind]
}

// aiContext is a test context with an AI connected to a fake.
func aiContext(t *testing.T) (*game.Context, *aiFake) {
	t.Helper()
	ctx := testContext(t)
	f := &aiFake{asked: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	ai := llm.Load(nil)
	ai.IgnoreEnvironment()
	ai.UseEndpoint(llm.OpenAI, srv.URL, srv.Client())
	ai.SetProvider(llm.OpenAI)
	ai.SetKey(llm.OpenAI, "test-key")
	for i := 0; !ai.Status(llm.OpenAI).Checked; i++ {
		if i > 1000 {
			t.Fatal("the key check never finished")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !ai.Ready() {
		t.Fatalf("AI not ready: %s", ai.Problem())
	}
	ctx.AI = ai
	return ctx, f
}

// pump polls the AI until done reports true.
func pump(t *testing.T, c *Crawl, what string, done func() bool) {
	t.Helper()
	for i := 0; !done(); i++ {
		if i > 2000 {
			t.Fatalf("timed out waiting for %s", what)
		}
		c.run.ai.poll(c)
		time.Sleep(time.Millisecond)
	}
}

func hasLog(r *run, s string) bool {
	for _, l := range r.log {
		if strings.Contains(l.text, s) {
			return true
		}
	}
	return false
}

func TestDungeonDirector(t *testing.T) {
	ctx, _ := aiContext(t)
	lang, _ := words.Lookup("fr")
	r := startRun(ctx, lang, rpg.Rogue, 5)
	c := newCrawl(r)
	// Floor 1's script arrives after the hero does, and dresses the floor.
	pump(t, c, "floor 1's script", func() bool { return hasLog(r, "The dungeon stirs") })
	for _, m := range c.level.Monsters {
		if !strings.HasPrefix(m.Name(), "Test ") && !strings.Contains(m.Name(), "Test ") {
			t.Errorf("monster %q was not renamed", m.Name())
		}
	}
	// Floor 2's script is fetched ahead, so it is there on arrival.
	pump(t, c, "floor 2's script", func() bool { return r.ai.scripts[2] != nil })
	r.depth++
	c2 := newCrawl(r)
	if c2.floorName() != "The Test Pantry" || c2.theme != &proc.Themes[3] || c2.sub != "The Test Pantry" {
		t.Fatalf("floor 2: %q, theme %q", c2.floorName(), c2.theme.Name)
	}
	if !hasLog(r, "Crumbs cover the floor.") {
		t.Error("no intro")
	}
	for i := 0; i < loreEvery*2; i++ {
		c2.walked()
	}
	if !hasLog(r, "Note one.") || !hasLog(r, "Note two.") {
		t.Error("no lore on the walls")
	}
	// The script is kept in the save.
	data, err := encodeSave(r, c2.level, c2.pos, c2.facing, true)
	if err != nil {
		t.Fatal(err)
	}
	back, err := decodeSave(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if sc := back.run.ai.scripts[2]; sc == nil || sc.Name != "The Test Pantry" {
		t.Fatalf("script not saved: %+v", sc)
	}
	if c3 := crawlOn(back.run, back.level); c3.floorName() != "The Test Pantry" || c3.theme != &proc.Themes[3] {
		t.Fatal("a loaded floor lost its script")
	}
}

func TestGeneratedContent(t *testing.T) {
	ctx, f := aiContext(t)
	lang, _ := words.Lookup("fr")
	r := startRun(ctx, lang, rpg.Knight, 11)
	c := newCrawl(r)
	pump(t, c, "cloze sentences", func() bool {
		g := r.ai.generated(r)
		return g != nil && len(g.Cloze) > 0 && r.ai.work == nil
	})
	lp := dealKind(t, c, puzzle.Door, puzzle.Cloze)
	if !strings.Contains(lp.p.Clue(), "Voici _____ ici.") {
		t.Fatalf("clue %q", lp.p.Clue())
	}
	if res := lp.p.Check(puzzle.Attempt{Text: puzzleAnswer(lp.p)}); !res.Passed() {
		t.Fatal("the right answer failed")
	}
	c.puzzle, c.mode = nil, modeExplore

	// Taunts come next; a monster shouts one about half the time.
	pump(t, c, "taunts", func() bool { return r.ai.bank(r).TauntCount() > 0 })
	m := c.level.Monsters[0]
	shouted := false
	for i := 0; i < 50 && !shouted; i++ {
		c.startBattle(ctx, m, false)
		shouted = c.battle.taunt.Text == "Je suis le roi !"
	}
	if !shouted || !hasLog(r, "shouts: “Je suis le roi !” (I am the king!)") {
		t.Fatal("no monster taunted")
	}
	c.battle, c.mode = nil, modeExplore

	// A word missed twice gets a memory tip, shown at the next campfire.
	e, id := r.deck.Next()
	for i := 0; i < 2; i++ {
		c.scoreAnswer(id, words.Result{Tier: words.Miss, Expected: e.Answers[0]}, "x", false, 0)
	}
	pump(t, c, "a memory tip", func() bool { _, ok := r.ai.tipFor(r, id); return ok })
	c.rest(&dungeon.Feature{Kind: dungeon.Campfire})
	if len(c.insight) == 0 || !strings.Contains(c.insight[0], "twice") {
		t.Fatalf("no Scroll of Insight: %v", c.insight)
	}
	if f.count("words") < 1 || f.count("taunts") != 1 {
		t.Errorf("asked %v", f.asked)
	}
	// What the AI wrote was paid for and counted.
	if s := ctx.AI.Spent(llm.OpenAI); s.Requests < 5 || s.Spent <= 0 {
		t.Errorf("spending not counted: %+v", s)
	}

	// Scored runs keep to the fixed puzzles, so scores compare.
	r.setMode(ctx, compete.Hardcore)
	if r.ai.generated(r) != nil {
		t.Error("a Hardcore run got generated puzzles")
	}
}

// Without an AI the game asks for nothing and plays as before.
func TestNoAI(t *testing.T) {
	ctx := testContext(t)
	lang, _ := words.Lookup("fr")
	r := startRun(ctx, lang, rpg.Knight, 11)
	c := newCrawl(r)
	c.run.ai.poll(c)
	if r.ai.generated(r) != nil || r.script() != nil || len(r.ai.directing) != 0 || r.ai.work != nil {
		t.Fatal("asked an AI that isn't there")
	}
	if _, ok := r.ai.taunt(r); ok {
		t.Fatal("taunt without AI")
	}
}

func TestMoneyAndModels(t *testing.T) {
	for usd, want := range map[float64]string{0: "$0", 0.0004: "under $0.001", 0.042: "$0.042", 4.995: "$4.995", 5: "$5.00", 12.5: "$12.50", llm.NoLimit: "no limit"} {
		if got := money(usd); got != want {
			t.Errorf("money(%v) = %q, want %q", usd, got, want)
		}
	}
	ai := llm.Load(nil)
	opts := modelOptions(ai, llm.Anthropic, "claude-custom-1")
	if opts[0] != "claude-haiku-4-5" || opts[len(opts)-1] != "claude-custom-1" {
		t.Errorf("options %v", opts)
	}
}
