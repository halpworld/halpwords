// Command worldsheet renders every dungeon world from a few camera spots it
// picks by itself (a corridor, a monster in a room, a door or the stairs)
// and writes contact sheets for reviewing the worlds' looks: one row per
// floor of the first lap, the same for the second lap, and a sheet per
// world with its textures.
//
//	go run ./tools/worldsheet [-o dist/worlds] [-seed 7]
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/raycast"
	"github.com/halpworld/halpwords/internal/unifont"
	"github.com/halpworld/halpwords/pkg/proc"
)

// The game's 3D view, in art pixels.
const viewW, viewH = 192, 120

// particleTick is the moment the particles are drawn at.
const particleTick = 600

var face *unifont.Face

// view is a named camera spot.
type view struct {
	name string
	cam  raycast.Camera
}

// shot is one floor rendered from its views.
type shot struct {
	title string
	theme *proc.Theme
	tex   *raycast.Textures
	views []view
	imgs  []*image.RGBA
}

func main() {
	out := flag.String("o", "dist/worlds", "folder to write the sheets to")
	seed := flag.Uint64("seed", 7, "dungeon seed")
	flag.Parse()
	var err error
	if face, err = unifont.ParseBytes(assets.UnifontHex); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	n := len(proc.FloorOrder)
	var lap1, lap2 []shot
	for depth := 1; depth <= n; depth++ {
		lap1 = append(lap1, shoot(*seed, depth))
		lap2 = append(lap2, shoot(*seed, depth+n))
	}
	writePNG(filepath.Join(*out, "contact.png"), contact(lap1, 2))
	writePNG(filepath.Join(*out, "lap2.png"), contact(lap2, 2))
	for _, s := range lap1 {
		writePNG(filepath.Join(*out, slug(s.theme.Name)+".png"), worldSheet(s))
	}
	log.Printf("wrote the sheets to %s", *out)
}

// shoot renders floor depth of a run with the given seed, as the game
// would draw it.
func shoot(seed uint64, depth int) shot {
	l := dungeon.Generate(seed*31+uint64(depth)*7919, depth)
	th := proc.ThemeFor(depth)
	decor := raycast.Decor(l, th)
	s := shot{
		title: fmt.Sprintf("Floor %d: %s", depth, th.Name),
		theme: th,
		tex:   raycast.NewTextures(th, l.Seed),
		views: pickViews(l, decor),
	}
	r := raycast.New(viewW, viewH)
	r.SetWorld(th)
	sprites := append(sceneSprites(l), decor...)
	for _, v := range s.views {
		r.Render(l, s.tex, v.cam, sprites, particleTick)
		r.DrawParticles(v.cam, particleTick)
		img := image.NewRGBA(r.Img.Bounds())
		copy(img.Pix, r.Img.Pix)
		s.imgs = append(s.imgs, img)
	}
	return s
}

// sceneSprites are the chests and monsters of a floor, drawn the way the
// crawl draws them.
func sceneSprites(l *dungeon.Level) []raycast.Sprite {
	var out []raycast.Sprite
	chest := proc.ChestSprite(false)
	for p := range l.Chests {
		x, y := center(p)
		out = append(out, raycast.Sprite{X: x, Y: y, Img: chest, Size: 0.3})
	}
	for _, m := range l.Monsters {
		x, y := center(m.At)
		img := proc.MonsterSprite(m.Kind.Family, m.Kind.Hue, m.Seed, 0)
		if m.Kind.Boss() {
			img = proc.Crown(img)
		}
		s := raycast.Sprite{X: x, Y: y, Img: img, Size: m.Kind.Size}
		switch m.Kind.Family {
		case dungeon.Bat, dungeon.Ghost, dungeon.Eye:
			s.Lift = 0.22
		}
		out = append(out, s)
	}
	return out
}

func slug(s string) string { return strings.ToLower(strings.ReplaceAll(s, " ", "-")) }

func center(p dungeon.Point) (float64, float64) { return float64(p.X) + 0.5, float64(p.Y) + 0.5 }

// cameraAt places the hero at the back of cell p facing d, as the game does.
func cameraAt(p dungeon.Point, d dungeon.Dir) raycast.Camera {
	x, y := center(p)
	a := raycast.Angle(d)
	return raycast.NewCamera(x-math.Cos(a)*0.45, y-math.Sin(a)*0.45, a, 0.75)
}

