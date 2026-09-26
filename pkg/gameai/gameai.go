// Package gameai is the game's AI content: the prompts the game asks a
// model for floor scripts, gap-fill sentences, riddles, monster taunts and
// memory tips with, the JSON a model replies with, and the checks every
// reply must pass before the game uses it.
//
// The game's llm package uses it with a player's own API key.
// halpwords-server uses it for Halpwords AI (POST /api/v1/ai/{task}): it
// asks with the same prompts and runs the same checks, and its answers
// have the same JSON shapes, so the game checks them again exactly as it
// checks a model's reply.
package gameai

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/halpworld/halpwords/pkg/safety"
	"github.com/halpworld/halpwords/pkg/words"
	"golang.org/x/text/unicode/norm"
)

// The tasks of Halpwords AI, as POST /api/v1/ai/{task} names them. There
// are no others: the server never offers a raw model.
const (
	Director = "director" // a floor script (DirectorRequest → ScriptReply)
	Cloze    = "cloze"    // gap-fill sentences (WordsRequest → WordsReply)
	Riddles  = "riddles"  // riddles (WordsRequest → WordsReply)
	Taunts   = "taunts"   // monster battle cries (WordsRequest → TauntsReply)
	Insight  = "insight"  // memory tips for the Scroll of Insight (WordsRequest → TipsReply)
)

// Tasks are every task, in the order above.
var Tasks = []string{Director, Cloze, Riddles, Taunts, Insight}

// How many words one request covers.
const (
	// WordBatch is the most words a cloze or riddles request asks about.
	WordBatch = 8
	// TipBatch is the most words an insight request asks about.
	TipBatch = 4
	// TauntWords is the most words a taunts request sends.
	TauntWords = 20
	// TauntCount is how many taunts a request asks for.
	TauntCount = 8
	// DirectorWords is the most words a floor script request sends.
	DirectorWords = 12
	// QuestWords is how many of a quest's words the Director is told.
	QuestWords = 8
	// MaxThemes and MaxMonsters limit a floor script request.
	MaxThemes   = 24
	MaxMonsters = 12
)

// Gap marks the missing word in a gap-fill sentence.
const Gap = "___"

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
	// QuestID is the quest's assignment on the server, for Halpwords AI,
	// which takes the title from there rather than from the game.
	QuestID string
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

// ClozeLine is a sentence in the language being learned with a gap (Gap)
// where a word goes.
type ClozeLine struct {
	Text    string // the sentence, with ___ for the word
	English string // the whole sentence in English
}

// Taunt is a monster's battle cry in the language being learned.
type Taunt struct {
	Text    string
	English string
}

// Word is a word in a request: its English prompt, its answers (the
// first is the one prompts use) and its group.
type Word struct {
	English string   `json:"english"`
	Answers []string `json:"answers"`
	Tag     string   `json:"tag,omitempty"`
}

// WordsOf returns entries as request words.
func WordsOf(entries []words.Entry) []Word {
	out := make([]Word, 0, len(entries))
	for _, e := range entries {
		out = append(out, Word{English: e.Prompt, Answers: append([]string(nil), e.Answers...), Tag: e.Tag})
	}
	return out
}

// Entries returns request words as entries.
func Entries(ws []Word) []words.Entry {
	out := make([]words.Entry, 0, len(ws))
	for _, w := range ws {
		out = append(out, words.Entry{Prompt: w.English, Answers: append([]string(nil), w.Answers...), Tag: w.Tag})
	}
	return out
}

// DirectorRequest is the body of POST /api/v1/ai/director.
type DirectorRequest struct {
	Language string   `json:"language"`
	Depth    int      `json:"depth"`
	Themes   []string `json:"themes"`
	Monsters []string `json:"monsters"`
	Boss     string   `json:"boss,omitempty"`
	Words    []Word   `json:"words"`
	// QuestID is the assignment the floor is built around, if any; the
	// server looks up its title.
	QuestID    string `json:"quest_id,omitempty"`
	QuestWords []Word `json:"quest_words,omitempty"`
}

// WordsRequest is the body of POST /api/v1/ai/cloze, riddles, taunts and
// insight: the language and the words to write about.
type WordsRequest struct {
	Language string `json:"language"`
	Words    []Word `json:"words"`
}

