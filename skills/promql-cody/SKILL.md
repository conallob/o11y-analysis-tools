---
name: promql-cody
description: Orchestrate all six o11y-analysis-tools PromQL/Alertmanager CLIs together as one workflow for a repo of Prometheus/Alertmanager rules — decide which tools are hermetic and CI-safe (promql-fmt, label-check, autogen-promql-tests, and e2e-alertmanager-test when given a skeleton Alertmanager config) versus interactive-only and judgment-assisted (alert-hysteresis, stale-alerts-analyzer), and in what order to run them end to end. Use this as the entry point whenever a user asks to format, validate, test, preview, tune, or clean up PromQL/Alertmanager rules in this repo — load it before reaching for an individual tool's skill so the right tool and the right mode (interactive vs. CI) get picked. Named after and dedicated to the late Cody Smith.
license: BSD-3-Clause
---

# PromQL-Cody

> **In memoriam.** This skill is named after and dedicated to the late
> [Cody Smith](https://supah.green)

PromQL-Cody is the unified Agent Skill for `o11y-analysis-tools`: it
teaches an agent how to use all six PromQL/Alertmanager CLIs in this repo
together — which are safe to run unattended, which need a human in the
loop, and what order they belong in. Load the per-tool skill
(`skills/<tool>/SKILL.md`) for the exact flags and output format of
whichever tool this points you at; this skill is the map, not the detail.

## The six tools at a glance

| Skill | Purpose | Hermetic? | Best run | External dependency |
|---|---|---|---|---|
| [`promql-fmt`](../promql-fmt/SKILL.md) | Format/validate PromQL multiline style + best-practice lint | Yes | Interactive **and** CI | none |
| [`label-check`](../label-check/SKILL.md) | Enforce required labels on expressions/alerts | Yes | Interactive **and** CI | none |
| [`autogen-promql-tests`](../autogen-promql-tests/SKILL.md) | Generate `promtool`-style unit test skeletons | Yes | Interactive, to bootstrap coverage | none |
| [`e2e-alertmanager-test`](../e2e-alertmanager-test/SKILL.md) | Render alert notifications (email/Slack/JSON) end-to-end — a "print preview" for alerts | Yes\*, with a skeleton config (see its skill) | CI, producing a diffable artifact; also useful interactively | an ephemeral Alertmanager + a Prometheus unit-test file |
| [`alert-hysteresis`](../alert-hysteresis/SKILL.md) | Recommend `for:` duration tuning from real firing history | No | Interactive, as needed | live Prometheus with historical `ALERTS` data |
| [`stale-alerts-analyzer`](../stale-alerts-analyzer/SKILL.md) | Flag alerts that rarely/never fire — "dead code analysis" for alerts | No | Interactive, periodic cleanup | live Prometheus with historical `ALERTS` data |

\* Only when its ephemeral Alertmanager is loaded with a skeleton config
that overrides receiver connection details. Pointed at a real/unmodified
Alertmanager config, it can dispatch real notifications and is not
hermetic.

## Two families of tools

**1. Hermetic static analyzers** — `promql-fmt`, `label-check`,
`autogen-promql-tests`. These are pure functions over the rule files on
disk: same input, same output, every time. They need no network access and
no running Prometheus. This makes them safe to run unattended and to wire
into CI as required, blocking checks on every commit or pull request.

**2. Historical-data tools** — `alert-hysteresis`, `stale-alerts-analyzer`.
Both query a live Prometheus's `ALERTS` metric over `--timeframe`/
`--timehorizon` to reason about *real* firing behavior. Their output is
only as trustworthy as Prometheus's retention window (an alert can look
"stale" simply because the lookback exceeds retention). They are not
reproducible in CI — there's no fixture that substitutes for months of
real production firing patterns — so treat them as interactive,
judgment-assisted tools you run during an alert-quality review, not as a
merge-blocking gate.

`e2e-alertmanager-test` needs a live Alertmanager to POST alerts to, but
that instance can be a short-lived container, and — unlike the two tools
above — its output doesn't depend on irreproducible production history.
Point that ephemeral Alertmanager at a **skeleton config** that preserves
the team's real `route:`/`inhibit_rules:` but overrides every receiver's
connection details (SMTP smarthost, Slack/webhook URLs) with local no-op
values, and the whole run becomes fully hermetic: real routing logic is
still exercised, but no notification ever leaves the runner. That's what
makes it safe to run in CI — its payoff there is a generated
notification-preview artifact that reviewers can diff release over
release, the way a snapshot test diffs UI screenshots. See the
[`e2e-alertmanager-test` skill](../e2e-alertmanager-test/SKILL.md#achieving-full-hermeticity-in-ci)
for how to build that skeleton config.

## Suggested workflow

1. Author or edit alert/recording rules in `*.yml`.
2. `promql-fmt --check` (or `--fix`) — formatting gate.
3. `label-check --labels=job,...` — required-label gate.
4. `autogen-promql-tests --rules=...` — interactively, the first time a rule
   is added, to scaffold a `_test.yml` (then hand-fill the `TODO`s).
5. `promtool test rules ./*_test.yml` — run the generated unit tests.
6. `e2e-alertmanager-test --tests=... --output=full` against an ephemeral
   Alertmanager loaded with a skeleton config in CI — produces a diffable
   notification-preview artifact without sending any real notification.
7. Periodically (e.g. quarterly, or when on-call flags alert fatigue), run
   `alert-hysteresis` and `stale-alerts-analyzer` interactively against
   production Prometheus to tune `for:` durations and cull dead alerts.

Steps 2–3 (and 6, given a disposable Alertmanager) belong in CI. Steps 4 and
7 are deliberately interactive — they either need a human to fill in
generated placeholders or a human to interpret a statistical recommendation
before touching production alerting.

## Agent workflow

1. When a user's request touches PromQL/Alertmanager rules in this repo,
   start here rather than guessing which of the six tools applies — match
   their intent (format? validate labels? bootstrap tests? preview a
   notification? investigate a noisy alert? find dead alerts?) against the
   table above.
2. Load the specific `skills/<tool>/SKILL.md` for the tool(s) that match
   before invoking anything — it has the exact flags, output format, and
   exit-code semantics this skill deliberately omits.
3. Default hermetic tools (`promql-fmt`, `label-check`, `autogen-promql-tests`,
   `e2e-alertmanager-test` with a skeleton config) to CI-safe, automatable
   usage. Default the two historical-data tools to interactive use with a
   human reviewing the recommendation — never wire them into a
   merge-blocking gate.
4. For install/setup instructions (this skill, the six tool skills, or the
   binaries themselves via Homebrew/`go install`/`ghcr.io` containers), see
   [`INSTALLING.md`](../../INSTALLING.md) and [`AGENTS.md`](../../AGENTS.md)
   at the repo root.
