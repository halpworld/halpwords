// Command icon draws the game's icon and writes it in the formats the
// release builds need: PNGs, a macOS .icns and a Windows .ico.
//
//	go run ./tools/icon [-o dir]
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/halpworld/halpwords/internal/proc"
)

func main() {
	out := flag.String("o", "dist/icon", "folder to write the icon files to")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	pngs := map[int][]byte{}
	for _, size := range []int{16, 32, 48, 64, 128, 256, 512, 1024} {
		pngs[size] = encode(proc.IconAt(size))
	}
	write := func(name string, data []byte) {
		if err := os.WriteFile(filepath.Join(*out, name), data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	write("halpwords.png", pngs[1024])
	write("halpwords-256.png", pngs[256])
	write("favicon.png", pngs[64])
	write("halpwords.icns", icns(pngs))
	write("halpwords.ico", ico(pngs, 16, 32, 48, 64, 128, 256))
	log.Printf("wrote the icon to %s", *out)
}

func encode(img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		log.Fatal(err)
	}
	return b.Bytes()
}

// icns packs PNGs into a macOS icon file: a header, then a typed chunk
// for each size.
func icns(pngs map[int][]byte) []byte {
	var body bytes.Buffer
	for _, e := range []struct {
		kind string
		size int
	}{
		{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256},
		{"ic09", 512}, {"ic10", 1024}, {"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512},
	} {
		// ic11 to ic14 are the @2x sizes: twice the pixels of their names.
		data := pngs[e.size]
		if e.kind >= "ic11" {
			data = pngs[e.size*2]
		}
		body.WriteString(e.kind)
		binary.Write(&body, binary.BigEndian, uint32(8+len(data)))
		body.Write(data)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

// ico packs PNGs into a Windows icon file: a directory of sizes, then the
// images.
func ico(pngs map[int][]byte, sizes ...int) []byte {
	var dir, data bytes.Buffer
	le := func(b *bytes.Buffer, v any) { binary.Write(b, binary.LittleEndian, v) }
	le(&dir, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for _, s := range sizes {
		p := pngs[s]
		wh := uint8(s)
		if s >= 256 {
			wh = 0 // 0 means 256
		}
		le(&dir, [4]uint8{wh, wh, 0, 0})
		le(&dir, [2]uint16{1, 32}) // colour planes, bits per pixel
		le(&dir, [2]uint32{uint32(len(p)), uint32(offset + data.Len())})
		data.Write(p)
	}
	return append(dir.Bytes(), data.Bytes()...)
}
