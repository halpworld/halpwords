package scene

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/raycast"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/pkg/proc"
	"github.com/halpworld/halpwords/pkg/raid"
	"github.com/halpworld/halpwords/pkg/words"
)

// Boss Raids (W7.5): in a raid room the teacher starts a raid from the
// class's big screen, and the whole class fights one boss. The player
// joins by code in the lobby, as for any room; when the raid starts the
// game shows the boss in the dungeon view, its health, and a typing
// panel for the words the server deals. The server grades every answer
// with the game's own rules (pkg/raid) and says what it did: the game
// never grades a raid itself, and keeps nothing of it. When the boss
// attacks, every raider types a dodge in time or is stunned for a
// moment. The finale shows the class's totals and what the player did.

// RaidScreen is a Boss Raid for the player.
type RaidScreen struct {
	play   *link.Play
	number int // the raid's link.Raid.Number
	st     link.PlayState

	lang  *words.Language
	field *typing.Field
	// n is the word being typed (its link.RaidWord.N), and erased is set
	// when Backspace was used on it.
	n      int
	erased bool
	// limit is the time the dodge being typed had, for its bar.
	limit time.Duration

	name  string // the boss's name
	look  [2]*proc.Indexed
	size  float64
	level *dungeon.Level
	tex   *raycast.Textures
	view  *raycast.Renderer
	img   *ebiten.Image
	cam   raycast.Camera
	bx    float64 // where the boss stands
	by    float64

	// What the screen has reacted to: hits on the boss, raiders who
	// joined late, the last attack started and ended, the last grade.
	hits, grew, attack, attacked, graded int
	// flash, shake and hurt are ticks of the boss flashing, the view
	// shaking and a red flash left.
	flash, shake, hurt int
	// news is a line over the view for newsT ticks.
	news    string
	newsCol color.RGBA
	newsT   int
	// log is the last hits, newest last.
	log []logLine
	// over is set once the raid has ended for the player.
	over    bool
	leaving bool
	bg      *ebiten.Image
}

// raidLogLines is how many hits the side panel lists.
const raidLogLines = 7

// newRaidScreen makes the screen for the raid in st, which must have one.
func newRaidScreen(ctx *game.Context, st link.PlayState) *RaidScreen {
	rd := st.Raid
	lang, ok := words.Lookup(rd.Lang)
	if !ok {
		lang = &words.Language{Code: rd.Lang, Name: strings.ToUpper(rd.Lang)}
	}
	b := raid.BossFor(rd.Seed)
	s := &RaidScreen{play: ctx.Link.Play(), number: rd.Number, st: st, lang: lang, field: typing.NewField(lang),
		name: b.Name, size: 0.95, bg: backdrop(11, 1.2)}
	for f := range s.look {
		s.look[f] = b.Sprite(f)
	}
	if k := dungeon.KindNamed(b.Name); k != nil {
		s.size = k.Size
	}
	s.stage(rd.Seed)
	// What happened before the screen opened isn't news.
	s.hits, s.grew = rd.Hits, rd.Grew
	if rd.Attack != nil {
		s.attack, s.attacked = rd.Attack.N, rd.Attack.N
	}
	if rd.Graded != nil {
		s.graded = rd.Graded.N
	}
	s.say(fmt.Sprintf("Boss Raid! Defeat the %s together!", b.Name), pal.Yellow)
	return s
}

// stage builds the boss's lair: a boss floor of the dungeon, with the
// camera at one end of its largest room and the boss across it.
func (s *RaidScreen) stage(seed uint64) {
	l := dungeon.Generate(seed, 3)
	room := l.Rooms[0]
	for _, r := range l.Rooms {
		if r.W*r.H > room.W*room.H {
			room = r
		}
	}
	th := proc.ThemeFor(3)
	y := float64(room.Y+room.H/2) + 0.5
	s.level, s.tex = l, raycast.NewTextures(th, l.Seed)
	s.view, s.img = raycast.New(viewW, viewH), ebiten.NewImage(viewW, viewH)
	s.cam = raycast.NewCamera(float64(room.X)+0.05, y, 0, 0.75)
	s.bx, s.by = float64(room.X+1)+0.5, y
}

// say puts a line over the view for a few seconds.
func (s *RaidScreen) say(text string, col color.RGBA) {
	s.news, s.newsCol, s.newsT = text, col, 180
}

