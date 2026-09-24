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
> **Status: milestone 4 (RPG layer) done; milestone 5 (learning and competition) is next.** The game is playable from the
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
- **Word puzzles.** Rune-sealed doors and locked chests hold nine kinds of
  puzzle: read a rune and give its meaning, pick the odd one out, match words
  to their meanings, solve a riddle, unscramble letter tiles, fill in missing
  letters, turn the letter wheels of a tumbler lock, fill in a mini
  crossword, or spell a word from memory. A wrong answer springs a trap, and
  from floor 2 some chests are **Mimics** that bite back.
- **Heroes and gear.** Play a sturdy **Knight**, a word-wise **Scribe** or
  a lucky **Rogue**. Level up, find and buy weapons, armour and trinkets
  with generated names ("Iron Ring of Swiftness"), and carry potions,
  ethers, Hint Scrolls, Hourglasses and Runes of Clarity.
- **Shrines, campfires, merchants and bosses.** Pray at **Save Shrines** to
  save your adventure, rest at **campfires** to heal and see the words you
  keep missing, trade with the **merchant**, and beat the crowned **boss**
  that guards the stairs every third floor.
- **Hints.** Stuck on a word? <kbd>F4</kbd> shows the next letter for a
  little MP.
- **Pause and suspend.** <kbd>Esc</kbd> pauses the game, battle clock
  included. Suspend and quit from the pause menu, and pick up where you
  left off with **Continue**.
- **Four languages.** French, Latin, Ancient Greek (polytonic) and Irish, with
  built-in starter word lists.
- **Forgiving grading.** Answers are graded as Perfect, Correct, Accent slip,
  Graze or Miss, so a missing accent or a small typo still counts for
  something.
- **Easy accents.** Press <kbd>Tab</kbd> to cycle the accent on the last letter
  (e → é → è → ê → ë). Ancient Greek has a built-in Greek keyboard with
  breathings and accents.
- **Bring your own words.** Word lists are plain text files you can write in
  any editor. Import them by dropping them on the game, add to or delete
  lists, and save them from the **Word Lists** screen.
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
  <tr>
    <td width="50%"><img src="docs/media/tumbler-lock.png" alt="A tumbler lock on a chest: letter wheels to turn until they spell the French for 'fish'"></td>
    <td width="50%"><img src="docs/media/crossword.png" alt="A mini crossword over the dungeon view: 'frère' across, crossed by the French for 'dog' and 'treasure'"></td>
  </tr>
  <tr>
    <td align="center"><b>Tumbler locks:</b> turn the wheels to spell the word.</td>
    <td align="center"><b>Mini crosswords:</b> words that share a letter.</td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/media/class-picker.png" alt="Choosing a hero: the Knight, the Scribe and the Rogue, with their stats"></td>
    <td width="50%"><img src="docs/media/boss.png" alt="A battle with the Slime King, a crowned boss, with a hint showing the first letters of the answer"></td>
  </tr>
  <tr>
    <td align="center"><b>Three heroes:</b> Knight, Scribe or Rogue.</td>
    <td align="center"><b>Bosses</b> guard the stairs. <kbd>F4</kbd> buys a hint.</td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/media/shop.png" alt="The merchant's shop: potions, ethers, scrolls and three pieces of gear for sale"></td>
    <td width="50%"><img src="docs/media/campfire.png" alt="Resting at a campfire, which restores HP and MP and shows four words the player missed"></td>
  </tr>
  <tr>
    <td align="center"><b>The merchant</b> buys and sells.</td>
    <td align="center"><b>Campfires</b> heal and show the words you missed.</td>
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

Then choose **New Adventure**, pick a language and a hero, and find the
stairs down.

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
   - Press <kbd>F1</kbd> to drink a potion, or <kbd>F4</kbd> for a hint:
     it shows the next letter for 2 MP (1 for a Scribe), or uses up a Hint
     Scroll. A word you needed a hint for comes back for practice later.
   - <kbd>Esc</kbd> pauses the battle and stops its clock. From the pause
     menu you can use **Items** or **Flee**. Using an item takes your turn.
   - A perfect answer restores 1 MP, and the first time you spell a word
     perfectly you earn 2 XP.
   - From floor 2, some monsters have **traits**, shown next to their name.
     The first time you meet one, the game explains it. From floor 4, any
     monster can have an extra trait, such as a *Swift Grumpy Rat*.
3. **Unlock.** Walk into a sealed door or a treasure chest to get a word
   puzzle. Doors ask for meanings, odd words out, matching pairs, riddles
   and anagrams; chests ask for careful spelling (missing letters, tumbler
   locks and, from floor 3, crosswords) and hold gold, items and sometimes
   gear. Solving a puzzle earns XP; <kbd>F4</kbd> gives hints on puzzles you
   type. A wrong answer sets off a trap, or wakes a Mimic: it keeps the loot
   until you defeat it.
4. **Grow stronger.** Each class grows differently as it levels up. Press
   <kbd>I</kbd> for your items and gear: wear better gear, drop what you
   don't need, and drink potions or ethers. An **Hourglass** gives you half
   as much time again to type in your next battle, and a **Rune of
   Clarity** turns your typing red as soon as it goes wrong.
5. **Use what you find.**
   - **Save Shrines** (a floating blue crystal) stand in the first room of
     floors 2, 3, 5, 6, 8, 9 and so on. Pray at one to save your adventure.
   - **Campfires** heal you and restore your MP once, and show the words you
     have been missing.
   - The **merchant** sells items and three pieces of gear, and buys gear
     from your bag for half its price.
