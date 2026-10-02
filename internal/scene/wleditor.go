package scene

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/words"
)

// What the quick list editor is asking, if anything.
const (
	edAskNone  = iota
	edAskLeave // save before leaving?
	edAskSkip  // save, leaving the lines that are not words out?
)

// The skipped-lines window.
const (
	pvW        = 580
	pvRowsShow = 10
	pvRowH     = 15
)

// openEditor opens the quick list editor on row r, or on a new list when r
// is nil. Starter lists and assigned lists can't be edited.
func (w *WordLists) openEditor(ctx *game.Context, r *shelfRow) {
	var e *listEditor
	switch {
	case r == nil:
		lang := 0
		if rows := w.shelf.rows; len(rows) > 0 {
			for i, l := range words.Languages {
				if l.Code == rows[w.sel].list.Language {
					lang = i
				}
			}
		}
		e = newListEditor(lang)
	case r.assigned:
		ctx.Sound.Play(audio.Wrong)
		w.say("Assigned lists can't be edited here. Unlink it to keep a copy of your own.", pal.Tan)
		return
	case !r.own:
		ctx.Sound.Play(audio.Wrong)
		w.say("Starter lists can't be edited. Press N to type your own list, or import words to add to it.", pal.Tan)
		return
	case r.broken != nil:
		raw, err := save.Read(game.WordsDir + "/" + r.file)
		if err != nil {
			ctx.Sound.Play(audio.Wrong)
			w.say("Could not open the file: "+err.Error(), pal.Rose)
			return
		}
		e = openListEditor(r.list, string(raw))
	default:
		e = openListEditor(r.list, "")
	}
	ctx.Sound.Play(audio.Select)
	w.ed, w.edRow, w.edAsk, w.edNote = e, r, edAskNone, ""
	w.mode = wlEdit
}

// openProblems shows the lines of a file in the words folder that the game
// can't read.
func (w *WordLists) openProblems(ctx *game.Context, r *shelfRow) {
	if r.broken == nil {
		ctx.Sound.Play(audio.Wrong)
		w.say("Nothing is wrong with this list.", pal.Lime)
		return
	}
	ctx.Sound.Play(audio.Select)
	w.pvProbs, w.pvScroll, w.pvRow = r.broken, 0, r
	w.mode = wlPreview
}

type pvLine struct {
	s      string
	col    color.RGBA
	indent int
}

// pvLines lays out the skipped lines: each line's number and text, then
// why, wrapped.
func (w *WordLists) pvLines(ctx *game.Context) []pvLine {
	f := ctx.Font
	iw := pvW - 40
	var out []pvLine
	for _, p := range w.pvProbs {
		ind := 0
		if p.Line > 0 {
			out = append(out, pvLine{s: fit(f, fmt.Sprintf("line %d: %s", p.Line, p.Text), iw, 1), col: pal.White})
			ind = 16
		}
		for _, l := range wrapText(f, p.Reason, 1, iw-ind) {
			out = append(out, pvLine{s: l, col: pal.Rose, indent: ind})
		}
	}
	return out
}

func (w *WordLists) updatePreview(ctx *game.Context) {
	n := len(w.pvLines(ctx))
	last := max(0, n-pvRowsShow)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		if w.pvRow == nil {
			w.say(fmt.Sprintf("Skipped %q.", w.imp.Title), pal.Steel)
			w.imp = nil
		}
		w.mode = wlBrowse
	case input.Repeat(ebiten.KeyArrowUp):
		w.pvScroll = max(0, w.pvScroll-1)
	case input.Repeat(ebiten.KeyArrowDown):
		w.pvScroll = min(last, w.pvScroll+1)
	case input.Repeat(ebiten.KeyPageUp):
		w.pvScroll = max(0, w.pvScroll-pvRowsShow)
	case input.Repeat(ebiten.KeyPageDown):
		w.pvScroll = min(last, w.pvScroll+pvRowsShow)
	case input.Confirm():
		switch {
		case w.pvRow != nil:
			w.openEditor(ctx, w.pvRow)
		case len(w.imp.Entries) == 0:
			ctx.Sound.Play(audio.Wrong)
		default:
			w.beginPlacement(ctx)
		}
	}
}

