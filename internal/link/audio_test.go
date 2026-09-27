package link

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/pkg/audiopack"
)

func animalsPack(t *testing.T, version int) []byte {
	t.Helper()
	wav := func(n int16) []byte { return audiopack.EncodeWAV([]int16{n, -n, n}, 16000) }
	data, err := audiopack.Encode(&audiopack.Pack{Language: "fr", List: "lst_animals", Version: version, Words: []audiopack.Word{
		{Text: "le chien", SoundOut: "luh shee-AN", Audio: wav(100)},
		{Text: "le chat", Audio: wav(200)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAudioPacks(t *testing.T) {
	f, c, st, _ := linked(t)
	var asked []string
	status := 202
	f.mu.Lock()
	f.audio = func(id, version string) (int, []byte) {
		asked = append(asked, id+"@"+version)
		if status != 200 {
			return status, nil
		}
		return 200, animalsPack(t, 3)
	}
	f.mu.Unlock()
	// Not on the plan: nothing is asked for, nothing is said.
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 0 {
		t.Fatalf("asked for audio without pronunciation: %v", asked)
	}
	f.mu.Lock()
	f.me["pronunciation"] = map[string]any{"available": true}
	f.mu.Unlock()
	// Still being made: asked again next time.
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Pronunciation("fr", dog); ok {
		t.Error("a word is said before its pack came")
	}
	status = 200
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, " ") != "lst_animals@3 lst_animals@3" {
		t.Errorf("asked %v", asked)
	}
	if !st.has("assigned/lst_animals.audio") {
		t.Fatalf("the pack isn't kept: %v", keys(st))
	}
	wav, ok := c.Pronunciation("fr", dog)
	if !ok || !bytes.Equal(wav, audiopack.EncodeWAV([]int16{100, -100, 100}, 16000)) {
		t.Errorf("dog: %v", ok)
	}
	if _, ok := c.Pronunciation("fr", own); ok {
		t.Error("a word from no assigned list is said")
	}
	// Kept: not asked again, and read from the store after a restart.
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 2 {
		t.Errorf("asked again: %v", asked)
	}
	c2 := Open(Options{Store: st, Server: f.srv.URL})
	if _, ok := c2.Pronunciation("fr", cat); !ok {
		t.Error("the kept pack isn't read after a restart")
	}
	// A new version needs a new pack; the old one goes.
	v4 := strings.Replace(animals, "version: 3", "version: 4", 1)
	f.setLists(wireList{ID: "lst_animals", Version: 4, Title: "Animals", Language: "fr", Text: v4})
	status = 500
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st.has("assigned/lst_animals.audio") {
		t.Error("the old version's pack is kept")
	}
	if _, ok := c.Pronunciation("fr", dog); ok {
		t.Error("the old version's pack is used")
	}
	// A pack for the wrong version is refused.
	status = 200
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Pronunciation("fr", dog); ok {
		t.Error("a pack for version 3 is used for version 4")
	}
	// Unlinking removes packs with the lists.
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Text: animals})
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !st.has("assigned/lst_animals.audio") {
		t.Fatal("no pack for version 3")
	}
	c.Unlink()
	if st.has("assigned/lst_animals.audio") {
		t.Error("the pack is kept after unlinking")
	}
	if _, ok := c.Pronunciation("fr", dog); ok {
		t.Error("a word is said after unlinking")
	}
}
