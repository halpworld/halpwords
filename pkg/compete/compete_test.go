package compete

import (
	"errors"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

func TestScore(t *testing.T) {
	tl := Tally{Damage: 400, Perfect: 10, BestCombo: 7, Bosses: 1, Chests: 3, Misses: 4}
	want := 5*1000 + 400 + 10*50 + 7*100 + 2500 + 3*150 - 4*25
	if got := tl.Score(5); got != want {
		t.Fatalf("score %d, want %d", got, want)
	}
	if got := (Tally{Misses: 100}).Score(1); got != 0 {
		t.Fatalf("score %d, want 0", got)
	}
}

func TestSeedCodes(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 200; i++ {
		seed := RandomSeed(rng)
		code := SeedCode(seed)
		if len(code) != 6 {
			t.Fatalf("code %q", code)
		}
		got, err := ParseSeed(strings.ToLower(code[:3]) + " " + code[3:])
		if err != nil || got != seed {
			t.Fatalf("ParseSeed(%q) = %d, %v; want %d", code, got, err, seed)
		}
	}
	if s, err := ParseSeed("O1IL00"); err != nil || SeedCode(s) != "011100" {
		t.Fatalf("look-alike letters: %v %v", SeedCode(s), err)
	}
	for _, bad := range []string{"", "ABCDE", "ABCDEFG", "ABCDU!"} {
		if _, err := ParseSeed(bad); err == nil {
			t.Errorf("ParseSeed(%q) passed", bad)
		}
	}
}

func TestDailySeed(t *testing.T) {
	es := []words.Entry{{Prompt: "dog", Answers: []string{"le chien"}}, {Prompt: "cat", Answers: []string{"le chat"}}}
	rev := []words.Entry{es[1], es[0]}
	day := time.Date(2026, 9, 22, 8, 0, 0, 0, time.Local)
	later := time.Date(2026, 9, 22, 23, 0, 0, 0, time.Local)
	a := DailySeed(day, "fr", es)
	if a != DailySeed(later, "fr", rev) {
		t.Fatal("the same day and words gave another dungeon")
	}
	if a == DailySeed(day.AddDate(0, 0, 1), "fr", es) || a == DailySeed(day, "la", es) || a == DailySeed(day, "fr", es[:1]) {
		t.Fatal("another day, language or list gave the same dungeon")
	}
	if a>>SeedBits != 0 {
		t.Fatal("a daily seed has no seed code")
	}
}

func TestShareCodes(t *testing.T) {
	for _, s := range []Share{
		{Lang: "fr", Daily: true, Month: 9, Day: 22, Floor: 12, Score: 18450},
		{Lang: "grc", Seed: 123456789, Floor: 3, Score: 0},
		{Lang: "la", Seed: 7, Floor: 1, Score: 999},
	} {
		code := s.Code()
		got, err := ParseShare(code)
		if err != nil || got != s {
			t.Fatalf("ParseShare(%q) = %+v, %v; want %+v", code, got, err, s)
		}
		if got, err := ParseShare(" " + strings.ToLower(code) + " "); err != nil || got != s {
			t.Fatalf("lower case %q: %v", code, err)
		}
		// Changing the score breaks the checksum.
		parts := strings.Split(code, "-")
		parts[4] = strconv.Itoa(s.Score + 1)
		if _, err := ParseShare(strings.Join(parts, "-")); !errors.Is(err, ErrBadChecksum) {
			t.Fatalf("an edited code: %v", err)
		}
	}
	if !strings.HasPrefix((Share{Lang: "fr", Daily: true, Month: 9, Day: 22, Floor: 12, Score: 18450}).Code(), "HW-FR-0922-F12-18450-") {
		t.Fatal("daily code layout")
	}
	for _, bad := range []string{"", "HW-XX-0922-F1-1-AAAA", "HW-FR-1322-F1-1-AAAA", "HW-FR-0922-12-1-AAAA", "HW-FR-0922-F1-01-AAAA"} {
		if _, err := ParseShare(bad); err == nil {
			t.Errorf("ParseShare(%q) passed", bad)
		}
	}
}

func TestSeedFromCode(t *testing.T) {
	if s, err := SeedFromCode("7k3qzp"); err != nil || SeedCode(s) != "7K3QZP" {
		t.Fatalf("seed code: %v %v", s, err)
	}
	code := Share{Lang: "fr", Seed: 4242, Floor: 2, Score: 10}.Code()
	if s, err := SeedFromCode(code); err != nil || s != 4242 {
		t.Fatalf("share code: %v %v", s, err)
	}
	daily := Share{Lang: "fr", Daily: true, Month: 1, Day: 2, Floor: 2, Score: 10}.Code()
	if _, err := SeedFromCode(daily); err == nil {
		t.Fatal("a daily share code gave a seed")
	}
}

func TestHallOfFame(t *testing.T) {
	var h HallOfFame
	key := TableKey(Hardcore, "fr")
	if h.Best(key) != 0 || h.Rank(key, 0) != 1 {
		t.Fatal("empty table")
	}
	for i := 1; i <= 12; i++ {
		h.Add(key, Fame{Name: "p", Score: i * 100})
	}
	tb := h.Table(key)
	if len(tb) != FameSize || tb[0].Score != 1200 || tb[FameSize-1].Score != 300 {
		t.Fatalf("table %+v", tb)
	}
	if r := h.Add(key, Fame{Score: 100}); r != 0 {
		t.Fatalf("a low score made place %d", r)
	}
	if r := h.Add(key, Fame{Name: "new", Score: 1200}); r != 2 || h.Table(key)[1].Name != "new" {
		t.Fatalf("a tie went to place %d", r)
	}
	if h.Best(TableKey(Daily, "fr")) != 0 {
		t.Fatal("tables are shared")
	}
}
