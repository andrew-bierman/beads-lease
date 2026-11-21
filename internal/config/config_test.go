package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultsWithoutFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("BEADLE_CONFIG", "")
	t.Setenv("BEADS_LEASE_CONFIG", "")
	got := Load()
	if got.Name != "beads-lease" || got.ContentionExit != 2 || !got.LegacySurface {
		t.Fatalf("defaults not applied: %+v", got)
	}
}

func TestLoadOverlayFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"name":"leasehub","contentionExit":1,"legacySurface":false,"unknownKey":true}`), 0o600)
	got, warn := loadFrom(path)
	if got.Name != "leasehub" {
		t.Errorf("name not overridden: %+v", got)
	}
	if got.ContentionExit != 1 {
		t.Errorf("contentionExit not overridden: %+v", got)
	}
	if got.LegacySurface {
		t.Errorf("legacySurface not overridden: %+v", got)
	}
	if got.DefaultTTL != "15m" {
		t.Errorf("unset keys must keep defaults: %+v", got)
	}
	if !strings.Contains(warn, "unknownKey") {
		t.Errorf("unknown key must warn, got: %q", warn)
	}
}

func TestLoadInvalidJSONFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte("not json"), 0o600)
	got, warn := loadFrom(path)
	if got.Name != "beads-lease" {
		t.Errorf("invalid config must fall back to defaults: %+v", got)
	}
	if !strings.Contains(warn, "not valid JSON") {
		t.Errorf("must warn about invalid JSON, got: %q", warn)
	}
}

func TestResolveCoordPrecedence(t *testing.T) {
	t.Setenv("BEADS_LEASE_COORD", "")
	t.Setenv("BEADLE_COORD", "/env/beadle")
	t.Setenv("FLEET_LEASE_COORD", "/env/fleet")
	if got := Defaults().ResolveCoord(); got != "/env/beadle" {
		t.Errorf("BEADLE_COORD must win: %s", got)
	}
	os.Unsetenv("BEADLE_COORD")
	if got := Defaults().ResolveCoord(); got != "/env/fleet" {
		t.Errorf("FLEET_LEASE_COORD must be next: %s", got)
	}
	os.Unsetenv("FLEET_LEASE_COORD")
	t.Setenv("HOME", "/home/x")
	if got := Defaults().ResolveCoord(); !strings.HasSuffix(got, "/Code/agent-coordination/bin/coord") {
		t.Errorf("default coord path wrong: %s", got)
	}
}

func TestConfigPathMigration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BEADS_LEASE_CONFIG", "")
	t.Setenv("BEADLE_CONFIG", "")
	current := filepath.Join(home, ".config", "beads-lease", "config.json")
	legacy := filepath.Join(home, ".config", "beadle", "config.json")
	if got := Path(); got != current {
		t.Fatalf("new default: %s", got)
	}
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{"defaultTTL":"20m"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Path(); got != legacy {
		t.Fatalf("legacy file fallback: %s", got)
	}
	if err := os.MkdirAll(filepath.Dir(current), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Path(); got != current {
		t.Fatalf("new file must win: %s", got)
	}
	t.Setenv("BEADLE_CONFIG", "/legacy/config.json")
	if got := Path(); got != "/legacy/config.json" {
		t.Fatalf("legacy env: %s", got)
	}
	t.Setenv("BEADS_LEASE_CONFIG", "/new/config.json")
	if got := Path(); got != "/new/config.json" {
		t.Fatalf("new env must win: %s", got)
	}
}

func TestNewCoordEnvOverridesLegacy(t *testing.T) {
	t.Setenv("BEADS_LEASE_COORD", "/new/coord")
	t.Setenv("BEADLE_COORD", "/old/coord")
	t.Setenv("FLEET_LEASE_COORD", "/fleet/coord")
	if got := Defaults().ResolveCoord(); got != "/new/coord" {
		t.Fatalf("new env must win: %s", got)
	}
}
