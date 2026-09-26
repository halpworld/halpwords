package llm

import (
	"errors"

	"github.com/halpworld/halpwords/pkg/gameai"
	"github.com/halpworld/halpwords/pkg/words"
)

// decodeJSON reads the first JSON value in a model's reply into v. Models
// sometimes wrap JSON in a code fence or add a sentence around it.
func decodeJSON(text string, v any) error {
	if err := gameai.DecodeJSON(text, v); err != nil {
		return errors.Join(ErrEmpty, err)
	}
	return nil
}

// The text helpers the game's AI content shares with the server
// (pkg/gameai).
var (
	short     = gameai.Short
	tidy      = gameai.Tidy
	nameLike  = gameai.NameLike
	langLine  = gameai.LangLine
	wordLines = gameai.WordLines
)

func hasScript(s string, lang *words.Language) bool { return gameai.HasScript(s, lang) }
