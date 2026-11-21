package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/andrew-bierman/beads-lease/internal/cliutil"
	"github.com/andrew-bierman/beads-lease/internal/hub"
	"github.com/andrew-bierman/beads-lease/internal/lease"
)

var (
	acquireResource string
	acquireTTL      string
	acquireWait     string
	acquireNote     string
)

var acquireCmd = &cobra.Command{
	Use:     "acquire [resource] [minutes] [note...]",
	Aliases: []string{"claim"},
	Short:   "claim a resource (registers it on first use)",
	Args:    cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		currentOp = "acquire"
		res, ttlRaw, note := parseCompatArgs(args, acquireResource, acquireTTL, acquireNote)
		ttl, err := cliutil.ParseTTL(ttlRaw)
		if err != nil {
			return err
		}
		wait, err := parseWait(acquireWait)
		if err != nil {
			return err
		}
		c, err := hub.New()
		if err != nil {
			return err
		}
		r := lease.Normalize(res)
		currentRes = r.ID()
		t, err := lease.Acquire(c, r, ttl, note, wait)
		if err != nil {
			return err
		}
		env := &cliutil.Envelope{
			Operation: "acquire",
			Resource:  r.ID(),
			Holder:    hub.Actor(),
			ExpiresAt: t.LeaseExpires(),
			TaskID:    t.ID,
		}
		text := fmt.Sprintf("resource:%s: claimed by %s%s. Release with: beads-lease release %s",
			r.Key, hub.Actor(), lease.ExpirySuffix(t), r.Key)
		cliutil.Emit(env, jsonOut, text)
		return nil
	},
}

func init() {
	acquireCmd.Flags().StringVarP(&acquireResource, "resource", "r", "", "resource name (or pass positionally)")
	acquireCmd.Flags().StringVarP(&acquireTTL, "ttl", "t", "", "advisory hold time (default: config defaultTTL)")
	acquireCmd.Flags().StringVarP(&acquireWait, "wait", "w", "0s", "wait up to this for a busy resource (e.g. 2m)")
	acquireCmd.Flags().StringVarP(&acquireNote, "note", "m", "", "why you hold it")
}