func (w *WordLists) drawPreview(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	lines := w.pvLines(ctx)
	const h = 48 + 24 + pvRowsShow*pvRowH + 36
	title := "CHECK THE FILE"
	if w.pvRow != nil {
		title = "LINES THE GAME CAN'T READ"
	}
	x, y := dialog(dst, ctx, title, pvW, h)
	iw := pvW - 40
	var head, hint string
	if w.pvRow != nil {
		head = fmt.Sprintf("“%s”: %s skipped", w.pvRow.file, plural(len(w.pvProbs), "line"))
		hint = "↑/↓ scroll   Enter fix it in the editor   Esc close"
	} else {
		head = fmt.Sprintf("“%s”: %s found, %s skipped", w.imp.Title, plural(len(w.imp.Entries), "word"), plural(len(w.pvProbs), "line"))
		hint = "↑/↓ scroll   Enter import the good lines   Esc cancel"
		if len(w.imp.Entries) == 0 {
			hint = "↑/↓ scroll   No words to import   Esc cancel"
		}
	}
	f.DrawShadow(dst, fit(f, head, iw, 1), x, y, 1, pal.White)
	top := y + 24
	last := max(0, len(lines)-pvRowsShow)
	w.pvScroll = min(w.pvScroll, last)
	for i := w.pvScroll; i < min(len(lines), w.pvScroll+pvRowsShow); i++ {
		l := lines[i]
		f.DrawShadow(dst, l.s, x+l.indent, top+(i-w.pvScroll)*pvRowH, 1, l.col)
	}
	if w.pvScroll > 0 {
		f.Draw(dst, "▲", x+iw-10, top-14, 1, pal.Tan)
	}
	if w.pvScroll < last {
		f.Draw(dst, "▼", x+iw-10, top+pvRowsShow*pvRowH, 1, pal.Tan)
	}
	f.DrawShadow(dst, hint, x, top+pvRowsShow*pvRowH+14, 1, pal.Ash)
}

// ctrlS reports whether Ctrl+S (Cmd+S on a Mac) was pressed.
func ctrlS() bool {
	return input.Pressed(ebiten.KeyS) && (input.Held(ebiten.KeyControl) || input.Held(ebiten.KeyMeta))
}

func (w *WordLists) updateEdit(ctx *game.Context) {
	e := w.ed
	switch w.edAsk {
	case edAskLeave:
		switch {
		case input.Pressed(ebiten.KeyY) || input.Confirm():
			w.trySaveEdit(ctx, false)
		case input.Pressed(ebiten.KeyN):
			ctx.Sound.Play(audio.Back)
			w.closeEdit()
		case input.Back():
			ctx.Sound.Play(audio.Back)
			w.edAsk = edAskNone
		}
		return
	case edAskSkip:
		switch {
		case input.Pressed(ebiten.KeyY) || input.Confirm():
			w.trySaveEdit(ctx, true)
		case input.Back() || input.Pressed(ebiten.KeyN):
			ctx.Sound.Play(audio.Back)
			w.edAsk = edAskNone
		}
		return
	}
	if len(ctx.Input.Chars) > 0 {
		w.edNote = ""
	}
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		if e.changed() {
			w.edAsk = edAskLeave
		} else {
			w.closeEdit()
		}
		return
	case ctrlS():
		w.trySaveEdit(ctx, false)
		return
	case input.Repeat(ebiten.KeyArrowUp):
		if e.Up() {
			ctx.Sound.Play(audio.Blip)
		}
		return
	case input.Repeat(ebiten.KeyArrowDown):
		if e.Down() {
			ctx.Sound.Play(audio.Blip)
		}
		return
	case e.focus == edLang && input.Repeat(ebiten.KeyArrowLeft):
		e.Language(-1)
		ctx.Sound.Play(audio.Blip)
		return
	case e.focus == edLang && input.Repeat(ebiten.KeyArrowRight):
		e.Language(1)
		ctx.Sound.Play(audio.Blip)
		return
	case input.Confirm():
		if e.Enter() {
			ctx.Sound.Play(audio.Blip)
		}
		return
	case input.Repeat(ebiten.KeyBackspace):
		if e.Backspace() {
			ctx.Sound.Play(audio.Erase)
		}
		return
	case input.Pressed(ebiten.KeyTab):
		if e.Accent() {
			ctx.Sound.Play(audio.Accent)
		}
		return
	case input.Pressed(ebiten.KeyF2):
		if e.language().Script == words.ScriptGreek {
			e.greekSwap = !e.greekSwap
			ctx.Sound.Play(audio.Blip)
		}
		return
	}
	if input.Held(ebiten.KeyControl) || input.Held(ebiten.KeyMeta) {
		return // a shortcut, not letters
	}
	for _, r := range ctx.Input.Chars {
		if e.Type(r) {
			ctx.Sound.Play(audio.Key)
		}
	}
}

