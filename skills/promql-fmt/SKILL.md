---
name: promql-fmt
description: Format and lint PromQL expressions embedded in Prometheus/Thanos/Cortex/Mimir rule YAML files — multiline layout, redundant aggregation-clause removal, aggregation-style consistency, and naming/instrumentation best-practice checks. Use whenever writing, editing, or reviewing alert/recording rule YAML, or as a CI formatting gate on pull requests that touch rule files. Fully hermetic: no network access or running Prometheus required.
license: BSD-3-Clause
---

# promql-fmt

Static analyzer and auto-formatter for PromQL `expr:`/`query:` fields inside
Prometheus-style rule YAML files. Hermetic — it only reads the files you
point it at.

## When to use this skill

- A user is authoring or editing a Prometheus/Thanos/Cortex/Mimir alert or
  recording rule and wants it formatted consistently.
- You're reviewing a diff to rule YAML and want to catch style or
  best-practice violations before commit.
- You're wiring a CI job that should fail a PR touching rule files with
  inconsistent formatting.

## Setup

```bash
go build -o bin/promql-fmt ./cmd/promql-fmt
# or: go install github.com/conallob/o11y-analysis-tools/cmd/promql-fmt@latest
```

## Usage

```
promql-fmt [options] <file|directory>...
```

Only `.yml`/`.yaml` files are scanned; directories are walked recursively.
There is no stdin mode (unlike `label-check`) — always pass a file or
directory path.

| Flag | Default | Effect |
|---|---|---|
| `--check` | `true` | Report issues, exit 1 if any found. Do not modify files. |
| `--fix` / `--fmt` | `false` | Rewrite files in place with the formatted output. `--fmt` is an alias for `--fix`. Passing either disables `--check` automatically. |
| `--verbose` | `false` | Print per-file "Fixing …" lines while fixing. |
| `--disable-line-length` | `false` | Suppress the "should use multiline formatting" nudge for long lines — useful for recording rules with unavoidably long metric names. |
| `--prometheus-url` | `""` | Optional. If set, additionally queries that Prometheus for **timeseries continuity** — i.e., flags expressions that reference metrics with no matching series. This is the one *non-hermetic* opt-in check; omit the flag to stay fully offline. |

### Typical invocations

```bash
# CI gate / pre-commit check (default mode)
promql-fmt --check ./prometheus/

# Auto-fix in place, then re-check
promql-fmt --fix ./prometheus/

# Verbose fix, skip line-length nudges for long recording-rule names
promql-fmt --fix --verbose --disable-line-length ./prometheus/recording-rules.yml
```

## What it actually checks/fixes

- **Multiline layout**: rewrites long/complex `expr:` blocks into an
  indented multiline YAML block scalar (`expr: |`), removing a redundant
  `by (...)` clause from the left operand of a binary expression when both
  operands share the same aggregation labels, and inserting an explicit
  `on (...)` vector-matching clause.
- **Aggregation clause consistency**: detects whether a file predominantly
  uses postfix (`sum(metric) by (label)`) or prefix
  (`sum by (label) (metric)`) style and flags expressions that break with
  the file's dominant style.
- **Naming conventions**: snake_case metric names, application-name
  prefixes, base units (warns on `_milliseconds`/`_minutes`/etc., wants
  `_seconds`), `_total` suffix on counters, `_ratio` instead of raw
  percentages, `level:metric:operations` naming for recording rules.
- **Instrumentation patterns**: `rate()`/`irate()` applied to something
  that doesn't look like a counter, division without zero-protection,
  utilization ratios where the denominator metric name doesn't contain
  `total`, `up{...}` used without a `job` selector.
- **Alert hysteresis sanity**: warns when an alert's expression is
  duration-sensitive but the rule has no `for:` at all (a distinct,
  static check from the separate `alert-hysteresis` tool, which instead
  tunes an *existing* `for:` value against real firing history).

## Reading the output

Check mode groups issues by file:

```
./alerts/api-alerts.yml:
  - Metric 'httpRequestCount' should use snake_case, not camelCase
  - Expression should use multiline formatting: sum(rate(http_requests_total...

Found formatting issues in 1/12 files
Run with --fix to automatically format
```

Exit code is `1` whenever any file has issues in check mode (or a write
error occurs in fix mode), `0` otherwise. Every issue is plain English —
there's no machine-readable JSON mode, so an agent should parse them as
free text and either explain them to the user or run `--fix` and re-check.

## Agent workflow

1. Run `promql-fmt --check <path>` first. If exit code is 0, nothing to do.
2. If issues are found and they're purely formatting (multiline/aggregation
   style), prefer `promql-fmt --fix <path>` over manual edits, then re-run
   `--check` to confirm.
3. If issues are best-practice warnings (naming, instrumentation), surface
   them to the user — these often require a human judgment call (e.g.
   renaming a metric is a breaking change for dashboards/alerts elsewhere).
4. In CI, use `--check` only; never run `--fix` unattended in a pipeline
   that auto-commits, since some rewrites (removing a "redundant"
   aggregation clause) change semantics if the assumption about matching
   labels doesn't actually hold — treat `--fix` output as something a human
   reviews before merge, same as any auto-formatter.
