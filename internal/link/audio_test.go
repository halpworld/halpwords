package link

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	waitAudio(t, c)
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
	waitAudio(t, c)
	if _, ok := c.Pronunciation("fr", dog); ok {
		t.Error("a word is said before its pack came")
	}
	status = 200
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitAudio(t, c)
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
	waitAudio(t, c)
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
	waitAudio(t, c)
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
	waitAudio(t, c)
	if _, ok := c.Pronunciation("fr", dog); ok {
		t.Error("a pack for version 3 is used for version 4")
	}
	// Unlinking removes packs with the lists.
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Text: animals})
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitAudio(t, c)
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

// waitAudio waits for the background audio download to end.
func waitAudio(t *testing.T, c *Client) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(2 * time.Millisecond) {
		c.mu.Lock()
		busy := c.audioRunning
		c.mu.Unlock()
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the audio download never ended")
		}
	}
}

// trickle serves the fake, except that audio packs come in small pieces
// with pauses between, as on a slow link.
func trickle(f *fake, t *testing.T, pieces int, pause time.Duration) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/audio") {
			f.srv.Config.Handler.ServeHTTP(w, r)
			return
		}
		data := animalsPack(t, 3)
		w.Header().Set("Content-Type", audiopack.MediaType)
		for i := 0; i < pieces; i++ {
			w.Write(data[i*len(data)/pieces : (i+1)*len(data)/pieces])
			w.(http.Flusher).Flush()
			time.Sleep(pause)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A pack that takes longer than a request may, but keeps arriving, is
// downloaded; and the sync lock isn't held meanwhile.
func TestSlowAudioPackDownloads(t *testing.T) {
	oldReq, oldIdle := requestTimeout, packIdle
	requestTimeout, packIdle = 150*time.Millisecond, 400*time.Millisecond
	t.Cleanup(func() { requestTimeout, packIdle = oldReq, oldIdle })
	f, c, _, _ := setup(t)
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Words: 2, Text: animals})
	f.mu.Lock()
	f.me["pronunciation"] = map[string]any{"available": true}
	f.audio = func(id, version string) (int, []byte) { return 200, nil }
	f.mu.Unlock()
	srv := trickle(f, t, 8, 60*time.Millisecond) // about 500 ms in all
	c.server = srv.URL
	if err := c.LinkNow(context.Background(), "abcd efgh"); err != nil {
		t.Fatal(err)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Mid-download, a sync, an AI call or a join can take the lock.
	time.Sleep(150 * time.Millisecond)
	if !c.syncMu.TryLock() {
		t.Error("syncMu is held while a pack downloads")
	} else {
		c.syncMu.Unlock()
	}
	waitAudio(t, c)
	if _, ok := c.Pronunciation("fr", dog); !ok {
		t.Error("the slow pack didn't come")
	}
}

// A pack download that stops for too long ends.
func TestStalledAudioPackEnds(t *testing.T) {
	old := packIdle
	packIdle = 100 * time.Millisecond
	t.Cleanup(func() { packIdle = old })
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.me["pronunciation"] = map[string]any{"available": true}
	f.audio = func(id, version string) (int, []byte) {
		time.Sleep(500 * time.Millisecond)
		return 200, animalsPack(t, 3)
	}
	f.mu.Unlock()
	start := time.Now()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitAudio(t, c)
	if time.Since(start) > 450*time.Millisecond {
		t.Errorf("the stalled download took %v", time.Since(start))
	}
	if _, ok := c.Pronunciation("fr", dog); ok {
		t.Error("a pack that never came is used")
	}
}

// A list whose pack keeps failing is asked for less and less often.
func TestAudioPackBacksOff(t *testing.T) {
	f, c, _, clk := linked(t)
	asked := 0
	f.mu.Lock()
	f.me["pronunciation"] = map[string]any{"available": true}
	f.audio = func(id, version string) (int, []byte) { asked++; return 500, nil }
	f.mu.Unlock()
	sync := func() {
		t.Helper()
		if err := c.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
		waitAudio(t, c)
		clk.add(SyncEvery)
	}
	sync() // the first failure is tried again at the next sync
	sync()
	if asked != 2 {
		t.Fatalf("asked %d times", asked)
	}
	sync() // then it waits
	sync()
	if asked != 2 {
		t.Errorf("asked %d times in the backoff", asked)
	}
	clk.add(time.Hour)
	sync()
	if asked != 3 {
		t.Errorf("not asked again after the wait: %d", asked)
	}
	// Once it works, the list is no longer held back.
	f.mu.Lock()
	f.audio = func(id, version string) (int, []byte) { asked++; return 200, animalsPack(t, 3) }
	f.mu.Unlock()
	clk.add(24 * time.Hour)
	sync()
	if _, ok := c.Pronunciation("fr", dog); !ok {
		t.Error("the pack didn't come after the wait")
	}
}
