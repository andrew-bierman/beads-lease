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
	got := Load()
	if got.Name != "beadle" || got.ContentionExit != 2 || !got.LegacySurface {
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
	if got.Name != "beadle" {
		t.Errorf("invalid config must fall back to defaults: %+v", got)
	}
	if !strings.Contains(warn, "not valid JSON") {
		t.Errorf("must warn about invalid JSON, got: %q", warn)
	}
}

func TestResolveCoordPrecedence(t *testing.T) {
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
