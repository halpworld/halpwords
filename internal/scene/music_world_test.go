package scene

import (
	"testing"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/pkg/proc"
)

func TestCrawlMusicFollowsTheWorld(t *testing.T) {
	seen := map[audio.World]string{}
	for i := range proc.Themes {
		th := &proc.Themes[i]
		c := &Crawl{run: &run{seed: 7, depth: 3}, theme: th}
		w := c.Music().World
		if w == audio.NoWorld {
			t.Errorf("%s: exploring has no world music", th.Name)
		}
		if o, ok := seen[w]; ok {
			t.Errorf("%s and %s share %s", th.Name, o, w)
		}
		seen[w] = th.Name
		c.mode = modeBattle
		if got := c.Music(); got.Mood != audio.Fight || got.World != w {
			t.Errorf("%s: battle plays %v in %v", th.Name, got.Mood, got.World)
		}
		for _, m := range []mode{modeDead, modeCampfire, modeShop, modeShrine} {
			c.mode = m
			if got := c.Music().World; got != audio.NoWorld {
				t.Errorf("%s mode %v: world %v, want none", th.Name, m, got)
			}
		}
	}
	if (&Crawl{run: &run{}}).Music().World != audio.NoWorld {
		t.Error("nil theme has world music")
	}
}
