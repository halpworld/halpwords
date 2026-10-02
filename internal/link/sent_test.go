package link

import (
	"context"
	"testing"
	"time"
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
