# Halpwords: Plan

An 8-bit dungeon-crawler RPG for learning to spell words in a foreign language.
You explore a randomly generated dungeon. Monsters are fought by **typing
translations**, doors and treasure chests are opened by **solving word
puzzles**, and an optional LLM connection makes the dungeon react to how you
are learning.

**Audience:** secondary-school students, around 13 years old. The tone is
friendly and adventurous, never gory, and all generated content is
family-safe.

**Languages (all English → target):**
- French
- Latin
- Ancient Greek (polytonic)
- Irish

The UI is in English.

---

## 1. Design pillars

1. **Spelling is the weapon.** Every action that matters (attack, dodge, unlock)
   comes down to typing a word correctly and quickly.
2. **Learn, don't grind.** Spaced repetition chooses which words show up, so the
   words you keep getting wrong come back until you know them.
3. **Almost no artwork.** Tiles, monsters, items, effects, sound and music are
   generated in code. The only required external asset is a pixel font. Any
   AI-generated art is an optional override, never a dependency.
4. **Fully playable offline.** The LLM makes the game better but is never
   needed. Every LLM feature has a fixed or procedural fallback.
5. **One codebase, many platforms.** Build natively for Apple Silicon first, and
   cross-compile to Windows, Linux and the web without rewrites.

---

## 2. Tech stack

