package combat

import "github.com/halpworld/halpwords/pkg/words"

// ArmorBlocks reports whether an armored monster shrugs off an attack graded
// tier. Only exact spelling gets through: accent slips and grazes do
// nothing.
func ArmorBlocks(tier words.Tier) bool { return tier < words.Correct }

// SwiftTime is the share of the usual dodge time a swift monster allows.
const SwiftTime = 0.7

// A ghostly monster's word is shown in full for FadeStart seconds, then
// fades out until it is gone at FadeEnd.
const (
	FadeStart = 1.5
	FadeEnd   = 2.0
)

// Visibility is how visible a ghostly monster's word is t seconds after it
// appeared, from 1 (clear) to 0 (gone).
func Visibility(t float64) float64 {
	return clamp((FadeEnd-t)/(FadeEnd-FadeStart), 0, 1)
}

// Mirror writes s backwards, for mirrored monsters.
func Mirror(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