func (w *WordLists) closeEdit() {
	w.ed, w.edRow, w.edAsk = nil, nil, edAskNone
	w.mode = wlBrowse
}

// trySaveEdit saves the list being typed to the words folder, after asking
// about lines that would be left out unless leaveOut is set.
func (w *WordLists) trySaveEdit(ctx *game.Context, leaveOut bool) {
	e := w.ed
	var orig *words.List
	if w.edRow != nil {
		orig = w.edRow.list
	}
	l, skipped, err := e.Result(orig)
	if err != nil {
		ctx.Sound.Play(audio.Wrong)
		w.edAsk, w.edNote = edAskNone, strings.ToUpper(err.Error()[:1])+err.Error()[1:]+"."
		return
	}
	if skipped > 0 && !leaveOut {
		ctx.Sound.Play(audio.Select)
		w.edAsk = edAskSkip
		return
	}
	if r := w.edRow; r != nil {
		r.list, r.broken, r.own, r.dirty = l, nil, true, true
		w.sel = rowIndex(w.shelf, r)
	} else {
		w.sel = w.shelf.create(l)
	}
	if !w.saveAll(ctx) {
		w.edAsk, w.edNote = edAskNone, w.msg
		return
	}
	w.say(fmt.Sprintf("Saved %q with %s.", l.Title, plural(len(l.Entries), "word")), pal.Lime)
	w.closeEdit()
}

func rowIndex(s *shelf, r *shelfRow) int {
	for i, x := range s.rows {
		if x == r {
			return i
		}
	}
	return 0
}

// The editor's layout.
const (
	edX, edY, edW, edH = 12, 34, 392, 262
	edHintX            = 412
	edHintW            = game.ScreenW - edHintX - 12
	edBodyY            = edY + 52
	edRowH             = 15
	edRows             = (edY + edH - 8 - edBodyY) / edRowH
)

var edHint = []struct {
	s   string
	col color.RGBA
}{
	{"One word on each line:", pal.Ice},
	{"english = answer", pal.Yellow},
	{"", pal.Ice},
	{"Other right answers go", pal.Ice},
	{"after a |", pal.Ice},
	{"friend = l'ami | l'amie", pal.Yellow},
	{"", pal.Ice},
	{"# a note, not a word", pal.Tan},
	{"## a group of words", pal.Tan},
	{"", pal.Ice},
	{"Tab adds an accent to", pal.Steel},
	{"the letter before it", pal.Steel},
}

