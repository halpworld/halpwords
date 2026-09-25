// Command halpwords is an 8-bit dungeon crawler for learning to spell words in
// a foreign language.
package main

import (
	"image"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
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

	g, err := game.New(scene.NewTitle)
	if err != nil {
		game.Crash(err, nil)
		log.Fatal(err)
	}
	if err := ebiten.RunGame(g); err != nil {
		game.Crash(err, nil)
		log.Fatal(err)
	}
}
