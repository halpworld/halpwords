package scene

import (
	"context"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/report"
	"github.com/halpworld/halpwords/internal/save"
)

// reportStep is where the Report screen is.
type reportStep int

const (
	reportChoose reportStep = iota // which kind of report
	reportBug                      // the game's details and crash.txt
	reportUpset                    // what upset the player
	reportText                     // an optional note, then send
	reportDone                     // thanks
)

// The first step's choices.
var reportKinds = []string{"Something's wrong with the game", "Something upset me"}

// Report is the pause menu's Report screen (PLAN, halpwords-server W1.11):
// a bug report with the game's version, platform and seed and, if the
// player ticks the box after seeing it in full, the last crash.txt; or
// "Something upset me", picked from a few choices, with optional text.
// Reports are queued in the user's folder and sent when the game is
// online (ctx.Reports).
type Report struct {
	bg   *ebiten.Image
	seed string

	step      reportStep
	sel       int
	kind      string
	about     string
	crash     string // the last crash.txt, or ""
	sendCrash bool   // the box is ticked
	scroll    int    // the first crash line shown
	text      []rune
	failed    bool // queuing the report failed

	crashLines []string // crash wrapped to the box, made when first drawn
}

// newReport creates the Report screen for a run with the seed ("" when
// there is no run).
func newReport(ctx *game.Context, seed string) *Report {
	r := &Report{bg: backdrop(5, 1.3), seed: seed}
	if b, err := save.Read(game.CrashFile); err == nil {
		r.crash = string(b)
		if len(r.crash) > report.MaxCrash {
			r.crash = r.crash[len(r.crash)-report.MaxCrash:] // the end says what went wrong
			for !utf8.ValidString(r.crash) {
				r.crash = r.crash[1:]
			}
		}
	}
	if ctx.Reports != nil {
		go ctx.Reports.Flush(context.Background()) // anything left from before
	}
	return r
}

// crashBox is how many crash lines the bug step shows at once.
const crashBox = 8

// Update implements game.Scene.
func (r *Report) Update(ctx *game.Context) error {
	switch r.step {
	case reportChoose:
		r.updateChoose(ctx)
	case reportBug:
		r.updateBug(ctx)
	case reportUpset:
		r.updateUpset(ctx)
	case reportText:
		r.updateText(ctx)
	case reportDone:
		if input.Back() || input.Confirm() || input.Pressed(ebiten.KeySpace) {
			ctx.Sound.Play(audio.Back)
			ctx.Pop()
		}
	}
	return nil
}

// pickFrom moves sel through n choices, and reports Enter or Space.
func pickFrom(ctx *game.Context, sel *int, n int) bool {
	switch {
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		*sel = (*sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		*sel = (*sel + 1) % n
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		ctx.Sound.Play(audio.Select)
		return true
	}
	return false
}

func (r *Report) updateChoose(ctx *game.Context) {
	if input.Back() {
		ctx.Sound.Play(audio.Back)
		ctx.Pop()
		return
	}
	if !pickFrom(ctx, &r.sel, len(reportKinds)) {
		return
	}
	if r.sel == 0 {
		r.kind, r.step = report.KindBug, reportBug
	} else {
		r.kind, r.step, r.sel = report.KindUpset, reportUpset, 0
	}
}

func (r *Report) updateBug(ctx *game.Context) {
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		r.step, r.sel = reportChoose, 0
	case r.crash != "" && input.Pressed(ebiten.KeySpace):
		r.sendCrash = !r.sendCrash
		ctx.Sound.Play(audio.Blip)
	case r.crash != "" && input.Up():
		r.scroll = max(0, r.scroll-1)
	case r.crash != "" && input.Down():
		r.scroll = min(max(0, len(r.crashLines)-crashBox), r.scroll+1)
	case input.Repeat(ebiten.KeyPageUp):
		r.scroll = max(0, r.scroll-crashBox)
	case input.Repeat(ebiten.KeyPageDown):
		r.scroll = min(max(0, len(r.crashLines)-crashBox), r.scroll+crashBox)
	case input.Confirm():
		ctx.Sound.Play(audio.Select)
		r.step = reportText
	}
}

