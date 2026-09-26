package scene

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/llm"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/words"
)

// wlMode is what the Word Lists screen is doing.
type wlMode int

const (
	wlBrowse  wlMode = iota
	wlPath           // typing the path of a file to import
	wlImport         // choosing where an imported list goes
	wlDelete         // confirming a delete
	wlLeaving        // asking whether to save before leaving
	wlForge          // ordering a new list from the Word Forge
	wlForging        // waiting for the Word Forge
)

// Layout of the Word Lists screen.
const (
	wlListX, wlListY, wlListW, wlListH = 12, 40, 384, 262
	wlInfoX, wlInfoW                   = 404, game.ScreenW - 404 - 12
	wlRowH                             = 18
	wlRows                             = (wlListH - 24) / wlRowH
)

// WordLists is the screen for importing, deleting and saving word lists.
type WordLists struct {
	bg     *ebiten.Image
	shelf  *shelf
	mode   wlMode
	sel    int
	scroll int

	msg    string // the last thing that happened
	msgCol color.RGBA

	path  []rune // the path being typed
	queue []*words.List

	// The import being placed.
	imp      *words.List
	pickLang bool // the file did not say its language
	langIdx  int
	opt      int // 0 = a new list, then the rows in targets
	targets  []int

	doomed []int // rows to delete when confirmed

	// The Word Forge order.
	topic     []rune
	forgeLang int
	forgeN    int // index into llm.ForgeCounts
	forging   *llm.Job[*words.List]
	forgeAt   uint64 // tick the order went in
}

// NewWordLists creates the Word Lists screen.
func NewWordLists(ctx *game.Context) game.Scene {
	w := &WordLists{bg: backdrop(5, 1.3)}
	w.reload(ctx)
	w.say("Drop word list files on the window, or press I to import.", pal.Steel)
	if len(ctx.ListErrors) > 0 {
		w.say("Error: "+ctx.ListErrors[0], pal.Rose)
	}
	return w
}

// reload rebuilds the shelf from the lists on disk.
func (w *WordLists) reload(ctx *game.Context) {
	starters, err := game.StarterLists()
	if err != nil {
		starters = nil
	}
	user, _ := game.UserLists()
	w.shelf = newShelf(starters, user)
	w.shelf.addAssigned(ctx.Link.Lists())
	w.sel = min(w.sel, len(w.shelf.rows)-1)
}

func (w *WordLists) say(msg string, c color.RGBA) { w.msg, w.msgCol = msg, c }

// onWeb reports whether the game runs in a web browser, which has no files
// to type the path of.
func onWeb() bool { _, err := save.Dir(); return err != nil }

// Update implements game.Scene.
func (w *WordLists) Update(ctx *game.Context) error {
	w.collectDrops(ctx)
	switch w.mode {
	case wlBrowse:
		w.updateBrowse(ctx)
	case wlPath:
		w.updatePath(ctx)
	case wlImport:
		w.updateImport(ctx)
	case wlDelete:
		w.updateDelete(ctx)
	case wlLeaving:
		w.updateLeaving(ctx)
	case wlForge:
		w.updateForge(ctx)
	case wlForging:
		w.updateForging(ctx)
	}
	if w.mode == wlBrowse && len(w.queue) > 0 {
		w.startImport(ctx)
	}
	return nil
}

// collectDrops queues the word lists in files dropped on the window.
func (w *WordLists) collectDrops(ctx *game.Context) {
	fsys := ebiten.DroppedFiles()
	if fsys == nil {
		return
	}
	var bad []string
	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err == nil {
			err = w.queueFile(path.Base(p), data)
		}
		if err != nil {
			bad = append(bad, err.Error())
		}
		return nil
	})
	if len(bad) > 0 {
		ctx.Sound.Play(audio.Wrong)
		w.say(bad[0], pal.Rose)
	}
}

// queueFile reads a file to import.
func (w *WordLists) queueFile(name string, data []byte) error {
	switch strings.ToLower(path.Ext(name)) {
	case ".txt", ".tsv", ".text", "":
	default:
		return fmt.Errorf("%s is not a text file", name)
	}
	l, err := words.ParseImport(bytes.NewReader(data), name)
	if err != nil {
		return err
	}
	w.queue = append(w.queue, l)
	return nil
}

