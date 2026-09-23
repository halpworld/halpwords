<h1 align="center">Halpwords</h1>

<p align="center">
  <strong>An 8-bit dungeon crawler where spelling is your sword.</strong><br>
  Learn French, Latin, Ancient Greek or Irish by typing your way through a
  first-person dungeon, in the style of the classic Commodore 64 crawlers.
</p>

<p align="center">
  <a href="https://github.com/halpworld/halpwords/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/halpworld/halpwords/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: Apache 2.0" src="https://img.shields.io/badge/license-Apache%202.0-blue.svg"></a>
  <a href="go.mod"><img alt="Go 1.26+" src="https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go&logoColor=white"></a>
  <img alt="Platforms: macOS, Windows, Linux, Web" src="https://img.shields.io/badge/platforms-macOS%20%7C%20Windows%20%7C%20Linux%20%7C%20Web-lightgrey">
  <a href="https://ebitengine.org"><img alt="Built with Ebitengine" src="https://img.shields.io/badge/built%20with-Ebitengine-e25c4b"></a>
</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#how-to-play">How to play</a> ·
  <a href="#controls">Controls</a> ·
  <a href="#your-own-word-lists">Word lists</a> ·
  <a href="#roadmap">Roadmap</a> ·
  <a href="#contributing">Contributing</a>
</p>

<p align="center">
  <img src="docs/media/demo-explore.gif" width="640" alt="Starting a French adventure: walking the dungeon, spelling 'la maison' to break a sealed door, then 'le trésor' to open a treasure chest">
</p>

## About

Halpwords is a spelling game disguised as a dungeon crawl. You explore a
randomly generated dungeon one grid step at a time. Everything that matters
comes down to typing a word: you attack monsters by translating quickly and
correctly, you dodge by typing before the timer runs out, and you break sealed
doors and open treasure chests by spelling.

It is written for secondary-school students (around 13) and keeps a friendly,
adventurous tone. It runs offline, needs no accounts, and ships as a single
file.

