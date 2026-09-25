package game

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestCRTShaderCompiles(t *testing.T) {
	if _, err := ebiten.NewShader([]byte(crtSource)); err != nil {
		t.Fatal(err)
	}
}
