package proc

import (
	"image/color"

	"github.com/halpworld/halpwords/internal/pal"
)

// Theme is the look of a dungeon floor, its world: what the walls, floor
// and ceiling are made of, the air (fog and light), the room height, one
// prop and one kind of particle. Each ramp runs dark to light.
//
// Doors, sealed doors, torches, stairs and chests keep the same shape in
// every world; only the frame material changes, so a child never has to
// relearn what an exit looks like.
type Theme struct {
	Name  string
	Wall  []color.RGBA
	Floor []color.RGBA
	Ceil  []color.RGBA
	Moss  []color.RGBA // growth on walls

	// The surfaces of the world. A Surface with no Look.Base paints with
	// the ramps above: Wall and Moss for walls, Floor for floors, and Ceil
	// with timber beams for ceilings.
	Walls    Surface
	Variants [2][]Modifier // added to Walls for wall variants 1 and 2
	Floors   Surface
	Ceiling  Surface
	Frame    []color.RGBA // door frames and stair steps; nil means Wall

	Fog    color.RGBA   // what darkness and distance fade to
	Light  color.RGBA   // the light's tint; the zero value is white
	Reach  float64      // multiplies the hero's light radius; 0 means 1
	Height float64      // ceiling height in wall heights; 0 means 1
	Sky    []color.RGBA // when set, open sky replaces the ceiling, top to horizon

	Prop      func(seed uint64) *Indexed // a decoration standing on the floor
	PropSize  float64                    // its height in wall heights, at most 0.4
	PropRooms float64                    // props per room, on average
	Particles Particles
}

// Particles is the kind of particle drifting through a world's air.
type Particles uint8

// The particle kinds.
const (
	NoParticles Particles = iota
	ParticleDust
	ParticleFireflies
	ParticleDrips
	ParticleSnow
	ParticleEmbers
	ParticleSparkles
	ParticleSteam
	ParticlePetals
	ParticleSand
	ParticleLetters
)

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 0xff} }

