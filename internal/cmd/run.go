package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew-bierman/beads-lease/internal/cliutil"
	"github.com/andrew-bierman/beads-lease/internal/config"
	"github.com/andrew-bierman/beads-lease/internal/hub"
	"github.com/andrew-bierman/beads-lease/internal/lease"
)

var (
	runResource string
	runTTL      string
	runWait     string
	runNote     string
	runPriority bool
)

var runCmd = &cobra.Command{
	Use:     "run [flags] <resource> [minutes] [note...] -- <command> [args...]",
	Aliases: []string{"exec"},
	Short:   "claim a resource, run a command while holding it, release on exit",
	Long: `Claims the resource (refusing — or waiting, with --wait — if another
actor holds it), runs the command, heartbeats the lease for as long as the
command lives, and releases on exit whatever the outcome. If the lease is
lost (expired and reclaimed), the worker is stopped: a claim you no longer
hold must not keep running.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		currentOp = "run"
		dash := cmd.ArgsLenAtDash()
		if dash < 0 || dash >= len(args) {
			return cliutil.Usagef("run needs a command after -- (beads-lease run <resource> -- <cmd...>)")
		}
		argv := args[dash:]
		res, ttlRaw, note := parseCompatArgs(args[:dash], runResource, runTTL, runNote)
		ttl, err := cliutil.ParseTTL(ttlRaw)
		if err != nil {
			return err
		}
		wait, err := parseWait(runWait)
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
		fmt.Fprintf(os.Stderr, "resource:%s: held by %s for: %s%s\n",
			r.Key, hub.Actor(), strings.Join(argv, " "), lease.ExpirySuffix(t))

		rc, lost := supervise(c, t.ID, argv, runPriority)

		if err := lease.Release(c, r, fmt.Sprintf("run exited rc=%d", rc)); err != nil && !lost {
			fmt.Fprintf(os.Stderr, "beads-lease: release failed: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "resource:%s: released (rc=%d)\n", r.Key, rc)
		}
		if lost {
			return cliutil.Internal("resource:%s: ownership lost — worker stopped", r.Key)
		}
		if rc != 0 {
			return cliutil.ChildExit(rc)
		}
		return nil
	},
}

// supervise runs the worker under the held lease: it heartbeats while the
// worker lives and stops the worker if the lease is lost.
func supervise(c *hub.Client, taskID string, argv []string, lowPriority bool) (int, bool) {
	cadence := 45 * time.Second
	// Cadence from bd's real lease TTL, kept well under it (bd heartbeats
	// must outrun expiry); clamped to [10s, 60s].
	if tt, err := leaseTTL(c, taskID); err == nil && tt > 30*time.Second {
		cadence = tt / 3
		if cadence > 60*time.Second {
			cadence = 60 * time.Second
		}
		if cadence < 10*time.Second {
			cadence = 10 * time.Second
		}
	}

	hbStop := make(chan struct{})
	lost := make(chan struct{})
	var once sync.Once
	go func() {
		tk := time.NewTicker(cadence)
		defer tk.Stop()
		for {
			select {
			case <-hbStop:
				return
			case <-tk.C:
				if err := c.Heartbeat(taskID); err != nil {
					once.Do(func() { close(lost) })
					return
				}
			}
		}
	}()
	defer close(hbStop)

	execArgv := argv
	if lowPriority {
		cfg := config.Load()
		var prefix []string
		switch runtime.GOOS {
		case "linux":
			prefix = strings.Fields(cfg.LowPriorityLinux)
		case "darwin":
			prefix = strings.Fields(cfg.LowPriorityDarwin)
		}
		if len(prefix) == 0 {
			fmt.Fprintf(os.Stderr, "%s: --low-priority unsupported on %s; running unprioritized\n", config.Load().Name, runtime.GOOS)
		} else {
			execArgv = append(prefix, argv...)
		}
	}

	child := exec.Command(execArgv[0], execArgv[1:]...)
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "beads-lease: %v\n", err)
		return cliutil.ExitUsage, false
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		for s := range sig {
			_ = child.Process.Signal(s)
		}
	}()

	done := make(chan struct{})
	go func() { _ = child.Wait(); close(done) }()

	select {
	case <-done:
		return childExitCode(child), false
	case <-lost:
		_ = child.Process.Kill()
		<-done
		return childExitCode(child), true
	}
}

// leaseTTL asks the hub how long bd grants per lease.
func leaseTTL(c *hub.Client, taskID string) (time.Duration, error) {
	t, err := c.Show(taskID)
	if err != nil {
		return 0, err
	}
	e := t.LeaseExpires()
	if e == nil {
		return 0, fmt.Errorf("no lease expiry recorded")
	}
	return time.Until(*e), nil
}

func childExitCode(c *exec.Cmd) int {
	if c.ProcessState == nil {
		return 1
	}
	if ws, ok := c.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 130
	}
	return c.ProcessState.ExitCode()
}

func init() {
	runCmd.Flags().StringVarP(&runResource, "resource", "r", "", "resource name (or pass positionally)")
	runCmd.Flags().StringVarP(&runTTL, "ttl", "t", "", "advisory hold time (default: config defaultTTL)")
	runCmd.Flags().StringVarP(&runWait, "wait", "w", "0s", "wait up to this for a busy resource (e.g. 2m)")
	runCmd.Flags().StringVarP(&runNote, "note", "m", "", "why you hold it")
	runCmd.Flags().BoolVar(&runPriority, "low-priority", false,
		"run the command deprioritized (nice 10 on Linux, taskpolicy -c utility on macOS); opt-in, never the default")
}
