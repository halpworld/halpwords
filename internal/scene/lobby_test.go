package scene

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/link"
)

func TestLobbyWords(t *testing.T) {
	host := link.Member{ID: "m1", Role: link.RoleHost}
	adult := link.Member{ID: "m4", Role: link.RoleAdult}
	me := link.Member{ID: "m2", Role: link.RoleLearner, Name: "Brave Otter"}
	fox := link.Member{ID: "m3", Role: link.RoleLearner, Name: "Quiet Fox"}
	for _, tc := range []struct {
		e    link.PlayEvent
		want string
	}{
		{link.PlayEvent{Kind: "joined", Member: fox}, "Quiet Fox joined."},
		{link.PlayEvent{Kind: "said", Member: host, Preset: "good-luck"}, "Host: Good luck!"},
		{link.PlayEvent{Kind: "said", Member: me, Mine: true, Preset: "thanks"}, "You: Thanks!"},
		{link.PlayEvent{Kind: "emoted", Member: fox, Emote: "thumbs-up"}, "Quiet Fox gives a thumbs up."},
		{link.PlayEvent{Kind: "emoted", Member: me, Mine: true, Emote: "wave"}, "You wave."},
		{link.PlayEvent{Kind: "left", Member: fox, Reason: "kicked"}, "Quiet Fox was removed by the host."},
		{link.PlayEvent{Kind: "away", Member: adult}, "Grown-up lost the connection."},
		// A preset of a later version isn't shown, not even its ID.
		{link.PlayEvent{Kind: "said", Member: fox, Preset: "brand-new"}, ""},
	} {
		if got := eventText(tc.e, "m2"); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.e, got, tc.want)
		}
	}
	if got := memberName(me, "m2"); got != "Brave Otter (you)" {
		t.Fatal(got)
	}
	if got := showRoomCode("ABC"); got != "ABC-___" {
		t.Fatal(got)
	}
	r := link.Room{You: "m2", Members: []link.Member{fox, me, adult, host}}
	var order []string
	for _, m := range sortedMembers(r) {
		order = append(order, m.ID)
	}
	if strings.Join(order, ",") != "m1,m4,m2,m3" {
		t.Fatal(order)
	}
	// Only what the server offers and the game can show.
	cs := chatChoices(link.PlayState{Presets: []string{"good-luck", "brand-new"}, Emotes: []string{"wave"}})
	if len(cs) != 2 || cs[0].label() != "Good luck!" || cs[1].label() != "*waves*" {
		t.Fatal(cs)
	}
	if _, ok := roomCodeRune('!'); ok {
		t.Fatal("typed a !")
	}
}

func TestLobbyNeedsALink(t *testing.T) {
	ctx := testContext(t)
	if ctx.Link.Play().State().Phase != link.PlayOff {
		t.Fatal("playing without a link")
	}
	NewLobby(ctx).Update(ctx)
}
