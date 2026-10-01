// Package raycast draws the first-person dungeon view on the CPU, the way
// classic grid crawlers do: textured walls cast column by column, textured
// floors and ceilings, and flat sprites for monsters and chests. It works at
// a low resolution in the game palette, and shades by distance with ordered
// dithering, so the result looks like 8-bit art rather than modern 3D.
package raycast

import (
	"cmp"
	"image"
	"math"
	"slices"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
)

// levels is the number of light levels, from black to fully lit.
const levels = 8

// Textures are the images for one dungeon world.
type Textures struct {
	Walls     []*proc.Indexed // variants, picked per cell
	Torch     []*proc.Indexed // animation frames
	Door      *proc.Indexed
	Sealed    *proc.Indexed
	Floor     *proc.Indexed
	FloorAnim []*proc.Indexed // animation frames of water or lava, or nil
	Ceil      *proc.Indexed
	Stairs    *proc.Indexed
	Sky       *proc.Indexed // an open sky panorama instead of the ceiling, or nil
}

// NewTextures generates the textures for a world. The Crypt's are exactly
// the ones it always had.
func NewTextures(t *proc.Theme, seed uint64) *Textures {
	tx := &Textures{
		Door:      t.DoorTex(seed+10, false),
		Sealed:    t.DoorTex(seed+10, true),
		Floor:     t.FloorTex(seed + 11),
		FloorAnim: t.FloorFrames(seed + 11),
		Ceil:      t.CeilTex(seed + 12),
		Stairs:    t.StairsTex(seed + 11),
		Sky:       t.SkyTex(seed + 13),
	}
	for v := range 3 {
		tx.Walls = append(tx.Walls, t.WallTex(seed+uint64(v), v))
	}
	for f := range 4 {
		tx.Torch = append(tx.Torch, t.TorchTex(seed, f))
	}
	return tx
}

// Camera is the viewer's position and direction in map units, where cell
// (x, y) covers [x, x+1) × [y, y+1).
type Camera struct {
	X, Y           float64
	DirX, DirY     float64
	PlaneX, PlaneY float64
}

// NewCamera returns a camera at (x, y) looking at angle (radians; 0 is east,
// π/2 is south). plane sets the field of view: 0.75 is about 74°.
func NewCamera(x, y, angle, plane float64) Camera {
	dx, dy := math.Cos(angle), math.Sin(angle)
	return Camera{X: x, Y: y, DirX: dx, DirY: dy, PlaneX: -dy * plane, PlaneY: dx * plane}
}

// Angle returns the angle a compass direction faces.
func Angle(d dungeon.Dir) float64 { return float64(int(d)-1) * math.Pi / 2 }

// Sprite is a flat picture that always faces the viewer.
type Sprite struct {
	X, Y  float64 // map position of its base
	Img   *proc.Indexed
	Size  float64 // height in map units (a wall is 1)
	Lift  float64 // height of its base above the floor
	Flash bool    // draw in solid white, when hit
}

// Renderer draws views into Img.
type Renderer struct {
	W, H int
	Img  *image.RGBA
	// Light is the hero's torch radius in cells.
	Light float64
	// Calm turns off moving effects: particles, rippling water and lava,
	// and flickering torches.
	Calm bool

	zbuf        []float64
	shade       [levels][256]uint8 // walls, floors and ceilings
	spriteShade [levels][256]uint8 // sprites, kept out of the fog
	rgba        [256][4]byte

	// The world's air, set by SetWorld.
	reach     float64 // multiplies Light
	ceilH     float64 // ceiling height in wall heights
	particles proc.Particles
	invLight  float64 // 1 / (Light × reach), for this frame

	skyCol []int  // the sky panorama column of each view column
	items  []item // sprites to draw, reused between frames

	level    *dungeon.Level
	torchMap []float64 // light from wall torches at each cell
	seed     uint64    // the level's seed for particles
}

// New returns a renderer for a w×h view, in The Crypt's air until SetWorld
// says otherwise.
func New(w, h int) *Renderer {
	r := &Renderer{
		W: w, H: h, Img: image.NewRGBA(image.Rect(0, 0, w, h)), Light: 5.5,
		zbuf: make([]float64, w), skyCol: make([]int, w),
	}
	for i, c := range pal.All {
		r.rgba[i] = [4]byte{c.R, c.G, c.B, 0xff}
	}
	r.SetWorld(&proc.Themes[0])
	return r
}

