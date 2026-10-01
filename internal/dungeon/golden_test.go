package dungeon

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// The golden hashes below were recorded before the floors got their own
// looks (#72). The look of a floor must never change the floor itself: the
// same seed has to give the same map, monsters and treasure, or Daily
// Dungeons, Seed Challenges and saved games would differ between versions.
// If one of these fails, a change has reached the generator.

// levelHash returns a sha256 of everything about a floor that play and
// saved games depend on.
func levelHash(t *testing.T, l *Level) string {
	t.Helper()
	h := sha256.New()
	fmt.Fprintf(h, "%dx%d depth %d seed %d\n", l.W, l.H, l.Depth, l.Seed)
	for _, tile := range l.tiles {
		h.Write([]byte{byte(tile)})
	}
	for _, on := range l.Torches {
		if on {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
	}
	fmt.Fprintf(h, "\nrooms %v start %v %v exit %v\n", l.Rooms, l.Start, l.StartDir, l.Exit)
	for _, p := range sortedPoints(l.Chests) {
		writeJSON(t, h, "chest", p, l.Chests[p])
	}
	for _, p := range sortedPoints(l.Features) {
		writeJSON(t, h, "feature", p, l.Features[p])
	}
	for _, m := range l.Monsters {
		fmt.Fprintf(h, "monster %s %v seed %d hp %d/%d atk %d traits %d facing %v title %q\n",
			m.Kind.Name, m.At, m.Seed, m.HP, m.MaxHP, m.ATK, m.Traits, m.Facing, m.Title)
		if m.Loot != nil {
			writeJSON(t, h, "loot", m.At, m.Loot)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeJSON(t *testing.T, h hash.Hash, what string, p Point, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(h, "%s %v %s\n", what, p, b)
}

// sortedPoints returns a map's keys in reading order, so the hash does not
// depend on map iteration.
func sortedPoints[V any](m map[Point]V) []Point {
	return slices.SortedFunc(maps.Keys(m), func(a, b Point) int {
		return cmp.Or(cmp.Compare(a.Y, b.Y), cmp.Compare(a.X, b.X))
	})
}

func checkGolden(t *testing.T, name string, l *Level, want string) {
	t.Helper()
	if got := levelHash(t, l); got != want {
		t.Errorf("%s: hash %s, want %s", name, got, want)
	}
}

func TestGoldenGeneratedFloors(t *testing.T) {
	want := map[uint64][]string{
		1: {
			"b77682fa37c4b7d5bf1ede81aa0dcdb08fb55593cbd6181adeec4e1d022c148d",
			"968bdf8bd37d65ae810cfa5da26f9672343f38ac19b62c963fa36df46e1c415c",
			"c6db23a37357b32c787d77fbc9f071aec0a683eb190185038173b4ce02bc6b54",
			"4fde097988f88db8d9d296c92895a0a5436b1200e39176233f73ac9b02b54f0e",
			"28abee4a2477c86ae96f2f199e62e0afb40e0c3507b71db05c29c9e9dfbecc2c",
			"1de3d128e2542d5466756cd2283a4747610e1081e1045112cf54cc4c7d8a9678",
			"2a52911f59ebfd5528d876a7f4f94602408fc95b430fc226b476caee855cc09b",
			"1b1090a1b89c54a0ff65a115abdc2f48cf8105bba1a16bc1fa3474f8ecfa58a3",
			"081ee144c3c74cf8887f1bdee8d2aaa3a26b9e5b64dea38015639e2b144fbf29",
			"e1b92c506a9185a8a003c23d5a2e3e0ee0395f01669e8ce881116a6e063c8b49",
			"72187e9f0d4194915c85f1e14714204561b03413a2a5a502bb088f56eb335113",
			"772bebff79863efb6ef1f363d198a1fb2e8fd8faf6558dd193c176343f0d3137",
		},
		0x5eed_1234: {
			"c650a2c703565b57db072971338a3c424c72d1b772de6f0c3c76d56593d2c23b",
			"4eb52a1d147c2693906e3b3e9bd83f9b3907774ec1f3ae76896bc648d7041ba2",
			"2b63dcd3e47ded2a5b87678a501d5caca34b6a31ce4885166fc150103adcbed4",
			"377d79d1c7ac750c7355922eca1b8b6cb79df20565b1e644d427174ea80ff891",
			"68f76531c863a8a2ad89b2c048a0f1871ebe7eff0cebfa3befaee4ad047025d3",
			"50d840828671aa278a5bcfde32782d390ae0d737066a7133e6883087bdd45f1c",
			"7bcbb35d45c08a734300c20a4a0256c286ff358e0c515808899fc4894dc76cb1",
			"60bbe623251c3028ce90a5ae1a84aaf6bd65db9e9cb823b35ee8412e9a1b748a",
			"f2075297bea49bb5574735f1a673ae6205984cc24260f1971446f754d225b33b",
			"ed67057bbb8b9a864a71e68bdc78a29a8f145965540cbad000929d4fea5cb767",
			"e4ce567019a30b67fd9ab3c9604be3329410b5913a461f3fac266b3c2ee76556",
			"0b9ffa202e08c0cb948bec5c172b3119cace34ef4f6d7c644e7dd7396a7d20f6",
		},
	}
	if !fusedMultiplyAdd {
		// NewMonster's HP and attack scaling, float64(k.HP) * (1 +
		// 0.15*float64(depth-1)), is fused into one multiply-add on arm64
		// and rounds differently elsewhere, so a monster's stats can be one
		// apart at depth 10. These are the hashes on amd64 and wasm.
		want[1][9] = "3f298f3bea586fb45e27470521af71276f58fc6d47d196110a3a75cdd4956fcf"
		want[0x5eed_1234][9] = "030a93d5c88abd32e1ee4eaa301cea8f0676029c1fa5102aabf01d98003155dc"
	}
	for _, seed := range slices.Sorted(maps.Keys(want)) {
		for depth := 1; depth <= 12; depth++ {
			checkGolden(t, fmt.Sprintf("seed %#x depth %d", seed, depth), Generate(seed, depth), want[seed][depth-1])
		}
	}
}

// goldenWords is a small fixed word list for the Daily seeds.
var goldenWords = []words.Entry{
	{Prompt: "the cat", Answers: []string{"le chat"}},
	{Prompt: "the dog", Answers: []string{"le chien"}},
	{Prompt: "the house", Answers: []string{"la maison"}},
	{Prompt: "water", Answers: []string{"l'eau"}},
}

func TestGoldenDailyFloors(t *testing.T) {
	cases := []struct {
		date, lang string
		want       [3]string // floors 1, 2 and 3
	}{
		{"2026-01-01", "fr", [3]string{"8a2e24012ec17f8272a3a595882c1f682c60a7f86c6247afbddc7ec5bbcf5f35", "f57b94b1804e208e690fc674058216f9ccdc8f8b99219f84245a4d45a239756b", "e0b93b040c5f818893c374c93f6b4c94907a2e79fa4905f5fa8e2a5edfeb3b3f"}},
		{"2026-10-01", "fr", [3]string{"7bff310c18fb58dba5bb1f3162e5ef84078ed2f753e52505c0136869fccb36a1", "a118a6a26ec06dbbf8e6cabe4b8686ba0c083b44880f1abbe874825d07ec527a", "589224aefeb1772e6309451e496914c48e1b22a2a31615637923e801203c3701"}},
		{"2026-10-01", "la", [3]string{"4068db364d38443531d0ce9a17d938c829e0f3090522091fcc3e26a0ed886253", "e52279798184745d7a81d51e2452ff696997d495f3c5bc9accdcab517bd911d2", "98e21c84bbbf9d0c2c5515b39ab090496c2f18cd880f8b1510fb0d1c8de76d09"}},
	}
	for _, c := range cases {
		day, err := time.Parse(time.DateOnly, c.date)
		if err != nil {
			t.Fatal(err)
		}
		seed := compete.DailySeed(day, c.lang, goldenWords)
		for i, want := range c.want {
			depth := i + 1
			// As the game does: run.floorSeed with no falls.
			checkGolden(t, fmt.Sprintf("daily %s %s floor %d", c.date, c.lang, depth), Generate(seed*31+uint64(depth)*7919, depth), want)
		}
	}
}

func TestGoldenSeedChallenge(t *testing.T) {
	code := compete.Share{Lang: "fr", Seed: 0x2a2a2a, Floor: 5, Score: 1234}.Code()
	seed, err := compete.SeedFromCode(code)
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "seed challenge floor 1", Generate(seed*31+7919, 1), "e2e8211f85d67c72109265503e27408f05b05544e24bbf353aa0d7fe940a8868")
	checkGolden(t, "seed challenge floor 4", Generate(seed*31+4*7919, 4), "c49facb5e0b3d3ae79ee6946e09d01ccb4a12b3ba78cd52780aefde4b2b5890c")
}

func TestGoldenHardcore(t *testing.T) {
	for _, depth := range []int{2, 5, 8} {
		l := Generate(77, depth)
		l.Harden()
		checkGolden(t, fmt.Sprintf("hardcore depth %d", depth), l, map[int]string{2: "fd7cb58c633da293c361ae9ef2d0c4ca5c94087ca4253524549f036f38b33279", 5: "b07a247a6b0c966cd660a2a5d727d4c70bde3d69291b38b2b4d2df7a1ec9e336", 8: "d846529d6eec78b94745e00b506328a3c7c457639436e5c979dc6dc0114b010c"}[depth])
	}
}

func TestGoldenFromMap(t *testing.T) {
	l, err := FromMap(vault(), 7)
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "vault map", l, "2be245830015ecdc3a094d5a3176c58ca65a3d27a1c1c425d8b05edb67ae5e86")
}
