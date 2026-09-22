# Halpwords

An 8-bit dungeon-crawler RPG for learning to spell words in a foreign language:
French, Latin, Ancient Greek and Irish. You fight monsters by typing
translations quickly and correctly.

See [PLAN.md](PLAN.md) for the full design and roadmap.

> **Status: milestone 0 (skeleton).** It has a title screen and a spelling
> practice mode with grading, the accent helper, and the Greek typing mode. The
> dungeon, battles and puzzles come in later milestones.

## Running on a Mac (Apple Silicon)

1. Install Go 1.26 or newer: `brew install go` (or download it from
   <https://go.dev/dl/>).
2. In this folder, run:
   ```sh
   make run          # or: go run ./cmd/halpwords
   ```
3. To make a double-clickable app: `make bundle-mac`, then open
   `dist/Halpwords.app`.

## Controls

| Key | Action |
|---|---|
| ↑ / ↓, Enter | Menus |
| Type + Enter | Answer |
| Tab | Cycle the accent on the last letter (e → é → è → ê → ë, a → ā, a → á) |
| ← / → | Change language (practice mode) |
| F2 | Greek letters on/off (Ancient Greek) |
| F11, Alt+Enter, or Ctrl+Cmd+F | Fullscreen |
| Esc | Back |

**Typing Greek:** with Greek letters on, Latin keys type Greek letters
(a→α, b→β, g→γ, d→δ, e→ε, z→ζ, h→η, q→θ, i→ι, k→κ, l→λ, m→μ, n→ν, c→ξ, o→ο,
p→π, r→ρ, s→σ/ς, t→τ, u→υ, f→φ, x→χ, y→ψ, w→ω). You can add marks after a
vowel: `)` smooth breathing, `(` rough breathing, `/` acute, `\` grave,
`=` circumflex, `|` iota subscript, `+` diaeresis. So `a)/nqrwpos` types
ἄνθρωπος. By default accents and breathings are optional.

## Your own word lists

Put `.txt` files in the game's `words` folder:

- macOS: `~/Library/Application Support/halpwords/words/`
- Windows: `%AppData%\halpwords\words\`
- Linux: `~/.config/halpwords/words/`

```
# Lines starting with # are comments.
title: French - Animals
language: fr

## animals
dog = le chien
friend = l'ami | l'amie
```

- Put one word per line: `english = answer`. Extra accepted answers go after
  `|`.
- The language codes are `fr` (French), `la` (Latin), `grc` (Ancient Greek)
  and `ga` (Irish).
- `## name` starts a group of related words.

See [`assets/words/`](assets/words) for the built-in lists.

## Building for other platforms

```sh
make build-mac       # Apple Silicon (from any OS)
make build-windows   # Windows .exe (from any OS)
make build-web       # WebAssembly, in dist/web
make build-linux     # Linux (run on Linux; needs X11/GL dev packages)
make help            # all targets
```

GitHub Actions builds every platform on each push (see `.github/workflows/ci.yml`).

## Project layout

```
cmd/halpwords/     entry point
internal/game/     main loop, scene stack, pixel-perfect scaling
internal/scene/    screens (title, practice, ...)
internal/words/    word lists, languages, answer grading
internal/typing/   text entry, Tab accents, Greek input mode
internal/combat/   battle formulas
internal/proc/     procedural pixel art (no Ebitengine dependency)
internal/gfx/      drawing: text, windows, torches
internal/pal/      the 32-colour palette
internal/unifont/  bitmap font parser
assets/            embedded font and starter word lists
tools/fontsubset/  regenerates the font subset from GNU Unifont
```

## Licences

- Code: Apache 2.0 (see [LICENSE](LICENSE)).
- Font: GNU Unifont, used under the SIL Open Font License 1.1 (see
  [assets/fonts/OFL-1.1.txt](assets/fonts/OFL-1.1.txt)).
