package link

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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

// syncAudio downloads the audio pack of each assigned list that has none
// for its version yet. Failures are left for the next sync: the words
// are heard a little later, and nothing else waits for them. A 202 means
// the server is still making the pack. syncMu is held.
func (c *Client) syncAudio(ctx context.Context, gen int) error {
	c.mu.Lock()
	if c.gen != gen {
		c.mu.Unlock()
		return ErrNotLinked
	}
	var want []ListInfo
	if c.pronunciationOn() && c.o.Store != nil {
		for _, li := range c.st.Lists {
			if _, ok := c.packs[li.ID]; ok && c.packs[li.ID].Version == li.Version {
				continue
			}
			if li.Audio != li.Version || inBrowser {
				want = append(want, li)
			}
		}
	}
	c.mu.Unlock()
	got := 0
	for _, li := range want {
		if got >= packsPerSync {
			break
		}
		var data []byte
		hdr := http.Header{"Accept": {audiopack.MediaType}}
		path := "/api/v1/lists/" + url.PathEscape(li.ID) + "/audio?version=" + strconv.Itoa(li.Version)
		status, _, err := c.authed(ctx, gen, http.MethodGet, path, hdr, nil, &data)
		if errors.Is(err, ErrNotLinked) || errors.Is(err, ErrUnlinked) {
			return err
		}
		var e *Error
		if errors.As(err, &e) && e.Status == http.StatusForbidden {
			return nil // off for this account: the next sync reads /me again
		}
		if err != nil || status != http.StatusOK {
			continue
		}
		p, err := audiopack.Read(data)
		if err != nil || p.List != li.ID || p.Version != li.Version {
			continue
		}
		got++
		c.mu.Lock()
		if c.gen != gen {
			c.mu.Unlock()
			return ErrNotLinked
		}
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
	return nil
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
