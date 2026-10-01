package scene

import (
	"encoding/json"
	"image"
	"strings"
	"testing"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/llm"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/pkg/proc"
)

var arrivalWorlds = []string{"The Crypt", "Mossy Cellars", "Flooded Caves", "Ice Halls", "Lava Forge",
	"Amethyst Vaults", "Clockwork Workshop", "Sky Garden", "Sandstone Tomb", "Whispering Library"}

func arrivalTexts(ls []arrivalLine) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.text)
	}
	return out
}

func TestArrivalHeadlinePrecedence(t *testing.T) {
	// A quest map's title leads, with the world as a small label and no tagline.
	q := questRun(t)
	q.startArrival()
	got := arrivalTexts(q.arrival)
	if len(got) != 3 || got[0] != "Floor 1" || got[1] != "The Gatehouse" || got[2] != "The Crypt" {
		t.Errorf("quest card %q", got)
	}

	// The Director's name beats the world's name, and a generated floor has a tagline.
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[1]
	c.run.ai = &runAI{scripts: map[int]*llm.Script{1: {Name: "The Test Pantry"}}}
	c.startArrival()
	got = arrivalTexts(c.arrival)
	if len(got) != 4 || got[1] != "The Test Pantry" || got[2] != "Mossy Cellars" || got[3] != taglines["Mossy Cellars"] {
		t.Errorf("director card %q", got)
	}

	// Alone, the world's name is the headline and is not repeated.
	c.run.ai = nil
	c.startArrival()
	got = arrivalTexts(c.arrival)
	if len(got) != 3 || got[1] != "Mossy Cellars" || got[2] != taglines["Mossy Cellars"] {
		t.Errorf("world card %q", got)
	}
	if c.arriveT != arrivalTicks {
		t.Errorf("card lasts %d ticks", c.arriveT)
	}
}

func TestArrivalTaglines(t *testing.T) {
	for _, n := range arrivalWorlds {
		tag := taglines[n]
		if tag == "" {
			t.Errorf("%s has no tagline", n)
		}
		if w := len(strings.Fields(tag)); w > 8 {
			t.Errorf("%s tagline has %d words", n, w)
		}
	}
	if len(taglines) != len(arrivalWorlds) {
		t.Errorf("%d taglines for %d worlds", len(taglines), len(arrivalWorlds))
	}
	for _, th := range proc.Themes {
		if taglines[th.Name] == "" {
			t.Errorf("theme %s has no tagline", th.Name)
		}
	}
}

// The key that skips the card does nothing else; movement is not blocked.
func TestArrivalSkipKeyDoesNotAct(t *testing.T) {
	for _, key := range []ebiten.Key{ebiten.KeySpace, ebiten.KeyEnter} {
		held := 0
		input.FakeKeys(t, func(k ebiten.Key) int {
			if k == key {
				return held
			}
			return 0
		})
		ctx := testContext(t)
		withFont(t, ctx)
		ctx.Input = &input.State{}
		c := testCrawl(t, ctx)
		c.theme = &proc.Themes[0]
		c.startArrival()
		held = 1
		if err := c.Update(ctx); err != nil {
			t.Fatal(err)
		}
		if c.arriveT != 0 {
			t.Errorf("key %v did not skip the card", key)
		}
		if c.mode != modeExplore {
			t.Errorf("key %v also acted: mode %d", key, c.mode)
		}
	}
}

func TestArrivalCardStaysOverTheView(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	view := image.Rect(viewX, viewY, viewX+vw, viewY+vh)
	long := strings.Repeat("The Very Long Name of a Floor ", 6)
	for _, name := range append([]string{long, "A"}, arrivalWorlds...) {
		ls := arrivalLines(12, name, "Whispering Library", true)
		placed, box := layoutArrival(ctx.Font, ls, vw, vh)
		if !box.In(view) {
			t.Errorf("%.20q: box %v outside view %v", name, box, view)
		}
		if box.Max.Y > panelY {
			t.Errorf("box reaches the panel: %v", box)
		}
		for _, p := range placed {
			r := image.Rect(p.x, p.y, p.x+ctx.Font.Width(p.text, p.scale), p.y+gfx.LineHeight*p.scale)
			if !r.In(box) {
				t.Errorf("%.20q: line %q at %v outside box %v", name, p.text, r, box)
			}
			if mid := (r.Min.X + r.Max.X) / 2; mid < viewX+vw/2-1 || mid > viewX+vw/2+1 {
				t.Errorf("line %q is not centred (%d)", p.text, mid)
			}
		}
	}
}

func TestArrivalDrawsAndFades(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[1]
	c.startArrival()
	dst := ebiten.NewImage(game.ScreenW, game.ScreenH)
	for _, n := range []int{arrivalTicks, arrivalFade, 10, 1, 0} {
		c.arriveT = n
		c.drawArrival(dst, ctx)
	}
}

func TestCalmOptionDefaultsOffAndRoundTrips(t *testing.T) {
	if profile.DefaultOptions().Calm {
		t.Fatal("calm effects are on by default")
	}
	// A settings file from before the option loads with it off.
	var s profile.Settings
	old := `{"Game":{"Music":6,"Effects":8,"CRT":0,"Fullscreen":false,"Shake":true}}`
	if err := json.Unmarshal([]byte(old), &s); err != nil {
		t.Fatal(err)
	}
	if s.Options().Calm {
		t.Error("an old file turns calm on")
	}
	// Off adds nothing to the file; on survives a save and load.
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "Calm") {
		t.Errorf("off is written: %s", b)
	}
	o := s.Options()
	o.Calm = true
	s.SetOptions(o)
	b, _ = json.Marshal(s)
	var back profile.Settings
	if err := json.Unmarshal(b, &back); err != nil || !back.Options().Calm {
		t.Fatalf("calm lost on the way: %s %v", b, err)
	}

	// The row in Sound & Screen changes it, and the context reports it.
	ctx := testContext(t)
	var row *option
	for _, l := range options {
		if l.name == "Calm effects" {
			l := l
			row = &l
		}
	}
	if row == nil || row.about == "" {
		t.Fatal("no Calm effects row")
	}
	row.set(&o, 0)
	ctx.Profile.Settings.SetOptions(o)
	ctx.ApplyOptions()
	if !ctx.Calm() || !(&Crawl{}).calm(ctx) {
		t.Error("the option is not reaching the crawl")
	}
}

