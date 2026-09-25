// Package link connects the game to a grown-up's account on
// halpwords-server: linking with a pairing code, the device tokens,
// assigned word lists, a queue of answers and play sessions sent in the
// background, the word memory from the server, and settings a grown-up
// set. The API is halpwords-server's docs/api. It has no Ebitengine
// dependency.
//
// Like the AI helper (PLAN §10), the link never blocks the game loop:
// requests run in goroutines with timeouts, answers wait in a queue on
// disk until the server can be reached, and failures are quiet. The game
// plays the same with the server down, or when it was never linked.
package link

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/pkg/words"
)

// Store keeps the link's files. The game passes its save folder (local
// storage on the web).
type Store interface {
	Read(name string) ([]byte, error)
	Write(name string, data []byte) error
	// WritePrivate writes a file only the user can read, for the tokens.
	WritePrivate(name string, data []byte) error
	Remove(name string) error
}

// The files the link keeps.
const (
	stateFile = "link.json"       // tokens, the learner and the lists: private
	queueFile = "link-queue.json" // events waiting to be sent
	// AssignedDir is the folder the assigned lists are kept in, as the
	// server sent them.
	AssignedDir = "assigned"
)

// DefaultServer is the server a game links to, unless HALPWORDS_SERVER
// names another (such as a staging server).
const DefaultServer = "https://halpwords.com"

// ServerURL is the server's address, without a trailing slash.
func ServerURL() string {
	if s := strings.TrimSpace(os.Getenv("HALPWORDS_SERVER")); s != "" {
		return strings.TrimRight(s, "/")
	}
	return DefaultServer
}

// Timings. They are variables so tests can shorten them.
var (
	// SyncEvery is how often a linked game syncs while it runs.
	SyncEvery = 3 * time.Minute
	// maxBackoff is the longest wait between syncs while the server
	// can't be reached.
	maxBackoff = 30 * time.Minute
	// saveEvery is how often the queue is written to disk while events
	// are coming in.
	saveEvery = 10 * time.Second
	// requestTimeout limits each request; syncTimeout a whole sync.
	requestTimeout = 20 * time.Second
	syncTimeout    = 2 * time.Minute
	// closeTimeout is how long quitting waits to send the last events.
	closeTimeout = 3 * time.Second
	// refreshEarly is how long before the access token runs out it is
	// refreshed.
	refreshEarly = 5 * time.Minute
)

// quietAfter is how many syncs in a row may fail before the game stops
// mentioning it and just says it is offline.
const quietAfter = 3

// Options set up a Client.
type Options struct {
	// Store keeps the files; nil keeps nothing.
	Store Store
	// Server is the server's address; empty is ServerURL().
	Server string
	// HTTP makes the requests; nil is a client with a timeout.
	HTTP *http.Client
	// Version is the game's version, such as "v1.2.0" or "dev", for the
	// User-Agent.
	Version string
	// Name is what the game calls itself on the learner's page, such as
	// "Halpwords on Windows"; empty names the system.
	Name string
	// OwnDir is the folder of the player's own word lists. A list that is
	// no longer assigned, or every list when the game is unlinked, is kept
	// there as the player's own (licensed lists are removed).
	OwnDir string
	// QueueCap is how many events the queue holds before the oldest
	// answers are folded into daily totals; 0 is DefaultQueueCap.
	QueueCap int
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// Me is the linked learner, from GET /api/v1/me.
type Me struct {
	Learner struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Avatar      struct {
			Class  string `json:"class"`
			Colour string `json:"colour"`
		} `json:"avatar"`
		Languages []string `json:"languages"`
	} `json:"learner"`
	// Settings are the settings a grown-up set, by language.
	Settings       profile.Settings `json:"settings"`
	Accommodations Accommodations   `json:"accommodations"`
	// SeenBy are the roles of the adults who can see the learner's
	// progress.
	SeenBy []string `json:"seen_by"`
	Game   struct {
		Version    string `json:"version"`
		MinVersion string `json:"min_version"`
		Supported  bool   `json:"supported"`
	} `json:"game"`
}

// Accommodations are what the family turned on for the learner.
type Accommodations struct {
	RelaxedTimers bool `json:"relaxed_timers,omitempty"`
	IgnoreAccents bool `json:"ignore_accents,omitempty"`
	CheaperHints  bool `json:"cheaper_hints,omitempty"`
	LargerText    bool `json:"larger_text,omitempty"`
	NoTimedDodges bool `json:"no_timed_dodges,omitempty"`
}

// ListInfo is an assigned list the game keeps.
type ListInfo struct {
	ID       string
	Version  int
	Title    string
	Language string
	// File is its file in AssignedDir.
	File string
	// Licensed lists are a publisher's: they can't be kept after
	// unlinking.
	Licensed bool `json:",omitempty"`
}

