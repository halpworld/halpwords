package raycast

import (
	"image/color"
	"math"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
)

// spriteMinLight is the least light a sprite gets, however far away, so
// monsters never sink fully into the fog.
const spriteMinLight = 0.35

// spriteMinLevel is the lowest light level dithering gives a sprite at
// spriteMinLight: 0.35 × (levels-1), rounded down.
const spriteMinLevel = 2

// spriteContrast is the luminance contrast against the fog that SetWorld
// lifts sprite colours to where it can.
const spriteContrast = 1.6

// SetWorld sets the air of a world, once per floor: darkness and distance
// fade towards its fog colour, light takes its tint, and the light reach,
// ceiling height and particles follow the world. Frames cost the same in
// every world.
func (r *Renderer) SetWorld(t *proc.Theme) {
	r.reach = t.LightReach()
	r.ceilH = t.CeilHeight()
	r.particles = t.Particles
	fog, tint := t.Fog, t.LightTint()
	for i := range r.shade[0] {
		for lv := range levels {
			r.shade[lv][i] = proc.Transparent
		}
	}
	for i, c := range pal.All {
		for lv := range levels {
			r.shade[lv][i] = pal.Index(shadeColour(c, tint, fog, float64(lv)/(levels-1)))
		}
	}
	r.spriteShade = r.shade
	r.liftSprites(tint, fog, mainColour(t.WallTex(0, 0)))
}

// shadeColour is colour c under light of the given tint at strength k in
// [0,1], fading to fog as k falls. A black fog and a warm tint give The
// Crypt's torchlight, where blue fades first.
func shadeColour(c, tint, fog color.RGBA, k float64) color.RGBA {
	mix := func(c, l, f uint8) uint8 {
		return uint8(min(255, float64(c)*float64(l)/255*k+float64(f)*(1-k)))
	}
	return color.RGBA{mix(c.R, tint.R, fog.R), mix(c.G, tint.G, fog.G), mix(c.B, tint.B, fog.B), 0xff}
}

// liftSprites keeps each sprite colour standing out from the fog, so a
// dark monster in black fog or a blue one in blue fog stays visible, and
// from the world's main wall colour, either itself or by its black
// outline. Where a colour falls short, it takes the brighter level that
// stands out best; failing that, it fades towards black and shows as a
// silhouette, which reads well in bright fog, or as a last resort moves
// towards the light's colour, which reads well in dark fog. Levels below
// the sprite minimum light are never used.
func (r *Renderer) liftSprites(tint, fog color.RGBA, wall uint8) {
	fogL := luminance(fog)
	var wallL [levels]float64
	for lv := range levels {
		wallL[lv] = luminance(pal.All[r.shade[lv][wall]])
	}
	stands := func(c uint8, lv int) float64 {
		l := luminance(pal.All[c])
		return min(contrast(l, fogL), max(contrast(l, wallL[lv]), contrast(0, wallL[lv])))
	}
	outline := pal.Index(pal.Black)
	for i, c := range pal.All {
		if uint8(i) == outline {
			// Outlines stay black, so sprites keep an edge against bright
			// walls and fog.
			for lv := range levels {
				r.spriteShade[lv][i] = outline
			}
			continue
		}
		for lv := spriteMinLevel; lv < levels; lv++ {
			r.spriteShade[lv][i] = r.liftColour(c, uint8(i), lv, tint, fog, func(c uint8) float64 { return stands(c, lv) })
		}
		for lv := range spriteMinLevel {
			r.spriteShade[lv][i] = r.spriteShade[spriteMinLevel][i]
		}
	}
}

// liftColour picks the sprite shade of palette colour c (index i) at light
// level lv, the first of liftSprites' choices to stand out enough.
func (r *Renderer) liftColour(c color.RGBA, i uint8, lv int, tint, fog color.RGBA, stands func(uint8) float64) uint8 {
	best := r.shade[lv][i]
	better := func(cand uint8) {
		if stands(cand) > stands(best) {
			best = cand
		}
	}
	if stands(best) >= spriteContrast {
		return best
	}
	for up := lv + 1; up < levels; up++ {
		better(r.shade[up][i])
	}
	k := float64(lv) / (levels - 1)
	if stands(best) < spriteContrast {
		better(pal.Index(shadeColour(c, tint, pal.Black, k)))
	}
	// Still lost: catch the light, like a rim-lit edge.
	for _, a := range rimSteps {
		if stands(best) >= spriteContrast {
			break
		}
		better(pal.Index(lerpColour(shadeColour(c, tint, fog, k), tint, a)))
	}
	return best
}

// mainColour is the colour a texture uses most.
func mainColour(m *proc.Indexed) uint8 {
	var n [256]int
	for _, p := range m.Pix {
		n[p]++
	}
	n[proc.Transparent] = 0
	best := 0
	for i := range n {
		if n[i] > n[best] {
			best = i
		}
	}
	return uint8(best)
}

// rimSteps are how far liftSprites may move a lost colour towards the light.
var rimSteps = [...]float64{0.15, 0.3, 0.45, 0.6}

