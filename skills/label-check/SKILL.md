---
name: label-check
description: Enforce that PromQL expressions (and, optionally, alert definitions) carry a required set of labels — e.g. job, namespace, cluster — to prevent label collisions in multi-tenant Prometheus/Thanos/Cortex/Mimir setups. Use when validating rule YAML for required labels or required alert labels, interactively or as a CI gate. Fully hermetic: no network access or running Prometheus required.
license: BSD-3-Clause
---

# label-check

Static validator that scans `expr:`/`query:` fields (and, in
`--check-alerts` mode, alert `labels:` blocks) for a required set of label
names. Hermetic — reads only the files/stdin you give it.

## When to use this skill

- A monitoring platform is multi-tenant and needs every rule to carry a
  tenant-scoping label (commonly `job`, sometimes `namespace`/`cluster`) to
  avoid one team's alert matching another team's series.
- You want a CI gate that fails a PR if a new/edited rule omits a required
  label.
- You want to enforce that every alert also carries required *alert*
  labels (see the important caveat below about what `--alert-labels`
  actually inspects).

## Setup

```bash
go build -o bin/label-check ./cmd/label-check
# or: go install github.com/conallob/o11y-analysis-tools/cmd/label-check@latest
```

## Usage

```
label-check [options] <file|directory>...
```

Accepts `.yml`/`.yaml` files, directories (walked recursively), **or**
stdin via a literal `-` argument (e.g. for pre-commit hooks piping a
single expression).

| Flag | Default | Effect |
|---|---|---|
| `--labels` | `job` | Comma-separated required labels checked against every PromQL expression. |
| `--check-alerts` | `false` | Also validate each `- alert: Name` block's `labels:` section. |
| `--alert-labels` | `""` | Comma-separated labels required in each alert's `labels:` section. Only takes effect with `--check-alerts`. |

### Typical invocations

```bash
# Default: require 'job' on every expression
label-check --check ./alerts/

# Multiple required labels
label-check --labels=job,namespace,cluster ./alerts/

# Also require alert-level labels
label-check --check-alerts --alert-labels=severity,team ./alerts/

# Single expression via stdin (e.g. pre-commit hook on a diff hunk)
echo 'rate(http_requests_total[5m])' | label-check --labels=job -
```

## Important gotchas

- **`--alert-labels` checks the alert's `labels:` block, not
  `annotations:`.** The CLI's own `-h` example (`--alert-labels=severity,
  grafana_url,runbook`) is misleading here: `runbook` and `grafana_url`
  conventionally live under `annotations:`, not `labels:`, so checking for
  them with this flag will always report them missing even when they're
  correctly set as annotations. Only use `--alert-labels` for names that
  are genuinely emitted as alert **labels** (e.g. `severity`, `team`,
  `tier`); it has no way to validate `annotations:`.
- **Label detection is regex-based**, not a full PromQL parse. It counts a
  label as "present" if it appears either in a `{label="..."}`/`label=~"..."`
  matcher **or** in a `by (...)`/`without (...)` clause anywhere in the
  expression. That means `sum(metric) by (job)` counts as having the `job`
  label even though `job` there is a grouping key, not a selector — the
  underlying series could still span multiple jobs. Don't treat a pass as
  a hard guarantee against label collisions; treat it as a lint, and read
  the actual expression when the stakes are high (e.g. an alert routed by
  tenant).
- There is **no config-file support** (`.label-check.yml`) despite it being
  mentioned in the repo's top-level README — as of this version, required
  labels can only be supplied via `--labels`/`--alert-labels` flags on
  every invocation. If a user asks for config-file support, tell them it's
  aspirational/undocumented-in-code, not implemented.

## Reading the output

```
./alerts/api-alerts.yml:
  Expression: rate(http_requests_total[5m])
    Missing required labels: job
    Line: 12

Found 1 expressions with missing required labels
Required labels: job
```

Exit code `1` if any violation is found (expression-level or, with
`--check-alerts`, alert-level), `0` otherwise.

## Agent workflow

1. Determine the required label set from repo conventions (check for a CI
   workflow invoking `label-check` already, or ask the user) rather than
   assuming the `job`-only default is sufficient for a multi-tenant setup.
2. Run `label-check --labels=<...> <path>`. There is no `--fix` mode — the
   agent must manually edit the flagged expression to add the missing
   label matcher, using the printed `Line:` number to locate it, then
   re-run to confirm.
3. Only add `--check-alerts --alert-labels=...` when the required names are
   confirmed to be alert **labels** in this rule set, not annotations —
   check an existing alert's `labels:` block for the convention first.
