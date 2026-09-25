package scene

import "github.com/halpworld/halpwords/internal/audio"

// The scenes' music. Scenes without a Music method play the title music.

// Music implements game.Musical: the floor's own tune while exploring,
// drums in a battle, and something calm by a fire or in a shop.
func (c *Crawl) Music() audio.Track {
	seed := c.run.seed*31 + uint64(c.run.depth)
	m := c.mode
	if m == modePause || m == modeQuit {
		m = c.resume
	}
	switch m {
	case modeDead:
		return lamentMusic
	case modeBattle:
		if c.battle != nil && c.battle.m.Kind.Boss() {
			return audio.Track{Mood: audio.Boss, Seed: seed}
		}
		return audio.Track{Mood: audio.Fight, Seed: seed}
	case modeCampfire, modeShop, modeShrine:
		return campMusic
	}
	return audio.Track{Mood: audio.Delve, Seed: seed}
}

var (
	campMusic   = audio.Track{Mood: audio.Camp, Seed: 1}
	lamentMusic = audio.Track{Mood: audio.Lament, Seed: 1}
)

// Music implements game.Musical.
func (g *GameOver) Music() audio.Track { return lamentMusic }

// Music implements game.Musical: calm, for thinking about spelling.
func (p *Practice) Music() audio.Track { return campMusic }
