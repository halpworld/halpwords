package link

import (
	"encoding/json"
	"time"

	"github.com/halpworld/halpwords/pkg/raid"
	"github.com/halpworld/halpwords/pkg/words"
)

// Boss Raids (W7.5, halpwords-server's docs/api/play.md): in a raid room
// a teacher starts a raid from the class's big screen, and every learner
// in the room fights the boss together. The server deals each raider
// their own words (Raid.Word) and grades what they type (Answer, then
// Raid.Graded); the game only shows what the server says. Nothing typed
// is kept on either side.

// ModeRaid is a raid room's mode.
const ModeRaid = "raid"

// The ways a raid ends (RaidFinale.Outcome).
const (
	RaidWon     = "won"
	RaidEscaped = "escaped"
	RaidStopped = "stopped"
)

// RaidWord is the raider's word to type.
type RaidWord struct {
	// N numbers the word, for its answer.
	N int
	// Prompt is the English word to translate.
	Prompt string
	// Dodge is set for a dodge of the boss's attack, which must be typed
	// before Until.
	Dodge bool
	Until time.Time
}

// RaidGraded is how the raider's last answer went.
type RaidGraded struct {
	N        int
	Tier     words.Tier
	Expected string
	// Damage is what it took off the boss.
	Damage int
	// Dodge is set for a dodge, and Dodged when it got out of the way;
	// Late is a dodge not typed in time.
	Dodge, Dodged, Late bool
	// StunnedUntil is when the next word comes, for a raider who didn't
	// dodge.
	StunnedUntil time.Time
}

// RaidAttack is the boss's attack.
type RaidAttack struct {
	N int
	// Of is how many raiders it attacks, and Dodged how many got out of
	// the way, once Done.
	Of, Dodged int
	// Until is the longest time to dodge.
	Until time.Time
	Done  bool
}

// RaidTally is what raiders did: the class's in RaidFinale, the
// player's own in PlayState.Summary.
type RaidTally struct {
	Answers int `json:"answers"`
	Right   int `json:"right"`
	Damage  int `json:"damage"`
	Dodged  int `json:"dodged"`
	Attacks int `json:"attacks"`
}

// RaidFinale is how the last raid ended, with the class's totals.
type RaidFinale struct {
	Outcome   string
	Boss      string
	HP, MaxHP int
	Raiders   int
	Time      time.Duration
	RaidTally
}

// Raid is the raid under way in the room.
type Raid struct {
	// Number counts the raids the game has seen, so the game can tell a
	// new raid from the one it shows.
	Number int
	// Boss is the boss's name, and Seed draws it (raid.BossFor).
	Boss string
	Seed uint64
	// Lang is the language of the answers.
	Lang      string
	HP, MaxHP int
	EndsAt    time.Time
	Raiders   int
	// Attack is the boss's last attack, or nil.
	Attack *RaidAttack
	// Word is the word to type now, or nil (between words, stunned, or
	// for someone who isn't raiding); Graded is the last answer's grade.
	Word   *RaidWord
	Graded *RaidGraded
	// Hits counts hits on the boss, by anyone, and LastHit is the last:
	// who (a member ID) and its damage. Grew counts raiders who joined
	// late and made the boss stronger.
	Hits    int
	LastHit RaidHit
	Grew    int
}

// RaidHit is a hit on the boss.
type RaidHit struct {
	ID     string
	Damage int
}

// clone copies the raid and what it points to.
func (r *Raid) clone() *Raid {
	if r == nil {
		return nil
	}
	c := *r
	if r.Attack != nil {
		a := *r.Attack
		c.Attack = &a
	}
	if r.Word != nil {
		w := *r.Word
		c.Word = &w
	}
	if r.Graded != nil {
		g := *r.Graded
		c.Graded = &g
	}
	return &c
}

// Answer sends what the raider typed for their word n, and whether they
// used backspace. It reports false when n isn't the word the raider has
// now (answered already, or replaced by a dodge). The server grades it:
// the grade comes back as Raid.Graded, and the next word as Raid.Word.
// Longer answers than raid.MaxAnswer are cut.
func (p *Play) Answer(n int, typed string, backspace bool) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	rd := p.st.Raid
	if p.st.Phase != PlayInRoom || p.out == nil || rd == nil || rd.Word == nil || rd.Word.N != n {
		return false
	}
	if r := []rune(typed); len(r) > raid.MaxAnswer {
		typed = string(r[:raid.MaxAnswer])
	}
	msg, _ := json.Marshal(struct {
		T         string `json:"t"`
		N         int    `json:"n"`
		Answer    string `json:"answer"`
		Backspace bool   `json:"backspace"`
	}{"answer", n, typed, backspace})
	select {
	case p.out <- msg:
		rd.Word = nil
		p.st.Changes++
		return true
	default:
		return false
	}
}

