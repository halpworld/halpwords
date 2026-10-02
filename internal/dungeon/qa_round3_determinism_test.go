package dungeon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"testing"

	"github.com/halpworld/halpwords/pkg/compete"
)

func allKinds() []*Kind {
	var ks []*Kind
	for i := range Kinds {
		ks = append(ks, &Kinds[i])
	}
	for i := range BossKinds {
		ks = append(ks, &BossKinds[i])
	}
	return append(ks, &MimicKind)
}

// Every kind at every depth has exactly the integer stats, so a float
// regression is caught on any platform.
func TestQANewMonsterIntegerFormulaTable(t *testing.T) {
	for _, k := range allKinds() {
		for depth := 1; depth <= 50; depth++ {
			m := NewMonster(k, depth, Point{}, 1)
			hp := k.HP * (100 + 15*(depth-1)) / 100
			atk := k.ATK * (100 + 22*(depth-1)) / 100
			if m.HP != hp || m.MaxHP != hp || m.ATK != atk {
				t.Errorf("%s depth %d: hp %d/%d atk %d, want %d/%d", k.Name, depth, m.HP, m.MaxHP, m.ATK, hp, atk)
			}
		}
	}
}

// Hand-computed spot values, so the formula itself cannot drift.
func TestQANewMonsterSpotValues(t *testing.T) {
	m := NewMonster(&BossKinds[0], 50, Point{}, 1) // 46*835/100, 6*1178/100
	if m.HP != 384 || m.ATK != 70 {
		t.Errorf("Slime King depth 50: hp %d atk %d, want 384 70", m.HP, m.ATK)
	}
	m = NewMonster(&Kinds[0], 1, Point{}, 1)
	if m.HP != Kinds[0].HP || m.ATK != Kinds[0].ATK {
		t.Errorf("depth 1 must be the base stats, got %d/%d", m.HP, m.ATK)
	}
	m = NewMonster(&Kinds[0], 3, Point{}, 1) // 10*130/100, 3*144/100
	if m.HP != 13 || m.ATK != 4 {
		t.Errorf("Green Slime depth 3: hp %d atk %d, want 13 4", m.HP, m.ATK)
	}
}

// One recorded hash of the whole stat table.
func TestQAMonsterStatTableGolden(t *testing.T) {
	h := sha256.New()
	for _, k := range allKinds() {
		for depth := 1; depth <= 50; depth++ {
			m := NewMonster(k, depth, Point{}, 1)
			fmt.Fprintf(h, "%s %d %d %d\n", k.Name, depth, m.HP, m.ATK)
		}
	}
	const want = "3ce1251bebee6c2910dbe98cd195360ef5b55cb1d6fc17c14ab99920bb6e9898"
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		t.Errorf("stat table hash %s, want %s", got, want)
	}
}

// MaxDamage must cover everything a legitimate run can deal: every
// monster's hit points (damage counts no more than that), healers twice.
func TestQAMaxDamageCoversLegitimateRuns(t *testing.T) {
	seeds := []uint64{1, 2, 3, 0x5eed_1234, 1<<30 - 1, 987654321}
	for _, seed := range seeds {
		cum := 0
		for depth := 1; depth <= 40; depth++ {
			l := Generate(seed, depth)
			n := len(l.Monsters)
			for _, m := range l.Monsters {
				cum += m.MaxHP
			}
			// A boss is placed on the floor too (counted in Monsters or
			// not, add one worst-case boss), and a mimic wakes per chest.
			boss := NewMonster(BossFor(depth), depth, Point{}, 1)
			cum += boss.MaxHP
			n++
			for range l.Chests {
				cum += NewMonster(&MimicKind, depth, Point{}, 1).MaxHP
				n++
			}
			if n > 3+depth+1+compete.MaxChests {
				t.Errorf("seed %d depth %d: %d monsters, MaxDamage assumes at most %d", seed, depth, n, 3+depth+1+compete.MaxChests)
			}
			if got := compete.MaxDamage(depth); got < 2*cum {
				t.Errorf("seed %d floor %d: MaxDamage %d < 2 x %d total monster HP", seed, depth, got, cum)
			}
		}
	}
	// No kind has more base HP than the bound assumes.
	for _, k := range allKinds() {
		if k.HP > 80 {
			t.Errorf("%s has %d HP, MaxDamage assumes at most 80", k.Name, k.HP)
		}
	}
	// And it never shrinks as floors go deeper.
	for f := 1; f < 60; f++ {
		if compete.MaxDamage(f+1) <= compete.MaxDamage(f) {
			t.Errorf("MaxDamage not increasing at %d", f)
		}
	}
}