// lerpColour moves colour a towards b by t in [0,1].
func lerpColour(a, b color.RGBA, t float64) color.RGBA {
	l := func(a, b uint8) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 0xff}
}

// luminance is the relative luminance of a colour (WCAG).
func luminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// contrast is the luminance contrast ratio of two luminances, at least 1.
func contrast(a, b float64) float64 {
	return (max(a, b) + 0.05) / (min(a, b) + 0.05)
}

// Particles.

// particleSalt and decorSalt keep the look's random numbers apart from the
// dungeon generator's: nothing visual may change the floor itself.
const (
	particleSalt = 0x9a27_1c1e_5eed_0002
	decorSalt    = 0xdec0_7a1e_5eed_0001
)

// The particles float in a box this many cells wide around the hero, and
// are never drawn nearer than particleNear.
const (
	particleBox  = 6.0
	particleNear = 0.5
)

// particleStyle is how one kind of particle moves and looks.
type particleStyle struct {
	n       int          // how many
	fall    float64      // height change per second; negative falls
	drift   float64      // sideways drift in cells per second
	wobble  float64      // how far they sway, in cells
	cols    []color.RGBA // picked per particle
	glow    bool         // ignores darkness
	blink   bool         // on for a third of each slow cycle
	big     bool         // two pixels wide up close
	floorZ  float64      // lowest height
	ceilZ   float64      // highest height
	flutter float64      // extra sway, for petals and letters
}

// particleStyles has a style for each proc.Particles kind. Snow and petals
// fall slowly enough to stay under 20 pixels a second on screen.
var particleStyles = [...]particleStyle{
	proc.ParticleDust:      {n: 18, fall: -0.02, drift: 0.05, wobble: 0.15, cols: []color.RGBA{pal.Ash, pal.Stone}, ceilZ: 1},
	proc.ParticleFireflies: {n: 14, wobble: 0.35, cols: []color.RGBA{pal.Lime, pal.Yellow}, glow: true, blink: true, floorZ: 0.15, ceilZ: 0.8},
	proc.ParticleDrips:     {n: 10, fall: -0.9, cols: []color.RGBA{pal.Cyan, pal.Sky}, ceilZ: 1},
	proc.ParticleSnow:      {n: 24, fall: -0.06, drift: 0.04, wobble: 0.2, cols: []color.RGBA{pal.White, pal.Ice}, big: true, ceilZ: 1},
	proc.ParticleEmbers:    {n: 16, fall: 0.15, wobble: 0.2, cols: []color.RGBA{pal.Orange, pal.Yellow, pal.Red}, glow: true, ceilZ: 1},
	proc.ParticleSparkles:  {n: 12, wobble: 0.1, cols: []color.RGBA{pal.White, pal.Pink, pal.Skin}, glow: true, blink: true, floorZ: 0.1, ceilZ: 0.9},
	proc.ParticleSteam:     {n: 16, fall: 0.08, drift: 0.03, wobble: 0.3, cols: []color.RGBA{pal.Ash, pal.Steel}, big: true, ceilZ: 1},
	proc.ParticlePetals:    {n: 16, fall: -0.05, drift: 0.12, wobble: 0.25, flutter: 0.15, cols: []color.RGBA{pal.Pink, pal.Rose, pal.White}, big: true, ceilZ: 1},
	proc.ParticleSand:      {n: 20, fall: -0.01, drift: 0.35, wobble: 0.1, cols: []color.RGBA{pal.Tan, pal.Skin}, floorZ: 0, ceilZ: 0.5},
	proc.ParticleLetters:   {n: 10, fall: 0.03, wobble: 0.3, flutter: 0.2, cols: []color.RGBA{pal.Skin, pal.Tan, pal.Ice}, big: true, floorZ: 0.2, ceilZ: 1},
}

// particleIdx holds each style's colours as palette indices.
var particleIdx = func() (idx [len(particleStyles)][]uint8) {
	for k, st := range particleStyles {
		for _, c := range st.cols {
			idx[k] = append(idx[k], pal.Index(c))
		}
	}
	return idx
}()

// mix64 scrambles a number (splitmix64), for particle positions that are a
// pure function of the level, the particle and time.
func mix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ x>>30) * 0xbf58476d1ce4e5b9
	x = (x ^ x>>27) * 0x94d049bb133111eb
	return x ^ x>>31
}

// unit turns a scrambled number into a float in [0,1).
func unit(x uint64) float64 { return float64(x>>11) / (1 << 53) }