**Chosen: Go + [Ebitengine](https://ebitengine.org) (v2).**

| Need | Why Go + Ebitengine fits |
|---|---|
| Arm Mac | Native arm64 Metal backend. `go build` and it runs. |
| Cross-compile | Windows builds from a Mac need **no cgo** (`GOOS=windows go build`). Linux builds in CI or Docker. Web via WASM. Mobile later via `ebitenmobile`. |
| 8-bit rendering | Built for 2D pixel games: offscreen low-res buffer, nearest-neighbour scaling, runtime-generated images (`ebiten.NewImage` + `WritePixels`). |
| Typed input | `ebiten.AppendInputChars` delivers real Unicode characters, so macOS accent input (Option+e, e → é) works. `exp/textinput` supports IME for future non-Latin languages. |
| Procedural audio | `audio` package takes raw PCM streams, so the chiptune synth is written in Go and needs no audio files. |
| LLM calls | `net/http` plus goroutines: requests run in the background and never stall the game loop. |
| Distribution | Single static binary; assets embedded with `go:embed`. |

Alternatives considered: **Rust + macroquad** (also good, but slower to iterate
and cross-compiling to Windows needs a toolchain), **Godot** (editor-centric and
less suited to generated content), **Love2D** (weak HTTPS and packaging story),
**Python/pygame** (hard to distribute).

---

## 3. Architecture

```
halpwords/
├── cmd/halpwords/main.go        # entry point, window setup
├── internal/
│   ├── game/                    # top-level Game, scene stack, fixed-timestep loop
│   ├── scene/                   # title, charcreate, explore, battle, puzzle,
│   │                            # inventory, shop, campfire, gameover, settings
│   ├── dungeon/                 # generation, FOV/shadowcasting, pathfinding, fog of war
│   ├── gfx/                     # palette, procedural tiles, sprite generator,
│   │                            # text rendering, particles, screen shake, lighting
│   ├── audio/                   # sfxr-style synth, procedural music sequencer
│   ├── words/                   # word packs, answer matching, Unicode normalisation, SRS
│   ├── combat/                  # damage/dodge formulas, monster traits, encounter builder
│   ├── puzzle/                  # puzzle interface, generators, fixed puzzle bank
│   ├── rpg/                     # stats, levelling, items, equipment, loot tables
│   ├── llm/                     # provider interface, Anthropic + OpenAI-compatible,
│   │                            # prompt templates, JSON validation, cache, budget
│   └── save/                    # profiles, run saves, settings (JSON in user config dir)
├── assets/                      # embedded: font, word packs, puzzle bank, hero template
│   ├── fonts/                   # Unifont subset (.hex) + OFL licence
│   ├── words/                   # starter lists: french.txt, latin.txt, greek.txt, irish.txt
│   └── puzzles/                 # hand-written riddles/cloze templates per pack
└── overrides/                   # optional drop-in PNGs (AI art) that replace procedural sprites
```

Rules:
- **Game logic is pure Go with no Ebitengine imports** (`dungeon`, `words`,
  `combat`, `puzzle`, `rpg`, `llm`). That makes it unit-testable, deterministic
  from a seed, and portable.
- **Scenes are a stack** (explore → battle → back to explore; puzzle overlays).
- **Seeded RNG everywhere**, so a dungeon seed can reproduce a run for debugging
  and sharing.
- **Logical screen 640×360** (16:9), integer-scaled to the window.
  - The world uses 16×16 tiles drawn at 2×, so the art has a chunky 320×180
    pixel grid and the viewport is 20×11 tiles.
  - Text is drawn at 1× (8×16 Unifont glyphs), so Greek accents and breathings
    stay readable. The typing line uses 2× or 3× text.

---

## 4. Words: packs, matching, learning

### Word list format: plain text, one word per line
Teachers and students can write these in any text editor. Files end in `.txt`
and go in the `words/` folder in the game's user folder, where the game picks
them up without rebuilding.

```
# Lines starting with # are comments.
title: French - Animals
language: fr

## animals
dog = le chien
cat = le chat
horse = le cheval
bird = l'oiseau | un oiseau

## food
bread = le pain
```
- `english = answer`. Extra accepted answers go after `|`.
- `title:` and `language:` (`fr`, `la`, `grc`, `ga`) are the only header
  fields.
- `## name` starts a tag group, used by puzzles such as odd-one-out and by LLM
  themes.
- Difficulty is worked out automatically from word length, special characters
  and your history, so there are no extra columns.
- Answers are written in normal Unicode (é, ā, á, ἀ).
- The game ships with starter packs for each language (§4a). With an LLM
  connected, the **Word Forge** can generate more (see §10).

### Answer matching
- Normalise to Unicode NFC, trim, case-insensitive (`golang.org/x/text`).
- Grading tiers:
  - **Perfect**: exact match, no backspace used.
  - **Correct**: exact match, but corrected with backspace.
  - **Accent slip**: correct only after stripping diacritics (é vs e).
  - **Graze**: Levenshtein distance 1, for words of 4+ letters.
  - **Miss**: anything else, or the timer ran out.
- The mistake type is classified locally (accent, doubled letter, swapped
  letters, missing letter, wrong word) for feedback and statistics.
- **Accent helper:** after typing a letter, press `Tab` to cycle its variants
  (e → é → è → ê → ë, a → ā for Latin, a → á for Irish). This works the same on
  every OS and keyboard layout. Native OS accent input also works.
- **Configurable strictness**, set per language in Settings:

  | Setting | Options | Default |
  |---|---|---|
  | Accents / fadas / macrons | strict · reduced credit · ignore | French *reduced*, Irish *reduced*, Latin macrons *ignore*, Greek *ignore* |
  | Greek breathings (ἀ vs ἁ) | strict · reduced credit · ignore | *ignore* |
  | Articles (French *le/la/l'/les*) | required · optional | *optional* |
  | Capitals | strict · ignore | *ignore* |
  | Live typo highlighting | on · off | *on* |
  | Timer speed | relaxed · normal · fast | *normal* |

### 4a. Language-specific details
- **French:** é è ê ë à â ç î ï ô ù û ü œ.
  - With articles *optional*, `chien` and `le chien` are both accepted.
    With articles *required*, the article is checked too, which is good
    practice for gender.
  - Elision (`l'oiseau`) is handled, and typographic apostrophes (’) are treated
    the same as `'`.
- **Latin:** answers use the dictionary headword (nominative singular for
  nouns, 1st person present for verbs, following common school courses).
  Macrons (ā ē ī ō ū) are optional by default; *strict* is for advanced
  students. Lists may give alternatives (`servus | serva`).
- **Ancient Greek:** most keyboards can't type Greek, so the game has a
  built-in **Greek input mode** that turns Latin keys into Greek letters as you
  type:
  - Letters follow Beta Code conventions: a→α b→β g→γ d→δ e→ε z→ζ h→η q→θ i→ι
    k→κ l→λ m→μ n→ν c→ξ o→ο p→π r→ρ s→σ t→τ u→υ f→φ x→χ y→ψ w→ω.
  - Final sigma (ς) is automatic.
  - Diacritics are optional keys after a vowel: `)` smooth, `(` rough, `/`
    acute, `\` grave, `=` circumflex, `|` iota subscript. For example,
    `a)/` → ἄ.
  - An on-screen key chart is shown during Greek battles. With breathings and
    accents on *ignore* (the default), students only need the letters.
  - Students with a real Greek keyboard layout can switch the input mode off.
- **Irish:** fadas (á é í ó ú) are part of the spelling and can change the
  meaning (e.g. *sean* "old" vs *Seán*), so the default is *reduced credit* and
  *strict* is available.
  - Answers follow the Official Standard (An Caighdeán Oifigiúil). Dialect
    variants can be listed as alternatives.
  - Lenition and eclipsis (`bhean`, `mbróg`) are compared as normal letters.
- **Starter packs:** about 60–100 words per language, grouped into themes
  (animals, family, house, school, food, body, numbers, colours, verbs,
  myths/gods). They follow typical secondary-school vocabulary: GCSE-style
  French, Cambridge Latin Course-style Latin, Athenaze/JACT-style Greek, and
  Junior Cycle Irish. Each pack is checked by a person before release.

### Spaced repetition
- A Leitner box system (5 boxes) per word per profile. Misses send a word back
  to box 1; perfect answers promote it.
- The encounter builder draws mostly *due* words and some *new* words, weighted
  by monster level (harder monsters get longer or harder words).
- **Grimoire** screen: mastery per word, accuracy, average speed, and the words
  you struggle with most.

---

## 5. Dungeon and exploration

- **Generation:** BSP room placement plus corridors, joined into a spanning tree
  with a few extra loops. Each room gets a tag: start, stairs, treasure,
  monster den, puzzle vault, shop, campfire or boss.
- **Locked doors** gate parts of the floor. The generator guarantees solvability
  by placing locks only on tree edges, so there is always a route to the stairs
  (some doors guard the stairs, some guard optional loot).
- **Floors:** each floor goes deeper, with a palette and theme change (Crypt →
  Flooded Caves → Ice Halls → Lava Forge → …) and a boss before the stairs.
- **Movement:** grid-based and turn-based (roguelike ticks) with smooth
  tweening between tiles for an 8-bit feel. Arrow keys or WASD; `Space`
  interacts.
- **FOV and fog of war:** recursive shadowcasting and a torch radius with
  dithered darkness and flicker. Explored tiles stay dimly remembered.
- **Monsters on the map** wander, chase within an aggro range and start a battle
  on contact. Sneaking up on a sleeping monster grants a free first strike.
- **Minimap:** top-right, one pixel per tile, explored tiles only, with icons for
  player, doors, chests and stairs. `M` opens a full-screen map.
- **HUD:** HP/MP bars, level, gold, floor number, and a message log in a retro
  text box with a typewriter effect.

---

## 6. Combat: typing battles

A JRPG-style battle screen with a big procedural monster sprite, the hero at the
bottom, and a text-entry line in large pixel letters.

### Turn flow
1. **Your attack:** a prompt word appears (e.g. *"dog"*). Type the translation
   and press Enter.
2. **Monster attack:** the monster telegraphs ("The Ghoul raises its claws!"),
   shows a word, and a timer bar starts shrinking. Type it before the bar
   empties to **dodge**.
3. Repeat until someone drops. Other actions: `F1` item menu, `Esc` flee
   (chance-based, taking a hit if it fails).

### Formulas (starting values, to be tuned)
```
target_time = 0.8s + 0.28s × len(answer)       # scaled by difficulty setting and gear
speed       = clamp(target_time / time_taken, 0.5, 2.0)
accuracy    = Perfect 1.0 | Correct 0.85 | Accent slip 0.6 | Graze 0.3 | Miss 0
combo       = 1 + 0.1 × streak (max 2.0)       # streak of Perfect/Correct answers
damage      = ATK × accuracy × speed × combo × (CRIT 1.5 if Perfect and speed ≥ 1.5)
```
- **Dodge:** Perfect or Correct → full dodge (a "Counter!" bonus if very fast);
  Accent slip or Graze → half damage; Miss or timeout → full damage.
- **Miss penalty:** the monster gets a free hit, your combo resets, the word
  drops to SRS box 1, and the **correct spelling is shown** for a moment, since
  that is when you learn it.
- **Feedback:** floating damage numbers, "PERFECT / CRITICAL / GRAZE" pop-ups,
  hit flash, screen shake, and a different SFX for each tier.

### Monster traits (variety without art)
| Trait | Effect |
|---|---|
| Armored | Grazes and accent slips do 0 damage. Needs perfect spelling. |
| Ghost | The prompt fades out after 1.5s, so you have to remember it. |
| Mirror Imp | The prompt is shown backwards. |
| Swift | Shorter dodge timers. |
| Trickster | Prompt and answer languages swap for one turn. |
| Mimic | Hides in a chest (see §7). |
| Boss | Several phases, longer words, then short phrases in the final phase. |

---

## 7. Puzzles: doors and treasure chests

A shared `Puzzle` interface (`Prompt`, `Render`, `HandleInput`, `Check`,
`Hint`) with several generators. All of them work from the active word pack,
so none need an LLM.

| Puzzle | Description |
|---|---|
| **Anagram rune** | The letters of a foreign word are scrambled on stone tiles. The native word is the clue. |
| **Missing letters** | `p _ r r _` with the native word as the hint. |
| **Tumbler lock** | Letter wheels on a lock. Rotate them with the arrow keys to spell the word. Very 8-bit. |
| **Pair matching** | Link 4 foreign words to their translations. |
| **Odd one out** | Pick the word that doesn't belong, using pack tags (animal, food, …). |
| **Mini crossword** | 2 or 3 pack words crossing on a shared letter. |
| **Riddle / cloze** | Hand-written riddles and fill-in-the-blank sentences from `assets/puzzles/`. |
| **Reverse rune** | Given the foreign word, type the native one (recognition practice). |

- **Doors** use the easier or mid-level puzzles. Failing costs a little HP (a
  trap) and gives a short cooldown. Hints cost MP or a *Hint Scroll*.
- **Chests** use harder puzzles and give better loot. Failing a chest has a
  chance to **wake a Mimic**, which you then have to fight.
- Puzzle difficulty scales with floor depth and word mastery.

---

## 8. RPG layer

- **Classes** (chosen at character creation):
  - *Knight*: more HP and DEF.
  - *Scribe*: bonus damage from speed; spells cost less MP.
  - *Rogue*: longer dodge windows; better chest loot.
- **Stats:** HP, MP, ATK, DEF, Focus (typing time bonus), Luck (crit and loot).
- **XP and levelling:** XP from monsters, puzzles and first-time perfect words.
  Level-ups show a classic stat-increase fanfare.
- **Items:** Potions (HP/MP); *Hint Scroll* (reveals the first letter);
  *Hourglass* (slows timers for one battle); *Rune of Clarity* (live typo
  highlighting for one battle); keys; gold.
- **Equipment:** weapon, armour and trinket, assembled procedurally from parts
  with generated names ("Rusty Quill of Swiftness"). Gear changes the formulas
  (e.g. +10% dodge window).
- **Shops and campfires:** a merchant on some floors; campfires restore HP and
  show your Grimoire and your weakest words.
### Game modes
Runs are roguelite: the dungeon is new every run, and dying ends the run. Word
mastery (SRS data) is **always** kept, so every run makes you better.

**Adventure mode (default)**
- **Save Shrines** appear every few floors (and always before a boss). Using
  one saves your hero, inventory and floor.
- When you die, you wake up at the last shrine you used with some gold lost,
  and that floor is regenerated.
- You can save and quit at a shrine. Quitting elsewhere keeps a suspend save
  that is deleted when you load it, so saves can't be abused.
- All strictness and timer settings are available.

**Hardcore mode (competitive)**
- One life and no shrines. Quitting only suspends the run (and the suspend save
  is deleted when you load it).
- **Fixed rules**, so scores are comparable: each language has a set
  strictness preset, normal timers, and no Hourglass or Rune of Clarity.
  Settings are locked during the run.
- **Score:** one number that grows as you go deeper.
  ```
  score = floor_reached × 1000
        + Σ damage dealt
        + perfect_words × 50
        + best_combo × 100
        + bosses × 2500
        + chests × 150
        − misses × 25
  ```
  The HUD shows the floor, score and a "personal best" marker.
- **Compete with friends:**
  - **Daily Dungeon:** the seed comes from the date plus the word list, so
    everyone playing that day with the same list gets the same dungeon.
  - **Seed challenge:** share a 6-character seed code so friends can play the
    same dungeon.
  - At the end of a run you get a **share code**, e.g.
    `HW-FR-0922-F12-18450-K7QX`. It holds the language, date/seed, floor, score
    and a checksum, so it can be pasted into a group chat. The game can check a
    friend's code. This isn't cheat-proof, but it catches typos and casual
    edits.
  - A local **Hall of Fame** lists the top 10 per language and per mode.
- An online leaderboard is a possible later addition (it needs a small server,
  which is out of scope for now).

**Saves:** JSON in `os.UserConfigDir()/halpwords/`: profiles (with SRS data),
shrine save, suspend save, Hall of Fame, and settings. Word lists go in the
`words/` folder next to them.

---

## 9. Procedural graphics and audio

**Palette:** a fixed 32-colour retro palette. Each floor theme remaps it (cold
blues for the Ice Halls, reds for the Forge). Palette cycling animates fire and
water.

**Tiles** (generated into a texture atlas at startup, per floor):
- Walls: brick or stone patterns with value noise, ordered dithering, and
  bitmask autotiling for edges, tops and shadows.
- Floors: noise speckle, cracks, moss and puddles, placed with seeded
  variation.
- Doors, stairs, torches and chests: small template bitmaps stored as string art
  in code, colourised by palette.

**Monster sprites** (seeded generator, so a given monster always looks the same):
- Body families (slime, bat, skeleton, eye, golem, spider, ghost), each a
  base mask template plus random cells.
- Mirror symmetry, cellular-automata smoothing, automatic outline, 2-tone
  shading, and eye placement.
- 2-frame idle animation from squash/offset. Hit, flash and death-dissolve
  effects are done in code.
- The same generator renders at 16×16 for the map and 48×48 for battles.
- Elite and boss variants get size, palette and extra features (horns, crowns).

**Hero:** a 16×16 template in code (string art), recoloured by class and gear.

**Items and icons:** procedural (flasks with random liquid colours, weapons
built from blade/guard/hilt parts).

**Effects:** particles, damage numbers, screen shake, torch flicker, and a
CRT/scanline filter as an optional shader (Kage).

**Font:** the only required external asset is **GNU Unifont**, an 8×16 pixel
bitmap font dual-licensed under the SIL OFL 1.1 and GPLv2+ with the font
embedding exception.
- It covers Latin-1, Latin Extended-A (macrons), and Greek plus Greek Extended
  (all polytonic forms such as ἄ ᾧ ῥ).
- We embed a small subset in Unifont's simple `.hex` text format and parse it
  ourselves, so no font library is needed. The OFL licence text ships with
  the game.
- Unifont's 8×16 glyphs already look retro.

**Audio (no files):**
- An sfxr-style synth (square, triangle, saw and noise with ADSR and pitch
  slides) for keystrokes, hits, crits, dodges, typos, doors and chests.
- A procedural music sequencer: per-floor key and scale, generated melody and
  bass patterns, and a battle tempo.

**AI art overrides:** if `overrides/monster_<family>.png` or
`overrides/title.png` exists, it is used instead of the procedural version. AI
art can be added later without changing code. Optional targets: title screen,
boss portraits, NPC portraits.

---

## 10. LLM integration (optional)

### Plumbing
- A `Provider` interface with an **Anthropic (Claude)** implementation first and
  an **OpenAI-compatible** implementation second. The second also covers local
  models via Ollama or LM Studio, for free, private play.
- Default models: a fast, cheap model (e.g. Claude Haiku 4.5) for in-game
  content, and a stronger model (e.g. Claude Sonnet 5) for Word Forge pack
  generation.
- The API key comes from the Settings screen or an environment variable. It is
  stored in the user config dir with `0600` permissions and never written into
  saves. OS keychain storage comes later.
- **Never on the critical path:** requests run in goroutines with timeouts. The
  game **pre-fetches content for the next floor** while you play the current
  one. If a request fails or is late, the procedural or fixed version is used.
- **Structured JSON output** validated against a schema. Anything that fails
  validation is discarded. For example, answers must exist in the word pack,
  and puzzles must be solvable by the local checker.
- **The LLM never grades spelling.** Correctness is always checked locally. The
  one exception is free-form conversation with the Oracle.
- Disk cache of generated content and a per-session cost/token cap.
- **Always family-safe:** every prompt includes an age-appropriate
  (13-year-old) content policy, and output is run through a local word filter.
  This can't be turned off.
- **Set up by a parent or teacher:** the LLM settings (key, provider, budget)
  sit in a separate "Parent/Teacher" settings page. Students never have to
  handle API keys.

### Ideas for a more dynamic game (ordered by value)
1. **Dungeon Director:** before each floor, the LLM gets the words that are due,
   your weak spots and the pack's tags. It returns a themed floor script: a
   floor name ("The Drowned Pantry" for a food pack), a theme from a fixed
   list, monster names and descriptions tied to the words, lore snippets and a
   boss concept. The procedural engine builds the floor around it.
2. **Generated puzzles:** riddles, cloze sentences and mini-stories that use your
   words in context, in the dungeon's voice. They go into the same puzzle
   interface as the fixed ones.
3. **Monsters speak your target language:** battle cries and taunts built from
   words you know. On deeper floors, dodge prompts become short phrases, which
   takes you from words to sentences naturally.
4. **Mnemonic Tutor ("Scroll of Insight"):** when a word is missed repeatedly,
   the LLM writes a memory hook or etymology tip, shown at the next campfire.
5. **The Oracle (NPC):** a wandering sage you can talk to in the target
   language. It answers simply, gently corrects your grammar, and rewards
   correct use of pack words. This is the only free-text grading.
6. **Word Forge:** "Give me 30 B1 Spanish travel verbs" creates a new pack. A
   second validation pass checks the translations, and you can edit the result
   before playing.
7. **Side quests:** "The cook lost 3 food words. Find them in monster drops."
   These create targeted practice.
8. **Bard's Tale recap:** at the end of a run, a short saga of your adventure
   that names the words you mastered. It could be shared.
9. **Adaptive difficulty coaching:** looks at your mistake patterns (such as
   always missing double consonants) and suggests or generates a focused mini
   pack.

---

## 11. Cross-platform build and distribution

| Target | How |
|---|---|
| macOS arm64 (primary) | `go build ./cmd/halpwords` (needs Xcode Command Line Tools). Package as a `.app` bundle. Universal binary later via `lipo` with amd64. |
| Windows amd64/arm64 | `GOOS=windows GOARCH=amd64 go build …` from a Mac, with no cgo needed. |
| Linux amd64/arm64 | Needs cgo plus X11/GL dev libs. Build on a GitHub Actions Ubuntu runner (or Docker). |
| Web (WASM) | `GOOS=js GOARCH=wasm`. LLM calls from the browser need CORS (Anthropic supports direct browser access with a header, but the user supplies the key). |
| Mobile (later) | `ebitenmobile`. Typing on phones needs an on-screen keyboard flow. |

- A `Makefile` (or `mage`) with targets: `run`, `test`, `build-mac`,
  `build-windows`, `build-linux`, `build-web`, `bundle-mac`.
- GitHub Actions: tests on every push, plus a release matrix producing all
  artifacts.
- macOS code signing and notarisation are documented for public releases.
  Unsigned builds work locally (right-click → Open).

---

## 12. Testing strategy

- Unit tests for the pure-logic packages:
  - dungeon generation: connectivity, locks always solvable, determinism from a
    seed;
  - answer grading tiers and Unicode edge cases;
  - damage formulas;
  - puzzle generators (every generated puzzle is solvable);
  - SRS scheduling;
  - LLM JSON validation using recorded fixture responses.
- A headless simulation: a bot plays N floors with seeded "typing skill" to
  check balance (time-to-kill, death rate).
- `go vet`, `staticcheck` and `gofmt` in CI.

---

## 13. Milestones

| # | Milestone | Deliverable |
|---|---|---|
| **M0** ✅ | Skeleton | Go module, Ebitengine window on Arm Mac, pixel-perfect scaling, scene stack, font rendering, Unicode text input, Makefile, CI. *Done: also includes the grading engine, Tab accents, Greek input mode, and a spelling practice screen.* |
| **M1** | Dungeon | BSP generator, procedural tiles, grid movement, camera, FOV and fog, minimap and full map, stairs to the next floor. |
| **M2** | Words and combat | Word list loader and starter lists (French, Latin, Greek, Irish), grading engine with per-language rules, Tab accent helper, Greek input mode, battle scene, attack/dodge loop, procedural monster sprites, SFX synth. **First playable.** |
| **M3** | Puzzles | Locked doors and chests, 6+ puzzle generators, fixed riddle bank, Mimic. |
| **M4** | RPG layer | Classes, stats, XP and levels, items, equipment, shop, campfire, bosses, Save Shrines, suspend save, title and menus. |
| **M5** | Learning and competition | Spaced repetition, Grimoire stats screen, per-language strictness settings, Hardcore mode with score, Daily Dungeon, seed and share codes, Hall of Fame. |
| **M6** | LLM | Provider interface (Claude plus OpenAI-compatible), settings UI, pre-fetch and cache, Dungeon Director, generated puzzles, monster taunts, Mnemonic Tutor, Word Forge. |
| **M7** | Polish and ship | Procedural music, CRT shader, juice pass, balance simulation, `.app` bundle, Windows/Linux/Web release builds. |

Stretch: Oracle NPC, side quests, Bard's Tale, text-to-speech pronunciation
(useful for French and Irish), two-player race mode, online leaderboard, a
teacher "class pack" export.

---

## 14. Decisions

| Topic | Decision |
|---|---|
| Languages | English → French, Latin, Ancient Greek, Irish. UI in English. |
| Word list format | Plain text, `english = answer \| alternative`, with `## tag` groups (§4). |
| Audience | Secondary school, about 13. Family-safe, encouraging tone, relaxed default timers. |
| Progression | Roguelite runs. Adventure mode has Save Shrines; Hardcore has one life and a comparable score, share codes and a Daily Dungeon (§8). |
| Strictness | Configurable per language: accents, breathings, articles, capitals, timers (§4). |
| LLM | Optional. Claude first, OpenAI-compatible/local second. Set up by a parent or teacher. Always family-safe. |
| Stack | Go + Ebitengine. |
| Font | GNU Unifont subset (OFL 1.1). |
