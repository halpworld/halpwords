package dungeon

import "fmt"

// State is everything about a floor that changes as it is played: opened
// doors, the automap, chests and monsters. The rest comes back from the
// floor's seed, so a saved game only needs to keep this.
type State struct {
	Tiles    []Tile
	Seen     []byte // 1 for each cell on the automap
	Chests   []ChestState
	Features []FeatureState `json:",omitempty"`
	Monsters []MonsterState
}

// FeatureState is a shrine, campfire or merchant and where it stands.
type FeatureState struct {
	At Point
	Feature
}

// ChestState is a chest and where it stands.
type ChestState struct {
	At Point
	Chest
}

// MonsterState is a monster, with its kind stored by name.
type MonsterState struct {
	Kind           string
	At             Point
	HP, MaxHP, ATK int
	Seed           uint64
	Traits, Extra  Trait
	Awake          bool
	Stun           int
	Facing         Dir
	Loot           *Chest `json:",omitempty"`
	Phase          int    `json:",omitempty"`
}

// State returns the floor's changeable state.
func (f *Level) State() State {
	s := State{
		Tiles: append([]Tile(nil), f.tiles...),
		Seen:  make([]byte, len(f.Seen)),
	}
	for i, seen := range f.Seen {
		if seen {
			s.Seen[i] = 1
		}
	}
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			p := Point{x, y}
			if c := f.Chests[p]; c != nil {
				s.Chests = append(s.Chests, ChestState{At: p, Chest: *c})
			}
			if ft := f.Features[p]; ft != nil {
				s.Features = append(s.Features, FeatureState{At: p, Feature: *ft})
			}
		}
	}
	for _, m := range f.Monsters {
		s.Monsters = append(s.Monsters, MonsterState{
			Kind: m.Kind.Name, At: m.At, HP: m.HP, MaxHP: m.MaxHP, ATK: m.ATK,
			Seed: m.Seed, Traits: m.Traits, Extra: m.Extra, Awake: m.Awake,
			Stun: m.Stun, Facing: m.Facing, Loot: m.Loot, Phase: m.Phase,
		})
	}
	return s
}

// Restore puts back a state from State. The level must be the same floor,
// generated from the same seed and depth.
func (f *Level) Restore(s State) error {
	if len(s.Tiles) != len(f.tiles) || len(s.Seen) != len(f.Seen) {
		return fmt.Errorf("saved floor is %d cells, want %d", len(s.Tiles), len(f.tiles))
	}
	chests := map[Point]*Chest{}
	for _, c := range s.Chests {
		if !f.In(c.At) {
			return fmt.Errorf("chest at %v is off the map", c.At)
		}
		chests[c.At] = &c.Chest
	}
	features := map[Point]*Feature{}
	for _, ft := range s.Features {
		if !f.In(ft.At) {
			return fmt.Errorf("%s at %v is off the map", ft.Kind, ft.At)
		}
		features[ft.At] = &ft.Feature
	}
	var monsters []*Monster
	for _, ms := range s.Monsters {
		k := KindNamed(ms.Kind)
		if k == nil {
			return fmt.Errorf("unknown monster %q", ms.Kind)
		}
		if !f.In(ms.At) {
			return fmt.Errorf("%s at %v is off the map", ms.Kind, ms.At)
		}
		monsters = append(monsters, &Monster{
			Kind: k, At: ms.At, HP: ms.HP, MaxHP: ms.MaxHP, ATK: ms.ATK,
			Seed: ms.Seed, Traits: ms.Traits, Extra: ms.Extra, Awake: ms.Awake,
			Stun: ms.Stun, Facing: ms.Facing, Loot: ms.Loot, Phase: ms.Phase,
		})
	}
	copy(f.tiles, s.Tiles)
	for i, v := range s.Seen {
		f.Seen[i] = v != 0
	}
	f.Chests, f.Features, f.Monsters = chests, features, monsters
	return nil
}

// KindNamed returns the monster kind called name, or nil.
func KindNamed(name string) *Kind {
	if name == MimicKind.Name {
		return &MimicKind
	}
	for i := range BossKinds {
		if BossKinds[i].Name == name {
			return &BossKinds[i]
		}
	}
	for i := range Kinds {
		if Kinds[i].Name == name {
			return &Kinds[i]
		}
	}
	return nil
}