func (w *WordLists) move(ctx *game.Context, step int) {
	n := len(w.shelf.rows)
	if n == 0 {
		return
	}
	ctx.Sound.Play(audio.Blip)
	w.sel = (w.sel + step + n) % n
}

func (w *WordLists) updateBrowse(ctx *game.Context) {
	rows := w.shelf.rows
	switch {
	case input.Back():
		if w.shelf.unsaved() {
			ctx.Sound.Play(audio.Select)
			w.mode = wlLeaving
			return
		}
		w.leave(ctx)
	case input.Repeat(ebiten.KeyArrowUp):
		w.move(ctx, -1)
	case input.Repeat(ebiten.KeyArrowDown):
		w.move(ctx, 1)
	case input.Repeat(ebiten.KeyPageUp):
		w.move(ctx, -min(wlRows, w.sel))
	case input.Repeat(ebiten.KeyPageDown):
		w.move(ctx, min(wlRows, len(rows)-1-w.sel))
	case input.Pressed(ebiten.KeySpace):
		r := rows[w.sel]
		if !r.deletable() {
			ctx.Sound.Play(audio.Wrong)
			w.say(undeletable(r), pal.Tan)
			return
		}
		ctx.Sound.Play(audio.Key)
		r.marked = !r.marked
	case input.Pressed(ebiten.KeyA):
		all := true
		for _, r := range rows {
			all = all && (r.marked || !r.deletable())
		}
		for _, r := range rows {
			r.marked = !all && r.deletable()
		}
		ctx.Sound.Play(audio.Key)
	case input.Pressed(ebiten.KeyI):
		if onWeb() {
			ctx.Sound.Play(audio.Wrong)
			w.say("Drag a .txt file from your computer onto the game to import it.", pal.Yellow)
			return
		}
		ctx.Sound.Play(audio.Select)
		w.path = w.path[:0]
		w.mode = wlPath
	case input.Pressed(ebiten.KeyDelete, ebiten.KeyX, ebiten.KeyBackspace):
		w.askDelete(ctx)
	case input.Pressed(ebiten.KeyS):
		w.saveAll(ctx)
	case input.Pressed(ebiten.KeyF):
		w.openForge(ctx)
	}
}

// undeletable says why row r can't be deleted.
func undeletable(r *shelfRow) string {
	if r.assigned {
		return "Assigned lists are read-only. Unlinking keeps them as your own."
	}
	return "Starter lists can't be deleted."
}

// openForge opens the Word Forge, when an AI is set up.
func (w *WordLists) openForge(ctx *game.Context) {
	if !ctx.AI.ForgeReady() {
		ctx.Sound.Play(audio.Wrong)
		w.say("The Word Forge needs an AI with its own key: set one up in AI Helper on the title screen.", pal.Tan)
		return
	}
	ctx.Sound.Play(audio.Select)
	if r := w.shelf.rows; len(r) > 0 {
		for i, l := range words.Languages {
			if l.Code == r[w.sel].list.Language {
				w.forgeLang = i
			}
		}
	}
	w.forgeN = min(w.forgeN, len(llm.ForgeCounts)-1)
	if w.forgeN == 0 {
		w.forgeN = 1 // 20 words
	}
	w.mode = wlForge
}

// updateForge takes the Word Forge order: a topic typed in, the language
// with ←/→ and the number of words with ↑/↓.
func (w *WordLists) updateForge(ctx *game.Context) {
	nl, nc := len(words.Languages), len(llm.ForgeCounts)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		w.mode = wlBrowse
		return
	case input.Repeat(ebiten.KeyArrowLeft):
		ctx.Sound.Play(audio.Blip)
		w.forgeLang = (w.forgeLang + nl - 1) % nl
		return
	case input.Repeat(ebiten.KeyArrowRight):
		ctx.Sound.Play(audio.Blip)
		w.forgeLang = (w.forgeLang + 1) % nl
		return
	case input.Repeat(ebiten.KeyArrowUp):
		ctx.Sound.Play(audio.Blip)
		w.forgeN = (w.forgeN + 1) % nc
		return
	case input.Repeat(ebiten.KeyArrowDown):
		ctx.Sound.Play(audio.Blip)
		w.forgeN = (w.forgeN + nc - 1) % nc
		return
	case input.Repeat(ebiten.KeyBackspace) && len(w.topic) > 0:
		w.topic = w.topic[:len(w.topic)-1]
		ctx.Sound.Play(audio.Erase)
		return
	case input.Confirm():
		topic := strings.TrimSpace(string(w.topic))
		if topic == "" {
			ctx.Sound.Play(audio.Wrong)
			return
		}
		lang, n, ai := words.Languages[w.forgeLang], llm.ForgeCounts[w.forgeN], ctx.AI
		ctx.Sound.Play(audio.Select)
		w.forging = llm.Start(func() (*words.List, error) {
			c, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			return ai.Forge(c, lang, topic, n)
		})
		w.forgeAt = ctx.Tick
		w.mode = wlForging
		return
	}
	for _, r := range ctx.Input.Chars {
		if r >= ' ' && len(w.topic) < 60 {
			w.topic = append(w.topic, r)
			ctx.Sound.Play(audio.Key)
		}
	}
}

