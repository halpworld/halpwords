package raycast

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/pkg/proc"
)

// mainFrames are frames of floors 1 to 10 (seed 5, 96x60, tick 40) as
// origin/main drew them, before the Sunken Temple and Candy Caverns.
var mainFrames = []string{
	"cff3ffe776df1fe6048f2967f0747a2b47fb8d514daf964d784140b0c8126cd1",
	"87d5b58f7e15fc40a96372f762726f2c0502ff60dbdb6f08eee747fb70e8c6b7",
	"81b72444703dd6a3b2eb23174d0fd72727476f3d98bf2a5e29f474fced00a844",
	"69ee2f9d5be1371715bc1760b2dc21d2c31975136393eab87220c31c8f758f7f",
	"28e9c5afae3e550d9f6540fef7df60279a508a9a0d9cae013f30db638f47e2ad",
	"0eaeecf44e76adce5ce00bef7df009fed3d587c7ca35651c600e1bd792ca473e",
	"55730811794dd0df38759e315c9d9225c6d9f6a4257cc2cc1b4450ab04effb7f",
	"fd64d2064810fafd441b68e819b5531ec59b9b48c1d07cfcd197610ee5d4514e",
	"0ba70ba16dbdfc1a15857936f16d74056ebac07d734da393f99e366d03d05ff3",
	"14511dc81c6f96f5a907347d2fc586a12a8c12525ee0b17463fb179f4c4a4bdb",
}

func floorFrame(depth int) []byte {
	l := dungeon.Generate(5, depth)
	th := proc.ThemeFor(depth)
	cam := startCamera(l)
	r := New(96, 60)
	r.SetWorld(th)
	r.Render(l, NewTextures(th, l.Seed), cam, Decor(l, th), 40)
	r.DrawParticles(cam, 40)
	return slices.Clone(r.Img.Pix)
}

func TestQAFloorsOneToTenRenderAsOnMain(t *testing.T) {
	for d, want := range mainFrames {
		if got := fmt.Sprintf("%x", sha256.Sum256(floorFrame(d+1))); got != want {
			t.Errorf("floor %d renders differently from main: %s", d+1, got)
		}
	}
}

// Every floor from 1 to 30 renders a frame, twice the same, with something
// drawn that is not one flat colour.
func TestQAEveryFloorRendersDeterministically(t *testing.T) {
	for d := 1; d <= 30; d++ {
		a, b := floorFrame(d), floorFrame(d)
		if !slices.Equal(a, b) {
			t.Errorf("floor %d: frames differ", d)
		}
		colours := map[[4]byte]bool{}
		for i := 0; i+4 <= len(a); i += 4 {
			colours[[4]byte(a[i:i+4])] = true
		}
		if len(colours) < 8 {
			t.Errorf("floor %d: only %d colours in the frame", d, len(colours))
		}
	}
}
