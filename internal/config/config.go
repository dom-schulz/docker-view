// Package config reads the optional ~/.config/dkrv/config.toml.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Volume choices for clean up.
const (
	VolumesAsk    = "ask"
	VolumesKeep   = "keep"
	VolumesRemove = "remove"
)

// Config is dkrv's settings.
type Config struct {
	Cleanup Cleanup `toml:"cleanup"`
}

// Cleanup configures the C key.
type Cleanup struct {
	// Volumes: "ask" offers y (down) / v (down -v) / n; "keep" always runs
	// down; "remove" always runs down -v.
	Volumes string `toml:"volumes"`
}

// Default returns the settings used when there is no config file.
func Default() Config {
	return Config{Cleanup: Cleanup{Volumes: VolumesAsk}}
}

// Path is ~/.config/dkrv/config.toml on both Linux and macOS
// (XDG_CONFIG_HOME is honoured when set).
func Path() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "dkrv", "config.toml")
}

// Load reads path. A missing file is not an error.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	return Parse(string(data))
}

// Parse decodes TOML on top of the defaults and validates it.
func Parse(data string) (Config, error) {
	cfg := Default()
	if _, err := toml.Decode(data, &cfg); err != nil {
		return Default(), err
	}
	switch cfg.Cleanup.Volumes {
	case "":
		cfg.Cleanup.Volumes = VolumesAsk
	case VolumesAsk, VolumesKeep, VolumesRemove:
	default:
		return Default(), fmt.Errorf("cleanup.volumes: %q is not one of ask, keep, remove", cfg.Cleanup.Volumes)
	}
	return cfg, nil
}
