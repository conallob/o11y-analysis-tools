# PromQL-Cody

**PromQL-Cody** is the unified Agent Skill for `o11y-analysis-tools`: it
teaches a coding agent how to use all six PromQL/Alertmanager CLIs in this
repo together — which are safe to run unattended, which need a human in
the loop, and what order they belong in.

This repo ships six focused Go CLIs for working with PromQL and Alertmanager
rules. Each one also has its own packaged
[Agent Skill](https://code.claude.com/docs) under `skills/<tool>/SKILL.md` —
a self-contained Markdown file with YAML frontmatter that teaches a coding
agent exactly how to invoke that one tool, parse its output, and where it
fits in a workflow. Each skill directory is independently publishable to a
skills marketplace; you can copy any single `skills/<tool>/` directory out
of this repo without the others, or bring this whole file along as
PromQL-Cody to get the combined picture.

## The six tools at a glance

| Skill | Purpose | Hermetic? | Best run | External dependency |
|---|---|---|---|---|
| [`promql-fmt`](skills/promql-fmt/SKILL.md) | Format/validate PromQL multiline style + best-practice lint | Yes | Interactive **and** CI | none |
| [`label-check`](skills/label-check/SKILL.md) | Enforce required labels on expressions/alerts | Yes | Interactive **and** CI | none |
| [`autogen-promql-tests`](skills/autogen-promql-tests/SKILL.md) | Generate `promtool`-style unit test skeletons | Yes | Interactive, to bootstrap coverage | none |
| [`e2e-alertmanager-test`](skills/e2e-alertmanager-test/SKILL.md) | Render alert notifications (email/Slack/JSON) end-to-end — a "print preview" for alerts | Semi (needs a running Alertmanager, but it can be an ephemeral/disposable one) | CI, producing a diffable artifact; also useful interactively | a running Alertmanager + a Prometheus unit-test file |
| [`alert-hysteresis`](skills/alert-hysteresis/SKILL.md) | Recommend `for:` duration tuning from real firing history | No | Interactive, as needed | live Prometheus with historical `ALERTS` data |
| [`stale-alerts-analyzer`](skills/stale-alerts-analyzer/SKILL.md) | Flag alerts that rarely/never fire — "dead code analysis" for alerts | No | Interactive, periodic cleanup | live Prometheus with historical `ALERTS` data |

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

`e2e-alertmanager-test` sits between the two families: it does need a
live Alertmanager to POST alerts to, but that instance can be a short-lived
container seeded purely from the fixtures in a Prometheus unit-test file
(the same file `autogen-promql-tests` scaffolds). Because that dependency
is disposable and reproducible, this tool *can* run in CI — its payoff
there is a generated notification-preview artifact that reviewers can diff
release over release, the way a snapshot test diffs UI screenshots.

## Suggested workflow

1. Author or edit alert/recording rules in `*.yml`.
2. `promql-fmt --check` (or `--fix`) — formatting gate.
3. `label-check --labels=job,...` — required-label gate.
4. `autogen-promql-tests --rules=...` — interactively, the first time a rule
   is added, to scaffold a `_test.yml` (then hand-fill the `TODO`s).
5. `promtool test rules ./*_test.yml` — run the generated unit tests.
6. `e2e-alertmanager-test --tests=... --output=full` against an ephemeral
   Alertmanager in CI — produces a diffable notification-preview artifact.
7. Periodically (e.g. quarterly, or when on-call flags alert fatigue), run
   `alert-hysteresis` and `stale-alerts-analyzer` interactively against
   production Prometheus to tune `for:` durations and cull dead alerts.

Steps 2–3 (and 6, given a disposable Alertmanager) belong in CI. Steps 4 and
7 are deliberately interactive — they either need a human to fill in
generated placeholders or a human to interpret a statistical recommendation
before touching production alerting.

## Using PromQL-Cody and its skills

- Each `skills/<tool>/SKILL.md` is a standalone Agent Skill: copy it into
  `.claude/skills/<tool>/SKILL.md` (or an equivalent skills directory for
  another agent host) to make that one tool's usage instructions available
  independently of the rest of this repo.
- Every binary is built from `cmd/<tool>/`. Build all of them with
  `make build` (binaries land in `./bin/`), build one with
  `go build -o bin/<tool> ./cmd/<tool>`, or install a released version with
  `go install github.com/conallob/o11y-analysis-tools/cmd/<tool>@latest`.
- Every tool prints full flag documentation via `-h`; the individual skill
  files describe only what an agent needs to decide *when* and *how* to
  invoke each flag, not a full flag dump.
