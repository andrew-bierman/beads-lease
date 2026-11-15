// Package cmd wires beadle's CLI: worklease-style verbs over the Beads hub,
// plus the legacy `lease` surface (bare-resource claim, --status/--release).
package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew-bierman/beadle/internal/cliutil"
)

var (
	jsonOut    bool
	currentOp  string
	currentRes string
)

const version = "0.1.0"

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "beadle",
		Short: "beadle — the fleet's keeper of leases (resource claims over the Beads hub)",
		Long: `beadle keeps the agent fleet's shared resources honestly held.

Claims are regular Beads claims on the coordination hub: atomic, leased
(expiry + heartbeat + reclaim come from bd), cross-machine via git. bd is the
sole authority; beadle owns no state of its own.

  beadle acquire cpu-quiet-window -t 15m          claim (registers on first use)
  beadle run cpu-quiet-window -- pytest -n auto   claim, run, release on exit
  beadle status | list                            who holds what
  beadle release cpu-quiet-window -m done         release

The legacy ` + "`lease`" + ` surface is kept: bare-resource claim, --status, --release.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVarP(&jsonOut, "json", "j", false, "print one JSON envelope instead of text")
	root.AddCommand(acquireCmd, releaseCmd, statusCmd, listCmd, heartbeatCmd, runCmd, versionCmd)
	return root
}

// Execute runs the CLI and returns the process exit code. It translates the
// legacy `lease` surface (root --status/--release flags, bare-resource claim)
// so old muscle memory and scripts keep working.
func Execute(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "--status":
			args[0] = "status"
		case "--release":
			args[0] = "release"
		default:
			if !strings.HasPrefix(args[0], "-") && !isKnown(args[0]) {
				args = append([]string{"acquire"}, args...)
			}
		}
	}
	root := NewRootCmd()
	root.SetArgs(args) // cobra defaults to os.Args[1:], ignoring the translation above
	err := root.Execute()
	if err == nil {
		return cliutil.ExitOK
	}
	var e *cliutil.E
	if errors.As(err, &e) {
		if e.Reason == "child-exit" {
			return e.Code
		}
		return cliutil.Fail(&cliutil.Envelope{Operation: currentOp, Resource: currentRes}, jsonOut, e)
	}
	fmt.Fprintf(os.Stderr, "beadle: %v\n", err)
	return cliutil.ExitUsage
}

func isKnown(s string) bool {
	switch s {
	case "acquire", "claim", "release", "status", "list", "ls",
		"heartbeat", "run", "exec", "version", "help", "completion":
		return true
	}
	return false
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "print beadle's version",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Printf("beadle %s (leases over the Beads hub; bd is the authority)\n", version)
		return err
	},
}

// parseCompatArgs maps the legacy positional surface (resource [minutes]
// [note...]) onto the flag-based one.
func parseCompatArgs(args []string, resFlag, ttlFlag, noteFlag string) (res, ttl, note string) {
	res = resFlag
	rest := args
	if res == "" && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		res = rest[0]
		rest = rest[1:]
	}
	ttl = ttlFlag
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		if _, err := cliutil.ParseTTL(rest[0]); err == nil {
			ttl = rest[0]
			rest = rest[1:]
		}
	}
	parts := make([]string, 0, 1+len(rest))
	if strings.TrimSpace(noteFlag) != "" {
		parts = append(parts, strings.TrimSpace(noteFlag))
	}
	parts = append(parts, rest...)
	note = strings.Join(parts, " ")
	return res, ttl, note
}

func parseWait(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0s" || s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, cliutil.Usagef("bad --wait %q (use 0s, 30s, 2m)", s)
	}
	return d, nil
}

func table(rows []cliutil.LeaseRow, full bool) string {
	if len(rows) == 0 {
		return "no resource leases registered"
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)
	if full {
		fmt.Fprintln(w, "STATE\tRESOURCE\tHOLDER\tSTATUS\tLEASE\tTASK")
	} else {
		fmt.Fprintln(w, "STATE\tRESOURCE\tHOLDER\tLEASE\tTASK")
	}
	for _, r := range rows {
		leaseCol := "—"
		if r.ExpiresAt != nil {
			leaseCol = cliutil.Rel(*r.ExpiresAt)
		}
		holder := orDash(r.Holder)
		if full {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				r.State, r.Resource, holder, orDash(r.Status), leaseCol, r.TaskID)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				r.State, r.Resource, holder, leaseCol, r.TaskID)
		}
	}
	w.Flush()
	return b.String()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
