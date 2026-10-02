// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	c, err := Load(path, false)
	if err != nil || c.RefreshMS != 400 || c.Graphics != "auto" {
		t.Fatalf("defaults: %+v %v", c, err)
	}
	if _, err = Load(path, true); err == nil {
		t.Fatal("explicit missing config was ignored")
	}
	if err = os.WriteFile(path, []byte("refresh_ms = 10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(path, true); err == nil {
		t.Fatal("unsafe refresh frequency was accepted")
	}
	if err = os.WriteFile(path, []byte("graphics = 'sixel'\nrefresh_ms = 500\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err = Load(path, true)
	if err != nil || c.Graphics != "sixel" || c.RefreshMS != 500 || c.Zoom != 1 {
		t.Fatalf("config: %+v %v", c, err)
	}
}
