package scene

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/maps"
	"github.com/halpworld/halpwords/pkg/puzzle"
	"github.com/halpworld/halpwords/pkg/words"
)

// The quests built into the game pass every check, in every language the
// game has lists for.
func TestBuiltInQuests(t *testing.T) {
	ctx := testContext(t)
	quests := builtInQuests()
	if len(quests) == 0 {
		t.Fatal("no built-in quests")
	}
	for _, qf := range quests {
		if qf.q.Language != "" {
			t.Errorf("%s is only in %s", qf.q.Title, qf.q.Language)
		}
		for _, lang := range words.Languages {
			if len(ctx.ListsFor(lang.Code)) == 0 {
				continue
			}
			q := *qf.q
			q.Language = lang.Code
			if ps := q.Check(ctx.Lists); len(ps) > 0 {
				t.Errorf("%s in %s: %v", q.Title, lang.Name, ps)
			}
		}
	}
}

// questRun starts the first built-in quest in French.
func questRun(t *testing.T) *Crawl {
	t.Helper()
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	q := builtInQuests()[0].q
	r := newRun(ctx, fr, rpg.Knight, runSetup{quest: q})
	c := crawlOn(r, r.floor(1))
	c.arrive()
	return c
}

// A quest's maps are its floors, with their titles, looks and locks.
func TestQuestFloors(t *testing.T) {
	c := questRun(t)
	r := c.run
	m := r.quest.Map(0)
	if w, h := m.Size(); c.level.W != w || c.level.H != h || c.level.At(c.pos) != dungeon.Floor || c.pos != (dungeon.Point{X: 1, Y: 1}) {
		t.Fatalf("floor 1 is %dx%d with the hero at %v, not the map", c.level.W, c.level.H, c.pos)
	}
	if c.floorName() != "The Gatehouse" || c.theme.Name != "The Crypt" || r.script() != nil || r.hardcore() {
		t.Errorf("floor %q theme %q", c.floorName(), c.theme.Name)
	}
	if last := r.log[len(r.log)-1].text; !strings.Contains(last, "Floor 1 of 2: The Gatehouse") {
		t.Errorf("arrived with %q", last)
	}
	if r.lastFloor() {
		t.Error("floor 1 of 2 is the last")
	}
	// The anagram lock gets an anagram (or a spelling puzzle, for a word
	// too short to scramble).
	door := dungeon.Point{X: 2, Y: 4}
	anagrams := 0
	for range 30 {
		switch k := c.makePuzzle(puzzle.Door, door).Kind(); k {
		case puzzle.Anagram:
			anagrams++
		case puzzle.Spell:
		default:
			t.Fatalf("the anagram door has a %s puzzle", k)
		}
	}
	if anagrams == 0 {
		t.Error("no anagrams on the anagram door")
	}
	// A lock with a word set asks for that word.
	id := maps.FindWord(r.deck.Entries(), "le chat")
	c.level.Locks[door] = dungeon.Lock{Kind: puzzle.Riddle, Set: true, Word: "cat"}
	if p := c.makePuzzle(puzzle.Door, door); p.Kind() != puzzle.Riddle || p.Word() != id {
		t.Errorf("made a %s about word %d, want a riddle about %d", p.Kind(), p.Word(), id)
	}
	c.level.Locks[door] = dungeon.Lock{Word: "le chat"}
	for range 20 {
		if p := c.makePuzzle(puzzle.Door, door); p.Word() != id {
			t.Fatalf("made a %s about word %d, want %d", p.Kind(), p.Word(), id)
		}
	}
	// A word the lists lack gets a puzzle about another word.
	c.level.Locks[door] = dungeon.Lock{Kind: puzzle.Reverse, Set: true, Word: "le zeppelin"}
	if p := c.makePuzzle(puzzle.Door, door); p.Kind() != puzzle.Reverse {
		t.Errorf("made a %s", p.Kind())
	}

	// Notes are read when the hero faces their wall, once.
	c.pos, c.facing = dungeon.Point{X: 6, Y: 3}, dungeon.South
	n := len(r.log)
	c.readNote()
	c.readNote()
	if len(r.log) != n+1 || !strings.Contains(r.log[n].text, "Two sealed doors lead on") {
		t.Errorf("log %q", r.log[n:])
	}

	// The next floor is the second map, with its boss.
	r.depth = 2
	l := r.floor(2)
	if l.Boss() == nil || l.Boss().Kind.Name != "Slime King" || l.Depth != 3 {
		t.Errorf("floor 2 has depth %d and boss %v", l.Depth, l.Boss())
	}
	if !r.lastFloor() {
		t.Error("floor 2 of 2 is not the last")
	}
	end := questEnd(r)
	if end.title != r.quest.Title || !strings.Contains(end.text[0], "scribe's book") {
		t.Errorf("ending %+v", end)
	}
}

// A quest saves and loads, floor and all.
func TestQuestSave(t *testing.T) {
	useTempDir(t)
	c := questRun(t)
	r := c.run
	r.depth = 2
	l := r.floor(2)
	l.Set(dungeon.Point{X: 3, Y: 4}, dungeon.OpenDoor)
	data, err := encodeSave(r, l, l.Start, dungeon.East, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := save.Write(saveName, data); err != nil {
		t.Fatal(err)
	}
	if s, _ := saveSummary(); s != "The Scribe's Cellars · French · Knight · Floor 2" {
		t.Errorf("summary %q", s)
	}
	got, err := decodeSave(testContext(t), data)
	if err != nil {
		t.Fatal(err)
	}
	if got.run.quest == nil || got.run.quest.Title != r.quest.Title || got.run.depth != 2 {
		t.Fatalf("loaded quest %v depth %d", got.run.quest, got.run.depth)
	}
	if got.level.At(dungeon.Point{X: 3, Y: 4}) != dungeon.OpenDoor || got.level.W != l.W || got.level.Boss() == nil {
		t.Error("the floor did not come back")
	}
}

// Dropped files become quests, kept in the quests folder; broken ones are
// refused with the reason.
func TestAddQuestFile(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	m := builtInQuests()[0].q.Map(1)
	m.Title, m.Language = "Week 5", "fr"
	data, _ := m.Encode()
	s := NewQuests(ctx, droppedFile{"week5.hwmap", data}).(*Quests)
	if s.list[s.sel].q.Title != "Week 5" || !strings.Contains(s.msg, "Added") {
		t.Fatalf("selected %q, said %q", s.list[s.sel].q.Title, s.msg)
	}
	names, _ := save.List(questsDir)
	if len(names) != 1 || names[0] != "week-5.hwquest" {
		t.Fatalf("saved %v", names)
	}
	// It is there next time, once.
	s = NewQuests(ctx, droppedFile{"week5.hwmap", data}).(*Quests)
	n := 0
	for _, qf := range s.list {
		if qf.q.Title == "Week 5" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the quest is listed %d times", n)
	}
	m.Rows[0] = "#"
	bad, _ := m.Encode()
	s = NewQuests(ctx, droppedFile{"broken.hwmap", bad}).(*Quests)
	if !strings.Contains(s.msg, "broken.hwmap") || !strings.Contains(s.msg, "Row 0") {
		t.Errorf("said %q", s.msg)
	}
	if got := questAbout(s.list[0]); got != "2 floors · any language" {
		t.Errorf("about %q", got)
	}
}