// The raid's messages as the server sends them.
type (
	raidMessage struct {
		Boss    string         `json:"boss"`
		Seed    uint64         `json:"seed"`
		Lang    string         `json:"lang"`
		HP      int            `json:"hp"`
		MaxHP   int            `json:"max_hp"`
		EndsIn  int64          `json:"ends_in"`
		Raiders int            `json:"raiders"`
		Attack  *attackMessage `json:"attack"`
		Word    *wordMessage   `json:"word"`
	}
	wordMessage struct {
		N      int    `json:"n"`
		Prompt string `json:"prompt"`
		Dodge  bool   `json:"dodge"`
		In     int64  `json:"in"`
	}
	gradedMessage struct {
		N        int    `json:"n"`
		Tier     int    `json:"tier"`
		Expected string `json:"expected"`
		Damage   int    `json:"damage"`
		Dodge    bool   `json:"dodge"`
		Dodged   bool   `json:"dodged"`
		Late     bool   `json:"late"`
		Stun     int64  `json:"stun"`
	}
	attackMessage struct {
		N      int   `json:"n"`
		Of     int   `json:"of"`
		In     int64 `json:"in"`
		Dodged int   `json:"dodged"`
	}
	bossMessage struct {
		HP    int `json:"hp"`
		MaxHP int `json:"max_hp"`
	}
	finaleMessage struct {
		Outcome string `json:"outcome"`
		Boss    string `json:"boss"`
		HP      int    `json:"hp"`
		MaxHP   int    `json:"max_hp"`
		Raiders int    `json:"raiders"`
		Time    int64  `json:"time"`
		RaidTally
	}
)

func (m *wordMessage) word(now time.Time) *RaidWord {
	if m == nil {
		return nil
	}
	w := &RaidWord{N: m.N, Prompt: m.Prompt, Dodge: m.Dodge}
	if m.Dodge {
		w.Until = now.Add(time.Duration(m.In) * time.Millisecond)
	}
	return w
}

func (m *attackMessage) attack(now time.Time) *RaidAttack {
	if m == nil {
		return nil
	}
	return &RaidAttack{N: m.N, Of: m.Of, Until: now.Add(time.Duration(m.In) * time.Millisecond)}
}

func (m *finaleMessage) finale() *RaidFinale {
	if m == nil {
		return nil
	}
	return &RaidFinale{Outcome: m.Outcome, Boss: m.Boss, HP: m.HP, MaxHP: m.MaxHP, Raiders: m.Raiders,
		Time: time.Duration(m.Time) * time.Millisecond, RaidTally: m.RaidTally}
}

// setRaid takes a raid from the server (nil for none). fresh is set for
// a raid that has just started; otherwise (a room sent whole) a raid
// under way keeps its number. p.mu is held.
func (p *Play) setRaid(m *raidMessage, fresh bool) {
	if m == nil {
		p.st.Raid = nil
		return
	}
	now := time.Now()
	rd := &Raid{Boss: m.Boss, Seed: m.Seed, Lang: m.Lang, HP: m.HP, MaxHP: m.MaxHP,
		EndsAt: now.Add(time.Duration(m.EndsIn) * time.Millisecond), Raiders: m.Raiders,
		Attack: m.Attack.attack(now), Word: m.Word.word(now)}
	if old := p.st.Raid; old != nil && !fresh {
		rd.Number, rd.Graded, rd.Hits, rd.LastHit, rd.Grew = old.Number, old.Graded, old.Hits, old.LastHit, old.Grew
	} else {
		p.raids++
		rd.Number = p.raids
	}
	p.st.Raid = rd
}

// handleRaid acts on a raid's messages to the player alone (word,
// graded, summary), which have no place in the room's sequence. p.mu is
// held.
func (p *Play) handleRaid(m inMessage) {
	now := time.Now()
	switch m.T {
	case "summary":
		if m.Summary != nil {
			s := *m.Summary
			p.st.Summary = &s
		}
		return
	}
	rd := p.st.Raid
	if rd == nil {
		return
	}
	switch m.T {
	case "word":
		rd.Word = m.Word.word(now)
	case "graded":
		if g := m.Graded; g != nil {
			rd.Graded = &RaidGraded{N: g.N, Tier: words.Tier(g.Tier), Expected: g.Expected, Damage: g.Damage,
				Dodge: g.Dodge, Dodged: g.Dodged, Late: g.Late}
			if g.Stun > 0 {
				rd.Graded.StunnedUntil = now.Add(time.Duration(g.Stun) * time.Millisecond)
			}
			if rd.Word != nil && rd.Word.N == g.N {
				rd.Word = nil
			}
		}
	}
}

// raidEvent acts on a raid's room event, in sequence. p.mu is held.
func (p *Play) raidEvent(m inMessage) {
	now := time.Now()
	switch m.T {
	case "raid":
		p.setRaid(m.Raid, true)
		p.st.Finale, p.st.Summary = nil, nil
		return
	case "finale":
		p.st.Raid = nil
		p.st.Finale = m.Finale.finale()
		return
	}
	rd := p.st.Raid
	if rd == nil {
		return
	}
	switch m.T {
	case "hit":
		rd.Hits++
		rd.LastHit = RaidHit{ID: m.ID, Damage: m.Damage}
		if m.Boss != nil {
			rd.HP, rd.MaxHP = m.Boss.HP, m.Boss.MaxHP
		}
	case "boss":
		rd.Grew++
		rd.Raiders++
		if m.Boss != nil {
			rd.HP, rd.MaxHP = m.Boss.HP, m.Boss.MaxHP
		}
	case "attack":
		rd.Attack = m.Attack.attack(now)
	case "attacked":
		if a := m.Attack; a != nil {
			rd.Attack = &RaidAttack{N: a.N, Of: a.Of, Dodged: a.Dodged, Done: true}
		}
	}
}
