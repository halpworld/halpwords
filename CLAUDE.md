# Working on Halpwords

Halpwords is an 8-bit dungeon crawler for learning to spell words in
French, Latin, Ancient Greek and Irish, for players around 13. It is Go and
Ebitengine, one binary for macOS, Windows, Linux and the web.

The optional server, [halpwords-server](https://github.com/halpworld/halpwords-server),
is a separate repo. Its agent docs, `docs/agents/` in that repo, have a map
of where things are in both repos, playbooks, a review checklist and a
glossary.

**Before any task:** read [PLAN.md](PLAN.md) §1 (design pillars), §3
(architecture), §12 (testing), §13 (milestones), and the sections the task
touches. The usual ones are §4 (word lists, matching, spaced repetition),
§5 to §8 (dungeon, combat, puzzles, RPG and game modes), §10 (the LLM) and
§10a (the link to a grown-up's account). §14 has the decisions already
made. For floors and worlds, read [docs/worlds.md](docs/worlds.md); for
releases, [docs/RELEASING.md](docs/RELEASING.md).

## Rules

- **The game always works offline**, without the server and without an
  AI. Every LLM feature has a fixed or procedural fallback. Network calls
  run in the background with timeouts and never block the game loop.
- **Children's privacy and safety come first.** Collect nothing the game
  doesn't need. No personal data in logs or crash reports. API keys and
  link tokens go in private files (`save.WritePrivate`), never in saves.
  Nothing is sent to a server unless an adult linked the game.
- **Family-safe content only.** Word lists, riddles, taunts and anything an
  LLM writes are for 13-year-olds: never gory, never rude. LLM output is
  checked against a schema and the local word filter; this can't be
  turned off.
- **The LLM never grades spelling.** Grading is always local
  (`words.Grade`).
- **`pkg/` and `assets` are a contract with the server.** halpwords-server
  imports eleven `pkg/` packages (`audiopack`, `compete`, `gameai`,
  `maps`, `proc`, `puzzle`, `race`, `raid`, `safety`, `settings`,
  `words`) and `assets` (the starter lists in `assets.Words` and the
  font in `assets.UnifontHex`). Check the current list in the server
  checkout with
  `grep -rhoE 'halpwords/(pkg/[a-z]+|assets)"' --include='*.go' . | sort -u`.
  Changes there must stay compatible with older lists, older games and
  the server: run `halpwords:contract-keeper` on any change under `pkg/`
  or `assets/`, to the word list format or to `internal/link`. Nothing under `pkg/` may import Ebitengine
  or the game's screens (`pkg/imports_test.go` checks this).
- **Tests sit next to the code** (`foo.go`, `foo_test.go`). Pure logic
  goes in packages with no Ebitengine dependency, so it can be tested.
- **No real AI calls in tests.** `internal/llm` tests use an
  `httptest` server and recorded replies. Never put a real API key in a
  test, a fixture or a commit.
- **Never touch the player's own saves** in
  `~/Library/Application Support/halpwords` (`os.UserConfigDir()`). Tests
  use a temporary `HOME` (see `useTempDir` in `internal/save/save_test.go`).
  To run the game yourself, give it a throwaway folder (below).
- **Don't send test data to the real server.** The link and reports go to
  `https://halpwords.com` unless `HALPWORDS_SERVER` says otherwise. Point
  it at a local server (`make run` in halpwords-server) when you try them.
- **Ask before adding a dependency.** Prefer the standard library.
- **Keep the plan true.** When a milestone or fix lands, add a short
  *Now (Mx):* note to the PLAN.md section it changed, with what was built
  and any numbers that were tuned. New owner decisions go in §14.

## Working with branches

- Start from `origin/main` in your own worktree:
  `git fetch -q origin && git worktree add .claude/worktrees/<name> -b claude/<name> origin/main`.
  Local `main` may be behind; never work in the main checkout.
- Changes reach `main` only through pull requests. Never push to `main`.
  The owner (or the lead agent, when the owner allows it) merges with
  `gh pr merge <N> --merge`.
- Before saying a change is done, run `make check` and say what it
  printed. CI also runs `go run ./tools/balance -runs 100 -floors 9` and
  the cross builds, so run `make balance` after any balance change.

## Commands

Every target is in the [Makefile](Makefile); `make help` lists them.

```sh
make check      # make vet + make test (CI also runs the balance check and the builds)
make vet        # go vet, staticcheck (pinned), gofmt
make test       # go test ./...
make run        # run the game (uses your real save folder; see below)
make build      # build for this computer into dist/halpwords
make balance    # play the dungeon with typing bots and print the tables
make sounds     # every sound effect and music loop as WAV files in dist/sounds
make icon       # the icon as PNGs, .icns and .ico in dist/icon
make build-web  # the WebAssembly build in dist/web
make serve-web  # build-web, then serve it at http://localhost:8000
make clean      # remove dist/
```

The `build-*`, `bundle-mac`, `build-moved` and `release-*` targets make
the platform builds and release archives; see
[docs/RELEASING.md](docs/RELEASING.md). `tools/worldsheet` (contact sheets
of every world) and `tools/fontsubset` (the embedded Unifont subset) have
no Make target; run them with `go run`.

To run the game without touching the player's saves, build it and give it
a throwaway home folder, with no AI keys and no server:

```sh
make build
env -u ANTHROPIC_API_KEY -u OPENAI_API_KEY -u MODEL_API_KEY -u META_API_KEY \
  -u DEEPSEEK_API_KEY -u HALPWORDS_SERVER HOME="$(mktemp -d)" dist/halpwords
```

Other settings the game reads: `HALPWORDS_SEED` (a fixed dungeon seed),
`HALPWORDS_SOUND=off` and `HALPWORDS_SERVER`.

## Sub-agents

The `halpwords` plugin in [plugins/halpwords](plugins/halpwords) (switched
on in `.claude/settings.json`) has agents for this work. Use them as
[plugins/halpwords/README.md](plugins/halpwords/README.md) describes:

- `halpwords:contract-keeper`: anything under `pkg/`, the word list
  format, `/api/v1` and `internal/link`.
- `halpwords:privacy-safety-reviewer`: the link, reports, crash reports,
  saves and LLM code.
- `halpwords:language-reviewer`: `assets/words`, `assets/puzzles` and
  answer matching.
- `halpwords:llm-engineer`: prompts, schemas, fixtures and fallbacks in
  `internal/llm` and `pkg/gameai`.
- `halpwords:balance-analyst`: formulas, stats, scores, word difficulty.
- `halpwords:ui-reviewer`: any screen or the web build.
- `halpwords:plan-keeper`: PLAN.md notes and milestones.
- `halpwords:docs-keeper`: docs in both repos after a change.
- `halpwords:test-server-operator`: the Docker test server.

A QA team always includes a logic and consistency auditor, as well as the
specialists.

## Style

Plain Go, standard library first. Short doc comments in full sentences.
British spelling in everything a player reads. Pixel screens are 640x360.
Commit messages start with the area (`scene: …`, `link: …`) or a plain
summary, in the present tense.
