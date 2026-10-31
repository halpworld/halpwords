package scene

import "github.com/halpworld/halpwords/internal/audio"

// The scenes' music. Scenes without a Music method play the title music.

// Music implements game.Musical: the floor's own tune while exploring,
// drums in a battle, and something calm by a fire or in a shop. The
// world sets the key, scale and instruments of the exploring and battle
// music; the floor's seed writes the tune.
func (c *Crawl) Music() audio.Track {
	seed := c.run.seed*31 + uint64(c.run.depth)
	var world audio.World
	if c.theme != nil {
		world = audio.WorldNamed(c.theme.Name)
	}
	m := c.mode
	if m == modePause || m == modeQuit {
		m = c.resume
	}
	switch m {
	case modeDead:
		return lamentMusic
	case modeBattle:
		if c.battle != nil && c.battle.m.Kind.Boss() {
			return audio.Track{Mood: audio.Boss, Seed: seed, World: world}
		}
		return audio.Track{Mood: audio.Fight, Seed: seed, World: world}
	case modeCampfire, modeShop, modeShrine:
		return campMusic
	}
	t := audio.Track{Mood: audio.Delve, Seed: seed, World: world}
	if c.theme != nil {
		t.Ambience = worldAmbience[c.theme.Name]
	}
	return t
}

// worldAmbience is the quiet sound under the music in each world, by the
// world's name. A world not listed has none.
var worldAmbience = map[string]audio.Ambience{
	"The Crypt":          audio.Dust,
	"Mossy Cellars":      audio.Crickets,
	"Flooded Caves":      audio.Drips,
	"Lava Forge":         audio.Rumble,
	"Ice Halls":          audio.Wind,
	"Whispering Library": audio.Pages,
	"Sky Garden":         audio.Birds,
	"Clockwork Workshop": audio.Ticking,
	"Amethyst Vaults":    audio.Chimes,
	"Sandstone Tomb":     audio.Sand,
}

var (
	campMusic   = audio.Track{Mood: audio.Camp, Seed: 1}
	lamentMusic = audio.Track{Mood: audio.Lament, Seed: 1}
)

// Music implements game.Musical.
func (g *GameOver) Music() audio.Track { return lamentMusic }

// Music implements game.Musical: calm, for thinking about spelling.
func (p *Practice) Music() audio.Track { return campMusic }