// bayer is a 4×4 ordered-dither matrix in [0,1).
var bayer = [4][4]float64{
	{0 / 16.0, 8 / 16.0, 2 / 16.0, 10 / 16.0},
	{12 / 16.0, 4 / 16.0, 14 / 16.0, 6 / 16.0},
	{3 / 16.0, 11 / 16.0, 1 / 16.0, 9 / 16.0},
	{15 / 16.0, 7 / 16.0, 13 / 16.0, 5 / 16.0},
}

// put writes palette index c at (x, y) with brightness b in [0,1]. Glowing
// pixels ignore b.
func (r *Renderer) put(x, y int, c uint8, b float64, glow bool) {
	if !glow {
		c = r.shade[light(x, y, b)][c]
	}
	r.plot(x, y, c)
}

// putSprite is put for sprites, which never sink fully into the fog.
func (r *Renderer) putSprite(x, y int, c uint8, b float64, glow bool) {
	if !glow {
		c = r.spriteShade[light(x, y, b)][c]
	}
	r.plot(x, y, c)
}

// light returns the dithered light level for brightness b at (x, y).
func light(x, y int, b float64) int {
	lv := int(b*(levels-1) + bayer[y&3][x&3])
	if lv >= levels {
		return levels - 1
	} else if lv < 0 {
		return 0
	}
	return lv
}

// plot writes palette index c at (x, y) as it is.
func (r *Renderer) plot(x, y int, c uint8) {
	o := (y*r.W + x) * 4
	copy(r.Img.Pix[o:o+4], r.rgba[c][:])
}

// texel samples tex at (u, v) in [0,1).
func texel(tex *proc.Indexed, u, v float64) (uint8, bool) {
	tx := int(u * float64(tex.W))
	ty := int(v * float64(tex.H))
	tx = min(max(tx, 0), tex.W-1)
	ty = min(max(ty, 0), tex.H-1)
	i := ty*tex.W + tx
	return tex.Pix[i], tex.Glow != nil && tex.Glow[i]
}

// prepare computes per-level data the first time a level is drawn.
func (r *Renderer) prepare(l *dungeon.Level) {
	if r.level == l {
		return
	}
	r.level = l
	r.seed = l.Seed ^ uint64(l.Depth)*0x9e3779b97f4a7c15 ^ particleSalt
	r.torchMap = make([]float64, l.W*l.H)
	const reach = 3.5
	for ty := 0; ty < l.H; ty++ {
		for tx := 0; tx < l.W; tx++ {
			if !l.Torches[ty*l.W+tx] {
				continue
			}
			for y := max(0, ty-4); y <= min(l.H-1, ty+4); y++ {
				for x := max(0, tx-4); x <= min(l.W-1, tx+4); x++ {
					d := math.Hypot(float64(x-tx), float64(y-ty))
					v := 0.9 * (1 - d/reach)
					if v > r.torchMap[y*l.W+x] {
						r.torchMap[y*l.W+x] = v
					}
				}
			}
		}
	}
}

// ambient returns torch light at map position (x, y), blended between cells.
func (r *Renderer) ambient(x, y float64) float64 {
	l := r.level
	x -= 0.5
	y -= 0.5
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	at := func(cx, cy int) float64 {
		if cx < 0 || cy < 0 || cx >= l.W || cy >= l.H {
			return 0
		}
		return r.torchMap[cy*l.W+cx]
	}
	a := at(x0, y0) + (at(x0+1, y0)-at(x0, y0))*fx
	b := at(x0, y0+1) + (at(x0+1, y0+1)-at(x0, y0+1))*fx
	return a + (b-a)*fy
}

// brightness combines the hero's torch at distance d with wall torches.
func (r *Renderer) brightness(d, x, y float64) float64 {
	b := 1.2 - d*r.invLight
	return math.Max(0, math.Min(1, math.Max(b, r.ambient(x, y))))
}

// seeRange is how far away cells are marked as seen for the automap.
const seeRange = 7

