package link

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/halpworld/halpwords/pkg/maps"
)

// AssignedQuest is a hand-made quest a grown-up gave the learner in the
// website's map editor (server W10.4), as GET /api/v1/maps sends it.
type AssignedQuest struct {
	ID       string `json:"id"`     // the assignment
	MapID    string `json:"map_id"` // the quest
	Version  int    `json:"version"`
	Title    string `json:"title"`
	Language string `json:"language"` // empty for any
	Floors   int    `json:"floors"`
	DueAt    string `json:"due_at,omitempty"`
	// Text is the quest as a .hwquest file (pkg/maps).
	Text string `json:"text"`
}

// Quest reads the quest. The link keeps only quests that read and pass
// maps' checks.
func (a AssignedQuest) Quest() (*maps.Quest, error) { return maps.ParseQuest([]byte(a.Text)) }

// WhenText says when the quest is due, as Quest.WhenText does, or "".
func (a AssignedQuest) WhenText(now time.Time) string { return Quest{DueAt: a.DueAt}.WhenText(now) }

// syncMaps downloads the quests given to the learner if they changed
// since the last time (by the ETag). A quest that is no longer there was
// ended. A server without them (404) has none. syncMu is held.
func (c *Client) syncMaps(ctx context.Context, gen int) error {
	c.mu.Lock()
	etag := c.st.MapsETag
	if c.st.MapsGame != c.userAgent() {
		etag = "" // read by another version of the game
	}
	c.mu.Unlock()
	hdr := http.Header{}
	if etag != "" {
		hdr.Set("If-None-Match", etag)
	}
	var body struct {
		Quests []AssignedQuest `json:"quests"`
	}
	status, h, err := c.authed(ctx, gen, http.MethodGet, "/api/v1/maps", hdr, nil, &body)
	var e *Error
	if errors.As(err, &e) && e.Status == http.StatusNotFound {
		status, h, err = http.StatusOK, http.Header{}, nil
	}
	if err != nil || status == http.StatusNotModified {
		return err
	}
	var got []AssignedQuest
	for _, a := range body.Quests {
		q, err := a.Quest()
		if err != nil || len(q.Check(nil)) > 0 {
			continue // a quest this game can't play is left out
		}
		if a.Title == "" {
			a.Title = q.Title
		}
		a.Language = q.Language
		got = append(got, a)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return ErrNotLinked
	}
	if !slices.EqualFunc(got, c.st.Maps, func(a, b AssignedQuest) bool {
		return a.ID == b.ID && a.Version == b.Version && a.DueAt == b.DueAt
	}) {
		c.changes++
	}
	c.st.Maps, c.st.MapsETag, c.st.MapsGame = got, h.Get("ETag"), c.userAgent()
	return c.saveState()
}

// AssignedQuests returns the quests grown-ups gave the learner on the
// website, newest first, or nil when the game isn't linked.
func (c *Client) AssignedQuests() []AssignedQuest {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.st.Maps)
}
