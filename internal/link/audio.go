package link

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/halpworld/halpwords/pkg/audiopack"
	"github.com/halpworld/halpwords/pkg/words"
)

// MePronunciation says whether the learner's account can hear its
// assigned lists' words (GET /api/v1/lists/{id}/audio): the account's
// plan includes pronunciation and the server has a voice for it.
type MePronunciation struct {
	Available bool `json:"available"`
}

// packsPerSync is how many audio packs one sync downloads at most, so a
// sync with many new lists still ends soon; the next one gets more.
const packsPerSync = 4

// audioFile is the file a list's audio pack is kept in, next to the list.
func audioFile(li ListInfo) string {
	return AssignedDir + "/" + strings.TrimSuffix(li.File, ".txt") + ".audio"
}

// pronunciationOn reports whether the server last said the learner can
// hear words. c.mu is held.
func (c *Client) pronunciationOn() bool {
	return c.st.linked() && c.st.Me != nil && c.st.Me.Pronunciation != nil && c.st.Me.Pronunciation.Available
}

// packFail is how a list's pack download has been going: how many times
// in a row it failed, and when it may be tried again.
type packFail struct {
	n     int
	retry time.Time
}

// packBackoff is the wait after a pack's nth failure in a row: none after
// the first (the next sync tries again), then 10 minutes, doubling up to
// 2 hours.
func packBackoff(n int) time.Duration {
	if n < 2 {
		return 0
	}
	return min(10*time.Minute<<min(n-2, 4), 2*time.Hour)
}

// packKey names a list's pack in packFails.
func packKey(li ListInfo) string { return li.ID + "@" + strconv.Itoa(li.Version) }

// syncAudio starts downloading the audio pack of each assigned list that
// has none for its version yet, in the background: a big pack takes
// longer than a sync may, and the sync lock isn't held meanwhile, so
// nothing else waits for it (an AI call, joining a room). It returns at
// once. A pack that failed is tried again later and later (packBackoff);
// the words are heard a little later, and nothing else waits for them.
// A 202 means the server is still making the pack. syncMu is held.
func (c *Client) syncAudio(_ context.Context, gen int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return ErrNotLinked
	}
	if c.audioRunning || c.life.Err() != nil || !c.pronunciationOn() || c.o.Store == nil {
		return nil
	}
	now := c.now()
	var want []ListInfo
	keep := map[string]packFail{}
	for _, li := range c.st.Lists {
		if _, ok := c.packs[li.ID]; ok && c.packs[li.ID].Version == li.Version {
			continue
		}
		if li.Audio == li.Version && !inBrowser {
			continue
		}
		fail := c.packFails[packKey(li)]
		if fail.n > 0 {
			keep[packKey(li)] = fail
		}
		if now.Before(fail.retry) {
			continue
		}
		want = append(want, li)
	}
	c.packFails = keep // those of lists no longer wanted are forgotten
	if len(want) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(c.life)
	c.audioRunning, c.audioCancel = true, cancel
	c.bg.Add(1)
	go c.downloadPacks(ctx, cancel, gen, want)
	return nil
}

