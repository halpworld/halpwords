//go:build !js

package profile

import (
	"testing"

	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

func useTempDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
}

func TestProfileRoundTrip(t *testing.T) {
	useTempDir(t)
	fr, _ := words.Lookup("fr")
	p, errs := Load()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if got := p.Settings.For(fr); got != Preset(fr) {
		t.Fatalf("new settings %+v", got)
	}
	ls := Preset(fr)
	ls.Rules.Accents, ls.Rules.ArticlesRequired, ls.Timer, ls.Highlight = words.Strict, true, Relaxed, true
	p.Settings.Set(fr, ls)
	if p.Settings.Options() != DefaultOptions() {
		t.Fatalf("new game settings %+v", p.Settings.Options())
	}
	opts := Options{Music: 2, Effects: 10, CRT: CRTStrong, Fullscreen: true}
	p.Settings.SetOptions(opts)
	e := words.Entry{Prompt: "dog", Answers: []string{"le chien"}}
	p.MemoryFor("fr").Record(e, words.Answer{Tier: words.Perfect})
	p.Name = "Ada"
	p.Fame.Add(compete.TableKey(compete.Hardcore, "fr"), compete.Fame{Name: "Ada", Score: 1234})
	for _, err := range []error{p.SaveSettings(), p.SaveMemory(), p.SaveFame()} {
		if err != nil {
			t.Fatal(err)
		}
	}

	q, errs := Load()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if q.Settings.For(fr) != ls {
		t.Fatalf("settings %+v, want %+v", q.Settings.For(fr), ls)
	}
	if q.Settings.Options() != opts {
		t.Fatalf("game settings %+v, want %+v", q.Settings.Options(), opts)
	}
	if q.MemoryFor("fr").Box(e) != 1 || q.Name != "Ada" || q.Fame.Best(compete.TableKey(compete.Hardcore, "fr")) != 1234 {
		t.Fatal("the profile did not come back")
	}
}

func TestOptionsAreKeptInRange(t *testing.T) {
	var s Settings
	s.SetOptions(Options{Music: 99, Effects: -3, CRT: 7})
	if o := s.Options(); o.Music != MaxVolume || o.Effects != 0 || o.CRT >= numCRT {
		t.Fatalf("options %+v", o)
	}
}

func TestDamagedFileIsKept(t *testing.T) {
	useTempDir(t)
	save.Write(memoryFile, []byte("{not json"))
	p, errs := Load()
	if len(errs) != 1 || p == nil {
		t.Fatalf("errors %v", errs)
	}
	if data, err := save.Read(memoryFile + ".bad"); err != nil || string(data) != "{not json" {
		t.Fatal("the damaged file was not kept")
	}
	p.MemoryFor("la") // still usable
}

func TestInMemoryProfileNeverWrites(t *testing.T) {
	useTempDir(t)
	p := New()
	p.Name = "x"
	if err := p.SaveFame(); err != nil {
		t.Fatal(err)
	}
	if _, err := save.Read(fameFile); err == nil {
		t.Fatal("an in-memory profile wrote a file")
	}
}

func TestLockedSettings(t *testing.T) {
	useTempDir(t)
	fr, _ := words.Lookup("fr")
	p, _ := Load()
	p.disk = true
	own := Preset(fr)
	own.Highlight = true
	p.Settings.Set(fr, own)
	locked := Preset(fr)
	locked.Timer = Relaxed
	p.Settings.Locked = map[string]LangSettings{"fr": locked}
	if p.Settings.For(fr) != locked || !p.Settings.IsLocked(fr) || p.Settings.Own(fr) != own {
		t.Fatal("the lock doesn't apply")
	}
	if err := p.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	q, _ := Load()
	if q.Settings.IsLocked(fr) || q.Settings.For(fr) != own {
		t.Fatal("the lock was saved")
	}
}