func inRoom(l *dungeon.Level, p dungeon.Point) bool {
	for _, r := range l.Rooms {
		if r.Contains(p) {
			return true
		}
	}
	return false
}

// open reports whether p and the next k-1 cells towards d are walkable.
func open(l *dungeon.Level, p dungeon.Point, d dungeon.Dir, k int) bool {
	for range k {
		if !l.At(p).Walkable() {
			return false
		}
		p = p.Step(d)
	}
	return true
}

// behind returns the cell k steps back from p, looking towards d.
func behind(p dungeon.Point, d dungeon.Dir, k int) dungeon.Point {
	for range k {
		p = p.Step(d.Back())
	}
	return p
}

// picker keeps the best scoring spot for each kind of view.
type picker struct {
	best  [3]view
	score [3]float64
	at    [3]dungeon.Point
	decor []raycast.Sprite
}

// props counts props in front of a camera at p facing d, up to two.
func (pk *picker) props(p dungeon.Point, d dungeon.Dir) float64 {
	n := 0.0
	x, y := center(p)
	dd := d.Delta()
	for _, s := range pk.decor {
		dx, dy := s.X-x, s.Y-y
		dist := math.Hypot(dx, dy)
		if dist > 0.5 && dist < 5 && (dx*float64(dd.X)+dy*float64(dd.Y))/dist > 0.75 {
			n++
		}
	}
	return math.Min(n, 2)
}

func (pk *picker) try(i int, s float64, name string, p dungeon.Point, d dungeon.Dir) {
	s += 1.5 * pk.props(p, d)
	if i == 2 && pk.score[1] >= 0 && p.Manhattan(pk.at[1]) < 4 {
		s -= 5 // show a different spot from the monster view
	}
	if s > pk.score[i] {
		pk.at[i], pk.score[i], pk.best[i] = p, s, view{name, cameraAt(p, d)}
	}
}

// pickViews searches the floor for a corridor, a monster in a room and a
// door or the stairs.
func pickViews(l *dungeon.Level, decor []raycast.Sprite) []view {
	pk := &picker{score: [3]float64{-1, -1, -1}, decor: decor}
	for y := range l.H {
		for x := range l.W {
			p := dungeon.Point{X: x, Y: y}
			if inRoom(l, p) || !l.At(p).Walkable() {
				continue
			}
			for d := range dungeon.Dir(4) {
				// A corridor: walls (or torches) on both sides, open ahead.
				run, sides := 0, 0
				for q := p; run < 9 && l.At(q).Walkable(); q = q.Step(d) {
					if l.At(q.Step(d.Left())).Solid() && l.At(q.Step(d.Right())).Solid() {
						sides++
					}
					if l.In(q.Step(d.Left())) && l.Torches[l.Index(q.Step(d.Left()))] {
						sides++
					}
					run++
				}
				if run >= 4 && sides >= 3 {
					pk.try(0, float64(min(run, 7)+sides), "corridor", p, d)
				}
			}
		}
	}
	for _, m := range l.Monsters {
		for d := range dungeon.Dir(4) {
			for k := 2; k <= 3; k++ {
				p := behind(m.At, d, k)
				if !open(l, p, d, k) || !inRoom(l, p) {
					continue
				}
				s := 10 - float64(k)
				if m.Kind.Boss() {
					s++
				}
				pk.try(1, s, "monster", p, d)
			}
		}
	}
	for y := range l.H {
		for x := range l.W {
			t := dungeon.Point{X: x, Y: y}
			tile := l.At(t)
			if tile != dungeon.Door && tile != dungeon.Sealed && tile != dungeon.Stairs {
				continue
			}
			for d := range dungeon.Dir(4) {
				for k := 2; k <= 3; k++ {
					p := behind(t, d, k)
					if !open(l, p, d, k) {
						continue
					}
					s := 6 - float64(k)
					switch tile {
					case dungeon.Stairs:
						s += 3
						if !inRoom(l, p) {
							s -= 2
						}
					case dungeon.Sealed:
						s++
					}
					pk.try(2, s, "door/stairs", p, d)
				}
			}
		}
	}
	var out []view
	for i, v := range pk.best {
		if pk.score[i] < 0 {
			v = view{"start", cameraAt(l.Start, l.StartDir)}
		}
		out = append(out, v)
	}
	return out
}

