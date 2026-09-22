// Package assets holds the files embedded into the game binary.
package assets

import "embed"

// UnifontHex is a subset of GNU Unifont in .hex format (SIL OFL 1.1, see
// fonts/OFL-1.1.txt).
//
//go:embed fonts/unifont-subset.hex
var UnifontHex []byte

// Words contains the starter word lists, one .txt file per list.
//
//go:embed words/*.txt
var Words embed.FS
