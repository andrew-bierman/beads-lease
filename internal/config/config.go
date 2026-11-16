// Package config carries everything about beadle that might be renamed or
// retuned: its display identity, where the hub lives, registry conventions,
// and policy knobs. Missing file = documented defaults; no config is required.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config is the on-disk (optional) configuration. Zero values are replaced
// by Defaults(); consumers never see unset fields.
type Config struct {
	Name              string `json:"name"`              // display name (help, version); the binary name itself comes from argv[0]
	Tagline           string `json:"tagline"`           // one-line description shown in help/version
	CoordPath         string `json:"coordPath"`         // hub wrapper (bin/coord); env BEADLE_COORD / FLEET_LEASE_COORD win
	ActorEnv          string `json:"actorEnv"`          // identity env var; BEADS_ACTOR is always accepted too
	ResourcePrefix    string `json:"resourcePrefix"`    // canonical resource id prefix
	TitlePrefix       string `json:"titlePrefix"`       // registry task title prefix
	DefaultTTL        string `json:"defaultTTL"`        // advisory hold time when -t is not given
	ContentionExit    int    `json:"contentionExit"`    // exit code for already-claimed / not-yours
	LegacySurface     bool   `json:"legacySurface"`     // bare-resource claim + --status/--release flags
	LowPriorityLinux  string `json:"lowPriorityLinux"`  // --low-priority prefix command (Linux)
	LowPriorityDarwin string `json:"lowPriorityDarwin"` // --low-priority prefix command (macOS)
	ProtocolDoc       string `json:"protocolDoc"`       // where the protocol lives (referenced in descriptions)
}

// Defaults returns the shipped configuration.
func Defaults() *Config {
	return &Config{
		Name:              "beadle",
		Tagline:           "the fleet's keeper of leases (resource claims over the Beads hub)",
		CoordPath:         "",
		ActorEnv:          "BEADS_ACTOR",
		ResourcePrefix:    "resource:",
		TitlePrefix:       "Resource lease: ",
		DefaultTTL:        "15m",
		ContentionExit:    2,
		LegacySurface:     true,
		LowPriorityLinux:  "nice -n 10",
		LowPriorityDarwin: "taskpolicy -c utility",
		ProtocolDoc:       "dotfiles docs/guides/agent-resource-protocol.md",
	}
}

var (
	once     sync.Once
	loaded   *Config
	loadWarn string
)

// Path returns the config file location: $BEADLE_CONFIG, else
// ~/.config/beadle/config.json. The location does not move when the tool is
// renamed — the file's `name` field carries the identity instead.
func Path() string {
	if p := os.Getenv("BEADLE_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "beadle", "config.json")
}

// Load reads the config once per process. A missing file is fine (defaults);
// an unreadable or invalid file warns on stderr and falls back to defaults —
// a broken config must not break the tool.
func Load() *Config {
	once.Do(func() {
		loaded, loadWarn = loadFrom(Path())
		if loadWarn != "" {
			fmt.Fprintln(os.Stderr, "beadle: "+loadWarn)
		}
	})
	return loaded
}

// loadFrom reads and overlays one config file. Warn is non-empty when the
// file was invalid or carried unknown keys.
func loadFrom(path string) (*Config, string) {
	cfg := Defaults()
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, "" // no config: defaults
	}
	var overrides map[string]any
	if err := json.Unmarshal(raw, &overrides); err != nil {
		return cfg, fmt.Sprintf("config %s is not valid JSON (%v); using defaults", path, err)
	}
	// Overlay only recognized keys so a typo cannot silently rename
	// behavior; unknown keys warn.
	known := map[string]struct{}{}
	for _, k := range []string{"name", "tagline", "coordPath", "actorEnv", "resourcePrefix",
		"titlePrefix", "defaultTTL", "contentionExit", "legacySurface",
		"lowPriorityLinux", "lowPriorityDarwin", "protocolDoc"} {
		known[k] = struct{}{}
	}
	warn := ""
	for k := range overrides {
		if _, ok := known[k]; !ok {
			warn = fmt.Sprintf("config %s: unknown key %q ignored", path, k)
		}
	}
	if s, ok := overrides["name"].(string); ok && strings.TrimSpace(s) != "" {
		cfg.Name = s
	}
	if s, ok := overrides["tagline"].(string); ok {
		cfg.Tagline = s
	}
	if s, ok := overrides["coordPath"].(string); ok {
		cfg.CoordPath = s
	}
	if s, ok := overrides["actorEnv"].(string); ok && strings.TrimSpace(s) != "" {
		cfg.ActorEnv = s
	}
	if s, ok := overrides["resourcePrefix"].(string); ok && s != "" {
		cfg.ResourcePrefix = s
	}
	if s, ok := overrides["titlePrefix"].(string); ok && s != "" {
		cfg.TitlePrefix = s
	}
	if s, ok := overrides["defaultTTL"].(string); ok && s != "" {
		cfg.DefaultTTL = s
	}
	if f, ok := overrides["contentionExit"].(float64); ok && f >= 0 && f <= 125 {
		cfg.ContentionExit = int(f)
	}
	if b, ok := overrides["legacySurface"].(bool); ok {
		cfg.LegacySurface = b
	}
	if s, ok := overrides["lowPriorityLinux"].(string); ok {
		cfg.LowPriorityLinux = s
	}
	if s, ok := overrides["lowPriorityDarwin"].(string); ok {
		cfg.LowPriorityDarwin = s
	}
	if s, ok := overrides["protocolDoc"].(string); ok {
		cfg.ProtocolDoc = s
	}
	return cfg, warn
}

// ExpandHome resolves a leading ~ to HOME (config paths may use it).
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	return p
}

// ResolveCoord returns the coordination hub wrapper path. Precedence:
// env BEADLE_COORD, env FLEET_LEASE_COORD (historic), config coordPath,
// then the default hub location.
func (c *Config) ResolveCoord() string {
	if p := os.Getenv("BEADLE_COORD"); p != "" {
		return p
	}
	if p := os.Getenv("FLEET_LEASE_COORD"); p != "" {
		return p
	}
	if c.CoordPath != "" {
		return ExpandHome(c.CoordPath)
	}
	return filepath.Join(os.Getenv("HOME"), "Code", "agent-coordination", "bin", "coord")
}

// Actor returns the claiming identity from the configured identity env var,
// falling back to BEADS_ACTOR (the hub-wide contract) when they differ.
func (c *Config) Actor() string {
	for _, env := range []string{c.ActorEnv, "BEADS_ACTOR"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v
		}
	}
	return ""
}
