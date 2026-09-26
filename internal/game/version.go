package game

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// Version is the game's version. Release builds set it with
//
//	-ldflags "-X github.com/halpworld/halpwords/internal/game.Version=v1.0.0"
var Version = "dev"

// VersionText describes the build for the title screen and crash reports,
// such as "v1.0.0", or "dev 1a2b3c4" for a build from source.
func VersionText() string {
	if Version != "dev" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}
	rev, dirty := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	switch {
	case rev == "":
		return Version
	case dirty:
		return Version + " " + rev + "+"
	}
	return Version + " " + rev
}

// UserAgent is the game's User-Agent for requests to Halpwords, such as
// "Halpwords/1.2.0 (linux; amd64)" or "Halpwords/dev (js; wasm)".
func UserAgent() string {
	v := strings.TrimPrefix(Version, "v")
	if v == "" || strings.ContainsAny(v, " ()/;") {
		v = "dev"
	}
	return "Halpwords/" + v + " (" + runtime.GOOS + "; " + runtime.GOARCH + ")"
}

// Platform is the operating system and processor, "linux/amd64", for bug
// reports.
func Platform() string { return runtime.GOOS + "/" + runtime.GOARCH }
