// Package audiopack is the format of a word list's pronunciation audio
// (halpwords-server W4.7): a linked game on a paid plan downloads one
// pack for each assigned list and plays a word after a miss.
//
// A pack is a zip file. Its first file is pack.json, the manifest:
//
//	{"format": 1, "language": "fr", "list": "<list id>", "version": 3,
//	 "words": [{"text": "la maison", "sound_out": "lah may-ZON", "audio": "audio/1.wav"}]}
//
// and then one WAV file for each word (audio/<n>.wav, from 1): 16-bit
// PCM, mono, 8,000 to 48,000 samples a second. text is what is spoken,
// the list's first answer for the word; sound_out is an optional
// plain-English way to say it. Readers ignore fields and files they
// don't know; a pack with a format above Format is refused.
package audiopack

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Format is the pack format this package writes and the newest it reads.
const Format = 1

// MediaType is a pack's Content-Type.
const MediaType = "application/vnd.halpwords.audiopack+zip"

// ManifestName is the manifest's file name in the zip.
const ManifestName = "pack.json"

// Limits. A reader refuses a pack over any of them.
const (
	// MaxWords is the most words a pack holds.
	MaxWords = 1000
	// MaxClipBytes is the largest WAV file for one word (about 30
	// seconds at 16,000 samples a second).
	MaxClipBytes = 1 << 20
	// MaxBytes is the largest pack, zipped or not.
	MaxBytes = 48 << 20
	// MaxTextChars is the longest text or sound-out.
	MaxTextChars = 200
	// MinRate and MaxRate are the sample rates allowed.
	MinRate, MaxRate = 8000, 48000
)

// ErrFormat is a pack this package can't read: not a zip, a newer
// format, a missing or bad file, or over a limit.
var ErrFormat = errors.New("audiopack: not a pack this game can read")

// Pack is one list's pronunciation audio.
type Pack struct {
	Language string
	List     string
	Version  int
	Words    []Word
}

// Word is one word's audio.
type Word struct {
	// Text is what is spoken: the list's first answer for the word.
	Text string
	// SoundOut is a plain-English way to say it ("lah may-ZON"), or "".
	SoundOut string
	// Audio is a WAV file: 16-bit PCM, mono.
	Audio []byte
}

// Key is a word's identity in a pack for a language: its text in
// Unicode NFC, with spaces squeezed and in lower case, so "La  maison"
// finds "la maison". Two words with the same key are the same word.
func Key(lang, text string) string {
	t := strings.ToLower(strings.Join(strings.Fields(norm.NFC.String(text)), " "))
	return lang + "\x00" + t
}

// Find returns the word whose text has the key of text, if the pack has
// it.
func (p *Pack) Find(text string) (Word, bool) {
	if p == nil {
		return Word{}, false
	}
	k := Key(p.Language, text)
	for _, w := range p.Words {
		if Key(p.Language, w.Text) == k {
			return w, true
		}
	}
	return Word{}, false
}

type manifest struct {
	Format   int         `json:"format"`
	Language string      `json:"language"`
	List     string      `json:"list"`
	Version  int         `json:"version"`
	Words    []entryJSON `json:"words"`
}

type entryJSON struct {
	Text     string `json:"text"`
	SoundOut string `json:"sound_out,omitempty"`
	Audio    string `json:"audio"`
}

// Check reports what is wrong with a pack, or nil: every limit and every
// clip's WAV format, as Read checks them.
func (p *Pack) Check() error {
	if p.Language == "" || !cleanText(p.Language, 16) {
		return fmt.Errorf("%w: no language", ErrFormat)
	}
	if len(p.Words) > MaxWords {
		return fmt.Errorf("%w: %d words, more than %d", ErrFormat, len(p.Words), MaxWords)
	}
	seen := map[string]bool{}
	total := 0
	for i, w := range p.Words {
		if w.Text == "" || !cleanText(w.Text, MaxTextChars) || !cleanText(w.SoundOut, MaxTextChars) {
			return fmt.Errorf("%w: word %d has no text, or a bad text or sound-out", ErrFormat, i+1)
		}
		k := Key(p.Language, w.Text)
		if seen[k] {
			return fmt.Errorf("%w: word %d is twice", ErrFormat, i+1)
		}
		seen[k] = true
		if len(w.Audio) > MaxClipBytes {
			return fmt.Errorf("%w: word %d's audio is over %d bytes", ErrFormat, i+1, MaxClipBytes)
		}
		if _, _, err := DecodeWAV(w.Audio); err != nil {
			return fmt.Errorf("word %d: %w", i+1, err)
		}
		total += len(w.Audio)
	}
	if total > MaxBytes {
		return fmt.Errorf("%w: over %d bytes", ErrFormat, MaxBytes)
	}
	return nil
}

