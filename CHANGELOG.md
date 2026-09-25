# Changelog

Notable changes to Halpwords. The release workflow uses the section for a
version as its release notes.

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