// Drawing the sheets.

var (
	background = color.RGBA{0x14, 0x12, 0x1c, 0xff}
	white      = color.RGBA{0xff, 0xff, 0xff, 0xff}
	dim        = color.RGBA{0x9b, 0xad, 0xb7, 0xff}
)

const gap = 8

func text(dst *image.RGBA, x, y int, s string, c color.RGBA) {
	for _, ch := range s {
		g := face.Glyph(ch)
		if g == nil {
			x += 8
			continue
		}
		for gy := range unifont.Height {
			for gx := range g.Width {
				if g.Set(gx, gy) {
					dst.SetRGBA(x+gx, y+gy, c)
				}
			}
		}
		x += g.Width
	}
}

// blit draws src onto dst at (x, y), each pixel scale×scale.
func blit(dst *image.RGBA, src *image.RGBA, x, y, scale int) {
	b := src.Bounds()
	for sy := range b.Dy() {
		for sx := range b.Dx() {
			c := src.RGBAAt(b.Min.X+sx, b.Min.Y+sy)
			if c.A == 0 {
				continue
			}
			draw.Draw(dst, image.Rect(x+sx*scale, y+sy*scale, x+(sx+1)*scale, y+(sy+1)*scale), &image.Uniform{c}, image.Point{}, draw.Src)
		}
	}
}

func newSheet(w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{background}, image.Point{}, draw.Src)
	return dst
}

// contact lays out one labelled row of views per floor.
func contact(shots []shot, scale int) *image.RGBA {
	const label = 22
	cw, ch := viewW*scale, viewH*scale
	dst := newSheet(gap+3*(cw+gap), gap+len(shots)*(label+ch+gap))
	for r, s := range shots {
		y := gap + r*(label+ch+gap)
		text(dst, gap, y+2, s.title, white)
		for c, img := range s.imgs {
			blit(dst, img, gap+c*(cw+gap), y+label, scale)
		}
	}
	return dst
}

// worldSheet shows one world's views large, with its textures below.
func worldSheet(s shot) *image.RGBA {
	const scale, ts = 3, 3
	cw, ch := viewW*scale, viewH*scale
	type tile struct {
		name string
		img  *proc.Indexed
	}
	tex := s.tex
	tiles := []tile{
		{"wall", tex.Walls[0]}, {"wall 1", tex.Walls[1]}, {"wall 2", tex.Walls[2]},
		{"door", tex.Door}, {"sealed", tex.Sealed}, {"torch", tex.Torch[0]},
		{"floor", tex.Floor}, {"stairs", tex.Stairs}, {"ceiling", tex.Ceil},
		{"prop", s.theme.Prop(1)},
	}
	for i, f := range tex.FloorAnim[min(1, len(tex.FloorAnim)):] {
		tiles = append(tiles, tile{fmt.Sprintf("frame %d", i+1), f})
	}
	tilesY := gap + 24 + ch + 24 + 20
	h := tilesY + proc.TexSize*ts + gap
	if tex.Sky != nil {
		h += 24 + tex.Sky.H + gap
	}
	dst := newSheet(gap+3*(cw+gap), h)
	text(dst, gap, gap, s.title, white)
	for i, img := range s.imgs {
		x := gap + i*(cw+gap)
		blit(dst, img, x, gap+24, scale)
		text(dst, x, gap+24+ch+4, s.views[i].name, dim)
	}
	x, y := gap, tilesY
	for _, t := range tiles {
		if x+t.img.W*ts > dst.Bounds().Dx() {
			break
		}
		text(dst, x, y-18, t.name, dim)
		blit(dst, t.img.RGBA(), x, y, ts)
		x += t.img.W*ts + 2*gap
	}
	if tex.Sky != nil {
		y += proc.TexSize*ts + gap + 20
		text(dst, gap, y-18, "sky", dim)
		blit(dst, tex.Sky.RGBA(), gap, y, 1)
	}
	return dst
}

func writePNG(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
