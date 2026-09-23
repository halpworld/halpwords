package dungeon

import "strings"

// Trait is a special power that changes how a monster fights. A monster can
// have several, so they are bit flags.
type Trait uint8

const (
	Armored  Trait = 1 << iota // only exact spelling hurts it
	Ghostly                    // its words fade away
	Mirrored                   // its words are written backwards
	Swift                      // less time to dodge its attacks
)

// Traits lists every trait, in the order they are shown.
var Traits = []Trait{Armored, Ghostly, Mirrored, Swift}

// String returns the names of the traits in t, such as "Armored" or
// "Ghostly, Swift".
func (t Trait) String() string {
	var names []string
	for _, x := range Traits {
		if t&x == 0 {
			continue
		}
		switch x {
		case Armored:
			names = append(names, "Armored")
		case Ghostly:
			names = append(names, "Ghostly")
		case Mirrored:
			names = append(names, "Mirrored")
		case Swift:
			names = append(names, "Swift")
		}
	}
	return strings.Join(names, ", ")
}

// Hint explains a single trait to the player.
func (t Trait) Hint() string {
	switch t {
	case Armored:
		return "Only exact spelling gets through its armor."
	case Ghostly:
		return "Its words fade away. Read them fast!"
	case Mirrored:
		return "Its words are written backwards."
	case Swift:
		return "It strikes fast: you have less time to dodge."
	}
	return ""
}