// Themes lists every world. The first six are the original themes: their
// names and indices are part of saved Director scripts and of the server's
// map tools, so worlds are only ever appended. FloorOrder decides which
// floor gets which world.
var Themes = []Theme{
	{
		Name:     "The Crypt",
		Wall:     []color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Granite, pal.Stone, pal.Ash},
		Floor:    []color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Granite, pal.Stone},
		Ceil:     []color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Mahogany},
		Moss:     []color.RGBA{pal.Forest, pal.Olive},
		Walls:    Surface{Paint: WallBricks},
		Variants: [2][]Modifier{{ModMoss}, {ModCrack}},
		Floors:   Surface{Paint: FloorFlagstones},
		Ceiling:  Surface{Paint: CeilBeams},
		Fog:      pal.Black,
		Light:    rgb(255, 239, 216), // today's warm torchlight, exactly
		Prop:     PropBones, PropSize: 0.35, PropRooms: 0.5,
		Particles: ParticleDust,
	},
	{
		Name:  "Mossy Cellars",
		Wall:  []color.RGBA{pal.Black, pal.Night, pal.Olive, pal.Bronze, pal.Moss, pal.Tan},
		Floor: []color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Olive, pal.Forest},
		Ceil:  []color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Olive},
		Moss:  []color.RGBA{pal.Forest, pal.Green},
		Walls: Surface{Paint: WallEarth, Mods: []Modifier{ModMoss}, Look: Look{
			Base:   []color.RGBA{pal.Black, pal.Plum, pal.Mahogany, pal.Brown, pal.Tan},
			Accent: []color.RGBA{pal.Forest, pal.Green, pal.Lime},
		}},
		Variants: [2][]Modifier{{ModVines}, {ModVines, ModMoss}},
		Floors: Surface{Paint: FloorCobbles, Look: Look{
			Base:   []color.RGBA{pal.Night, pal.Slate, pal.Granite, pal.Stone, pal.Ash},
			Accent: []color.RGBA{pal.Forest, pal.Green},
		}},
		Ceiling: Surface{Paint: WallBricks, Mods: []Modifier{ModMoss}, Look: Look{
			Base:   []color.RGBA{pal.Black, pal.Plum, pal.Mahogany, pal.Brown, pal.Bronze},
			Accent: []color.RGBA{pal.Forest, pal.Olive},
		}},
		Frame: []color.RGBA{pal.Black, pal.Olive, pal.Forest, pal.Moss, pal.Lime, pal.Skin},
		Fog:   rgb(24, 32, 12),
		Light: rgb(240, 255, 200),
		Prop:  PropMushrooms, PropSize: 0.4, PropRooms: 0.6,
		Particles: ParticleFireflies,
	},
	{
		Name:  "Flooded Caves",
		Wall:  []color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Navy, pal.Teal, pal.Cyan},
		Floor: []color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Navy, pal.Teal},
		Ceil:  []color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Navy},
		Moss:  []color.RGBA{pal.Teal, pal.Green},
		Walls: Surface{Paint: WallCave, Look: Look{
			Base:   []color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Navy, pal.Teal, pal.Cyan},
			Accent: []color.RGBA{pal.Navy, pal.Cyan, pal.Ice},
		}},
		Variants: [2][]Modifier{{ModDrips}, {ModDrips, ModDrips}},
		Floors: Surface{Paint: FloorWater, Anim: AnimRipples, Look: Look{
			Base:   []color.RGBA{pal.Night, pal.Indigo, pal.Navy, pal.Blue},
			Accent: []color.RGBA{pal.Sky, pal.Cyan},
		}},
		Ceiling: Surface{Paint: CeilRock, Look: Look{
			Base: []color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Navy, pal.Teal},
		}},
		Frame: []color.RGBA{pal.Black, pal.Night, pal.Navy, pal.Steel, pal.Ice, pal.White},
		Fog:   rgb(10, 24, 40),
		Light: rgb(210, 240, 255),
		Reach: 1.25,
		Prop:  PropStalagmite, PropSize: 0.4, PropRooms: 0.6,
		Particles: ParticleDrips,
	},
	{
		Name:  "Ice Halls",
		Wall:  []color.RGBA{pal.Night, pal.Indigo, pal.Navy, pal.Steel, pal.Ice, pal.White},
		Floor: []color.RGBA{pal.Night, pal.Indigo, pal.Navy, pal.Steel, pal.Ice},
		Ceil:  []color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Navy},
		Moss:  []color.RGBA{pal.Sky, pal.Cyan},
		Walls: Surface{Paint: WallIce, Look: Look{
			Base: []color.RGBA{pal.Indigo, pal.Navy, pal.Blue, pal.Sky, pal.Cyan, pal.Ice, pal.White},
		}},
		Variants: [2][]Modifier{{ModFrost}, {ModCrack}},
		Floors: Surface{Paint: FloorSnow, Look: Look{
			Base: []color.RGBA{pal.Navy, pal.Steel, pal.Ice, pal.White},
		}},
		Ceiling: Surface{Paint: CeilIce, Look: Look{
			Base:   []color.RGBA{pal.Indigo, pal.Navy, pal.Steel},
			Accent: []color.RGBA{pal.Navy, pal.Cyan, pal.Ice, pal.White},
		}},
		Frame:  []color.RGBA{pal.Black, pal.Black, pal.Night, pal.Navy, pal.Indigo, pal.Navy},
		Fog:    rgb(40, 50, 100),
		Light:  rgb(220, 235, 255),
		Height: 1.5,
		Prop:   PropIce, PropSize: 0.4, PropRooms: 0.5,
		Particles: ParticleSnow,
	},
	{
		Name:  "Lava Forge",
		Wall:  []color.RGBA{pal.Black, pal.Plum, pal.Mahogany, pal.Brown, pal.Red, pal.Orange},
		Floor: []color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Mahogany, pal.Brown},
		Ceil:  []color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Mahogany},
		Moss:  []color.RGBA{pal.Orange, pal.Yellow},
		Walls: Surface{Paint: WallBasalt, Look: Look{
			Base:   []color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Granite, pal.Stone},
			Accent: []color.RGBA{pal.Red, pal.Orange, pal.Yellow},
		}},
		Variants: [2][]Modifier{{ModGlowCracks}, {ModSoot}},
		Floors: Surface{Paint: FloorLava, Anim: AnimLava, Look: Look{
			Base:   []color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Mahogany},
			Accent: []color.RGBA{pal.Red, pal.Orange, pal.Yellow},
		}},
		Ceiling: Surface{Paint: CeilRock, Look: Look{
			Base: []color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Mahogany, pal.Red},
		}},
		Frame: []color.RGBA{pal.Black, pal.Mahogany, pal.Red, pal.Orange, pal.Yellow, pal.White},
		Fog:   rgb(60, 14, 10),
		Light: rgb(255, 210, 170),
		Prop:  PropAnvil, PropSize: 0.35, PropRooms: 0.4,
		Particles: ParticleEmbers,
	},
	{
		Name:  "Amethyst Vaults",
		Wall:  []color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Purple, pal.Pink, pal.Skin},
		Floor: []color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Plum, pal.Purple},
		Ceil:  []color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Plum},
		Moss:  []color.RGBA{pal.Pink, pal.Skin},
		Walls: Surface{Paint: WallCrystal, Look: Look{
			Base:   []color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Purple, pal.Pink, pal.Skin},
			Accent: []color.RGBA{pal.Pink, pal.Skin, pal.White},
		}},
		Variants: [2][]Modifier{{ModVeins}, {ModVeins, ModVeins}},
		Floors: Surface{Paint: FloorChecker, Look: Look{
			Base:   []color.RGBA{pal.Night, pal.Indigo, pal.Plum, pal.Purple},
			Accent: []color.RGBA{pal.Plum, pal.Purple, pal.Pink, pal.Skin},
		}},
		Ceiling: Surface{Paint: CeilCrystal, Look: Look{
			Base:   []color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Plum, pal.Purple},
			Accent: []color.RGBA{pal.Purple, pal.Pink, pal.Skin},
		}},
		Frame: []color.RGBA{pal.Black, pal.Plum, pal.Purple, pal.Pink, pal.White, pal.Ice},
		Fog:   rgb(40, 16, 56),
		Light: rgb(245, 220, 255),
		Prop:  PropCrystals, PropSize: 0.4, PropRooms: 0.5,
		Particles: ParticleSparkles,
	},
	{
		Name:  "Clockwork Workshop",
		Wall:  []color.RGBA{pal.Black, pal.Slate, pal.Granite, pal.Stone, pal.Ash, pal.Steel},
		Floor: []color.RGBA{pal.Black, pal.Mahogany, pal.Brown, pal.Bronze, pal.Tan},
		Ceil:  []color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Granite},
		Moss:  []color.RGBA{pal.Bronze, pal.Tan},
		Walls: Surface{Paint: WallMetal, Look: Look{
			Base:   []color.RGBA{pal.Black, pal.Slate, pal.Granite, pal.Stone, pal.Ash, pal.Steel},
			Accent: []color.RGBA{pal.Mahogany, pal.Bronze, pal.Tan, pal.Yellow},
		}},
		Variants: [2][]Modifier{{ModGear}, {ModPipe}},
		Floors:   Surface{Paint: FloorGrate},
		Ceiling: Surface{Paint: CeilPipes, Look: Look{
			Base:   []color.RGBA{pal.Black, pal.Night, pal.Slate},
			Accent: []color.RGBA{pal.Mahogany, pal.Bronze, pal.Tan, pal.Yellow},
		}},
		Frame:  []color.RGBA{pal.Mahogany, pal.Brown, pal.Bronze, pal.Tan, pal.Yellow, pal.White},
		Fog:    rgb(36, 26, 16),
		Light:  rgb(255, 245, 210),
		Height: 1.25,
		Prop:   PropCog, PropSize: 0.4, PropRooms: 0.5,
		Particles: ParticleSteam,
	},
	{
		Name:  "Sky Garden",
		Wall:  []color.RGBA{pal.Black, pal.Forest, pal.Green, pal.Lime},
		Floor: []color.RGBA{pal.Olive, pal.Forest, pal.Green, pal.Lime},
		Ceil:  []color.RGBA{pal.Navy, pal.Blue, pal.Sky, pal.Cyan},
		Moss:  []color.RGBA{pal.Pink, pal.Rose},
		Walls: Surface{Paint: WallHedge, Mods: []Modifier{ModRaggedTop}, Look: Look{
			Base:   []color.RGBA{pal.Black, pal.Forest, pal.Green, pal.Lime},
			Accent: []color.RGBA{pal.Pink, pal.Rose, pal.White, pal.Yellow},
		}},
		Variants: [2][]Modifier{{ModFlowers}, {ModFlowers, ModFlowers}},
		Floors: Surface{Paint: FloorGrass, Look: Look{
			Base:   []color.RGBA{pal.Olive, pal.Forest, pal.Green, pal.Lime},
			Accent: []color.RGBA{pal.Yellow, pal.White, pal.Pink},
		}},
		// The ceiling is only seen if the sky is turned off.
		Ceiling: Surface{Paint: CeilSlabs, Look: Look{Base: []color.RGBA{pal.Navy, pal.Blue, pal.Sky, pal.Cyan}}},
		Frame:   []color.RGBA{pal.Black, pal.Black, pal.Night, pal.Forest, pal.Night, pal.Forest},
		// Below 85% luminance everywhere, so the open sky never glares.
		Fog:   rgb(110, 160, 250),
		Light: rgb(255, 255, 240),
		Reach: 2.6,
		Sky:   []color.RGBA{pal.Blue, pal.Sky, pal.Sky, pal.Cyan},
		Prop:  PropFlowers, PropSize: 0.4, PropRooms: 0.8,
		Particles: ParticlePetals,
	},
	{
		Name:  "Sandstone Tomb",
		Wall:  []color.RGBA{pal.Mahogany, pal.Brown, pal.Bronze, pal.Tan, pal.Skin},
		Floor: []color.RGBA{pal.Mahogany, pal.Brown, pal.Bronze, pal.Tan},
		Ceil:  []color.RGBA{pal.Night, pal.Indigo, pal.Granite, pal.Stone},
		Moss:  []color.RGBA{pal.Teal, pal.Cyan},
		// Glyphs inlaid with glowing turquoise, and a cool dusk in the air,
		// so the tomb is not all orange and brown monsters stay visible.
		Walls: Surface{Paint: WallSandstone, Look: Look{
			Base:   []color.RGBA{pal.Mahogany, pal.Brown, pal.Bronze, pal.Tan, pal.Skin},
			Accent: []color.RGBA{pal.Teal, pal.Cyan},
		}},
		Variants: [2][]Modifier{{ModSandDrift}, {ModCrack}},
		Floors:   Surface{Paint: FloorSand},
		Ceiling: Surface{Paint: CeilSlabs, Look: Look{
			Base: []color.RGBA{pal.Night, pal.Indigo, pal.Slate, pal.Granite, pal.Stone},
		}},
		Frame: []color.RGBA{pal.Black, pal.Black, pal.Night, pal.Navy, pal.Night, pal.Teal},
		Fog:   rgb(40, 36, 70),
		Light: rgb(225, 228, 255),
		Prop:  PropUrn, PropSize: 0.4, PropRooms: 0.5,
		Particles: ParticleSand,
	},
	{
		Name:  "Whispering Library",
		Wall:  []color.RGBA{pal.Black, pal.Plum, pal.Mahogany, pal.Brown, pal.Tan, pal.Skin},
		Floor: []color.RGBA{pal.Black, pal.Plum, pal.Mahogany, pal.Brown, pal.Tan},
		Ceil:  []color.RGBA{pal.Night, pal.Plum, pal.Mahogany},
		Moss:  []color.RGBA{pal.Ash, pal.Steel},
		Walls: Surface{Paint: WallBooks, Look: Look{
			Base: []color.RGBA{pal.Black, pal.Plum, pal.Mahogany, pal.Brown},
			// Book spines in dark and light pairs.
			Accent: []color.RGBA{pal.Red, pal.Rose, pal.Navy, pal.Blue, pal.Forest, pal.Green, pal.Bronze, pal.Tan, pal.Purple, pal.Pink, pal.Teal, pal.Cyan},
		}},
		Variants: [2][]Modifier{{ModCobweb}, {ModCobweb, ModSoot}},
		Floors:   Surface{Paint: FloorPlanks},
		Ceiling: Surface{Paint: CeilCoffers, Look: Look{
			Base:   []color.RGBA{pal.Night, pal.Plum, pal.Mahogany},
			Accent: []color.RGBA{pal.Plum, pal.Mahogany, pal.Brown, pal.Tan},
		}},
		Frame:  []color.RGBA{pal.Black, pal.Mahogany, pal.Brown, pal.Tan, pal.Skin, pal.White},
		Fog:    rgb(30, 18, 30),
		Light:  rgb(255, 230, 180),
		Height: 1.6,
		Prop:   PropBooks, PropSize: 0.3, PropRooms: 0.5,
		Particles: ParticleLetters,
	},
}

