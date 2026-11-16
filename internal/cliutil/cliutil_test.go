package cliutil

import (
	"testing"
	"time"
)

func TestParseTTL(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		fail bool
	}{
		{in: "15m", want: 15 * time.Minute},
		{in: "1h", want: time.Hour},
		{in: "90s", want: 90 * time.Second},
		{in: "20", want: 20 * time.Minute}, // bare int = minutes (legacy form)
		{in: "", want: 15 * time.Minute},   // empty = configured default
		{in: "0", fail: true},
		{in: "banana", fail: true},
	}
	for _, c := range cases {
		got, err := ParseTTL(c.in)
		if c.fail {
			if err == nil {
				t.Errorf("ParseTTL(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTTL(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseTTL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRel(t *testing.T) {
	if got := Rel(time.Now().Add(12 * time.Minute)); got != "expires in 12m" {
		t.Errorf("Rel(future) = %q", got)
	}
	if got := Rel(time.Now().Add(-3 * time.Minute)); got != "expired 3m ago" {
		t.Errorf("Rel(past) = %q", got)
	}
}

func TestFailExitCodes(t *testing.T) {
	if code := Fail(&Envelope{}, false, Contention("busy")); code != ExitContention {
		t.Errorf("contention exit = %d, want %d", code, ExitContention)
	}
	if code := Fail(&Envelope{}, false, Usagef("bad")); code != ExitUsage {
		t.Errorf("usage exit = %d, want %d", code, ExitUsage)
	}
}
