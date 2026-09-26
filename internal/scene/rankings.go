package scene

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
)

// The Hall of Fame's Rankings tab shows the boards a grown-up put the
// linked learner on (halpwords-server's rankings), for the language
// chosen. Players are shown by their made-up Halpwords names only, as the
// server sends them. There is no tab until there is a board.

// ranking reports whether the Rankings tab is showing.
func (h *HallOfFame) ranking() bool { return h.mi == len(fameModes) }

// tabs is how many tabs the Hall of Fame has: a table for each mode,
// and Rankings when the learner is on a board.
func (h *HallOfFame) tabs(ctx *game.Context) int {
	if len(ctx.Link.Boards()) > 0 {
		return len(fameModes) + 1
	}
	return len(fameModes)
}

// boards are the boards in the chosen language.
func (h *HallOfFame) boards(ctx *game.Context) []link.Board {
	var out []link.Board
	for _, b := range ctx.Link.Boards() {
		if b.Lang == h.lang().Code {
			out = append(out, b)
		}
	}
	return out
}

// boardTitle says which board it is: "Class · Hardcore · this week".
func boardTitle(b link.Board) string {
	scope := map[string]string{"friends": "Friends", "class": "Class", "school": "School", "region": "Region",
		"country": "Country", "world": "World"}[b.Scope]
	if scope == "" {
		scope = b.Scope
	}
	if b.Scope == "friends" && b.Name != "" {
		scope = b.Name
	}
	kind := "Hardcore"
	if b.Kind == "daily" {
		kind = "Daily Dungeon"
	}
	season := map[string]string{"week": "this week", "month": "this month", "all": "all time", "day": "today"}[b.Season]
	if season == "" {
		season = b.Period
	}
	return scope + " · " + kind + " · " + season
}

func (h *HallOfFame) drawRankings(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	f.DrawCentered(dst, "◄ "+h.lang().Name+" ►   ·   Rankings", cx, 60, 1, pal.Tan)
	const x, w, rowH = 24, game.ScreenW - 48, 20
	y := 82
	gfx.Window(dst, x, y, w, rowH*11+34)
	bs := h.boards(ctx)
	if len(bs) == 0 {
		f.DrawCentered(dst, "No boards in "+h.lang().Name+".", cx, y+90, 1, pal.Ash)
		return
	}
	h.bi = min(h.bi, len(bs)-1)
	b := bs[h.bi]
	title := boardTitle(b)
	if len(bs) > 1 {
		title = fmt.Sprintf("%s  (%d/%d)", title, h.bi+1, len(bs))
	}
	f.DrawCentered(dst, title, cx, y+8, 1, pal.Yellow)
	cols := [...]int{x + 16, x + 56, x + 300, x + 380, x + 478}
	for i, head := range []string{"#", "Name", "Floor", "Score", ""} {
		f.DrawShadow(dst, head, cols[i], y+26, 1, pal.Tan)
	}
	if len(b.Entries) == 0 {
		f.DrawCentered(dst, "Nobody yet. Will you be the first?", cx, y+110, 1, pal.Ash)
	}
	for i, e := range b.Entries {
		if i >= 11 {
			break
		}
		ry := y + 44 + i*rowH
		col := pal.Steel
		if e.Place == 1 {
			col = pal.Yellow
		}
		if e.You {
			gfx.FillRect(dst, x+6, ry-2, w-12, rowH-2, pal.Indigo)
			col = pal.White
		}
		for k, v := range []string{fmt.Sprint(e.Place), e.Name, fmt.Sprint(e.Floor), groupDigits(e.Score), e.Extra} {
			f.DrawShadow(dst, clip(f, v, 230), cols[k], ry, 1, col)
		}
	}
}
