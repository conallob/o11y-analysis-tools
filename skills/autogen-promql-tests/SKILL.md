---
name: autogen-promql-tests
description: Identify Prometheus alert/recording rules that have no promtool unit-test coverage, and scaffold true-positive/false-positive/hysteresis test-case skeletons for them. Use when bootstrapping unit tests for new or existing PromQL rules, or auditing test coverage of a rules file. Fully hermetic: reads only local rule/test YAML, no running Prometheus required.
license: BSD-3-Clause
---

# autogen-promql-tests

Coverage auditor and test-skeleton generator for Prometheus rule files. It
does not know your actual metric shapes — it emits a `promtool`-compatible
test file full of `TODO` placeholders that a human (or an agent that has
read the real metrics) must fill in with realistic input series and
expected values. Hermetic — no network access.

## When to use this skill

- A rules file has alerts or recording rules with no corresponding
  `promtool test rules` coverage, and you want a starting skeleton rather
  than writing `input_series`/`exp_alerts` blocks from scratch.
- You want a quick coverage report: how many of N rules currently have
  tests, and which ones don't.
- This is a **bootstrap** tool, best run interactively the first time a
  rule is added — not a CI gate, since its generated output is
  intentionally incomplete (`TODO` markers) and would fail `promtool test
  rules` until a human fills them in.

## Setup

```bash
go build -o bin/autogen-promql-tests ./cmd/autogen-promql-tests
# or: go install github.com/conallob/o11y-analysis-tools/cmd/autogen-promql-tests@latest
```

## Usage

```
autogen-promql-tests [options]
```

| Flag | Default | Effect |
|---|---|---|
| `--rules` | *(required)* | Path to the Prometheus rules YAML file to audit. |
| `--tests` | `""` | Path to an existing test file to check coverage against. If omitted, the tool auto-discovers `<rules-basename>_test.yml` next to the rules file. |
| `--fix` | `false` | Generate a test file covering the untested alerts/rules. |
| `--verbose` | `false` | Print discovery/loading detail. |

### Typical invocations

```bash
# Coverage report only (exit 1 if anything is untested)
autogen-promql-tests --rules=./alerts.yml

# Generate a test skeleton for whatever's untested
autogen-promql-tests --rules=./alerts.yml --fix

# Check against an explicit, non-default-named test file
autogen-promql-tests --rules=./rules.yml --tests=./custom_tests.yml
```

Without `--tests`, it looks for `<rules>_test.yml` (rules file's basename
with `.yml`/`.yaml` stripped, `_test.yml` appended) in the same directory.
With `--fix`, the same path is the default write target unless `--tests`
is also given, in which case that path is used both to read *and* write.

## What the generated file contains

For every rule not already covered (`alert:` or `record:` name absent from
the tested set), it appends four test cases to the output YAML:

1. **True positive** — `input_series` tuned (via a `TODO`) to make the
   alert fire; asserts `exp_alerts` with the rule's actual `labels:` and a
   `TODO` placeholder for each `annotations:` template value.
2. **False positive** — same shape, but `TODO`'d to *not* trigger the
   condition; asserts `exp_alerts: []`.
3. **Hysteresis check** — only emitted if the rule has a `for:` duration:
   evaluates at roughly half the `for:` duration and asserts the alert has
   **not** fired yet, to test the hold-down timer itself.
4. **Edge cases** — an empty placeholder section reminding the human to add
   boundary values, missing-metric behavior, label-combination cases, and
   recovery behavior.

The `input_series` blocks always start from a single placeholder
`example_metric{job="test", instance="localhost:9090"}` series with a
`TODO` — the tool does not parse the expression's actual metric names into
realistic series, it only echoes the first line of the expression as a
comment reminder. **An agent should replace these placeholders using the
real metric names and label sets from the rule's `expr:`.**

## Reading the output

```
═══════════════════════════════════════════════════════════
Test Coverage Analysis
═══════════════════════════════════════════════════════════

Total rules/alerts: 8
Tested: 5
Untested: 3

Untested alerts/rules:
  • HighErrorRate
  • job:http_requests:rate5m
  • LowDiskSpace

Run with --fix to generate tests for untested alerts
```

Exit code `1` when untested rules exist and `--fix` wasn't passed, `0`
otherwise (including after a successful `--fix` run).

## Agent workflow

1. Run without `--fix` first to see the coverage gap and decide scope.
2. Run with `--fix` to generate the skeleton file.
3. **Do not stop there.** Open the generated `_test.yml` and replace every
   `TODO` with real series data derived from the rule's actual `expr:` —
   correct metric names, label sets that match what the alert's `labels:`
   expects, and values that genuinely cross (or stay under) the alerting
   threshold. The file will not pass `promtool test rules` until this is
   done.
4. Verify with `promtool test rules <generated-file>` once filled in.
5. Re-run `autogen-promql-tests --rules=... --tests=<generated-file>` to
   confirm the rule now shows as tested.
