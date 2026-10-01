package raycast

import (
	"image/color"
	"math"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
)

// outlineLight is the least luminance a wall or floor needs for a
// sprite's black outline to count as standing out from it.
const outlineLight = 0.1

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
	r.spriteLight = t.SpriteMinLight()
	r.spriteLevel = int(r.spriteLight * (levels - 1))
	r.spriteShade = r.shade
	r.liftSprites(tint, fog, mainColour(t.WallTex(0, 0)), mainColour(t.FloorTex(0)))
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
// from the world's main wall and floor colours: on a light surface its
// black outline is enough, on a dark one the colour itself must stand out.
// Where a colour falls short, it takes the brighter level that stands out
// best; failing that, it fades towards black and shows as a silhouette,
// which reads well in bright fog, or as a last resort moves towards the
// light's colour, which reads well in dark fog. Levels below the world's
// sprite light are never used.
func (r *Renderer) liftSprites(tint, fog color.RGBA, surfaces ...uint8) {
	fogL := luminance(fog)
	var surfL [levels][]float64
	for lv := range levels {
		for _, s := range surfaces {
			surfL[lv] = append(surfL[lv], luminance(pal.All[r.shade[lv][s]]))
		}
	}
	stands := func(c uint8, lv int) float64 {
		l := luminance(pal.All[c])
		worst := contrast(l, fogL)
		for _, sl := range surfL[lv] {
			c := contrast(l, sl)
			if sl >= outlineLight {
				c = max(c, contrast(0, sl))
			}
			worst = min(worst, c)
		}
		return worst
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
		for lv := r.spriteLevel; lv < levels; lv++ {
			r.spriteShade[lv][i] = r.liftColour(c, uint8(i), lv, tint, fog, func(c uint8) float64 { return stands(c, lv) })
		}
		for lv := range r.spriteLevel {
			r.spriteShade[lv][i] = r.spriteShade[r.spriteLevel][i]
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
	return 0.2126*linear[c.R] + 0.7152*linear[c.G] + 0.0722*linear[c.B]
}

// linear is each sRGB channel value in linear light, ((v/255+0.055)/1.055)^2.4,
// or v/255/12.92 near black. It is written out rather than computed, so
// math.Pow's platform-specific rounding never moves a sprite colour.
var linear = [256]float64{
	0.0, 0.0003035269835488375, 0.000607053967097675, 0.0009105809506465125,
	0.00121410793419535, 0.0015176349177441874, 0.001821161901293025, 0.0021246888848418626,
	0.0024282158683907, 0.0027317428519395373, 0.003035269835488375, 0.003346535763899161,
	0.003676507324047436, 0.004024717018496307, 0.004391442037410293, 0.004776953480693729,
	0.005181516702338386, 0.005605391624202723, 0.006048833022857054, 0.006512090792594475,
	0.006995410187265387, 0.007499032043226175, 0.008023192985384994, 0.008568125618069307,
	0.009134058702220787, 0.00972121732023785, 0.010329823029626936, 0.010960094006488246,
	0.011612245179743885, 0.012286488356915872, 0.012983032342173012, 0.013702083047289686,
	0.014443843596092545, 0.01520851442291271, 0.01599629336550963, 0.016807375752887384,
	0.017641954488384078, 0.018500220128379697, 0.019382360956935723, 0.0202885630566524,
	0.021219010376003555, 0.02217388479338738, 0.02315336617811041, 0.024157632448504756,
	0.02518685962736163, 0.026241221894849898, 0.027320891639074894, 0.028426039504420793,
	0.0295568344378088, 0.030713443732993635, 0.03189603307301153, 0.033104766570885055,
	0.03433980680868217, 0.03560131487502034, 0.03688945040110004, 0.0382043715953465,
	0.03954623527673284, 0.04091519690685319, 0.042311410620809675, 0.043735029256973465,
	0.04518620438567554, 0.046665086336880095, 0.04817182422688942, 0.04970656598412723,
	0.05126945837404324, 0.052860647023180246, 0.05448027644244237, 0.05612849004960009,
	0.05780543019106723, 0.0595112381629812, 0.06124605423161761, 0.06301001765316767,
	0.06480326669290577, 0.06662593864377289, 0.06847816984440017, 0.07036009569659588,
	0.07227185068231748, 0.07421356838014963, 0.07618538148130785, 0.07818742180518633,
	0.08021982031446832, 0.0822827071298148, 0.08437621154414882, 0.08650046203654976,
	0.08865558628577294, 0.09084171118340768, 0.09305896284668745, 0.0953074666309647,
	0.09758734714186246, 0.09989872824711389, 0.10224173308810132, 0.10461648409110419,
	0.10702310297826761, 0.10946171077829933, 0.1119324278369056, 0.11443537382697373,
	0.11697066775851084, 0.11953842798834562, 0.12213877222960187, 0.12477181756095049,
	0.12743768043564743, 0.1301364766903643, 0.13286832155381798, 0.13563332965520566,
	0.13843161503245183, 0.14126329114027164, 0.14412847085805777, 0.14702726649759498,
	0.14995978981060856, 0.15292615199615017, 0.1559264637078274, 0.1589608350608804,
	0.162029375639111, 0.1651321945016676, 0.16826940018969075, 0.1714411007328226,
	0.17464740365558504, 0.17788841598362912, 0.18116424424986022, 0.184474994500441,
	0.18782077230067787, 0.19120168274079138, 0.1946178304415758, 0.19806931955994886,
	0.20155625379439707, 0.20507873639031693, 0.20863687014525575, 0.21223075741405523,
	0.21586050011389926, 0.2195261997292692, 0.2232279573168085, 0.22696587351009836,
	0.23074004852434915, 0.23455058216100522, 0.238397573812271, 0.24228112246555486,
	0.24620132670783548, 0.25015828472995344, 0.25415209433082675, 0.2581828529215958,
	0.26225065752969623, 0.26635560480286247, 0.2704977910130658, 0.27467731206038465,
	0.2788942634768104, 0.2831487404299921, 0.2874408377269175, 0.29177064981753587,
	0.2961382707983211, 0.3005437944157765, 0.3049873140698863, 0.30946892281750854,
	0.31398871337571754, 0.31854677812509186, 0.32314320911295075, 0.3277780980565422,
	0.33245153634617935, 0.33716361504833037, 0.3419144249086609, 0.3467040563550296,
	0.35153259950043936, 0.3564001441459435, 0.3613067797835095, 0.3662525955988395,
	0.3712376804741491, 0.3762621229909065, 0.38132601143253014, 0.386429433787049,
	0.39157247774972326, 0.39675523072562685, 0.4019777798321958, 0.4072402119017367,
	0.41254261348390375, 0.4178850708481375, 0.4232676699860717, 0.4286904966139066,
	0.43415363617474895, 0.4396571738409188, 0.44520119451622786, 0.45078578283822346,
	0.45641102318040466, 0.4620769996544071, 0.467783796112159, 0.47353149614800955,
	0.4793201831008268, 0.4851499400560704, 0.4910208498478356, 0.4969329950608704,
	0.5028864580325687, 0.5088813208549338, 0.5149176653765214, 0.5209955732043543,
	0.5271151257058131, 0.5332764040105052, 0.5394794890121072, 0.5457244613701866,
	0.5520114015120001, 0.5583403896342679, 0.5647115057049292, 0.5711248294648731,
	0.5775804404296506, 0.5840784178911641, 0.5906188409193369, 0.5972017883637634,
	0.6038273388553378, 0.6104955708078648, 0.6172065624196511, 0.6239603916750761,
	0.6307571363461468, 0.6375968739940326, 0.6444796819705821, 0.6514056374198242,
	0.6583748172794485, 0.665387298282272, 0.6724431569576875, 0.6795424696330938,
	0.6866853124353135, 0.6938717612919899, 0.7011018919329731, 0.7083757798916868,
	0.7156935005064807, 0.7230551289219693, 0.7304607400903537, 0.7379104087727308,
	0.7454042095403874, 0.7529422167760779, 0.7605245046752924, 0.768151147247507,
	0.7758222183174236, 0.7835377915261935, 0.7912979403326302, 0.799102738014409,
	0.8069522576692516, 0.8148465722161012, 0.8227857543962835, 0.8307698767746546,
	0.83879901174074, 0.846873231509858, 0.8549926081242338, 0.8631572134541023,
	0.8713671191987972, 0.8796223968878317, 0.8879231178819663, 0.8962693533742664,
	0.9046611743911496, 0.9130986517934192, 0.9215818562772946, 0.9301108583754237,
	0.938685728457888, 0.9473065367331999, 0.9559733532492861, 0.9646862478944651,
	0.9734452903984125, 0.9822505503331171, 0.9911020971138298, 1.0,
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
// with the same camera. They are hidden behind walls and sprites, and
// never touch anything outside the view. With Calm set it draws nothing.
func (r *Renderer) DrawParticles(cam Camera, tick uint64) {
	if r.Calm || r.level == nil || r.particles == proc.NoParticles || int(r.particles) >= len(particleStyles) {
		return
	}
	st := &particleStyles[r.particles]
	cols := particleIdx[r.particles]
	k := float64(r.W) / (2 * hypot(cam.PlaneX, cam.PlaneY))
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
		if sx < 0 || sx >= r.W || sy < 0 || sy >= r.H || ty >= r.zbuf[sx] || r.behindSprite(sx, sy, ty) {
			continue
		}
		c := cols[int(unit(h4)*float64(len(cols)))]
		b := r.brightness(ty, wx, wy)
		r.put(sx, sy, c, b, st.glow)
		if st.big && ty < 1.5 && sx+1 < r.W && ty < r.zbuf[sx+1] && !r.behindSprite(sx+1, sy, ty) {
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
// walls, away from doorways, notes, chests, features, monsters, the start and the
// stairs. It draws its random numbers from its own generator, seeded from
// the level's seed and depth, and only reads the level: the look of a floor
// never changes the floor.
func Decor(l *dungeon.Level, t *proc.Theme) []Sprite {
	if t.Prop == nil || t.PropRooms <= 0 {
		return nil
	}
	rng := proc.NewRand(mix64(l.Seed^decorSalt) ^ uint64(l.Depth)*0xd6e8feb86659fd93)
	img := t.Prop(rng.Uint64())
	// The spots are drawn from static data only (the rooms, walls, the
	// start, the stairs and the notes), so they are the same before and after
	// a save is restored. Props that land on something live are dropped
	// afterwards, without drawing again.
	busy := map[dungeon.Point]bool{l.Start: true, l.Exit: true}
	for p := range l.Notes {
		busy[p] = true
	}
	live := map[dungeon.Point]bool{}
	for p := range l.Chests {
		live[p] = true
	}
	for p := range l.Features {
		live[p] = true
	}
	for _, m := range l.Monsters {
		live[m.At] = true
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
			if live[s.at] {
				continue
			}
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

// hypot is the length of (x, y). Unlike math.Hypot, which has its own
// assembly on amd64, it rounds the same on every platform, so views are
// the same wherever they are drawn.
func hypot(x, y float64) float64 { return math.Sqrt(x*x + y*y) }
