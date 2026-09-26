package gameai

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

func french() *words.Language { l, _ := words.Lookup("fr"); return l }
func greek() *words.Language  { l, _ := words.Lookup("grc"); return l }

func TestCheckCloze(t *testing.T) {
	dog := words.Entry{Prompt: "dog", Answers: []string{"le chien"}}
	for _, c := range []struct {
		text, en string
		want     string // "" when rejected
	}{
		{"Je promène ___ au parc.", "I walk the dog in the park.", "Je promène ___ au parc."},
		{"Je promène le ____ au parc.", "I walk the dog in the park.", "Je promène ___ au parc."},
		{"Le ___ aboie.", "The dog barks.", "___ aboie."},
		{"Le chien et ___ jouent.", "The dog and the dog play.", ""}, // gives it away
		{"Je promène le chien.", "I walk the dog.", ""},              // no gap
		{"___ et ___", "x", ""},                      // two gaps
		{"Je promène ___ au parc, putain.", "x", ""}, // not clean
		{"I walk ___ in the park.", "I walk the dog in the park.", "I walk ___ in the park."},
	} {
		got, ok := CheckCloze(c.text, c.en, dog, french())
		if (c.want == "") == ok || (ok && got.Text != c.want) {
			t.Errorf("CheckCloze(%q) = %q, %v; want %q", c.text, got.Text, ok, c.want)
		}
	}
	horse := words.Entry{Prompt: "horse", Answers: []string{"ὁ ἵππος"}}
	if _, ok := CheckCloze("The ___ runs.", "The horse runs.", horse, greek()); ok {
		t.Error("a Greek cloze needs Greek")
	}
	if _, ok := CheckCloze("τρέχει ___.", "The horse runs.", horse, greek()); !ok {
		t.Error("a Greek cloze was refused")
	}
	if _, ok := CheckCloze("Je vois ___.", "I see.", words.Entry{Prompt: "x"}, french()); ok {
		t.Error("a word without answers got a sentence")
	}
}

func TestCheckRiddleTauntTip(t *testing.T) {
	cat := words.Entry{Prompt: "cat", Answers: []string{"le chat"}}
	if _, ok := CheckRiddle("I purr on your lap and chase mice.", cat); !ok {
		t.Error("good riddle refused")
	}
	if _, ok := CheckRiddle("I am a cat.", cat); ok {
		t.Error("riddle naming the word accepted")
	}
	if _, ok := CheckTaunt("«Ton pain est à moi !»", "Your bread is mine!", french()); !ok {
		t.Error("good taunt refused")
	}
	if tt, _ := CheckTaunt("«Ton pain est à moi !»", "Your bread is mine!", french()); tt.Text != "Ton pain est à moi !" {
		t.Errorf("quotes not trimmed: %q", tt.Text)
	}
	if _, ok := CheckTaunt("I will eat you", "I will eat you", french()); ok {
		t.Error("untranslated taunt accepted")
	}
	if Tidy("Hi 🐉  there\nfriend") != "Hi there friend" {
		t.Errorf("Tidy: %q", Tidy("Hi 🐉  there\nfriend"))
	}
	if _, ok := CheckTip("visit http://x"); ok {
		t.Error("a tip with a link was kept")
	}
	if tip, ok := CheckTip("  Chien sounds like 'she-an'. "); !ok || tip != "Chien sounds like 'she-an'." {
		t.Errorf("CheckTip: %q, %v", tip, ok)
	}
}