func (r *Report) updateUpset(ctx *game.Context) {
	if input.Back() {
		ctx.Sound.Play(audio.Back)
		r.step, r.sel = reportChoose, 1
		return
	}
	if pickFrom(ctx, &r.sel, len(report.Abouts)) {
		r.about, r.step = report.Abouts[r.sel], reportText
	}
}

func (r *Report) updateText(ctx *game.Context) {
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		r.step = reportBug
		if r.kind == report.KindUpset {
			r.step = reportUpset
		}
		return
	case input.Confirm():
		r.send(ctx)
		return
	}
	for _, c := range ctx.Input.Chars {
		if !unicode.IsControl(c) && len(r.text) < report.MaxText {
			r.text = append(r.text, c)
			ctx.Sound.Play(audio.Key)
		}
	}
	if input.Repeat(ebiten.KeyBackspace) && len(r.text) > 0 {
		r.text = r.text[:len(r.text)-1]
		ctx.Sound.Play(audio.Erase)
	}
}

// build makes the report from what the player chose. The crash is only
// in it when the box is ticked.
func (r *Report) build() report.Report {
	g := report.Game{Version: game.VersionText(), Platform: game.Platform()}
	if r.kind == report.KindUpset {
		return report.Upset(g, r.about, string(r.text))
	}
	return report.Bug(g, r.seed, string(r.text), r.crash, r.sendCrash)
}

// send queues the report; the outbox sends it when it can.
func (r *Report) send(ctx *game.Context) {
	r.failed = ctx.Reports == nil || ctx.Reports.Submit(r.build()) != nil
	if r.failed {
		ctx.Sound.Play(audio.Wrong)
	} else {
		ctx.Sound.Play(audio.Select)
	}
	r.step = reportDone
}

// linked reports whether reports carry the linked game's token.
func linked(ctx *game.Context) bool {
	s := ctx.Reports
	return s != nil && s.Sender != nil && s.Sender.Token != nil && s.Sender.Token() != ""
}

// Draw implements game.Scene.
func (r *Report) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, r.bg, 0, 0)
	f.DrawCentered(dst, "Report", cx, 4, 3, pal.Yellow)
	const x, w = 24, game.ScreenW - 48
	y := 60
	help := ""
	switch r.step {
	case reportChoose:
		gfx.Window(dst, x, y, w, 110)
		f.DrawCentered(dst, "What would you like to tell us?", cx, y+14, 1, pal.Ice)
		for i, k := range reportKinds {
			r.drawChoice(dst, ctx, k, x, y+44+i*24, w, i == r.sel)
		}
		who := "Your game isn't linked, so the report doesn't say who you are."
		if linked(ctx) {
			who = "Your game is linked, so the report says which game sent it."
		}
		f.DrawCentered(dst, who, cx, y+128, 1, pal.Tan)
		help = "↑/↓ choose   Enter select   Esc back"
	case reportBug:
		r.drawBug(dst, ctx, x, y, w)
		help = "Enter next   Esc back"
		if r.crash != "" {
			help = "↑/↓ PgUp/PgDn read   Space tick   Enter next   Esc back"
		}
	case reportUpset:
		gfx.Window(dst, x, y, w, 40+len(report.Abouts)*22)
		f.DrawCentered(dst, "What upset you?", cx, y+12, 1, pal.Ice)
		for i, a := range report.Abouts {
			r.drawChoice(dst, ctx, report.AboutLabels[a], x, y+36+i*22, w, i == r.sel)
		}
		f.DrawCentered(dst, "If you ever feel unsafe, tell a grown-up you trust.", cx, y+60+len(report.Abouts)*22, 1, pal.Tan)
		help = "↑/↓ choose   Enter select   Esc back"
	case reportText:
		r.drawText(dst, ctx, x, y, w)
		help = "Type a note if you like   Enter send   Esc back"
	case reportDone:
		gfx.Window(dst, x, y+30, w, 80)
		if r.failed {
			f.DrawCentered(dst, "Sorry, the report couldn't be saved.", cx, y+50, 1, pal.Rose)
			f.DrawCentered(dst, "Please tell a grown-up, who can report it on halpwords.com.", cx, y+74, 1, pal.Ice)
		} else {
			f.DrawCentered(dst, "Thank you! Your report is on its way.", cx, y+50, 1, pal.Lime)
			f.DrawCentered(dst, "If the game is offline, it goes the next time it is online.", cx, y+74, 1, pal.Ice)
		}
		help = "Enter back to the game"
	}
	f.DrawShadow(dst, help, 8, game.ScreenH-20, 1, pal.Ash)
}

