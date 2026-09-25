// Package pkg holds nothing itself. Its subpackages are the game's public
// API, shared with halpwords-server:
//
//   - words: the word list format, languages, grading, word memory and
//     mistake kinds
//   - compete: Hardcore seeds, share codes and scores
//   - puzzle: the door and chest word puzzles
//   - proc: procedural pixel art
//   - safety: the family-safe policy and filter for generated text
//
// None of them may import Ebitengine or the game's screens, so a server
// can build them without graphics code (see imports_test.go). Changes to
// them need a note in CHANGELOG.md.
package pkg