// Update implements game.Scene.
func (s *RaidScreen) Update(ctx *game.Context) error {
	s.st = s.play.State()
	s.flash, s.shake, s.hurt, s.newsT = max(0, s.flash-1), max(0, s.shake-1), max(0, s.hurt-1), max(0, s.newsT-1)
	if s.st.Phase == link.PlayOff {
		// Out of the room: the lobby says why.
		ctx.Replace(NewLobby(ctx))
		return nil
	}
	rd := s.st.Raid
	if rd == nil || rd.Number != s.number {
		if !s.over {
			s.over = true
			if f := s.st.Finale; f != nil && f.Outcome == link.RaidWon {
				ctx.Sound.Play(audio.Defeat)
			} else {
				ctx.Sound.Play(audio.Flee)
			}
		}
		if input.Confirm() || input.Back() {
			ctx.Sound.Play(audio.Select)
			ctx.Replace(&Lobby{bg: backdrop(7, 1.15), raided: s.number})
		}
		return nil
	}
	s.react(ctx, rd)
	if s.leaving {
		switch {
		case input.Pressed(ebiten.KeyY):
			ctx.Sound.Play(audio.Back)
			s.play.Leave()
			ctx.Replace(NewLobby(ctx))
		case input.Pressed(ebiten.KeyN) || input.Back():
			ctx.Sound.Play(audio.Back)
			s.leaving = false
		}
		return nil
	}
	if input.Back() {
		ctx.Sound.Play(audio.Select)
		s.leaving = true
		return nil
	}
	w := rd.Word
	if w == nil {
		return nil
	}
	if w.N != s.n {
		s.n, s.erased, s.limit = w.N, false, time.Until(w.Until)
		s.field.Reset()
	}
	if input.Repeat(ebiten.KeyBackspace) && s.field.Len() > 0 {
		s.erased = true
	}
	if typeInto(ctx, s.field) && s.play.Answer(w.N, s.field.Text(), s.erased) {
		s.field.Reset()
	}
	return nil
}

// react shows what happened in the raid since the last frame: hits on
// the boss, a late raider, the boss's attacks and the player's grades.
func (s *RaidScreen) react(ctx *game.Context, rd *link.Raid) {
	if rd.Hits > s.hits {
		s.hits = rd.Hits
		s.flash = 8
		h := rd.LastHit
		who := raiderName(s.st.Room, h.ID)
		if h.ID == s.st.Room.You {
			who = "You"
		}
		s.log = append(s.log, logLine{fmt.Sprintf("%s: %d damage", who, h.Damage), pal.Ice})
		if n := len(s.log) - raidLogLines; n > 0 {
			s.log = s.log[n:]
		}
	}
	if rd.Grew > s.grew {
		s.grew = rd.Grew
		ctx.Sound.Play(audio.Rage)
		s.say("Another raider joined. The boss grows stronger!", pal.Orange)
	}
	if a := rd.Attack; a != nil {
		if !a.Done && a.N != s.attack {
			s.attack = a.N
			s.shake = 20
			ctx.Sound.Play(audio.Alert)
			s.say("The boss attacks! Dodge!", pal.Orange)
		}
		if a.Done && a.N != s.attacked {
			s.attack, s.attacked = a.N, a.N
			s.say(fmt.Sprintf("%d of %d raiders dodged.", a.Dodged, a.Of), pal.Cyan)
		}
	}
	if g := rd.Graded; g != nil && g.N != s.graded {
		s.graded = g.N
		switch {
		case g.Dodge && g.Dodged:
			ctx.Sound.Play(audio.Dodge)
			s.say("You dodged!", pal.Lime)
		case g.Dodge:
			ctx.Sound.Play(audio.Hurt)
			s.hurt = 12
			if g.Late {
				s.say("Too slow! You are stunned.", pal.Rose)
			} else {
				s.say("Hit! You are stunned.", pal.Rose)
			}
		default:
			ctx.Sound.Play(tierSound[g.Tier])
		}
	}
}

// raiderName is a raider's pseudonym.
func raiderName(room link.Room, id string) string {
	for _, m := range room.Members {
		if m.ID == id && m.Name != "" {
			return m.Name
		}
	}
	return "A raider"
}

// Draw implements game.Scene.
func (s *RaidScreen) Draw(dst *ebiten.Image, ctx *game.Context) {
	if s.over {
		s.drawFinale(dst, ctx)
		return
	}
	dst.Fill(pal.Black)
	s.drawView(dst, ctx)
	s.drawSide(dst, ctx)
	s.drawPanel(dst, ctx)
	if s.leaving {
		f := ctx.Font
		dx, dy := dialog(dst, ctx, "LEAVE THE RAID?", 380, 130)
		f.DrawShadow(dst, "You can join again with the code", dx, dy, 1, pal.Ice)
		f.DrawShadow(dst, "while the room is open.", dx, dy+16, 1, pal.Ice)
		f.DrawShadow(dst, "Y leave   N stay", dx, dy+48, 1, pal.Ash)
	}
}

