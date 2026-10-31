package link

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

// Hostile and odd names are made fit to show: no control or format
// characters (bidi overrides, zero-width, tags), never above maxWhere
// characters, never an invalid string, and a school with nothing left to
// show is no school.
func TestQALinkedToCleanHostileNames(t *testing.T) {
	school := func(n string) *LinkedTo { return &LinkedTo{Kind: LinkedSchool, Name: n} }
	long := strings.Repeat("x", 200)
	for _, tc := range []struct {
		name string
		in   *LinkedTo
		want string // "-": dropped
	}{
		{"nil", nil, "-"},
		{"unknown kind", school("A"), "A"},
		{"kind in capitals", &LinkedTo{Kind: "School", Name: "A"}, "-"},
		{"kind with space", &LinkedTo{Kind: "school ", Name: "A"}, "-"},
		{"empty kind", &LinkedTo{Name: "A"}, "-"},
		{"school, no name", school(""), "-"},
		{"school, only controls", school("\x00\x01\x1b\x7f\u0085"), "-"},
		{"school, only spaces and zero-width", school(" ​‍\ufeff  "), "-"},
		{"right-to-left override", school("‮St Mary‬"), "St Mary"},
		{"isolates and marks", school("⁦St⁩ ‎Mary‏"), "St Mary"},
		{"tag characters", school("St\U000E0041\U000E007F Mary"), "St Mary"},
		{"zero-width inside a word", school("Sa​int"), "Sa int"},
		{"newline and tab", school("St\nMary\t's"), "St Mary 's"},
		{"emoji", school("St Mary 🎓"), "St Mary 🎓"},
		{"family", &LinkedTo{Kind: LinkedFamily}, ""},
		{"family with a name", &LinkedTo{Kind: LinkedFamily, Name: "  The‮ Byrnes "}, "The Byrnes"},
		{"family, only controls", &LinkedTo{Kind: LinkedFamily, Name: "\x00"}, ""},
	} {
		got := tc.in.clean()
		if tc.want == "-" {
			if got != nil {
				t.Errorf("%s: kept %+v", tc.name, got)
			}
			continue
		}
		if got == nil || got.Name != tc.want {
			t.Errorf("%s: %+v, want name %q", tc.name, got, tc.want)
		}
	}
	// Long names, of every width of character, are cut to maxWhere runes
	// in valid UTF-8.
	for _, n := range []string{long, strings.Repeat("Ó", 200), strings.Repeat("漢", 200), strings.Repeat("🎓", 200),
		strings.Repeat("é", 200), strings.Repeat("a ", 200), " " + long + " "} {
		got := school(n).clean()
		if got == nil {
			t.Fatalf("%.20q dropped", n)
		}
		if !utf8.ValidString(got.Name) || utf8.RuneCountInString(got.Name) > maxWhere {
			t.Errorf("%.20q cleaned to %d runes, valid=%v", n, utf8.RuneCountInString(got.Name), utf8.ValidString(got.Name))
		}
		if got.Name != strings.TrimSpace(got.Name) {
			t.Errorf("%.20q cleaned to %q with spaces at an end", n, got.Name)
		}
	}
	// Exactly maxWhere runes is kept whole; one more is cut with an ellipsis.
	if got := school(strings.Repeat("x", maxWhere)).clean(); got.Name != strings.Repeat("x", maxWhere) {
		t.Errorf("exactly %d kept as %q", maxWhere, got.Name)
	}
	if got := school(strings.Repeat("x", maxWhere+1)).clean(); !strings.HasSuffix(got.Name, "…") ||
		utf8.RuneCountInString(got.Name) != maxWhere {
		t.Errorf("one over cut as %q", got.Name)
	}
	// clean doesn't change its input.
	in := &LinkedTo{Kind: LinkedSchool, Name: "‮A"}
	in.clean()
	if in.Name != "‮A" {
		t.Error("clean changed its input")
	}
}

// linked_to in the answer of a link that is not by QR code is ignored:
// nothing is kept, and nothing shows.
func TestQALinkedToIgnoredWithoutQR(t *testing.T) {
	_, c, st, _, _ := pairSetup(t)
	c.mu.Lock()
	c.signedIn(tokens{AccessToken: "a", RefreshToken: "r", ExpiresIn: 3600,
		LinkedTo: &LinkedTo{Kind: LinkedSchool, Name: "St Mary's"}}, WayCard)
	c.mu.Unlock()
	if c.st.LinkedTo != nil || c.PairedWhere() != "" {
		t.Errorf("kept %+v / %q", c.st.LinkedTo, c.PairedWhere())
	}
	if strings.Contains(string(st.files[stateFile]), "LinkedTo") {
		t.Error("link.json holds LinkedTo for a code link")
	}
}

// A hostile name through the whole flow (the server's answer, link.json,
// a restart before confirming): what is stored and shown is clean, and a
// link.json edited by hand with a hostile name is not trusted blindly:
// it is shown cleaned or not at all, never as the raw text.
func TestQALinkedToHostileThroughTheFlow(t *testing.T) {
	f, c, st, clk, url := pairSetup(t)
	f.mu.Lock()
	f.approved = true
	f.linkedTo = map[string]any{"kind": "school", "name": "‮St​ Mary's\x00 " + strings.Repeat("Z", 100)}
	f.mu.Unlock()
	ctx := context.Background()
	if _, err := c.StartPair(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.PollPair(ctx); err != nil {
		t.Fatal(err)
	}
	got := c.PairedWhere()
	if strings.ContainsAny(got, "‮​\x00") || utf8.RuneCountInString(got) > maxWhere || !strings.HasPrefix(got, "St Mary's") {
		t.Errorf("shown %q", got)
	}
	if raw := string(st.files[stateFile]); strings.Contains(raw, `‮`) || strings.Contains(raw, "‮") {
		t.Errorf("hostile characters stored: %s", raw)
	}
	c.Close()
	// Restart before confirming: still there, and still the same.
	c2 := Open(Options{Store: st, Server: url, Now: clk.now})
	defer c2.Close()
	if c2.PairedWhere() != got {
		t.Errorf("after a restart %q, want %q", c2.PairedWhere(), got)
	}
	// Not confirmed, not unlinked: no where when the game is unlinked.
	c2.Unlink()
	if c2.PairedWhere() != "" {
		t.Errorf("shown after unlinking: %q", c2.PairedWhere())
	}
	var nilClient *Client
	if nilClient.PairedWhere() != "" {
		t.Error("nil client")
	}
}
