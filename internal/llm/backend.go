package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Request is one question for the model.
type Request struct {
	Model  string
	System string
	Prompt string
	// MaxTokens caps the reply. It also caps what the request can cost.
	MaxTokens int
}

// Reply is the model's answer and the tokens it used.
type Reply struct {
	Text    string
	In, Out int64
}

// Balance is the money left on an account, for providers that can say.
type Balance struct {
	Amount   float64
	Currency string
	// Usable is false when the provider says the account can't pay for
	// more requests.
	Usable bool
}

// backend talks to one provider's API.
type backend interface {
	complete(ctx context.Context, r Request) (Reply, error)
	models(ctx context.Context) ([]string, error)
	balance(ctx context.Context) (Balance, error)
}

// The errors a request can fail with, so the game can explain them.
var (
	ErrNoKey   = errors.New("no API key is set")
	ErrKey     = errors.New("the API key was refused")
	ErrFunds   = errors.New("the account has run out of credit")
	ErrBudget  = errors.New("the budget is spent")
	ErrBusy    = errors.New("the service is busy; try again soon")
	ErrModel   = errors.New("that model is not available with this key")
	ErrRefused = errors.New("the model declined to answer")
	ErrEmpty   = errors.New("the model's answer could not be used")
)

// Explain describes err in words a parent or teacher can act on.
func Explain(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNoKey):
		return "No API key yet."
	case errors.Is(err, ErrKey):
		return "The key was refused. Check it was copied in full."
	case errors.Is(err, ErrFunds):
		return "The account has no credit left. Add some on the provider's site."
	case errors.Is(err, ErrBudget):
		return "The budget set here is spent. Raise it or reset the counter."
	case errors.Is(err, ErrBusy):
		return "The service is busy. The game will try again later."
	case errors.Is(err, ErrModel):
		return "This key can't use that model. Pick another."
	case errors.Is(err, ErrNoHalpwords):
		return "Halpwords AI needs a linked game on a plan that includes it."
	case errors.Is(err, ErrAllowance):
		return "This month's Halpwords AI is used up. It comes back next month."
	case errors.Is(err, ErrNeedsKey):
		return "This needs an AI with your own key: set one up in AI Helper."
	case errors.Is(err, ErrRefused):
		return "The model declined that request."
	case errors.Is(err, context.DeadlineExceeded):
		return "The service took too long to answer."
	}
	msg := err.Error()
	if len(msg) > 90 {
		msg = msg[:87] + "..."
	}
	return msg
}

// statusError turns an HTTP error status and message into one of the errors
// above where it can.
func statusError(code int, msg string) error {
	low := strings.ToLower(msg)
	switch {
	case code == 401 || code == 403:
		return fmt.Errorf("%w (%d)", ErrKey, code)
	case code == 402 || strings.Contains(low, "insufficient") || strings.Contains(low, "credit balance") ||
		strings.Contains(low, "quota") || strings.Contains(low, "billing"):
		return ErrFunds
	case code == 404 || (code == 400 && strings.Contains(low, "model")):
		return fmt.Errorf("%w: %s", ErrModel, msg)
	case code == 429 || code == 529 || code >= 500:
		return ErrBusy
	}
	if msg == "" {
		msg = http.StatusText(code)
	}
	return fmt.Errorf("error %d: %s", code, msg)
}

// newBackend makes the backend for provider p with key.
func newBackend(p *Provider, key string, hc *http.Client, baseURL string) backend {
	if baseURL == "" {
		baseURL = p.BaseURL
	}
	if p.wire == wireAnthropic {
		return newClaude(key, hc, baseURL)
	}
	return &chat{p: p, key: key, hc: hc, base: strings.TrimSuffix(baseURL, "/")}
}

// sendJSON sends a request with a JSON body, if there is one, and decodes
// the JSON reply into out. An error status becomes one of the errors above
// where it can.
func sendJSON(ctx context.Context, hc *http.Client, method, url string, header http.Header, body, out any) error {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		// Both APIs describe errors as {"error": {"message": ...}}.
		var e struct {
			Error struct{ Message string }
		}
		msg := strings.TrimSpace(string(data))
		if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		return statusError(resp.StatusCode, msg)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("unexpected reply: %w", err)
	}
	return nil
}

// claude speaks the Claude Messages API over plain HTTP, which keeps the
// game small.
type claude struct {
	key  string
	hc   *http.Client
	base string
}

func newClaude(key string, hc *http.Client, baseURL string) *claude {
	return &claude{key: key, hc: hc, base: strings.TrimSuffix(baseURL, "/")}
}

