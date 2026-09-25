package scene

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/proc"
	"github.com/halpworld/halpwords/internal/puzzle"
	"github.com/halpworld/halpwords/internal/raycast"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/save"
)

// Layout of the crawl screen, in screen pixels unless noted.
const (
	viewW, viewH = 192, 120 // the 3D view, in art pixels
	viewX, viewY = 8, 8
	sideX, sideW = 404, game.ScreenW - 404 - 4 // right-hand column
	panelY       = 256                         // bottom panel
)

// mode is what the crawl screen is doing.
type mode int

const (
	modeExplore mode = iota
	modeBattle
	modePuzzle
	modeMap
	modePause    // the pause menu
	modeQuit     // asking whether to quit without saving
	modeShrine   // asking whether to pray at a Save Shrine
	modeCampfire // resting at a campfire
	modeShop     // buying and selling with the merchant
	modeItems    // the hero's items and gear
	modeDead
)

// action is one grid move.
type action int

const (
	actNone action = iota
	actForward
	actBack
	actTurnLeft
	actTurnRight
	actStrafeLeft
	actStrafeRight
)

var actionKeys = []struct {
	act  action
	keys []ebiten.Key
}{
	{actForward, []ebiten.Key{ebiten.KeyArrowUp, ebiten.KeyW}},
	{actBack, []ebiten.Key{ebiten.KeyArrowDown, ebiten.KeyS}},
	{actTurnLeft, []ebiten.Key{ebiten.KeyArrowLeft, ebiten.KeyA}},
	{actTurnRight, []ebiten.Key{ebiten.KeyArrowRight, ebiten.KeyD}},
	{actStrafeLeft, []ebiten.Key{ebiten.KeyQ}},
	{actStrafeRight, []ebiten.Key{ebiten.KeyE}},
}

// holdDelay is how long, in ticks, a movement key must be held before it
// repeats. It is longer than a step, so a tap moves exactly one cell.
const holdDelay = 14

// readAction returns the move asked for this tick. While busy, only fresh
// key presses count, so they can be queued.
func readAction(busy bool) action {
	for _, ak := range actionKeys {
		for _, k := range ak.keys {
			d := inpututil.KeyPressDuration(k)
			if d == 1 || (!busy && d >= holdDelay) {
				return ak.act
			}
		}
	}
	return actNone
}

// movementHeld reports whether any movement key is down. Typing is ignored
// until they are released, so a held W does not type "wwww" into a battle.
func movementHeld() bool {
	for _, ak := range actionKeys {
		for _, k := range ak.keys {
			if ebiten.IsKeyPressed(k) {
				return true
			}
		}
	}
	return false
}

// anim moves the camera smoothly between cells and facings.
type anim struct {
	t, dur     int
	x0, y0, a0 float64
	x1, y1, a1 float64
	bump       bool // walking into something: lean towards it and back
}

func (a *anim) active() bool { return a.t < a.dur }

// frac returns the progress, eased in and out.
func (a *anim) frac() float64 {
	if a.dur == 0 {
		return 1
	}
	f := float64(a.t) / float64(a.dur)
	return f * f * (3 - 2*f)
}

const (
	stepTicks = 9
	turnTicks = 8
)

// floater is a number or word that rises over the view, like damage.
type floater struct {
	text string
	col  color.RGBA
	t    int
}

