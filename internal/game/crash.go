package game

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/halpworld/halpwords/internal/save"
)

// CrashFile is where the last crash is described, in the user's folder, so
// a player or teacher can send it with a bug report.
const CrashFile = "crash.txt"

// Crash writes what went wrong, and where, to CrashFile.
func Crash(what any, stack []byte) {
	report := fmt.Sprintf("Halpwords %s crashed at %s on %s/%s.\n\n%v\n\n%s",
		VersionText(), time.Now().Format(time.RFC3339), runtime.GOOS, runtime.GOARCH, what, stack)
	if err := save.Write(CrashFile, []byte(report)); err != nil {
		return
	}
	if dir, err := save.Dir(); err == nil {
		fmt.Fprintf(os.Stderr, "A report is in %s. Please send it with a bug report.\n", dir+string(os.PathSeparator)+CrashFile)
	}
}

// guard reports a panic in the game loop before letting it end the game.
// Ebitengine runs the loop on its own goroutine, so main can't catch it.
func guard() {
	if r := recover(); r != nil {
		Crash(r, debug.Stack())
		panic(r)
	}
}
