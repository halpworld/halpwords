package llm

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/halpworld/halpwords/pkg/safety"
	"github.com/halpworld/halpwords/pkg/words"
)

// Floor is what the Dungeon Director is told about the next floor.
type Floor struct {
	Lang  *words.Language
	Depth int
	// Themes are the looks a floor can have; the script picks one.
	Themes []string
	// Words are the words the hero will meet, due and weak ones first.
	Words []words.Entry
	// Monsters are the kinds of monster on the floor, by name.
	Monsters []string
	// Boss is the boss guarding the stairs, if there is one.
	Boss string
	// Quest is the title of a quest (an assignment from a grown-up) to
	// build the floor around, or "". QuestWords are its words; when they
	// are left out, Words are the quest's words.
	Quest      string
	QuestWords []words.Entry
}

// Script is the Dungeon Director's plan for a floor: names and words that
// dress the procedural floor. Only what passed the checks is set.
type Script struct {
	Name  string // the floor's name, such as "The Drowned Pantry"
	Theme int    // index into Floor.Themes, or -1 for the usual one
	Intro string // said when the hero arrives
	// Lore are short notes the hero finds on the walls.
	Lore []string `json:",omitempty"`
	// Names are new names for the floor's monsters, by kind name.
	Names map[string]string `json:",omitempty"`
	Boss  string            `json:",omitempty"` // the boss's name
}

// Direct asks the Dungeon Director for a floor script.
func (s *Service) Direct(ctx context.Context, f Floor) (*Script, error) {
	var themes strings.Builder
	for i, t := range f.Themes {
		fmt.Fprintf(&themes, "%d: %s\n", i, t)
	}
	var tags []string
	seen := map[string]bool{}
	for _, e := range f.Words {
		if e.Tag != "" && !seen[e.Tag] {
			seen[e.Tag] = true
			tags = append(tags, e.Tag)
		}
	}
	boss := ""
	if f.Boss != "" {
		boss = fmt.Sprintf("\nThe stairs are guarded by a boss, the %s. Give it a grand new name in \"boss\" (at most 24 characters).", f.Boss)
	}
	prompt := fmt.Sprintf(`You are the Dungeon Director. Plan floor %d of a dungeon for a student learning %s.
The student will practise these words on this floor:
%sWord groups: %s%s
Monsters on this floor: %s.%s

Themes to choose from:
%s
Build the floor around the words: a food list might become "The Drowned Pantry".
Reply with JSON only:
{"name": "floor name, at most 28 characters",
 "theme": theme number,
 "intro": "one sentence the hero hears on arrival, at most 90 characters",
 "lore": ["three short notes scratched on the walls, each at most 90 characters, in English, which may mention a %s word from the list"],
 "monsters": {"monster name from the list": "a new fun name tied to the words, at most 22 characters"},
 "boss": ""}`,
		f.Depth, f.Lang.Name, wordLines(f.Words[:min(len(f.Words), 12)]), strings.Join(tags, ", "), questLines(f),
		strings.Join(f.Monsters, ", "), boss, themes.String(), f.Lang.Name)
	text, err := s.Ask(ctx, false, safety.Policy, prompt, 700)
	if err != nil {
		return nil, err
	}
	var out struct {
		Name     string
		Theme    *int
		Intro    string
		Lore     []string
		Monsters map[string]string
		Boss     string
	}
	if err := decodeJSON(text, &out); err != nil {
		return nil, err
	}
	return CheckScript(out.Name, out.Theme, out.Intro, out.Lore, out.Monsters, out.Boss, f)
}

// maxQuestWords is how many of a quest's words the Director is told.
const maxQuestWords = 8

// questLines tell the Director about the player's quest, if there is one.
func questLines(f Floor) string {
	title := questTitle(f.Quest)
	if title == "" {
		return ""
	}
	if len(f.QuestWords) == 0 {
		return fmt.Sprintf("\nThese words are the student's quest %q, set by their teacher or parent. Make the floor feel like part of that quest.", title)
	}
	return fmt.Sprintf("\nThe student is on a quest %q, set by their teacher or parent. Weave its words into the floor too:\n%s",
		title, strings.TrimRight(wordLines(f.QuestWords[:min(len(f.QuestWords), maxQuestWords)]), "\n"))
}

// questTitle tidies a quest's title for a prompt: one line of letters,
// digits, spaces and a few marks, at most 60 characters.
func questTitle(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.Join(strings.Fields(s), " ") {
		if n == 60 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" '-’.,:()&/", r) {
			b.WriteRune(r)
			n++
		}
	}
	return strings.TrimSpace(b.String())
}

// nameLike reports whether s is fit to be a name: letters, spaces and a
// few marks only.
func nameLike(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !strings.ContainsRune(" '-’.,", r) {
			return false
		}
	}
	return true
}

// CheckScript checks and tidies a floor script. A script needs a good name;
// the other parts are dropped if they fail the checks.
func CheckScript(name string, theme *int, intro string, lore []string, monsters map[string]string, boss string, f Floor) (*Script, error) {
	sc := &Script{Name: tidy(name), Theme: -1, Intro: tidy(intro)}
	if !short(sc.Name, 28) || !safety.Clean(sc.Name) || !nameLike(sc.Name) {
		return nil, ErrEmpty
	}
	if theme != nil && *theme >= 0 && *theme < len(f.Themes) {
		sc.Theme = *theme
	}
	if !short(sc.Intro, 100) || !safety.Clean(sc.Intro) {
		sc.Intro = ""
	}
	for _, l := range lore {
		if l = tidy(l); short(l, 100) && safety.Clean(l) && len(sc.Lore) < 4 {
			sc.Lore = append(sc.Lore, l)
		}
	}
	known := map[string]bool{}
	for _, m := range f.Monsters {
		known[m] = true
	}
	for k, v := range monsters {
		v = tidy(v)
		if known[k] && short(v, 22) && safety.Clean(v) && nameLike(v) {
			if sc.Names == nil {
				sc.Names = map[string]string{}
			}
			sc.Names[k] = v
		}
	}
	if b := tidy(boss); f.Boss != "" && short(b, 24) && safety.Clean(b) && nameLike(b) {
		sc.Boss = b
	}
	return sc, nil
}