// Crawl is the first-person dungeon. Exploring, battles and puzzles all
// happen here, so the dungeon stays on screen.
type Crawl struct {
	run   *run
	level *dungeon.Level
	theme *proc.Theme
	tex   *raycast.Textures
	view  *raycast.Renderer
	img   *ebiten.Image
	chest [2]*proc.Indexed
	looks map[*dungeon.Monster][2]*proc.Indexed
	props props // shrine, campfire and merchant sprites

	pos    dungeon.Point
	facing dungeon.Dir
	angle  float64 // the facing as an angle; it can wind past 2π
	anim   anim
	queued action
	prev   map[*dungeon.Monster]dungeon.Point // monster cells before their last step
	zoom   float64                            // how far the camera leans in for a battle

	mode    mode
	battle  *battle
	puzzle  *lockPuzzle
	pending *dungeon.Monster // a monster to fight once the hero faces it
	muted   bool             // ignore typing until movement keys are released

	resume   mode   // where the pause menu goes back to
	pausedAt uint64 // tick the game was paused
	menuSel  int    // the highlighted pause menu item
	lastSave []byte // the game as last saved or loaded
	unsaved  bool   // something has happened since lastSave

	menu    *menu            // the shop or items screen
	feature *dungeon.Feature // the shrine, campfire or merchant in use
	weakest []logLine        // the words shown at a campfire
	insight []string         // memory tips shown at a campfire

	lore  []string // notes from the Dungeon Director still to find
	steps int      // steps taken on this floor

	shake, hurt int // ticks of screen shake and red flash left
	banner      string
	sub         string // smaller line under the banner
	bannerT     int
	floats      []floater
}

// newCrawl starts the floor r.depth.
func newCrawl(r *run) *Crawl {
	c := crawlOn(r, r.floor(r.depth))
	c.showBanner(fmt.Sprintf("Floor %d", r.depth), c.floorName())
	c.arrive()
	r.ai.startFloor(r)
	return c
}

// crawlOn puts the hero at the start of floor l.
func crawlOn(r *run, l *dungeon.Level) *Crawl {
	th := r.themeFor()
	dress(l, r.script())
	return &Crawl{
		run:    r,
		level:  l,
		theme:  th,
		tex:    raycast.NewTextures(th, l.Seed),
		view:   raycast.New(viewW, viewH),
		img:    ebiten.NewImage(viewW, viewH),
		chest:  [2]*proc.Indexed{proc.ChestSprite(false), proc.ChestSprite(true)},
		looks:  map[*dungeon.Monster][2]*proc.Indexed{},
		props:  newProps(),
		pos:    l.Start,
		facing: l.StartDir,
		angle:  raycast.Angle(l.StartDir),
	}
}

func (c *Crawl) showBanner(text, sub string) {
	c.banner, c.sub, c.bannerT = text, sub, 150
}

func (c *Crawl) play(id audio.ID) { c.run.sound.Play(id) }

func (c *Crawl) float(text string, col color.RGBA) {
	c.floats = append(c.floats, floater{text: text, col: col})
}

func secs(ticks uint64) float64 { return float64(ticks) / float64(ebiten.TPS()) }

func center(p dungeon.Point) (float64, float64) { return float64(p.X) + 0.5, float64(p.Y) + 0.5 }

// dirTo returns the direction from a to a neighbouring cell b.
func dirTo(a, b dungeon.Point) (dungeon.Dir, bool) {
	for d := dungeon.North; d <= dungeon.West; d++ {
		if a.Step(d) == b {
			return d, true
		}
	}
	return 0, false
}

