package scene

import (
	"fmt"
	"image/color"
	"io/fs"
	"path"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/maps"
	"github.com/halpworld/halpwords/pkg/words"
)

// questsDir is the folder, inside the user's folder, that keeps the quests
// dropped on the window.
const questsDir = "quests"

// questFile is a quest and where it came from.
type questFile struct {
	q       *maps.Quest
	file    string // the file in questsDir, or "" for a built-in quest
	builtIn bool
}

// droppedFile is a map or quest file dropped on the window.
type droppedFile struct {
	name string
	data []byte
}

// droppedQuests returns the map and quest files dropped on the window this
// tick. Other files are left for other screens.
func droppedQuests() []droppedFile {
	fsys := ebiten.DroppedFiles()
	if fsys == nil {
		return nil
	}
	var out []droppedFile
	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !maps.IsFile(p) {
			return nil
		}
		if data, err := fs.ReadFile(fsys, p); err == nil {
			out = append(out, droppedFile{path.Base(p), data})
		}
		return nil
	})
	return out
}

// builtInQuests returns the quests that come with the game.
func builtInQuests() []questFile {
	var out []questFile
	names, _ := fs.Glob(assets.Quests, "quests/*.hwquest")
	for _, name := range names {
		data, err := fs.ReadFile(assets.Quests, name)
		if err != nil {
			continue
		}
		if q, err := maps.Load(name, data); err == nil && len(q.Check(nil)) == 0 {
			out = append(out, questFile{q: q, builtIn: true})
		}
	}
	return out
}

// savedQuests returns the quests the player has added, skipping any that
// no longer load.
func savedQuests() []questFile {
	names, _ := save.List(questsDir)
	var out []questFile
	for _, name := range names {
		data, err := save.Read(questsDir + "/" + name)
		if err != nil {
			continue
		}
		if q, err := maps.Load(name, data); err == nil && len(q.Check(nil)) == 0 {
			out = append(out, questFile{q: q, file: name})
		}
	}
	return out
}

// questFileName is the file a quest called title is kept in.
func questFileName(title string) string {
	return strings.TrimSuffix(words.FileName(title), ".txt") + "." + maps.QuestFormat
}

// Quests picks a hand-made quest to play: one built into the game, or one
// dropped on the window as a .hwquest or .hwmap file.
type Quests struct {
	bg     *ebiten.Image
	list   []questFile
	sel    int
	msg    string
	msgCol color.RGBA
}

// NewQuests creates the quest picker, adding any dropped files.
func NewQuests(ctx *game.Context, dropped ...droppedFile) game.Scene {
	s := &Quests{bg: backdrop(3, 1.4)}
	s.list = append(builtInQuests(), savedQuests()...)
	s.add(ctx, dropped)
	return s
}

// add checks dropped files and keeps the good ones.
func (s *Quests) add(ctx *game.Context, dropped []droppedFile) {
	for _, d := range dropped {
		if err := s.addFile(d); err != nil {
			ctx.Sound.Play(audio.Wrong)
			s.msg, s.msgCol = err.Error(), pal.Rose
			continue
		}
		ctx.Sound.Play(audio.Select)
		s.msg, s.msgCol = "Added “"+s.list[s.sel].q.Title+"”.", pal.Lime
	}
}

// addFile reads a dropped file, checks it and keeps it in the quests
// folder. A quest with the same title as one there replaces it.
func (s *Quests) addFile(d droppedFile) error {
	q, err := maps.Load(d.name, d.data)
	if err != nil {
		return fmt.Errorf("%s: %v", d.name, err)
	}
	if ps := q.Check(nil); len(ps) > 0 {
		return fmt.Errorf("%s: %s", d.name, ps[0])
	}
	qf := questFile{q: q, file: questFileName(q.Title)}
	if data, err := q.Encode(); err == nil {
		// Without a folder (as in a web browser), the quest is kept
		// until the game closes.
		save.Write(questsDir+"/"+qf.file, data)
	}
	for i, o := range s.list {
		if !o.builtIn && o.file == qf.file {
			s.list[i], s.sel = qf, i
			return nil
		}
	}
	s.list = append(s.list, qf)
	s.sel = len(s.list) - 1
	return nil
}

// Update implements game.Scene.
func (s *Quests) Update(ctx *game.Context) error {
	s.add(ctx, droppedQuests())
	n := len(s.list)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewNewGame(ctx))
	case n == 0:
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		s.sel = (s.sel + 1) % n
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		s.start(ctx)
	}
	return nil
}

// start sets off on the chosen quest: to the language picker when the
// quest works in any language, or straight to the class.
func (s *Quests) start(ctx *game.Context) {
	next, msg := questStart(ctx, s.list[s.sel].q)
	if next == nil {
		ctx.Sound.Play(audio.Wrong)
		s.msg, s.msgCol = msg, pal.Rose
		return
	}
	ctx.Sound.Play(audio.Select)
	ctx.Replace(next)
}

// questStart returns the scene that sets off on quest q: the language
// picker when the quest works in any language, or the class. When the
// quest can't start, it says why instead.
func questStart(ctx *game.Context, q *maps.Quest) (game.Scene, string) {
	setup := runSetup{mode: compete.Adventure, quest: q}
	if q.Language == "" {
		return NewAdventure(ctx, setup), ""
	}
	lang, _ := words.Lookup(q.Language)
	if len(ctx.ListsFor(lang.Code)) == 0 {
		return nil, "This quest is in " + lang.Name + ", and there are no " + lang.Name + " word lists."
	}
	return NewClassPick(lang, setup), ""
}

