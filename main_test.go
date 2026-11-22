package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bl-bin")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "beads-lease")
	if out, err := exec.Command("go", "build", "-o", binPath, ".").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// sandbox is an isolated HOME plus a fake coord that logs every argv and
// answers read-only probes. Nothing here touches the real hub.
type sandbox struct {
	t     *testing.T
	home  string
	coord string
	log   string
}

func newSandbox(t *testing.T, coordBody string) *sandbox {
	t.Helper()
	home := t.TempDir()
	s := &sandbox{t: t, home: home, coord: filepath.Join(home, "coord"), log: filepath.Join(home, "coord.log")}
	if coordBody == "" {
		coordBody = `echo '[{"id":"x-1","title":"Resource lease: a","status":"open"}]'`
	}
	script := "#!/bin/sh\necho \"$@\" >> " + s.log + "\n" + coordBody + "\n"
	if err := os.WriteFile(s.coord, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *sandbox) run(extraEnv map[string]string, args ...string) (stdout, stderr string, code int) {
	s.t.Helper()
	cmd := exec.Command(binPath, args...)
	env := map[string]string{
		"HOME": s.home, "XDG_CONFIG_HOME": filepath.Join(s.home, ".config"), "PATH": "/usr/bin:/bin",
		"BEADS_LEASE_COORD": s.coord, "BEADS_ACTOR": "test-actor",
	}
	for k, v := range extraEnv {
		env[k] = v
	}
	for k, v := range env {
		if v != "" {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		s.t.Fatal(err)
	}
	return o.String(), e.String(), code
}

func (s *sandbox) coordCalls() []string {
	b, _ := os.ReadFile(s.log)
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func TestVersionTextUnchanged(t *testing.T) {
	out, _, code := newSandbox(t, "").run(nil, "version")
	want := "beads-lease 0.1.0 — the fleet's keeper of leases (resource claims over the Beads hub)\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestVersionJSON(t *testing.T) {
	out, _, code := newSandbox(t, "").run(nil, "version", "--json")
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil || code != 0 {
		t.Fatalf("code=%d err=%v out=%q", code, err, out)
	}
	if v["version"] != "0.1.0" {
		t.Errorf("version = %v", v["version"])
	}
	if n, ok := v["api_version"].(float64); !ok || n != 1 {
		t.Errorf("api_version = %v (want integer 1)", v["api_version"])
	}
}
