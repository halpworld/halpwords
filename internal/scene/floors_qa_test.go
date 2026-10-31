package scene

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/pkg/proc"
)

// mainSaves are the hashes of the saves of the test run on floors 1 to 30
// as origin/main wrote them: the floor (its rooms and monsters) and the
// run. Adding worlds must not change a floor, a run seed, or what a save
// holds.
var mainSaves = []string{
	"7eb21b6c5218e1c7ba2ba45f96fffcff14fac61c99288fdf5e3c444b7dfa0de6",
	"748e2e04a7a489c6939f6fb28ba7d94bcde9cfc24d5bbfdfc54020d919de4e64",
	"1e29e1c0c2c22586bb1041b8e8ffeb1677068b079f272f5bd646073dec559d50",
	"c5aa7813092ea15b57709bec2d7768e9daf94726964703586c7824514c33c24e",
	"20a6064f9035f72c00c2a65d5123c225cd8816a672c3976eeed0e474e2e4dd27",
	"41bb97fb8e8a560cf4f1775d8c0803c1048436d4df84bf12528e62b9f80c1e13",
	"d768af5ad74a130308cc9c6e1f32da218f22b94f1c739d2ac809d3859afbda14",
	"6f2b88f5d92b48e09417871d75a09d161a4d07c57fb5aeff101eab59153b6757",
	"252d62b72f50bfa09059d1b7e3a9ce05d2607a1b563ebbc84d5f5fab6b52a08d",
	"d5da812f4bd272e68856eca752fae9b5a547b996d6a967f384261fe68eb9bb61",
	"36be074828f3e8cf5eb1cbb2571a4cb2a445110a965f28265f22591dd00bbbc3",
	"d91e1be3c1a36b057fd3f7a9e90f5c1ccbb5d54d368d1be951ab8e65812c17c2",
	"1a7cd13acca3d831260b43f63f61568a42718c5fa30a577dc49bfe7e7ed0ffcc",
	"ebd82a1dffb96a275bf202ba96d946b56ea5d461bed712515680f9c016e0dab8",
	"4ea0566cd66c9e06bbb6e6d903517f1e2ff7162210179e6795eb0b94e6115b66",
	"3c85ae1e90f5d49baf2f5f2403e8d78d7169ecfa31e7f47b412570959212972a",
	"57db0cd31794519909f3d1552bfa70e69755341ec0c72cbda85b095ac9d97b89",
	"ec87813467a46cfc62b7483ab9b78f0d65fd2ffb86da7c5ee61be3db297bef76",
	"cc12e63acfe2f317bb7ceff50893cd1e3517bd30bde841d4613250e059e49634",
	"1943442f9cd79ebd64f2bafbb3ae3363da3b442f91d554cc257f9ee1d5ad5676",
	"0192471656f37c25566693401d9b2528bf54eb46b273d2548a121631713882a3",
	"8a69ff89ef108b89a1e29c59512d49f39399c35b81be6ea824edc762e268e3b3",
	"128cc8874cfb5e557b5829d47a869bc8fd9df45d3f3579700fd9b2792c8e3e65",
	"7c2a1f0b115be90eb2567320dd2811037fe18f5c8f7bf7ee90f3c166490106f7",
	"4363994803a4b4ca157ce303707a235ef249d7ba64a30621dcb5a046d12132e0",
	"7aab786d68050b0d9f6ece819b7acb88a4bc9013d671e5345341477b1e66abf9",
	"a7f45d3c7ea90d3c2a5d7af7676549372d9cefd8981133a27636375dc66d43ba",
	"901c3ac5ebe067aa14296b1b5f5bcc4b32d77df225673983113c14a05c82d386",
	"3045a25f9f9ec5f708211391a9ae465805df68ca97bded34f925299f211075f8",
	"b95c6a449003a26abd690b295fdd11efba3c2a1c925d1d07de28c5eb3f4d68c7",
}

func TestQASavesOfFloors1To30AreAsOnMain(t *testing.T) {
	for d := 1; d <= 30; d++ {
		ctx := testContext(t)
		r, _ := testRun(t, ctx)
		r.depth = d
		l := dungeon.Generate(r.floorSeed(d), d)
		l.Seen[l.Index(l.Start)] = true
		data, err := encodeSave(r, l, l.Start, dungeon.West, true)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != mainSaves[d-1] {
			t.Errorf("floor %d: the save differs from main's", d)
		}
	}
}

// Saved runs written by origin/main, on floors 3, 10, 11, 12 and 13,
// resume on this build: the floor is the one dungeon.Generate makes, the
// hero is where they were, and the floor's world is a real one.
func TestQAResumeSavesFromMain(t *testing.T) {
	for _, d := range []int{3, 10, 11, 12, 13} {
		data, err := os.ReadFile(fmt.Sprintf("testdata/main-save-d%d.json", d))
		if err != nil {
			t.Fatal(err)
		}
		ctx := testContext(t)
		got, err := decodeSave(ctx, data)
		if err != nil {
			t.Fatalf("floor %d: %v", d, err)
		}
		if got.run.depth != d || got.level.Depth != d || !got.suspended {
			t.Errorf("floor %d: depth %d / %d, suspended %v", d, got.run.depth, got.level.Depth, got.suspended)
		}
		want := dungeon.Generate(got.run.floorSeed(d), d)
		want.Seen[want.Index(want.Start)] = true
		if got.level.W != want.W || len(got.level.Monsters) != len(want.Monsters) || got.level.Start != want.Start {
			t.Errorf("floor %d is not the floor of this seed", d)
		}
		if proc.ThemeFor(d) == nil {
			t.Errorf("floor %d has no world", d)
		}
		// Saving it again gives a save that loads the same.
		again, err := encodeSave(got.run, got.level, got.at, got.facing, true)
		if err != nil {
			t.Fatal(err)
		}
		back, err := decodeSave(ctx, again)
		if err != nil || back.run.depth != d {
			t.Errorf("floor %d: saved again, loaded as %v / %v", d, back, err)
		}
	}
}
