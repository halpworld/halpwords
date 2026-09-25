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

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
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

// claude speaks the Claude Messages API through Anthropic's Go SDK.
type claude struct {
	c anthropic.Client
}

func newClaude(key string, hc *http.Client, baseURL string) *claude {
	return &claude{c: anthropic.NewClient(
		option.WithoutEnvironmentDefaults(), // the key is the one set in the game
		option.WithAPIKey(key),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(hc),
		option.WithMaxRetries(1),
		// Lets the web version of the game call the API from the browser.
		option.WithHeader("anthropic-dangerous-direct-browser-access", "true"),
	)}
}

func claudeError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		msg := apiErr.RawJSON()
		var body struct {
			Error struct{ Message string }
		}
		if json.Unmarshal([]byte(msg), &body) == nil && body.Error.Message != "" {
			msg = body.Error.Message
		}
		return statusError(apiErr.StatusCode, msg)
	}
	return err
}

func (b *claude) complete(ctx context.Context, r Request) (Reply, error) {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(r.Model),
		MaxTokens: int64(r.MaxTokens),
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(r.Prompt))},
	}
	if r.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: r.System}}
	}
	msg, err := b.c.Messages.New(ctx, params)
	if err != nil {
		return Reply{}, claudeError(err)
	}
	rep := Reply{In: msg.Usage.InputTokens, Out: msg.Usage.OutputTokens}
	if msg.StopReason == anthropic.StopReasonRefusal {
		return rep, ErrRefused
	}
	var sb strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	rep.Text = sb.String()
	return rep, nil
}

func (b *claude) models(ctx context.Context) ([]string, error) {
	var ids []string
	pager := b.c.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	for pager.Next() {
		ids = append(ids, pager.Current().ID)
	}
	if err := pager.Err(); err != nil {
		return nil, claudeError(err)
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

// do sends a request and decodes a JSON reply into out.
func (b *chat) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error struct {
				Message string
				Code    any
			}
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
