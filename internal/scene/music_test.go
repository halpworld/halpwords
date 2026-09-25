package scene

import (
	"testing"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/profile"
)

func TestCrawlMusicFollowsTheAction(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	explore := c.Music()
	if explore.Mood != audio.Delve {
		t.Fatalf("exploring plays %v", explore.Mood)
	}

	c.mode, c.battle = modeBattle, &battle{m: c.level.Monsters[0]}
	fight := c.Music()
	if fight.Mood != audio.Fight {
		t.Fatalf("a battle plays %v", fight.Mood)
	}
	c.resume, c.mode = modeBattle, modePause
	if c.Music() != fight {
		t.Fatal("pausing a battle changes the music")
	}

	boss := dungeon.NewMonster(dungeon.BossFor(3), 3, dungeon.Point{}, 1)
	c.mode, c.battle = modeBattle, &battle{m: boss}
	if c.Music().Mood != audio.Boss {
		t.Fatalf("a boss battle plays %v", c.Music().Mood)
	}

	for _, m := range []mode{modeCampfire, modeShop, modeShrine} {
		c.mode = m
		if c.Music().Mood != audio.Camp {
			t.Errorf("mode %d plays %v, want calm music", m, c.Music().Mood)
		}
	}
	c.mode = modeDead
	if c.Music().Mood != audio.Lament {
		t.Errorf("falling plays %v", c.Music().Mood)
	}

	c.mode, c.battle = modeExplore, nil
	c.run.depth++
	if c.Music() == explore {
		t.Error("every floor should have its own tune")
	}
}

func TestSettingsOverAnAdventure(t *testing.T) {
	ctx := testContext(t)
	s := newOptions(ctx).(*Settings)
	if s.tabs() != 1 || s.lang() != nil {
		t.Fatal("over an adventure, Settings should only show Sound & Screen")
	}
	lines := s.lines(ctx)
	if len(lines) != len(options) || lines[0].name != "Music" {
		t.Fatalf("lines %+v", lines)
	}
	lines[0].change(99) // volumes stop at the top
	lines[1].change(-5) // and at the bottom
	o := ctx.Profile.Settings.Options()
	if o.Music != profile.MaxVolume || o.Effects != 0 {
		t.Fatalf("options %+v", o)
	}
	if ctx.Sound.Music != 1 || ctx.Sound.Effects != 0 {
		t.Fatalf("the sound didn't follow the settings: music %v effects %v", ctx.Sound.Music, ctx.Sound.Effects)
	}
	if !ctx.Shake() {
		t.Fatal("screen shake should be on by default")
	}
}

func TestSettingsTabs(t *testing.T) {
	ctx := testContext(t)
	s := NewSettings(ctx).(*Settings)
	if s.tabs() != 5 || s.lang() != nil {
		t.Fatalf("%d tabs, want Sound & Screen then four languages", s.tabs())
	}
	s.switchTab(ctx, -1)
	if s.lang() == nil || s.lang().Code != "ga" {
		t.Fatalf("going left from the first tab should wrap to the last language, got %v", s.lang())
	}
}
