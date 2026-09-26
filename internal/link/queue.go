package link

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

// DefaultQueueCap is how many events wait to be sent before the oldest
// answers are folded into daily totals (about 50,000; PLAN §3 of
// halpwords-server).
const DefaultQueueCap = 50_000

// defaultCap is the cap when Options.QueueCap is 0: DefaultQueueCap,
// or less in a web browser.
var defaultCap = DefaultQueueCap

// MaxBatch is the most events one POST /api/v1/events may carry.
const MaxBatch = 1000

// Limits of a totals event, from the events schema.
const (
	maxTotalAnswers = 100_000
	maxTotalSecs    = 86_400
)

// qAnswer is an answer to a word of an assigned list, waiting to be sent.
type qAnswer struct {
	Seq         int64
	At          time.Time
	Lang        string // not sent: for folding into totals and merging
	ListID      string
	ListVersion int
	Word        string // words.Key
	Mode        string
	Tier        int
	Hinted      bool     `json:",omitempty"`
	Secs        *float64 `json:",omitempty"`
	Mistake     *int     `json:",omitempty"`
}

// qSession is a play session waiting to be sent.
type qSession struct {
	Seq    int64
	At     time.Time
	Mode   string
	Lang   string
	Secs   int
	Floor  int    `json:",omitempty"`
	ListID string `json:",omitempty"`
}

// qTotals is what the player did with their own lists on a day in a
// language, waiting to be sent. Later answers are added to it until it is
// sent.
type qTotals struct {
	Seq     int64
	Day     string
	Lang    string
	Answers int
	Right   int
	Secs    float64
}

// queue is the events waiting to be sent, each kind in the order of its
// sequence numbers.
type queue struct {
	Answers  []qAnswer  `json:",omitempty"`
	Sessions []qSession `json:",omitempty"`
	Totals   []qTotals  `json:",omitempty"`
	// Folded counts answers folded into totals because the queue was
	// full, for the Account screen.
	Folded int `json:",omitempty"`

	// sending are the sequence numbers of the batch being sent; they
	// can't change until the server has answered.
	sending map[int64]bool
}

func (q *queue) len() int { return len(q.Answers) + len(q.Sessions) + len(q.Totals) }

func (q *queue) maxSeq() int64 {
	var m int64
	for _, a := range q.Answers {
		m = max(m, a.Seq)
	}
	for _, s := range q.Sessions {
		m = max(m, s.Seq)
	}
	for _, t := range q.Totals {
		m = max(m, t.Seq)
	}
	return m
}

// nextSeq hands out a sequence number. c.mu is held.
func (c *Client) nextSeq() int64 {
	s := c.st.NextSeq
	c.st.NextSeq++
	return s
}

// Answer queues an answer given in the game, if the game is linked. lang
// is the language code, mode the game mode and the kind of answer, such as
// "adventure:attack" or "practice". An answer to a word of an assigned
// list is sent as it is; any other answer (to the player's own lists)
// only adds to the day's totals. It returns at once.
func (c *Client) Answer(lang string, e words.Entry, mode string, a words.Answer) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.st.linked() {
		return
	}
	now := c.now()
	key := words.Key(e)
	ref, assigned := c.keys[lang][key]
	if !assigned {
		secs := 0.0
		if a.Timed {
			secs = a.Secs
		}
		c.addTotals(now.Format(time.DateOnly), lang, 1, b2i(a.Tier >= words.Correct), secs)
		c.dirty = true
		return
	}
	qa := qAnswer{Seq: c.nextSeq(), At: now, Lang: lang, ListID: ref.ID, ListVersion: ref.Version,
		Word: key, Mode: cleanMode(mode, true), Tier: int(max(words.Miss, min(words.Perfect, a.Tier))), Hinted: a.Hinted}
	if a.Timed && a.Secs >= 0 {
		s := math.Min(a.Secs, 3600)
		qa.Secs = &s
	}
	if a.Tier < words.Correct && a.Mistake > words.NoMistake && a.Mistake < words.NumMistakes {
		m := int(a.Mistake)
		qa.Mistake = &m
	}
	c.q.Answers = append(c.q.Answers, qa)
	c.dirty = true
	c.fold()
}

// Session is a play session, for Client.Session.
type Session struct {
	Start time.Time
	// Mode is the game mode: "practice", "adventure" or "hardcore".
	Mode string
	Lang string
	// Secs is how long the player was playing, not counting pauses.
	Secs  int
	Floor int // the deepest floor reached, in the dungeon
	// ListID is the assigned list played, if one was; empty otherwise.
	ListID string
}

