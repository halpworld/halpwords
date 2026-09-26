# Halpwords: Plan

An 8-bit dungeon-crawler RPG for learning to spell words in a foreign language.
You explore a randomly generated dungeon **in first person, one grid step at a
time**, like the classic Commodore 64 crawlers (The Bard's Tale, Dungeon
Master, Eye of the Beholder). Monsters are fought by **typing
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
│   ├── dungeon/                 # generation, monsters, automap memory
│   ├── raycast/                 # first-person view: walls, floor, ceiling, sprites
│   ├── gfx/                     # palette, procedural tiles, sprite generator,
│   │                            # text rendering, particles, screen shake, lighting
│   ├── audio/                   # sfxr-style synth, procedural music sequencer
│   ├── combat/                  # damage/dodge formulas, monster traits, encounter builder
│   ├── rpg/                     # stats, levelling, items, equipment, loot tables
│   ├── profile/                 # settings, word memory and Hall of Fame, kept between runs
│   ├── llm/                     # provider interface, Anthropic + OpenAI-compatible,
│   │                            # prompt templates, JSON validation, cache, budget
│   ├── playtest/                # the web game's play-test: a quest fetched from its own website
│   ├── save/                    # profiles, run saves, settings (JSON in user config dir, or memory)
│   └── move/                    # the web game's addresses; hands browser saves to a new address
├── pkg/                         # public API, also used by halpwords-server
│   ├── words/                   # word packs, answer matching, Unicode normalisation, SRS
│   ├── puzzle/                  # puzzle interface, generators, fixed puzzle bank
│   ├── compete/                 # Hardcore score, seed and share codes, Daily Dungeon, Hall of Fame
│   ├── proc/                    # procedural pixel art, the app icon
│   ├── safety/                  # family-safe AI policy and text filter
│   └── maps/                    # hand-made maps and quests (.hwmap, .hwquest) and their checks
├── assets/                      # embedded: font, word packs, puzzle bank, quests, hero template
│   ├── fonts/                   # Unifont subset (.hex) + OFL licence
│   ├── words/                   # starter lists: french.txt, latin.txt, greek.txt, irish.txt
│   └── puzzles/                 # hand-written riddles/cloze templates per pack
├── web/moved/                   # the "We've moved" page for the web game's old address
└── overrides/                   # optional drop-in PNGs (AI art) that replace procedural sprites
```

Rules:
- **Game logic is pure Go with no Ebitengine imports** (`dungeon`, `words`,
  `combat`, `puzzle`, `rpg`, `compete`, `profile`, `llm`). That makes it unit-testable, deterministic
  from a seed, and portable.
- **`pkg/` is a public API.** halpwords-server imports `pkg/words`,
  `pkg/compete`, `pkg/puzzle`, `pkg/proc`, `pkg/safety`, `pkg/settings`
  and `pkg/maps` (list format, grading, word memory, seeds and scores,
  worksheets, pictures, the AI policy, per-language settings, the map
  format and its checks), so the two never disagree. Rules for it:
  - No Ebitengine, `internal/game`, `internal/scene` or `internal/gfx`,
    directly or through another package; `pkg/imports_test.go` checks this.
  - Any change to an exported name or to behaviour the server relies on
    (list format, grading, scores) needs a note in `CHANGELOG.md`, and
    breaking changes are avoided: add, don't rename.
  - The server requires a commit or tag of this module, so nothing there
    changes until it moves to a newer one.
- **Scenes are a stack** (explore → battle → back to explore; puzzle overlays).
- **Seeded RNG everywhere**, so a dungeon seed can reproduce a run for debugging
  and sharing.
- **Logical screen 640×360** (16:9), integer-scaled to the window.
  - The screen is laid out like a C64 crawler: the 3D view top left, the hero
    and automap windows on the right, and a message log below that turns into
    the typing panel in battles and puzzles.
  - The 3D view is raycast at half resolution and drawn at 2×, so the art has
    a chunky pixel grid.
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

## sentences
>> Le ___ mange du pain. | chien
```
- `english = answer`. Extra accepted answers go after `|`.
- Header fields: `title:` and `language:` (`fr`, `la`, `grc`, `ga`), and
  the optional `id:`, `version:` (a whole number), `level:` (such as a CEFR
  level), `source:` and `licence:`. `id:` and `version:` are set by
  halpwords-server; the others describe where a list comes from. A header
  line is read as a header even if its value has `=` in it.
- Keys starting with `x-` are ignored, so newer lists can add settings
  without breaking this game. Any other unknown `key:` is an error (it is
  usually a typo). Game 1.0 knows only `title:` and `language:`, so the
  server offers downloads *for older games* without the new lines.
- `## name` starts a tag group, used by puzzles such as odd-one-out and by LLM
  themes. A bare `##` goes back to no group.