// Update implements game.Scene.
func (c *Crawl) Update(ctx *game.Context) error {
	c.run.ai.poll(c)
	switch {
	case c.mode == modePause:
		c.updatePause(ctx)
		return nil
	case c.mode == modeQuit:
		c.updateQuit(ctx)
		return nil
	case c.mode == modeShrine:
		c.updateShrine(ctx)
		return nil
	case c.mode == modeCampfire:
		switch {
		case input.Pressed(ebiten.KeyG):
			c.play(audio.Select)
			ctx.Push(NewGrimoire(ctx, c.run.lang))
		case input.Confirm() || input.Back() || input.Pressed(ebiten.KeySpace):
			c.mode = modeExplore
		}
		return nil
	case c.mode == modeShop || c.mode == modeItems:
		c.updateMenu(ctx)
		return nil
	case c.mode == modeBattle && !ebiten.IsFocused():
		// Don't let the battle clock run while the player is in another
		// window.
		c.pause(ctx)
		return nil
	}
	if c.shake > 0 {
		c.shake--
	}
	if c.hurt > 0 {
		c.hurt--
	}
	if c.bannerT > 0 {
		c.bannerT--
	}
	for i := 0; i < len(c.floats); i++ {
		if c.floats[i].t++; c.floats[i].t > 50 {
			c.floats = append(c.floats[:i], c.floats[i+1:]...)
			i--
		}
	}
	lean := 0.0
	if c.mode == modeBattle {
		lean = 0.3
	}
	c.zoom += (lean - c.zoom) * 0.2
	if c.muted && !movementHeld() {
		c.muted = false
		if b := c.battle; b != nil && (b.phase == phaseAttack || b.phase == phaseDefend) {
			b.start = ctx.Tick // the clock starts once the hero can type
		}
	}

	if c.anim.active() {
		c.anim.t++
		if !c.anim.active() {
			c.arrived()
		}
		if a := readAction(true); a != actNone && c.mode == modeExplore {
			c.queued = a
		}
		return nil
	}

	switch c.mode {
	case modeExplore:
		c.explore(ctx)
	case modeBattle:
		c.updateBattle(ctx)
	case modePuzzle:
		c.updatePuzzle(ctx)
	case modeMap:
		if input.Back() || input.Confirm() || input.Pressed(ebiten.KeyM, ebiten.KeySpace) {
			c.mode = modeExplore
		}
	case modeDead:
		switch {
		case c.run.hardcore() && (input.Confirm() || input.Back()):
			ctx.Replace(newGameOver(ctx, c.run, false))
		case input.Confirm():
			c.wake(ctx)
		case input.Back():
			c.woken(ctx) // the fall still counts in the save
			ctx.Replace(NewTitle(ctx))
		}
	}
	return nil
}

func (c *Crawl) explore(ctx *game.Context) {
	if m := c.pending; m != nil {
		c.pending = nil
		d, ok := dirTo(c.pos, m.At)
		if !ok || m.HP <= 0 {
			return
		}
		if d != c.facing {
			c.turnTo(d)
			c.pending = m // fight once turned
			return
		}
		c.startBattle(ctx, m, true)
		return
	}
	switch {
	case input.Back():
		c.pause(ctx)
		return
	case input.Pressed(ebiten.KeyM):
		c.mode = modeMap
		return
	case input.Pressed(ebiten.KeyP):
		c.drinkPotion()
		return
	case input.Pressed(ebiten.KeyI):
		c.openItems(ctx)
		return
	case input.Pressed(ebiten.KeySpace) || input.Confirm():
		c.interact(ctx)
		return
	}
	a := c.queued
	c.queued = actNone
	if a == actNone {
		a = readAction(false)
	}
	switch a {
	case actNone:
	case actTurnLeft:
		c.turnTo(c.facing.Left())
	case actTurnRight:
		c.turnTo(c.facing.Right())
	default:
		d := c.facing
		switch a {
		case actBack:
			d = d.Back()
		case actStrafeLeft:
			d = d.Left()
		case actStrafeRight:
			d = d.Right()
		}
		c.step(ctx, d, a == actForward)
	}
}

// animate starts a camera move from the current cell and angle.
func (c *Crawl) animate(to dungeon.Point, angle float64, dur int, bump bool) {
	x0, y0 := center(c.pos)
	x1, y1 := center(to)
	c.anim = anim{dur: dur, x0: x0, y0: y0, a0: c.angle, x1: x1, y1: y1, a1: angle, bump: bump}
}

