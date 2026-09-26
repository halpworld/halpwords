// Command halpwords is an 8-bit dungeon crawler for learning to spell words in
// a foreign language.
package main

import (
	"image"
	"log"
	"net/http"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/playtest"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/internal/scene"
	"github.com/halpworld/halpwords/pkg/proc"
)

func main() {
	ebiten.SetWindowTitle("Halpwords")
	ebiten.SetWindowSize(game.ScreenW*2, game.ScreenH*2)
	ebiten.SetWindowSizeLimits(game.ScreenW, game.ScreenH, -1, -1)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	var icons []image.Image
	for _, size := range []int{16, 32, 48, 64, 128, 256} {
		icons = append(icons, proc.IconAt(size))
	}
	ebiten.SetWindowIcon(icons) // macOS shows the app's own icon instead

	g, err := game.New(firstScene())
	if err != nil {
		game.Crash(err, nil)
		log.Fatal(err)
	}
	err = ebiten.RunGame(g)
	g.Close() // sends the last answers to a linked account
	if err != nil {
		game.Crash(err, nil)
		log.Fatal(err)
	}
}

// firstScene is the title, or in the web game a play-test when the page's
// address names a quest (internal/playtest). A play-test keeps every file
// in memory from the start, so the player's own saves are neither read
// nor changed.
func firstScene() func(*game.Context) game.Scene {
	page := playtest.Page()
	if page == "" {
		return scene.NewStart
	}
	u, err := playtest.FromPage(page)
	switch {
	case err != nil:
		save.UseMemory()
		return scene.PlaytestError(err)
	case u != nil:
		save.UseMemory()
		return scene.NewPlaytest(http.DefaultClient, u)
	}
	return scene.NewStart
}