// updateForging waits for the Word Forge, then offers the new list to
// import like a dropped file.
func (w *WordLists) updateForging(ctx *game.Context) {
	if input.Back() {
		ctx.Sound.Play(audio.Back)
		w.forging = nil // it finishes in the background, unused
		w.say("Stopped waiting for the Word Forge.", pal.Tan)
		w.mode = wlBrowse
		return
	}
	if !w.forging.Done() {
		return
	}
	l, err := w.forging.Result()
	w.forging = nil
	w.mode = wlBrowse
	if err != nil {
		ctx.Sound.Play(audio.Wrong)
		w.say("The Word Forge failed: "+llm.Explain(err), pal.Rose)
		return
	}
	ctx.Sound.Play(audio.Correct)
	w.topic = w.topic[:0]
	w.queue = append(w.queue, l)
	w.say(fmt.Sprintf("Forged %q: check the words on the right, then press S to save.", l.Title), pal.Lime)
}

// askDelete asks before deleting the marked lists, or the selected one if
// none are marked.
func (w *WordLists) askDelete(ctx *game.Context) {
	w.doomed = w.doomed[:0]
	for i, r := range w.shelf.rows {
		if r.marked && r.deletable() {
			w.doomed = append(w.doomed, i)
		}
	}
	if len(w.doomed) == 0 && w.shelf.rows[w.sel].deletable() {
		w.doomed = append(w.doomed, w.sel)
	}
	if len(w.doomed) == 0 {
		ctx.Sound.Play(audio.Wrong)
		w.say("Starter lists can't be deleted. Mark your own lists with Space.", pal.Tan)
		return
	}
	ctx.Sound.Play(audio.Select)
	w.mode = wlDelete
}

func (w *WordLists) updateDelete(ctx *game.Context) {
	switch {
	case input.Pressed(ebiten.KeyY) || input.Confirm():
		d, r := w.shelf.deleteRows(w.doomed)
		w.sel = min(w.sel, len(w.shelf.rows)-1)
		ctx.Sound.Play(audio.Erase)
		msg := []string{}
		if d > 0 {
			msg = append(msg, plural(d, "list")+" deleted")
		}
		if r > 0 {
			msg = append(msg, plural(r, "starter list")+" reset")
		}
		w.say(strings.Join(msg, ", ")+". Press S to save.", pal.Yellow)
		w.mode = wlBrowse
	case input.Pressed(ebiten.KeyN) || input.Back():
		ctx.Sound.Play(audio.Back)
		w.mode = wlBrowse
	}
}

// saveAll writes every change to the words folder and reloads the game's
// lists, so new adventures use them.
func (w *WordLists) saveAll(ctx *game.Context) bool {
	if !w.shelf.unsaved() {
		ctx.Sound.Play(audio.Blip)
		w.say("Everything is already saved.", pal.Lime)
		return true
	}
	if err := w.shelf.save(); err != nil {
		ctx.Sound.Play(audio.Wrong)
		w.say("Could not save: "+err.Error(), pal.Rose)
		return false
	}
	file := w.shelf.rows[w.sel].file
	if err := ctx.LoadLists(); err != nil {
		w.say("Could not reload: "+err.Error(), pal.Rose)
	}
	w.reload(ctx)
	for i, r := range w.shelf.rows {
		if r.file == file {
			w.sel = i
		}
	}
	ctx.Sound.Play(audio.Select)
	ctx.Notify("Word lists saved")
	if len(ctx.ListErrors) > 0 {
		w.say("Saved, but: "+ctx.ListErrors[0], pal.Rose)
	} else {
		w.say("All word lists saved.", pal.Lime)
	}
	return true
}

