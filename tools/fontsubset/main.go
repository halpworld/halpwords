// Command fontsubset extracts the Unicode ranges the game needs from a GNU
// Unifont .hex file (optionally gzipped) and writes a smaller .hex file that is
// embedded into the game.
//
// Usage:
//
//	go run ./tools/fontsubset -in unifont_all-17.0.05.hex.gz -out assets/fonts/unifont-subset.hex
package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
)

// ranges lists the Unicode blocks kept in the subset.
var ranges = [][2]rune{
	{0x0020, 0x007E}, // Basic Latin
	{0x00A0, 0x00FF}, // Latin-1 Supplement (French, Irish)
	{0x0100, 0x017F}, // Latin Extended-A (Latin macrons, œ)
	{0x0180, 0x024F}, // Latin Extended-B
	{0x0370, 0x03FF}, // Greek and Coptic
	{0x1E00, 0x1EFF}, // Latin Extended Additional
	{0x1F00, 0x1FFF}, // Greek Extended (polytonic)
	{0x2000, 0x206F}, // General Punctuation
	{0x20A0, 0x20CF}, // Currency Symbols
	{0x2190, 0x21FF}, // Arrows
	{0x2500, 0x25FF}, // Box Drawing, Block Elements, Geometric Shapes
	{0x2600, 0x26FF}, // Miscellaneous Symbols (♥, ⚔, ☠ ...)
	{0x2700, 0x27BF}, // Dingbats
	{0xFFFD, 0xFFFD}, // Replacement character
}

func keep(r rune) bool {
	for _, rg := range ranges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

func main() {
	in := flag.String("in", "", "input Unifont .hex or .hex.gz file")
	out := flag.String("out", "assets/fonts/unifont-subset.hex", "output .hex file")
	flag.Parse()
	if *in == "" {
		log.Fatal("-in is required")
	}

	f, err := os.Open(*in)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(*in, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			log.Fatal(err)
		}
		r = gz
	}

	o, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer o.Close()
	w := bufio.NewWriter(o)
	defer w.Flush()

	n := 0
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		cp, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v, err := strconv.ParseUint(cp, 16, 32)
		if err != nil || !keep(rune(v)) {
			continue
		}
		fmt.Fprintln(w, line)
		n++
	}
	if err := sc.Err(); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d glyphs to %s", n, *out)
}