// turnTo faces d, animating the turn.
func (c *Crawl) turnTo(d dungeon.Dir) {
	quarter := (int(d) - int(c.facing) + 4) % 4
	if quarter == 0 {
		return
	}
	if quarter == 3 {
		quarter = -1
	}
	a := c.angle + float64(quarter)*math.Pi/2
	dur := turnTicks
	if quarter == 2 {
		dur = turnTicks * 3 / 2
	}
	c.animate(c.pos, a, dur, false)
	c.facing, c.angle = d, a
}

func (c *Crawl) bump(d dungeon.Dir) {
	c.play(audio.Bump)
	c.animate(c.pos.Step(d), c.angle, turnTicks, true)
}

// wait passes a turn in place, letting the monsters move.
func (c *Crawl) wait() {
	c.monstersTurn(c.pos)
	c.animate(c.pos, c.angle, stepTicks, false)
}

// step tries to move one cell in direction d. Walking forward into things
// uses them: doors open, monsters are attacked, chests and sealed doors
// start puzzles.
func (c *Crawl) step(ctx *game.Context, d dungeon.Dir, forward bool) {
	l := c.level
	to := c.pos.Step(d)
	if m := l.MonsterAt(to); m != nil {
		if forward {
			c.startBattle(ctx, m, false)
		} else {
			c.bump(d)
		}
		return
	}
	if ch := l.Chests[to]; ch != nil {
		switch {
		case forward && !ch.Open:
			c.startPuzzle(to, puzzle.Chest)
		case forward:
			c.emptyChest(ch)
			c.bump(d)
		default:
			c.bump(d)
		}
		return
	}
	if ft := l.FeatureAt(to); ft != nil {
		if forward {
			c.useFeature(ctx, ft)
		} else {
			c.bump(d)
		}
		return
	}
	switch l.At(to) {
	case dungeon.Door:
		c.openDoor(to)
		return
	case dungeon.Sealed:
		if forward {
			c.startPuzzle(to, puzzle.Door)
		} else {
			c.bump(d)
		}
		return
	}
	if l.Blocked(to) {
		c.bump(d)
		return
	}
	c.monstersTurn(to)
	c.animate(to, c.angle, stepTicks, false)
	c.pos = to
	c.play(audio.Step)
	c.walked()
}

// emptyChest looks in an opened chest, where gear may have been left
// behind when the hero's bag was full.
func (c *Crawl) emptyChest(ch *dungeon.Chest) {
	if ch.Gear == nil {
		c.run.info("The chest is empty.")
		return
	}
	c.takeLoot(ch, "")
}

func (c *Crawl) openDoor(p dungeon.Point) {
	c.level.Set(p, dungeon.OpenDoor)
	c.play(audio.Door)
	c.run.info("The door creaks open.")
	c.wait()
}

// monstersTurn lets every monster act as the hero moves to hero. A monster
// that ends up next to the hero will attack once the animation is over.
func (c *Crawl) monstersTurn(hero dungeon.Point) {
	c.prev = map[*dungeon.Monster]dungeon.Point{}
	for _, m := range c.level.Monsters {
		c.prev[m] = m.At
	}
	if m := c.level.MoveMonsters(hero, c.run.rng); m != nil {
		c.pending = m
	}
}

// arrived runs when a camera animation ends.
func (c *Crawl) arrived() {
	a := c.anim
	c.prev = nil
	if !a.bump && (a.x0 != a.x1 || a.y0 != a.y1) && c.level.At(c.pos) == dungeon.Stairs {
		if b := c.level.Boss(); b != nil {
			c.run.say(fmt.Sprintf("The %s's dark power holds the stairs shut!", b.Name()), pal.Orange)
		} else {
			c.run.say("Stairs lead down! Press Enter to descend.", pal.Lime)
		}
	}
}

