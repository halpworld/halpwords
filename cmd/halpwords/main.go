// Command halpwords is an 8-bit dungeon crawler for learning to spell words in
// a foreign language.
package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/scene"
)

func main() {
	ebiten.SetWindowTitle("Halpwords")
	ebiten.SetWindowSize(game.ScreenW*2, game.ScreenH*2)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	g, err := game.New(scene.NewTitle)
	if err != nil {
		log.Fatal(err)
	}
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
