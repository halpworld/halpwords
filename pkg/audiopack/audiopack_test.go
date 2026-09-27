package audiopack

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func tone(n int) []byte {
	s := make([]int16, n)
	for i := range s {
		s[i] = int16((i%40 - 20) * 1000)
	}
	return EncodeWAV(s, 16000)
}

func sample() *Pack {
	return &Pack{Language: "fr", List: "L1", Version: 3, Words: []Word{
		{Text: "la maison", SoundOut: "lah may-ZON", Audio: tone(800)},
		{Text: "le chat", Audio: tone(400)},
	}}
}

func TestRoundTrip(t *testing.T) {
	data, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	again, _ := Encode(sample())
	if !bytes.Equal(data, again) {
		t.Error("the same pack wrote different bytes")
	}
	p, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Language != "fr" || p.List != "L1" || p.Version != 3 || len(p.Words) != 2 {
		t.Fatalf("read %+v", p)
	}
	w, ok := p.Find("  La   MAISON ")
	if !ok || w.SoundOut != "lah may-ZON" || !bytes.Equal(w.Audio, tone(800)) {
		t.Errorf("Find = %+v, %v", w, ok)
	}
	if _, ok := p.Find("le chien"); ok {
		t.Error("found a word that isn't there")
	}
	samples, rate, err := DecodeWAV(w.Audio)
	if err != nil || rate != 16000 || len(samples) != 800 {
		t.Errorf("DecodeWAV = %d samples at %d, %v", len(samples), rate, err)
	}
}

func TestKeyNormalises(t *testing.T) {
	// "é" as one code point and as e + combining acute.
	if Key("fr", "café") != Key("fr", "café") {
		t.Error("NFC forms differ")
	}
	if Key("fr", "chat") == Key("la", "chat") {
		t.Error("the language is not part of the key")
	}
}

func TestWriteRefuses(t *testing.T) {
	cases := map[string]func(p *Pack){
		"no language":  func(p *Pack) { p.Language = "" },
		"no text":      func(p *Pack) { p.Words[0].Text = "" },
		"control char": func(p *Pack) { p.Words[0].Text = "a\nb" },
		"long text":    func(p *Pack) { p.Words[0].Text = strings.Repeat("a", MaxTextChars+1) },
		"twice":        func(p *Pack) { p.Words[1].Text = "La Maison" },
		"not wav":      func(p *Pack) { p.Words[0].Audio = []byte("hello") },
		"big clip":     func(p *Pack) { p.Words[0].Audio = tone(MaxClipBytes) },
		"stereo":       func(p *Pack) { a := tone(10); a[22] = 2; p.Words[0].Audio = a },
		"low rate":     func(p *Pack) { p.Words[0].Audio = EncodeWAV([]int16{1, 2}, 4000) },
		"no samples":   func(p *Pack) { p.Words[0].Audio = EncodeWAV(nil, 16000) },
		"too many words": func(p *Pack) {
			for i := 0; i <= MaxWords; i++ {
				p.Words = append(p.Words, Word{Text: "w" + strings.Repeat("x", i%50) + string(rune('a'+i%26)) + string(rune('a'+i/26%26)), Audio: tone(2)})
			}
		},
	}
	for name, change := range cases {
		p := sample()
		change(p)
		if _, err := Encode(p); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: err = %v, want ErrFormat", name, err)
		}
	}
}

// rawZip writes files in order, as a hostile or broken server might.
func rawZip(t *testing.T, files ...[2]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, f := range files {
		w, err := z.Create(f[0])
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(f[1]))
	}
	z.Close()
	return b.Bytes()
}

func TestReadRefuses(t *testing.T) {
	wav := string(tone(10))
	cases := map[string][]byte{
		"not a zip":   []byte("PK nope"),
		"no manifest": rawZip(t, [2]string{"audio/1.wav", wav}),
		"bad json":    rawZip(t, [2]string{ManifestName, "{"}),
		"newer format": rawZip(t, [2]string{ManifestName,
			`{"format":2,"language":"fr","words":[]}`}),
		"no format": rawZip(t, [2]string{ManifestName, `{"language":"fr","words":[]}`}),
		"missing clip": rawZip(t, [2]string{ManifestName,
			`{"format":1,"language":"fr","words":[{"text":"a","audio":"audio/1.wav"}]}`}),
		"path escape": rawZip(t,
			[2]string{ManifestName, `{"format":1,"language":"fr","words":[{"text":"a","audio":"../x.wav"}]}`},
			[2]string{"../x.wav", wav}),
		"bad clip": rawZip(t,
			[2]string{ManifestName, `{"format":1,"language":"fr","words":[{"text":"a","audio":"audio/1.wav"}]}`},
			[2]string{"audio/1.wav", "RIFFxxxxWAVE"}),
		"big clip": rawZip(t,
			[2]string{ManifestName, `{"format":1,"language":"fr","words":[{"text":"a","audio":"audio/1.wav"}]}`},
			[2]string{"audio/1.wav", wav + strings.Repeat("\x00", MaxClipBytes)}),
	}
	for name, data := range cases {
		if _, err := Read(data); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: err = %v, want ErrFormat", name, err)
		}
	}
}

func TestReadIgnoresUnknown(t *testing.T) {
	data := rawZip(t,
		[2]string{ManifestName, `{"format":1,"language":"fr","list":"L","version":1,"voice":"x",` +
			`"words":[{"text":"a","audio":"audio/1.wav","ipa":"a"}]}`},
		[2]string{"audio/1.wav", string(tone(10))},
		[2]string{"README.txt", "hi"})
	p, err := Read(data)
	if err != nil || len(p.Words) != 1 {
		t.Fatalf("Read = %+v, %v", p, err)
	}
}

func TestDecodeWAVSkipsChunks(t *testing.T) {
	w := tone(4)
	// Put an odd-sized LIST chunk (with its pad byte) before "data".
	extra := []byte("LIST\x03\x00\x00\x00abc\x00")
	withList := append(append(append([]byte{}, w[:36]...), extra...), w[36:]...)
	s, rate, err := DecodeWAV(withList)
	if err != nil || rate != 16000 || len(s) != 4 {
		t.Fatalf("DecodeWAV = %d samples at %d, %v", len(s), rate, err)
	}
}
