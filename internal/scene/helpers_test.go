package scene

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

// importList parses a word list as the import screen does. It has no build
// tag so the tests for every platform (including wasm vet) can use it.
func importList(t *testing.T, src string) *words.List {
	t.Helper()
	l, err := words.ParseImport(strings.NewReader(src), "import.txt")
	if err != nil {
		t.Fatal(err)
	}
	return l
}