func (w *WordLists) updateLeaving(ctx *game.Context) {
	switch {
	case input.Pressed(ebiten.KeyY) || input.Pressed(ebiten.KeyS) || input.Confirm():
		if w.saveAll(ctx) {
			w.leave(ctx)
		} else {
			w.mode = wlBrowse
		}
	case input.Pressed(ebiten.KeyN):
		ctx.Sound.Play(audio.Back)
		w.leave(ctx)
	case input.Back():
		ctx.Sound.Play(audio.Back)
		w.mode = wlBrowse
	}
}

func (w *WordLists) leave(ctx *game.Context) {
	ctx.Sound.Play(audio.Back)
	ctx.Replace(NewTitle(ctx))
}

func (w *WordLists) updatePath(ctx *game.Context) {
	for _, r := range ctx.Input.Chars {
		if r >= ' ' && len(w.path) < 400 {
			w.path = append(w.path, r)
			ctx.Sound.Play(audio.Key)
		}
	}
	switch {
	case input.Repeat(ebiten.KeyBackspace) && len(w.path) > 0:
		w.path = w.path[:len(w.path)-1]
		ctx.Sound.Play(audio.Erase)
	case input.Back():
		ctx.Sound.Play(audio.Back)
		w.mode = wlBrowse
	case input.Confirm() && len(w.path) > 0:
		p := cleanPath(string(w.path))
		data, err := os.ReadFile(p)
		if err == nil {
			err = w.queueFile(filepath.Base(p), data)
		}
		if err != nil {
			ctx.Sound.Play(audio.Wrong)
			w.say(err.Error(), pal.Rose)
			return
		}
		w.mode = wlBrowse
	}
}

// cleanPath undoes the quoting a path gets when it is copied from a file
// manager, and expands ~ to the home folder.
func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 && (p[0] == '"' || p[0] == '\'') && p[len(p)-1] == p[0] {
		p = p[1 : len(p)-1]
	}
	if rest, ok := strings.CutPrefix(p, "~"); ok && (rest == "" || rest[0] == '/' || rest[0] == '\\') {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + rest
		}
	}
	if filepath.Separator == '/' {
		p = strings.ReplaceAll(p, `\ `, " ") // shell-escaped spaces
	}
	return p
}

// startImport opens the next queued import.
func (w *WordLists) startImport(ctx *game.Context) {
	w.imp, w.queue = w.queue[0], w.queue[1:]
	w.pickLang = w.imp.Language == ""
	w.langIdx = 0
	if !w.pickLang {
		for i, l := range words.Languages {
			if l.Code == w.imp.Language {
				w.langIdx = i
			}
		}
	} else if r := w.shelf.rows; len(r) > 0 {
		// Guess the language of the list the user was looking at.
		for i, l := range words.Languages {
			if l.Code == r[w.sel].list.Language {
				w.langIdx = i
			}
		}
	}
	w.setImportLang()
	ctx.Sound.Play(audio.Select)
	w.mode = wlImport
}

func (w *WordLists) setImportLang() {
	w.targets = w.shelf.targets(words.Languages[w.langIdx].Code)
	w.opt = 0
}

