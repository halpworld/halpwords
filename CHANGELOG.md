# Changelog

Notable changes to Halpwords. The release workflow uses the section for a
version as its release notes.

## Unreleased

### Added

- **Halpwords AI.** A game linked to an account whose plan includes AI in
  the game gets floor stories, gap-fill sentences, riddles, monster taunts
  and memory tips from the Halpwords server, with no API key. It is on by
  itself when available; AI Helper offers it as a provider and Off turns
  it off. Only the words being learned are sent. Riddles a grown-up adds
  to an assigned list come with the list.
- `pkg/gameai`: the game's AI content shared with halpwords-server: the
  task names (`Tasks`), the prompts (`DirectorPrompt`, `WordsPrompt`,
  `ClozePrompt`, `RiddlesPrompt`, `TauntsPrompt`, `TipsPrompt`), the request
  and reply shapes of `POST /api/v1/ai/{task}` (`DirectorRequest`,
  `WordsRequest`, `ScriptReply`, `WordsReply`, `TauntsReply`, `TipsReply`),
  and the checks every reply passes (`CheckScript`, `CheckCloze`,
  `CheckRiddle`, `CheckTaunt`, `CheckTip`, and `Keep*` for whole replies).
- **The web game can move house.** Ready for the move from
  `halpworld.github.io/halpwords` to `play.halpwords.com` (not live yet): a
  *We've moved* page for the old address (`web/moved`, `make build-moved`)
  hands the hero, word memory, own word lists, Hall of Fame and settings
  over in the new address's URL fragment, in parts if a save is very big.
  The game takes them in once, asks before replacing progress that is
  already there, and clears the address. AI helper keys stay behind. The
  cut-over steps are in `docs/RELEASING.md`.
- **Report from the pause menu.** "Report" sends a bug report (game
  version, system, seed, and crash.txt only if you tick the box) or an
  "I was upset by..." report, with an optional note, to the Halpwords
  team. Reports wait in a small queue while offline and are sent
  anonymously unless the game is linked. Set `HALPWORDS_SERVER` to use
  another server.
- **Race** (Play Together): in a race room, the host starts a race and
  every racer plays the same dungeon, from the same seed and word list, to
  floor 3. Hardcore rules, one class, nothing saved. The other racers show
  as small dots on the map, and the race ends with a results screen.
  `link.Play` gets `Race`, `Results` and `Report`.
- `pkg/raid`: the rules of a Boss Raid in halpwords-server's play rooms
  (`TimeLimit`, `BossHP`, `Damage`, `Streak`, `AttackEvery`, `DodgeTime`,
  `Dodged`, `StunTime`), `Grade` (the game's grading with a language's
  default rules) and `BossFor`, whose `Sprite` is the crowned boss.
- `pkg/race`: the rules of a Race in halpwords-server's play rooms
  (`Goal`, `TimeLimit`, `Report`), the `Judge` the server checks each
  racer's reports with (floors in order and not sooner than walking there
  takes, monsters no faster than they fall, cells on the floor, no
  running faster than the hero walks), and the `Reporter` that decides
  when a game reports.
- **Account: link the game to a grown-up's account** on the Halpwords
  website (title screen → Account). A linked game sends answers and play
  sessions in the background, gets assigned word lists (read-only, in Word
  Lists), progress from the grown-up's other games, and settings a grown-up
  set, which Settings then shows locked. It works the same offline, and
  unlinking keeps your progress and removes the game from the website's
  list too. The web version links too, keeping everything in the
  browser's local storage.
- **Assignment quests.** Word lists a grown-up assigns show as quests on
  the title screen (the next one in a banner, all of them under
  Assignments) and at campfires, with a progress bar and a due date.
  Choosing one practises its list, or starts an Adventure with only its
  words, as the grown-up chose. With the AI helper on, the Dungeon
  Director weaves the assignment's words into its floors.
- **Quests and hand-made maps.** *New Adventure → Quest* plays hand-made
  floors in order, with an introduction and an ending. A `.hwquest` (a
  quest) or `.hwmap` (one floor) file dropped on the window is checked and
  kept in the `quests` folder; one quest comes with the game.
