package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew-bierman/beads-lease/internal/cliutil"
	"github.com/andrew-bierman/beads-lease/internal/config"
	"github.com/andrew-bierman/beads-lease/internal/hub"
)

// apiVersion versions the --json surface (version, doctor, list envelope).
// Bump only for breaking changes.
const apiVersion = 1

// Check is one doctor result; the JSON shape is fixed by abacus's tool contract.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | fail | skip
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// runChecks is read-only: the only hub call is `coord list --json`.
func runChecks(cfg *config.Config) []Check {
	var out []Check
	actorOK := cfg.Actor() != ""
	if actorOK {
		out = append(out, Check{Name: "actor", Status: "ok", Detail: "identity set: " + cfg.Actor()})
	} else {
		out = append(out, Check{Name: "actor", Status: "fail", Detail: cfg.ActorEnv + " is not set",
			Fix: fmt.Sprintf("export %s=<agent-machine>", cfg.ActorEnv)})
	}

	out = append(out, configCheck())

	coord := cfg.ResolveCoord()
	coordOK := false
	st, err := os.Stat(coord)
	switch {
	case err != nil:
		out = append(out, Check{Name: "coord", Status: "fail", Detail: "coord wrapper not found: " + coord,
			Fix: "set BEADS_LEASE_COORD or coordPath to the hub's bin/coord"})
	case st.IsDir() || st.Mode().Perm()&0o111 == 0:
		out = append(out, Check{Name: "coord", Status: "fail", Detail: "coord wrapper is not an executable file: " + coord,
			Fix: "chmod +x " + coord})
	default:
		coordOK = true
		out = append(out, Check{Name: "coord", Status: "ok", Detail: "coord wrapper: " + coord})
	}

	if !actorOK || !coordOK {
		return append(out, Check{Name: "hub", Status: "skip", Detail: "needs a set actor and a usable coord wrapper"})
	}
	return append(out, hubCheck(&hub.Client{Coord: coord}))
}

func configCheck() Check {
	path := config.Path()
	if _, err := os.Stat(path); err != nil {
		return Check{Name: "config", Status: "ok", Detail: "no config file at " + path + "; using defaults"}
	}
	if warn := config.LoadWarning(); warn != "" {
		return Check{Name: "config", Status: "warn", Detail: warn,
			Fix: "fix or remove " + path + " (defaults are used meanwhile)"}
	}
	return Check{Name: "config", Status: "ok", Detail: "parsed " + path}
}

func hubCheck(c *hub.Client) Check {
	type res struct {
		n   int
		err error
	}
	ch := make(chan res, 1)
	go func() {
		tasks, err := c.List()
		ch <- res{len(tasks), err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return Check{Name: "hub", Status: "fail", Detail: r.err.Error(),
				Fix: "run the coord wrapper's list --json by hand and check bd/dolt"}
		}
		return Check{Name: "hub", Status: "ok", Detail: fmt.Sprintf("coord list --json answered (%d issues)", r.n)}
	case <-time.After(10 * time.Second):
		return Check{Name: "hub", Status: "fail", Detail: "coord list --json timed out after 10s",
			Fix: "check hub connectivity"}
	}
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "read-only health checks (exit 1 only when a check fails)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		checks := runChecks(config.Load())
		failed := false
		for _, c := range checks {
			failed = failed || c.Status == "fail"
		}
		if jsonOut {
			b, _ := json.MarshalIndent(checks, "", "  ")
			fmt.Println(string(b))
		} else {
			for _, c := range checks {
				fmt.Printf("%-4s %-6s %s\n", c.Status, c.Name, c.Detail)
				if c.Fix != "" {
					fmt.Printf("      fix: %s\n", c.Fix)
				}
			}
		}
		if failed {
			return cliutil.ChildExit(cliutil.ExitInternal) // report already printed
		}
		return nil
	},
}
