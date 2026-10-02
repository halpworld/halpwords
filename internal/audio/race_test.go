//go:build race

package audio

// raceEnabled is true under the race detector, which skews timings.
const raceEnabled = true