// FloorOrder gives the world (an index into Themes) for floors 1, 2, 3 and
// so on. Neighbouring floors differ in hue and brightness, and the bright
// Sky Garden never follows the Ice Halls. Past the end the order repeats as
// another lap.
var FloorOrder = []int{0, 1, 2, 4, 3, 9, 7, 6, 5, 8}

// Lap returns how many times the floor order has repeated by depth: 0 for
// floors 1 to 10, 1 for floors 11 to 20, and so on.
func Lap(depth int) int { return max(depth-1, 0) / len(FloorOrder) }

// ThemeFor returns the world for a floor depth. Floor 1 is always The Crypt.
// From the second lap on, it returns the world's remix (see Remix), so
// floor 11 never looks exactly like floor 1.
func ThemeFor(depth int) *Theme {
	i := FloorOrder[max(depth-1, 0)%len(FloorOrder)]
	if lap := Lap(depth); lap > 0 {
		return &remixes[(lap-1)%len(remixes)][i]
	}
	return &Themes[i]
}

// remixes holds the second and third lap of every world; later laps repeat
// them.
var remixes = [2][]Theme{remixAll(1), remixAll(2)}

func remixAll(lap int) []Theme {
	ts := make([]Theme, len(Themes))
	for i := range Themes {
		ts[i] = *Themes[i].Remix(lap)
	}
	return ts
}

