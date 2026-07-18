# PromQL-Cody

> **In memoriam.** This skill is named after and dedicated to the late
> [Cody Smith](https://supah.green)

**PromQL-Cody** is the unified Agent Skill for `o11y-analysis-tools`: it
teaches a coding agent how to use all six PromQL/Alertmanager CLIs in this
repo together — which are safe to run unattended, which need a human in
the loop, and what order they belong in.

The canonical, installable version of this skill lives at
**[`skills/promql-cody/SKILL.md`](skills/promql-cody/SKILL.md)** — a real
`SKILL.md` with YAML frontmatter, installable the same way as the six
per-tool skills below (see [`INSTALLING.md`](INSTALLING.md)). This root
file is a short landing page; read the skill file itself for the full
tool-comparison table, the two-families breakdown, and the suggested
end-to-end workflow.

## The six tools

Each tool also has its own packaged
[Agent Skill](https://agentskills.io) under `skills/<tool>/SKILL.md` — a
self-contained Markdown file with YAML frontmatter that teaches a coding
agent exactly how to invoke that one tool, parse its output, and where it
fits in a workflow:

- [`promql-fmt`](skills/promql-fmt/SKILL.md) — format/validate PromQL, hermetic
- [`label-check`](skills/label-check/SKILL.md) — enforce required labels, hermetic
- [`autogen-promql-tests`](skills/autogen-promql-tests/SKILL.md) — scaffold `promtool` unit tests, hermetic
- [`e2e-alertmanager-test`](skills/e2e-alertmanager-test/SKILL.md) — notification "print preview", hermetic with a skeleton config
- [`alert-hysteresis`](skills/alert-hysteresis/SKILL.md) — tune `for:` durations, interactive
- [`stale-alerts-analyzer`](skills/stale-alerts-analyzer/SKILL.md) — find dead alerts, interactive

Every skill directory (including `skills/promql-cody/`) is independently
publishable and installable on its own; you can copy any single
`skills/<tool>/` directory out of this repo without the others, or install
all seven together.

## Using PromQL-Cody and its skills

- Each `skills/<tool>/SKILL.md` (and `skills/promql-cody/SKILL.md`) is a
  standalone Agent Skill: copy it into `.claude/skills/<tool>/SKILL.md`
  (or an equivalent skills directory for another agent host) to make its
  instructions available independently of the rest of this repo.
- For every install path — local copy, the Claude Code plugin marketplace
  this repo ships (`.claude-plugin/marketplace.json`), or the equivalent
  mechanism in other agentskills.io-compliant agents (OpenAI Codex,
  Cursor, etc.) — see [`INSTALLING.md`](INSTALLING.md).
- Every binary is built from `cmd/<tool>/`. Build all of them with
  `make build` (binaries land in `./bin/`), build one with
  `go build -o bin/<tool> ./cmd/<tool>`, install a released version with
  `go install github.com/conallob/o11y-analysis-tools/cmd/<tool>@latest`,
  via Homebrew (`brew install conallob/tap/o11y-analysis-tools`), or pull
  a container image from `ghcr.io/conallob/<tool>` — see
  [`AGENTS.md`](AGENTS.md#installing-the-binaries) for details.
- Every tool prints full flag documentation via `-h`; the individual skill
  files describe only what an agent needs to decide *when* and *how* to
  invoke each flag, not a full flag dump.
