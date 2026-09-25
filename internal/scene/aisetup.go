package scene

import (
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"math"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/clipboard"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/llm"
	"github.com/halpworld/halpwords/internal/pal"
)

// aiMode is what the AI Helper screen is doing.
type aiMode int

const (
	aiBrowse aiMode = iota
	aiKey           // typing or pasting a key
	aiPick          // choosing a model from a list
	aiReset         // confirming a reset of the spending counter
)

// The AI Helper screen's rows.
const (
	aiRowProvider = iota
	aiRowKey
	aiRowGame
	aiRowForge
	aiRowBudget
	aiRowTry
	aiRowReset
	aiRows
)

// AISetup is where a parent or teacher connects a language model: they
// choose a provider, give its API key, and can then pick the models, set a
// budget and see what has been spent.
type AISetup struct {
	bg   *ebiten.Image
	sel  int
	mode aiMode

	key   []rune // the key being typed
	show  bool   // show the key being typed
	paste *llm.Job[string]

	forge   bool // choosing the Word Forge model, not the game model
	opts    []string
	pick    int
	pickTop int

	try  *llm.Job[string]
	note string
	col  color.RGBA
}

// NewAISetup creates the AI Helper screen.
func NewAISetup(ctx *game.Context) game.Scene {
	a := &AISetup{bg: backdrop(9, 1.3)}
	if p := ctx.AI.Provider(); p != nil {
		ctx.AI.Check(p.ID)
	}
	return a
}

// providerChoices are the choices on the Provider row: off, then each one.
func providerChoices() []*llm.Provider { return append([]*llm.Provider{nil}, llm.Providers...) }

func (a *AISetup) rows(ctx *game.Context) int {
	if ctx.AI.Provider() == nil {
		return 1
	}
	return aiRows
}

func (a *AISetup) say(msg string, c color.RGBA) { a.note, a.col = msg, c }

// Update implements game.Scene.
func (a *AISetup) Update(ctx *game.Context) error {
	if a.try.Done() {
		text, err := a.try.Result()
		a.try = nil
		if err != nil {
			ctx.Sound.Play(audio.Wrong)
			a.say(llm.Explain(err), pal.Rose)
		} else {
			ctx.Sound.Play(audio.Correct)
			a.say("It works! The dungeon says: “"+text+"”", pal.Lime)
		}
	}
	switch a.mode {
	case aiKey:
		a.updateKey(ctx)
	case aiPick:
		a.updatePick(ctx)
	case aiReset:
		a.updateReset(ctx)
	default:
		a.updateBrowse(ctx)
	}
	return nil
}

func (a *AISetup) updateBrowse(ctx *game.Context) {
	n := a.rows(ctx)
	a.sel = min(a.sel, n-1)
	step := 0
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewTitle(ctx))
		return
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		a.sel = (a.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		a.sel = (a.sel + 1) % n
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyA):
		step = -1
	case input.Repeat(ebiten.KeyArrowRight) || input.Repeat(ebiten.KeyD):
		step = 1
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		a.choose(ctx)
		return
	}
	if step != 0 {
		a.change(ctx, step)
	}
}

// change steps the setting on the chosen row.
func (a *AISetup) change(ctx *game.Context, step int) {
	ai, p := ctx.AI, ctx.AI.Provider()
	switch a.sel {
	case aiRowProvider:
		choices := providerChoices()
		i := 0
		for k, c := range choices {
			if c == p {
				i = k
			}
		}
		next := choices[(i+step+len(choices))%len(choices)]
		id := llm.Off
		if next != nil {
			id = next.ID
			ai.Check(id)
		}
		a.saved(ctx, ai.SetProvider(id))
		a.note = ""
	case aiRowGame, aiRowForge:
		forge := a.sel == aiRowForge
		opts := modelOptions(ai, p.ID, ai.Model(p.ID, forge))
		i := indexOf(opts, ai.Model(p.ID, forge))
		a.saved(ctx, ai.SetModel(p.ID, forge, opts[(i+step+len(opts))%len(opts)]))
	case aiRowBudget:
		i := 0
		for k, b := range llm.Budgets {
			if b == ai.Budget(p.ID) {
				i = k
			}
		}
		a.saved(ctx, ai.SetBudget(p.ID, llm.Budgets[(i+step+len(llm.Budgets))%len(llm.Budgets)]))
	default:
		return
	}
}

