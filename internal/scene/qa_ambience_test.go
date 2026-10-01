package scene

import (
	"testing"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/pkg/proc"
)

func TestQAMusicAmbienceByTheme(t *testing.T) {
	want := map[string]audio.Ambience{
		"The Crypt": audio.Dust, "Mossy Cellars": audio.Crickets, "Flooded Caves": audio.Drips,
		"Lava Forge": audio.Rumble, "Ice Halls": audio.Wind, "Whispering Library": audio.Pages,
		"Sky Garden": audio.Birds, "Clockwork Workshop": audio.Ticking,
		"Amethyst Vaults": audio.Chimes, "Sandstone Tomb": audio.Sand,
	}
	for name, a := range want {
		c := &Crawl{run: &run{seed: 7, depth: 3}, theme: &proc.Theme{Name: name}}
		tr := c.Music()
		if tr.Mood != audio.Delve || tr.Ambience != a {
			t.Errorf("%s: got mood %v ambience %v, want Delve %v", name, tr.Mood, tr.Ambience, a)
		}
		for _, m := range []mode{modeBattle, modeDead, modeCampfire, modeShop, modeShrine} {
			c.mode = m
			if got := c.Music().Ambience; got != audio.None {
				t.Errorf("%s mode %v: ambience %v, want none", name, m, got)
			}
		}
		c.mode, c.resume = modePause, modeBattle
		if got := c.Music().Ambience; got != audio.None {
			t.Errorf("%s paused in battle: ambience %v", name, got)
		}
		c.resume = modeExplore
		if got := c.Music().Ambience; got != a {
			t.Errorf("%s paused exploring: ambience %v, want %v", name, got, a)
		}
	}
	// Every shipped theme has an ambience, and a nil/unknown theme has none.
	for i := range proc.Themes {
		if worldAmbience[proc.Themes[i].Name] == audio.None {
			t.Errorf("shipped theme %q has no ambience", proc.Themes[i].Name)
		}
	}
	if (&Crawl{run: &run{}}).Music().Ambience != audio.None {
		t.Error("nil theme should have no ambience")
	}
	if (&Crawl{run: &run{}, theme: &proc.Theme{Name: "Nowhere"}}).Music().Ambience != audio.None {
		t.Error("unknown theme should have no ambience")
	}
}
