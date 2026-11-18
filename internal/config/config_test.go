package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsWhenMissing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil || cfg.Cleanup.Volumes != VolumesAsk {
		t.Fatalf("cfg = %+v, err = %v", cfg, err)
	}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]string{
		"":                                  VolumesAsk,
		"[cleanup]\nvolumes = \"keep\"\n":   VolumesKeep,
		"[cleanup]\nvolumes = \"remove\"\n": VolumesRemove,
		"cleanup.volumes = \"ask\"\n":       VolumesAsk,
	} {
		cfg, err := Parse(in)
		if err != nil || cfg.Cleanup.Volumes != want {
			t.Errorf("Parse(%q) = %+v, %v; want %s", in, cfg, err, want)
		}
	}
}

func TestParseRejectsUnknown(t *testing.T) {
	if _, err := Parse("[cleanup]\nvolumes = \"sometimes\"\n"); err == nil {
		t.Fatal("want an error for an unknown value")
	}
	if _, err := Parse("not toml ="); err == nil {
		t.Fatal("want an error for bad TOML")
	}
}

func TestPathHonoursXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if got, want := Path(), filepath.Join(dir, "dkrv", "config.toml"); got != want {
		t.Errorf("Path() = %s, want %s", got, want)
	}
	os.MkdirAll(filepath.Join(dir, "dkrv"), 0o755)
	os.WriteFile(Path(), []byte("[cleanup]\nvolumes = \"remove\"\n"), 0o644)
	if cfg, err := Load(Path()); err != nil || cfg.Cleanup.Volumes != VolumesRemove {
		t.Errorf("Load = %+v, %v", cfg, err)
	}
}
