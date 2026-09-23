// Command sfxdump writes every sound effect to a WAV file, so they can be
// listened to and tuned without playing the game.
//
//	go run ./tools/sfxdump [-o dir]
package main

import (
	"encoding/binary"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/halpworld/halpwords/internal/audio"
)

func main() {
	out := flag.String("o", "dist/sounds", "folder to write the WAV files to")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for id := audio.ID(0); id < audio.Count; id++ {
		path := filepath.Join(*out, id.String()+".wav")
		if err := os.WriteFile(path, wav(audio.Render(audio.Sounds[id], audio.SampleRate)), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("wrote %d sounds to %s", audio.Count, *out)
}

// wav encodes mono samples as a 16-bit PCM WAV file.
func wav(samples []float32) []byte {
	data := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[2*i:], uint16(int16(s*32767)))
	}
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(data)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16) // format chunk size
	binary.LittleEndian.PutUint16(h[20:], 1)  // PCM
	binary.LittleEndian.PutUint16(h[22:], 1)  // mono
	binary.LittleEndian.PutUint32(h[24:], audio.SampleRate)
	binary.LittleEndian.PutUint32(h[28:], audio.SampleRate*2) // bytes per second
	binary.LittleEndian.PutUint16(h[32:], 2)                  // bytes per sample
	binary.LittleEndian.PutUint16(h[34:], 16)                 // bits per sample
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(data)))
	return append(h, data...)
}