func (w *WordLists) updateImport(ctx *game.Context) {
	n := 1 + len(w.targets)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		w.say(fmt.Sprintf("Skipped %q.", w.imp.Title), pal.Steel)
		w.imp = nil
		w.mode = wlBrowse
	case input.Repeat(ebiten.KeyArrowUp):
		ctx.Sound.Play(audio.Blip)
		w.opt = (w.opt + n - 1) % n
	case input.Repeat(ebiten.KeyArrowDown):
		ctx.Sound.Play(audio.Blip)
		w.opt = (w.opt + 1) % n
	case w.pickLang && input.Pressed(ebiten.KeyArrowLeft):
		ctx.Sound.Play(audio.Blip)
		w.langIdx = (w.langIdx + len(words.Languages) - 1) % len(words.Languages)
		w.setImportLang()
	case w.pickLang && input.Pressed(ebiten.KeyArrowRight):
		ctx.Sound.Play(audio.Blip)
		w.langIdx = (w.langIdx + 1) % len(words.Languages)
		w.setImportLang()
	case input.Confirm():
		imp := w.imp
		imp.Language = words.Languages[w.langIdx].Code
		if w.opt == 0 {
			w.sel = w.shelf.create(imp)
			w.say(fmt.Sprintf("New list %q with %s. Press S to save.", imp.Title, plural(len(imp.Entries), "word")), pal.Yellow)
		} else {
			w.sel = w.targets[w.opt-1]
			added := w.shelf.add(w.sel, imp)
			w.say(fmt.Sprintf("Added %s to %q. Press S to save.", plural(added, "new word"), w.shelf.rows[w.sel].list.Title), pal.Yellow)
		}
		ctx.Sound.Play(audio.Correct)
		w.imp = nil
		w.mode = wlBrowse
	}
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// fit shortens s with an ellipsis so it is at most width pixels wide.
func fit(f *gfx.Font, s string, width, scale int) string {
	if f.Width(s, scale) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && f.Width(string(r)+"…", scale) > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// Draw implements game.Scene.
func (w *WordLists) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	gfx.DrawArt(dst, w.bg, 0, 0)
	f.DrawOutline(dst, "WORD LISTS", game.ScreenW/2-f.Width("WORD LISTS", 2)/2, 6, 2, pal.Yellow, pal.Black)
	w.drawList(dst, ctx)
	w.drawInfo(dst, ctx)

	f.DrawShadow(dst, fit(f, w.msg, game.ScreenW-24, 1), 12, 306, 1, w.msgCol)
	help1, help2 := "↑/↓ choose   Space mark   A mark all   X delete   F Word Forge", "I import (or drop files)   S save all   Esc back"
	if onWeb() {
		help2 = "Drop .txt files on the page to import   S save all   Esc back"
	}
	f.DrawShadow(dst, help1, 12, 324, 1, pal.Ash)
	f.DrawShadow(dst, help2, 12, 340, 1, pal.Ash)

	switch w.mode {
	case wlPath:
		w.drawPath(dst, ctx)
	case wlImport:
		w.drawImport(dst, ctx)
	case wlDelete:
		w.drawDelete(dst, ctx)
	case wlLeaving:
		w.drawLeaving(dst, ctx)
	case wlForge, wlForging:
		w.drawForge(dst, ctx)
	}
}

func (w *WordLists) drawForge(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	const dw = 520
	x, y := dialog(dst, ctx, "WORD FORGE", dw, 196)
	if w.mode == wlForging {
		dots := strings.Repeat(".", int(ctx.Tick/20%4))
		f.DrawShadow(dst, "The forge is hot"+dots, x, y+10, 2, pal.Orange)
		secs := int(ctx.Tick-w.forgeAt) / ebiten.TPS()
		f.DrawShadow(dst, fmt.Sprintf("Making the words, then checking them (%ds).", secs), x, y+56, 1, pal.Ice)
		f.DrawShadow(dst, "Esc stop waiting", x, y+106, 1, pal.Ash)
		return
	}
	f.DrawShadow(dst, "An AI makes a new word list on any topic.", x, y, 1, pal.Ice)
	f.DrawShadow(dst, "Language:", x, y+24, 1, pal.Tan)
	f.DrawShadow(dst, "◄ "+words.Languages[w.forgeLang].Name+" ►", x+100, y+24, 1, pal.Yellow)
	f.DrawShadow(dst, "Words:", x, y+42, 1, pal.Tan)
	f.DrawShadow(dst, fmt.Sprintf("▲ %d ▼", llm.ForgeCounts[w.forgeN]), x+100, y+42, 1, pal.Yellow)
	f.DrawShadow(dst, "Topic:", x, y+60, 1, pal.Tan)
	text := string(w.topic)
	if text == "" {
		f.DrawShadow(dst, "such as: at the market, sports, the weather", x+100, y+60, 1, pal.Ash)
	} else {
		text = fit(f, text, dw-150, 1)
		f.DrawShadow(dst, text, x+100, y+60, 1, pal.White)
	}
	if ctx.Tick/16%2 == 0 {
		gfx.FillRect(dst, x+100+f.Width(text, 1)+1, y+62, 6, 13, pal.Yellow)
	}
	gfx.FillRect(dst, x+100, y+78, dw-140, 1, pal.Indigo)
	f.DrawShadow(dst, "A second pass checks each translation. Look the list over", x, y+88, 1, pal.Steel)
	f.DrawShadow(dst, "before playing, and fix any word in its .txt file.", x, y+104, 1, pal.Steel)
	f.DrawShadow(dst, "←/→ language   ↑/↓ words   Enter forge   Esc cancel", x, y+126, 1, pal.Ash)
}

