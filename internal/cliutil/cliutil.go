// Package cliutil carries beadle's shared conventions: exit-code families,
// the JSON envelope, duration parsing, and human time formatting.
//
// Exit codes follow the worklease families beadle deliberately copies:
//
//	0 success; 1 internal failure; 2 ownership/contention;
//	64 invalid input or configuration; 130 interrupted.
package cliutil

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/andrew-bierman/beadle/internal/config"
)

const (
	ExitOK         = 0
	ExitInternal   = 1
	ExitContention = 2
	ExitUsage      = 64
	ExitInterrupt  = 130
)

// E is a failure that knows how the process should exit.
type E struct {
	Code   int
	Reason string
	Msg    string
	Detail map[string]string
}

func (e *E) Error() string { return e.Msg }

func Contention(format string, a ...any) *E {
	return &E{Code: config.Load().ContentionExit, Reason: "already-claimed", Msg: fmt.Sprintf(format, a...)}
}

func NotYours(format string, a ...any) *E {
	return &E{Code: config.Load().ContentionExit, Reason: "not-claim-holder", Msg: fmt.Sprintf(format, a...)}
}

func Usagef(format string, a ...any) *E {
	return &E{Code: ExitUsage, Reason: "invalid-argument", Msg: fmt.Sprintf(format, a...)}
}

func Internal(format string, a ...any) *E {
	return &E{Code: ExitInternal, Reason: "internal", Msg: fmt.Sprintf(format, a...)}
}

func WithReason(e *E, reason string) *E { e.Reason = reason; return e }

// ChildExit carries the supervised command's own exit code; Execute returns
// it without printing (the child already reported itself).
func ChildExit(code int) *E { return &E{Code: code, Reason: "child-exit"} }

// LeaseRow is one row of `list`/`status --json`.
type LeaseRow struct {
	Resource  string     `json:"resource"`
	Holder    string     `json:"holder,omitempty"`
	State     string     `json:"state"` // "held" | "free" | "unregistered"
	Status    string     `json:"status,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	TaskID    string     `json:"task,omitempty"`
}

// Envelope is the single JSON shape every command prints under --json.
type Envelope struct {
	SchemaVersion int        `json:"schemaVersion"`
	OK            bool       `json:"ok"`
	Operation     string     `json:"operation,omitempty"`
	Resource      string     `json:"resource,omitempty"`
	Error         *ErrBody   `json:"error,omitempty"`
	Holder        string     `json:"holder,omitempty"`
	State         string     `json:"state,omitempty"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	Found         *bool      `json:"found,omitempty"`
	TaskID        string     `json:"task,omitempty"`
	Leases        []LeaseRow `json:"leases,omitempty"`
	ExitCode      int        `json:"-"`
}

type ErrBody struct {
	Reason   string            `json:"reason"`
	ExitCode int               `json:"exitCode"`
	Message  string            `json:"message"`
	Details  map[string]string `json:"details,omitempty"`
}

// Emit prints the text line (unless jsonMode) or the JSON envelope, and
// returns the process exit code.
func Emit(env *Envelope, jsonMode bool, text string) int {
	if env.SchemaVersion == 0 {
		env.SchemaVersion = 1
	}
	if jsonMode {
		if env.Error == nil {
			env.OK = true
		}
		env.ExitCode = 0
		out, err := json.MarshalIndent(env, "", "  ")
		if err != nil { // fall back to text on marshal failure
			fmt.Fprintln(os.Stderr, text)
			return ExitInternal
		}
		fmt.Println(string(out))
		return ExitOK
	}
	if text != "" {
		fmt.Println(text)
	}
	return ExitOK
}

// Fail prints the error (text to stderr, JSON envelope to stdout) and returns
// the process exit code.
func Fail(env *Envelope, jsonMode bool, e *E) int {
	if env.SchemaVersion == 0 {
		env.SchemaVersion = 1
	}
	if env.Error == nil {
		env.Error = &ErrBody{Reason: e.Reason, ExitCode: e.Code, Message: e.Msg, Details: e.Detail}
	}
	env.ExitCode = e.Code
	if jsonMode {
		out, _ := json.MarshalIndent(env, "", "  ")
		fmt.Println(string(out))
	} else if e.Code == ExitContention || e.Code == ExitUsage {
		fmt.Fprintln(os.Stderr, e.Msg)
	} else {
		fmt.Fprintf(os.Stderr, "beadle: %s\n", e.Msg)
	}
	return e.Code
}

// ParseTTL accepts "15m", "1h", "90s", or a bare integer meaning minutes.
// Empty means the configured default TTL.
func ParseTTL(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		s = config.Load().DefaultTTL
	}
	if s == "" {
		return 15 * time.Minute, nil
	}
	if allDigits(s) {
		s += "m"
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, Usagef("bad duration %q (use 15m, 1h, 90s, or bare minutes)", s)
	}
	return d, nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// Rel renders a relative time like "in 12m" or "expired 3m ago".
func Rel(t time.Time) string {
	d := time.Until(t)
	if d >= 0 {
		return "expires in " + Human(d)
	}
	return "expired " + Human(-d) + " ago"
}

// Human renders a non-negative duration compactly ("45s", "12m", "1h05m").
func Human(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}