// saved plays a sound for a changed setting, or says it could not be saved.
func (a *AISetup) saved(ctx *game.Context, err error) {
	if err != nil {
		ctx.Sound.Play(audio.Wrong)
		a.say("Could not save: "+err.Error(), pal.Rose)
		return
	}
	ctx.Sound.Play(audio.Accent)
}

// choose acts on the chosen row.
func (a *AISetup) choose(ctx *game.Context) {
	ai, p := ctx.AI, ctx.AI.Provider()
	switch a.sel {
	case aiRowKey:
		ctx.Sound.Play(audio.Select)
		a.key, a.show = a.key[:0], false
		a.mode = aiKey
	case aiRowGame, aiRowForge:
		ctx.Sound.Play(audio.Select)
		a.forge = a.sel == aiRowForge
		cur := ai.Model(p.ID, a.forge)
		a.opts = modelOptions(ai, p.ID, cur)
		a.pick = indexOf(a.opts, cur)
		a.pickTop = 0
		a.mode = aiPick
	case aiRowTry:
		if a.try != nil {
			return
		}
		if !ai.Ready() {
			ctx.Sound.Play(audio.Wrong)
			a.say(ai.Problem(), pal.Rose)
			return
		}
		ctx.Sound.Play(audio.Select)
		a.say("Sending a test message…", pal.Yellow)
		a.try = ai.Try()
	case aiRowReset:
		ctx.Sound.Play(audio.Select)
		a.mode = aiReset
	default:
		a.change(ctx, 1)
	}
}

// pasteDown reports whether the paste keys were pressed: Ctrl+V, Cmd+V or
// Shift+Insert.
func pasteDown() bool {
	mod := input.Held(ebiten.KeyControl) || input.Held(ebiten.KeyMeta)
	return (mod && input.Pressed(ebiten.KeyV)) || (input.Held(ebiten.KeyShift) && input.Pressed(ebiten.KeyInsert))
}

func (a *AISetup) updateKey(ctx *game.Context) {
	p := ctx.AI.Provider()
	if a.paste.Done() {
		text, err := a.paste.Result()
		a.paste = nil
		if k := llm.CleanKey(text); err == nil && k != "" {
			a.key = []rune(k)
			ctx.Sound.Play(audio.Accent)
		} else {
			ctx.Sound.Play(audio.Wrong)
			a.say("Nothing to paste. Copy the key first, or drop a text file with it.", pal.Rose)
		}
	}
	if data, ok := droppedText(); ok {
		a.key = []rune(llm.CleanKey(data))
		ctx.Sound.Play(audio.Accent)
	}
	mod := input.Held(ebiten.KeyControl) || input.Held(ebiten.KeyMeta)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		a.mode = aiBrowse
		return
	case pasteDown():
		if a.paste == nil {
			a.paste = llm.Start(clipboard.Read)
		}
		return
	case input.Pressed(ebiten.KeyTab):
		a.show = !a.show
		ctx.Sound.Play(audio.Blip)
		return
	case input.Pressed(ebiten.KeyDelete) || (mod && input.Pressed(ebiten.KeyBackspace)):
		a.key = a.key[:0]
		ctx.Sound.Play(audio.Erase)
		return
	case input.Repeat(ebiten.KeyBackspace) && len(a.key) > 0:
		a.key = a.key[:len(a.key)-1]
		ctx.Sound.Play(audio.Erase)
		return
	case input.Confirm():
		key := llm.CleanKey(string(a.key))
		if err := ctx.AI.SetKey(p.ID, key); err != nil {
			a.saved(ctx, err)
			return
		}
		ctx.Sound.Play(audio.Select)
		switch {
		case key == "" && ctx.AI.EnvVar(p.ID) != "":
			a.say("Using the key in "+ctx.AI.EnvVar(p.ID)+". Checking it…", pal.Yellow)
		case key == "":
			a.say("Key removed.", pal.Yellow)
		default:
			a.say("Key saved on this computer. Checking it…", pal.Yellow)
		}
		a.mode = aiBrowse
		return
	}
	if mod {
		return // shortcuts, not typing
	}
	for _, r := range ctx.Input.Chars {
		if r > ' ' && r != 0x7f && len(a.key) < 300 {
			a.key = append(a.key, r)
			ctx.Sound.Play(audio.Key)
		}
	}
}

