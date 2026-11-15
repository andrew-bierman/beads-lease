// Package lease is beadle's domain logic: register/claim/release/status/
// heartbeat over hub tasks titled "Resource lease: <name>".
package lease

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/andrew-bierman/beadle/internal/cliutil"
	"github.com/andrew-bierman/beadle/internal/hub"
)

const (
	Prefix      = "resource:"
	TitlePrefix = "Resource lease: "
)

// Ref is a normalized resource name.
type Ref struct{ Key string }

// Normalize accepts "cpu-quiet-window" or "resource:cpu-quiet-window".
func Normalize(s string) Ref {
	return Ref{Key: strings.TrimPrefix(strings.TrimSpace(s), Prefix)}
}

func (r Ref) ID() string    { return Prefix + r.Key }
func (r Ref) Title() string { return TitlePrefix + r.Key }

// Holder returns the task's current holder ("" when free).
func Holder(t hub.Task) string { return t.Holder() }

// Held reports a live claim: assigned AND in_progress.
func Held(t hub.Task) bool { return Holder(t) != "" && t.Status == "in_progress" }

func noteSuffix(note string) string {
	if strings.TrimSpace(note) == "" {
		return ""
	}
	return " — " + strings.TrimSpace(note)
}

// ExpirySuffix renders " (expires in 12m)" when bd granted a lease expiry.
func ExpirySuffix(t hub.Task) string {
	if e := t.LeaseExpires(); e != nil {
		return " (" + cliutil.Rel(*e) + ")"
	}
	return ""
}

// Resolve finds the registry task or registers it.
func Resolve(c *hub.Client, r Ref) (hub.Task, bool, error) {
	t, found, err := c.FindByTitle(r.Title())
	if err != nil || found {
		return t, false, err
	}
	created, err := c.Create(r.Title(), hub.Describe(r.Key))
	if err != nil {
		return hub.Task{}, false, err
	}
	return created, true, nil
}

// Acquire claims the resource, waiting up to `wait` (0 = refuse immediately).
// bd's claim carries a real lease TTL configured by bd itself; `ttl` is the
// advisory intended hold time recorded in the task note.
func Acquire(c *hub.Client, r Ref, ttl time.Duration, note string, wait time.Duration) (hub.Task, error) {
	actor := hub.Actor()
	deadline := time.Now().Add(wait)
	polls := 0
	for {
		t, fresh, err := Resolve(c, r)
		if err != nil {
			return hub.Task{}, err
		}
		if fresh {
			fmt.Fprintf(os.Stderr, "registered resource:%s (task %s)\n", r.Key, t.ID)
		}
		h := Holder(t)
		if !Held(t) || h == actor {
			if err := c.Claim(t.ID); err != nil {
				return hub.Task{}, cliutil.Contention(
					"resource:%s: claim failed (held by %s?)", r.Key, orNobody(h))
			}
			freshT, err := c.Show(t.ID)
			if err != nil {
				freshT = t
			}
			c.Note(t.ID, fmt.Sprintf("reserved ~%s by %s%s",
				cliutil.Human(ttl), actor, noteSuffix(note)))
			return freshT, nil
		}
		if wait <= 0 || time.Now().After(deadline) {
			return hub.Task{}, cliutil.Contention(
				"resource:%s: already-claimed by %s%s — defer heavy work or message them (beadle status %s re-checks)",
				r.Key, h, ExpirySuffix(t), r.Key)
		}
		// Wait loop: re-check the local clone every 5s; git-sync the hub
		// every 30s so cross-machine claims become visible.
		time.Sleep(5 * time.Second)
		polls++
		if polls%6 == 0 {
			_ = c.Sync()
		}
	}
}

// Release ends the caller's own claim; anyone else's is refused.
func Release(c *hub.Client, r Ref, reason string) error {
	t, found, err := c.FindByTitle(r.Title())
	if err != nil {
		return err
	}
	if !found {
		return cliutil.NotYours("resource:%s: nothing to release (never registered)", r.Key)
	}
	h := Holder(t)
	if h != hub.Actor() {
		return cliutil.NotYours("resource:%s: not yours (held by '%s') — refusing",
			r.Key, orNobody(h))
	}
	c.Note(t.ID, fmt.Sprintf("released by %s%s", hub.Actor(), noteSuffix(reason)))
	c.Reopen(t.ID)
	return nil
}

// Heartbeat renews the caller's own lease.
func Heartbeat(c *hub.Client, r Ref) (hub.Task, error) {
	t, found, err := c.FindByTitle(r.Title())
	if err != nil {
		return hub.Task{}, err
	}
	if !found {
		return hub.Task{}, cliutil.NotYours("resource:%s: never registered", r.Key)
	}
	if h := Holder(t); h != hub.Actor() {
		return hub.Task{}, cliutil.NotYours("resource:%s: not yours (held by '%s') — refusing",
			r.Key, orNobody(h))
	}
	if err := c.Heartbeat(t.ID); err != nil {
		return hub.Task{}, cliutil.WithReason(cliutil.Internal(
			"resource:%s: heartbeat refused — lease lost or reclaimed (%v)", r.Key, err),
			"claim-lost")
	}
	return c.Show(t.ID)
}

func rowOf(t hub.Task, key string) cliutil.LeaseRow {
	row := cliutil.LeaseRow{
		Resource:  Prefix + key,
		Holder:    Holder(t),
		Status:    t.Status,
		ExpiresAt: t.LeaseExpires(),
		TaskID:    t.ID,
	}
	switch {
	case Held(t):
		row.State = "held"
	case Holder(t) != "":
		row.State = "stale"
	default:
		row.State = "free"
	}
	return row
}

// Lookup returns one resource's row; registered=false when never claimed.
func Lookup(c *hub.Client, r Ref) (cliutil.LeaseRow, bool, error) {
	t, found, err := c.FindByTitle(r.Title())
	if err != nil || !found {
		return cliutil.LeaseRow{Resource: r.ID(), State: "unregistered"}, false, err
	}
	return rowOf(t, r.Key), true, nil
}

// Rows lists every resource lease in the hub, sorted by name.
func Rows(c *hub.Client) ([]cliutil.LeaseRow, error) {
	tasks, err := c.List()
	if err != nil {
		return nil, err
	}
	var rows []cliutil.LeaseRow
	for _, t := range tasks {
		title := strings.TrimSpace(t.Title)
		if !strings.HasPrefix(strings.ToLower(title), strings.ToLower(TitlePrefix)) {
			continue
		}
		name := strings.TrimSpace(strings.SplitN(title, ":", 2)[1])
		rows = append(rows, rowOf(t, name))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Resource < rows[j].Resource })
	return rows, nil
}

func orNobody(h string) string {
	if h == "" {
		return "nobody"
	}
	return h
}
