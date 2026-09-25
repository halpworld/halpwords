package llm

import (
	"sort"
	"strings"
	"time"
)

// Model is a model the game knows the price of.
type Model struct {
	Provider ProviderID
	ID       string
	Name     string
	// In and Out are US dollars per million input and output tokens.
	In, Out float64
	// PeakIn and PeakOut, when set, are the prices in the provider's busy
	// hours (see peak).
	PeakIn, PeakOut float64
	// About is a few words for the model picker.
	About string
}

// Catalog lists the models the game knows, with their prices in September
// 2026. Providers change prices, so the spending shown is an estimate; the
// provider's own dashboard has the exact bill. Models a provider offers that
// are not here can still be chosen: they are counted at the provider's
// highest price here, so the budget is never overrun.
var Catalog = []Model{
	{Anthropic, "claude-haiku-4-5", "Claude Haiku 4.5", 1, 5, 0, 0, "fast and cheap"},
	{Anthropic, "claude-sonnet-5", "Claude Sonnet 5", 2, 10, 0, 0, "smart, good value"},
	{Anthropic, "claude-sonnet-4-6", "Claude Sonnet 4.6", 3, 15, 0, 0, "older Sonnet"},
	{Anthropic, "claude-opus-5-5", "Claude Opus 5.5", 4, 20, 0, 0, "very smart"},
	{Anthropic, "claude-opus-5", "Claude Opus 5", 5, 25, 0, 0, "very smart"},
	{Anthropic, "claude-fable-5-1", "Claude Fable 5.1", 10, 50, 0, 0, "most capable, costly"},

	{OpenAI, "gpt-6-luna", "GPT-6 Luna", 0.10, 0.50, 0, 0, "fast and cheap"},
	{OpenAI, "gpt-5-nano", "GPT-5 nano", 0.05, 0.40, 0, 0, "cheapest"},
	{OpenAI, "gpt-5.6-luna", "GPT-5.6 Luna", 0.20, 1.20, 0, 0, "fast and cheap"},
	{OpenAI, "gpt-5-mini", "GPT-5 mini", 0.25, 2, 0, 0, "cheap"},
	{OpenAI, "gpt-6-sol", "GPT-6 Sol", 2, 10, 0, 0, "smart, good value"},
	{OpenAI, "gpt-5.6-terra", "GPT-5.6 Terra", 2, 12, 0, 0, "smart"},
	{OpenAI, "gpt-6-astra", "GPT-6 Astra", 10, 50, 0, 0, "most capable, costly"},

	{Meta, "muse-spark-1.3", "Muse Spark 1.3", 1.25, 4.25, 0, 0, "Meta's newest"},
	{Meta, "muse-spark-1.2", "Muse Spark 1.2", 1.25, 4.25, 0, 0, "older"},
	{Meta, "muse-spark-1.3-contributor", "Muse Spark 1.3 (contributor)", 0.10, 0.20, 0, 0, "cheap: Meta trains on the prompts"},

	{DeepSeek, "deepseek-flash", "DeepSeek V4.1 Flash", 0.15, 0.60, 0.30, 1.20, "fast and very cheap"},
	{DeepSeek, "deepseek-v4-pro", "DeepSeek V4 Pro", 0.66, 1.98, 1.32, 3.96, "smart, cheap"},
}

// Known returns the catalog entry for a model.
func Known(p ProviderID, id string) (Model, bool) {
	for _, m := range Catalog {
		if m.Provider == p && m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// ModelsFor returns the catalog models of a provider, cheapest first.
func ModelsFor(p ProviderID) []Model {
	var out []Model
	for _, m := range Catalog {
		if m.Provider == p {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Out < out[j].Out })
	return out
}

// peak reports whether t is in DeepSeek's busy hours, when its prices
// double: 01:00-04:00 and 06:00-10:00 UTC on weekdays.
func peak(t time.Time) bool {
	t = t.UTC()
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false
	}
	h := t.Hour()
	return (h >= 1 && h < 4) || (h >= 6 && h < 10)
}

// Rate returns the price of a model at time t, in dollars per million input
// and output tokens. For a model the catalog does not know, it returns the
// provider's highest known price and exact is false.
func Rate(p ProviderID, id string, t time.Time) (in, out float64, exact bool) {
	m, ok := Known(p, id)
	if !ok {
		for _, c := range ModelsFor(p) {
			in, out = max(in, c.In, c.PeakIn), max(out, c.Out, c.PeakOut)
		}
		if out == 0 {
			in, out = 10, 50 // a provider with no known models: be careful
		}
		return in, out, false
	}
	if m.PeakOut > 0 && peak(t) {
		return m.PeakIn, m.PeakOut, true
	}
	return m.In, m.Out, true
}

// Cost is what in input and out output tokens cost, in dollars.
func Cost(p ProviderID, id string, in, out int64, t time.Time) (usd float64, exact bool) {
	ri, ro, exact := Rate(p, id, t)
	return (float64(in)*ri + float64(out)*ro) / 1e6, exact
}

// chatModel reports whether a model listed by a provider can write text, so
// the picker can leave out image, voice and embedding models.
func chatModel(p ProviderID, id string) bool {
	id = strings.ToLower(id)
	for _, bad := range []string{"embed", "tts", "whisper", "transcribe", "audio", "realtime",
		"image", "dall-e", "moderation", "search", "davinci", "babbage", "voice", "sam-", "sora", "codex"} {
		if strings.Contains(id, bad) {
			return false
		}
	}
	switch p {
	case OpenAI:
		return strings.HasPrefix(id, "gpt-") || strings.HasPrefix(id, "chatgpt-") ||
			(len(id) > 1 && id[0] == 'o' && id[1] >= '0' && id[1] <= '9')
	case Anthropic:
		return strings.HasPrefix(id, "claude-")
	}
	return true
}