6. **Beat the bosses.** Every third floor, a crowned boss guards the stairs,
   and the stairs won't open until it is defeated. Bosses grow swift and
   then write their words backwards as they weaken, ask for longer words,
   and always drop gear.
7. **Falling.** If you are defeated, you wake up at the last shrine you
   prayed at (or the dungeon's entrance) as you were when you prayed, with a
   fifth of your gold gone. The floor there is new. The words you have
   practised are never lost.
8. **Take a break.** Press <kbd>Esc</kbd> to pause. **Suspend and quit**
   keeps the game exactly as it is, and **Continue** on the title screen
   picks it up. A suspended game can only be continued once; after that,
   Continue takes you back to your last shrine. You can't suspend in the
   middle of a battle. There is one save slot. On the desktop it is
   `adventure.json`, next to the [`words` folder](#your-own-word-lists); on
   the web it is kept in the browser's local storage.

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
| <kbd>I</kbd> | Items and gear (<kbd>Enter</kbd> use or wear, <kbd>D</kbd> drop) |
| <kbd>M</kbd> | Full map |
| <kbd>Esc</kbd> | Pause menu: items, suspend, or quit to the title |

**In battles and puzzles**

| Key | Action |
|---|---|
| Type + <kbd>Enter</kbd> | Attack, dodge, or solve |
| <kbd>1</kbd>–<kbd>4</kbd>, or <kbd>←</kbd> / <kbd>→</kbd> + <kbd>Enter</kbd> | Pick a word (odd-one-out puzzles) |
| <kbd>↑</kbd> / <kbd>↓</kbd> to choose a word, <kbd>←</kbd> / <kbd>→</kbd> to swap its meaning | Pair matching |
| <kbd>←</kbd> / <kbd>→</kbd> to choose a wheel, <kbd>↑</kbd> / <kbd>↓</kbd> to turn it | Tumbler locks |
| <kbd>↑</kbd> / <kbd>↓</kbd> to choose a word, <kbd>Enter</kbd> for the next one | Crosswords (<kbd>Enter</kbd> checks when every word is filled in) |
| <kbd>F1</kbd> | Drink a potion (in battle, instead of attacking) |
| <kbd>F4</kbd> | Hint: show the next letter, for MP or a Hint Scroll |
| <kbd>Esc</kbd> | Pause (items and flee are in the pause menu), or leave a puzzle |

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

Choose **Word Lists** on the title screen to manage your lists in the game:

<p align="center">
  <img src="docs/media/word-lists.png" width="640" alt="The Word Lists screen, importing a list of farm animals: it can become a new list or be added to French - First Steps">
</p>

- **Import:** drag one or more files onto the game window (or the web page),
  or press <kbd>I</kbd> and type the file's path. Then choose to make it
  **a new list** or **add** its words to an existing list in the same
  language. Words that are already in the list are skipped. If the file has no
  `language:` line, pick the language with <kbd>←</kbd> / <kbd>→</kbd>.
- **Delete:** mark lists with <kbd>Space</kbd> (<kbd>A</kbd> marks them all),
  then press <kbd>X</kbd> or <kbd>Delete</kbd>. With nothing marked, it
  deletes the selected list. Starter lists can't be deleted, but deleting one
  you've added words to puts it back as it was.
- **Save:** changes are kept until you press <kbd>S</kbd>, which saves all
  lists at once. Leaving with unsaved changes asks first.

Imported files can use the format below, or be a plain two-column
`english<Tab>answer` file, as spreadsheets and flashcard sites export them.

You can also put `.txt` files in the game's `words` folder yourself (in a web
browser, lists are kept in the page's local storage instead):

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
- Riddle puzzles use the English riddles in
  [`assets/puzzles/riddles.txt`](assets/puzzles/riddles.txt), so they work
  for any language. Words without a riddle there just get other puzzles.
- A file with the same name as a starter list (such as `french.txt`)
  replaces that starter list.

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
internal/dungeon/  floor generation, monsters, bosses, shrines and shops
internal/rpg/      classes, stats, levels, items and gear (no Ebitengine dependency)
internal/save/     save files (local storage on the web)
internal/raycast/  first-person 3D view
internal/words/    word lists, languages, answer grading
internal/typing/   text entry, Tab accents, Greek input mode
internal/combat/   battle formulas and monster trait effects
internal/puzzle/   door and chest word puzzles (no Ebitengine dependency)
internal/audio/    sound effect synth (no Ebitengine dependency)
internal/input/    keyboard helpers
internal/proc/     procedural pixel art (no Ebitengine dependency)
internal/gfx/      drawing: text, windows, torches
internal/pal/      the 32-colour palette
internal/unifont/  bitmap font parser
assets/            embedded font, starter word lists and riddle bank
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
- [x] **M3: Puzzles.** Nine puzzle types (reverse rune, odd one out, pair
      matching, riddle, anagram, missing letters, tumbler lock, mini
      crossword, spelling), a riddle bank, and the Mimic.
- [x] **M4: RPG layer.** Three classes, six stats, items, generated gear,
      a merchant, campfires, bosses, Save Shrines, suspend saves, hints.
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
  [starter word lists](assets/words) are very welcome, and so are new
  [riddles](assets/puzzles/riddles.txt).

## License

- Code: [Apache License 2.0](LICENSE).
- Font: [GNU Unifont](https://unifoundry.com/unifont/), used under the SIL
  Open Font License 1.1 (see
  [assets/fonts/OFL-1.1.txt](assets/fonts/OFL-1.1.txt)).

## Acknowledgements

- [Ebitengine](https://ebitengine.org), the 2D game engine for Go.
- The Commodore 64 and Amiga crawlers that inspired it: *The Bard's Tale*,
  *Dungeon Master* and *Eye of the Beholder*.
