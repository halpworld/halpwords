// Command balance plays the dungeon with bots of different typing skill
// and prints, floor by floor, how long fights take, how much they hurt and
// how often heroes fall. Use it to check changes to monsters, heroes and
// the battle formulas.
//
//	go run ./tools/balance [-runs 200] [-floors 12] [-lang fr] [-class knight]
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/sim"
	"github.com/halpworld/halpwords/pkg/words"
)

func main() {
	runs := flag.Int("runs", 200, "runs to average for each typist")
	floors := flag.Int("floors", 12, "floors to play")
	lang := flag.String("lang", "fr", "the language of the word lists: fr, la, grc or ga")
	class := flag.String("class", "all", "knight, scribe, rogue or all")
	flag.Parse()

	lists, err := words.LoadFS(assets.Words, "words")
	if err != nil {
		log.Fatal(err)
	}
	var entries []words.Entry
	for _, l := range lists {
		if l.Language == *lang {
			entries = append(entries, l.Entries...)
		}
	}
	if len(entries) == 0 {
		log.Fatalf("no words for language %q", *lang)
	}
	for _, c := range rpg.Classes {
		if *class != "all" && !strings.EqualFold(*class, c.String()) {
			continue
		}
		for _, t := range sim.Typists {
			var all [][]sim.FloorStats
			for i := 0; i < *runs; i++ {
				all = append(all, sim.Run(t, c, entries, uint64(i)+1, *floors))
			}
			fmt.Printf("\n%s, %s typist (%d runs)\n", c, t.Name, *runs)
			fmt.Println("floor  turns/fight  HP lost/fight  falls  fell%  boss won  boss HP  level  potions  gold")
			for _, s := range sim.Summarize(all) {
				boss := "                 "
				if s.BossFloor {
					boss = fmt.Sprintf("    %4.0f%%    %4.0f%%", 100*s.BossWon, 100*s.BossHP)
				}
				fmt.Printf("%5d  %11.1f  %12.0f%%  %5.2f  %4.0f%%  %s  %5.1f  %7.2f  %4.0f\n",
					s.Depth, s.TurnsPer, 100*s.HPPer, s.Falls, 100*s.FellOnce, boss, s.Level, s.Potions, s.Gold)
			}
		}
	}
}