// drawView draws the boss in its lair, with its health across the top.
func (s *RaidScreen) drawView(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	sp := raycast.Sprite{X: s.bx, Y: s.by, Img: s.look[int(ctx.Tick/24)%2], Size: s.size, Flash: s.flash > 4}
	s.view.Render(s.level, s.tex, s.cam, []raycast.Sprite{sp}, ctx.Tick)
	s.img.WritePixels(s.view.Img.Pix)
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	gfx.Window(dst, viewX-4, viewY-4, vw+8, vh+8)
	view := dst.SubImage(image.Rect(viewX, viewY, viewX+vw, viewY+vh)).(*ebiten.Image)
	ox, oy := viewX, viewY
	if s.shake > 0 && ctx.Shake() {
		ox += (int(ctx.Tick*7)%5 - 2) * gfx.ArtScale
		oy += (int(ctx.Tick*3)%3 - 1) * gfx.ArtScale
	}
	gfx.DrawArt(view, s.img, ox, oy)
	if s.hurt > 0 {
		gfx.FillRect(view, viewX, viewY, vw, vh, pal.Fade(pal.Red, 0.4*float64(s.hurt)/12))
	}
	if rd := s.st.Raid; rd != nil {
		f.DrawCentered(dst, s.name, viewX+vw/2, viewY+6, 1, pal.Yellow)
		bar(dst, viewX+24, viewY+24, vw-48, 10, float64(rd.HP)/float64(max(rd.MaxHP, 1)), pal.Rose, pal.Night)
		hp := fmt.Sprintf("%d / %d", max(rd.HP, 0), rd.MaxHP)
		f.DrawCentered(dst, hp, viewX+vw/2, viewY+38, 1, pal.Ice)
	}
	if s.newsT > 0 {
		sc := f.FitScale(s.news, vw-16, 1)
		gfx.FillRect(dst, viewX, viewY+vh-28, vw, 22, pal.Fade(pal.Black, 0.6))
		f.DrawCentered(dst, s.news, viewX+vw/2, viewY+vh-24, sc, s.newsCol)
	}
}

// drawSide draws the raid's room, time and latest hits beside the view.
func (s *RaidScreen) drawSide(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	const h = panelY - 8 - 4
	gfx.Window(dst, sideX, 4, sideW, h)
	x, y := sideX+10, 12
	f.DrawShadow(dst, "BOSS RAID", x, y, 1, pal.Yellow)
	f.DrawShadow(dst, "Room "+showRoomCode(s.st.Room.Code), x, y+18, 1, pal.Steel)
	if rd := s.st.Raid; rd != nil {
		f.DrawShadow(dst, plural(rd.Raiders, "raider"), x, y+34, 1, pal.Ice)
		if left := time.Until(rd.EndsAt); left > 0 {
			f.DrawShadow(dst, fmt.Sprintf("%d:%02d left", int(left.Minutes()), int(left.Seconds())%60), x, y+50, 1, pal.Ice)
		}
	}
	f.DrawShadow(dst, "Hits", x, y+76, 1, pal.Yellow)
	for i, l := range s.log {
		col := l.col
		if i < len(s.log)-1 {
			col = pal.Fade(col, 0.7)
		}
		f.DrawShadow(dst, fit(f, l.text, sideW-20, 1), x, y+94+i*16, 1, col)
	}
}

// drawPanel draws the typing panel: the word to type, or why there is
// none, and how the last answer went.
func (s *RaidScreen) drawPanel(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	x, y, w, h := 4, panelY, game.ScreenW-8, game.ScreenH-panelY-4
	cx := x + w/2
	gfx.Window(dst, x, y, w, h)
	rd := s.st.Raid
	if rd == nil {
		return
	}
	var word *link.RaidWord
	if rd.Word != nil && rd.Word.N == s.n {
		word = rd.Word
	}
	switch {
	case word != nil && word.Dodge:
		f.DrawShadow(dst, "DODGE! Translate before it strikes:", x+12, y+10, 1, pal.Orange)
		left := max(0, time.Until(word.Until).Seconds())
		limit := max(s.limit.Seconds(), left, 0.1)
		bc := pal.Lime
		if left < 2 {
			bc = pal.Rose
		}
		const bw = 180
		bx := x + w - bw - 14
		f.DrawShadow(dst, fmt.Sprintf("%.1fs", left), bx-44, y+10, 1, pal.Steel)
		bar(dst, bx, y+13, bw, 10, left/limit, bc, pal.Night)
	case word != nil:
		f.DrawShadow(dst, "ATTACK! Translate into "+s.lang.Name+":", x+12, y+10, 1, pal.Yellow)
	}
	if word != nil {
		sc := f.FitScale(word.Prompt, w-40, 2)
		f.DrawCentered(dst, word.Prompt, cx, y+28, sc, pal.White)
		drawTyped(dst, ctx, s.field.Text(), cx, y+56, w-80, 2, true)
	} else {
		msg, col := "Sending…", pal.Steel
		if g := rd.Graded; g != nil && time.Now().Before(g.StunnedUntil) {
			msg = fmt.Sprintf("Stunned! Your next word comes in %.0fs.", time.Until(g.StunnedUntil).Seconds()+0.5)
			col = pal.Rose
		} else if rd.Word == nil && rd.Graded == nil {
			msg = "Get ready…"
		}
		f.DrawCentered(dst, msg, cx, y+40, 1, col)
	}
	if g := rd.Graded; g != nil {
		line, col := gradeLine(g), tierColor[g.Tier]
		f.DrawShadow(dst, fit(f, line, w-24, 1), x+12, y+h-24, 1, col)
	}
	hint := "Enter answer   Tab accent   Esc leave"
	if s.lang.Script == words.ScriptGreek {
		hint = "Enter answer   Tab accent   F2 Greek keys   Esc leave"
	}
	f.DrawShadow(dst, hint, x+w-12-f.Width(hint, 1), y+h-24, 1, pal.Ash)
}

