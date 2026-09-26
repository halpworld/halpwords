package scene

import (
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/raycast"
	"github.com/halpworld/halpwords/internal/unifont"
	"github.com/halpworld/halpwords/pkg/raid"
	"github.com/halpworld/halpwords/pkg/words"
)

// raidState is a room with a raid under way, as the link has it.
func raidState() link.PlayState {
	return link.PlayState{Phase: link.PlayInRoom,
		Room: link.Room{Code: "ABCDEF", Mode: link.ModeRaid, You: "m2", Members: []link.Member{
			{ID: "m1", Role: link.RoleHost}, {ID: "m2", Role: link.RoleLearner, Name: "Brave Otter"},
			{ID: "m3", Role: link.RoleLearner, Name: "Quiet Fox"}}},
		Raid: &link.Raid{Number: 1, Boss: raid.BossFor(7).Name, Seed: 7, Lang: "fr", HP: 1000, MaxHP: 1000,
			EndsAt: time.Now().Add(raid.TimeLimit), Raiders: 2, Word: &link.RaidWord{N: 1, Prompt: "the dog"}}}
}

func withFont(t *testing.T, ctx *game.Context) {
	t.Helper()
	face, err := unifont.ParseBytes(assets.UnifontHex)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Font = gfx.NewFont(face)
}

// The lobby opens a raid for the room's learners, not for grown-ups.
func TestRaidIsForLearners(t *testing.T) {
	st := raidState()
	if !isLearner(st.Room) {
		t.Fatal("m2 isn't a learner")
	}
	st.Room.You = "m1"
	if isLearner(st.Room) {
		t.Fatal("the host is a learner")
	}
}

// The boss stands in the dungeon view, in full sight.
func TestRaidBossIsInView(t *testing.T) {
	s := newRaidScreen(testContext(t), raidState())
	if s.name != raid.BossFor(7).Name || s.lang.Code != "fr" || s.field == nil {
		t.Fatalf("screen %+v", s)
	}
	drawn := func(sprites []raycast.Sprite) []byte {
		s.view.Render(s.level, s.tex, s.cam, sprites, 0)
		return append([]byte(nil), s.view.Img.Pix...)
	}
	empty := drawn(nil)
	boss := drawn([]raycast.Sprite{{X: s.bx, Y: s.by, Img: s.look[0], Size: s.size}})
	changed := 0
	for i := 0; i < len(empty); i += 4 {
		if empty[i] != boss[i] || empty[i+1] != boss[i+1] || empty[i+2] != boss[i+2] {
			changed++
		}
	}
	// At least a tenth of the view is the boss.
	if changed < viewW*viewH/10 {
		t.Fatalf("the boss covers %d of %d pixels", changed, viewW*viewH)
	}
}

// Hits, a late raider, an attack and a failed dodge all show.
func TestRaidScreenReacts(t *testing.T) {
	ctx := testContext(t)
	s := newRaidScreen(ctx, raidState())
	rd := s.st.Raid
	rd.Hits, rd.LastHit = 1, link.RaidHit{ID: "m3", Damage: 12}
	s.react(ctx, rd)
	if s.flash == 0 || len(s.log) != 1 || !strings.HasPrefix(s.log[0].text, "Quiet Fox: 12") {
		t.Fatalf("hit: flash %d, log %v", s.flash, s.log)
	}
	rd.Hits, rd.LastHit = 2, link.RaidHit{ID: "m2", Damage: 10}
	s.react(ctx, rd)
	if s.log[1].text != "You: 10 damage" {
		t.Fatalf("own hit: %v", s.log)
	}

	rd.Grew = 1
	s.react(ctx, rd)
	if !strings.Contains(s.news, "stronger") {
		t.Fatalf("late raider: %q", s.news)
	}

	rd.Attack = &link.RaidAttack{N: 1, Of: 3, Until: time.Now().Add(6 * time.Second)}
	s.react(ctx, rd)
	if s.shake == 0 || !strings.Contains(s.news, "Dodge") {
		t.Fatalf("attack: shake %d, %q", s.shake, s.news)
	}
	rd.Graded = &link.RaidGraded{N: 2, Tier: words.Miss, Expected: "la maison", Dodge: true, Late: true,
		StunnedUntil: time.Now().Add(5 * time.Second)}
	s.react(ctx, rd)
	if s.hurt == 0 || !strings.Contains(s.news, "stunned") || gradeLine(rd.Graded) != "Too slow: la maison" {
		t.Fatalf("failed dodge: hurt %d, %q", s.hurt, s.news)
	}
	rd.Attack = &link.RaidAttack{N: 1, Of: 3, Dodged: 2, Done: true}
	s.react(ctx, rd)
	if s.news != "2 of 3 raiders dodged." {
		t.Fatalf("attack over: %q", s.news)
	}
	// Seen once, shown once.
	s.shake, s.flash = 0, 0
	s.react(ctx, rd)
	if s.shake != 0 || s.flash != 0 || len(s.log) != 2 {
		t.Fatal("reacted twice to the same news")
	}
}

func TestRaidGradeLines(t *testing.T) {
	for _, c := range []struct {
		g    link.RaidGraded
		want string
	}{
		{link.RaidGraded{Tier: words.Perfect, Damage: 12}, "Perfect! 12 damage"},
		{link.RaidGraded{Tier: words.AccentSlip, Expected: "la mère", Damage: 5}, "Accent slip: la mère"},
		{link.RaidGraded{Tier: words.Miss, Expected: "le chien"}, "Miss: le chien"},
		{link.RaidGraded{Tier: words.Correct, Dodge: true, Dodged: true}, "Dodged!"},
	} {
		if got := gradeLine(&c.g); got != c.want {
			t.Errorf("%+v: %q, want %q", c.g, got, c.want)
		}
	}
}

// The raid, its typing panel and the finale draw without failing.
func TestRaidScreenDraws(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	dst := ebiten.NewImage(game.ScreenW, game.ScreenH)
	s := newRaidScreen(ctx, raidState())
	s.n = 1
	s.field.Type('l')
	s.Draw(dst, ctx)
	s.st.Raid.Word = &link.RaidWord{N: 2, Prompt: "the house", Dodge: true, Until: time.Now().Add(4 * time.Second)}
	s.n, s.limit = 2, 6*time.Second
	s.Draw(dst, ctx)
	s.st.Raid.Word = nil
	s.st.Raid.Graded = &link.RaidGraded{N: 2, Dodge: true, Late: true, Expected: "la maison", StunnedUntil: time.Now().Add(3 * time.Second)}
	s.leaving = true
	s.Draw(dst, ctx)

	s.over, s.st.Raid = true, nil
	s.st.Finale = &link.RaidFinale{Outcome: link.RaidWon, Boss: s.name, MaxHP: 1000, Raiders: 2, Time: 3 * time.Minute,
		RaidTally: link.RaidTally{Answers: 90, Right: 80, Damage: 1000, Dodged: 3, Attacks: 2}}
	s.st.Summary = &link.RaidTally{Answers: 45, Right: 40, Damage: 500, Dodged: 1, Attacks: 2}
	s.Draw(dst, ctx)
}
