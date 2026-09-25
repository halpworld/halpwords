package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store keeps the service's files. The game passes its save folder.
type Store interface {
	Read(name string) ([]byte, error)
	Write(name string, data []byte) error
	// WritePrivate writes a file only the user can read, for the keys.
	WritePrivate(name string, data []byte) error
}

// The files the service keeps.
const (
	configFile = "ai.json"       // provider, keys, models and budgets: private
	spendFile  = "ai-spend.json" // what has been spent
)

// NoLimit is the budget that never runs out.
const NoLimit = -1.0

// DefaultBudget is the budget a provider starts with, in dollars.
const DefaultBudget = 5.0

// Budgets are the budgets the setup screen offers.
var Budgets = []float64{0.5, 1, 2, 5, 10, 20, 50, 100, NoLimit}

// Models are the models chosen for a provider.
type Models struct {
	Game  string `json:",omitempty"`
	Forge string `json:",omitempty"`
}

// Config is what a parent or teacher set up.
type Config struct {
	Provider ProviderID
	Keys     map[ProviderID]string  `json:",omitempty"`
	Models   map[ProviderID]Models  `json:",omitempty"`
	Budgets  map[ProviderID]float64 `json:",omitempty"`
}

// Tally is what has been spent with a provider since its counter was last
// reset.
type Tally struct {
	Since    time.Time
	Spent    float64 // dollars
	Requests int
	In, Out  int64 // tokens
	// Estimated is set when some requests used a model whose price the game
	// does not know, so they were counted at a high price.
	Estimated bool `json:",omitempty"`
}

// Status is what the game knows about the connection to a provider.
type Status struct {
	Checking bool
	// Checked is set once the key has been tried.
	Checked bool
	// Err is the last error, or nil when the last request worked.
	Err error
	// Listed are the models the provider says the key can use, chat
	// models only.
	Listed []string
	// Balance is the money left on the account, for providers that can
	// say; BalanceOK is set once it has been read.
	Balance   Balance
	BalanceOK bool
	// Tried is the model's answer to the setup screen's test.
	Tried string
}

// Service is the game's connection to a model. It is safe to use from
// several goroutines.
type Service struct {
	mu      sync.Mutex
	store   Store
	cfg     Config
	spent   map[ProviderID]*Tally
	session float64 // dollars spent since the game started
	held    float64 // dollars set aside for requests in flight
	status  map[ProviderID]*Status
	paused  time.Time     // after a busy reply, wait until then
	slots   chan struct{} // one for each request in flight
	banks   map[string]*Bank

	// For tests.
	hc   *http.Client
	urls map[ProviderID]string
	now  func() time.Time
	env  func(string) string
}

// maxRunning is how many requests may be in flight at once; others wait
// their turn.
const maxRunning = 2

// minTokens is the smallest output limit a request gets, so a model that
// thinks first still has room to answer.
const minTokens = 512

// busyPause is how long to leave a provider alone after it says it's busy.
const busyPause = 90 * time.Second

// Load reads the service's settings from store. With a nil store nothing is
// kept.
func Load(store Store) *Service {
	s := &Service{
		store:  store,
		spent:  map[ProviderID]*Tally{},
		status: map[ProviderID]*Status{},
		slots:  make(chan struct{}, maxRunning),
		hc:     httpClient(),
		now:    time.Now,
		env:    os.Getenv,
	}
	if store != nil {
		if data, err := store.Read(configFile); err == nil {
			json.Unmarshal(data, &s.cfg)
		}
		if data, err := store.Read(spendFile); err == nil {
			json.Unmarshal(data, &s.spent)
		}
	}
	if ProviderFor(s.cfg.Provider) == nil {
		s.cfg.Provider = Off
	}
	return s
}

func (s *Service) saveConfig() error {
	if s.store == nil {
		return nil
	}
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	return s.store.WritePrivate(configFile, data)
}

func (s *Service) saveSpent() error {
	if s.store == nil {
		return nil
	}
	data, err := json.MarshalIndent(s.spent, "", "  ")
	if err != nil {
		return err
	}
	return s.store.Write(spendFile, data)
}

// UseEndpoint sends p's requests to url with hc instead of to the
// provider's own API, for tests or a local proxy.
func (s *Service) UseEndpoint(p ProviderID, url string, hc *http.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.urls == nil {
		s.urls = map[ProviderID]string{}
	}
	s.urls[p], s.hc = url, hc
}

// IgnoreEnvironment stops keys being read from environment variables.
func (s *Service) IgnoreEnvironment() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.env = func(string) string { return "" }
}

// Provider returns the chosen provider, or nil when the game plays without
// a model.
func (s *Service) Provider() *Provider {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return ProviderFor(s.cfg.Provider)
}

// SetProvider chooses the provider, or Off.
func (s *Service) SetProvider(id ProviderID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Provider = id
	return s.saveConfig()
}

