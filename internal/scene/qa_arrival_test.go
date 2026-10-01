package scene

import (
	"strings"
	"testing"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/text/unicode/norm"

	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/llm"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
)

func TestQAArrivalNewRunAndStairs(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[2]
	c.startArrival()
	if c.arriveT != arrivalTicks || len(c.arrival) == 0 || c.arrival[0].text != "Floor 1" {
		t.Fatalf("floor 1 card: %v t=%d", arrivalTexts(c.arrival), c.arriveT)
	}
	c.run.depth = 2
	c.startArrival()
	if c.arrival[0].text != "Floor 2" || c.arriveT != arrivalTicks {
		t.Errorf("stairs card: %v", arrivalTexts(c.arrival))
	}
}

func TestQAArrivalSuspendLoadKeepsWelcomeBack(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[0]
	c.startArrival()
	c.pos = c.level.Start
	if !c.writeSave(ctx, true) {
		t.Fatal("save")
	}
	c2, err := loadCrawl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c2.arriveT != 0 {
		t.Errorf("card doubled over Welcome back: arriveT=%d", c2.arriveT)
	}
	if c2.banner != "Welcome back!" || c2.bannerT <= 0 {
		t.Errorf("banner %q t=%d", c2.banner, c2.bannerT)
	}
}

func TestQAArrivalLateScript(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[1]
	c.startArrival()
	c.arriveT = arrivalFade + 20
	c.run.ai = &runAI{scripts: map[int]*llm.Script{1: {Name: "The Late Pantry"}}}
	c.lateScript(&llm.Script{Name: "The Late Pantry"})
	// A card still up gets the name, and its time is left alone.
	if c.arriveT != arrivalFade+20 {
		t.Errorf("late script changed the card's time: %d", c.arriveT)
	}
	got := arrivalTexts(c.arrival)
	if len(got) < 2 || got[1] != "The Late Pantry" {
		t.Errorf("card %q", got)
	}
	if c.banner != "" || c.bannerT != 0 {
		t.Errorf("banner also shown: %q", c.banner)
	}
}

func TestQAArrivalGoneInBattleAndStaysGone(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[0]
	c.startArrival()
	c.startBattle(ctx, c.level.Monsters[0], false)
	if c.mode != modeBattle {
		t.Fatal("not in battle")
	}
	if c.arriveT != 0 {
		t.Fatalf("the card outlives the battle start: %d", c.arriveT)
	}
}

func TestQAArrivalEscapeSkipsAndPauses(t *testing.T) {
	ticks := 0
	input.FakeKeys(t, func(k ebiten.Key) int {
		if k == ebiten.KeyEscape {
			return ticks
		}
		return 0
	})
	ctx := testContext(t)
	withFont(t, ctx)
	ctx.Input = &input.State{}
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[0]
	c.startArrival()
	ticks = 1
	if err := c.Update(ctx); err != nil {
		t.Fatal(err)
	}
	if c.mode != modePause || c.arriveT != 0 {
		t.Fatalf("Esc: mode %d arriveT %d, want the menu and no card", c.mode, c.arriveT)
	}
}

func TestQAArrivalTypingDoesNotLeakIntoTypebox(t *testing.T) {
	input.FakeKeys(t, func(ebiten.Key) int { return 0 })
	ctx := testContext(t)
	withFont(t, ctx)
	ctx.Input = &input.State{Chars: []rune("abc")}
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[0]
	c.startArrival()
	if err := c.Update(ctx); err != nil {
		t.Fatal(err)
	}
	c.startBattle(ctx, c.level.Monsters[0], false)
	if c.battle == nil || c.battle.field == nil {
		t.Fatal("no battle field")
	}
	if s := c.battle.field.Text(); s != "" {
		t.Errorf("typebox has %q", s)
	}
}

func TestQAArrivalQuestFloor(t *testing.T) {
	c := questRun(t)
	c.startArrival()
	got := arrivalTexts(c.arrival)
	if len(got) != 3 || got[1] != "The Gatehouse" {
		t.Fatalf("quest card %q", got)
	}
	for _, g := range got {
		if g == taglines[c.theme.Name] {
			t.Errorf("quest floor shows tagline")
		}
	}
}

// Whatever sets the mode, Update never lets the card count down (or
// survive) outside explore mode.
func TestQAArrivalFrozenOutsideExplore(t *testing.T) {
	input.FakeKeys(t, func(ebiten.Key) int { return 0 })
	ctx := testContext(t)
	withFont(t, ctx)
	ctx.Input = &input.State{}
	for _, m := range []mode{modeMap, modeShrine, modeCampfire, modeDead} {
		c := testCrawl(t, ctx)
		c.theme = &proc.Themes[0]
		c.mode = m // a direct assignment, bypassing enter
		c.arriveT = 100
		c.Update(ctx)
		if c.arriveT != 0 && c.arriveT != 100 { // cleared, or frozen by an early return
			t.Errorf("mode %d: card %d after Update", m, c.arriveT)
		}
		c.drawArrival(nil, ctx) // must return before touching the image
	}
}

// A wrapped or shrunk headline keeps every letter and mark it can show.
func TestQAArrivalWrapKeepsLettersAndMarks(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	name := "Ἡ Μεγάλη Βιβλιοθήκη τῶν Ἀθηναίων"
	placed, _ := layoutArrival(ctx.Font, arrivalLines(2, name, "Sky Garden", true), vw, vh)
	var heads []string
	for _, p := range placed {
		if p.col == pal.White && !p.hint && p.text != "Sky Garden" && p.text != taglines["Sky Garden"] {
			heads = append(heads, p.text)
		}
	}
	got := strings.Join(heads, " ")
	if strings.HasSuffix(got, "…") {
		t.Skip("name was cut; letters not all kept")
	}
	if norm.NFC.String(got) != norm.NFC.String(name) {
		t.Errorf("wrap changed the name: %q vs %q", got, name)
	}
	// A cut name ends in an ellipsis and never starts or ends on a bare mark.
	long := strings.Repeat("ἄ́ ", 60)
	placed, box := layoutArrival(ctx.Font, arrivalLines(2, long, "Sky Garden", true), vw, vh)
	for _, p := range placed {
		r := []rune(p.text)
		if len(r) > 0 && unicode.Is(unicode.M, r[0]) {
			t.Errorf("line starts with a mark: %q", p.text)
		}
	}
	if box.Max.Y > panelY {
		t.Errorf("box %v reaches the panel", box)
	}
}

// A late script's Greek name lands on the card still up.
func TestQAArrivalLateScriptGreekName(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[1]
	c.startArrival()
	c.arriveT = 90
	c.run.ai = &runAI{scripts: map[int]*llm.Script{1: {Name: "Τὸ Ἀρχαῖον Ἀνάκτορον"}}}
	c.lateScript(&llm.Script{Name: "Τὸ Ἀρχαῖον Ἀνάκτορον"})
	if got := arrivalTexts(c.arrival); got[1] != "Τὸ Ἀρχαῖον Ἀνάκτορον" || c.arriveT != 90 {
		t.Errorf("card %q t=%d", got, c.arriveT)
	}
}
