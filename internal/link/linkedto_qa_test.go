package link

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Only the kind is ever read: whatever name, hostile or not, a server
// might send with linked_to is dropped (owner, 2026-10-31), and a kind
// the game doesn't know, spelt in any other way, is no kind at all.
func TestQALinkedToKindOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string // "-": dropped
	}{
		{"family", `{"kind":"family"}`, "family"},
		{"school", `{"kind":"school"}`, "school"},
		{"school with a name", `{"kind":"school","name":"St Mary's"}`, "school"},
		{"family with a hostile name", `{"kind":"family","name":"\u202eThe Byrnes\u0000"}`, "family"},
		{"kind in capitals", `{"kind":"School"}`, "-"},
		{"kind with space", `{"kind":"school "}`, "-"},
		{"empty kind", `{"name":"A"}`, "-"},
		{"unknown kind", `{"kind":"club"}`, "-"},
	} {
		var l LinkedTo
		if err := json.Unmarshal([]byte(tc.body), &l); err != nil {
			t.Fatal(err)
		}
		got := l.clean()
		switch {
		case tc.want == "-" && got != nil:
			t.Errorf("%s: kept %+v", tc.name, got)
		case tc.want != "-" && (got == nil || got.Kind != tc.want):
			t.Errorf("%s: %+v, want %q", tc.name, got, tc.want)
		}
		if got != nil {
			if b, _ := json.Marshal(got); string(b) != `{"kind":"`+tc.want+`"}` {
				t.Errorf("%s: kept as %s", tc.name, b)
			}
		}
	}
	var nilL *LinkedTo
	if nilL.clean() != nil {
		t.Error("nil")
	}
}

// linked_to in the answer of a link that is not by QR code is ignored:
// nothing is kept, and nothing shows.
func TestQALinkedToIgnoredWithoutQR(t *testing.T) {
	_, c, st, _, _ := pairSetup(t)
	c.mu.Lock()
	c.signedIn(tokens{AccessToken: "a", RefreshToken: "r", ExpiresIn: 3600,
		LinkedTo: &LinkedTo{Kind: LinkedSchool}}, WayCard)
	c.mu.Unlock()
	if c.st.LinkedTo != nil || c.PairedWhere() != "" {
		t.Errorf("kept %+v / %q", c.st.LinkedTo, c.PairedWhere())
	}
	if strings.Contains(string(st.files[stateFile]), "LinkedTo") {
		t.Error("link.json holds LinkedTo for a code link")
	}
}

// A name sent through the whole flow (the server's answer, link.json, a
// restart before confirming) is never stored or shown; a link.json
// edited by hand with an odd kind shows nothing; unlinking clears it.
func TestQALinkedToNameNeverKept(t *testing.T) {
	f, c, st, clk, url := pairSetup(t)
	f.mu.Lock()
	f.approved = true
	f.linkedTo = map[string]any{"kind": "school", "name": "\u202eSt Mary's"}
	f.mu.Unlock()
	ctx := context.Background()
	if _, err := c.StartPair(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.PollPair(ctx); err != nil {
		t.Fatal(err)
	}
	if got := c.PairedWhere(); got != "school" {
		t.Errorf("shown %q", got)
	}
	if raw := string(st.files[stateFile]); strings.Contains(raw, "Mary") || strings.Contains(raw, `"name"`) {
		t.Errorf("a name stored: %s", raw)
	}
	c.Close()
	c2 := Open(Options{Store: st, Server: url, Now: clk.now})
	defer c2.Close()
	if c2.PairedWhere() != "school" {
		t.Errorf("after a restart %q", c2.PairedWhere())
	}
	c2.mu.Lock()
	c2.st.LinkedTo = &LinkedTo{Kind: "St Mary's"} // edited by hand
	c2.mu.Unlock()
	if c2.PairedWhere() != "" {
		t.Errorf("an edited kind shown: %q", c2.PairedWhere())
	}
	c2.Unlink()
	if c2.PairedWhere() != "" || strings.Contains(string(st.files[stateFile]), "LinkedTo") {
		t.Errorf("kept after unlinking: %q %s", c2.PairedWhere(), st.files[stateFile])
	}
	var nilClient *Client
	if nilClient.PairedWhere() != "" {
		t.Error("nil client")
	}
}