// key returns the key for p: the one set in the game, or else one from the
// environment. The lock must be held.
func (s *Service) key(p ProviderID) (key string, fromEnv bool) {
	if k := s.cfg.Keys[p]; k != "" {
		return k, false
	}
	if pr := ProviderFor(p); pr != nil {
		for _, v := range pr.EnvVars {
			if k := strings.TrimSpace(s.env(v)); k != "" {
				return k, true
			}
		}
	}
	return "", false
}

// Key returns the key for p, masked, and whether it came from an
// environment variable. It is empty when there is none.
func (s *Service) Key(p ProviderID) (masked string, fromEnv bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, env := s.key(p)
	if k == "" {
		return "", false
	}
	return Mask(k), env
}

// EnvVar returns the environment variable that holds a key for p, or "".
func (s *Service) EnvVar(p ProviderID) string {
	if pr := ProviderFor(p); pr != nil {
		for _, v := range pr.EnvVars {
			if strings.TrimSpace(s.env(v)) != "" {
				return v
			}
		}
	}
	return ""
}

// SetKey sets the key for p. An empty key removes it. The key is checked
// in the background.
func (s *Service) SetKey(p ProviderID, key string) error {
	key = CleanKey(key)
	s.mu.Lock()
	if s.cfg.Keys == nil {
		s.cfg.Keys = map[ProviderID]string{}
	}
	if key == "" {
		delete(s.cfg.Keys, p)
	} else {
		s.cfg.Keys[p] = key
	}
	s.status[p] = &Status{}
	err := s.saveConfig()
	s.mu.Unlock()
	s.Check(p)
	return err
}

// CleanKey tidies a pasted key: spaces, line breaks and quotes around it,
// or a whole "NAME=key" line from a settings file.
func CleanKey(key string) string {
	key = strings.TrimPrefix(strings.TrimSpace(key), "export ")
	if name, v, ok := strings.Cut(key, "="); ok && !strings.ContainsAny(name, " \t") && strings.ToUpper(name) == name {
		key = strings.TrimSpace(v)
	}
	key = strings.Trim(key, "\"'` ")
	return strings.Join(strings.Fields(key), "")
}

// Model returns the model used for play, or with forge the one for making
// word lists.
func (s *Service) Model(p ProviderID, forge bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model(p, forge)
}

func (s *Service) model(p ProviderID, forge bool) string {
	m := s.cfg.Models[p]
	pr := ProviderFor(p)
	switch {
	case forge && m.Forge != "":
		return m.Forge
	case !forge && m.Game != "":
		return m.Game
	case pr == nil:
		return ""
	case forge:
		return pr.ForgeModel
	}
	return pr.GameModel
}

// SetModel chooses a model.
func (s *Service) SetModel(p ProviderID, forge bool, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Models == nil {
		s.cfg.Models = map[ProviderID]Models{}
	}
	m := s.cfg.Models[p]
	if forge {
		m.Forge = id
	} else {
		m.Game = id
	}
	s.cfg.Models[p] = m
	if st := s.status[p]; st != nil && errors.Is(st.Err, ErrModel) {
		st.Err = nil
	}
	return s.saveConfig()
}

// Budget returns the budget for p in dollars, or NoLimit.
func (s *Service) Budget(p ProviderID) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.budget(p)
}

func (s *Service) budget(p ProviderID) float64 {
	if b, ok := s.cfg.Budgets[p]; ok && (b > 0 || b == NoLimit) {
		return b
	}
	return DefaultBudget
}

// SetBudget sets the budget for p.
func (s *Service) SetBudget(p ProviderID, usd float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Budgets == nil {
		s.cfg.Budgets = map[ProviderID]float64{}
	}
	s.cfg.Budgets[p] = usd
	if st := s.status[p]; st != nil && errors.Is(st.Err, ErrBudget) {
		st.Err = nil
	}
	return s.saveConfig()
}

// ResetSpent starts counting what p spends from zero, after adding credit.
func (s *Service) ResetSpent(p ProviderID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spent[p] = &Tally{Since: s.now()}
	if st := s.status[p]; st != nil && (errors.Is(st.Err, ErrBudget) || errors.Is(st.Err, ErrFunds)) {
		st.Err = nil
	}
	return s.saveSpent()
}

// Spent returns what p has spent since its counter was reset.
func (s *Service) Spent(p ProviderID) Tally {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.spent[p]; t != nil {
		return *t
	}
	return Tally{}
}

// Session is what has been spent since the game started, with any
// provider.
func (s *Service) Session() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session
}

// Left is the money left in p's budget, or NoLimit.
func (s *Service) Left(p ProviderID) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.left(p)
}

func (s *Service) left(p ProviderID) float64 {
	b := s.budget(p)
	if b == NoLimit {
		return NoLimit
	}
	spent := 0.0
	if t := s.spent[p]; t != nil {
		spent = t.Spent
	}
	return max(0, b-spent)
}

// Status returns what is known about the connection to p.
func (s *Service) Status(p ProviderID) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.status[p]; st != nil {
		return *st
	}
	return Status{}
}

