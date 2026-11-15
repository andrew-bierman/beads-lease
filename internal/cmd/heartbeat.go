package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/andrew-bierman/beadle/internal/cliutil"
	"github.com/andrew-bierman/beadle/internal/hub"
	"github.com/andrew-bierman/beadle/internal/lease"
)

var hbResource string

var heartbeatCmd = &cobra.Command{
	Use:   "heartbeat <resource>",
	Short: "renew your lease (bd pushes the expiry forward)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		currentOp = "heartbeat"
		res := hbResource
		if res == "" && len(args) > 0 {
			res = args[0]
		}
		c, err := hub.New()
		if err != nil {
			return err
		}
		r := lease.Normalize(res)
		currentRes = r.ID()
		t, err := lease.Heartbeat(c, r)
		if err != nil {
			return err
		}
		env := &cliutil.Envelope{
			Operation: "heartbeat", Resource: r.ID(), Holder: hub.Actor(),
			ExpiresAt: t.LeaseExpires(), TaskID: t.ID,
		}
		cliutil.Emit(env, jsonOut, fmt.Sprintf("resource:%s: lease refreshed%s", r.Key, lease.ExpirySuffix(t)))
		return nil
	},
}

func init() {
	heartbeatCmd.Flags().StringVarP(&hbResource, "resource", "r", "", "resource name (or pass positionally)")
}