// Quest is an assignment as a quest, from GET /api/v1/assignments.
type Quest struct {
	ID   string `json:"id"`
	List struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
		Title   string `json:"title"`
	} `json:"list"`
	Goal struct {
		Kind string `json:"kind"`
		N    int    `json:"n,omitempty"`
	} `json:"goal"`
	Mode     string `json:"mode"`
	StartsAt string `json:"starts_at"`
	DueAt    string `json:"due_at,omitempty"`
	Progress struct {
		Answers  int  `json:"answers"`
		Done     int  `json:"done"`
		Target   int  `json:"target"`
		Percent  int  `json:"percent"`
		Complete bool `json:"complete"`
	} `json:"progress"`
}

// state is what link.json keeps.
type state struct {
	Server     string
	DeviceID   string
	Access     string
	AccessExp  time.Time
	Refresh    string
	RefreshExp time.Time
	LinkedAt   time.Time
	LastSync   time.Time `json:",omitempty"`
	// NextSeq is the sequence number of the next event. It only grows,
	// even across unlinking, so no number is ever used twice.
	NextSeq   int64
	Me        *Me        `json:",omitempty"`
	ListsETag string     `json:",omitempty"`
	Lists     []ListInfo `json:",omitempty"`
	Quests    []Quest    `json:",omitempty"`
}

func (s *state) linked() bool { return s.Refresh != "" }

// Status is what the Account screen shows.
type Status struct {
	Linked bool
	// Busy is set while the game is linking or syncing.
	Busy bool
	// Learner is the linked learner's display name.
	Learner string
	// SeenBy are the roles of the adults who can see the learner's
	// progress; SeenByText says it in words.
	SeenBy   []string
	LinkedAt time.Time
	// LastSync is when the game last synced; zero before the first.
	LastSync time.Time
	// Pending is how many events wait to be sent.
	Pending int
	// Err is why the last link or sync failed, or nil if it worked.
	Err error
	// Offline is set once several syncs in a row have failed. The game
	// keeps trying, less often, and says nothing more.
	Offline bool
	// Note is something to tell the player once, such as that the game
	// was unlinked on the website.
	Note string
	// Outdated is set when the server says this game is too old for
	// some of its features.
	Outdated bool
	Lists    int
}

// Client is the game's link to the server. It is safe to use from
// several goroutines; nothing it does from the game loop waits for the
// network.
type Client struct {
	o      Options
	hc     *http.Client
	now    func() time.Time
	server string

	// syncMu is held by whatever talks to the server, so one exchange
	// runs at a time and tokens are never refreshed twice at once.
	syncMu sync.Mutex

	mu    sync.Mutex
	st    state
	q     queue
	dirty bool // the queue changed since it was written
	// gen counts links and unlinks. A sync that started under another
	// generation throws away what it got.
	gen int
	// lists are the assigned lists, parsed; keys finds the list a word
	// of a language is in.
	lists []*words.List
	keys  map[string]map[string]listRef
	// memories are word memories from the server waiting for the game
	// loop to merge them, and uploads counts uploads, so a merge never
	// uses a memory older than an upload.
	memories map[string]*fetched
	uploads  int
	// changes counts changes the game loop should pick up: lists, the
	// learner's settings or memory.
	changes int

	busy     bool
	err      error
	failures int
	note     string
	batch    int // events per batch; halved after a too_large answer

	kick    chan struct{}
	stop    chan struct{}
	stopped chan struct{}
	sleep   func(time.Duration) // between retries; tests make it instant
}

// listRef is the list an answer names.
type listRef struct {
	ID      string
	Version int
}

// fetched is a word memory from the server.
type fetched struct {
	mem     *words.Memory
	uploads int
}

// Open reads the link's files from o.Store and returns the client. A
// damaged file is treated as missing: the game then simply isn't linked.
// Call Start to sync in the background.
func Open(o Options) *Client {
	c := &Client{
		o:        o,
		hc:       o.HTTP,
		now:      o.Now,
		server:   strings.TrimRight(o.Server, "/"),
		memories: map[string]*fetched{},
		batch:    MaxBatch,
		kick:     make(chan struct{}, 1),
		sleep:    time.Sleep,
	}
	if c.hc == nil {
		c.hc = &http.Client{Timeout: requestTimeout}
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.server == "" {
		c.server = ServerURL()
	}
	if c.o.QueueCap <= 0 {
		c.o.QueueCap = DefaultQueueCap
	}
	if o.Store != nil {
		if data, err := o.Store.Read(stateFile); err == nil {
			json.Unmarshal(data, &c.st)
		}
		if data, err := o.Store.Read(queueFile); err == nil {
			json.Unmarshal(data, &c.q)
		}
	}
	c.st.NextSeq = max(c.st.NextSeq, c.q.maxSeq()+1, 1)
	if c.st.linked() && c.st.Server != "" && c.st.Server != c.server {
		// Linked to another server (HALPWORDS_SERVER changed): talk to
		// the one the tokens are for.
		c.server = c.st.Server
	}
	c.loadLists()
	return c
}

// Linked reports whether the game is linked to a learner.
func (c *Client) Linked() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.linked()
}