func (r *Report) drawChoice(dst *ebiten.Image, ctx *game.Context, label string, x, y, w int, sel bool) {
	f := ctx.Font
	col := pal.Steel
	if sel {
		gfx.FillRect(dst, x+6, y-3, w-12, 20, pal.Fade(pal.Indigo, 0.8))
		col = pal.White
		f.Draw(dst, "►", x+20, y, 1, pal.Yellow)
	}
	f.DrawShadow(dst, label, x+40, y, 1, col)
}

func (r *Report) drawBug(dst *ebiten.Image, ctx *game.Context, x, y, w int) {
	f := ctx.Font
	seed := r.seed
	if seed == "" {
		seed = "none"
	}
	h := 80
	if r.crash != "" {
		h = 80 + 36 + crashBox*16
	}
	gfx.Window(dst, x, y, w, h)
	f.DrawShadow(dst, "We'll send this about your game:", x+16, y+10, 1, pal.Ice)
	f.DrawShadow(dst, "Version  "+game.VersionText(), x+32, y+30, 1, pal.White)
	f.DrawShadow(dst, "System   "+game.Platform(), x+32, y+46, 1, pal.White)
	f.DrawShadow(dst, "Seed     "+seed, x+32, y+62, 1, pal.White)
	if r.crash == "" {
		return
	}
	box := "[ ]"
	col := pal.Steel
	if r.sendCrash {
		box, col = "[x]", pal.Lime
	}
	f.DrawShadow(dst, box+" Send crash.txt too (it is shown below)", x+16, y+86, 1, col)
	if r.crashLines == nil {
		r.crashLines = hardWrap(f, r.crash, w-40)
	}
	by := y + 106
	gfx.FillRect(dst, x+12, by-2, w-24, crashBox*16+4, pal.Fade(pal.Black, 0.6))
	end := min(len(r.crashLines), r.scroll+crashBox)
	for i, l := range r.crashLines[r.scroll:end] {
		f.Draw(dst, l, x+18, by+i*16, 1, pal.Tan)
	}
	if len(r.crashLines) > crashBox {
		pos := "lines " + strconv.Itoa(r.scroll+1) + "–" + strconv.Itoa(end) + " of " + strconv.Itoa(len(r.crashLines))
		f.DrawShadow(dst, pos, x+w-16-f.Width(pos, 1), y+86, 1, pal.Ash)
	}
}

func (r *Report) drawText(dst *ebiten.Image, ctx *game.Context, x, y, w int) {
	f := ctx.Font
	title := "Anything to add? (you don't have to)"
	if r.kind == report.KindUpset {
		title = report.AboutLabels[r.about] + " upset me. Anything to add? (you don't have to)"
	}
	gfx.Window(dst, x, y, w, 190)
	f.DrawShadow(dst, title, x+16, y+10, 1, pal.Ice)
	lines := hardWrap(f, string(r.text)+"_", w-40)
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	for i, l := range lines {
		f.DrawShadow(dst, l, x+20, y+34+i*16, 1, pal.White)
	}
	left := report.MaxText - len(r.text)
	f.DrawShadow(dst, strconv.Itoa(left)+" letters left", x+16, y+168, 1, pal.Ash)
	if r.kind == report.KindBug {
		note := "No crash report."
		if r.crash != "" && r.sendCrash {
			note = "With the crash report."
		} else if r.crash != "" {
			note = "Without the crash report."
		}
		f.DrawShadow(dst, note, x+w-16-f.Width(note, 1), y+168, 1, pal.Ash)
	}
}

// hardWrap splits s into lines no wider than width at scale 1, keeping
// its line breaks and breaking long words (a crash has long paths).
func hardWrap(f *gfx.Font, s string, width int) []string {
	var out []string
	for _, para := range strings.Split(strings.NewReplacer("\r", "", "\t", "    ").Replace(s), "\n") {
		line := ""
		for _, c := range para {
			if line != "" && f.Width(line+string(c), 1) > width {
				out = append(out, line)
				line = ""
			}
			line += string(c)
		}
		out = append(out, line)
	}
	return out
}