// droppedText returns the text of a small file dropped on the window.
func droppedText() (string, bool) {
	fsys := ebiten.DroppedFiles()
	if fsys == nil {
		return "", false
	}
	var text string
	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || text != "" {
			return nil
		}
		if data, err := fs.ReadFile(fsys, p); err == nil && len(data) < 4096 {
			text = string(data)
		}
		return nil
	})
	return text, text != ""
}

const aiPickRows = 9

func (a *AISetup) updatePick(ctx *game.Context) {
	n := len(a.opts)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		a.mode = aiBrowse
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		a.pick = (a.pick + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		a.pick = (a.pick + 1) % n
	case input.Repeat(ebiten.KeyPageUp):
		a.pick = max(0, a.pick-aiPickRows)
	case input.Repeat(ebiten.KeyPageDown):
		a.pick = min(n-1, a.pick+aiPickRows)
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		p := ctx.AI.Provider()
		a.saved(ctx, ctx.AI.SetModel(p.ID, a.forge, a.opts[a.pick]))
		a.mode = aiBrowse
	}
	a.pickTop = max(0, min(a.pickTop, a.pick, n-aiPickRows))
	if a.pick >= a.pickTop+aiPickRows {
		a.pickTop = a.pick - aiPickRows + 1
	}
}

func (a *AISetup) updateReset(ctx *game.Context) {
	switch {
	case input.Pressed(ebiten.KeyY) || input.Confirm():
		p := ctx.AI.Provider()
		a.saved(ctx, ctx.AI.ResetSpent(p.ID))
		a.say("The spending counter starts again from $0.", pal.Lime)
		a.mode = aiBrowse
	case input.Pressed(ebiten.KeyN) || input.Back():
		ctx.Sound.Play(audio.Back)
		a.mode = aiBrowse
	}
}

// modelOptions lists the models to choose from: the known ones the key can
// use, cheapest first, then any others the provider lists. The current
// choice is always there.
func modelOptions(ai *llm.Service, p llm.ProviderID, cur string) []string {
	listed := ai.Status(p).Listed
	offered := map[string]bool{}
	for _, id := range listed {
		offered[id] = true
	}
	var out []string
	have := map[string]bool{}
	add := func(id string) {
		if !have[id] {
			have[id] = true
			out = append(out, id)
		}
	}
	for _, m := range llm.ModelsFor(p) {
		if len(listed) == 0 || offered[m.ID] || m.ID == cur {
			add(m.ID)
		}
	}
	for _, id := range listed {
		add(id)
	}
	add(cur)
	return out
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return 0
}

// money formats dollars.
func money(usd float64) string {
	switch {
	case usd == llm.NoLimit:
		return "no limit"
	case usd == 0:
		return "$0"
	case usd < 0.001:
		return "under $0.001"
	case usd < 0.1:
		return fmt.Sprintf("$%.3f", usd)
	case usd < 100 && math.Abs(usd*100-math.Round(usd*100)) > 0.05:
		return fmt.Sprintf("$%.3f", usd) // part of a cent: show it
	}
	return fmt.Sprintf("$%.2f", usd)
}

// modelLabel names a model and its price.
func modelLabel(p llm.ProviderID, id string) (name, price string) {
	m, ok := llm.Known(p, id)
	if !ok {
		in, out, _ := llm.Rate(p, id, time.Now())
		return id, fmt.Sprintf("price unknown: counted as $%g/$%g", in, out)
	}
	price = fmt.Sprintf("$%g / $%g per M tokens", m.In, m.Out)
	if m.PeakOut > 0 {
		price = fmt.Sprintf("$%g / $%g, ×%g when busy", m.In, m.Out, m.PeakOut/m.Out)
	}
	return m.Name, price
}