// Session queues a play session that has ended, if the game is linked.
// Sessions shorter than a few seconds are not sent.
func (c *Client) Session(s Session) {
	if c == nil || s.Secs < 5 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.st.linked() {
		return
	}
	c.q.Sessions = append(c.q.Sessions, qSession{Seq: c.nextSeq(), At: s.Start, Mode: cleanMode(s.Mode, false),
		Lang: s.Lang, Secs: min(s.Secs, maxTotalSecs), Floor: max(0, min(10000, s.Floor)), ListID: s.ListID})
	c.dirty = true
	c.fold()
}

// cleanMode makes a mode fit the schema: lower-case letters and
// underscores, and for answers the kind after a colon.
func cleanMode(mode string, answer bool) string {
	part := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if (r >= 'a' && r <= 'z') || (r == '_' && b.Len() > 0) {
				b.WriteRune(r)
			}
			if b.Len() == 20 {
				break
			}
		}
		return b.String()
	}
	game, kind, _ := strings.Cut(mode, ":")
	game = part(game)
	if game == "" {
		game = "practice"
	}
	if kind = part(kind); answer && kind != "" {
		return game + ":" + kind
	}
	return game
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// addTotals adds to the day's totals in lang that haven't been sent, or
// starts new ones. c.mu is held.
func (c *Client) addTotals(day, lang string, answers, right int, secs float64) {
	for i := len(c.q.Totals) - 1; i >= 0; i-- {
		t := &c.q.Totals[i]
		if t.Day != day || t.Lang != lang || c.q.sending[t.Seq] {
			continue
		}
		if t.Answers+answers <= maxTotalAnswers && t.Secs+secs <= maxTotalSecs {
			t.Answers += answers
			t.Right += right
			t.Secs += secs
			return
		}
		break
	}
	c.q.Totals = append(c.q.Totals, qTotals{Seq: c.nextSeq(), Day: day, Lang: lang,
		Answers: answers, Right: right, Secs: math.Min(secs, maxTotalSecs)})
}

// fold keeps the queue within its cap. Once it is over, the oldest
// answers are folded into their day's totals until it is down to nine
// tenths of the cap (so folding doesn't happen at every answer); if
// that is not enough, the oldest sessions are dropped. c.mu is held.
func (c *Client) fold() {
	limit := c.o.QueueCap
	if c.q.len() <= limit {
		return
	}
	target := limit * 9 / 10
	keep := c.q.Answers[:0:0]
	for i, a := range c.q.Answers {
		if c.q.len()-i+len(keep) <= target || c.q.sending[a.Seq] {
			keep = append(keep, a)
			continue
		}
		secs := 0.0
		if a.Secs != nil {
			secs = *a.Secs
		}
		c.addTotals(a.At.Format(time.DateOnly), a.Lang, 1, b2i(a.Tier >= int(words.Correct)), secs)
		c.q.Folded++
	}
	c.q.Answers = keep
	for len(c.q.Sessions) > 0 && c.q.len() > limit && !c.q.sending[c.q.Sessions[0].Seq] {
		c.q.Sessions = c.q.Sessions[1:]
	}
}

// saveQueue writes the queue if it changed. It marshals a copy, so the
// game loop is held up only while the slices are copied.
func (c *Client) saveQueue() error { return c.writeQueue(false) }

// writeQueue writes the queue if it changed. With try, it gives up rather
// than wait for the lock.
func (c *Client) writeQueue(try bool) error {
	if try {
		if !c.mu.TryLock() {
			return errBusy
		}
	} else {
		c.mu.Lock()
	}
	if !c.dirty || c.o.Store == nil {
		c.mu.Unlock()
		return nil
	}
	q := queue{
		Answers:  append([]qAnswer(nil), c.q.Answers...),
		Sessions: append([]qSession(nil), c.q.Sessions...),
		Totals:   append([]qTotals(nil), c.q.Totals...),
		Folded:   c.q.Folded,
	}
	c.dirty = false
	c.mu.Unlock()
	var err error
	if q.len() == 0 && q.Folded == 0 {
		err = c.o.Store.Remove(queueFile)
	} else {
		var data []byte
		if data, err = json.Marshal(q); err == nil {
			err = c.o.Store.Write(queueFile, data)
		}
	}
	if err != nil {
		if try && !c.mu.TryLock() {
			return err
		}
		if !try {
			c.mu.Lock()
		}
		c.dirty = true
		c.mu.Unlock()
	}
	return err
}

// errBusy is a TrySave that found the link busy.
var errBusy = errors.New("link: busy")

// Save writes the queue of events to disk, if it changed. The link saves
// it every few seconds by itself once started, and on Close.
func (c *Client) Save() error {
	if c == nil {
		return nil
	}
	return c.saveQueue()
}

// TrySave is Save that never waits: it fails if the link is busy. A web
// page's event handlers use it, as they must not block.
func (c *Client) TrySave() error {
	if c == nil {
		return nil
	}
	return c.writeQueue(true)
}

