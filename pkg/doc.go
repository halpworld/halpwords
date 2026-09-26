// Package pkg holds nothing itself. Its subpackages are the game's public
// API, shared with halpwords-server:
//
//   - words: the word list format, languages, grading, word memory,
//     mistake kinds, what the Grimoire shows and the Greek typing keys
//   - compete: Hardcore seeds, share codes and scores
//   - puzzle: the door and chest word puzzles
//   - proc: procedural pixel art
//   - safety: the family-safe policy and filter for generated text
//   - settings: a player's settings for one language, which a grown-up
//     can lock
//   - maps: hand-made maps and quests, and the checks they must pass
//   - race: the rules of a Race, and the checks the server runs on
//     racers' reports
//   - raid: the rules of a Boss Raid (health, damage, dodges, grading)
//     and the boss's sprite
//   - gameai: the prompts, replies and checks of the game's AI content
//     (floor scripts, gap-fill sentences, riddles, taunts, memory tips),
//     which halpwords-server's Halpwords AI uses too
//
// None of them may import Ebitengine or the game's screens, so a server
// can build them without graphics code (see imports_test.go). Changes to
// them need a note in CHANGELOG.md.
package pkg
