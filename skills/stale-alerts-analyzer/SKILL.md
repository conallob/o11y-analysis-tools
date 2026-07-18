---
name: stale-alerts-analyzer
description: Query a live Prometheus's historical ALERTS series to find alert rules that haven't fired in a long time — "dead code analysis" for alerting rules, surfacing candidates for deletion or threshold review. Use interactively during alert-quality/on-call hygiene reviews, not as a CI gate. Requires a live Prometheus with sufficient ALERTS retention.
license: BSD-3-Clause
---

# stale-alerts-analyzer

Cross-references every `alert:` name defined in a rules file against
Prometheus's historical `ALERTS` series to find ones that haven't fired
within a configurable time horizon. Functionally this is dead-code
analysis applied to alerting rules: an alert nobody has seen fire in a
year is either (a) protecting against something that no longer happens,
(b) has a threshold too conservative to ever trigger, or (c) is genuinely
still needed and just hasn't had cause to fire — this tool flags
candidates, a human makes the call.

## When to use this skill

- Periodic alert hygiene / on-call load review: "which of our alerts are
  actually doing anything?"
- Before a large alerting-rules refactor, to identify safe-to-remove dead
  weight.
- **Not** a CI gate: it depends on live, non-reproducible production
  history, and "delete this alert" is a decision that deserves a human
  in the loop, even when using `--fix`.

## Prerequisites

- A reachable Prometheus (or Prometheus-API-compatible Thanos/Cortex/
  Mimir) that has actually been evaluating the rules file's alerts, so
  `ALERTS` has history.
- **Retention vs. `--timehorizon` matters just as much as for
  `alert-hysteresis`**: an alert reported "stale (no firings in 380
  days)" is meaningless if Prometheus only retains 30 days — the tool has
  no way to distinguish "never fired" from "fired outside what Prometheus
  still remembers." Confirm retention before trusting a long horizon.

## Setup

```bash
go build -o bin/stale-alerts-analyzer ./cmd/stale-alerts-analyzer
# or: go install github.com/conallob/o11y-analysis-tools/cmd/stale-alerts-analyzer@latest
```

## Usage

```
stale-alerts-analyzer [options]
```

| Flag | Default | Effect |
|---|---|---|
| `--prometheus-url` | `http://localhost:9090` | Prometheus API base URL. Required. |
| `--rules` | *(required)* | Path to the rules YAML file whose alert names to check. |
| `--timehorizon` | `12M` | Staleness lookback window. Accepts Go durations (`h`, `m`, `s`) **and** extended units: `d` (days), `w` (weeks), `M` (30-day months), `y` (365-day years) — e.g. `90d`, `6M`, `1y`. |
| `--fix` | `false` | Delete the identified stale alerts directly from the rules file. |
| `--verbose` | `false` | Print query details while fetching. |

Note: unlike the other tools, there is no `--output=json` flag despite
being mentioned in the top-level README's examples — as currently
implemented, output is always the human-readable report below; there is
no machine-readable export mode. Don't tell a user to pipe `--output=json`
expecting it to work.

### Typical invocations

```bash
# Default: alerts with no firings in the last 12 months
stale-alerts-analyzer --prometheus-url=http://prometheus:9090 --rules=./alerts.yml

# Shorter horizon in days
stale-alerts-analyzer --prometheus-url=http://prometheus:9090 --rules=./alerts.yml --timehorizon=90d

# Delete stale alerts directly (review the diff before committing!)
stale-alerts-analyzer --fix --prometheus-url=http://prometheus:9090 --rules=./alerts.yml --timehorizon=1y
```

## Reading the output

Lists active alerts (fired within the horizon, with last-fired time and
age) separately from stale alerts (last-fired time or "Never" within the
lookback), followed by a summary with total/active/stale counts and a
never-fired-vs-fired-but-stale breakdown. Exit code `1` when stale alerts
exist and `--fix` wasn't passed (so the run can be used as a nudge, even
if you don't want it as a hard CI gate); `0` when nothing is stale or
after a successful `--fix`.

## Agent workflow

1. Run without `--fix` first and read the stale list with the user —
   "stale" here is a signal to investigate, not an automatic deletion
   order. A dead-man's-switch-style alert (e.g. `Watchdog`) is *expected*
   to never "stale-fire" in the sense this tool measures, since it fires
   continuously rather than transiently; don't recommend deleting
   continuously-firing meta-alerts just because they show odd stats. This
   tool has no built-in exclude list (despite an `--exclude` flag being
   mentioned in the top-level README) — as currently implemented there is
   no such flag, so filter candidates yourself before recommending
   deletion.
2. For each stale candidate, distinguish "never fired" from "fired, but
   outside the horizon" using the printed breakdown — the former is a
   stronger deletion signal than the latter.
3. Cross-check `--timehorizon` against the target Prometheus's actual
   retention before treating a "stale" verdict as reliable.
4. Prefer proposing a deletion PR for human review over `--fix` unless
   explicitly asked to apply it directly — removing an alert is a
   coverage-reducing change.