// ScriptReply is a floor script as a model writes it, and the answer of
// POST /api/v1/ai/director.
type ScriptReply struct {
	Name     string            `json:"name"`
	Theme    *int              `json:"theme"`
	Intro    string            `json:"intro"`
	Lore     []string          `json:"lore"`
	Monsters map[string]string `json:"monsters"`
	Boss     string            `json:"boss"`
}

// WordsReply is gap-fill sentences and riddles as a model writes them,
// and the answer of POST /api/v1/ai/cloze and riddles. N numbers the
// request's words from 1.
type WordsReply struct {
	Items []Item `json:"items"`
}

// Item is what was written for one word.
type Item struct {
	N       int    `json:"n"`
	Cloze   string `json:"cloze,omitempty"`
	ClozeEn string `json:"cloze_en,omitempty"`
	Riddle  string `json:"riddle,omitempty"`
}

// TauntsReply is monster battle cries, and the answer of POST
// /api/v1/ai/taunts.
type TauntsReply struct {
	Taunts []TauntItem `json:"taunts"`
}

// TauntItem is one battle cry and its English meaning.
type TauntItem struct {
	Text    string `json:"text"`
	English string `json:"english"`
}

// TipsReply is memory tips, and the answer of POST /api/v1/ai/insight.
// N numbers the request's words from 1.
type TipsReply struct {
	Tips []TipItem `json:"tips"`
}

// TipItem is the memory tip for one word.
type TipItem struct {
	N   int    `json:"n"`
	Tip string `json:"tip"`
}

// DecodeJSON reads the first JSON object in a model's reply into v.
// Models sometimes wrap JSON in a code fence or add a sentence around it.
func DecodeJSON(text string, v any) error {
	err := errors.New("no JSON object")
	for tries := 0; tries < 8; tries++ {
		start := strings.IndexByte(text, '{')
		if start < 0 {
			break
		}
		text = text[start:]
		if err = json.NewDecoder(strings.NewReader(text)).Decode(v); err == nil {
			return nil
		}
		text = text[1:]
	}
	return err
}

// WordLines are the lines of a prompt listing words, numbered from 1.
func WordLines(entries []words.Entry) string {
	var b strings.Builder
	for i, e := range entries {
		answer := ""
		if len(e.Answers) > 0 {
			answer = e.Answers[0]
		}
		fmt.Fprintf(&b, "%d. %s = %s\n", i+1, e.Prompt, answer)
	}
	return b.String()
}

// LangLine names the language for a prompt, with a note on how it is
// written.
func LangLine(lang *words.Language) string {
	switch lang.Code {
	case "grc":
		return "Ancient Greek (Attic, polytonic, with accents and breathings)"
	case "la":
		return "Latin (classical, with macrons where the word list uses them)"
	case "ga":
		return "Irish (Gaeilge, standard spelling with fadas)"
	}
	return lang.Name
}

