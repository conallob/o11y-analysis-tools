---
name: e2e-alertmanager-test
description: Render end-to-end previews of what an alert notification will actually look like (plain-text email, HTML email, Slack attachment JSON, raw webhook JSON) by replaying a Prometheus unit-test file's expected alerts through a live Alertmanager. Effectively a "print preview" for alerts. Use to review notification formatting/content before merging alert changes, or in CI against an ephemeral Alertmanager to produce a diffable preview artifact. Requires a reachable Alertmanager API and a promtool-style unit-test file as input. Can be made fully hermetic in CI by pointing the ephemeral Alertmanager at a skeleton config that keeps the team's real routing/grouping/inhibition rules but overrides receiver connection details, so no real notification ever leaves the runner.
license: BSD-3-Clause
---

# e2e-alertmanager-test

Takes the `exp_alerts` fixtures out of a Prometheus `promtool`-style unit
test file, POSTs each one to a real Alertmanager's `/api/v2/alerts`
endpoint, and renders what the resulting notification would look like in
several formats. It does not test routing/inhibition logic inside this
tool itself — it exercises whatever routing your target Alertmanager
instance is actually configured with, and shows you the rendered output.

## When to use this skill

- Before merging an alert change, to see the actual notification content
  (subject line, body, Slack attachment) a human on-call would receive —
  not just "does it fire," but "does it read well."
- As a CI step against a **throwaway/ephemeral** Alertmanager container,
  producing a rendered-output artifact that reviewers can diff between
  runs, the same way a UI snapshot test is diffed. Fed a skeleton config
  (see below), this run is fully hermetic — see [Achieving full
  hermeticity in CI](#achieving-full-hermeticity-in-ci).
- This tool's own external dependency is the Alertmanager instance, not
  historical data — unlike `alert-hysteresis`/`stale-alerts-analyzer`, it
  needs no real production history, so a fresh, disposable Alertmanager
  instance is sufficient and the run is fully reproducible.

## Prerequisites

1. **A Prometheus unit-test file** with `exp_alerts` fixtures — typically
   the output of `autogen-promql-tests` (see that skill) with real,
   filled-in labels/annotations, or a hand-written `promtool` test file.
   Test cases whose `exp_alerts` is empty (asserting "does not fire") are
   silently skipped — there's nothing to render for those.
2. **A running Alertmanager** reachable over HTTP, defaulting to
   `http://localhost:9093`. For CI or local preview, run one disposably:
   ```bash
   docker run -d -p 9093:9093 prom/alertmanager
   ```

## Achieving full hermeticity in CI

The `e2e-alertmanager-test` binary itself is never hermetic on its own —
it always needs to POST to a real, reachable Alertmanager. But the
routing config *that Alertmanager instance loads* is entirely under your
control, and that's where every external dependency can be removed:

1. Treat the team's real `alertmanager.yml` — the one that governs
   production `route:` and `inhibit_rules:` — as the source of truth for
   routing/grouping/inhibition logic. That's the logic worth exercising
   in a test.
2. Build a **skeleton config** for CI that keeps that routing tree intact
   but overrides every receiver's connection details — `smtp_smarthost`,
   Slack `api_url`, PagerDuty/webhook URLs and keys — with local, no-op
   values. For example, point `smtp_smarthost` at a local SMTP catcher
   (e.g. MailHog/smtp4dev) instead of the real relay, so the receiver
   still accepts a connection and the message headers can be rendered and
   diffed, but no mail actually leaves the runner.
3. Start the ephemeral Alertmanager for the CI job with the skeleton
   config, not the real one, and point `--alertmanager-url` at it.

With this setup, the Alertmanager under test still runs the real
routing/grouping/inhibition rules — so a routing regression still
surfaces in the diff — but delivery stops at the local catcher: no real
email, Slack message, or PagerDuty incident is ever created. That's what
makes this tool's CI output (rendered notification content, including
SMTP headers) safe to treat as a fully hermetic, deterministic artifact.
Generating and maintaining the skeleton config (importing the team's
routing tree while stripping receiver secrets) is on you — this repo
doesn't ship tooling for that merge/override step, and `--alertmanager-config`
(below) does not perform it either; it only enriches this tool's own
rendered output text.

Keep the skeleton config's routing tree in sync with the real one, ideally
generated from it rather than hand-maintained — if it drifts, a green CI
run stops guaranteeing anything about production routing.

## Setup

```bash
go build -o bin/e2e-alertmanager-test ./cmd/e2e-alertmanager-test
# or: go install github.com/conallob/o11y-analysis-tools/cmd/e2e-alertmanager-test@latest
```

## Usage

```
e2e-alertmanager-test [options]
```

| Flag | Default | Effect |
|---|---|---|
| `--tests` | *(required)* | Path to the Prometheus unit-test YAML file to replay. |
| `--alertmanager-url` | `http://localhost:9093` | Alertmanager API base URL to POST alerts to. |
| `--alertmanager-config` | `""` | Optional path to `alertmanager.yml` — used only to enrich rendered output (e.g. `smtp_from`, receiver name in the routing-info footer), not to actually apply routing rules locally. |
| `--output` | `email` | One of `email`, `email-html`, `slack`, `json`, `full`. |
| `--full` | `false` | Also render the HTML body inside `email`/`email-html` output. |
| `--verbose` | `false` | Print the Alertmanager URL being posted to per alert. |

### Typical invocations

```bash
# Plain-text email preview
e2e-alertmanager-test --tests=./alerts_test.yml --output=email

# Full HTML email rendering
e2e-alertmanager-test --tests=./alerts_test.yml --output=email-html --full

# Slack attachment JSON preview
e2e-alertmanager-test --tests=./alerts_test.yml --output=slack

# Everything, redirected to a diffable artifact for CI
e2e-alertmanager-test --tests=./alerts_test.yml --output=full > notifications.txt
```

## Reading the output

Each test case with non-empty `exp_alerts` becomes one numbered "Test #N",
printing the alert name and then the rendered output in the requested
format(s), followed by a summary:

```
═══════════════════════════════════════════════════════════
Summary
═══════════════════════════════════════════════════════════
Total test cases: 3
Successful: 3
Failed: 0
```

Exit code `1` if any alert failed to POST successfully (e.g. Alertmanager
unreachable or returned non-200), `0` otherwise. Note: "successful" here
means the POST to Alertmanager succeeded, not that routing/inhibition
matched any particular expectation — this tool doesn't assert routing
outcomes, it only renders notification content.

## Agent workflow

1. Confirm a unit-test file with real (non-`TODO`) `exp_alerts` exists —
   if not, point the user at `autogen-promql-tests` first.
2. Confirm an Alertmanager is reachable at `--alertmanager-url` (start a
   disposable one via Docker if none is running and this is a local/CI
   context). **Never point this at a production Alertmanager** — it really
   posts alerts, and a receiver with live connection details will really
   dispatch them. For CI, load the disposable instance with a [skeleton
   config](#achieving-full-hermeticity-in-ci) that keeps real
   routing/grouping/inhibition but swaps receiver connection details for
   local no-op ones, so the run stays hermetic even though it exercises
   real routing logic.
3. Run with `--output=full` to capture every rendering for a
   comprehensive review, or `--output=slack`/`--output=email-html` when a
   user specifically wants to review one channel's format.
4. For CI: redirect output to a file and store it as a build artifact so
   subsequent runs can be diffed to catch unintended notification-content
   regressions from template/label changes.