func TestRaidIsInTheMossyCellars(t *testing.T) {
	if th := raidTheme(); th.Name != "Mossy Cellars" {
		t.Errorf("raid theme %q", th.Name)
	}
}

// A late Director script after the card is gone is a banner, as before; and
// starting a card never wipes a banner that is up.
func TestArrivalLateScriptAfterCardUsesBanner(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[1]
	c.arriveT = 0
	c.run.ai = &runAI{scripts: map[int]*llm.Script{1: {Name: "The Late Pantry"}}}
	c.lateScript(&llm.Script{Name: "The Late Pantry"})
	if c.arriveT != 0 || c.banner != "Floor 1" || c.sub != "The Late Pantry" || c.bannerT == 0 {
		t.Errorf("card %d banner %q/%q", c.arriveT, c.banner, c.sub)
	}
	c.showBanner("LEVEL UP!", "Level 2")
	c.startArrival()
	if c.banner != "LEVEL UP!" || c.bannerT == 0 {
		t.Errorf("startArrival wiped the banner: %q", c.banner)
	}
}

// The card counts down in explore mode only, and goes when the hero opens
// a menu, a battle or a puzzle.
func TestArrivalLeavesWithExploreMode(t *testing.T) {
	input.FakeKeys(t, func(ebiten.Key) int { return 0 })
	ctx := testContext(t)
	withFont(t, ctx)
	ctx.Input = &input.State{}
	c := testCrawl(t, ctx)
	c.theme = &proc.Themes[0]
	c.startArrival()
	c.Update(ctx)
	if c.arriveT != arrivalTicks-1 {
		t.Fatalf("explore did not count down: %d", c.arriveT)
	}
	for _, m := range []mode{modeBattle, modePuzzle, modeShop, modeCampfire, modeShrine, modeMap, modeDead, modePause} {
		c.mode = modeExplore
		c.startArrival()
		c.enter(m)
		if c.arriveT != 0 {
			t.Errorf("mode %d keeps the card", m)
		}
	}
}

// A quest floor's wall note goes to the log and leaves the card up; a boss
// banner takes the card's place once, at the start of the fight.
func TestArrivalWithNoteAndBossBanner(t *testing.T) {
	ctx := testContext(t)
	c := questRun(t)
	c.run.sound = &game.Sound{Muted: true}
	c.startArrival()
	c.level.Notes = map[dungeon.Point]string{c.pos.Step(c.facing): "Mind the gap."}
	c.arrive()
	if c.arriveT != arrivalTicks || c.banner != "" {
		t.Errorf("a note disturbed the card: %d %q", c.arriveT, c.banner)
	}
	c.showBanner("Boss", "guards the stairs!")
	if c.arriveT != 0 || c.banner != "Boss" {
		t.Errorf("banner and card both up: %d %q", c.arriveT, c.banner)
	}
	_ = ctx
}

func TestArrivalLongNamesWrapAndFit(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	view := image.Rect(viewX, viewY, viewX+vw, viewY+vh)
	names := []string{
		"The Marvellous Kingdom of the Forgotten Clockwork Gardens",
		"Η Μεγάλη Βιβλιοθήκη τῶν Ἀθηναίων καὶ τῶν Ἑλλήνων ἁπάντων τοῦ κόσμου",
		strings.Repeat("ἀ", 80),
	}
	for _, name := range names {
		placed, box := layoutArrival(ctx.Font, arrivalLines(3, name, "Sky Garden", true), vw, vh)
		if !box.In(view) || box.Max.Y > panelY {
			t.Errorf("%.12q: box %v outside the view", name, box)
		}
		head := 0
		for _, p := range placed {
			if p.scale == 2 || (p.col == pal.White && p.scale == 1 && p.text != "Sky Garden" && p.text != taglines["Sky Garden"]) {
				head++
				if r := []rune(p.text); unicode.Is(unicode.M, r[0]) {
					t.Errorf("line starts with a combining mark: %q", p.text)
				}
			}
			if w := ctx.Font.Width(p.text, p.scale); w > box.Dx() {
				t.Errorf("%q wider than the box", p.text)
			}
		}
		if head > 2 {
			t.Errorf("%.12q takes %d lines", name, head)
		}
	}
	// A middling name wraps at full size onto two lines rather than shrinking.
	placed, _ := layoutArrival(ctx.Font, arrivalLines(3, "The Great Hall of Clockwork Wonders", "Sky Garden", true), vw, vh)
	n := 0
	for _, p := range placed {
		if p.scale == 2 {
			n++
		}
	}
	if n != 2 {
		t.Errorf("middling name has %d full-size lines, want 2", n)
	}
}

// Letters keep their accents when a name is cut.
func TestArrivalClustersKeepMarks(t *testing.T) {
	for _, cl := range clusters("ἀ\u0301b" + "ε\u0308") {
		if len([]rune(cl)) == 0 {
			t.Fatal("empty cluster")
		}
	}
	if got := clusters("e\u0301x"); len(got) != 2 || got[0] != "\u00e9" {
		t.Errorf("clusters %q", got)
	}
}