// DirectorPrompt is the request for a floor script.
func DirectorPrompt(f Floor) string {
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
	return fmt.Sprintf(`You are the Dungeon Director. Plan floor %d of a dungeon for a student learning %s.
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
		f.Depth, f.Lang.Name, WordLines(f.Words[:min(len(f.Words), DirectorWords)]), strings.Join(tags, ", "), questLines(f),
		strings.Join(f.Monsters, ", "), boss, themes.String(), f.Lang.Name)
}

// DirectorTokens caps a floor script's reply.
const DirectorTokens = 700

// questLines tell the Director about the player's quest, if there is one.
func questLines(f Floor) string {
	title := QuestTitle(f.Quest)
	if title == "" {
		return ""
	}
	if len(f.QuestWords) == 0 {
		return fmt.Sprintf("\nThese words are the student's quest %q, set by their teacher or parent. Make the floor feel like part of that quest.", title)
	}
	return fmt.Sprintf("\nThe student is on a quest %q, set by their teacher or parent. Weave its words into the floor too:\n%s",
		title, strings.TrimRight(WordLines(f.QuestWords[:min(len(f.QuestWords), QuestWords)]), "\n"))
}

// QuestTitle tidies a quest's title for a prompt: one line of letters,
// digits, spaces and a few marks, at most 60 characters.
func QuestTitle(s string) string {
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

// WordsPrompt asks for a gap-fill sentence and a riddle for each word at
// once, which is what the game asks a model with a player's own key.
func WordsPrompt(lang *words.Language, entries []words.Entry) string {
	return fmt.Sprintf(`Language being learned: %s.
For each numbered word below (English = %s answer), write:
- "cloze": one short, simple %s sentence (at most 60 characters) that uses the answer exactly as written, including any article, with the answer replaced by ___ (three underscores). The gap must not start the sentence, and the answer must be the natural word for the gap.
- "cloze_en": the whole sentence in English.
- "riddle": a playful English riddle of at most 90 characters, in the voice of a friendly dungeon, that describes the English word without using it.

Words:
%s
Reply with JSON only: {"items":[{"n":1,"cloze":"...","cloze_en":"...","riddle":"..."}]}`,
		LangLine(lang), lang.Name, lang.Name, WordLines(entries))
}

// WordsTokens caps the reply to WordsPrompt for n words.
func WordsTokens(n int) int { return 250*n + 200 }

// ClozePrompt asks for a gap-fill sentence for each word.
func ClozePrompt(lang *words.Language, entries []words.Entry) string {
	return fmt.Sprintf(`Language being learned: %s.
For each numbered word below (English = %s answer), write:
- "cloze": one short, simple %s sentence (at most 60 characters) that uses the answer exactly as written, including any article, with the answer replaced by ___ (three underscores). The gap must not start the sentence, and the answer must be the natural word for the gap.
- "cloze_en": the whole sentence in English.

Words:
%s
Reply with JSON only: {"items":[{"n":1,"cloze":"...","cloze_en":"..."}]}`,
		LangLine(lang), lang.Name, lang.Name, WordLines(entries))
}

// ClozeTokens caps the reply to ClozePrompt for n words.
func ClozeTokens(n int) int { return 160*n + 150 }

// RiddlesPrompt asks for a riddle for each word.
func RiddlesPrompt(lang *words.Language, entries []words.Entry) string {
	return fmt.Sprintf(`Language being learned: %s.
For each numbered word below (English = %s answer), write:
- "riddle": a playful English riddle of at most 90 characters, in the voice of a friendly dungeon, that describes the English word without using it.

Words:
%s
Reply with JSON only: {"items":[{"n":1,"riddle":"..."}]}`,
		LangLine(lang), lang.Name, WordLines(entries))
}

// RiddlesTokens caps the reply to RiddlesPrompt for n words.
func RiddlesTokens(n int) int { return 100*n + 100 }

// TauntsPrompt asks for monster battle cries, using some of the words.
func TauntsPrompt(lang *words.Language, entries []words.Entry) string {
	sample := entries[:min(len(entries), TauntWords)]
	return fmt.Sprintf(`Language being learned: %s.
Write %d short battle cries that silly dungeon monsters shout at a young hero, in simple %s (at most 45 characters each), each with its English meaning. Playful boasting only, nothing cruel. Use some of these words where they fit:
%s
Reply with JSON only: {"taunts":[{"text":"...","english":"..."}]}`,
		LangLine(lang), TauntCount, lang.Name, WordLines(sample))
}

// TauntsTokens caps the reply to TauntsPrompt.
const TauntsTokens = 900

// TipsPrompt asks for a memory tip for each word, for the Scroll of
// Insight.
func TipsPrompt(lang *words.Language, entries []words.Entry) string {
	return fmt.Sprintf(`Language being learned: %s.
A student keeps misspelling these words. For each, write one memory tip of at most 100 characters: a vivid memory hook, a link to an English word, or a note on the tricky letters. Be accurate; only give an etymology you are sure of.
%s
Reply with JSON only: {"tips":[{"n":1,"tip":"..."}]}`, LangLine(lang), WordLines(entries))
}

// TipsTokens caps the reply to TipsPrompt for n words.
func TipsTokens(n int) int { return 120*n + 120 }

// ErrNoName is a floor script without a usable name: the whole script is
// thrown away.
var ErrNoName = errors.New("gameai: the floor script has no usable name")

// CheckScript checks and tidies a floor script. A script needs a good name;
// the other parts are dropped if they fail the checks.
func CheckScript(r ScriptReply, f Floor) (*Script, error) {
	sc := &Script{Name: Tidy(r.Name), Theme: -1, Intro: Tidy(r.Intro)}
	if !Short(sc.Name, 28) || !safety.Clean(sc.Name) || !NameLike(sc.Name) {
		return nil, ErrNoName
	}
	if r.Theme != nil && *r.Theme >= 0 && *r.Theme < len(f.Themes) {
		sc.Theme = *r.Theme
	}
	if !Short(sc.Intro, 100) || !safety.Clean(sc.Intro) {
		sc.Intro = ""
	}
	for _, l := range r.Lore {
		if l = Tidy(l); Short(l, 100) && safety.Clean(l) && len(sc.Lore) < 4 {
			sc.Lore = append(sc.Lore, l)
		}
	}
	known := map[string]bool{}
	for _, m := range f.Monsters {
		known[m] = true
	}
	for k, v := range r.Monsters {
		v = Tidy(v)
		if known[k] && Short(v, 22) && safety.Clean(v) && NameLike(v) {
			if sc.Names == nil {
				sc.Names = map[string]string{}
			}
			sc.Names[k] = v
		}
	}
	if b := Tidy(r.Boss); f.Boss != "" && Short(b, 24) && safety.Clean(b) && NameLike(b) {
		sc.Boss = b
	}
	return sc, nil
}

// Reply is s as a ScriptReply: what passed the checks, so checking it
// again gives s.
func (s *Script) Reply() ScriptReply {
	r := ScriptReply{Name: s.Name, Intro: s.Intro, Lore: append([]string{}, s.Lore...), Monsters: map[string]string{}, Boss: s.Boss}
	if s.Theme >= 0 {
		t := s.Theme
		r.Theme = &t
	}
	for k, v := range s.Names {
		r.Monsters[k] = v
	}
	return r
}

var gaps = regexp.MustCompile(`_{2,}`)

// HasScript reports whether s has a letter in lang's script.
func HasScript(s string, lang *words.Language) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		greek := unicode.Is(unicode.Greek, r)
		if greek == (lang.Script == words.ScriptGreek) {
			return true
		}
	}
	return false
}

// core is a word's answer without its article.
func core(answer string, lang *words.Language) string {
	low := strings.ToLower(answer)
	for _, a := range lang.Articles {
		if rest, ok := strings.CutPrefix(low, a); ok && rest != "" {
			return rest
		}
	}
	return low
}

// CheckCloze checks a generated gap-fill sentence for e and tidies it. The
// gap must stand for the whole first answer of e, so the local grader can
// mark it; the sentence must not give the answer away, and must be short
// enough to show.
func CheckCloze(text, english string, e words.Entry, lang *words.Language) (ClozeLine, bool) {
	text, english = Tidy(text), Tidy(english)
	text = gaps.ReplaceAllString(text, Gap)
	if len(e.Answers) == 0 || strings.Count(text, Gap) != 1 || !Short(text, 70) || !Short(english, 100) {
		return ClozeLine{}, false
	}
	if !safety.Clean(text) || !safety.Clean(english) || !HasScript(text, lang) {
		return ClozeLine{}, false
	}
	answer := e.Answers[0]
	// "Le ___ aboie" for "le chien": the article is outside the gap. Take
	// it out so the gap is the whole answer.
	before, after, _ := strings.Cut(text, Gap)
	lowBefore := strings.ToLower(before)
	for _, a := range lang.Articles {
		if strings.HasPrefix(strings.ToLower(answer), a) && strings.HasSuffix(lowBefore, a) {
			before = before[:len(before)-len(a)]
			break
		}
	}
	text = before + Gap + after
	if c := core(answer, lang); len([]rune(c)) >= 3 && strings.Contains(strings.ToLower(before+after), c) {
		return ClozeLine{}, false // the answer is in the sentence already
	}
	if strings.EqualFold(strings.ReplaceAll(text, Gap, answer), english) {
		return ClozeLine{}, false // not translated
	}
	return ClozeLine{Text: text, English: english}, true
}

// CheckRiddle checks a generated riddle for e and tidies it.
func CheckRiddle(riddle string, e words.Entry) (string, bool) {
	riddle = Tidy(riddle)
	if len(e.Answers) == 0 || !Short(riddle, 110) || !safety.Clean(riddle) {
		return "", false
	}
	low := strings.ToLower(riddle)
	for _, w := range strings.Fields(strings.ToLower(e.Prompt)) {
		if len(w) >= 3 && w != "the" && w != "and" && strings.Contains(low, w) {
			return "", false // gives the word away
		}
	}
	if strings.Contains(low, strings.ToLower(e.Answers[0])) {
		return "", false
	}
	return riddle, true
}

// CheckTaunt checks a generated taunt and tidies it.
func CheckTaunt(text, english string, lang *words.Language) (Taunt, bool) {
	text, english = Tidy(text), Tidy(english)
	text = strings.Trim(text, "\"«»“”")
	english = strings.Trim(english, "\"“”()")
	if !Short(text, 50) || !Short(english, 70) || !safety.Clean(text) || !safety.Clean(english) ||
		!HasScript(text, lang) || strings.EqualFold(text, english) {
		return Taunt{}, false
	}
	return Taunt{Text: text, English: english}, true
}

// CheckTip checks a generated memory tip and tidies it.
func CheckTip(tip string) (string, bool) {
	tip = Tidy(tip)
	if !Short(tip, 120) || !safety.Clean(tip) {
		return "", false
	}
	return tip, true
}

// KeepCloze returns the gap-fill sentences in r that pass CheckCloze, by
// the index of their word in entries.
func KeepCloze(r WordsReply, lang *words.Language, entries []words.Entry) map[int]ClozeLine {
	out := map[int]ClozeLine{}
	for _, it := range r.Items {
		if it.N < 1 || it.N > len(entries) {
			continue
		}
		if _, dup := out[it.N-1]; dup {
			continue
		}
		if c, ok := CheckCloze(it.Cloze, it.ClozeEn, entries[it.N-1], lang); ok {
			out[it.N-1] = c
		}
	}
	return out
}

// KeepRiddles returns the riddles in r that pass CheckRiddle, by the index
// of their word in entries.
func KeepRiddles(r WordsReply, entries []words.Entry) map[int]string {
	out := map[int]string{}
	for _, it := range r.Items {
		if it.N < 1 || it.N > len(entries) {
			continue
		}
		if _, dup := out[it.N-1]; dup {
			continue
		}
		if rd, ok := CheckRiddle(it.Riddle, entries[it.N-1]); ok {
			out[it.N-1] = rd
		}
	}
	return out
}

// KeepTaunts returns the taunts in r that pass CheckTaunt, at most
// TauntCount of them, without repeats.
func KeepTaunts(r TauntsReply, lang *words.Language) []Taunt {
	var out []Taunt
	seen := map[string]bool{}
	for _, t := range r.Taunts {
		if tt, ok := CheckTaunt(t.Text, t.English, lang); ok && !seen[tt.Text] && len(out) < TauntCount {
			seen[tt.Text] = true
			out = append(out, tt)
		}
	}
	return out
}

// KeepTips returns the tips in r that pass CheckTip, by the index of their
// word in entries.
func KeepTips(r TipsReply, entries []words.Entry) map[int]string {
	out := map[int]string{}
	for _, t := range r.Tips {
		if t.N < 1 || t.N > len(entries) {
			continue
		}
		if _, dup := out[t.N-1]; dup {
			continue
		}
		if tip, ok := CheckTip(t.Tip); ok {
			out[t.N-1] = tip
		}
	}
	return out
}

// NameLike reports whether s is fit to be a name: letters, spaces and a
// few marks only.
func NameLike(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !strings.ContainsRune(" '-’.,", r) {
			return false
		}
	}
	return true
}

// Short reports whether s is non-empty and at most n characters long.
func Short(s string, n int) bool {
	c := len([]rune(s))
	return c > 0 && c <= n
}

// drawable are the characters the game's font has (see tools/fontsubset).
var drawable = [][2]rune{
	{0x0020, 0x007E}, {0x00A0, 0x024F}, {0x0370, 0x03FF}, {0x1E00, 0x1EFF},
	{0x1F00, 0x1FFF}, {0x2000, 0x206F}, {0x20A0, 0x20CF}, {0x2190, 0x21FF}, {0x2500, 0x27BF},
}

// Tidy makes generated text ready to draw: composed accents, single
// spaces, and nothing the game's font can't draw, such as emoji.
func Tidy(s string) string {
	s = norm.NFC.String(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) {
			b.WriteByte(' ')
			continue
		}
		for _, rg := range drawable {
			if r >= rg[0] && r <= rg[1] {
				b.WriteRune(r)
				break
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
