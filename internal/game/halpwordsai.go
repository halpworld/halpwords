package game

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/llm"
	"github.com/halpworld/halpwords/internal/save"
)

// loadAI reads the AI settings and connects the AI to Halpwords AI
// through the link.
func (c *Context) loadAI() *llm.Service {
	ai := llm.Load(save.Root)
	ai.UseHalpwords(halpwordsAI{c})
	return ai
}

// halpwordsAI is Halpwords AI through the game's link to the server.
type halpwordsAI struct{ c *Context }

func (h halpwordsAI) Available() bool { return h.c.Link.AIAvailable() }

func (h halpwordsAI) Do(ctx context.Context, task string, req, out any) error {
	return aiError(h.c.Link.AskAI(ctx, task, req, out))
}

// aiError is an error from Halpwords AI as the AI service's error, so the
// game waits, stops or explains as it does for a provider's.
func aiError(err error) error {
	var e *link.Error
	switch {
	case err == nil:
		return nil
	case errors.Is(err, link.ErrNoAI), errors.Is(err, link.ErrNotLinked), errors.Is(err, link.ErrUnlinked):
		return fmt.Errorf("%w: %w", llm.ErrNoHalpwords, err)
	case !errors.As(err, &e):
		return fmt.Errorf("%w: %w", llm.ErrBusy, err) // no answer: offline, say
	case e.Code == link.CodeAIOff:
		return fmt.Errorf("%w: %w", llm.ErrNoHalpwords, err)
	case e.Code == link.CodeAIBudget:
		return fmt.Errorf("%w: %w", llm.ErrAllowance, err)
	case e.Code == link.CodeAINoOutput, e.Code == link.CodeNoWords:
		return fmt.Errorf("%w: %w", llm.ErrEmpty, err)
	case e.Code == link.CodeAIBusy, e.Status == http.StatusTooManyRequests, e.Status >= 500:
		return fmt.Errorf("%w: %w", llm.ErrBusy, err)
	}
	return err
}
