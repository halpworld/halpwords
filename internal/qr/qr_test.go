package qr

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// golden is a matrix in testdata, made once outside the repo by two
// independent reference encoders (rsc.io/qr with each mask forced, and
// github.com/boombuler/barcode with its own choice of mask, checked
// against rsc.io/qr): see the first line of each file.
type golden struct {
	name    string
	data    []byte
	version int
	mask    int
	auto    bool
	rows    []string
}

func readGoldens(t *testing.T) []golden {
	t.Helper()
	files, err := filepath.Glob("testdata/*.txt")
	if err != nil || len(files) == 0 {
		t.Fatalf("no goldens: %v", err)
	}
	var out []golden
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		g := golden{name: filepath.Base(f)}
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			k, v, _ := strings.Cut(line, " ")
			switch {
			case strings.HasPrefix(line, "#") && strings.Contains(line, " "):
				// the comment line
			case k == "data":
				g.data, err = hex.DecodeString(v)
			case k == "version":
				g.version, err = strconv.Atoi(v)
			case k == "mask":
				g.mask, err = strconv.Atoi(v)
			case k == "auto":
				g.auto = v == "true"
			default:
				g.rows = append(g.rows, line)
			}
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
		}
		out = append(out, g)
	}
	return out
}

func render(c *Code) []string {
	var rows []string
	for y := range c.Size() {
		var b strings.Builder
		for x := range c.Size() {
			if c.Dark(x, y) {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
		rows = append(rows, b.String())
	}
	return rows
}

func diff(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d rows, want %d", name, len(got), len(want))
	}
	bad := 0
	for y := range got {
		if got[y] != want[y] {
			if bad < 3 {
				t.Errorf("%s row %d:\n got %s\nwant %s", name, y, got[y], want[y])
			}
			bad++
		}
	}
	if bad > 0 {
		t.Errorf("%s: %d rows differ", name, bad)
	}
}

// TestGoldenMasks checks every module against the reference encoder with
// the same version and mask: the bit stream, the Reed–Solomon codewords,
// the interleaving, the placement, the function patterns, the format
// and version information and each mask.
func TestGoldenMasks(t *testing.T) {
	n := 0
	for _, g := range readGoldens(t) {
		c := encode(g.data, g.version, g.mask)
		diff(t, g.name, render(c), g.rows)
		n++
	}
	if n < 40 {
		t.Errorf("only %d goldens", n)
	}
}

// TestGoldenAuto checks Encode's choice of version and mask against the
// reference encoder's, which scores the masks the same way.
func TestGoldenAuto(t *testing.T) {
	n := 0
	for _, g := range readGoldens(t) {
		if !g.auto {
			continue
		}
		c, err := Encode(g.data)
		if err != nil {
			t.Fatalf("%s: %v", g.name, err)
		}
		if c.Version() != g.version || c.Mask() != g.mask {
			t.Errorf("%s: version %d mask %d, want %d mask %d", g.name, c.Version(), c.Mask(), g.version, g.mask)
		}
		diff(t, g.name, render(c), g.rows)
		n++
	}
	if n < 20 {
		t.Errorf("only %d auto goldens", n)
	}
}

// TestHelloWorldCodewords is the worked example of version 1-M from
// ISO/IEC 18004 annex I and thonky.com's tutorial: "HELLO WORLD"'s data
// codewords (in alphanumeric mode, written here as the bytes) give these
// error correction codewords.
func TestHelloWorldCodewords(t *testing.T) {
	data := []byte{32, 91, 11, 120, 209, 114, 220, 77, 67, 64, 236, 17, 236, 17, 236, 17}
	want := []byte{196, 35, 39, 119, 235, 215, 231, 226, 93, 23}
	if got := rsRemainder(data, rsGenerator(10)); !bytes.Equal(got, want) {
		t.Errorf("EC = %v, want %v", got, want)
	}
}

// TestGenerator checks a generator polynomial from thonky.com's table:
// degree 7 is α^0 x^7 + α^87 x^6 + α^229 x^5 + α^146 x^4 + α^149 x^3 +
// α^238 x^2 + α^102 x + α^21.
func TestGenerator(t *testing.T) {
	exp := func(n int) byte {
		v := byte(1)
		for range n {
			v = gfMul(v, 2)
		}
		return v
	}
	var want []byte
	for _, e := range []int{87, 229, 146, 149, 238, 102, 21} {
		want = append(want, exp(e))
	}
	if got := rsGenerator(7); !bytes.Equal(got, want) {
		t.Errorf("generator = %v, want %v", got, want)
	}
}

// TestFormatBits checks level M's format strings (ISO/IEC 18004 table
// C.1, thonky.com's format string table).
func TestFormatBits(t *testing.T) {
	want := []string{
		"101010000010010", "101000100100101", "101111001111100", "101101101001011",
		"100010111111001", "100000011001110", "100111110010111", "100101010100000",
	}
	for m, w := range want {
		got := strconv.FormatUint(uint64(formatBits(m)), 2)
		got = strings.Repeat("0", 15-len(got)) + got
		if got != w {
			t.Errorf("mask %d: %s, want %s", m, got, w)
		}
	}
}

// TestVersionBits checks the version strings (ISO/IEC 18004 table D.1).
func TestVersionBits(t *testing.T) {
	want := map[int]string{
		7: "000111110010010100", 8: "001000010110111100",
		9: "001001101010011001", 10: "001010010011010011",
	}
	for v, w := range want {
		got := strconv.FormatUint(uint64(versionBits(v)), 2)
		got = strings.Repeat("0", 18-len(got)) + got
		if got != w {
			t.Errorf("version %d: %s, want %s", v, got, w)
		}
	}
}

// TestCapacity checks each version's byte capacity at level M (ISO/IEC
// 18004 table 7) and that Encode moves up a version one byte past it.
func TestCapacity(t *testing.T) {
	want := []int{0, 14, 26, 42, 62, 84, 106, 122, 152, 180, 213}
	for v := 1; v <= MaxVersion; v++ {
		if got := Capacity(v); got != want[v] {
			t.Errorf("Capacity(%d) = %d, want %d", v, got, want[v])
		}
		for _, n := range []int{want[v-1] + 1, want[v]} {
			c, err := Encode(bytes.Repeat([]byte{'a'}, n))
			if err != nil || c.Version() != v || c.Size() != 17+4*v {
				t.Errorf("%d bytes: version %d (size %d), %v; want %d", n, c.Version(), c.Size(), err, v)
			}
		}
	}
	if _, err := Encode(make([]byte, 214)); !errors.Is(err, ErrTooLong) {
		t.Errorf("214 bytes: %v, want ErrTooLong", err)
	}
	if Capacity(0) != 0 || Capacity(11) != 0 {
		t.Error("capacity outside 1–10")
	}
}

// TestLinkAddress: the address the game shows fits a version 3 code, as
// the spec says (42 bytes on the test server).
func TestLinkAddress(t *testing.T) {
	for _, s := range []string{"https://halpwords.com/link?c=ABCDEFGH", "https://test.halpwords.com/link?c=ABCDEFGH"} {
		c, err := Encode([]byte(s))
		if err != nil || c.Version() > 3 {
			t.Errorf("%s: %v, version %d", s, err, c.Version())
		}
	}
}

// TestQuietZone: outside the code is light.
func TestQuietZone(t *testing.T) {
	c, _ := Encode([]byte("x"))
	for _, p := range [][2]int{{-1, 0}, {0, -1}, {21, 0}, {0, 21}, {-4, -4}} {
		if c.Dark(p[0], p[1]) {
			t.Errorf("Dark(%d, %d) outside the code", p[0], p[1])
		}
	}
	if !c.Dark(0, 0) || !c.Dark(8, c.Size()-8) {
		t.Error("the finder's corner and the dark module are dark")
	}
}