func (b *claude) header() http.Header {
	h := http.Header{}
	h.Set("x-api-key", b.key)
	h.Set("anthropic-version", "2023-06-01")
	// Lets the web version of the game call the API from the browser.
	h.Set("anthropic-dangerous-direct-browser-access", "true")
	return h
}

func (b *claude) complete(ctx context.Context, r Request) (Reply, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	body := map[string]any{
		"model":      r.Model,
		"max_tokens": r.MaxTokens,
		"messages":   []msg{{"user", r.Prompt}},
	}
	if r.System != "" {
		body["system"] = r.System
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := sendJSON(ctx, b.hc, http.MethodPost, b.base+"/v1/messages", b.header(), body, &out); err != nil {
		return Reply{}, err
	}
	rep := Reply{In: out.Usage.InputTokens, Out: out.Usage.OutputTokens}
	if out.StopReason == "refusal" {
		return rep, ErrRefused
	}
	for _, c := range out.Content {
		if c.Type == "text" {
			rep.Text += c.Text
		}
	}
	return rep, nil
}

func (b *claude) models(ctx context.Context) ([]string, error) {
	var ids []string
	after := ""
	for page := 0; page < 10; page++ {
		url := b.base + "/v1/models?limit=1000"
		if after != "" {
			url += "&after_id=" + after
		}
		var out struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := sendJSON(ctx, b.hc, http.MethodGet, url, b.header(), nil, &out); err != nil {
			return nil, err
		}
		for _, m := range out.Data {
			ids = append(ids, m.ID)
		}
		if !out.HasMore || out.LastID == "" {
			break
		}
		after = out.LastID
	}
	return ids, nil
}

func (b *claude) balance(context.Context) (Balance, error) {
	return Balance{}, errors.ErrUnsupported
}

// chat speaks OpenAI-style chat completions, which OpenAI, Meta's Model API
// and DeepSeek all offer.
type chat struct {
	p    *Provider
	key  string
	hc   *http.Client
	base string
}

// do sends a request to path.
func (b *chat) do(ctx context.Context, method, path string, body, out any) error {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+b.key)
	return sendJSON(ctx, b.hc, method, b.base+path, h, body, out)
}

func (b *chat) complete(ctx context.Context, r Request) (Reply, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	body := map[string]any{
		"model":    r.Model,
		"messages": []msg{{"system", r.System}, {"user", r.Prompt}},
	}
	if r.System == "" {
		body["messages"] = []msg{{"user", r.Prompt}}
	}
	body[b.p.maxField] = r.MaxTokens
	var out struct {
		Choices []struct {
			Message struct {
				Content any    `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := b.do(ctx, http.MethodPost, "/chat/completions", body, &out); err != nil {
		return Reply{}, err
	}
	rep := Reply{In: out.Usage.PromptTokens, Out: out.Usage.CompletionTokens}
	if len(out.Choices) == 0 {
		return rep, ErrEmpty
	}
	c := out.Choices[0]
	if c.Message.Refusal != "" || c.FinishReason == "content_filter" {
		return rep, ErrRefused
	}
	switch v := c.Message.Content.(type) {
	case string:
		rep.Text = v
	case []any: // content parts
		for _, part := range v {
			if m, ok := part.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					rep.Text += t
				}
			}
		}
	}
	return rep, nil
}

func (b *chat) models(ctx context.Context) ([]string, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := b.do(ctx, http.MethodGet, "/models", nil, &out); err != nil {
		return nil, err
	}
	var ids []string
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

func (b *chat) balance(ctx context.Context) (Balance, error) {
	if !b.p.Balance {
		return Balance{}, errors.ErrUnsupported
	}
	var out struct {
		IsAvailable  bool `json:"is_available"`
		BalanceInfos []struct {
			Currency     string `json:"currency"`
			TotalBalance string `json:"total_balance"`
		} `json:"balance_infos"`
	}
	if err := b.do(ctx, http.MethodGet, "/user/balance", nil, &out); err != nil {
		return Balance{}, err
	}
	bal := Balance{Usable: out.IsAvailable, Currency: "USD"}
	for i, info := range out.BalanceInfos {
		amount, err := strconv.ParseFloat(info.TotalBalance, 64)
		if err != nil {
			continue
		}
		// Prefer dollars; otherwise take the first currency listed.
		if info.Currency == "USD" || i == 0 {
			bal.Amount, bal.Currency = amount, info.Currency
		}
		if info.Currency == "USD" {
			break
		}
	}
	return bal, nil
}

// httpClient is the client for real requests. Game content is small, so a
// minute is plenty.
func httpClient() *http.Client { return &http.Client{Timeout: 60 * time.Second} }
