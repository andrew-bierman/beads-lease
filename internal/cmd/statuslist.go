package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/andrew-bierman/beadle/internal/cliutil"
	"github.com/andrew-bierman/beadle/internal/hub"
	"github.com/andrew-bierman/beadle/internal/lease"
)

var statusFull bool

var statusCmd = &cobra.Command{
	Use:   "status [resource]",
	Short: "who holds what (one resource or all)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		currentOp = "status"
		c, err := hub.New()
		if err != nil {
			return err
		}
		if len(args) > 0 {
			r := lease.Normalize(args[0])
			currentRes = r.ID()
			row, registered, err := lease.Lookup(c, r)
			if err != nil {
				return err
			}
			found := registered
			env := &cliutil.Envelope{
				Operation: "status", Resource: row.Resource, Holder: row.Holder,
				State: row.State, ExpiresAt: row.ExpiresAt, TaskID: row.TaskID, Found: &found,
			}
			cliutil.Emit(env, jsonOut, statusText(row, registered))
			return nil
		}
		rows, err := lease.Rows(c)
		if err != nil {
			return err
		}
		env := &cliutil.Envelope{Operation: "status", Leases: rows}
		cliutil.Emit(env, jsonOut, table(rows, statusFull))
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "list every resource lease in the hub",
	RunE: func(cmd *cobra.Command, args []string) error {
		currentOp = "list"
		c, err := hub.New()
		if err != nil {
			return err
		}
		rows, err := lease.Rows(c)
		if err != nil {
			return err
		}
		env := &cliutil.Envelope{Operation: "list", Leases: rows}
		cliutil.Emit(env, jsonOut, table(rows, statusFull))
		return nil
	},
}

func statusText(row cliutil.LeaseRow, registered bool) string {
	switch {
	case !registered:
		return fmt.Sprintf("%s: free (never registered)", row.Resource)
	case row.State == "held":
		e := ""
		if row.ExpiresAt != nil {
			e = " (" + cliutil.Rel(*row.ExpiresAt) + ")"
		}
		return fmt.Sprintf("%s: held by %s%s", row.Resource, row.Holder, e)
	case row.State == "stale":
		return fmt.Sprintf("%s: free (stale claim by %s, status %s)",
			row.Resource, row.Holder, orDash(row.Status))
	default:
		return fmt.Sprintf("%s: free (task %s, %s)",
			row.Resource, orDash(row.TaskID), orDash(row.Status))
	}
}

func init() {
	statusCmd.Flags().BoolVarP(&statusFull, "full", "f", false, "include status and task id")
	listCmd.Flags().BoolVarP(&statusFull, "full", "f", false, "include status and task id")
}
