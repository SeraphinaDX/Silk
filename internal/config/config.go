// SPDX-License-Identifier: GPL-3.0-or-later

// Package config loads Silk's optional TOML settings. No config is required.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Home       string  `toml:"home"`
	Chrome     string  `toml:"chrome"`
	Graphics   string  `toml:"graphics"`
	CellWidth  int     `toml:"cell_width"`
	CellHeight int     `toml:"cell_height"`
	RefreshMS  int     `toml:"refresh_ms"`
	Zoom       float64 `toml:"zoom"`
	Profile    string  `toml:"profile"`
	NoSandbox  bool    `toml:"no_sandbox"`
	ImageRows  int     `toml:"image_max_rows"`
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(dir, "silk", "config.toml")
}

func Load(path string, explicit bool) (Config, error) {
	c := Config{Home: "https://example.org", Graphics: "auto", CellWidth: 8, CellHeight: 16, RefreshMS: 400, Zoom: 1, ImageRows: 10}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) && !explicit {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("config: %w", err)
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if c.Graphics != "auto" && c.Graphics != "sixel" && c.Graphics != "halfblock" && c.Graphics != "none" {
		return fmt.Errorf("graphics must be auto, sixel, halfblock, or none")
	}
	if c.CellWidth < 1 || c.CellWidth > 64 || c.CellHeight < 1 || c.CellHeight > 128 {
		return fmt.Errorf("cell dimensions must be 1..64 by 1..128")
	}
	if c.RefreshMS < 100 || c.RefreshMS > 10000 {
		return fmt.Errorf("refresh_ms must be 100..10000")
	}
	if c.Zoom < .25 || c.Zoom > 3 {
		return fmt.Errorf("zoom must be 0.25..3")
	}
	if c.ImageRows < 1 || c.ImageRows > 40 {
		return fmt.Errorf("image_max_rows must be 1..40")
	}
	return nil
}
