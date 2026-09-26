package settings

import (
	"encoding/json"
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

// TestValuesAreFixed pins the numbers saves and the server store.
func TestValuesAreFixed(t *testing.T) {
	for tm, want := range map[Timer]struct {
		n     uint8
		name  string
		scale float64
	}{
		Normal:  {0, "normal", 1},
		Relaxed: {1, "relaxed", 1.5},
		Fast:    {2, "fast", 0.75},
	} {
		if uint8(tm) != want.n || tm.String() != want.name || tm.Scale() != want.scale || !tm.Valid() {
			t.Errorf("%d: %q, %v, valid %v; want %+v", tm, tm, tm.Scale(), tm.Valid(), want)
		}
		if got, ok := ParseTimer(want.name); !ok || got != tm {
			t.Errorf("ParseTimer(%q) = %v, %v", want.name, got, ok)
		}
	}
	if Timer(3).Valid() || Timer(3).String() != "normal" || Timer(3).Scale() != 1 {
		t.Error("an unknown timer should be invalid and play as normal")
	}
	if _, ok := ParseTimer("slow"); ok {
		t.Error("ParseTimer accepted an unknown timer")
	}
	if len(Timers) != 3 || Timers[0] != Relaxed || Timers[1] != Normal || Timers[2] != Fast {
		t.Errorf("Timers = %v", Timers)
	}
}

func TestLangJSON(t *testing.T) {
	const want = `{"Rules":{"Accents":2,"Breathings":1,"ArticlesRequired":true,"CaseSensitive":false},"Highlight":true,"Timer":1}`
	ls := Lang{Rules: words.Rules{Accents: words.Strict, Breathings: words.Reduced, ArticlesRequired: true}, Highlight: true, Timer: Relaxed}
	b, err := json.Marshal(ls)
	if err != nil || string(b) != want {
		t.Fatalf("json = %s, %v; want %s", b, err, want)
	}
	var back Lang
	if err := json.Unmarshal([]byte(want), &back); err != nil || back != ls {
		t.Fatalf("read back %+v, %v", back, err)
	}
}

func TestPreset(t *testing.T) {
	for _, lang := range words.Languages {
		if p := Preset(lang); p != (Lang{Rules: lang.Defaults}) {
			t.Errorf("%s: Preset = %+v", lang.Code, p)
		}
	}
}
