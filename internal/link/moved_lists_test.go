package link

import (
	"context"
	"testing"
)

// A game that moved to a new address has no link.json and, since assigned
// lists don't move (#88), no assigned lists: a client over a store with no
// link shows none, and once linked at the new address it fetches them.
func TestAssignedListsAreFetchedAfterMove(t *testing.T) {
	f, c, st, clk := linked(t)
	want := string(st.files["assigned/lst_animals.txt"])
	if want == "" {
		t.Fatal("no assigned list after linking")
	}
	c.Close()
	// The moved game: neither the link nor its lists came along.
	delete(st.files, stateFile)
	delete(st.files, "assigned/lst_animals.txt")
	c2 := reopen(f, st, clk)
	if len(c2.Lists()) != 0 {
		t.Fatalf("lists without a link: %v", c2.Lists())
	}
	f.code = "QRST-VWXY"
	if err := c2.LinkNow(context.Background(), f.code); err != nil {
		t.Fatal(err)
	}
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(c2.Lists()) != 1 || string(st.files["assigned/lst_animals.txt"]) != want {
		t.Fatalf("the list didn't come back after linking: %v", c2.Lists())
	}
}

// A copy of an assigned list left by a game from before they stayed behind
// is replaced by the server's at the first sync after linking.
func TestLeftoverAssignedListIsReplaced(t *testing.T) {
	f, c, st, clk := linked(t)
	want := string(st.files["assigned/lst_animals.txt"])
	c.Close()
	delete(st.files, stateFile)
	st.files["assigned/lst_animals.txt"] = []byte("title: Old\nlanguage: fr\n\ncat = le chat\n")
	c2 := reopen(f, st, clk)
	if len(c2.Lists()) != 0 {
		t.Fatalf("a leftover list is shown without a link: %v", c2.Lists())
	}
	f.code = "QRST-VWXY"
	if err := c2.LinkNow(context.Background(), f.code); err != nil {
		t.Fatal(err)
	}
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if string(st.files["assigned/lst_animals.txt"]) != want {
		t.Errorf("the leftover list was kept: %q", st.files["assigned/lst_animals.txt"])
	}
}