- **Play-testing quests in the web game.** `?quest=<address>` in the web
  game's address fetches a quest file from the game's own website and
  starts it, saving nothing (for halpwords-server's map editor).
- `pkg/maps` (new): the `.hwmap` and `.hwquest` formats (`Map`, `Quest`,
  `ParseMap`, `ParseQuest`, `Load`, `Encode`) and their checks
  (`Map.Check`, `Quest.Check`, returning `[]Problem` with the cell), plus
  the names a map can use (`MonsterKinds`, `BossKinds`, `Traits`, `Themes`,
  `PuzzleNames`, `DoorPuzzles`, `ChestPuzzles`) and `FindWord`.
  `pkg/puzzle`: `OneWord` and `MakeWord`, a puzzle about a chosen word.
- **Gap-fill sentences in word lists.** A `>> sentence with ___ | answer`
  line gives a list its own cloze puzzles, used with or without the AI
  helper (not on scored runs). The answer must be one of the list's
  answers.
- **New list header lines**: `id:`, `version:`, `level:`, `source:` and
  `licence:`. Lines whose key starts with `x-` are ignored, so later lists
  won't break this version. Other unknown keys are still an error.
- `pkg/words`: `List` gains `ID`, `Version`, `Level`, `Source`, `Licence`
  and `Cloze []ClozeLine`; `words.Format(list)` writes a list back in the
  text format, so that it parses to the same `List`. `pkg/puzzle`:
  `FromLists`, `Generated.Add`, and `ClozeLine.Answer`.
- `pkg/words`: the Grimoire's numbers, so halpwords-server shows a
  grown-up what the child sees: `GrimoireSort` (`GrimoireWeakest`,
  `GrimoireListOrder`, `GrimoireAZ`, with `String`, `ID` and
  `ParseGrimoireSort`), `Memory.SortGrimoire`, `Memory.NewGrimoire`
  (a `Grimoire` of `GrimoireRow`s), `Distinct`, `Percent` and
  `Card.WatchOut`. And the Greek typing keys: `BetaKey`,
  `BetaCodeChart`, `BetaCodeLetter`, `BetaCodeMark`, `BetaCodeLetters`,
  `BetaCodeMarks`, `BetaCodeMarkGroups` and the `Mark…` constants.
- `pkg/settings`: a player's settings for one language, `Lang` (was
  `profile.LangSettings`), and the battle `Timer` (`Normal`, `Relaxed`,
  `Fast`, `Timers`, `ParseTimer`, `Valid`), with `Preset`. settings.json
  is unchanged. The Grimoire, the Greek typing field and the settings play
  as before.
- `internal/link`: the client for halpwords-server's game API (not used
  by the game yet). It links with a pairing code, keeps its tokens in a
  private `link.json`, queues answers and sessions on disk (at most about
  50,000; older answers fold into daily totals), syncs in the background,
  downloads assigned lists by ETag, merges the server's word memory, and
  works quietly with the server down.

### Changed

- **Shared packages.** `words`, `compete`, `puzzle` and `proc` moved from
  `internal/` to `pkg/`, and the AI safety policy and filter (`Policy`,
  `Clean`) moved from `internal/llm` to `pkg/safety`, so halpwords-server
  can import them. They are now a public API: changes to them are noted
  here. No change to how the game plays.
- `pkg/words`: `List.Source` (the file name) is now `List.File`; `Source`
  is the new `source:` header. `List.Format` (still grouped by tag)
  writes the new lines too.

## v1.0.0

The first release: every milestone in [PLAN.md](PLAN.md) is done.

### Added

- **Music.** A chiptune composer writes each piece from a seed: a calm tune
  for each floor, drums for battles, a darker, faster theme for bosses, a
  quiet one by campfires, shrines and the merchant, and a lament when the
  hero falls. Tracks cross-fade and loop without a seam.
- **Sound & Screen settings**, on the title screen and in the pause menu:
  music and effect volumes, full screen (also remembered from F11), screen
  shake, and a CRT filter (off, soft or strong) with scanlines, curved glass
  and darker corners.
- **Juice.** Sparks fly off blows, critical hits stop time for a moment and
  hit harder on screen, defeated monsters crumble to dust, gold showers out
  of chests, seals break in a burst of runes, and healing and level ups
  glow. Damage numbers pop in and settle.
- **An app icon**, drawn in code from the game's own art, for macOS,
  Windows, Linux and the web page.
- **Release builds**: a universal macOS app, Windows (x64 and Arm), Linux
  and the web, stamped with their version, with checksums. The web version
  has a loading screen.
- **Crash reports.** If the game crashes it writes `crash.txt` to the user
  folder, to send with a bug report.
- **Bigger starter lists.** Each language now has about 100 words (was
  about 20), in themes: animals, family, home, school, food, the body,
  numbers, colours, verbs, gods, adjectives and the dungeon. Every word has
  a riddle, so the riddle bank has grown from 54 to 180.
- **A balance simulation.** Typing bots of three skills play the dungeon
  with every class; `make balance` prints the tables and tests keep the
  balance within bounds.

### Changed

- **Balance**, from the simulation. The dungeon now gets harder as you go
  deeper, instead of easier:
  - Levels take longer: level *n* needs 8*n* + 4*n*² XP (was 12*n*), so a
    hero is about level 12 on floor 12 rather than 21.
  - Monster attack grows 22% a floor (was 15%); their HP still grows 15%.
  - Bosses have about 40% more HP and 1 more attack, so they are a real
    fight.
  - DEF can take off at most two thirds of a blow.
  - Every class gains DEF on even levels. The Knight starts with DEF 1
    (was 2); the Scribe starts with 30 HP and gains 6 a level (was 28 and
    5); the Rogue starts with DEF 0.
- Monsters on the first three floors ask for words of difficulty 6 or
  more, so a quick typist still needs a few words a fight now that the
  lists hold very short words.
- `make vet` (and CI) runs staticcheck.
- The pause menu is a little more compact, to fit Sound & Screen.
- Quit is not on the title screen in a web browser, where it did nothing.