func (w *WordLists) drawList(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	x, y := wlListX, wlListY
	gfx.Window(dst, x, y, wlListW, wlListH)
	rows := w.shelf.rows
	w.scroll = max(0, min(w.scroll, w.sel, len(rows)-wlRows))
	if w.sel >= w.scroll+wlRows {
		w.scroll = w.sel - wlRows + 1
	}
	for i := w.scroll; i < min(len(rows), w.scroll+wlRows); i++ {
		r := rows[i]
		ry := y + 12 + (i-w.scroll)*wlRowH
		col := pal.Steel
		if i == w.sel {
			gfx.FillRect(dst, x+6, ry-1, wlListW-12, wlRowH, pal.Indigo)
			col = pal.White
		}
		switch {
		case r.assigned:
			f.Draw(dst, "◆", x+12, ry, 1, pal.Sky)
		case r.marked:
			f.Draw(dst, "■", x+12, ry, 1, pal.Rose)
		case r.deletable():
			f.Draw(dst, "□", x+12, ry, 1, pal.Ash)
		}
		right := fmt.Sprintf("%-3s %4d", r.list.Language, len(r.list.Entries))
		rw := f.Width(right, 1)
		title := r.list.Title
		if r.dirty {
			f.Draw(dst, "*", x+wlListW-rw-24, ry, 1, pal.Yellow)
		}
		f.DrawShadow(dst, fit(f, title, wlListW-rw-68, 1), x+30, ry, 1, col)
		f.DrawShadow(dst, right, x+wlListW-rw-12, ry, 1, pal.Ash)
	}
	// Scroll arrows.
	if w.scroll > 0 {
		f.Draw(dst, "▲", x+wlListW/2-4, y+1, 1, pal.Tan)
	}
	if w.scroll+wlRows < len(rows) {
		f.Draw(dst, "▼", x+wlListW/2-4, y+wlListH-15, 1, pal.Tan)
	}
}

func (w *WordLists) drawInfo(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	x, y := wlInfoX, wlListY
	gfx.Window(dst, x, y, wlInfoW, wlListH)
	if len(w.shelf.rows) == 0 {
		return
	}
	r := w.shelf.rows[w.sel]
	l := r.list
	tx, tw := x+12, wlInfoW-24
	line := func(s string, c color.RGBA) {
		f.DrawShadow(dst, fit(f, s, tw, 1), tx, y, 1, c)
		y += 17
	}
	y += 10
	line(l.Title, pal.Yellow)
	name := l.Language
	if lang, ok := words.Lookup(l.Language); ok {
		name = lang.Name
	}
	groups := map[string]bool{}
	for _, e := range l.Entries {
		if e.Tag != "" {
			groups[e.Tag] = true
		}
	}
	line(name, pal.Ice)
	info := plural(len(l.Entries), "word")
	if len(groups) > 0 {
		info += " · " + plural(len(groups), "group")
	}
	line(info, pal.Ice)
	switch {
	case r.assigned:
		line("◆ Assigned on the website", pal.Sky)
	case r.own && r.starter != nil:
		line("Starter list + your words", pal.Tan)
	case r.own:
		line("Your own list", pal.Tan)
	default:
		line("Starter list", pal.Steel)
	}
	switch {
	case r.assigned:
		line("Read-only: it updates when the game syncs", pal.Ash)
	case r.dirty:
		line("* Not saved yet", pal.Yellow)
	case r.own:
		line("Saved", pal.Lime)
	default:
		line("Built into the game", pal.Ash)
	}
	y += 4
	gfx.FillRect(dst, tx, y, tw, 1, pal.Indigo)
	y += 6
	for i, e := range l.Entries {
		if y > wlListY+wlListH-28 {
			line(fmt.Sprintf("… and %d more", len(l.Entries)-i), pal.Ash)
			break
		}
		line(e.Prompt+" = "+strings.Join(e.Answers, " | "), pal.Steel)
	}
}

// dialog draws a centred window with a title and returns its inside corner.
func dialog(dst *ebiten.Image, ctx *game.Context, title string, w, h int) (x, y int) {
	gfx.FillRect(dst, 0, 0, game.ScreenW, game.ScreenH, pal.Fade(pal.Black, 0.5))
	x, y = game.ScreenW/2-w/2, (game.ScreenH-h)/2
	gfx.Window(dst, x, y, w, h)
	ctx.Font.DrawCentered(dst, title, game.ScreenW/2, y+10, 2, pal.Yellow)
	return x + 20, y + 48
}