func tokens(n int64) string {
	switch {
	case n >= 1e6:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1e4:
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprint(n)
}

// Draw implements game.Scene.
func (a *AISetup) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, a.bg, 0, 0)
	f.DrawCentered(dst, "AI Helper", cx, 6, 2, pal.Yellow)
	f.DrawCentered(dst, "For parents and teachers: connect an AI to make the dungeon smarter.", cx, 40, 1, pal.Tan)

	ai, p := ctx.AI, ctx.AI.Provider()
	const x, w, rowH, valX = 16, game.ScreenW - 32, 18, 170
	y := 56
	n := a.rows(ctx)
	gfx.Window(dst, x, y, w, rowH*n+14)
	row := func(i int, name, value string, vcol color.RGBA, extra string, ecol color.RGBA) {
		ry := y + 8 + i*rowH
		col := pal.Steel
		if i == a.sel {
			gfx.FillRect(dst, x+6, ry-2, w-12, rowH-1, pal.Fade(pal.Indigo, 0.8))
			col = pal.White
			if vcol == pal.Ice {
				vcol = pal.Yellow
			}
		}
		f.DrawShadow(dst, name, x+16, ry, 1, col)
		f.DrawShadow(dst, value, x+valX, ry, 1, vcol)
		if extra != "" {
			ex := x + w - 14 - f.Width(extra, 1)
			ex = max(ex, x+valX+f.Width(value, 1)+16)
			f.DrawShadow(dst, fit(f, extra, x+w-14-ex, 1), ex, ry, 1, ecol)
		}
	}
	pname := "Off: play without AI"
	if p != nil {
		pname = p.Name
	}
	row(aiRowProvider, "Provider", "◄ "+pname+" ►", pal.Ice, "", pal.Ash)
	if p != nil {
		st := ai.Status(p.ID)
		masked, env := ai.Key(p.ID)
		kv, kcol := masked, pal.Ice
		if masked == "" {
			kv, kcol = "none: press Enter to add one", pal.Orange
		} else if env {
			kv += " (from " + ai.EnvVar(p.ID) + ")"
		}
		ks, kscol := "", pal.Ash
		switch {
		case masked == "":
		case st.Checking:
			ks, kscol = "checking…", pal.Yellow
		case errors.Is(st.Err, llm.ErrKey):
			ks, kscol = "✗ refused", pal.Rose
		case st.Checked && st.Err == nil:
			ks, kscol = "✓ works", pal.Lime
		}
		row(aiRowKey, "API key", kv, kcol, ks, kscol)
		for i, forge := range []bool{false, true} {
			name, price := modelLabel(p.ID, ai.Model(p.ID, forge))
			label := "Game model"
			if forge {
				label = "Word Forge model"
			}
			row(aiRowGame+i, label, "◄ "+name+" ►", pal.Ice, price, pal.Ash)
		}
		row(aiRowBudget, "Budget", "◄ "+money(ai.Budget(p.ID))+" ►", pal.Ice, "", pal.Ash)
		tv := "Send a test message"
		if a.try != nil {
			tv = "Sending…"
		}
		row(aiRowTry, "Try it", tv, pal.Ice, "", pal.Ash)
		row(aiRowReset, "Spending", "Reset the counter to $0", pal.Ice, "", pal.Ash)
		a.drawSpending(dst, ctx, p, x, y+rowH*n+20, w)
	} else {
		a.drawIntro(dst, ctx, x, y+rowH+26, w)
	}

	by := game.ScreenH - 52
	if a.note != "" {
		f.DrawCentered(dst, fit(f, a.note, game.ScreenW-24, 1), cx, by, 1, a.col)
	} else {
		f.DrawCentered(dst, fit(f, a.about(ctx), game.ScreenW-24, 1), cx, by, 1, pal.Ice)
	}
	status, scol := "AI is off. The game plays the same, with its built-in puzzles.", pal.Ash
	if ai.Ready() {
		status, scol = "✦ AI is ON: floor stories, gap-fill puzzles, taunts, tips, Word Forge", pal.Lime
	} else if p != nil {
		status, scol = "AI is not working yet: "+ai.Problem(), pal.Orange
	}
	f.DrawCentered(dst, fit(f, status, game.ScreenW-24, 1), cx, by+18, 1, scol)
	f.DrawShadow(dst, "↑/↓ choose   ←/→ change   Enter select   Esc back", 8, game.ScreenH-20, 1, pal.Ash)

	switch a.mode {
	case aiKey:
		a.drawKey(dst, ctx, p)
	case aiPick:
		a.drawPick(dst, ctx, p)
	case aiReset:
		x, y := dialog(dst, ctx, "RESET SPENDING?", 440, 120)
		f.DrawShadow(dst, "Count "+p.Name+" spending from $0 again,", x, y, 1, pal.Ice)
		f.DrawShadow(dst, "for example after adding credit to the account.", x, y+16, 1, pal.Ice)
		f.DrawShadow(dst, "Y / Enter reset   N / Esc keep", x, y+42, 1, pal.Ash)
	}
}