// Server is the address of the server the game links to.
func (c *Client) Server() string { return c.server }

// AccessToken is the device's access token, or "" when the game isn't
// linked. It may have run out; the link refreshes it at its next sync.
func (c *Client) AccessToken() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.Access
}

// Status says how the link is doing.
func (c *Client) Status() Status {
	if c == nil {
		return Status{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Status{
		Linked:   c.st.linked(),
		Busy:     c.busy,
		LinkedAt: c.st.LinkedAt,
		LastSync: c.st.LastSync,
		Pending:  c.q.len(),
		Err:      c.err,
		Offline:  c.failures >= quietAfter,
		Note:     c.note,
		Lists:    len(c.st.Lists),
	}
	if m := c.st.Me; m != nil {
		s.Learner = m.Learner.DisplayName
		s.SeenBy = slices.Clone(m.SeenBy)
		s.Outdated = !m.Game.Supported
	}
	return s
}

// ClearNote forgets Status.Note once the player has seen it.
func (c *Client) ClearNote() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.note = ""
}

// Changes counts changes the game should pick up: new or removed lists,
// settings from a grown-up, or a word memory to merge. The game loop
// compares it with the last count it saw.
func (c *Client) Changes() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.changes
}

// Quests are the learner's assignments, as the server last sent them.
func (c *Client) Quests() []Quest {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.st.Quests)
}

// Me is the linked learner as the server last described them, or nil.
func (c *Client) Me() *Me {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.Me == nil {
		return nil
	}
	m := *c.st.Me
	return &m
}

// SeenByText says who can see the learner's progress, such as "Your
// teacher and your parent can see your progress."
func SeenByText(roles []string) string {
	var who []string
	add := func(s string) {
		if !slices.Contains(who, s) {
			who = append(who, s)
		}
	}
	parents := 0
	for _, r := range roles {
		if r == "guardian" || r == "co-guardian" {
			parents++
		}
	}
	for _, r := range roles {
		switch r {
		case "teacher":
			add("your teacher")
		case "teaching-assistant":
			add("a teaching assistant")
		case "guardian", "co-guardian":
			if parents > 1 {
				add("your parents")
			} else {
				add("your parent")
			}
		}
	}
	switch len(who) {
	case 0:
		return "Only the grown-up who linked this game can see your progress."
	case 1:
		return upper(who[0]) + " can see your progress."
	}
	return upper(strings.Join(who[:len(who)-1], ", ")+" and "+who[len(who)-1]) + " can see your progress."
}

func upper(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Locked returns the settings a grown-up set for lang, starting from
// own, the player's own: the language's settings from the website, then
// the accommodations (relaxed timers, accents ignored). It reports false
// when nothing is set, and the player's own settings apply.
func (c *Client) Locked(lang *words.Language, own profile.LangSettings) (profile.LangSettings, bool) {
	if c == nil {
		return own, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.st.Me
	if m == nil || !c.st.linked() {
		return own, false
	}
	ls, locked := m.Settings.Langs[lang.Code]
	if !locked {
		ls = own
	}
	if m.Accommodations.RelaxedTimers {
		ls.Timer, locked = profile.Relaxed, true
	}
	if m.Accommodations.IgnoreAccents {
		ls.Rules.Accents, ls.Rules.Breathings, locked = words.Ignore, words.Ignore, true
	}
	return ls, locked
}

// userAgent is the game's User-Agent: "Halpwords/1.2.0 (darwin; arm64)",
// or "Halpwords/dev (…)" for a build without a release version.
func (c *Client) userAgent() string {
	v := strings.TrimPrefix(strings.TrimSpace(c.o.Version), "v")
	if v == "" || v[0] < '0' || v[0] > '9' || strings.ContainsAny(v, " ;()") {
		v = "dev"
	}
	return "Halpwords/" + v + " (" + runtime.GOOS + "; " + runtime.GOARCH + ")"
}

// deviceName is what the game calls itself on the learner's page.
func (c *Client) deviceName() string {
	if c.o.Name != "" {
		return c.o.Name
	}
	return "Halpwords on " + map[string]string{
		"darwin": "a Mac", "windows": "Windows", "linux": "Linux", "js": "the web",
	}[runtime.GOOS]
}

// saveState writes link.json. c.mu is held.
func (c *Client) saveState() error {
	if c.o.Store == nil {
		return nil
	}
	if !c.st.linked() && c.st.NextSeq <= 1 {
		return c.o.Store.Remove(stateFile)
	}
	data, err := json.MarshalIndent(c.st, "", "  ")
	if err != nil {
		return err
	}
	return c.o.Store.WritePrivate(stateFile, data)
}

// errNoStore is returned when a file is needed and there is no store.
var errNoStore = errors.New("link: nowhere to keep files")