// Render draws the level from cam. tick animates torches and animated
// floors.
func (r *Renderer) Render(l *dungeon.Level, tex *Textures, cam Camera, sprites []Sprite, tick uint64) {
	r.prepare(l)
	r.invLight = 1 / (r.Light * r.reach)
	w, h := r.W, r.H
	k := float64(w) / (2 * math.Hypot(cam.PlaneX, cam.PlaneY)) // pixels per map unit at distance 1
	horizon := float64(h) / 2
	torch, floorTex := tex.Torch[0], tex.Floor
	if !r.Calm {
		torch = tex.Torch[int(tick/8)%len(tex.Torch)]
		if n := len(tex.FloorAnim); n > 0 {
			floorTex = tex.FloorAnim[int(tick/floorFrameTicks)%n]
		}
	}
	if tex.Sky != nil {
		r.aimSky(cam, tex.Sky.W)
	}

	// Floor and ceiling, row by row.
	for y := range h {
		floor := float64(y)+0.5 > horizon
		if !floor && tex.Sky != nil {
			r.skyRow(tex.Sky, y, horizon)
			continue
		}
		p := math.Abs(float64(y) + 0.5 - horizon)
		dist := k * 0.5 / p
		if !floor {
			dist = k * (r.ceilH - 0.5) / p
		}
		lx := cam.X + dist*(cam.DirX-cam.PlaneX)
		ly := cam.Y + dist*(cam.DirY-cam.PlaneY)
		sx := dist * 2 * cam.PlaneX / float64(w)
		sy := dist * 2 * cam.PlaneY / float64(w)
		for x := range w {
			fx, fy := lx+sx*float64(x), ly+sy*float64(x)
			cx, cy := int(math.Floor(fx)), int(math.Floor(fy))
			t := tex.Ceil
			if floor {
				t = floorTex
				if l.At(dungeon.Point{X: cx, Y: cy}) == dungeon.Stairs {
					t = tex.Stairs
				}
			}
			c, glow := texel(t, fx-float64(cx), fy-float64(cy))
			b := r.brightness(dist, fx, fy)
			if !floor {
				b *= 0.8
			}
			r.put(x, y, c, b, glow)
		}
	}

	// Walls, column by column. The hero's own cell counts as seen too.
	if p := (dungeon.Point{X: int(math.Floor(cam.X)), Y: int(math.Floor(cam.Y))}); l.In(p) {
		l.Seen[l.Index(p)] = true
	}
	for x := range w {
		camX := 2*(float64(x)+0.5)/float64(w) - 1
		rdx := cam.DirX + cam.PlaneX*camX
		rdy := cam.DirY + cam.PlaneY*camX
		mx, my := int(math.Floor(cam.X)), int(math.Floor(cam.Y))
		ddx, ddy := math.Inf(1), math.Inf(1)
		if rdx != 0 {
			ddx = math.Abs(1 / rdx)
		}
		if rdy != 0 {
			ddy = math.Abs(1 / rdy)
		}
		stepX, stepY := 1, 1
		sdx := (float64(mx) + 1 - cam.X) * ddx
		sdy := (float64(my) + 1 - cam.Y) * ddy
		if rdx < 0 {
			stepX, sdx = -1, (cam.X-float64(mx))*ddx
		}
		if rdy < 0 {
			stepY, sdy = -1, (cam.Y-float64(my))*ddy
		}
		side := 0
		var tile dungeon.Tile
		for range 64 {
			if sdx < sdy {
				sdx += ddx
				mx += stepX
				side = 0
			} else {
				sdy += ddy
				my += stepY
				side = 1
			}
			p := dungeon.Point{X: mx, Y: my}
			tile = l.At(p)
			near := math.Min(sdx-ddx, sdy-ddy) < seeRange
			if l.In(p) && near {
				l.Seen[l.Index(p)] = true
			}
			if tile.Solid() {
				break
			}
		}
		perp := sdy - ddy
		if side == 0 {
			perp = sdx - ddx
		}
		perp = math.Max(perp, 0.01)
		r.zbuf[x] = perp

		hx, hy := cam.X+perp*rdx, cam.Y+perp*rdy
		u := hx - math.Floor(hx)
		if side == 0 {
			u = hy - math.Floor(hy)
		}
		if (side == 0 && rdx < 0) || (side == 1 && rdy > 0) {
			u = 1 - u
		}
		plain := tex.Walls[wallVariant(mx, my, len(tex.Walls))]
		t := plain
		switch {
		case tile == dungeon.Door:
			t = tex.Door
		case tile == dungeon.Sealed:
			t = tex.Sealed
		case l.In(dungeon.Point{X: mx, Y: my}) && l.Torches[my*l.W+mx]:
			t = torch
		}
		// Light the wall from the open cell in front of it.
		b := r.brightness(perp, hx-rdx*0.02, hy-rdy*0.02)
		if side == 1 {
			b -= 0.1
		}
		r.wallColumn(x, t, plain, u, b, horizon, k/perp)
	}

	r.drawSprites(cam, sprites, k, horizon)
}

// floorFrameTicks is how long each frame of an animated floor shows: at 60
// ticks a second, water and lava change twice a second.
const floorFrameTicks = 30