// Remix returns the world as it looks on a later lap of the floor order:
// one of its wall variants becomes the plain wall, props are denser and the
// fog shifts slightly towards the light. Lap 0 returns t itself. A remix
// never changes the ramps, so the old generators draw it as before.
func (t *Theme) Remix(lap int) *Theme {
	if lap <= 0 {
		return t
	}
	r := *t
	k := (lap - 1) % 2
	extra, other := t.Variants[k], t.Variants[1-k]
	r.Walls.Mods = append(append([]Modifier(nil), t.Walls.Mods...), extra...)
	r.Variants = [2][]Modifier{other, append(append([]Modifier(nil), other...), extra...)}
	r.Fog = mix(t.Fog, t.light(), 0.08)
	r.PropRooms = t.PropRooms * 1.6
	return &r
}

// mix blends a towards b by f.
func mix(a, b color.RGBA, f float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f + 0.5) }
	return color.RGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xff}
}

// light returns the light's tint, white when unset.
func (t *Theme) light() color.RGBA {
	if t.Light == (color.RGBA{}) {
		return pal.White
	}
	return t.Light
}

// LightTint returns the light's tint; white when unset.
func (t *Theme) LightTint() color.RGBA { return t.light() }

// LightReach returns how far the light reaches, relative to The Crypt.
func (t *Theme) LightReach() float64 {
	if t.Reach <= 0 {
		return 1
	}
	return t.Reach
}

