package profile

import (
	"testing"

	"github.com/halpworld/halpwords/internal/save"
)

func TestQACalmRoundTripAndUnknownField(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	f := save.Current()

	p, _ := LoadFrom(f)
	o := p.Settings.Options()
	o.Calm = true
	p.Settings.SetOptions(o)
	if err := p.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	q, errs := LoadFrom(f)
	if len(errs) > 0 || !q.Settings.Options().Calm {
		t.Fatalf("calm lost: %v", errs)
	}

	// A file from a newer version, with fields this one doesn't know.
	in := `{"Game":{"Music":6,"Calm":true,"FutureThing":{"x":1}},"Brand":"new"}`
	if err := f.Write("settings.json", []byte(in)); err != nil {
		t.Fatal(err)
	}
	r, errs := LoadFrom(f)
	if len(errs) > 0 {
		t.Fatalf("unknown field broke load: %v", errs)
	}
	if !r.Settings.Options().Calm {
		t.Error("calm lost next to unknown field")
	}
}