> [!NOTE]
> **Status: milestone 3 (puzzles) in progress.** The game is playable from the
> first floor down, with sound, but it is early. Expect rough edges and balance changes. See the
> [roadmap](#roadmap).

## Features

- **First-person dungeon crawling.** A raycast 3D view, step-by-step grid
  movement, an automap and a full map, a compass, and floors that get bigger
  as you go down.
- **Typing battles.** Translate to attack: the faster and more accurate you
  are, the more damage you do, and a streak of good answers builds a combo.
  Translate before the monster's timer runs out to dodge.
- **Monster traits.** Deeper down, monsters have tricks: **Armored** ones
  only take damage from exact spelling, **Ghostly** ones make their words
  fade away, **Mirrored** ones write them backwards, and **Swift** ones leave
  less time to dodge.
- **Word puzzles.** Rune-sealed doors and locked chests hold five kinds of
  puzzle: read a rune and give its meaning, pick the odd one out, unscramble
  letter tiles, fill in missing letters, or spell a word from memory. A wrong
  answer springs a trap, and from floor 2 some chests are **Mimics** that bite
  back.
- **Pause and save.** <kbd>Esc</kbd> pauses the game, battle clock
  included. Save from the pause menu and pick up where you left off with
  **Continue**.
- **RPG progression.** XP, levels, gold, potions, and a checkpoint on each
  floor.
- **Four languages.** French, Latin, Ancient Greek (polytonic) and Irish, with
  built-in starter word lists.
- **Forgiving grading.** Answers are graded as Perfect, Correct, Accent slip,
  Graze or Miss, so a missing accent or a small typo still counts for
  something.
- **Easy accents.** Press <kbd>Tab</kbd> to cycle the accent on the last letter
  (e → é → è → ê → ë). Ancient Greek has a built-in Greek keyboard with
  breathings and accents.
- **Bring your own words.** Word lists are plain text files you can write in
  any editor.
- **Spelling Practice mode.** Drill words without the dungeon.
- **Nearly all generated in code.** Textures, monsters, effects and sound
  effects are procedural. The only art asset is a pixel font.
- **Runs everywhere.** macOS (Apple Silicon and Intel), Windows, Linux and the
  web (WebAssembly).

## Screenshots

<table>
  <tr>
    <td width="50%"><img src="docs/media/battle.png" alt="A battle against a Grumpy Rat: the player must translate 'castle' into French to attack"></td>
    <td width="50%"><img src="docs/media/sealed-door.png" alt="A rune-sealed door asking the player to spell 'house' in French"></td>
  </tr>
  <tr>
    <td align="center"><b>Typing battles:</b> translate to attack, and type fast to dodge.</td>
    <td align="center"><b>Sealed doors:</b> spell the word to break the runes.</td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/media/treasure-chest.png" alt="An opened treasure chest after correctly spelling 'le trésor', rewarding 14 gold"></td>
    <td width="50%"><img src="docs/media/greek-practice.png" alt="Spelling Practice in Ancient Greek, with the answer ψυχή typed using the built-in Greek keyboard"></td>
  </tr>
  <tr>
    <td align="center"><b>Treasure chests:</b> fill in the missing letters.</td>
    <td align="center"><b>Ancient Greek:</b> type polytonic Greek on any keyboard.</td>
  </tr>
</table>

## Quick start

You need [Go 1.26 or newer](https://go.dev/dl/). There are no packaged
releases yet, so you build the game from source. It takes one command.

```sh
git clone https://github.com/halpworld/halpwords.git
cd halpwords
make run            # or: go run ./cmd/halpwords
```

Then choose **New Adventure**, pick a language, and find the stairs down.

<details>
<summary><b>macOS</b></summary>

1. Install Go: `brew install go` (or use the installer from
   <https://go.dev/dl/>).
2. Run `make run`.
3. To make a double-clickable app, run `make bundle-mac` and open
   `dist/Halpwords.app`.

</details>

<details>
<summary><b>Windows</b></summary>

1. Install Go from <https://go.dev/dl/>.
2. In the `halpwords` folder, run `go run ./cmd/halpwords`.
3. To make an `.exe`, run `make build-windows` (it works from any OS). The
   file is `dist/windows-amd64/halpwords.exe`.

</details>

<details>
<summary><b>Linux</b></summary>

Ebitengine needs the X11, OpenGL and ALSA development packages. On Debian or
Ubuntu:

```sh
sudo apt-get install libgl1-mesa-dev libxrandr-dev libxcursor-dev \
  libxinerama-dev libxi-dev libxxf86vm-dev libasound2-dev
make run
```

</details>

<details>
<summary><b>Web browser</b></summary>

```sh
make build-web
python3 -m http.server -d dist/web
```

Then open <http://localhost:8000>.

</details>

> [!TIP]
> Every push is built for all platforms by
> [GitHub Actions](https://github.com/halpworld/halpwords/actions/workflows/ci.yml).
> You can download these builds from a run's **Artifacts** section.

## How to play

<p align="center">
  <img src="docs/media/demo-battle.gif" width="640" alt="A battle with a Grumpy Rat: dodging by typing 'le frère' and 'la sœur', and attacking with 'le château' and 'le livre' until the rat is defeated">
</p>

1. **Explore.** Walk through the dungeon one step at a time. The minimap fills
   in as you go. Press <kbd>M</kbd> for the full map.
2. **Fight.** Monsters wander, and they chase you once they spot you. When one
   reaches you, a battle starts.
   - **Attack:** translate the English word into your language and press
     <kbd>Enter</kbd>. Speed, accuracy and your combo all add to the damage.
   - **Dodge:** when the monster strikes, translate the word before the timer
     runs out.
   - Press <kbd>F1</kbd> to drink a potion. <kbd>Esc</kbd> pauses the battle
     and stops its clock; choose **Flee** in the pause menu to try to run
     away.
   - From floor 2, some monsters have **traits**, shown next to their name.
     The first time you meet one, the game explains it. From floor 4, any
     monster can have an extra trait, such as a *Swift Grumpy Rat*.
3. **Unlock.** Walk into a sealed door or a treasure chest to get a word
   puzzle. Doors ask for meanings, odd words out and anagrams; chests ask for
   careful spelling and hold gold and potions. A wrong answer sets off a
   trap, or wakes a Mimic: it keeps the loot until you defeat it.
4. **Go deeper.** Find the stairs down on each floor. Each new floor is a
   checkpoint: if you are defeated, you wake up at the start of the floor with
   the stats you arrived with.
5. **Take a break.** Press <kbd>Esc</kbd> to pause. From the pause menu you
   can save and quit to the title. Choose **Continue** on the title screen to
   carry on exactly where you were. You can't save in the middle of a battle.
   There is one save slot, and each save replaces the last one. On the
   desktop the save is `adventure.json`, next to the
   [`words` folder](#your-own-word-lists); on the web it is kept in the
   browser's local storage.

## Controls

**In the dungeon**

| Key | Action |
|---|---|
| <kbd>↑</kbd> / <kbd>W</kbd> | Step forward (walk into doors, chests and monsters to use them) |
| <kbd>↓</kbd> / <kbd>S</kbd> | Step back |
| <kbd>←</kbd> / <kbd>A</kbd>, <kbd>→</kbd> / <kbd>D</kbd> | Turn left or right |
| <kbd>Q</kbd> / <kbd>E</kbd> | Strafe left or right |
| <kbd>Space</kbd> / <kbd>Enter</kbd> | Use what's in front (or wait a turn); go down the stairs |
| <kbd>P</kbd> | Drink a potion |
| <kbd>M</kbd> | Full map |
| <kbd>Esc</kbd> | Pause menu: save, or quit to the title |

**In battles and puzzles**

| Key | Action |
|---|---|
| Type + <kbd>Enter</kbd> | Attack, dodge, or solve |
| <kbd>1</kbd>–<kbd>4</kbd>, or <kbd>←</kbd> / <kbd>→</kbd> + <kbd>Enter</kbd> | Pick a word (odd-one-out puzzles) |
| <kbd>F1</kbd> | Drink a potion (in battle, instead of attacking) |
| <kbd>Esc</kbd> | Pause (flee from the pause menu), or leave a puzzle |

**Everywhere**

| Key | Action |
|---|---|
| <kbd>↑</kbd> / <kbd>↓</kbd>, <kbd>Enter</kbd> | Menus |
| <kbd>Tab</kbd> | Cycle the accent on the last letter (e → é → è → ê → ë, a → ā, a → á) |
| <kbd>←</kbd> / <kbd>→</kbd> | Change language (practice mode) |
| <kbd>F2</kbd> | Greek letters on/off (Ancient Greek) |
| <kbd>F3</kbd> | Sound on/off |
| <kbd>F11</kbd>, <kbd>Alt</kbd>+<kbd>Enter</kbd>, or <kbd>Ctrl</kbd>+<kbd>Cmd</kbd>+<kbd>F</kbd> | Fullscreen |
| <kbd>Esc</kbd> | Back |

### Typing Greek

With Greek letters on, Latin keys type Greek letters:

| a | b | g | d | e | z | h | q | i | k | l | m | n | c | o | p | r | s | t | u | f | x | y | w |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| α | β | γ | δ | ε | ζ | η | θ | ι | κ | λ | μ | ν | ξ | ο | π | ρ | σ/ς | τ | υ | φ | χ | ψ | ω |

Add marks after a vowel: `)` smooth breathing, `(` rough breathing, `/` acute,
`\` grave, `=` circumflex, `|` iota subscript, `+` diaeresis. For example,
`a)/nqrwpos` types **ἄνθρωπος**. By default, accents and breathings are
optional.

### Sound

All the sound effects are made in code. If your computer has no sound
device, the game runs silently. Set `HALPWORDS_SOUND=off` to start with sound
off completely. Run `make sounds` to write every effect to `dist/sounds` as
WAV files, which helps when tuning them.

### Replaying a dungeon

Set `HALPWORDS_SEED` to a number before you start the game to get the same
dungeon every time. This is useful for bug reports.

```sh
HALPWORDS_SEED=42 make run
```

## Your own word lists

Put `.txt` files in the game's `words` folder:

| OS | Folder |
|---|---|
| macOS | `~/Library/Application Support/halpwords/words/` |
| Windows | `%AppData%\halpwords\words\` |
| Linux | `~/.config/halpwords/words/` |

```text
# Lines starting with # are comments.
title: French - Animals
language: fr

## animals
dog = le chien
friend = l'ami | l'amie
```

- Put one word per line: `english = answer`. Put any other accepted answers
  after `|`.
- The language codes are `fr` (French), `la` (Latin), `grc` (Ancient Greek)
  and `ga` (Irish).
- `## name` starts a group of related words. Groups are used for odd-one-out
  puzzles, so give each group at least three words.

The built-in lists are in [`assets/words/`](assets/words).

## Building

```sh
make run             # run the game
make test            # unit tests
make vet             # go vet and gofmt check
make sounds          # write the sound effects to dist/sounds as WAV files
make build           # build for this computer
make build-mac       # Apple Silicon (from any OS)
make build-mac-intel # Intel Macs (from any OS)
make bundle-mac      # dist/Halpwords.app
make build-windows   # Windows .exe (from any OS)
make build-linux     # Linux (run on Linux; needs the packages above)
make build-web       # WebAssembly, in dist/web
make help            # list all targets
```

<details>
<summary><b>Project layout</b></summary>

```text
cmd/halpwords/     entry point
internal/game/     main loop, scene stack, pixel-perfect scaling
internal/scene/    screens (title, practice, the dungeon crawl, ...)
internal/dungeon/  floor generation, monsters, automap memory
internal/save/     save files (local storage on the web)
internal/raycast/  first-person 3D view
internal/words/    word lists, languages, answer grading
internal/typing/   text entry, Tab accents, Greek input mode
internal/combat/   battle formulas and monster trait effects
internal/audio/    sound effect synth (no Ebitengine dependency)
internal/input/    keyboard helpers
internal/proc/     procedural pixel art (no Ebitengine dependency)
internal/gfx/      drawing: text, windows, torches
internal/pal/      the 32-colour palette
internal/unifont/  bitmap font parser
assets/            embedded font and starter word lists
tools/fontsubset/  regenerates the font subset from GNU Unifont
tools/sfxdump/     writes the sound effects as WAV files
docs/media/        README screenshots and GIFs
```

</details>

## Roadmap

The full design is in [PLAN.md](PLAN.md). In short:

- [x] **M0: Skeleton.** Window, scaling, font, Unicode input, grading, Tab
      accents, Greek input, Spelling Practice.
- [x] **M1: Dungeon crawl.** Raycast view, generator, automap, torches,
      stairs and floor save points, typing battles, sealed doors, chests.
- [x] **M2: Words and combat.** Mostly done early in M0 and M1, plus sound
      effects and monster traits (Armored, Ghostly, Mirrored, Swift).
- [ ] **M3: Puzzles.** *In progress:* the Mimic and five puzzle types
      (reverse rune, odd one out, anagram, missing letters, spelling) are in.
      Still to come: more puzzle types and a riddle bank.
- [ ] **M4: RPG layer.** Classes, items, a shop, campfires, bosses, Save
      Shrines.
- [ ] **M5: Learning and competition.** Spaced repetition, stats, Hardcore
      mode, Daily Dungeon, share codes, Hall of Fame.
- [ ] **M6: LLM (optional).** A "Dungeon Director" that reacts to how you are
      learning, generated puzzles and memory tips.
- [ ] **M7: Polish and ship.** Procedural music, CRT shader, balancing, and
      release builds.

## Contributing

Contributions are welcome, from bug reports to new word lists to code.

- **Found a bug or have an idea?** [Open an issue](https://github.com/halpworld/halpwords/issues).
  For dungeon bugs, include the `HALPWORDS_SEED` if you can.
- **Sending a pull request?** Run `make vet test` first. CI runs the same
  checks and builds every platform.
- **Know one of the languages?** Corrections to the
  [starter word lists](assets/words) are very welcome.

## License

- Code: [Apache License 2.0](LICENSE).
- Font: [GNU Unifont](https://unifoundry.com/unifont/), used under the SIL
  Open Font License 1.1 (see
  [assets/fonts/OFL-1.1.txt](assets/fonts/OFL-1.1.txt)).

## Acknowledgements

- [Ebitengine](https://ebitengine.org), the 2D game engine for Go.
- The Commodore 64 and Amiga crawlers that inspired it: *The Bard's Tale*,
  *Dungeon Master* and *Eye of the Beholder*.