// cleanText reports whether s is short, valid UTF-8 with no control
// characters.
func cleanText(s string, n int) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > n {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Write writes a pack. It checks it first (Check). The same pack is
// always written as the same bytes.
func Write(w io.Writer, p *Pack) error {
	if err := p.Check(); err != nil {
		return err
	}
	m := manifest{Format: Format, Language: p.Language, List: p.List, Version: p.Version, Words: []entryJSON{}}
	for i, word := range p.Words {
		m.Words = append(m.Words, entryJSON{Text: word.Text, SoundOut: word.SoundOut, Audio: fmt.Sprintf("audio/%d.wav", i+1)})
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	z := zip.NewWriter(w)
	put := func(name string, method uint16, b []byte) error {
		f, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		return err
	}
	if err := put(ManifestName, zip.Deflate, data); err != nil {
		return err
	}
	for i, word := range p.Words {
		if err := put(m.Words[i].Audio, zip.Deflate, word.Audio); err != nil {
			return err
		}
	}
	return z.Close()
}

// Encode returns a pack's bytes (Write).
func Encode(p *Pack) ([]byte, error) {
	var b bytes.Buffer
	if err := Write(&b, p); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

var audioName = regexp.MustCompile(`^audio/[1-9][0-9]{0,5}\.wav$`)

// Read reads a pack and checks it (Check). Anything wrong is ErrFormat.
func Read(data []byte) (*Pack, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("%w: over %d bytes", ErrFormat, MaxBytes)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		files[f.Name] = f
	}
	read := func(name string, limit int) ([]byte, error) {
		f := files[name]
		if f == nil {
			return nil, fmt.Errorf("%w: no %s", ErrFormat, name)
		}
		if f.UncompressedSize64 > uint64(limit) {
			return nil, fmt.Errorf("%w: %s is over %d bytes", ErrFormat, name, limit)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrFormat, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, int64(limit)+1))
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrFormat, name, err)
		}
		if len(b) > limit {
			return nil, fmt.Errorf("%w: %s is over %d bytes", ErrFormat, name, limit)
		}
		return b, nil
	}
	data, err = read(ManifestName, 1<<20)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrFormat, ManifestName, err)
	}
	if m.Format < 1 || m.Format > Format {
		return nil, fmt.Errorf("%w: format %d", ErrFormat, m.Format)
	}
	if len(m.Words) > MaxWords {
		return nil, fmt.Errorf("%w: %d words, more than %d", ErrFormat, len(m.Words), MaxWords)
	}
	p := &Pack{Language: m.Language, List: m.List, Version: m.Version}
	total := 0
	for _, e := range m.Words {
		if !audioName.MatchString(e.Audio) {
			return nil, fmt.Errorf("%w: bad audio file name %q", ErrFormat, e.Audio)
		}
		clip, err := read(e.Audio, MaxClipBytes)
		if err != nil {
			return nil, err
		}
		if total += len(clip); total > MaxBytes {
			return nil, fmt.Errorf("%w: over %d bytes", ErrFormat, MaxBytes)
		}
		p.Words = append(p.Words, Word{Text: e.Text, SoundOut: e.SoundOut, Audio: clip})
	}
	if err := p.Check(); err != nil {
		return nil, err
	}
	return p, nil
}

// EncodeWAV makes a WAV file of 16-bit mono samples at rate.
func EncodeWAV(samples []int16, rate int) []byte {
	var b bytes.Buffer
	n := 2 * len(samples)
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+n))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&b, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&b, binary.LittleEndian, uint32(rate))
	binary.Write(&b, binary.LittleEndian, uint32(rate*2))
	binary.Write(&b, binary.LittleEndian, uint16(2))
	binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(n))
	binary.Write(&b, binary.LittleEndian, samples)
	return b.Bytes()
}

// DecodeWAV reads a WAV file of 16-bit PCM mono samples, and returns them
// from -1 to 1, with the sample rate. Other WAV files are ErrFormat.
func DecodeWAV(data []byte) ([]float32, int, error) {
	bad := func(why string) ([]float32, int, error) {
		return nil, 0, fmt.Errorf("%w: WAV: %s", ErrFormat, why)
	}
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return bad("not a WAV file")
	}
	rest := data[12:]
	rate := 0
	var pcm []byte
	for len(rest) >= 8 {
		id, size := string(rest[:4]), binary.LittleEndian.Uint32(rest[4:8])
		rest = rest[8:]
		if uint64(size) > uint64(len(rest)) {
			return bad("a chunk runs past the end")
		}
		body := rest[:size]
		switch id {
		case "fmt ":
			if size < 16 {
				return bad("short fmt chunk")
			}
			format, channels := binary.LittleEndian.Uint16(body[0:2]), binary.LittleEndian.Uint16(body[2:4])
			bits := binary.LittleEndian.Uint16(body[14:16])
			if format != 1 || channels != 1 || bits != 16 {
				return bad("not 16-bit PCM mono")
			}
			rate = int(binary.LittleEndian.Uint32(body[4:8]))
		case "data":
			pcm = body
		}
		rest = rest[size:]
		if size%2 == 1 && len(rest) > 0 {
			rest = rest[1:] // chunks are padded to an even size
		}
	}
	if rate < MinRate || rate > MaxRate {
		return bad(fmt.Sprintf("sample rate %d", rate))
	}
	if len(pcm) == 0 {
		return bad("no samples")
	}
	out := make([]float32, len(pcm)/2)
	for i := range out {
		out[i] = float32(int16(binary.LittleEndian.Uint16(pcm[2*i:]))) / 32768
	}
	return out, rate, nil
}
