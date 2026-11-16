// Package hub is beadle's Beads client. It shells out to the coordination
// hub's bin/coord wrapper (bd + dolt) — beadle owns no state of its own; the
// hub is the sole source of truth.
package hub

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/andrew-bierman/beadle/internal/cliutil"
	"github.com/andrew-bierman/beadle/internal/config"
)

// Task is the subset of a bd issue beadle reads.
type Task struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Assignee *string `json:"assignee"`
	Status   string  `json:"status"`

	LeaseExpiresRaw string `json:"lease_expires_at"`
	LeaseExpiresAlt string `json:"leaseExpiresAt"`
	HeartbeatAtRaw  string `json:"heartbeat_at"`
}

// LeaseExpires parses the lease expiry in whatever shape bd emits.
func (t Task) LeaseExpires() *time.Time {
	for _, raw := range []string{t.LeaseExpiresRaw, t.LeaseExpiresAlt} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
			if v, err := time.Parse(layout, raw); err == nil {
				return &v
			}
		}
	}
	return nil
}

// Holder returns the trimmed assignee ("" when unassigned).
func (t Task) Holder() string {
	if t.Assignee == nil {
		return ""
	}
	return strings.TrimSpace(*t.Assignee)
}

// Client runs the coordination hub wrapper.
type Client struct {
	Coord string
}

// New verifies the actor identity (environment-provided, never stored) and
// resolves the coord wrapper path. Both come from configuration.
func New() (*Client, error) {
	cfg := config.Load()
	if cfg.Actor() == "" {
		return nil, cliutil.Usagef(
			"%s is not set; export %s=<agent-machine> (e.g. glm-andrew-bierman)",
			cfg.ActorEnv, cfg.ActorEnv)
	}
	return &Client{Coord: cfg.ResolveCoord()}, nil
}

// Actor is the claiming identity (the configured actor env var; BEADS_ACTOR
// is always accepted as a fallback).
func Actor() string { return config.Load().Actor() }

func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command(c.Coord, args...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", cliutil.WithReason(
			cliutil.Internal("coord %s failed: %s", args[0], msg), "hub-failure")
	}
	return out.String(), nil
}

// List returns every issue in the hub.
func (c *Client) List() ([]Task, error) {
	out, err := c.run("list", "--json")
	if err != nil {
		return nil, err
	}
	var tasks []Task
	if err := json.Unmarshal([]byte(out), &tasks); err != nil {
		return nil, cliutil.Internal("could not parse hub list output: %v", err)
	}
	return tasks, nil
}

// Show returns one issue, tolerating both bd shapes (object and array).
func (c *Client) Show(id string) (Task, error) {
	out, err := c.run("show", id, "--json")
	if err != nil {
		return Task{}, err
	}
	var one Task
	if err := json.Unmarshal([]byte(out), &one); err == nil && one.ID != "" {
		return one, nil
	}
	var arr []Task
	if err := json.Unmarshal([]byte(out), &arr); err == nil && len(arr) > 0 {
		return arr[0], nil
	}
	return Task{}, cliutil.Internal("could not parse hub show output for %s", id)
}

// FindByTitle locates a task by exact (case-insensitive) title.
func (c *Client) FindByTitle(title string) (Task, bool, error) {
	tasks, err := c.List()
	if err != nil {
		return Task{}, false, err
	}
	for _, t := range tasks {
		if strings.EqualFold(strings.TrimSpace(t.Title), title) {
			return t, true, nil
		}
	}
	return Task{}, false, nil
}

// Create registers a new task and returns it.
func (c *Client) Create(title, description string) (Task, error) {
	out, err := c.run("create", "--title", title, "--description", description, "--json")
	if err != nil {
		return Task{}, err
	}
	var t Task
	if json.Unmarshal([]byte(out), &t) != nil || t.ID == "" {
		return Task{}, cliutil.Internal("could not create task %q", title)
	}
	return t, nil
}

// Claim atomically claims the task for BEADS_ACTOR (bd semantics: idempotent
// when the holder is you).
func (c *Client) Claim(id string) error {
	_, err := c.run("update", id, "--claim", "--json")
	return err
}

// Note appends a provenance line (best effort; never fails a command).
func (c *Client) Note(id, text string) {
	_, _ = c.run("note", id, text)
}

// Reopen clears both status and assignee. bd's `--status open` alone leaves
// the assignee set, so both updates run unconditionally (a released lease
// must read as free everywhere).
func (c *Client) Reopen(id string) {
	_, _ = c.run("update", id, "--status", "open", "--json")
	_, _ = c.run("update", id, "--assignee", "", "--json")
}

// Heartbeat pushes the lease expiry forward (bd: only the holder may).
func (c *Client) Heartbeat(id string) error {
	_, err := c.run("heartbeat", id)
	return err
}

// Sync git-round-trips the hub so cross-machine claims become visible.
func (c *Client) Sync() error {
	_, err := c.run("sync", "--json")
	return err
}

// Describe is the registry task's description on first registration.
func Describe(key string) string {
	return fmt.Sprintf(
		"Fleet resource lease. Claim with %s %s, release when done. Protocol: %s",
		config.Load().Name, key, config.Load().ProtocolDoc)
}