// CeilHeight returns the ceiling height in wall heights.
func (t *Theme) CeilHeight() float64 {
	if t.Height <= 0 {
		return 1
	}
	return t.Height
}

// FrameRamp returns the ramp for door frames and stair steps.
func (t *Theme) FrameRamp() []color.RGBA {
	if t.Frame == nil {
		return t.Wall
	}
	return t.Frame
}

// withLook fills in an empty Look from fallback ramps.
func withLook(s Surface, base, accent []color.RGBA) Surface {
	if s.Look.Base == nil {
		s.Look.Base = base
	}
	if s.Look.Accent == nil {
		s.Look.Accent = accent
	}
	return s
}

// WallTex paints the world's wall: variant 0 is plain, 1 and 2 add the
// world's variant modifiers.
func (t *Theme) WallTex(seed uint64, v int) *Indexed {
	s := withLook(t.Walls, t.Wall, t.Moss)
	if v > 0 {
		s.Mods = append(append([]Modifier(nil), s.Mods...), t.Variants[v-1]...)
	}
	return s.Make(seed)
}

func (t *Theme) floors() Surface { return withLook(t.Floors, t.Floor, t.Moss) }

// FloorTex paints the world's floor, the first frame if it is animated.
func (t *Theme) FloorTex(seed uint64) *Indexed { return t.floors().Make(seed) }