// about explains the chosen row.
func (a *AISetup) about(ctx *game.Context) string {
	p := ctx.AI.Provider()
	switch a.sel {
	case aiRowProvider:
		return "Choose who runs the AI with ←/→. Off plays the game without it."
	case aiRowKey:
		return "Get a key at " + p.KeyPage + ". It stays on this computer."
	case aiRowGame:
		return "Writes floor stories, puzzles and taunts during play. Cheap and fast is best."
	case aiRowForge:
		return "Makes new word lists in the Word Forge. Smarter models make fewer mistakes."
	case aiRowBudget:
		return "AI stops when this is spent. Match it to the credit on the account."
	case aiRowTry:
		return "Sends one tiny message to check that everything works."
	case aiRowReset:
		return "Start counting from zero, for example after adding more credit."
	}
	return ""
}

// drawIntro explains what the AI does, when none is chosen.
func (a *AISetup) drawIntro(dst *ebiten.Image, ctx *game.Context, x, y, w int) {
	f := ctx.Font
	lines := []string{
		"With an AI connected, the dungeon reacts to the words you are learning:",
		"• floors get names and stories built around your words",
		"• new puzzles: gap-fill sentences in your language, and fresh riddles",
		"• monsters shout taunts in the language you learn",
		"• a Scroll of Insight at campfires with memory tips for tricky words",
		"• the Word Forge makes new word lists on any topic",
		"Choose Anthropic, OpenAI, Meta or DeepSeek, then paste an API key.",
	}
	gfx.Window(dst, x, y, w, len(lines)*17+16)
	for i, l := range lines {
		col := pal.Steel
		if i == 0 || i == len(lines)-1 {
			col = pal.Ice
		}
		f.DrawShadow(dst, l, x+16, y+9+i*17, 1, col)
	}
}

// drawSpending shows what has been spent, the budget left and, where the
// provider says, the account's balance.
func (a *AISetup) drawSpending(dst *ebiten.Image, ctx *game.Context, p *llm.Provider, x, y, w int) {
	f := ctx.Font
	ai := ctx.AI
	t := ai.Spent(p.ID)
	budget, left := ai.Budget(p.ID), ai.Left(p.ID)
	gfx.Window(dst, x, y, w, 80)
	tx := x + 16
	spent := "Spent: " + money(t.Spent)
	if !t.Since.IsZero() {
		spent += " since " + t.Since.Local().Format("2 Jan 2006")
	}
	spent += fmt.Sprintf(" · %d requests · %s tokens", t.Requests, tokens(t.In+t.Out))
	if t.Estimated {
		spent += " (estimated)"
	}
	f.DrawShadow(dst, fit(f, spent, w-32, 1), tx, y+8, 1, pal.Ice)
	if budget == llm.NoLimit {
		f.DrawShadow(dst, "Budget: no limit. Keep an eye on the account!", tx, y+25, 1, pal.Orange)
	} else {
		lcol := pal.Lime
		if left < budget/5 {
			lcol = pal.Orange
		}
		label := "Left in the budget: " + money(left) + " of " + money(budget)
		f.DrawShadow(dst, label, tx, y+25, 1, lcol)
		bar(dst, tx+f.Width(label, 1)+12, y+28, 120, 8, left/budget, lcol, pal.Night)
	}
	st := ai.Status(p.ID)
	short, _, _ := strings.Cut(p.Name, " (")
	bal, bcol := short+" doesn't share the account balance: check it on its website.", pal.Ash
	switch {
	case p.Balance && !st.BalanceOK && !st.Checking:
		bal = "Add a working key to see the account balance."
	case st.BalanceOK:
		bal, bcol = fmt.Sprintf("Account balance at %s: %.2f %s", p.Name, st.Balance.Amount, st.Balance.Currency), pal.Lime
		if !st.Balance.Usable {
			bcol = pal.Rose
		}
	case p.Balance && st.Checking:
		bal, bcol = "Reading the account balance…", pal.Yellow
	}
	f.DrawShadow(dst, fit(f, bal, w-32, 1), tx, y+42, 1, bcol)
	f.DrawShadow(dst, "This session: "+money(ai.Session()), tx, y+59, 1, pal.Steel)
}

