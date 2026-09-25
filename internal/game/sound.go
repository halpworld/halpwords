package game

import (
	"bytes"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/ebitengine/oto/v3"

	"github.com/halpworld/halpwords/internal/audio"
)

// maxVoices is how many sounds can play at once. Starting another stops
// the oldest.
const maxVoices = 8

// masterVolume keeps the effects comfortable next to other programs.
const masterVolume = 0.5

// musicVolume is the music's volume at full, under the effects so words
// and warnings stand out.
const musicVolume = 0.35

// Sound plays sound effects. It talks to the audio device directly rather
// than through Ebitengine's audio package, so a computer without sound
// (a school PC with no speakers, a VM) just runs silently instead of
// stopping the game with an error.
type Sound struct {
	Muted bool
	// Effects and Music are the volumes, 0 to 1.
	Effects, Music float64

	ctx    *oto.Context
	ready  chan struct{}
	failed bool
	pcm    [audio.Count][]byte // rendered on first use
	voices []*oto.Player
	played [audio.Count]uint64 // tick each sound last started
	later  []delayed
	tick   uint64
	music  music
}

type delayed struct {
	id audio.ID
	at uint64
}

// newSound opens the audio device. HALPWORDS_SOUND=off skips it entirely.
func newSound() *Sound {
	s := &Sound{Effects: 1, Music: 1}
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
	// Let finished voices go, and make room for the new one. Since oto
	// v3.4 a player is freed once nothing refers to it, so a voice still
	// playing is paused before it is dropped.
	live := s.voices[:0]
	for _, v := range s.voices {
		if v.IsPlaying() {
			live = append(live, v)
		}
	}
	s.voices = live
	if len(s.voices) >= maxVoices {
		s.voices[0].Pause()
		s.voices = s.voices[1:]
	}
	p := s.ctx.NewPlayer(bytes.NewReader(s.pcm[id]))
	p.SetVolume(masterVolume * s.Effects)
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
	s.music.volume(s)
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
	if s.usable() {
		s.music.update(s)
	}
}

// PlayMusic asks for a track. The music playing fades out and the new
// track fades in, once it has been rendered. Asking for the track that is
// already playing does nothing, so scenes can ask every tick.
func (s *Sound) PlayMusic(t audio.Track) { s.music.want = t }

// musicFade is how many ticks the music takes to fade out or in.
const musicFade = 30

// musicCache is how many rendered tracks are kept, so going from a battle
// back to exploring doesn't render the floor's music again.
const musicCache = 4

// music plays one looping track at a time.
type music struct {
	want    audio.Track
	playing audio.Track
	p       *oto.Player
	fade    float64 // 0 silent to 1 full

	cache   map[audio.Track][]byte
	order   []audio.Track // oldest first
	working map[audio.Track]bool
	done    chan rendered
}

type rendered struct {
	t   audio.Track
	pcm []byte
}

func (m *music) update(s *Sound) {
	if m.done == nil {
		m.done = make(chan rendered, musicCache)
		m.cache = map[audio.Track][]byte{}
		m.working = map[audio.Track]bool{}
	}
	for len(m.done) > 0 {
		m.store(<-m.done)
	}
	if _, ok := m.cache[m.want]; !ok && m.want.Mood != audio.Quiet {
		m.render(m.want) // start now, while the old track fades
	}
	if m.p != nil && m.playing != m.want {
		// Fade out what is playing, then let it go.
		m.fade -= 1.0 / musicFade
		if m.fade > 0 {
			m.volume(s)
			return
		}
		m.p.Pause()
		m.p, m.fade = nil, 0
	}
	if m.p == nil && m.want.Mood != audio.Quiet {
		pcm, ok := m.cache[m.want]
		if !ok || len(pcm) == 0 {
			return
		}
		m.p = s.ctx.NewPlayer(&loop{pcm: pcm})
		m.playing, m.fade = m.want, 0
		m.volume(s)
		m.p.Play()
	}
	if m.p != nil && m.fade < 1 {
		m.fade = min(1, m.fade+1.0/musicFade)
		m.volume(s)
	}
}

// volume sets the player's volume from the fade and the settings.
func (m *music) volume(s *Sound) {
	if m.p == nil {
		return
	}
	v := musicVolume * s.Music * m.fade
	if s.Muted {
		v = 0
	}
	m.p.SetVolume(v)
}

// render composes and renders a track in the background.
func (m *music) render(t audio.Track) {
	if m.working[t] {
		return
	}
	m.working[t] = true
	go func() {
		// A web browser runs one thing at a time: pause now and then so
		// the game keeps drawing while the music is made.
		var yield func()
		if runtime.GOOS == "js" {
			yield = func() { time.Sleep(time.Millisecond) }
		}
		pcm := audio.Encode(audio.RenderLoop(audio.Compose(t), audio.SampleRate, yield))
		m.done <- rendered{t, pcm}
	}()
}

// store keeps a rendered track, dropping the oldest when the cache is full.
func (m *music) store(r rendered) {
	delete(m.working, r.t)
	if _, ok := m.cache[r.t]; ok {
		return
	}
	for len(m.order) >= musicCache {
		old := m.order[0]
		m.order = m.order[1:]
		delete(m.cache, old)
	}
	m.cache[r.t] = r.pcm
	m.order = append(m.order, r.t)
}

// loop reads the same samples over and over.
type loop struct {
	pcm []byte
	at  int
}

func (l *loop) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		c := copy(p[n:], l.pcm[l.at:])
		n += c
		l.at = (l.at + c) % len(l.pcm)
	}
	return n, nil
}
