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

type check struct {
	Name, Status, Detail, Fix string
}

func doctor(t *testing.T, s *sandbox, env map[string]string) ([]check, int) {
	t.Helper()
	out, _, code := s.run(env, "doctor", "--json")
	var cs []check
	if err := json.Unmarshal([]byte(out), &cs); err != nil {
		t.Fatalf("not a JSON array: %v\n%s", err, out)
	}
	return cs, code
}

func find(t *testing.T, cs []check, name string) check {
	t.Helper()
	for _, c := range cs {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check %q in %+v", name, cs)
	return check{}
}

func TestDoctorHealthy(t *testing.T) {
	s := newSandbox(t, "")
	cs, code := doctor(t, s, nil)
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, cs)
	}
	for _, n := range []string{"actor", "config", "coord", "hub"} {
		if c := find(t, cs, n); c.Status != "ok" || c.Detail == "" {
			t.Errorf("%s = %+v", n, c)
		}
	}
	// The probe must be strictly read-only.
	calls := s.coordCalls()
	if len(calls) == 0 {
		t.Fatal("expected a probe call")
	}
	for _, c := range calls {
		if c != "list --json" {
			t.Errorf("non-read-only coord call %q in %v", c, calls)
		}
	}
}

func TestDoctorFailingChecksExit1AndPrintEverything(t *testing.T) {
	s := newSandbox(t, "")
	cs, code := doctor(t, s, map[string]string{"BEADS_ACTOR": "", "BEADS_LEASE_COORD": "/nonexistent/coord"})
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if c := find(t, cs, "actor"); c.Status != "fail" || c.Fix == "" {
		t.Errorf("actor = %+v", c)
	}
	if c := find(t, cs, "coord"); c.Status != "fail" || c.Fix == "" {
		t.Errorf("coord = %+v", c)
	}
	if c := find(t, cs, "hub"); c.Status != "skip" {
		t.Errorf("hub should skip when prerequisites fail: %+v", c)
	}
	if len(s.coordCalls()) != 0 {
		t.Error("coord must not be called when prerequisites fail")
	}
}

func TestDoctorNotExecutableCoordFails(t *testing.T) {
	s := newSandbox(t, "")
	os.Chmod(s.coord, 0o644)
	cs, code := doctor(t, s, nil)
	if code != 1 || find(t, cs, "coord").Status != "fail" {
		t.Fatalf("code=%d %+v", code, cs)
	}
}

func TestDoctorBrokenConfigWarnsNotFails(t *testing.T) {
	s := newSandbox(t, "")
	cfg := filepath.Join(s.home, "bad.json")
	os.WriteFile(cfg, []byte("not json"), 0o600)
	cs, code := doctor(t, s, map[string]string{"BEADS_LEASE_CONFIG": cfg})
	if code != 0 {
		t.Fatalf("a warn must not exit 1: %d", code)
	}
	if c := find(t, cs, "config"); c.Status != "warn" || !strings.Contains(c.Detail, "not valid JSON") {
		t.Errorf("config = %+v", c)
	}
}

func TestDoctorHubProbeFailure(t *testing.T) {
	s := newSandbox(t, "echo boom >&2; exit 3")
	cs, code := doctor(t, s, nil)
	if code != 1 || find(t, cs, "hub").Status != "fail" {
		t.Fatalf("code=%d %+v", code, cs)
	}
}

func TestDoctorText(t *testing.T) {
	out, _, code := newSandbox(t, "").run(nil, "doctor")
	if code != 0 || !strings.Contains(out, "ok") || !strings.Contains(out, "coord") || strings.HasPrefix(out, "[") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}
