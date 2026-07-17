---
name: alert-hysteresis
description: Query a live Prometheus's historical ALERTS series to statistically recommend better 'for:' hold-down durations, reducing spurious short-lived alerts and alert fatigue. Use interactively when an alert is firing too often/too briefly and needs its hysteresis tuned, or when auditing a rules file's for: values against real firing behavior. Requires a live Prometheus with sufficient ALERTS retention — not hermetic, not a CI check.
license: BSD-3-Clause
---

# alert-hysteresis

Analyzes how long alerts have actually stayed firing in the past (via
Prometheus's `ALERTS{alertname=...}` series) and recommends a `for:`
duration that would filter out short-lived, non-actionable firings while
still catching the ones that matter. This is a judgment-assisted,
interactive tool, not a static analyzer — every run depends on real,
non-reproducible production history.

## When to use this skill

- On-call has flagged an alert as noisy/flappy and you want a
  data-driven `for:` recommendation instead of guessing.
- You're auditing a rules file's existing `for:` values against how the
  alert has actually behaved in production.
- **Do not** wire this into CI as a blocking check: its output depends on
  live Prometheus data that CI can't reproduce deterministically, and the
  right response to a recommendation is a human decision, not an
  auto-applied diff (except deliberately, via `--fix`, described below).

## Prerequisites

- A reachable Prometheus (or Thanos/Cortex/Mimir with a Prometheus-
  compatible `/api/v1/query_range` endpoint) that has been actually
  evaluating the alerting rules in question, so the `ALERTS` metric has
  history to query.
- **Retention matters**: this tool can only see as far back as
  `--timeframe` *and* as far back as Prometheus has actually retained
  samples. A `--timeframe=30d` against a Prometheus with 15d retention
  will silently undercount — always sanity-check retention before trusting
  a "no data" or thin-sample result.

## Setup

```bash
go build -o bin/alert-hysteresis ./cmd/alert-hysteresis
# or: go install github.com/conallob/o11y-analysis-tools/cmd/alert-hysteresis@latest
```

## Usage

```
alert-hysteresis [options]
```

| Flag | Default | Effect |
|---|---|---|
| `--prometheus-url` | `http://localhost:9090` | Prometheus API base URL. Required to be reachable — the tool errors out otherwise. |
| `--alert` | `""` | Restrict analysis to one alert name. Omit to analyze every alert with firing history in the window. |
| `--timeframe` | `168h` (7d) | Lookback window, any Go duration (`24h`, `72h`, …). |
| `--rules` | `""` | Path to a rules YAML file — lets the tool compare its recommendation against the *currently configured* `for:` value and only flag a mismatch beyond `--threshold`. Without it, every alert's recommendation is just reported, not flagged as a mismatch. |
| `--threshold` | `0.2` | Fractional mismatch (vs. configured `for:`) required before flagging a recommendation, e.g. `0.3` = only flag when recommended and configured differ by >30%. |
| `--target-percentile` | `0.3` | Which percentile of historical firing durations to recommend as the new `for:` (e.g. `0.5` = median). Higher values are more conservative (fewer alerts prevented, less risk of missing a real incident). |
| `--fix` | `false` | Requires `--rules`. Writes the recommended `for:` values directly into the rules file instead of just printing them. |

### Typical invocations

```bash
# Survey all alerts over the last 7 days (default)
alert-hysteresis --prometheus-url=http://prometheus:9090

# Focus on one noisy alert over the last day
alert-hysteresis --prometheus-url=http://prometheus:9090 --alert=HighErrorRate --timeframe=24h

# Compare against configured values, only flag >30% mismatches
alert-hysteresis --prometheus-url=http://prometheus:9090 --rules=./alerts.yml --threshold=0.3

# Apply recommendations directly (review the diff before committing!)
alert-hysteresis --prometheus-url=http://prometheus:9090 --fix --rules=./alerts.yml --target-percentile=0.5
```

## Reading the output

Per alert: firing count, average/median/P75/P90/min/max durations, the
configured `for:` (if `--rules` given), and either a `RECOMMENDATION` with
reasoning and prevented-alert count, or a confirmation that the current
value is acceptable. A closing summary counts how many alerts need
adjustment. Exit code `1` if any alert needs adjustment (or, without
`--rules`, whenever any alert simply has a nonzero recommendation to
report against — read the summary line to disambiguate "found N alerts
with recommendations" from "found N alerts that need hysteresis
adjustment").

## Agent workflow

1. Always pass `--rules` when one exists — without it you only get raw
   statistics, not an actionable "current vs. recommended" comparison.
2. Start with the default `--target-percentile=0.3`/`--threshold=0.2` and
   discuss the tradeoff with the user before changing them: a higher
   percentile means a longer `for:` (fewer alerts, slower detection); a
   lower one means faster detection but less spurious-alert filtering.
3. Prefer printing recommendations and letting a human review the diff
   over using `--fix` directly, unless the user has explicitly asked for
   the change to be applied — this tool is tuning production alerting
   behavior, and a bad `for:` value can hide a real incident.
4. If results look empty or thin, check Prometheus retention against
   `--timeframe` before concluding the alert genuinely never fires long
   enough — that's a job better suited to `stale-alerts-analyzer` anyway.