// interact uses whatever is in front of the hero, or waits a turn.
func (c *Crawl) interact(ctx *game.Context) {
	l := c.level
	if l.At(c.pos) == dungeon.Stairs {
		c.descend(ctx)
		return
	}
	ahead := c.pos.Step(c.facing)
	if m := l.MonsterAt(ahead); m != nil {
		c.startBattle(ctx, m, false)
		return
	}
	if ch := l.Chests[ahead]; ch != nil {
		if ch.Open {
			c.emptyChest(ch)
		} else {
			c.startPuzzle(ahead, puzzle.Chest)
		}
		return
	}
	if ft := l.FeatureAt(ahead); ft != nil {
		c.useFeature(ctx, ft)
		return
	}
	switch l.At(ahead) {
	case dungeon.Door:
		c.openDoor(ahead)
	case dungeon.Sealed:
		c.startPuzzle(ahead, puzzle.Door)
	default:
		c.run.info("You wait and listen...")
		c.wait()
	}
}

func (c *Crawl) drinkPotion() bool {
	h := &c.run.hero
	switch {
	case h.Items[rpg.Potion] == 0:
		c.run.info("You have no potions.")
		return false
	case h.HP >= h.MaxHP():
		c.run.info("You are already at full health.")
		return false
	}
	n := h.Heal(h.PotionHeal())
	h.Items[rpg.Potion]--
	c.play(audio.Potion)
	c.run.say(fmt.Sprintf("You drink a potion and recover %d HP.", n), pal.Lime)
	c.float(fmt.Sprintf("+%d", n), pal.Lime)
	return true
}

// descend goes down the stairs, unless a boss still holds them.
func (c *Crawl) descend(ctx *game.Context) {
	if b := c.level.Boss(); b != nil {
		c.play(audio.Bump)
		c.run.say(fmt.Sprintf("The stairs will not open while the %s lives!", b.Name()), pal.Orange)
		return
	}
	r := c.run
	r.depth++
	r.remember()
	c.play(audio.Stairs)
	ctx.Replace(newCrawl(r))
}

func (c *Crawl) die() {
	c.mode = modeDead
	c.battle, c.puzzle = nil, nil
	c.run.hero.HP = 0
	c.play(audio.Fall)
	c.run.say("You have fallen!", pal.Rose)
	if c.run.hardcore() && c.run.onDisk {
		save.Remove(saveName) // one life
		c.run.onDisk = false
	}
}

// goldLost is the share of their gold a fallen hero loses.
const goldLost = 0.2

// wake takes a fallen hero back to their last shrine, with some gold lost.
// The floor there is made anew. Word practice is never lost.
func (c *Crawl) wake(ctx *game.Context) { ctx.Replace(c.woken(ctx)) }

// woken is the crawl a fallen hero wakes up in.
func (c *Crawl) woken(ctx *game.Context) *Crawl {
	r := c.run
	r.regen++
	r.depth = r.shrine.Depth
	r.hero = r.shrine.Hero.Clone()
	lost := int(float64(r.hero.Gold)*goldLost + 0.5)
	r.hero.Gold -= lost
	r.hero.HP, r.hero.MP = r.hero.MaxHP(), r.hero.MaxMP()
	r.hero.Streak = 0
	r.shrine = checkpoint{Depth: r.depth, Regen: r.regen, Hero: r.hero.Clone()}
	next := newCrawl(r)
	msg := fmt.Sprintf("You wake at the shrine on floor %d.", r.depth)
	if r.depth == 1 && !dungeon.ShrineFloor(1) {
		msg = "You wake at the dungeon's entrance."
	}
	if lost > 0 {
		msg += fmt.Sprintf(" You dropped %d gold.", lost)
	}
	r.say(msg, pal.Yellow)
	if r.onDisk {
		// Keep the save in step, so quitting now can't undo the fall.
		next.writeSave(ctx, false)
	}
	return next
}