// questRows is how many quests the picker shows at once.
const questRows = 7

// Draw implements game.Scene.
func (s *Quests) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, s.bg, 0, 0)
	f.DrawCentered(dst, "Quests", cx, 24, 3, pal.Yellow)
	f.DrawCentered(dst, "Hand-made dungeons, floor by floor. Drop a .hwquest or .hwmap", cx, 72, 1, pal.Tan)
	f.DrawCentered(dst, "file on the window to add one.", cx, 88, 1, pal.Tan)

	const w, rowH = 480, 26
	x, y := cx-w/2, 112
	gfx.Window(dst, x, y, w, rowH*questRows+16)
	first := min(max(0, s.sel-questRows+1), max(0, len(s.list)-questRows))
	for i := first; i < min(len(s.list), first+questRows); i++ {
		qf := s.list[i]
		ry := y + 10 + (i-first)*rowH
		col := pal.Steel
		if i == s.sel {
			col = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+14, ry, 2, pal.Yellow)
			}
		}
		f.DrawShadow(dst, qf.q.Title, x+40, ry, 2, col)
		about := questAbout(qf)
		f.DrawShadow(dst, about, x+w-16-f.Width(about, 1), ry+8, 1, pal.Ash)
	}
	if len(s.list) == 0 {
		f.DrawCentered(dst, "No quests yet.", cx, y+40, 1, pal.Ash)
	}
	if s.msg != "" {
		for i, line := range wrap(f, s.msg, game.ScreenW-40) {
			f.DrawCentered(dst, line, cx, y+rowH*questRows+26+i*16, 1, s.msgCol)
		}
	}
	f.DrawShadow(dst, "↑/↓ choose   Enter play   Esc back", 8, game.ScreenH-20, 1, pal.Ash)
}

// questAbout describes a quest in a few words, such as "3 floors ·
// French".
func questAbout(qf questFile) string {
	floors := "1 floor"
	if n := len(qf.q.Maps); n != 1 {
		floors = fmt.Sprintf("%d floors", n)
	}
	lang := "any language"
	if l, ok := words.Lookup(qf.q.Language); ok {
		lang = l.Name
	}
	return floors + " · " + lang
}

// QuestPage shows a quest's introduction or ending, then moves on.
type QuestPage struct {
	bg      *ebiten.Image
	heading string
	title   string
	text    []string // paragraphs
	prompt  string
	next    func(*game.Context) game.Scene
}

// Update implements game.Scene.
func (p *QuestPage) Update(ctx *game.Context) error {
	if input.Confirm() || input.Pressed(ebiten.KeySpace) || input.Back() {
		ctx.Sound.Play(audio.Select)
		ctx.Replace(p.next(ctx))
	}
	return nil
}

// Draw implements game.Scene.
func (p *QuestPage) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, p.bg, 0, 0)
	f.DrawCentered(dst, p.heading, cx, 24, 3, pal.Yellow)
	f.DrawCentered(dst, p.title, cx, 72, 2, pal.Tan)
	const w = 520
	var lines []string
	for i, para := range p.text {
		if i > 0 {
			lines = append(lines, "")
		}
		for _, l := range strings.Split(para, "\n") {
			lines = append(lines, wrap(f, l, w-32)...)
		}
	}
	lines = lines[:min(len(lines), 12)]
	y := 108
	gfx.Window(dst, cx-w/2, y, w, len(lines)*16+24)
	for i, l := range lines {
		f.DrawCentered(dst, l, cx, y+12+i*16, 1, pal.Ice)
	}
	f.DrawShadow(dst, p.prompt, 8, game.ScreenH-20, 1, pal.Ash)
}

// questIntro shows the quest's introduction, if it has one, before the
// first floor.
func questIntro(r *run) game.Scene {
	c := newCrawl(r)
	if strings.TrimSpace(r.quest.Intro) == "" {
		return c
	}
	return &QuestPage{
		bg: backdrop(4, 1.4), heading: "A Quest", title: r.quest.Title,
		text: []string{r.quest.Intro}, prompt: "Enter begin",
		next: func(*game.Context) game.Scene { return c },
	}
}

// finishQuest ends a quest when the hero takes the last map's stairs. The
// adventure is over, so its save goes; what the player learned stays.
func (c *Crawl) finishQuest(ctx *game.Context) {
	r := c.run
	r.remember()
	if r.onDisk {
		save.Remove(saveName)
		r.onDisk = false
	}
	ctx.Sound.Play(audio.LevelUp)
	ctx.Replace(questEnd(r))
}

// questEnd is the page that ends run r's quest.
func questEnd(r *run) *QuestPage {
	ending := r.quest.Ending
	if strings.TrimSpace(ending) == "" {
		ending = "You reached the end of the quest."
	}
	h := r.hero
	return &QuestPage{
		bg: backdrop(9, 1.2), heading: "Quest complete!", title: r.quest.Title,
		text:   []string{ending, fmt.Sprintf("Your hero: a level %d %s with %d gold.", h.Level, h.Class, h.Gold)},
		prompt: "Enter back to the title",
		next:   NewTitle,
	}
}