func (a *AISetup) drawKey(dst *ebiten.Image, ctx *game.Context, p *llm.Provider) {
	f := ctx.Font
	const dw = 560
	x, y := dialog(dst, ctx, strings.ToUpper(p.Name)+" KEY", dw, 190)
	f.DrawShadow(dst, "Make a key at "+p.KeyPage+" and paste it here.", x, y, 1, pal.Ice)
	text := string(a.key)
	if !a.show {
		text = strings.Repeat("•", len(a.key))
	}
	for len(text) > 0 && f.Width(text+"_", 1) > dw-40 {
		text = string([]rune(text)[1:]) // keep the end in view
	}
	f.DrawShadow(dst, text, x, y+30, 1, pal.White)
	gfx.FillRect(dst, x, y+48, dw-40, 1, pal.Indigo)
	if ctx.Tick/16%2 == 0 {
		gfx.FillRect(dst, x+f.Width(text, 1)+1, y+32, 6, 13, pal.Yellow)
	}
	info, icol := fmt.Sprintf("%d characters", len(a.key)), pal.Steel
	switch {
	case a.paste != nil:
		info, icol = "Pasting…", pal.Yellow
	case len(a.key) > 0 && p.LooksWrong(string(a.key)):
		info, icol = "This doesn't look like a "+p.Name+" key.", pal.Orange
	case len(a.key) == 0 && ctx.AI.EnvVar(p.ID) != "":
		info, icol = "Leave it empty to use the key in "+ctx.AI.EnvVar(p.ID)+".", pal.Lime
	}
	f.DrawShadow(dst, info, x, y+56, 1, icol)
	f.DrawShadow(dst, "Ctrl+V or Cmd+V paste · or drop a text file with the key", x, y+80, 1, pal.Tan)
	f.DrawShadow(dst, "Tab show/hide · Delete clear · Enter save · Esc cancel", x, y+98, 1, pal.Ash)
}

func (a *AISetup) drawPick(dst *ebiten.Image, ctx *game.Context, p *llm.Provider) {
	f := ctx.Font
	const dw = 600
	title := "GAME MODEL"
	if a.forge {
		title = "WORD FORGE MODEL"
	}
	shown := min(len(a.opts), aiPickRows)
	x, y := dialog(dst, ctx, title, dw, 110+shown*20)
	for i := a.pickTop; i < a.pickTop+shown; i++ {
		ry := y + (i-a.pickTop)*20
		name, price := modelLabel(p.ID, a.opts[i])
		col, pcol := pal.Steel, pal.Ash
		if i == a.pick {
			gfx.FillRect(dst, x-6, ry-2, dw-28, 19, pal.Indigo)
			col, pcol = pal.White, pal.Ice
		}
		if m, ok := llm.Known(p.ID, a.opts[i]); ok {
			name += " · " + m.About
		}
		f.DrawShadow(dst, fit(f, name, 330, 1), x, ry, 1, col)
		f.DrawShadow(dst, fit(f, price, dw-40-340, 1), x+340, ry, 1, pcol)
	}
	if a.pickTop > 0 {
		f.Draw(dst, "▲", x+dw/2-24, y-14, 1, pal.Tan)
	}
	if a.pickTop+shown < len(a.opts) {
		f.Draw(dst, "▼", x+dw/2-24, y+shown*20-4, 1, pal.Tan)
	}
	if len(ctx.AI.Status(p.ID).Listed) == 0 {
		f.DrawShadow(dst, "With a working key, every model it can use is listed.", x, y+shown*20+6, 1, pal.Tan)
	}
	f.DrawShadow(dst, "↑/↓ choose   Enter use this model   Esc cancel", x, y+shown*20+24, 1, pal.Ash)
}
