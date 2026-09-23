package game

import (
	"bytes"
	"log"
	"os"

	"github.com/ebitengine/oto/v3"

	"github.com/halpworld/halpwords/internal/audio"
)

// maxVoices is how many sounds can play at once. Starting another stops
// the oldest.
const maxVoices = 8

// masterVolume keeps the effects comfortable next to other programs.
const masterVolume = 0.5

// Sound plays sound effects. It talks to the audio device directly rather
// than through Ebitengine's audio package, so a computer without sound
// (a school PC with no speakers, a VM) just runs silently instead of
// stopping the game with an error.
type Sound struct {
	Muted bool

	ctx    *oto.Context
	ready  chan struct{}
	failed bool
	pcm    [audio.Count][]byte // rendered on first use
	voices []*oto.Player
	played [audio.Count]uint64 // tick each sound last started
	later  []delayed
	tick   uint64
}

type delayed struct {
	id audio.ID
	at uint64
}

// newSound opens the audio device. HALPWORDS_SOUND=off skips it entirely.
func newSound() *Sound {
	s := &Sound{}
	if os.Getenv("HALPWORDS_SOUND") == "off" {
		s.failed = true
		return s
	}
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   audio.SampleRate,
		ChannelCount: 1,
		Format:       oto.FormatFloat32LE,
	})
	if err != nil {
		log.Printf("sound is off: %v", err)
		s.failed = true
		return s
	}
	s.ctx, s.ready = ctx, ready
	return s
}

// usable reports whether sounds can play now. In a web browser the device
// only starts after the first key press.
func (s *Sound) usable() bool {
	if s.failed {
		return false
	}
	select {
	case <-s.ready:
	default:
		return false
	}
	if err := s.ctx.Err(); err != nil {
		log.Printf("sound is off: %v", err)
		s.failed = true
		s.voices = nil
		return false
	}
	return true
}

// Play starts a sound effect. The same sound is not started twice in one
// tick, so typing several letters at once only clicks once.
func (s *Sound) Play(id audio.ID) {
	if s.Muted || !s.usable() || s.played[id] == s.tick {
		return
	}
	s.played[id] = s.tick
	if s.pcm[id] == nil {
		s.pcm[id] = audio.Encode(audio.Render(audio.Sounds[id], audio.SampleRate))
	}
	// Let finished voices go, and make room for the new one.
	live := s.voices[:0]
	for _, v := range s.voices {
		if v.IsPlaying() {
			live = append(live, v)
		} else {
			v.Close()
		}
	}
	s.voices = live
	if len(s.voices) >= maxVoices {
		s.voices[0].Close()
		s.voices = s.voices[1:]
	}
	p := s.ctx.NewPlayer(bytes.NewReader(s.pcm[id]))
	p.SetVolume(masterVolume)
	p.Play()
	s.voices = append(s.voices, p)
}

// PlayLater starts a sound after ticks updates, to follow another one.
func (s *Sound) PlayLater(id audio.ID, ticks int) {
	s.later = append(s.later, delayed{id, s.tick + uint64(ticks)})
}

// Toggle mutes or unmutes, and reports whether sound is now on.
func (s *Sound) Toggle() bool {
	s.Muted = !s.Muted
	if s.Muted {
		for _, v := range s.voices {
			v.Pause()
		}
		s.later = s.later[:0]
	}
	return !s.Muted
}

// Available reports whether there is an audio device to play on.
func (s *Sound) Available() bool { return !s.failed }

// update runs once per tick, before the scenes, and starts delayed sounds.
func (s *Sound) update(tick uint64) {
	s.tick = tick
	due := s.later[:0]
	var now []audio.ID
	for _, d := range s.later {
		if d.at <= tick {
			now = append(now, d.id)
		} else {
			due = append(due, d)
		}
	}
	s.later = due
	for _, id := range now {
		s.Play(id)
	}
}
