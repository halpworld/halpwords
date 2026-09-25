package pkg

import (
	"os/exec"
	"strings"
	"testing"
)

// forbidden are import paths the shared packages must never depend on,
// directly or through another package: Ebitengine and the game's screens.
var forbidden = []string{
	"github.com/hajimehoshi/ebiten",
	"github.com/halpworld/halpwords/internal/game",
	"github.com/halpworld/halpwords/internal/scene",
	"github.com/halpworld/halpwords/internal/gfx",
}

// TestNoGraphicsImports walks every pkg/ package's imports, all the way
// down, so the server can import them without pulling in graphics code.
func TestNoGraphicsImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Deps " "}}`, "./...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 5 {
		t.Fatalf("go list found %d packages, want the shared ones", len(lines))
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		for _, dep := range fields[1:] {
			for _, bad := range forbidden {
				if dep == bad || strings.HasPrefix(dep, bad+"/") || strings.HasPrefix(dep, bad+"@") {
					t.Errorf("%s imports %s", fields[0], dep)
				}
			}
		}
	}
}
