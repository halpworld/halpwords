package llm

import (
	"context"
	"errors"

	"github.com/halpworld/halpwords/pkg/gameai"
	"github.com/halpworld/halpwords/pkg/safety"
)

// Floor is what the Dungeon Director is told about the next floor.
type Floor = gameai.Floor

// Script is the Dungeon Director's plan for a floor: names and words that
// dress the procedural floor. Only what passed the checks is set.
type Script = gameai.Script

// Direct asks the Dungeon Director for a floor script.
func (s *Service) Direct(ctx context.Context, f Floor) (*Script, error) {
	var out gameai.ScriptReply
	if h := s.halpwordsAI(); h != nil {
		if err := s.askHalpwords(ctx, h, gameai.Director, directorRequest(f), &out); err != nil {
			return nil, err
		}
	} else {
		text, err := s.Ask(ctx, false, safety.Policy, gameai.DirectorPrompt(f), gameai.DirectorTokens)
		if err != nil {
			return nil, err
		}
		if err := decodeJSON(text, &out); err != nil {
			return nil, err
		}
	}
	sc, err := gameai.CheckScript(out, f)
	if errors.Is(err, gameai.ErrNoName) {
		return nil, ErrEmpty
	}
	return sc, err
}

// CheckScript checks and tidies a floor script. A script needs a good name;
// the other parts are dropped if they fail the checks.
func CheckScript(name string, theme *int, intro string, lore []string, monsters map[string]string, boss string, f Floor) (*Script, error) {
	sc, err := gameai.CheckScript(gameai.ScriptReply{Name: name, Theme: theme, Intro: intro, Lore: lore, Monsters: monsters, Boss: boss}, f)
	if err != nil {
		return nil, ErrEmpty
	}
	return sc, nil
}
