// Package llm connects the game to a large language model, when a parent or
// teacher has set one up: Anthropic (Claude), OpenAI, Meta or DeepSeek. It
// keeps the API key, counts what is spent against a budget, and makes the
// dungeon's generated content: floor scripts, cloze sentences, riddles,
// monster taunts, memory tips and new word lists. Everything it makes is
// checked before the game uses it, and the game always has a fallback, so
// the model is never needed to play. It has no Ebitengine dependency.
package llm

import "strings"

// ProviderID names a model provider. The empty ID means none: the game plays
// without a model.
type ProviderID string

const (
	Off       ProviderID = ""
	Anthropic ProviderID = "anthropic"
	OpenAI    ProviderID = "openai"
	Meta      ProviderID = "meta"
	DeepSeek  ProviderID = "deepseek"
)

// wire is how a provider's API is spoken.
type wire int

const (
	wireAnthropic wire = iota // the Claude Messages API
	wireChat                  // OpenAI-style chat completions
)

// Provider describes one model provider.
type Provider struct {
	ID ProviderID
	// Name is shown in the setup screen.
	Name string
	// BaseURL is where the API lives.
	BaseURL string
	// EnvVars are environment variables a key can be read from.
	EnvVars []string
	// KeyPage is where a parent or teacher makes a key.
	KeyPage string
	// KeyHint is how keys start, such as "sk-ant-", to spot a key pasted for
	// the wrong provider. Empty when keys have no fixed start.
	KeyHint string
	// GameModel is the default model for content made during play: it should
	// be fast and cheap. ForgeModel is the default for making word lists,
	// where quality matters more.
	GameModel, ForgeModel string
	// Balance is set when the provider can say how much money is left on
	// the account.
	Balance bool

	wire wire
	// maxField is the name chat completions give the output limit.
	maxField string
	// headroom multiplies the output limit a request asks for, to leave
	// room for models that think before they answer: their thinking counts
	// against the limit.
	headroom int
}

// Providers lists the providers in the order the setup screen offers them.
var Providers = []*Provider{
	{
		ID:         Anthropic,
		Name:       "Anthropic (Claude)",
		BaseURL:    "https://api.anthropic.com",
		EnvVars:    []string{"ANTHROPIC_API_KEY"},
		KeyPage:    "platform.claude.com/settings/keys",
		KeyHint:    "sk-ant-",
		GameModel:  "claude-haiku-4-5",
		ForgeModel: "claude-sonnet-5",
		wire:       wireAnthropic,
		headroom:   3,
	},
	{
		ID:         OpenAI,
		Name:       "OpenAI (GPT)",
		BaseURL:    "https://api.openai.com/v1",
		EnvVars:    []string{"OPENAI_API_KEY"},
		KeyPage:    "platform.openai.com/api-keys",
		KeyHint:    "sk-",
		GameModel:  "gpt-6-luna",
		ForgeModel: "gpt-6-sol",
		wire:       wireChat,
		maxField:   "max_completion_tokens",
		headroom:   4,
	},
	{
		ID:         Meta,
		Name:       "Meta (Muse Spark)",
		BaseURL:    "https://api.meta.ai/v1",
		EnvVars:    []string{"MODEL_API_KEY", "META_API_KEY"},
		KeyPage:    "dev.meta.ai (Model API → API keys)",
		GameModel:  "muse-spark-1.3",
		ForgeModel: "muse-spark-1.3",
		wire:       wireChat,
		maxField:   "max_tokens",
		headroom:   2,
	},
	{
		ID:         DeepSeek,
		Name:       "DeepSeek",
		BaseURL:    "https://api.deepseek.com",
		EnvVars:    []string{"DEEPSEEK_API_KEY"},
		KeyPage:    "platform.deepseek.com/api_keys",
		KeyHint:    "sk-",
		GameModel:  "deepseek-flash",
		ForgeModel: "deepseek-v4-pro",
		Balance:    true,
		wire:       wireChat,
		maxField:   "max_tokens",
		headroom:   2,
	},
}

// ProviderFor returns the provider with id, or nil.
func ProviderFor(id ProviderID) *Provider {
	for _, p := range Providers {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// LooksWrong reports whether key looks like a key for another provider, so
// the setup screen can warn about it.
func (p *Provider) LooksWrong(key string) bool {
	if strings.HasPrefix(key, "sk-ant-") {
		return p.ID != Anthropic
	}
	return p.KeyHint != "" && !strings.HasPrefix(key, p.KeyHint)
}

// Mask shows the start and end of a key, so a parent can tell which key is
// set without it being on screen for anyone to copy.
func Mask(key string) string {
	r := []rune(key)
	if len(r) <= 12 {
		return strings.Repeat("•", len(r))
	}
	head := 3
	if strings.HasPrefix(key, "sk-ant-") {
		head = 7
	}
	return string(r[:head]) + "••••" + string(r[len(r)-4:])
}