// The wire format of POST /api/v1/events (docs/api/events.request).
type (
	wireAnswer struct {
		Seq         int64    `json:"seq"`
		At          string   `json:"at"`
		ListID      string   `json:"list_id"`
		ListVersion int      `json:"list_version"`
		Word        string   `json:"word"`
		Mode        string   `json:"mode"`
		Tier        int      `json:"tier"`
		Hinted      bool     `json:"hinted,omitempty"`
		Secs        *float64 `json:"secs,omitempty"`
		Mistake     *int     `json:"mistake,omitempty"`
	}
	wireSession struct {
		Seq    int64  `json:"seq"`
		At     string `json:"at"`
		Mode   string `json:"mode"`
		Lang   string `json:"lang"`
		Secs   int    `json:"secs"`
		Floor  int    `json:"floor,omitempty"`
		ListID string `json:"list_id,omitempty"`
	}
	wireTotals struct {
		Seq     int64  `json:"seq"`
		Day     string `json:"day"`
		Lang    string `json:"lang"`
		Answers int    `json:"answers"`
		Right   int    `json:"right"`
		Secs    int    `json:"secs"`
	}
	wireBatch struct {
		Answers  []wireAnswer  `json:"answers,omitempty"`
		Sessions []wireSession `json:"sessions,omitempty"`
		Totals   []wireTotals  `json:"totals,omitempty"`
	}
)

// wireTime is a time as the events schema wants it: RFC 3339 with an
// offset, to the millisecond.
func wireTime(t time.Time) string { return t.Truncate(time.Millisecond).Format(time.RFC3339Nano) }

// takeBatch makes a batch of up to n of the oldest events and marks them
// as being sent. It returns the batch's sequence numbers in the order of
// its arrays: answers, sessions, then totals. c.mu is held.
func (c *Client) takeBatch(n int) (wireBatch, []int64) {
	var b wireBatch
	var seqs []int64
	ia, is, it := 0, 0, 0
	qa, qs, qt := c.q.Answers, c.q.Sessions, c.q.Totals
	const none = math.MaxInt64
	for len(b.Answers)+len(b.Sessions)+len(b.Totals) < n {
		sa, ss, st := int64(none), int64(none), int64(none)
		if ia < len(qa) {
			sa = qa[ia].Seq
		}
		if is < len(qs) {
			ss = qs[is].Seq
		}
		if it < len(qt) {
			st = qt[it].Seq
		}
		switch {
		case sa == none && ss == none && st == none:
			n = 0
		case sa <= ss && sa <= st:
			a := qa[ia]
			b.Answers = append(b.Answers, wireAnswer{Seq: a.Seq, At: wireTime(a.At), ListID: a.ListID, ListVersion: a.ListVersion,
				Word: a.Word, Mode: a.Mode, Tier: a.Tier, Hinted: a.Hinted, Secs: a.Secs, Mistake: a.Mistake})
			ia++
		case ss <= st:
			s := qs[is]
			b.Sessions = append(b.Sessions, wireSession{Seq: s.Seq, At: wireTime(s.At), Mode: s.Mode, Lang: s.Lang,
				Secs: s.Secs, Floor: s.Floor, ListID: s.ListID})
			is++
		default:
			t := qt[it]
			b.Totals = append(b.Totals, wireTotals{Seq: t.Seq, Day: t.Day, Lang: t.Lang, Answers: t.Answers,
				Right: min(t.Right, t.Answers), Secs: int(math.Round(math.Min(t.Secs, maxTotalSecs)))})
			it++
		}
	}
	c.q.sending = map[int64]bool{}
	for _, a := range b.Answers {
		seqs = append(seqs, a.Seq)
	}
	for _, s := range b.Sessions {
		seqs = append(seqs, s.Seq)
	}
	for _, t := range b.Totals {
		seqs = append(seqs, t.Seq)
	}
	for _, s := range seqs {
		c.q.sending[s] = true
	}
	return b, seqs
}

// drop removes the events with the given sequence numbers from the queue.
// c.mu is held.
func (c *Client) drop(seqs map[int64]bool) {
	if len(seqs) == 0 {
		return
	}
	a := c.q.Answers[:0]
	for _, x := range c.q.Answers {
		if !seqs[x.Seq] {
			a = append(a, x)
		}
	}
	s := c.q.Sessions[:0]
	for _, x := range c.q.Sessions {
		if !seqs[x.Seq] {
			s = append(s, x)
		}
	}
	t := c.q.Totals[:0]
	for _, x := range c.q.Totals {
		if !seqs[x.Seq] {
			t = append(t, x)
		}
	}
	c.q.Answers, c.q.Sessions, c.q.Totals = a, s, t
	c.dirty = true
}