func (s *Service) statusFor(p ProviderID) *Status {
	st := s.status[p]
	if st == nil {
		st = &Status{}
		s.status[p] = st
	}
	return st
}

// Ready reports whether the game can use the model now: a provider is
// chosen, it has a key that has not been refused, and there is money left.
// Features that need the model are on exactly when Ready.
func (s *Service) Ready() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready() == nil
}

// Problem says why the model can't be used now, or "" when it can.
func (s *Service) Problem() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Explain(s.ready())
}

func (s *Service) ready() error {
	p := s.cfg.Provider
	if p == Off {
		return errors.New("no provider is chosen")
	}
	if k, _ := s.key(p); k == "" {
		return ErrNoKey
	}
	if st := s.status[p]; st != nil && st.Err != nil &&
		(errors.Is(st.Err, ErrKey) || errors.Is(st.Err, ErrFunds) || errors.Is(st.Err, ErrModel)) {
		return st.Err
	}
	if s.left(p) != NoLimit && s.left(p) < 0.001 {
		return ErrBudget
	}
	return nil
}

// Check tries p's key in the background: it lists the models the key can
// use, which costs nothing, and reads the account's balance where the
// provider can say.
func (s *Service) Check(p ProviderID) {
	pr := ProviderFor(p)
	if pr == nil {
		return
	}
	s.mu.Lock()
	key, _ := s.key(p)
	st := s.statusFor(p)
	if key == "" || st.Checking {
		s.mu.Unlock()
		return
	}
	st.Checking = true
	b := newBackend(pr, key, s.hc, s.urls[p])
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ids, err := b.models(ctx)
		var listed []string
		for _, id := range ids {
			if chatModel(p, id) {
				listed = append(listed, id)
			}
		}
		sort.Strings(listed)
		bal, balErr := b.balance(ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		if cur, _ := s.key(p); cur != key {
			return // the key changed meanwhile
		}
		st := s.statusFor(p)
		st.Checking, st.Checked, st.Err = false, true, err
		if err == nil {
			st.Listed = listed
		}
		if balErr == nil {
			st.Balance, st.BalanceOK = bal, true
			if !bal.Usable && err == nil {
				st.Err = ErrFunds
			}
		}
	}()
}

// Try sends a tiny request with the game model, so the setup screen can
// show the connection working and what one request costs.
func (s *Service) Try() *Job[string] {
	return Start(func() (string, error) {
		text, err := s.Ask(context.Background(), false,
			Policy, "In at most 12 words, greet a young adventurer about to enter a dungeon of words.", 60)
		if err == nil {
			text = firstLine(text)
			s.mu.Lock()
			s.statusFor(s.cfg.Provider).Tried = text
			s.mu.Unlock()
		}
		return text, err
	})
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(s, "\"")
}

// Ask sends a request to the chosen provider with the game model, or with
// forge the word list model, and returns the reply's text. It fails with
// ErrBudget, without sending anything, if the request could cost more than
// is left in the budget. What was spent is counted and saved.
func (s *Service) Ask(ctx context.Context, forge bool, system, prompt string, maxTokens int) (string, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	s.mu.Lock()
	if err := s.ready(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	now := s.now()
	if now.Before(s.paused) {
		s.mu.Unlock()
		return "", ErrBusy
	}
	p := s.cfg.Provider
	pr := ProviderFor(p)
	key, _ := s.key(p)
	model := s.model(p, forge)
	maxTokens = max(minTokens, maxTokens*max(1, pr.headroom))
	// Set aside the most this request can cost: about four characters a
	// token going in, and every token allowed coming out.
	worst, _ := Cost(p, model, int64(len(system)+len(prompt))/3+20, int64(maxTokens), now)
	if left := s.left(p); left != NoLimit && worst > left-s.held {
		s.mu.Unlock()
		return "", ErrBudget
	}
	s.held += worst
	b := newBackend(pr, key, s.hc, s.urls[p])
	s.mu.Unlock()

	rep, err := b.complete(ctx, Request{Model: model, System: system, Prompt: prompt, MaxTokens: maxTokens})

	s.mu.Lock()
	defer s.mu.Unlock()
	s.held -= worst
	if rep.In > 0 || rep.Out > 0 {
		usd, exact := Cost(p, model, rep.In, rep.Out, now)
		t := s.spent[p]
		if t == nil {
			t = &Tally{Since: now}
			s.spent[p] = t
		}
		t.Spent += usd
		t.Requests++
		t.In += rep.In
		t.Out += rep.Out
		t.Estimated = t.Estimated || !exact
		s.session += usd
		s.saveSpent()
	}
	st := s.statusFor(p)
	switch {
	case err == nil:
		st.Err = nil
	case errors.Is(err, ErrBusy):
		s.paused = now.Add(busyPause)
	case errors.Is(err, ErrRefused), errors.Is(err, context.Canceled):
	default:
		st.Err = err
	}
	if err != nil {
		return "", err
	}
	return rep.Text, nil
}