- `>> sentence with ___ | answer` is a gap-fill sentence for cloze puzzles
  (§7): one `___` gap, then after the last `|` the word that fills it,
  which must match one of the list's answers (ignoring case, spacing,
  apostrophe style and Unicode composition), or the list doesn't load.
  Only that answer is accepted in the gap. Lists' sentences are used with
  or without an AI, but not on scored runs (Hardcore, Daily Dungeon).
- `words.Format` writes a list back in this format (headers, then words in
  order with their groups, then sentences), and parsing its output gives the
  same list; the server exports lists with it.
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
  | Live typo highlighting | on · off | *off* (the Rune of Clarity gives it for one battle) |
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

  *Now (1.0):* French 102, Latin 98, Greek 96 and Irish 100 words. Nouns
  carry their article in French and are bare in the others; Latin and
  Greek adjectives are masculine nominative singular, Latin and Greek verbs
  1st person present, French verbs infinitives, and Irish verbs the
  imperative (the dictionary form). Every word has a riddle (180 in all).
  The lists were drafted for 1.0 and **still need checking by a speaker of
  each language**.

### Spaced repetition
- A Leitner box system (5 boxes) per word per profile. Misses send a word back
  to box 1; perfect answers promote it.
- The encounter builder draws mostly *due* words and some *new* words, weighted
  by monster level (harder monsters get longer or harder words).
- **Grimoire** screen: mastery per word, accuracy, average speed, and the words
  you struggle with most.