func TestQAGenerateClampsDepth(t *testing.T) {
	for _, seed := range []uint64{1, 42, 0x5eed_1234} {
		want := levelHash(t, Generate(seed, 1))
		for _, d := range []int{0, -1, -7, math.MinInt32} {
			l := Generate(seed, d)
			if l.Depth != 1 {
				t.Errorf("seed %d depth %d: Level.Depth %d, want 1", seed, d, l.Depth)
			}
			if got := levelHash(t, l); got != want {
				t.Errorf("seed %d depth %d differs from depth 1", seed, d)
			}
		}
	}
}

// A seed above 30 bits is the dungeon of its masked seed code, and the
// share code round-trips (this mirrors what HALPWORDS_SEED does).
func TestQAWideSeedEqualsMaskedCode(t *testing.T) {
	mask := uint64(1)<<compete.SeedBits - 1
	for _, wide := range []uint64{1<<30 + 5, 0xdead_beef_cafe, math.MaxUint64, 1 << 63} {
		masked := wide & mask
		code := compete.SeedCode(wide)
		if code != compete.SeedCode(masked) {
			t.Errorf("seed %d: code %q differs from masked code", wide, code)
		}
		back, err := compete.ParseSeed(code)
		if err != nil || back != masked {
			t.Errorf("seed %d: ParseSeed(%q) = %d, %v; want %d", wide, code, back, err, masked)
		}
		for depth := 1; depth <= 6; depth++ {
			if levelHash(t, Generate(back, depth)) != levelHash(t, Generate(masked, depth)) {
				t.Errorf("seed %d depth %d: dungeon from code differs", wide, depth)
			}
		}
		share := compete.Share{Lang: "fr", Seed: masked, Floor: 4, Score: 123}
		got, err := compete.SeedFromCode(share.Code())
		if err != nil || got != masked {
			t.Errorf("share code round trip of %d: %d, %v", masked, got, err)
		}
	}
}

// Golden floors and a golden stat sum at depths and seeds the older golden
// test does not use (deep floors, wide seeds). Runs under wasm too.
func TestQAGoldenDeepFloors(t *testing.T) {
	h := sha256.New()
	for _, seed := range []uint64{7, 1<<30 - 1, 0x2bad_f00d} {
		for _, depth := range []int{0, 1, 6, 15, 30, 50} {
			fmt.Fprintf(h, "%d %d %s\n", seed, depth, levelHash(t, Generate(seed, depth)))
		}
	}
	const want = "c7707aef1ef5962a1e5dd82c8d11465051dba99fd01d43065564279069bde262"
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		t.Errorf("deep floor hash %s, want %s", got, want)
	}
}

// Chances are exact to the bit on every platform: the fused multiply-add
// must not change them.
func TestQAChancesStable(t *testing.T) {
	for depth := 1; depth <= 20; depth++ {
		g := min(0.4, 0.2+float64(0.03*float64(depth-1)))
		if GearChance(depth) != g {
			t.Errorf("GearChance(%d) = %v, want %v", depth, GearChance(depth), g)
		}
	}
	if GearChance(1) != 0.2 || GearChance(100) != 0.4 || MimicChance(1) != 0 || MimicChance(100) != 0.35 {
		t.Error("chance bounds")
	}
}
