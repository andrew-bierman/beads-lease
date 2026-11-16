<!-- TODO(logo): drop the logo at docs/logo.png and uncomment — sent by Andrew, not yet on disk
<p align="center"><img src="docs/logo.png" alt="beadle" width="180"></p>
-->

# beadle

[![ci](https://github.com/andrew-bierman/beadle/actions/workflows/ci.yml/badge.svg)](https://github.com/andrew-bierman/beadle/actions/workflows/ci.yml)

The fleet's keeper of leases. `beadle` keeps shared resources — machine-heavy
work, measurement quiet windows, anything two agents might grab at once —
honestly held across every agent and machine, with **one binary and one CLI
surface on macOS and Linux**.

Claims are ordinary Beads claims on the coordination hub: atomic, leased
(expiry + heartbeat + reclaim come from `bd`), cross-machine via git sync.
**bd is the sole authority; beadle owns no state of its own.** On Linux boxes,
[leasehold](https://github.com/chalkworth/leasehold) sits underneath as
optional capacity admission (queueing, sizing, containment); on macOS the
claims are the whole system.

## Install

```sh
# Release binaries (macOS / Linux, amd64 + arm64):
#   grab beadle-<os>-<arch>.tar.gz from GitHub Releases, or:
go install github.com/andrew-bierman/beadle@latest

# From a clone (needs Go 1.27+, provisioned via mise in dotfiles setup):
make install     # builds + links ~/.local/bin/beadle
```

`lease` — the old name — keeps working: the dotfiles `bin/lease` shim execs
the beadle binary when it is built.

## Usage

```sh
export BEADS_ACTOR=glm-andrew-bierman        # your identity, same as the hub

beadle acquire cpu-quiet-window -t 15m -m "exp-001 measurement"
beadle status cpu-quiet-window               # held by you (expires in 12m)
beadle heartbeat cpu-quiet-window            # push the expiry forward
beadle release cpu-quiet-window -m done

# Guarded command: claim, run, heartbeat while it lives, release on exit —
# whatever the outcome. A busy resource fails the run without starting it.
beadle run resource-bench -- pytest -n auto

# Wait for a busy resource instead of failing (syncs the hub while polling)
beadle acquire deploy-slot -w 2m

beadle status                                # every lease in the hub
beadle --json list                           # one JSON envelope
```

Legacy `lease` surface is kept: `beadle <resource> [minutes] [note...]`,
`beadle --status [resource]`, `beadle --release <resource> [note...]`.

Environment: `BEADS_ACTOR` (required — unique per machine+agent, never reuse
another's), `BEADLE_COORD` / `FLEET_LEASE_COORD` (override the hub wrapper
path; default `~/Code/agent-coordination/bin/coord`).

## Semantics

- Resource ids are `resource:<name>`, stored as hub tasks titled
  `Resource lease: <name>` — visible next to the work agents already read.
- Contention is a structured outcome: exit `2`, holder named, expiry shown.
  With `--wait`, acquire polls the local clone every 5s and git-syncs the hub
  every 30s until free or the deadline passes.
- `run` heartbeats the lease for as long as the worker lives and **stops the
  worker if the lease is lost** (expired and reclaimed) — a claim you no
  longer hold must not keep running. Exit codes propagate; interruption
  exits `130`.
- The `-t/--ttl` you pass is the *advisory* intended hold time recorded in
  the task note; bd configures the real lease TTL, and `status`/`list` show
  the live expiry.

## Configuration

Optional config at `~/.config/beadle/config.json` (override location with
`BEADLE_CONFIG`); missing file = defaults. Identity, hub wiring and policy
are all config — **a rename is an edit here, not a code change**, and the
binary's help text follows the name it is invoked as (`argv[0]`):

```json
{
  "name": "beadle",
  "tagline": "the fleet's keeper of leases",
  "coordPath": "~/Code/agent-coordination/bin/coord",
  "actorEnv": "BEADS_ACTOR",
  "resourcePrefix": "resource:",
  "titlePrefix": "Resource lease: ",
  "defaultTTL": "15m",
  "contentionExit": 2,
  "legacySurface": true,
  "lowPriorityLinux": "nice -n 10",
  "lowPriorityDarwin": "taskpolicy -c utility",
  "protocolDoc": "dotfiles docs/guides/agent-resource-protocol.md"
}
```

`contentionExit` is the exit code for already-claimed / not-yours (worklease
convention: 2). `legacySurface` toggles the old `lease` forms (bare-resource
claim, `--status`, `--release`). `coordPath`/`actorEnv` have env overrides
that win: `BEADLE_COORD`/`FLEET_LEASE_COORD` and `BEADS_ACTOR`. See
`config.example.json`. An invalid config file warns and falls back to
defaults — a broken config never breaks the tool.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | success |
| `1` | internal failure |
| `2` | ownership/contention (already-claimed, not-yours) |
| `64` | invalid input or configuration |
| `130` | interrupted |

## What was copied from worklease — and what wasn't

[worklease](https://github.com/brettinternet/worklease) is the design
reference (its CLI is good). Adopted: the verb set (`acquire`/`release`/
`status`/`list`/`heartbeat`/`run`), short flags (`-r -t -w -m -j -f`), exit
families, the JSON envelope with stable reasons, relative expiry display,
and run-supervision semantics.

Deliberately skipped: any second authority (its local SQLite handles and
TLS/invite remote server) — the Beads hub is canonical for this fleet and
git sync is the cross-machine transport; plus TUI, MCP, completions,
`watch`/`events`/`history` (bd notes + hub git history cover these),
`transfer`/`checkpoint`/`replace-file`/operation replay (bd claims + notes
cover the need), and provider policies.

## Guardrails

- The Beads hub is canonical; beadle adds no second coordination system and
  stores nothing outside it.
- Sync before claiming on a different machine (`bin/coord sync --json`);
  `--wait` syncs while it polls.
- Protocol: `dotfiles docs/guides/agent-resource-protocol.md`.

## License

[MIT](LICENSE)