*Now (M5, done):* `words.Memory` keeps a `Card` per word (keyed by its
prompt and first answer, so extra alternatives don't reset it) in
`progress.json`, per language, across every adventure and Practice.

- **Boxes:** the first answer puts a word in box 1. Perfect moves it up,
  Correct keeps it, an accent slip, graze or hint moves it down one, and a
  miss sends it to box 1. A word is due again after 3, 8, 20, 50 or 120 more
  answers in that language (box 1 to 5): the clock counts answers, not days,
  so a week away doesn't flood the player with due words.
- **Dealing:** the deck still retries words missed on this adventure two
  times in five. Otherwise 60% of deals go to due words and 25% to new ones
  when there are some. Monsters aim at a difficulty (length, plus 1.5 for
  each marked letter) that grows with the floor and is higher for bosses:
  the deck looks at four candidates and takes the nearest.
- **Mistake kinds:** `words.Classify` names what went wrong: accents,
  double letters, swapped letters, a missing, extra or wrong letter, or a
  wrong word (an article left out doesn't count). Battles, puzzles and
  Practice show a tip, and the Grimoire counts them.
- **Grimoire:** from the title screen, the pause menu (not in a battle) or
  a campfire (<kbd>G</kbd>): words mastered, right %, average typing time,
  a bar of the boxes, the most common slip, and every word with its box,
  right % and time. <kbd>Tab</kbd> sorts by weakest first, list order or A
  to Z. Campfires show this adventure's missed words first, then the
  Grimoire's weakest.
- **Settings:** the table above, per language, in `settings.json`. They
  apply to battles, puzzles and Practice in Adventure mode.

---

## 5. Dungeon and exploration

The dungeon is a **first-person, grid-based crawler** (a "blobber"): the hero
stands in one cell, faces north, east, south or west, and moves one cell or a
quarter turn at a time.

- **View:** a raycaster draws textured walls, floors and ceilings, doors,
  wall torches that light their surroundings, and billboarded monster and
  chest sprites. Steps and turns are tweened over a few frames, and a bump
  nudges the camera when you walk into a wall.
- **Movement:** `↑`/`W` forward, `↓`/`S` back, `←`/`A` and `→`/`D` turn,
  `Q`/`E` strafe. A tap moves exactly one cell and holding a key repeats.
  Walking into something uses it: doors open, monsters are attacked, chests and
  sealed doors start puzzles. `Space` or `Enter` does the same for the cell in
  front, and descends when standing on the stairs.
- **Turns:** monsters act each time the hero steps, waits or opens a door.
  They wander, chase when close, and **ambush** when they reach you: the hero
  turns to face them and the fight opens with a dodge.
- **Generation:** rooms plus corridors, joined into a spanning tree
  with a few extra loops. Each room gets a tag: start, stairs, treasure,
  monster den, puzzle vault, shop, campfire or boss.
- **Locked doors** gate parts of the floor. The generator guarantees solvability
  by placing locks only on tree edges, so there is always a route to the stairs
  (some doors guard the stairs, some guard optional loot).
- **Floors:** each floor goes deeper, with a palette and theme change (Crypt →
  Flooded Caves → Ice Halls → Lava Forge → …) and a boss before the stairs.
- **Light:** the hero's torch fades with distance, with dithered darkness.
- **Automap:** cells the raycaster sees (up to 7 cells away) are remembered and
  drawn in the map window with walls, torches, doors, sealed doors, chests,
  stairs, nearby monsters, and the hero with an arrow for facing. `M` opens
  the full map over the 3D view. A compass shows the facing.
- **HUD:** level, language, HP and XP bars, ATK, gold, potions and combo, plus
  a four-line message log.
- **Hand-made floors:** a map (`.hwmap`) is a grid of cells with monsters,
  the puzzles and words on its locks, and notes on walls; a quest
  (`.hwquest`) is 1 to 10 maps in order with an introduction and an ending.
  `pkg/maps` holds the format and its checks (a wall all round, one start
  and one stairs, doors between walls, everything reachable, a start that
  isn't sealed in, and, given the word lists, that each lock's word is in
  them and its puzzle can be made), shared with halpwords-server's map
  editor. `dungeon.FromMap` builds a floor from a map; loot, stock and
  monster looks are rolled from the seed as usual, and generated floors
  turned into maps (`dungeon.ToMap`) pass the same checks. Quests are
  under *New Adventure → Quest*, or dropped on the window.
  *Now (W10.1, done):* the formats, the checks, `FromMap`, the Quest
  picker with a built-in quest, dropped files kept in the `quests` folder,
  quest saves, and the intro and ending pages. Quests from a linked game
  and play-testing from the website come with the server (W10.3, W10.4).
  *Now (server W10.3, done):* **play-testing** in the web game. The
  server's *Play-test* button opens the web game with
  `?quest=<address of the quest file>` (a short-lived signed link).
  `internal/playtest` accepts only an address on the page's own origin
  (same scheme, host and port, no user name, no backslashes), fetches it
  with no cookies and no redirects, and checks it with `pkg/maps`; the
  game then starts the quest (the language picker or the class). A
  play-test keeps every file in memory (`save.UseMemory`) from before the
  profile loads: nothing is saved, the player's own saves are neither
  read nor changed, no AI key is loaded, and nothing is sent anywhere.
- **Save points:** Save Shrines (§8). Falling wakes the hero at the last
  shrine they prayed at, on a new floor.
- **Later:** sneaking up on a sleeping monster for a free first strike, and
  shadowcast fog for the full map.

---

## 6. Combat: typing battles

Battles happen in the 3D view, crawler-style: the camera closes in on the
monster, its name and HP bar appear at the top of the view, and the bottom
panel becomes a text-entry line in large pixel letters. The monster flashes
when hit, lunges when it attacks, and shrinks away when defeated.

### Turn flow
1. **Your attack:** a prompt word appears (e.g. *"dog"*). Type the translation
   and press Enter.
2. **Monster attack:** the monster telegraphs ("The Ghoul raises its claws!"),
   shows a word, and a timer bar starts shrinking. Type it before the bar
   empties to **dodge**.
3. Repeat until someone drops. Other actions: `F1` drinks a potion (costs your
   attack), `Esc` flees (50% chance; a failed escape lets the monster attack,
   and a monster you escape from is stunned for a few turns).

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

*Now (M2):* Armored (Moss Golem), Ghostly (Wisp), Mirrored (Mirror Imp) and
Swift (Crypt Spider, 70% of the usual dodge time). No monster on floor 1 has
a trait. From floor 4, any monster can get one extra trait (10% per floor
past 3, up to 40%), which goes in front of its name ("Swift Grumpy Rat").
The first time the hero meets a trait, the battle opens with a card that
explains it. The Mimic came in M3 (§7).

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

*Now (M3, done):* `internal/puzzle` (now `pkg/puzzle`) has a `Puzzle` interface (`Kind`,
`Answer`, `Ask`, `Clue`, `Tiles`, `Check`, `Word`) with no Ebitengine
dependency; the crawl scene draws it and reads the keys. Puzzles answered by
setting slots (pairs, tumblers) also implement `Chooser`, and crosswords
implement `Crossworder`. Nine kinds are in:

- **Reverse rune** (doors): read the foreign word, type it in English. Every
  meaning the lists give that word is accepted.
- **Odd one out** (doors): pick one of four words with <kbd>1</kbd>–<kbd>4</kbd>
  or the arrow keys. Needs lists with `## tag` groups; otherwise it becomes a
  spelling puzzle.
- **Anagram rune** (doors and chests): the letters are on stone tiles and the
  article stays in place. From floor 4, chest anagrams have one extra letter.
- **Missing letters** (chests): hides 35% of the letters on floor 1, growing
  to 60%.
- **Spelling** (doors from floor 2, chests from floor 3): the English word
  only.
- **Pair matching** (doors): four foreign words, each next to one of their
  English meanings, shuffled so none starts right. `↑`/`↓` choose a word and
  `←`/`→` swap its meaning with another word's. All four must be right.
  Words that share a meaning or a spelling are never in the same puzzle.
- **Riddle** (doors): an English riddle from `assets/puzzles/riddles.txt`,
  answered in the target language. The bank is keyed by English word, so it
  works for every language; every starter word has at least one riddle.
- **Tumbler lock** (chests): a wheel per letter (3 to 10 letters) with the
  right letter and decoys from the lists: 3 letters per wheel on floors 1–2,
  4 on floors 3–5, then 5. At most one decoy is the right letter with other
  accents. Wheels start on wrong letters; the article and punctuation are
  fixed plates.
- **Mini crossword** (chests from floor 3): an across word crossed by one
  down word, or two from floor 6 (at least two columns apart), drawn over
  the 3D view. Words are typed without articles, `↑`/`↓` choose a word and
  `Enter` moves to the next empty one or checks. The crossword is as good
  as its worst word.

Accent slips are accepted; anything worse zaps you for 2 HP, and
<kbd>Enter</kbd> deals a new puzzle for the same lock. Chests give gold and
potions. Puzzles have no timer. The start room of a floor never has a sealed
door, so you can always walk out of it. From floor 2, some chests are
**Mimics** (15%, plus 5% per floor, up to 35%). They look like any other
chest; solve the puzzle and it opens as usual, fail it and the Mimic ambushes
you. It never moves, and when defeated it drops the chest's gold and potions.
Hints came with MP in M4: <kbd>F4</kbd> shows the next letter of a typed
puzzle's answer.

**Finishing M3: work plan** (all done)

- [x] **Riddle bank** (doors): `assets/puzzles/riddles.txt` holds English
  riddles keyed by English word (`dog = I wag my tail and bark at the
  postman.`). The hero reads the riddle and types the answer in the target
  language, so one bank works for every language. Only words in the active
  lists with a riddle can be used; otherwise the lock gets another puzzle.
  Cloze sentences in the target language came with LLM generation in M6
  (gap-fill puzzles), and word lists can carry their own (`>>` lines, §4).
- [x] **Pair matching** (doors): four foreign words on the left, their
  English meanings shuffled on the right. `↑`/`↓` choose a row and `←`/`→`
  swap its meaning with another row's. All four pairs must be right. Needs
  four words with different meanings.
- [x] **Tumbler lock** (chests): one letter wheel per letter of the word,
  each with the right letter and a few decoys from the lists, set to a wrong
  position. `←`/`→` choose a wheel, `↑`/`↓` turn it. The article and
  punctuation are fixed plates. Words of 3 to 10 letters.
- [x] **Mini crossword** (chests, from floor 3): two words crossing on a
  shared letter, three from floor 6, drawn as a grid over the 3D view. The
  clues are the English words; `↑`/`↓` or `Enter` move between words. Words
  are without articles, and a crossing must be the same letter.
- [x] Puzzle mix: doors get riddles and pair matching; chests get tumblers
  and crosswords. Tests: every generated puzzle is solvable and its right
  answer passes; wrong answers fail.
- [x] Update README, and mark M3 done.

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

*Now (M4, done):* `internal/rpg` holds the hero with no Ebitengine
dependency: classes, stats, levels, items and gear.

- **Classes** are picked after the language. Knight: 36 HP, DEF 1, +8 HP
  a level. Scribe: 30 HP, 10 MP, Focus 2, half as much damage again from
  speed above ×1, hints for 1 MP instead of 2. Rogue: 30 HP, Luck 4, 25%
  more dodge time, half as much gold again from chests. Each has a 16×16
  string-art portrait recoloured per class. *(Numbers as tuned in M7.)*
- **Stats:** HP, MP, ATK, DEF (taken off every blow, but a blow always does
  a third of its power, and at least 1), Focus (5% more typing time each,
  for the attack speed bonus and dodging), Luck (2% chance each, up to 30%,
  that a good hit is a lucky critical, and 5% more gold), and Dodge % from
  gear. Levels grow HP, MP and ATK; DEF, Focus and Luck grow on even
  levels. Level ups restore HP and MP and list what grew. Level *n* needs
  8*n* + 4*n*² XP.
- **XP** comes from monsters, from solving puzzles (2 or 3 plus half the
  depth), and 2 XP for the first perfect spelling of each word.
- **MP** pays for hints: <kbd>F4</kbd> in a battle or a typed puzzle shows
  the next letter. With no MP, a Hint Scroll is used. A hinted word goes
  back for practice and can't crit. Perfect answers in battle restore 1 MP.
- **Items:** Potion, Ether (MP), Hint Scroll, Hourglass (half as much time
  again for one battle), Rune of Clarity (typed letters turn red from the
  first mistake, for one battle). <kbd>I</kbd> opens the items screen; in a
  battle, items are in the pause menu and take the turn.
- **Gear:** weapon, armour and trinket, each a material tier (Rusty, Iron,
  Steel, Silver, Runed, Starforged; about one tier per two floors), a base
  and maybe an affix (of Swiftness, Focus, Fortune, Might, Warding, Vigor,
  the Owl), so "Iron Ring of Swiftness". Gear is saved as four numbers.
  Chests hold gear 20% of the time on floor 1, up to 40%, and other items a
  third of the time. The bag holds 6; gear that doesn't fit stays in the
  chest.
- **Floors** get features, one per room so nothing blocks a path: a **Save
  Shrine** in the start room of floors 2, 3, 5, 6, 8, 9…, a **campfire** on
  half the floors from 2 (always on boss floors) that heals once and shows
  the last five missed words, and a **merchant** on floor 2 and half the
  floors after, selling items and three pieces of gear and buying gear for
  half price.
- **Bosses** guard the stairs on every third floor (Slime King, Bone Lord,
  Gazer Queen, Golem Titan, Imp Overlord), wear a generated crown, never
  move, and hold the stairs shut until defeated. At two thirds HP they turn
  Swift, at one third Mirrored, and angry bosses ask for words of 6 letters
  or more. They always drop gear a tier or so above the floor.
- **Saving:** praying at a shrine saves the hero and the floor. Falling
  wakes the hero at the last shrine (or the entrance) as they were when
  they prayed, with 20% of their gold gone, on a remade floor; the save on
  disk follows, so quitting can't undo a fall. **Suspend and quit** in the
  pause menu keeps the exact game; Continue loads it once and deletes it.
- **Title and menus:** New Adventure → language → class. Continue shows
  the language, class and floor.

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

*Now (M5, done):* `internal/compete` (now `pkg/compete`) (no Ebitengine dependency) holds the
score, codes and Hall of Fame; `internal/profile` keeps the Hall of Fame in
`halloffame.json`.

- **New Adventure** asks for the mode first: Adventure, Hardcore, Daily
  Dungeon or Seed Challenge, then the language and class.
- **Hardcore** floors are made from the same seed as Adventure floors, then
  lose their shrines, and chests swap Hourglasses for Ethers and Runes of
  Clarity for Hint Scrolls; the merchant doesn't sell them. Hardcore uses
  `profile.Preset` (the language's grading rules, normal timers, no
  highlighting). Falling ends the run. **Give up the run** replaces Quit in
  the pause menu and records the score. A suspended run is deleted from
  disk as it is loaded, so it can't be replayed from a copy of the save.
- **Score:** as above. Damage counts up to the monster's remaining HP, a
  Mimic counts as a chest, and perfect words are perfect answers without a
  hint. The map window shows the score, and ★ BEST once it passes the
  table's best.
- **Seeds:** new runs get a 30-bit seed, written as 6 characters of
  Crockford's base 32 (no I, L, O or U; typed O, I and L read as 0, 1 and
  1). The pause menu shows it. `HALPWORDS_SEED` still overrides it.
- **Daily Dungeon:** the seed is an FNV hash of the date, the language and
  the sorted word keys of every list in that language.
- **Share codes:** `HW-FR-0924-F12-18450-K7QX` for a Daily Dungeon (month
  and day) or `HW-FR-7K3QZP-F12-18450-K7QX` for a seed, with a 4-character
  checksum. The Hall of Fame checks a friend's code (<kbd>C</kbd>) and
  compares it with your best; Seed Challenge accepts a share code too.
- **Hall of Fame:** top 10 per language for Hardcore (random and seed runs)
  and for the Daily Dungeon, with name, class, floor, score and date. The
  Game Over screen adds the score up part by part and asks for a name when
  the run makes the table.
- **Title:** Continue, New Adventure, Practice, Grimoire, Hall of Fame,
  Word Lists, Settings, Quit, in two columns.

**Saves:** JSON in `os.UserConfigDir()/halpwords/`: profiles (with SRS data),
shrine save, suspend save, Hall of Fame, and settings. Word lists go in the
`words/` folder next to them.

---

## 9. Procedural graphics and audio

**Palette:** a fixed 32-colour retro palette. Each floor theme remaps it (cold
blues for the Ice Halls, reds for the Forge). Palette cycling animates fire and
water.

**Textures** (generated at startup, per floor, for the 3D view):
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
- Sprites are billboarded in the 3D view and scale with distance.
- Elite and boss variants get size, palette and extra features (horns, crowns).

**Hero:** a 16×16 template in code (string art), recoloured by class and gear.

**Items and icons:** procedural (flasks with random liquid colours, weapons
built from blade/guard/hilt parts).

**Effects:** particles, damage numbers, screen shake, torch flicker, and a
CRT/scanline filter as an optional shader (Kage).
*Now (M7):* `gfx.Sparks` throws art-pixel sparks: off every blow (more and
faster for a critical hit, which also freezes the action for five ticks),
off armour, as dust from a defeated monster, as coins from chests and
monsters, as runes when a seal breaks or the hero prays, and as light when
healing or levelling up. Damage numbers pop in a size larger and settle.
The CRT filter is a Kage shader in `DrawFinalScreen`: curved glass,
scanlines (from 2× scale), a little glow between pixels and darker
corners, soft or strong. Screen shake can be turned off.

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
  *Now (M2):* `internal/audio` renders 32 effects from tone tables, and
  `game.Sound` plays them through `oto`. It uses `oto` directly because
  Ebitengine's audio package stops the game with an error when there is no
  sound device, and sound must never be required. `F3` mutes, and
  `HALPWORDS_SOUND=off` skips the audio device entirely.
- A procedural music sequencer: per-floor key and scale, generated melody and
  bass patterns, and a battle tempo.
  *Now (M7):* `audio.Compose` writes an 8-bar loop (4 for the lament) from a
  `Track`, a mood and a seed: a key near A4, a scale and tempo range and a
  chord progression per mood, a bass line (long notes, eighths or pumping
  octaves), an arpeggio or a pad, drums, and a lead that walks the scale
  and lands on chord tones on strong beats, as a phrase, a variation, a
  contrasting phrase and a return that ends on the home note. Moods:
  title, delve (each floor its own seed), fight, boss, camp (campfires,
  shrines, the merchant, Practice) and lament. `RenderLoop` wraps tails
  round so the loop has no seam. `game.Sound` renders a track in the
  background (pausing now and then in a browser, which runs one thing at a
  time), keeps four, loops it through its own `oto` player and cross-fades
  over half a second. Scenes implement `game.Musical`; a scene without it
  plays the music of the scene under it, or the title's.

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
  *Now (M6):* `internal/llm` offers Anthropic (the Messages API over plain HTTP,
  as Anthropic's Go SDK would nearly triple the web download),
  OpenAI, Meta (its Model API with the Muse Spark models, since the Llama API
  closed in July 2026) and DeepSeek. A catalog prices the known models; any
  other model a key can use is counted at the provider's highest price. The
  AI Helper screen sets the provider, key, models and a budget per provider,
  and shows the spending the game has counted plus DeepSeek's own balance
  (the others have no balance API for normal keys). `UseEndpoint` can point
  a provider at a local server.
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

## 10a. The link to a grown-up's account

A family or class can link the game to an account on halpwords-server (its
`docs/api`), so a grown-up sees progress and assigns word lists. Like the
AI helper it is optional, never blocks the game loop, and the game plays the
same with the server down or when it was never linked.

*Now (W1.8, done):* `internal/link` (no Ebitengine dependency) links with a
pairing code and keeps the device tokens in a private `link.json` (local
storage on the web). Answers to assigned words, daily totals for the rest,
and play sessions wait in `link-queue.json` (at most 50,000 events; the
oldest answers fold into daily totals) and go up every three minutes, when
asked and on quitting. Assigned lists are downloaded by ETag into
`assigned/` and shown read-only in Word Lists; the server's word memory is
merged in (server cards for server words, queued answers replayed on top);
settings and accommodations from `/me` are locked in Settings with who set
them. Title → **Account** links, syncs and unlinks (progress and lists stay;
licensed lists go). Hardcore runs keep the standard settings. The web build
uses the same client (Go's `net/http` runs on `fetch` there) with the files
in local storage, a queue of at most 10,000 events for its ~5 MB, and saves
the queue when the page is hidden or closed. The server allows the web
game's origin on the game API (CORS, halpwords-server W1.7d), and since a
browser won't let a page set `User-Agent`, the web build sends its version
as `X-Halpwords-Client: halpwords/1.2.0 (js; wasm)` instead. Unlinking
also tells the server (`POST /api/v1/unlink`, in the background, after
refreshing an old access token), so the game leaves the child's page on
the website; offline, the family removes it there. Still to do:
checking it all against staging (W1.8's "Done when"),
per-assignment settings, and the accommodations the game doesn't have yet
(cheaper hints, larger text, no timed dodges).

*Now (W1.9, done):* assignments show as **assignment quests** (`link.Quest`
methods in `internal/link/quest.go` turn the server's goal, dates and
progress into "Master 20 words", "8/20 words", "due tomorrow"). The title
screen shows the one to do next in a banner and gains **Assignments**,
which lists them (to do by due date, then not started, then complete) with
a bar and a due date; Enter plays one in Practice (only its list) or in an
Adventure (only its words, kept in the save as `Assignment`), from where
its answers count (←/→ when both do). Campfires show the current one, and
Q there opens the list to look at. With the AI helper on, the Dungeon
Director is told the assignment's name, and in other Adventures in that
language up to 8 of its words, weakest first. Progress is the server's as
of the last sync; per-assignment settings are read but not applied yet,
and goal kinds or modes the game doesn't know are shown as just practise,
anywhere. In the code they are `assignRun`, `Assignments` and
`run.assign` (`internal/scene/assignments.go`), apart from W10.1's
hand-made quests (`maps.Quest`, `run.quest`); a save can carry both.

*Now (W7.4, done):* Title → **Play Together** joins a room a grown-up
opened on the website (`/play/host`), by its 6-character code, over
halpwords-server's `/api/v1/play` WebSocket (its `docs/api/play.md`). Only
a linked game can: it gets a one-use ticket with its device token. The
lobby shows who is in the room (learners by pseudonym, grown-ups by role)
and what happens, and sends the six preset phrases and four emotes the
server offers, at most one a second; there is no free text and nothing
else can be sent. `link.Play` gets back into the room after a drop (for up
to 55 seconds; the server keeps the place for 60), joins again to resync
after a gap in the room's event numbers, pings every heartbeat, and goes
back to the lobby with a reason when the room ends, the host removes the
player or the server restarts. The desktop game speaks the WebSocket
protocol itself (`internal/link/wsframe.go`: RFC 6455 text frames,
ping/pong, close and client masking, with a fuzzed frame reader), so the
game keeps its three dependencies; the web build uses the browser's
`WebSocket`. Boss Raid (W7.5) and Race (W7.6) start from this lobby.

*Now (W7.6, done):* **Race.** In a race room (`mode` `race`) the host
starts a race on the website with one of the family's or teacher's word
lists; the lobby shows a 5-second countdown and then every racer's game
plays **the same dungeon**, built from the race's seed and list alone
(`startRunWith`: no player lists, profile, memory or AI touch the floors
or the deal of words). A race run has Hardcore rules (one life, no
shrines, so floors never change), the first class, no saves or suspend,
and sends nothing to the grown-up's account (no answers, no play
session): only `progress` to the room, through `link.Play.Report`
(throttled by `pkg/race.Reporter`: floor, monsters beaten, the hero's
cell, a fall). Other racers on the same floor are small coloured dots on
the automap and the side map; the side panel shows the floor of the goal
and the time left. Reaching floor 3, falling or giving up opens the race
screen, which waits for the others and then shows the results by place
(a racer the server flagged is "not counted"). `pkg/race` holds the
rules both sides use; the server's `Judge` checks every report against
the crawl's own timings (a step is 9 ticks = 150 ms, a monster falls in
30 ticks = 0.5 s; tests keep them in step). A game that can't read the
race's list drops out at once. Still to do: a live race between two
games against staging, and friend-group and class rooms (W7.1, W2.2).

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

*Now (M7):* builds are stamped with `git describe` (`game.Version`), shown on
the title screen. `make bundle-mac` makes a universal `Halpwords.app` with
an `.icns` icon, signed ad hoc or with `MACOS_SIGN_IDENTITY`; the Windows
builds (x64 and Arm) carry the icon and version through `go-winres`; the
web page has a loading bar and favicon. The icon is drawn in code
(`proc.Icon`) and written by `tools/icon`; the window uses it too. Pushing
a `v*` tag runs `.github/workflows/release.yml`: tests, every build, a
GitHub release with checksums and the changelog's notes, notarisation when
the secrets are set, and the web version on GitHub Pages
([docs/RELEASING.md](docs/RELEASING.md)). A panic in the game loop writes
`crash.txt` to the user folder.

*Now (W1.16, code done; the move waits for DNS):* the web game is moving
from `halpworld.github.io/halpwords` to `play.halpwords.com`. `move.WebURL`
is the one place the address players are sent to lives (README and
`docs/RELEASING.md` must agree; a test checks). `web/moved` (`make
build-moved`) is the page the old address will serve: it reads the saves
from local storage and opens the new address with them in the fragment
(`#import=k/n:id:data`, raw DEFLATE and base64url, split into parts over
900,000 characters, the AI keys left behind). On start the web game takes
the parts (`move.Receive`), asks before replacing progress already there
(`scene.moveAsk`), imports once and clears the fragment. The cut-over steps
for people, and the proposed release workflow, are in
[docs/RELEASING.md](docs/RELEASING.md) and `docs/cutover/release.yml`.

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
  *Now (M7):* `internal/sim` plays real floors with the game's monsters,
  hero, deck and formulas: every monster (half of them ambush), every
  sealed door and chest (Mimics included), the merchant (better gear, then
  potions up to three), the campfire before the boss, and a potion below
  35% HP. Typists are *beginner*, *average* and *strong*, with shares of
  each grade that worsen with word length, and typing times that vary.
  `tools/balance` (`make balance`) prints, per class and typist, attacks per
  fight, HP lost per fight and to the boss, falls, level and gold per floor;
  `sim_test.go` keeps the balance within bounds.
- `go vet`, `staticcheck` and `gofmt` in CI (`make vet`).

---

## 13. Milestones

| # | Milestone | Deliverable |
|---|---|---|
| **M0** ✅ | Skeleton | Go module, Ebitengine window on Arm Mac, pixel-perfect scaling, scene stack, font rendering, Unicode text input, Makefile, CI. *Done: also includes the grading engine, Tab accents, Greek input mode, and a spelling practice screen.* |
| **M1** ✅ | Dungeon crawl | First-person grid movement with a raycast view, room-and-corridor generator, procedural wall/floor/ceiling textures per floor theme, wall torches, automap and full map, compass, stairs and floor save points. *Done: also pulls forward the battle loop from M2 (attack/dodge in the 3D view, procedural monster sprites, XP and levels, potions, fleeing) and the first two puzzles from M3 (sealed doors, missing-letter chests).* |
| **M2** ✅ | Words and combat | Word list loader and starter lists (French, Latin, Greek, Irish), grading engine with per-language rules, Tab accent helper, Greek input mode, battle scene, attack/dodge loop, procedural monster sprites, SFX synth. **First playable.** *Done: also includes the monster traits Armored, Ghostly, Mirrored and Swift (§6). Trickster, Mimic and Boss come later.* |
| **M3** ✅ | Puzzles | Locked doors and chests, 6+ puzzle generators, fixed riddle bank, Mimic. *Done: the `Puzzle` interface, nine generators (reverse rune, odd one out, pair matching, riddle, anagram, missing letters, tumbler lock, mini crossword, spelling), an English riddle bank that works for every language, and the Mimic. Target-language cloze sentences wait for M6.* |
| **M4** ✅ | RPG layer | Classes, stats, XP and levels, items, equipment, shop, campfire, bosses, Save Shrines, suspend save, title and menus. *Done: three classes, six stats, five items, generated gear, a merchant, campfires, five bosses with phases, Save Shrines with a one-use suspend save, and MP hints (§8). The Grimoire screen waits for M5; campfires show the missed words for now.* |
| **M5** ✅ | Learning and competition | Spaced repetition, Grimoire stats screen, per-language strictness settings, Hardcore mode with score, Daily Dungeon, seed and share codes, Hall of Fame. *Done: a five-box Leitner memory per word that lasts across runs and Practice, mistake kinds with tips, difficulty that grows with depth, the Grimoire, a Settings screen, Hardcore, Daily Dungeon and Seed Challenge modes, share codes that can be checked, and a Hall of Fame (§4, §8).* |
| **M6** ✅ | LLM | Provider interface (Claude plus OpenAI-compatible), settings UI, pre-fetch and cache, Dungeon Director, generated puzzles, monster taunts, Mnemonic Tutor, Word Forge. *Done: four providers (Anthropic's Messages API, plus OpenAI, Meta's Model API and DeepSeek through chat completions, all over plain `net/http` to keep the web build small), an AI Helper screen (provider, pasted key checked for free, game and Word Forge models with prices, a budget, spending and DeepSeek's live balance), the Dungeon Director with the next floor pre-fetched, gap-fill (cloze) puzzles and riddles kept in a content bank, monster taunts, the Scroll of Insight at campfires, and the Word Forge with a checking pass (§10). Phrase dodges, the Oracle, side quests, the Bard's Tale recap and coaching wait.* |
| **M7** ✅ | Polish and ship | Procedural music, CRT shader, juice pass, balance simulation, `.app` bundle, Windows/Linux/Web release builds. *Done: a music composer with six moods and cross-fades, Sound & Screen settings (volumes, CRT filter, full screen, screen shake) also in the pause menu, sparks, hit-stop and popping damage numbers, a balance simulation with typing bots and the tuning it led to (§13a), a procedural app icon, crash reports, version stamping, a universal macOS app, Windows x64/Arm builds with icons, a web loading page, and a tag-driven release workflow that publishes to GitHub Releases and Pages (§9, §11, §12).* |

### 13a. Balance (M7)

The first simulation showed the dungeon getting *easier* with depth: heroes
reached level 21 by floor 12 (each level a full heal), damage taken per
fight fell floor by floor, Knights took almost nothing, and bosses hurt
little more than other monsters. The tuning:

- Levels need 8*n* + 4*n*² XP (was 12*n*): about level 12 on floor 12.
- Monster ATK grows 22% a floor (HP still 15%), so deeper floors hurt more
  without longer fights.
- Bosses have about 40% more HP and 1 more ATK.
- DEF takes off at most two thirds of a blow.
- Every class gains DEF on even levels; the Knight starts with DEF 1, the
  Scribe with 30 HP (+6 a level), the Rogue with DEF 0.
- *For 1.0*, the starter lists grew from about 20 words to about 100,
  including very short ones (numbers, colours). Monsters then found easy
  words for their target, and strong typists finished early fights in one
  word. Word targets now start at difficulty 6 (floors 1–3 all ask for 6,
  then it grows by 0.6 a floor as before).

Now, on average (300 runs with the French list; HP lost per fight, then to
the boss):

| Typist | Floor 1 | Floor 6 | Floor 12 | Fell on floor 12 |
|---|---|---|---|---|
| Beginner | 5–8% | 21%, boss 42–48% | 18–22%, boss 55–84% | 75–84% |
| Average | 1–2% | 8–9%, boss 16–19% | 10–12%, boss 25–36% | 2–13% |
| Strong | 0% | 4%, boss 6–9% | 6–7%, boss 13–19% | 0% |

Beginners are safe on floors 1–3 and meet real danger from floor 5, where
shrines and relaxed timers help. Fights take about one and a half to six
attacks (strong typists at the short end), so each asks for three to
twelve words. Greek plays a little easier than French, as its accents and
breathings are ignored by default.

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