// DrawParticles draws the world's particles into the view, after Render
// with the same camera. They are hidden behind walls and never touch
// anything outside the view. With Calm set it draws nothing.
func (r *Renderer) DrawParticles(cam Camera, tick uint64) {
	if r.Calm || r.level == nil || r.particles == proc.NoParticles || int(r.particles) >= len(particleStyles) {
		return
	}
	st := &particleStyles[r.particles]
	cols := particleIdx[r.particles]
	k := float64(r.W) / (2 * math.Hypot(cam.PlaneX, cam.PlaneY))
	horizon := float64(r.H) / 2
	inv := 1 / (cam.PlaneX*cam.DirY - cam.DirX*cam.PlaneY)
	secs := float64(tick) / 60
	for i := range st.n {
		h := mix64(r.seed + uint64(i))
		h1, h2, h3, h4 := mix64(h), mix64(h+1), mix64(h+2), mix64(h+3)
		phase := unit(h4) * 2 * math.Pi
		sway := st.wobble * math.Sin(secs*0.7+phase)
		// A position in the world that drifts with time, wrapped into the
		// box around the hero so the air is always full.
		wx := wrapBox(unit(h1)*particleBox+st.drift*secs+sway, cam.X)
		wy := wrapBox(unit(h2)*particleBox+st.wobble*math.Cos(secs*0.5+phase), cam.Y)
		span := st.ceilZ - st.floorZ
		z := st.floorZ + span*frac(unit(h3)+st.fall*secs/span)
		if st.flutter > 0 {
			z += st.flutter * 0.1 * math.Sin(secs*1.3+phase)
		}
		if st.blink && frac(secs/3+unit(h4)) > 1.0/3 {
			continue
		}
		dx, dy := wx-cam.X, wy-cam.Y
		tx := inv * (cam.DirY*dx - cam.DirX*dy)
		ty := inv * (-cam.PlaneY*dx + cam.PlaneX*dy)
		if ty < particleNear {
			continue
		}
		sx := int(float64(r.W) / 2 * (1 + tx/ty))
		sy := int(horizon + (0.5-z)*k/ty)
		if sx < 0 || sx >= r.W || sy < 0 || sy >= r.H || ty >= r.zbuf[sx] {
			continue
		}
		c := cols[int(unit(h4)*float64(len(cols)))]
		b := r.brightness(ty, wx, wy)
		r.put(sx, sy, c, b, st.glow)
		if st.big && ty < 1.5 && sx+1 < r.W && ty < r.zbuf[sx+1] {
			r.put(sx+1, sy, c, b, st.glow)
		}
	}
}

// wrapBox wraps v into the particle box centred on c.
func wrapBox(v, c float64) float64 {
	lo := c - particleBox/2
	return lo + particleBox*frac((v-lo)/particleBox)
}

func frac(v float64) float64 { return v - math.Floor(v) }

// Decor.

// propNudge is how far a prop stands from its cell's centre towards the
// wall behind it.
const propNudge = 0.28

// Decor places the world's props on a few empty floor cells against room
// walls, away from doorways, chests, features, monsters, the start and the
// stairs. It draws its random numbers from its own generator, seeded from
// the level's seed and depth, and only reads the level: the look of a floor
// never changes the floor.
func Decor(l *dungeon.Level, t *proc.Theme) []Sprite {
	if t.Prop == nil || t.PropRooms <= 0 {
		return nil
	}
	rng := proc.NewRand(mix64(l.Seed^decorSalt) ^ uint64(l.Depth)*0xd6e8feb86659fd93)
	img := t.Prop(rng.Uint64())
	busy := map[dungeon.Point]bool{l.Start: true, l.Exit: true}
	for p := range l.Chests {
		busy[p] = true
	}
	for p := range l.Features {
		busy[p] = true
	}
	for _, m := range l.Monsters {
		busy[m.At] = true
	}
	var out []Sprite
	for _, room := range l.Rooms {
		spots := propSpots(l, room, busy)
		n := int(t.PropRooms + rng.Float64()) // PropRooms on average
		for range min(n, 2) {
			if len(spots) == 0 {
				break
			}
			j := rng.IntN(len(spots))
			s := spots[j]
			spots[j] = spots[len(spots)-1]
			spots = spots[:len(spots)-1]
			out = append(out, Sprite{
				X:    float64(s.at.X) + 0.5 + propNudge*float64(s.wall.X),
				Y:    float64(s.at.Y) + 0.5 + propNudge*float64(s.wall.Y),
				Img:  img,
				Size: t.PropSize,
			})
		}
	}
	return out
}

// propSpot is a floor cell a prop can stand on, and the way to its wall.
type propSpot struct {
	at, wall dungeon.Point
}

// propSpots lists a room's free floor cells that touch a wall and no
// doorway or corridor.
func propSpots(l *dungeon.Level, room dungeon.Room, busy map[dungeon.Point]bool) []propSpot {
	var spots []propSpot
	for y := room.Y; y < room.Y+room.H; y++ {
		for x := room.X; x < room.X+room.W; x++ {
			p := dungeon.Point{X: x, Y: y}
			if l.At(p) != dungeon.Floor || busy[p] {
				continue
			}
			var wall dungeon.Point
			walls, open := 0, false
			for d := range dungeon.Dir(4) {
				q := p.Step(d)
				switch {
				case l.At(q) == dungeon.Wall:
					walls++
					wall = d.Delta()
				case !room.Contains(q):
					open = true // a doorway or corridor
				}
			}
			if walls > 0 && !open {
				spots = append(spots, propSpot{p, wall})
			}
		}
	}
	return spots
}