// camera returns the viewpoint, part way through any animation. The hero
// stands at the back of their cell, so the cell ahead is in full view.
func (c *Crawl) camera() raycast.Camera {
	x, y := center(c.pos)
	ang := c.angle
	if a := &c.anim; a.active() {
		f := a.frac()
		if a.bump {
			f = math.Sin(math.Pi*float64(a.t)/float64(a.dur)) * 0.12
		}
		x, y = a.x0+(a.x1-a.x0)*f, a.y0+(a.y1-a.y0)*f
		ang = a.a0 + (a.a1-a.a0)*f
	}
	back := 0.45 - c.zoom
	return raycast.NewCamera(x-math.Cos(ang)*back, y-math.Sin(ang)*back, ang, 0.75)
}

// look returns a monster's two animation frames.
func (c *Crawl) look(m *dungeon.Monster) [2]*proc.Indexed {
	l, ok := c.looks[m]
	if !ok {
		for f := range l {
			l[f] = proc.MonsterSprite(m.Kind.Family, m.Kind.Hue, m.Seed, f)
			if m.Kind.Boss() {
				l[f] = proc.Crown(l[f])
			}
		}
		c.looks[m] = l
	}
	return l
}

func (c *Crawl) sprites(tick uint64) []raycast.Sprite {
	var out []raycast.Sprite
	for p, ch := range c.level.Chests {
		if p.Manhattan(c.pos) > 12 {
			continue
		}
		x, y := center(p)
		img := c.chest[0]
		if ch.Open {
			img = c.chest[1]
		}
		out = append(out, raycast.Sprite{X: x, Y: y, Img: img, Size: 0.3})
	}
	for p, ft := range c.level.Features {
		if p.Manhattan(c.pos) > 12 {
			continue
		}
		x, y := center(p)
		out = append(out, c.props.sprite(ft, x, y, tick))
	}
	moving := c.anim.active()
	f := c.anim.frac()
	for _, m := range c.level.Monsters {
		fighting := c.battle != nil && c.battle.m == m
		if (m.HP <= 0 && !fighting) || m.At.Manhattan(c.pos) > 12 {
			continue
		}
		x, y := center(m.At)
		if p, ok := c.prev[m]; ok && moving {
			px, py := center(p)
			x, y = px+(x-px)*f, py+(y-py)*f
		}
		frame := int((tick/24 + m.Seed) % 2)
		s := raycast.Sprite{X: x, Y: y, Img: c.look(m)[frame], Size: m.Kind.Size}
		switch m.Kind.Family {
		case dungeon.Bat, dungeon.Ghost, dungeon.Eye:
			s.Lift = 0.22 + 0.04*math.Sin(float64(tick)*0.08+float64(m.Seed%7))
		}
		if fighting {
			c.battle.dress(&s, c)
		}
		out = append(out, s)
	}
	return out
}

// Draw implements game.Scene.
func (c *Crawl) Draw(dst *ebiten.Image, ctx *game.Context) {
	dst.Fill(pal.Black)
	c.view.Render(c.level, c.tex, c.camera(), c.sprites(ctx.Tick), ctx.Tick)
	c.img.WritePixels(c.view.Img.Pix)

	gfx.Window(dst, viewX-4, viewY-4, viewW*gfx.ArtScale+8, viewH*gfx.ArtScale+8)
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	view := dst.SubImage(image.Rect(viewX, viewY, viewX+vw, viewY+vh)).(*ebiten.Image)
	ox, oy := viewX, viewY
	if c.shake > 0 {
		ox += (int(ctx.Tick*7)%5 - 2) * gfx.ArtScale
		oy += (int(ctx.Tick*3)%3 - 1) * gfx.ArtScale
	}
	gfx.DrawArt(view, c.img, ox, oy)
	if c.hurt > 0 {
		gfx.FillRect(view, viewX, viewY, vw, vh, pal.Fade(pal.Red, 0.4*float64(c.hurt)/12))
	}
	c.drawViewOverlay(view, ctx)
	c.drawSide(dst, ctx)
	c.drawPanel(dst, ctx)
	if c.menu != nil {
		c.drawMenu(dst, ctx)
	}
}