func (w *WordLists) drawEdit(dst *ebiten.Image, ctx *game.Context) {
	f, e := ctx.Font, w.ed
	gfx.DrawArt(dst, w.bg, 0, 0)
	head := "NEW WORD LIST"
	if w.edRow != nil {
		head = "EDIT WORD LIST"
	}
	f.DrawOutline(dst, head, game.ScreenW/2-f.Width(head, 2)/2, 6, 2, pal.Yellow, pal.Black)
	gfx.Window(dst, edX, edY, edW, edH)
	gfx.Window(dst, edHintX, edY, edHintW, edH)
	tx, iw := edX+14, edW-28
	blink := ctx.Tick/16%2 == 0
	caret := func(text string, x, y int) {
		if blink {
			gfx.FillRect(dst, x+f.Width(text, 1)+1, y+1, 6, 13, pal.Yellow)
		}
	}
	label := func(s string, y int, focus bool) {
		col := pal.Tan
		if focus {
			col = pal.White
			f.Draw(dst, "►", edX+3, y, 1, pal.Yellow)
		}
		f.DrawShadow(dst, s, tx, y, 1, col)
	}
	const fy = 80 // where a title or language starts after its label
	label("Title:", edY+10, e.focus == edTitle)
	title := tailFit(f, e.title.Text(), iw-fy)
	if title == "" && e.focus != edTitle {
		f.DrawShadow(dst, "such as: Spelling week 5", tx+fy, edY+10, 1, pal.Ash)
	} else {
		f.DrawShadow(dst, title, tx+fy, edY+10, 1, pal.White)
	}
	if e.focus == edTitle {
		caret(title, tx+fy, edY+10)
	}
	label("Language:", edY+26, e.focus == edLang)
	f.DrawShadow(dst, "◄ "+e.language().Name+" ►", tx+fy, edY+26, 1, pal.Yellow)
	gfx.FillRect(dst, edX+8, edY+44, edW-16, 1, pal.Indigo)

	first := 0
	if e.focus == edBody {
		first = max(0, min(e.cur-edRows/2, len(e.lines)-edRows))
	}
	const gutter = 28
	for i := first; i < min(len(e.lines), first+edRows); i++ {
		ry := edBodyY + (i-first)*edRowH
		cur := e.focus == edBody && i == e.cur
		if cur {
			gfx.FillRect(dst, edX+6, ry-1, edW-12, edRowH, pal.Indigo)
		}
		f.Draw(dst, fmt.Sprintf("%2d", i+1), tx, ry, 1, pal.Ash)
		text := e.lines[i].Text()
		col := pal.Steel
		if e.Problem(i+1) != "" {
			f.Draw(dst, "!", tx+gutter-10, ry, 1, pal.Rose)
			col = pal.Rose
		} else if cur {
			col = pal.White
		}
		if cur {
			text = tailFit(f, text, iw-gutter-8)
			f.DrawShadow(dst, text, tx+gutter, ry, 1, col)
			caret(text, tx+gutter, ry)
		} else {
			f.DrawShadow(dst, fit(f, text, iw-gutter, 1), tx+gutter, ry, 1, col)
		}
	}
	if first > 0 {
		f.Draw(dst, "▲", edX+edW-18, edBodyY-8, 1, pal.Tan)
	}
	if first+edRows < len(e.lines) {
		f.Draw(dst, "▼", edX+edW-18, edY+edH-16, 1, pal.Tan)
	}

	// The format hint, and how the lines are doing.
	hx, hw := edHintX+12, edHintW-24
	f.DrawShadow(dst, "HOW TO WRITE IT", hx, edY+10, 1, pal.Yellow)
	for i, l := range edHint {
		f.DrawShadow(dst, fit(f, l.s, hw, 1), hx, edY+30+i*15, 1, l.col)
	}
	f.DrawShadow(dst, fit(f, e.summary(), hw, 1), hx, edY+edH-24, 1, pal.Lime)
	if e.language().Script == words.ScriptGreek {
		on := "Greek keys after the ="
		if e.greekSwap {
			on = "Greek keys flipped (F2)"
		}
		f.DrawShadow(dst, fit(f, on, hw, 1), hx, edY+edH-42, 1, pal.Sky)
	}

	// One line of news: a note, or why the current line is skipped.
	msg, mc := "", pal.Steel
	switch {
	case w.edNote != "":
		msg, mc = w.edNote, pal.Rose
	case e.focus == edBody && e.Problem(e.cur+1) != "":
		msg, mc = fmt.Sprintf("line %d: %s", e.cur+1, e.Problem(e.cur+1)), pal.Rose
	case e.focus == edBody:
		msg = "Type a word, then press Enter for the next line."
	case e.focus == edLang:
		msg = "Pick the language with ←/→, then press Enter."
	default:
		msg = "Give your list a name, then press Enter."
	}
	f.DrawShadow(dst, fit(f, msg, game.ScreenW-24, 1), 12, 306, 1, mc)
	help := "↑/↓ line   Enter next line   Tab accent   Ctrl+S save   Esc finish"
	if e.language().Script == words.ScriptGreek {
		help = "↑/↓ line  Enter next line  Tab accent  F2 Greek  Ctrl+S save  Esc finish"
	}
	f.DrawShadow(dst, help, 12, 324, 1, pal.Ash)
	f.DrawShadow(dst, "Lists you type stay on this computer.", 12, 340, 1, pal.Ash)

	switch w.edAsk {
	case edAskLeave:
		x, y := dialog(dst, ctx, "SAVE THE LIST?", 380, 110)
		f.DrawShadow(dst, "You typed words that are not saved yet.", x, y, 1, pal.Ice)
		f.DrawShadow(dst, "Y save   N throw away   Esc keep typing", x, y+28, 1, pal.Ash)
	case edAskSkip:
		_, probs := e.Checked()
		x, y := dialog(dst, ctx, "SAVE THE GOOD LINES?", 440, 120)
		f.DrawShadow(dst, plural(len(probs), "line")+" are not words, so they are left out.", x, y, 1, pal.Ice)
		f.DrawShadow(dst, "Y save anyway   Esc go back and fix them", x, y+28, 1, pal.Ash)
	}
}

// tailFit keeps the end of s in view, with an ellipsis at the front, so
// the caret and what was just typed show.
func tailFit(f *gfx.Font, s string, width int) string {
	if f.Width(s, 1) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && f.Width("…"+string(r), 1) > width {
		r = r[1:]
	}
	return "…" + string(r)
}
