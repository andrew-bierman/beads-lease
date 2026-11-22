// Package cmd wires beads-lease's CLI: worklease-style verbs over the Beads hub,
// plus the legacy `lease` surface (bare-resource claim, --status/--release).
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew-bierman/beads-lease/internal/cliutil"
	"github.com/andrew-bierman/beads-lease/internal/config"
)

var (
	jsonOut    bool
	currentOp  string
	currentRes string
)

// version is stamped at build time (-ldflags -X); keep as var, not const.
var version = "0.1.0"

func NewRootCmd(cfg *config.Config, invokedAs string) *cobra.Command {
	if invokedAs == "" {
		invokedAs = cfg.Name
	}
	root := &cobra.Command{
		Use:   invokedAs,
		Short: fmt.Sprintf("%s — %s", cfg.Name, cfg.Tagline),
		Long: fmt.Sprintf(`%s keeps the agent fleet's shared resources honestly held.

Claims are regular Beads claims on the coordination hub: atomic, leased
(expiry + heartbeat + reclaim come from bd), cross-machine via git. bd is the
sole authority; %s owns no state of its own.

  %s acquire cpu-quiet-window -t 15m          claim (registers on first use)
  %s run cpu-quiet-window -- pytest -n auto   claim, run, release on exit
  %s status | list                            who holds what
  %s release cpu-quiet-window -m done         release

Identity and policy come from %s; naming is configurable there.`,
			cfg.Name, cfg.Name, invokedAs, invokedAs, invokedAs, invokedAs, config.Path()),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVarP(&jsonOut, "json", "j", false, "print one JSON envelope instead of text")
	root.AddCommand(acquireCmd, releaseCmd, statusCmd, listCmd, heartbeatCmd, runCmd, versionCmd, doctorCmd)
	return root
}

// Execute runs the CLI and returns the process exit code. Identity (the
// command name shown in help) comes from argv[0]; policy and naming come
// from the config file. When the legacy surface is enabled it also
// translates the old `lease` forms (root --status/--release flags,
// bare-resource claim).
func Execute(args []string) int {
	cfg := config.Load()
	invokedAs := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if invokedAs == "" || invokedAs == "main" {
		invokedAs = cfg.Name
	}
	if cfg.LegacySurface && len(args) > 0 {
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
	root := NewRootCmd(cfg, invokedAs)
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
	fmt.Fprintf(os.Stderr, "%s: %v\n", cfg.Name, err)
	return cliutil.ExitUsage
}

func isKnown(s string) bool {
	switch s {
	case "acquire", "claim", "release", "status", "list", "ls",
		"heartbeat", "run", "exec", "version", "doctor", "help", "completion":
		return true
	}
	return false
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "print version and identity",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Load()
		if jsonOut {
			b, _ := json.Marshal(map[string]any{"name": cfg.Name, "version": version, "api_version": apiVersion})
			_, err := fmt.Println(string(b))
			return err
		}
		_, err := fmt.Printf("%s %s — %s\n", cfg.Name, version, cfg.Tagline)
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