func (w *WordLists) drawPath(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	const dw = 540
	x, y := dialog(dst, ctx, "IMPORT A FILE", dw, 150)
	f.DrawShadow(dst, "Type the path of a .txt word list, or drop the file", x, y, 1, pal.Ice)
	f.DrawShadow(dst, "onto this window.", x, y+16, 1, pal.Ice)
	text := string(w.path)
	for len(text) > 0 && f.Width(text+"_", 1) > dw-40 {
		_, n := utf8.DecodeRuneInString(text)
		text = text[n:] // keep the end of a long path in view
	}
	f.DrawShadow(dst, text, x, y+46, 1, pal.White)
	gfx.FillRect(dst, x, y+64, dw-40, 1, pal.Indigo)
	if ctx.Tick/16%2 == 0 {
		gfx.FillRect(dst, x+f.Width(text, 1)+1, y+48, 6, 13, pal.Yellow)
	}
	f.DrawShadow(dst, "Enter import   Esc cancel", x, y+74, 1, pal.Ash)
}

func (w *WordLists) drawImport(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	const dw, maxOpts = 460, 7
	n := 1 + len(w.targets)
	shown := min(n, maxOpts)
	x, y := dialog(dst, ctx, "IMPORT", dw, 150+shown*18)
	iw := dw - 40
	f.DrawShadow(dst, fit(f, fmt.Sprintf("“%s” · %s", w.imp.Title, plural(len(w.imp.Entries), "word")), iw, 1), x, y, 1, pal.White)
	lang := words.Languages[w.langIdx].Name
	if w.pickLang {
		f.DrawShadow(dst, "Language:", x, y+20, 1, pal.Tan)
		f.DrawShadow(dst, "◄ "+lang+" ►", x+80, y+20, 1, pal.Yellow)
	} else {
		f.DrawShadow(dst, "Language: "+lang, x, y+20, 1, pal.Tan)
	}
	f.DrawShadow(dst, "Put the words in:", x, y+44, 1, pal.Ice)
	first := max(0, min(w.opt-shown/2, n-shown))
	for i := first; i < first+shown; i++ {
		oy := y + 64 + (i-first)*18
		label := "A new list"
		if i > 0 {
			label = "Add to " + w.shelf.rows[w.targets[i-1]].list.Title
		}
		col := pal.Steel
		if i == w.opt {
			col = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+4, oy, 1, pal.Yellow)
			}
		}
		f.DrawShadow(dst, fit(f, label, iw-24, 1), x+22, oy, 1, col)
	}
	hint := "↑/↓ choose   Enter OK   Esc skip"
	if w.pickLang {
		hint = "←/→ language   " + hint
	}
	if len(w.queue) > 0 {
		hint += fmt.Sprintf("   (%d more)", len(w.queue))
	}
	f.DrawShadow(dst, hint, x, y+72+shown*18, 1, pal.Ash)
}

func (w *WordLists) drawDelete(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	const dw, maxNames = 420, 6
	shown := min(len(w.doomed), maxNames)
	x, y := dialog(dst, ctx, "DELETE?", dw, 118+shown*18)
	for i, idx := range w.doomed[:shown] {
		r := w.shelf.rows[idx]
		label := r.list.Title
		if r.starter != nil {
			label += " (back to the starter words)"
		}
		f.DrawShadow(dst, fit(f, "• "+label, dw-40, 1), x, y+i*18, 1, pal.White)
	}
	ny := y + shown*18 + 6
	if len(w.doomed) > maxNames {
		f.DrawShadow(dst, fmt.Sprintf("… and %d more", len(w.doomed)-maxNames), x, ny-4, 1, pal.Ash)
		ny += 14
	}
	f.DrawShadow(dst, "Y / Enter delete   N / Esc keep", x, ny+14, 1, pal.Ash)
}

func (w *WordLists) drawLeaving(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	x, y := dialog(dst, ctx, "SAVE CHANGES?", 380, 110)
	f.DrawShadow(dst, "Your word lists have unsaved changes.", x, y, 1, pal.Ice)
	f.DrawShadow(dst, "Y save   N throw away   Esc stay", x, y+28, 1, pal.Ash)
}