// downloadPacks downloads the packs of the lists, until ctx ends (the
// client is closed or the game unlinked).
func (c *Client) downloadPacks(ctx context.Context, cancel context.CancelFunc, gen int, want []ListInfo) {
	defer c.bg.Done()
	defer func() {
		cancel()
		c.mu.Lock()
		c.audioRunning = false
		c.mu.Unlock()
	}()
	got := 0
	for _, li := range want {
		if got >= packsPerSync || ctx.Err() != nil {
			return
		}
		data, status, err := c.fetchPack(ctx, gen, li)
		if errors.Is(err, ErrNotLinked) || errors.Is(err, ErrUnlinked) {
			return
		}
		var e *Error
		if errors.As(err, &e) && e.Status == http.StatusForbidden {
			return // off for this account: the next sync reads /me again
		}
		if err == nil && status == http.StatusAccepted {
			continue // still being made: not a failure
		}
		var p *audiopack.Pack
		if err == nil && status == http.StatusOK {
			if p, err = audiopack.Read(data); err == nil && (p.List != li.ID || p.Version != li.Version) {
				err = errors.New("link: an audio pack for another list")
			}
		} else if err == nil {
			err = errors.New("link: no audio pack")
		}
		c.mu.Lock()
		if c.gen != gen || ctx.Err() != nil {
			c.mu.Unlock()
			return
		}
		if err != nil {
			f := c.packFails[packKey(li)]
			f.n++
			f.retry = c.now().Add(packBackoff(f.n))
			if c.packFails == nil {
				c.packFails = map[string]packFail{}
			}
			c.packFails[packKey(li)] = f
			c.mu.Unlock()
			continue
		}
		delete(c.packFails, packKey(li))
		got++
		for i := range c.st.Lists {
			if c.st.Lists[i].ID != li.ID || c.st.Lists[i].Version != li.Version {
				continue
			}
			c.packs[li.ID] = p
			// A web browser's storage is small: its packs live in memory.
			if !inBrowser && c.o.Store.Write(audioFile(li), data) == nil {
				c.st.Lists[i].Audio = li.Version
				c.saveState()
			}
		}
		c.mu.Unlock()
	}
}

// fetchPack downloads a list's pack. It has its own limits (packIdle,
// packMax), not a request's, and holds syncMu only to get a token (as
// AskAI does). In a web browser the whole answer may arrive at once, so
// only packMax applies.
func (c *Client) fetchPack(ctx context.Context, gen int, li ListInfo) ([]byte, int, error) {
	token, err := c.aiToken(ctx, gen, "")
	if err != nil {
		return nil, 0, err
	}
	hc := *c.hc
	hc.Timeout = 0 // the limits are the request's
	idle := packIdle
	if inBrowser {
		idle = packMax
	}
	hdr := http.Header{"Accept": {audiopack.MediaType}}
	path := "/api/v1/lists/" + url.PathEscape(li.ID) + "/audio?version=" + strconv.Itoa(li.Version)
	var data []byte
	status, _, err := c.request(ctx, &hc, packMax, idle, http.MethodGet, path, token, hdr, nil, &data)
	var e *Error
	if errors.As(err, &e) && e.Status == http.StatusUnauthorized {
		if token, err = c.aiToken(ctx, gen, token); err != nil {
			return nil, 0, err
		}
		data = nil
		status, _, err = c.request(ctx, &hc, packMax, idle, http.MethodGet, path, token, hdr, nil, &data)
	}
	return data, status, err
}

// Pronunciation returns the spoken word for entry (its first answer) in
// lang, from the audio pack of the assigned list it is in: a WAV file,
// 16-bit PCM mono. It reports false when the game isn't linked, the
// account can't hear words, or there is no audio for the word.
func (c *Client) Pronunciation(lang string, entry words.Entry) ([]byte, bool) {
	if c == nil || len(entry.Answers) == 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pronunciationOn() {
		return nil, false
	}
	ref, ok := c.keys[lang][words.Key(entry)]
	if !ok {
		return nil, false
	}
	p := c.pack(ref)
	if p == nil || p.Language != lang {
		return nil, false
	}
	w, ok := p.Find(entry.Answers[0])
	if !ok {
		return nil, false
	}
	return w.Audio, true
}

// pack returns the audio pack of an assigned list's version, reading it
// from the store the first time. c.mu is held.
func (c *Client) pack(ref listRef) *audiopack.Pack {
	if p, ok := c.packs[ref.ID]; ok && p.Version == ref.Version {
		return p
	}
	for _, li := range c.st.Lists {
		if li.ID != ref.ID || li.Audio != ref.Version || li.Audio == 0 || c.o.Store == nil {
			continue
		}
		data, err := c.o.Store.Read(audioFile(li))
		if err != nil {
			return nil
		}
		p, err := audiopack.Read(data)
		if err != nil || p.List != li.ID || p.Version != li.Version {
			return nil
		}
		c.packs[li.ID] = p
		return p
	}
	return nil
}
