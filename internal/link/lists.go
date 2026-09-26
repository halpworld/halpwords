package link

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/halpworld/halpwords/pkg/gameai"
	"github.com/halpworld/halpwords/pkg/words"
)

// wireList is one list in GET /api/v1/lists.
type wireList struct {
	ID       string `json:"id"`
	Version  int    `json:"version"`
	Title    string `json:"title"`
	Language string `json:"language"`
	Words    int    `json:"words"`
	Text     string `json:"text"`
	// Riddles came with W4.8; older servers leave them out.
	Riddles []ListRiddle `json:"riddles,omitempty"`
}

// Lists returns the assigned lists, read-only: the game plays them like
// its own, and the Word Lists screen shows them under Assigned. Each
// list's File is "assigned/<file>" so it never clashes with the player's.
func (c *Client) Lists() []*words.List {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.lists)
}

// IsAssigned reports whether a list (by its File) is an assigned list.
func IsAssigned(l *words.List) bool { return l != nil && strings.HasPrefix(l.File, AssignedDir+"/") }

// loadLists reads the assigned lists named in the state from the store,
// and indexes their words. c.mu is held, or the client is new.
func (c *Client) loadLists() {
	c.lists, c.keys = nil, map[string]map[string]listRef{}
	if c.o.Store == nil || !c.st.linked() {
		return
	}
	var keep []ListInfo
	for _, li := range c.st.Lists {
		data, err := c.o.Store.Read(AssignedDir + "/" + li.File)
		if err != nil {
			continue
		}
		l, err := words.Parse(bytes.NewReader(data), AssignedDir+"/"+li.File)
		if err != nil {
			continue
		}
		keep = append(keep, li)
		c.addList(l, li)
	}
	c.st.Lists = keep
}

// addList adds a parsed list to the lists and the word index. c.mu is
// held.
func (c *Client) addList(l *words.List, li ListInfo) {
	l.ID, l.Version = li.ID, li.Version // the list answers name
	c.lists = append(c.lists, l)
	m := c.keys[l.Language]
	if m == nil {
		m = map[string]listRef{}
		c.keys[l.Language] = m
	}
	for _, e := range l.Entries {
		k := words.Key(e)
		if _, ok := m[k]; !ok {
			m[k] = listRef{ID: li.ID, Version: li.Version}
		}
	}
}

// fileFor is the file an assigned list is kept in: its ID if that makes
// a safe file name, or else a hash of it.
func fileFor(id string) string {
	ok := id != "" && len(id) <= 64
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			ok = false
		}
	}
	if !ok {
		h := sha256.Sum256([]byte(id))
		id = hex.EncodeToString(h[:8])
	}
	return id + ".txt"
}

// syncLists downloads the assigned lists if they changed since the last
// time (by the ETag), keeps them, and moves lists no longer assigned out
// of Assigned. syncMu is held.
func (c *Client) syncLists(ctx context.Context, gen int) error {
	c.mu.Lock()
	etag := c.st.ListsETag
	c.mu.Unlock()
	hdr := http.Header{}
	if etag != "" {
		hdr.Set("If-None-Match", etag)
	}
	var body struct {
		Lists []wireList `json:"lists"`
	}
	status, h, err := c.authed(ctx, gen, http.MethodGet, "/api/v1/lists", hdr, nil, &body)
	if err != nil || status == http.StatusNotModified {
		return err
	}
	if c.o.Store == nil {
		return errNoStore
	}
	type got struct {
		l    *words.List
		li   ListInfo
		text string
	}
	var lists []got
	for _, w := range body.Lists {
		li := ListInfo{ID: w.ID, Version: w.Version, Title: w.Title, Language: w.Language, File: fileFor(w.ID), Riddles: w.Riddles}
		l, err := words.Parse(strings.NewReader(w.Text), AssignedDir+"/"+li.File)
		if err != nil || len(l.Entries) == 0 || !knownLang(l.Language) {
			continue // a list this game can't read is left out
		}
		li.Licensed = strings.EqualFold(strings.TrimSpace(l.Licence), "licensed")
		li.Language = l.Language
		if li.Title == "" {
			li.Title = l.Title
		}
		lists = append(lists, got{l, li, w.Text})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return ErrNotLinked
	}
	now := map[string]bool{}
	for _, g := range lists {
		now[g.li.File] = true
	}
	for _, g := range lists {
		old := slices.IndexFunc(c.st.Lists, func(o ListInfo) bool { return o.File == g.li.File })
		if old >= 0 && c.st.Lists[old].Version == g.li.Version {
			continue // unchanged
		}
		if err := c.o.Store.Write(AssignedDir+"/"+g.li.File, []byte(g.text)); err != nil {
			return err
		}
	}
	for _, o := range c.st.Lists {
		if !now[o.File] {
			c.moveOut(o)
		}
	}
	c.st.Lists = c.st.Lists[:0]
	for _, g := range lists {
		c.st.Lists = append(c.st.Lists, g.li)
	}
	c.st.ListsETag = h.Get("ETag")
	c.loadLists()
	c.saveState()
	c.changes++
	return nil
}

// moveOut takes a list out of Assigned: it becomes one of the player's
// own lists, in OwnDir, unless it is licensed, and then it is removed.
// c.mu is held.
func (c *Client) moveOut(li ListInfo) {
	src := AssignedDir + "/" + li.File
	defer c.o.Store.Remove(src)
	if li.Licensed || c.o.OwnDir == "" {
		return
	}
	data, err := c.o.Store.Read(src)
	if err != nil {
		return
	}
	base := strings.TrimSuffix(words.FileName(li.Title), ".txt")
	if base == "" {
		base = "assigned"
	}
	name := base + ".txt"
	for n := 2; n < 100; n++ {
		old, err := c.o.Store.Read(c.o.OwnDir + "/" + name)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err == nil && bytes.Equal(old, data) {
			return // already kept
		}
		name = base + "-" + strconv.Itoa(n) + ".txt"
	}
	c.o.Store.Write(c.o.OwnDir+"/"+name, data)
}

// syncQuests downloads the assignments as quests. syncMu is held.
func (c *Client) syncQuests(ctx context.Context, gen int) error {
	var body struct {
		Assignments []Quest `json:"assignments"`
	}
	if _, _, err := c.authed(ctx, gen, http.MethodGet, "/api/v1/assignments", nil, nil, &body); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return ErrNotLinked
	}
	c.st.Quests = body.Assignments
	return c.saveState()
}

// Riddles returns the riddles that came with those of lists that are
// assigned, by English word in lower case, as puzzle.Generated keeps
// them. Each is checked again as the game checks an AI's riddle.
func (c *Client) Riddles(lists []*words.List) map[string][]string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var out map[string][]string
	for _, l := range lists {
		if !IsAssigned(l) {
			continue
		}
		i := slices.IndexFunc(c.st.Lists, func(li ListInfo) bool { return AssignedDir+"/"+li.File == l.File })
		if i < 0 {
			continue
		}
		for _, r := range c.st.Lists[i].Riddles {
			k := strings.ToLower(strings.TrimSpace(r.English))
			j := slices.IndexFunc(l.Entries, func(e words.Entry) bool { return strings.ToLower(e.Prompt) == k })
			if j < 0 {
				continue
			}
			rd, ok := gameai.CheckRiddle(r.Riddle, l.Entries[j])
			if !ok || slices.Contains(out[k], rd) {
				continue
			}
			if out == nil {
				out = map[string][]string{}
			}
			out[k] = append(out[k], rd)
		}
	}
	return out
}

func knownLang(code string) bool { _, ok := words.Lookup(code); return ok }