// FloorFrames paints the Frames animation frames of an animated floor, such
// as water or lava. It returns nil for a still floor.
func (t *Theme) FloorFrames(seed uint64) []*Indexed {
	s := t.floors()
	if s.Anim == nil {
		return nil
	}
	fs := make([]*Indexed, Frames)
	for f := range fs {
		fs[f] = s.Frame(seed, f)
	}
	return fs
}

// CeilTex paints the world's ceiling.
func (t *Theme) CeilTex(seed uint64) *Indexed {
	return withLook(t.Ceiling, t.Ceil, beamWood).Make(seed)
}

// DoorTex paints a door, or a sealed door, set in the world's wall.
func (t *Theme) DoorTex(seed uint64, sealed bool) *Indexed {
	f := t.FrameRamp()
	return doorOn(t.WallTex(seed, 0), f[len(f)-2], seed, sealed)
}

// TorchTex paints a torch on the world's wall. frame animates the flame.
func (t *Theme) TorchTex(seed uint64, frame int) *Indexed {
	return torchOn(t.WallTex(seed, 0), frame)
}

// StairsTex paints the stairwell in the world's floor.
func (t *Theme) StairsTex(seed uint64) *Indexed {
	return stairsOn(t.FloorTex(seed), t.FrameRamp())
}

// The sky panorama's size. It wraps around horizontally, and is wide enough
// that the 3D view samples it about one texel per pixel, so clouds stay
// crisp instead of stretching into stripes.
const (
	SkyW = 1024
	SkyH = 64
)

// SkyTex paints the world's sky panorama, from the top of the sky down to
// the horizon, with soft clouds that thin out towards the horizon. It
// returns nil for a world with a ceiling.
func (t *Theme) SkyTex(seed uint64) *Indexed {
	if len(t.Sky) == 0 {
		return nil
	}
	m := NewIndexed(SkyW, SkyH)
	for y := range SkyH {
		f := float64(y) / (SkyH - 1)
		for x := range SkyW {
			fx, fy := float64(x), float64(y)*3 // clouds are wide and flat
			n := 0.65*wrapNoise(fx, fy, SkyW, 16, seed) + 0.35*wrapNoise(fx, fy, SkyW, 48, seed+1)
			n -= 0.3 * f * f
			c := Dither(t.Sky, f, x, y)
			switch {
			case n > 0.62:
				c = pal.Ice
			case n > 0.57 && bayer4[y&3][x&3] < 0.5:
				c = pal.Ice
			}
			m.Set(x, y, c)
		}
	}
	return m
}
