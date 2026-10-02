package link

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

// Sent lists (#89): GET /api/v1/lists says where each list comes from,
// when a sent one was sent and whether an assignment locks it, and the
// game keeps that with the list. An older server leaves the fields out.
func TestSentAndLockedLists(t *testing.T) {
	f, c, _, _ := linked(t)
	if li, ok := c.Info(c.Lists()[0]); !ok || li.Source != SourceAssignment || !li.SentAt.IsZero() || li.Locked {
		t.Fatalf("a list from an older server: %+v %v", li, ok)
	}
	verbs := "title: Verbs\nlanguage: fr\nid: lst_verbs\nversion: 2\n\nto be = être\nto have = avoir\n"
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Text: animals, Source: "assignment", Locked: true},
		wireList{ID: "lst_verbs", Version: 2, Title: "Verbs", Language: "fr", Text: verbs, Source: "sent", SentAt: "2026-10-01T18:30:00Z"})
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	lists := c.Lists()
	if len(lists) != 2 {
		t.Fatalf("%d lists", len(lists))
	}
	animalsInfo, _ := c.Info(lists[0])
	verbsInfo, _ := c.Info(lists[1])
	if !animalsInfo.Locked || animalsInfo.Source != SourceAssignment {
		t.Errorf("the locked assignment list: %+v", animalsInfo)
	}
	want := time.Date(2026, 10, 1, 18, 30, 0, 0, time.UTC)
	if verbsInfo.Source != SourceSent || !verbsInfo.SentAt.Equal(want) || verbsInfo.Locked {
		t.Errorf("the sent list: %+v", verbsInfo)
	}
	// It is kept with the link.
	again := Open(c.o)
	if li, ok := again.Info(again.Lists()[1]); !ok || !li.SentAt.Equal(want) {
		t.Errorf("after opening again: %+v %v", li, ok)
	}

	// The lock goes: the server changes the ETag, and the game follows,
	// though the version is the same.
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Text: animals, Source: "assignment"},
		wireList{ID: "lst_verbs", Version: 2, Title: "Verbs", Language: "fr", Text: verbs, Source: "sent", SentAt: "2026-10-01T18:30:00Z"})
	f.mu.Lock()
	f.etag = `"unlocked"`
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if li, _ := c.Info(c.Lists()[0]); li.Locked {
		t.Error("the lock stayed after the server took it off")
	}
	// A list that is not the server's has no info.
	if _, ok := c.Info(nil); ok {
		t.Error("info for no list")
	}
	var none *Client
	if _, ok := none.Info(c.Lists()[0]); ok {
		t.Error("info without a link")
	}
}

// An assignment's "Lock to this list" setting is read; older servers
// leave it out.
func TestQuestLockLists(t *testing.T) {
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.quests = []byte(`{"assignments":[
		{"id":"asg_1","list":{"id":"lst_animals","version":3,"title":"Animals"},"goal":{"kind":"practise"},"mode":"any","settings":{"lock_lists":true},"starts_at":"2026-09-01T00:00:00Z"},
		{"id":"asg_2","list":{"id":"lst_animals","version":3,"title":"Animals"},"goal":{"kind":"practise"},"mode":"any","settings":{},"starts_at":"2026-09-01T00:00:00Z"}]}`)
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	qs := c.Quests()
	if len(qs) != 2 || !qs[0].Settings.LockLists || qs[1].Settings.LockLists {
		t.Fatalf("quests: %+v", qs)
	}
}

// readLog is a Store that records the files read.
type readLog struct {
	*memStore
	mu    sync.Mutex
	reads []string
}

func (r *readLog) Read(name string) ([]byte, error) {
	r.mu.Lock()
	r.reads = append(r.reads, name)
	r.mu.Unlock()
	return r.memStore.Read(name)
}

// The player's own lists are never the link's (#89): the link reads its
// own folders, not the words folder, while lists come and go on the
// server, and no own list has a source, a send time or a lock. The one
// read of the words folder is when a list stops being assigned and is
// kept as the player's own: it looks for a copy already kept.
func TestOwnListsAreNotTheLinks(t *testing.T) {
	f := newFake(t)
	st := &readLog{memStore: newMemStore()}
	st.Write("words/tonight.txt", []byte("title: Tonight\nlanguage: fr\n\nred = rouge\n"))
	c := Open(Options{Store: st, Server: f.srv.URL, Version: "v1.2.0", OwnDir: "words"})
	c.sleep = func(time.Duration) {}
	verbs := "title: Verbs\nlanguage: fr\nid: lst_verbs\nversion: 2\n\nto be = être\n"
	f.setLists(wireList{ID: "lst_verbs", Version: 2, Title: "Verbs", Language: "fr", Text: verbs, Source: "sent", SentAt: "2026-10-01T18:30:00Z"})
	if err := c.LinkNow(context.Background(), "abcd efgh"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := c.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	reopened := Open(Options{Store: st, Server: f.srv.URL, Version: "v1.2.0", OwnDir: "words"})
	for _, n := range st.reads {
		if strings.HasPrefix(n, "words/") {
			t.Errorf("the link read %s", n)
		}
	}
	for _, cl := range []*Client{c, reopened} {
		for _, l := range cl.Lists() {
			if !IsAssigned(l) {
				t.Errorf("the link has %s", l.File)
			}
		}
	}
	own := &words.List{Title: "Tonight", Language: "fr", File: "tonight.txt", ID: "lst_verbs"}
	if li, ok := c.Info(own); ok || li.Source == SourceSent {
		t.Errorf("an own list has link info: %+v", li)
	}

	// The list stops being sent: it is kept as the player's own.
	st.mu.Lock()
	st.reads = nil
	st.mu.Unlock()
	f.setLists()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, n := range st.reads {
		if strings.HasPrefix(n, "words/") && n != "words/verbs.txt" {
			t.Errorf("the link read %s", n)
		}
	}
	if _, err := st.memStore.Read("words/verbs.txt"); err != nil {
		t.Error("the list was not kept")
	}
}

// A server from before #89 sends lists without source, sent_at or
// locked, and assignments without lock_lists or settings at all: they are
// assigned lists, unlocked.
func TestOldServerLists(t *testing.T) {
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.quests = []byte(`{"assignments":[
		{"id":"asg_1","list":{"id":"lst_animals","version":3,"title":"Animals"},"goal":{"kind":"practise"},"mode":"any","starts_at":"2026-09-01T00:00:00Z"}]}`)
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(c.Lists()) != 1 {
		t.Fatalf("%d lists", len(c.Lists()))
	}
	li, ok := c.Info(c.Lists()[0])
	if !ok || li.Source != SourceAssignment || !li.SentAt.IsZero() || li.Locked || li.ID != "lst_animals" {
		t.Errorf("list %+v", li)
	}
	if qs := c.Quests(); len(qs) != 1 || qs[0].Settings.LockLists {
		t.Errorf("quests %+v", qs)
	}
}
