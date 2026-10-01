package proc

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// indexedHash returns a sha256 of an indexed image's size, pixels and glow.
func indexedHash(ms ...*Indexed) string {
	h := sha256.New()
	for _, m := range ms {
		h.Write([]byte{byte(m.W), byte(m.H)})
		h.Write(m.Pix)
		for _, g := range m.Glow {
			if g {
				h.Write([]byte{1})
			} else {
				h.Write([]byte{0})
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// The server draws this door on its web pages, so it must not change.
func TestServerDoorUnchanged(t *testing.T) {
	const want = "72d69a30674b4fc66760604172cfe01243664325c6233bf884eae8d66dd9efcb"
	if got := indexedHash(DoorTexture(ThemeFor(1), 7, true)); got != want {
		t.Errorf("DoorTexture(ThemeFor(1), 7, true) hash %s, want %s", got, want)
	}
}

// The texture generators that take a Theme keep their output for the first
// six themes, recorded before the floors got their own looks (#72).
func TestThemeTexturesUnchanged(t *testing.T) {
	want := [6][7]string{
		{
			"77ede0bb76c56d8af44588be449182f56b4b4ef37ebf3c477fe515b71e015da3",
			"f605ea3320811b9f287a8499969b573b5360d869797696c755a70320de791e1f",
			"1962d06cce7ed802e624d9cff92562bb7a9470629f83540c4b6c50c1c58dc266",
			"28faaf60936b441213ac5a13cd713cd242a8a2b00041dd414b441460136853a1",
			"86eb8749fe749ad06f4013a5809954c0909bf42de73419b31ae9cc721a3d5747",
			"b3776ac13ea2cf16ecf9227602ca10a128c8253ff9614078a788160db6b25a94",
			"49e3e3b4700aefaab1ecc893cd66a8aa4888a80e80f7997cd5fcc0cfef60e906",
		},
		{
			"736f9d913fd61cd447c0717d1b44e026f652a9f23567f996128597d7f9f0b8f8",
			"c508739dac529222819754695d11ab9ab0d87b1a7c5c09a29a1059d24e0b8b3f",
			"77a860d4256349d244f32bb134aed2a05378ce7ee23a8e23fbd4f710e90413fe",
			"70d2c5f90a78e6e7706d457c69d2568043762c44ac56db0ff9301144d08f4a83",
			"ca8ccc8cb961a156a639ddf35e4f9d49b2a56c91b4b66a21367575a8166d7354",
			"61ad3cd9fbe04ed002b678b10e2362296681adefe4c0a264099c29dbe8e69e5f",
			"868264b23c92d73b1356881d4b501d09d2256de22467ecdf32aa375aa50d2a1f",
		},
		{
			"3b52ab509d46c6caefb3b529cd8a738283a6e6aaa47cfd634c552d0652da62cc",
			"878ea057df82eee2dadea04ee9ccedde415ce48129946f11100ce1102c33f6f2",
			"3d8c0c08e01aee65055f61e964861690ae720ccc0f4ffca1fe17b414b96a3c17",
			"5fd8bcd91c0b7b7fb780ec2cae48d9cdf9e969f1920d570c3dc7ce47a9d50e95",
			"f026bed94b245c326fce6d718b99b9b00df35c4a753e231c9343b6b419094890",
			"a7a61bee3fcdc970066e76359f9b9281ad877d8b470724688f5b78ddf23778dd",
			"677360e5d399e0ac3aa96a9ab680c3d3b4691b08a982fa663e01c0fae382b4c1",
		},
		{
			"17e3a99a59210cbaa89d9ee42efd37aac4b5934f908d213c98549e6ebddb1400",
			"67eab6b6461c70d6e725a3580e788604522e409ef56e8fe8b8a4f5426b2b57e1",
			"4801d63790e87c91b0cc0de17c06e46550d792c48d646ee54e315daef9ba09ba",
			"758a1656d09ab80af244f9b45b537f3802bef2acc8d9ed49e5e567a554063761",
			"8ce9e50532024a75204138d7e94c4baf5f3a65eafaa5d1617b67f2344458f801",
			"f5d18a21516d660a6f563eeecefa28f3414c97073f61ee894ea0611c9dd15855",
			"677360e5d399e0ac3aa96a9ab680c3d3b4691b08a982fa663e01c0fae382b4c1",
		},
		{
			"0ee1ec3bb82b0eb3ce1a767e3ed7764ebcef6767f064cce3bdfb6d47dd61697b",
			"ca1b4aa4bd93fd898dfc88a7308e0956344cc969fd55bcf93e936146e39c14b5",
			"d1df85f87f95bf7db85270a908c68137503b91e35d17c69034c5a366fdadac06",
			"7c6f95b713ac89c8b5f1ee862362d8f28ed0c464d24bc5d0ebc1e9a53826ef94",
			"a20df7ad9e058c0c3525a46eabba621b540607817dc221087db4e9351f8c1edf",
			"2fde9bb8c34399c5f0de8d45d54a6dc24e5dc3dd772df0790c628e3b52072370",
			"49e3e3b4700aefaab1ecc893cd66a8aa4888a80e80f7997cd5fcc0cfef60e906",
		},
		{
			"be624be0539311a4846904ac4d667423f737bfc7e3e3b296db5264b9bac46a10",
			"cd9539e1e5ab02b0fcdafc85b88d4a910407d6fbf44aced4acdf6953b711867e",
			"3796f616487b5dafcba4d8effcae27c6ac04c776695555549395314adcf9e2d3",
			"516f86fdeceef66e8904454fe2ab395d3312c9e6863119cf02cfad7e8d280c01",
			"e55d7708e97b67c8a16bdbe5a651fe1fb15421bc7588484d850b5e0b4b00a5f8",
			"cd4828ad4a8fca5a16e7179710dde027590065e807a678f926eb41cccf2d70d6",
			"0068a8f6dac20fe639ad3b98a82bcaf2fa6297ef56954414621d765f55121499",
		},
	}
	names := [7]string{"wall 0", "wall 1", "wall 2", "door", "torch", "floor/stairs", "ceiling"}
	for i := range want {
		th := &Themes[i]
		const seed = 1
		got := [7]string{
			indexedHash(WallTexture(th, seed, 0)),
			indexedHash(WallTexture(th, seed+1, 1)),
			indexedHash(WallTexture(th, seed+2, 2)),
			indexedHash(DoorTexture(th, seed+10, false), DoorTexture(th, seed+10, true)),
			indexedHash(TorchWall(th, seed, 0), TorchWall(th, seed, 1), TorchWall(th, seed, 2), TorchWall(th, seed, 3)),
			indexedHash(FloorTexture(th, seed+11), StairsTexture(th, seed+11)),
			indexedHash(CeilingTexture(th, seed+12)),
		}
		for j := range got {
			if got[j] != want[i][j] {
				t.Errorf("%s %s: hash %s, want %s", th.Name, names[j], got[j], want[i][j])
			}
		}
	}
}
