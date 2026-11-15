package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/andrew-bierman/beadle/internal/cliutil"
	"github.com/andrew-bierman/beadle/internal/hub"
	"github.com/andrew-bierman/beadle/internal/lease"
)

var (
	releaseResource string
	releaseReason   string
)

var releaseCmd = &cobra.Command{
	Use:   "release <resource> [reason...]",
	Short: "release your claim (refuses someone else's)",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		currentOp = "release"
		res := releaseResource
		reason := releaseReason
		if res == "" && len(args) > 0 {
			res = args[0]
			if reason == "" && len(args) > 1 {
				reason = strings.Join(args[1:], " ")
			}
		}
		c, err := hub.New()
		if err != nil {
			return err
		}
		r := lease.Normalize(res)
		currentRes = r.ID()
		if err := lease.Release(c, r, releaseReason); err != nil {
			return err
		}
		cliutil.Emit(&cliutil.Envelope{Operation: "release", Resource: r.ID()}, jsonOut,
			fmt.Sprintf("resource:%s: released", r.Key))
		return nil
	},
}

func init() {
	releaseCmd.Flags().StringVarP(&releaseResource, "resource", "r", "", "resource name (or pass positionally)")
	releaseCmd.Flags().StringVarP(&releaseReason, "reason", "m", "", "why you released it")
}