func TestKeep(t *testing.T) {
	entries := []words.Entry{
		{Prompt: "dog", Answers: []string{"le chien"}},
		{Prompt: "cat", Answers: []string{"le chat"}},
	}
	var r WordsReply
	if err := DecodeJSON("Here:\n```json\n"+`{"items":[
		{"n":1,"cloze":"Je promène ___ au parc.","cloze_en":"I walk the dog in the park.","riddle":"I wag my tail at the postman."},
		{"n":1,"cloze":"Je vois ___.","cloze_en":"I see the dog.","riddle":"I fetch sticks."},
		{"n":2,"cloze":"Le chat dort sur ___.","cloze_en":"The cat sleeps.","riddle":"I am a cat."},
		{"n":9,"cloze":"x ___","cloze_en":"y","riddle":"z"}]}`+"\n```", &r); err != nil {
		t.Fatal(err)
	}
	cloze := KeepCloze(r, french(), entries)
	if len(cloze) != 1 || cloze[0].Text != "Je promène ___ au parc." {
		t.Fatalf("KeepCloze: %v", cloze)
	}
	riddles := KeepRiddles(r, entries)
	if len(riddles) != 1 || riddles[0] != "I wag my tail at the postman." {
		t.Fatalf("KeepRiddles: %v", riddles)
	}
	var tr TauntsReply
	json.Unmarshal([]byte(`{"taunts":[{"text":"Ton pain est à moi !","english":"Your bread is mine!"},
		{"text":"Ton pain est à moi !","english":"Your bread is mine!"},{"text":"x","english":"x"}]}`), &tr)
	if got := KeepTaunts(tr, french()); len(got) != 1 {
		t.Fatalf("KeepTaunts: %v", got)
	}
	tips := KeepTips(TipsReply{Tips: []TipItem{{N: 2, Tip: "Chat: a cat that chats."}, {N: 0, Tip: "x"}}}, entries)
	if len(tips) != 1 || tips[1] == "" {
		t.Fatalf("KeepTips: %v", tips)
	}
}

func TestScript(t *testing.T) {
	two := 2
	f := Floor{Lang: french(), Themes: []string{"Crypt", "Cellars", "Caves"}, Monsters: []string{"Green Slime", "Cave Bat"}, Boss: "Slime King"}
	r := ScriptReply{Name: "The Drowned Pantry", Theme: &two, Intro: "Water drips on old loaves.",
		Lore:     []string{"The cook hid le pain here.", "http://bad", "Beware the soup."},
		Monsters: map[string]string{"Green Slime": "Soggy Baguette", "Dragon": "Nope", "Cave Bat": "Fork 🍴 Bat"}, Boss: "The Crust King"}
	sc, err := CheckScript(r, f)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Name != "The Drowned Pantry" || sc.Theme != 2 || len(sc.Lore) != 2 || sc.Boss != "The Crust King" ||
		sc.Names["Cave Bat"] != "Fork Bat" || sc.Names["Dragon"] != "" {
		t.Fatalf("script %+v", sc)
	}
	// What passed the checks passes them again unchanged: the server
	// checks a script, and the game checks the server's answer.
	again, err := CheckScript(sc.Reply(), f)
	if err != nil || !reflect.DeepEqual(again, sc) {
		t.Fatalf("checked again: %+v, %v", again, err)
	}
	if _, err := CheckScript(ScriptReply{Name: "Bad http://"}, Floor{}); err != ErrNoName {
		t.Fatalf("a script with a bad name: %v", err)
	}
}

func TestPrompts(t *testing.T) {
	entries := []words.Entry{{Prompt: "dog", Answers: []string{"le chien"}, Tag: "pets"}}
	for name, p := range map[string]string{
		"words":   WordsPrompt(french(), entries),
		"cloze":   ClozePrompt(french(), entries),
		"riddles": RiddlesPrompt(french(), entries),
		"taunts":  TauntsPrompt(french(), entries),
		"tips":    TipsPrompt(french(), entries),
		"director": DirectorPrompt(Floor{Lang: french(), Depth: 2, Themes: []string{"Crypt"}, Words: entries,
			Monsters: []string{"Cave Bat"}, Quest: "Pets\nweek 3 <b>", QuestWords: entries}),
	} {
		if !strings.Contains(p, "1. dog = le chien") || !strings.Contains(p, "JSON only") {
			t.Errorf("%s prompt: %s", name, p)
		}
	}
	if p := DirectorPrompt(Floor{Lang: french(), Words: entries, Quest: "Pets\nweek 3 <b>"}); !strings.Contains(p, `"Pets week 3 b"`) {
		t.Errorf("quest title not tidied: %s", p)
	}
	if !slices.Equal(Tasks, []string{"director", "cloze", "riddles", "taunts", "insight"}) {
		t.Errorf("tasks %v", Tasks)
	}
}

func TestWords(t *testing.T) {
	entries := []words.Entry{{Prompt: "dog", Answers: []string{"le chien", "chien"}, Tag: "pets"}}
	if got := Entries(WordsOf(entries)); !reflect.DeepEqual(got, entries) {
		t.Fatalf("round trip: %+v", got)
	}
}
