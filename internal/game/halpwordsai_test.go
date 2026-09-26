package game

import (
	"errors"
	"testing"

	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/llm"
)

func TestAIError(t *testing.T) {
	for _, c := range []struct {
		err  error
		want error
	}{
		{link.ErrNoAI, llm.ErrNoHalpwords},
		{link.ErrNotLinked, llm.ErrNoHalpwords},
		{&link.Error{Status: 403, Code: link.CodeAIOff}, llm.ErrNoHalpwords},
		{&link.Error{Status: 429, Code: link.CodeAIBudget}, llm.ErrAllowance},
		{&link.Error{Status: 422, Code: link.CodeAINoOutput}, llm.ErrEmpty},
		{&link.Error{Status: 422, Code: link.CodeNoWords}, llm.ErrEmpty},
		{&link.Error{Status: 503, Code: link.CodeAIBusy}, llm.ErrBusy},
		{&link.Error{Status: 429, Code: "rate_limited"}, llm.ErrBusy},
		{errors.New("no network"), llm.ErrBusy},
	} {
		if got := aiError(c.err); !errors.Is(got, c.want) {
			t.Errorf("aiError(%v) = %v; want %v", c.err, got, c.want)
		}
	}
	if aiError(nil) != nil {
		t.Error("aiError(nil)")
	}
}