// gradeLine says how an answer went: its grade and, when it wasn't
// right, the answer.
func gradeLine(g *link.RaidGraded) string {
	switch {
	case g.Dodge && g.Late:
		return "Too slow: " + g.Expected
	case g.Dodge && g.Dodged:
		return "Dodged!"
	case g.Tier >= words.Correct && g.Damage > 0:
		return fmt.Sprintf("%s! %d damage", tierName(g.Tier), g.Damage)
	case g.Tier >= words.Correct:
		return tierName(g.Tier) + "!"
	}
	return tierName(g.Tier) + ": " + g.Expected
}

// tierName is a grade in words, for the typing panel.
func tierName(t words.Tier) string {
	s := strings.ToLower(t.String())
	return strings.ToUpper(s[:1]) + s[1:]
}

// drawFinale shows how the raid ended: the class's totals, and what the
// player did.
func (s *RaidScreen) drawFinale(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, s.bg, 0, 0)
	fin := s.st.Finale
	title, col, sub := "THE RAID ENDED", pal.Ice, ""
	if fin != nil {
		switch fin.Outcome {
		case link.RaidWon:
			title, col, sub = "VICTORY!", pal.Lime, fmt.Sprintf("Your class defeated the %s.", fin.Boss)
		case link.RaidEscaped:
			title, col, sub = "IT ESCAPED", pal.Orange, fmt.Sprintf("The %s got away this time.", fin.Boss)
		default:
			title, col, sub = "RAID OVER", pal.Ice, "The raid was stopped."
		}
	}
	f.DrawCentered(dst, title, cx, 8, 3, col)
	f.DrawCentered(dst, sub, cx, 44, 1, pal.Ice)

	const x, y, w, h = 70, 66, game.ScreenW - 140, 230
	gfx.Window(dst, x, y, w, h)
	row := func(i int, label, value string) {
		f.DrawShadow(dst, label, x+12, y+32+i*18, 1, pal.Steel)
		f.DrawShadow(dst, value, x+w/2-40, y+32+i*18, 1, pal.White)
	}
	if fin != nil {
		f.DrawShadow(dst, "Your class", x+12, y+10, 1, pal.Yellow)
		row(0, "Raiders", fmt.Sprint(fin.Raiders))
		row(1, "Time", fmt.Sprintf("%d:%02d", int(fin.Time.Minutes()), int(fin.Time.Seconds())%60))
		row(2, "Right answers", fmt.Sprintf("%d of %d", fin.Right, fin.Answers))
		row(3, "Damage", fmt.Sprint(fin.Damage))
	}
	if me := s.st.Summary; me != nil {
		yy := y + 32 + 5*18
		f.DrawShadow(dst, "You", x+12, yy, 1, pal.Yellow)
		f.DrawShadow(dst, fmt.Sprintf("%d of %d right", me.Right, me.Answers), x+12, yy+20, 1, pal.White)
		f.DrawShadow(dst, fmt.Sprintf("%d damage", me.Damage), x+12, yy+38, 1, pal.White)
		f.DrawShadow(dst, fmt.Sprintf("%d of %d attacks dodged", me.Dodged, me.Attacks), x+12, yy+56, 1, pal.White)
	}
	if s.st.Phase == link.PlayOff {
		f.DrawShadow(dst, "Enter continue", 8, game.ScreenH-20, 1, pal.Ash)
		return
	}
	f.DrawShadow(dst, "Enter back to the room", 8, game.ScreenH-20, 1, pal.Ash)
}
