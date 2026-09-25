package game

import "runtime/debug"

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