// wallColumn draws column x of a wall lineH pixels per wall height. The
// bottom unit shows t (which may be a door or torch); the rest of a tall
// wall stacks plain wall above it. Transparent texels let the sky through.
func (r *Renderer) wallColumn(x int, t, plain *proc.Indexed, u, b, horizon, lineH float64) {
	bottom := horizon + lineH/2
	top := bottom - lineH*r.ceilH
	y0 := max(0, int(math.Ceil(top-0.5)))
	y1 := min(r.H, int(math.Ceil(bottom-0.5)))
	unitTop := bottom - lineH // where t starts
	for y := y0; y < y1; y++ {
		fy := float64(y) + 0.5
		tt, v := t, (fy-unitTop)/lineH
		if v < 0 {
			tt, v = plain, v-math.Floor(v)
		}
		c, glow := texel(tt, u, v)
		if c == proc.Transparent {
			continue
		}
		r.put(x, y, c, b, glow)
	}
}

// aimSky works out which sky panorama column each view column sees. One
// panorama width is a full turn, so the sky turns with the view.
func (r *Renderer) aimSky(cam Camera, skyW int) {
	for x := range r.W {
		camX := 2*(float64(x)+0.5)/float64(r.W) - 1
		a := math.Atan2(cam.DirY+cam.PlaneY*camX, cam.DirX+cam.PlaneX*camX)
		r.skyCol[x] = int((a/(2*math.Pi)+1)*float64(skyW)) % skyW
	}
}

// skyRow draws view row y of an open sky. The sky is far away, so it is
// neither shaded nor lit.
func (r *Renderer) skyRow(sky *proc.Indexed, y int, horizon float64) {
	sy := min(sky.H-1, int(float64(y)/horizon*float64(sky.H)))
	row := sky.Pix[sy*sky.W : (sy+1)*sky.W]
	for x, col := range r.skyCol {
		r.plot(x, y, row[col])
	}
}

// wallVariant picks a wall texture for a cell: mostly plain, some mossy or
// cracked.
func wallVariant(x, y, n int) int {
	h := uint32(x)*73856093 ^ uint32(y)*19349663
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	switch v := h % 10; {
	case v < 6 || n < 2:
		return 0
	case v < 8 || n < 3:
		return 1
	default:
		return 2
	}
}

// item is a sprite to draw, with its depth and screen column.
type item struct {
	s     *Sprite
	depth float64
	sx    float64
}

func (r *Renderer) drawSprites(cam Camera, sprites []Sprite, k, horizon float64) {
	inv := 1 / (cam.PlaneX*cam.DirY - cam.DirX*cam.PlaneY)
	r.items = r.items[:0]
	for i := range sprites {
		s := &sprites[i]
		dx, dy := s.X-cam.X, s.Y-cam.Y
		tx := inv * (cam.DirY*dx - cam.DirX*dy)
		ty := inv * (-cam.PlaneY*dx + cam.PlaneX*dy)
		if ty < 0.1 {
			continue
		}
		r.items = append(r.items, item{s, ty, float64(r.W) / 2 * (1 + tx/ty)})
	}
	// Far to near, so near sprites cover far ones.
	slices.SortStableFunc(r.items, func(a, b item) int { return cmp.Compare(b.depth, a.depth) })

	for _, it := range r.items {
		s := it.s
		img := s.Img
		hgt := s.Size * k / it.depth
		wid := hgt * float64(img.W) / float64(img.H)
		bottom := horizon + (0.5-s.Lift)*k/it.depth
		top := bottom - hgt
		left := it.sx - wid/2
		b := max(spriteMinLight, r.brightness(it.depth, s.X, s.Y))
		x0 := max(0, int(math.Ceil(left-0.5)))
		x1 := min(r.W, int(math.Ceil(left+wid-0.5)))
		y0 := max(0, int(math.Ceil(top-0.5)))
		y1 := min(r.H, int(math.Ceil(bottom-0.5)))
		for x := x0; x < x1; x++ {
			if it.depth >= r.zbuf[x] {
				continue
			}
			u := (float64(x) + 0.5 - left) / wid
			for y := y0; y < y1; y++ {
				v := (float64(y) + 0.5 - top) / hgt
				c, glow := texel(img, u, v)
				if c == proc.Transparent {
					continue
				}
				if s.Flash {
					c, glow = white, true
				}
				r.putSprite(x, y, c, b, glow)
			}
		}
	}
}

var white = pal.Index(pal.White)
