# AGENTS.md

Instructions for AI coding agents working in `o11y-analysis-tools`, following
the cross-tool [AGENTS.md](https://agents.md) convention (read natively by
Codex CLI, Cursor, GitHub Copilot, Windsurf, Aider, Zed, Jules, and others —
Claude Code reads this too, but prefers the richer
[`CLAUDE.md`](CLAUDE.md) in this repo when both are present).

## What this repo is

Six Go CLIs for working with PromQL and Alertmanager rules:
`promql-fmt`, `label-check`, `alert-hysteresis`, `autogen-promql-tests`,
`e2e-alertmanager-test`, `stale-alerts-analyzer`. Source lives under
`cmd/<tool>/` (entry points), `pkg/` (public libraries), and `internal/`
(private libraries).

## Build, test, lint

```bash
go mod download
go test -v -race -coverprofile=./coverage.txt ./...
go build -o bin/ ./cmd/promql-fmt
go build -o bin/ ./cmd/label-check
go build -o bin/ ./cmd/alert-hysteresis
go build -o bin/ ./cmd/autogen-promql-tests
go build -o bin/ ./cmd/e2e-alertmanager-test
go build -o bin/ ./cmd/stale-alerts-analyzer
golangci-lint run   # v2.8+; config in .golangci.yml
```

Or via `make build`, `make test`, `make lint`. Run the full suite before
committing — CI (`.github/workflows/test.yml`) runs the same commands with
`-race` on Go 1.24/1.25 across ubuntu/macos.

## Agent Skills for this repo

This repo packages its own tools as [Agent Skills](https://agentskills.io) —
`SKILL.md` files with YAML frontmatter that teach an agent exactly how to
invoke each tool, parse its output, and where it fits in a workflow. Load
these instead of re-deriving usage from `cmd/*/main.go` on your own:

- **[`PROMQL-CODY.md`](PROMQL-CODY.md)** — unified reference: all six tools,
  which are hermetic/CI-safe vs. interactive-only, and the recommended
  end-to-end workflow (format → label-check → generate tests → preview
  notifications → periodically tune/cull alerts).
- **`skills/<tool>/SKILL.md`** — one per tool, each independently usable:
  [`promql-fmt`](skills/promql-fmt/SKILL.md),
  [`label-check`](skills/label-check/SKILL.md),
  [`autogen-promql-tests`](skills/autogen-promql-tests/SKILL.md),
  [`e2e-alertmanager-test`](skills/e2e-alertmanager-test/SKILL.md),
  [`alert-hysteresis`](skills/alert-hysteresis/SKILL.md),
  [`stale-alerts-analyzer`](skills/stale-alerts-analyzer/SKILL.md).

**Read the relevant skill before running its tool.** `promql-fmt` and
`label-check` are hermetic (safe to run freely, no external dependencies).
`alert-hysteresis` and `stale-alerts-analyzer` require a live Prometheus and
are interactive/judgment-assisted, not something to run unattended or wire
into CI. `e2e-alertmanager-test` needs a reachable Alertmanager — see its
skill for how to make that hermetic with a skeleton config before using it
in CI.

## Installing these skills via a marketplace

Don't just read the skill files in place — install them into your own
skills directory so they load automatically in future sessions, in this
repo or elsewhere. Full details, including community/official marketplace
listings, are in **[`INSTALLING.md`](INSTALLING.md)**; short version:

- **Any agentskills.io-compliant agent**: copy a `skills/<tool>/` directory
  as-is into that agent's skills path — e.g. `~/.codex/skills/<tool>/`
  (OpenAI Codex CLI), `.cursor/skills/<tool>/` (Cursor), or
  `~/.claude/skills/<tool>/` / `.claude/skills/<tool>/` (Claude Code,
  personal/project respectively). No file edits needed; the format is the
  same across compliant agents.
- **Claude Code plugin marketplace**: this repo ships a working
  `.claude-plugin/marketplace.json`. From inside Claude Code:
  ```
  /plugin marketplace add conallob/o11y-analysis-tools
  /plugin install promql-fmt@o11y-analysis-tools
  ```
  (repeat per tool — `label-check`, `autogen-promql-tests`,
  `e2e-alertmanager-test`, `alert-hysteresis`, `stale-alerts-analyzer` —
  or non-interactively via `claude plugin marketplace add ...` /
  `claude plugin install ...`).
- **Community/official marketplaces**: see `INSTALLING.md` for
  `claudemarketplaces.com`, `skillsmp.com`, `aitmpl.com`, and the
  `anthropics/claude-plugins-official` / `anthropics/skills` submission
  paths.

## Conventions

- Go stdlib-first; minimize external dependencies (see `go.mod`).
- `go fmt` enforced; exported identifiers documented; errors wrapped with
  `fmt.Errorf("...: %w", err)`, never dropped.
- Every new function/behavior change needs a table-driven test alongside it
  (`*_test.go`, same package or `package_test`).
- Don't run `--fix`/`--fix`-equivalent flags unattended in CI — see the
  individual skill files for which flags are safe to automate and which
  need a human reviewing the diff first.
