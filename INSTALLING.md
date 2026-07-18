# Installing the PromQL-Cody skills

This repo publishes seven installable Agent Skills:
[`skills/promql-cody/SKILL.md`](skills/promql-cody/SKILL.md) (the unified
agent that orchestrates the other six) and the six standalone
`skills/<tool>/SKILL.md` files. All seven are plain
[Agent Skills](https://agentskills.io) — a folder with a `SKILL.md`
containing YAML frontmatter (`name`, `description`) plus instructions. That
format is an open, cross-vendor standard, not something specific to Claude,
so the same seven directories install into any agent that speaks it. This
doc covers three ways to get them: local copy (works everywhere, zero
setup), the Claude Code plugin marketplace (this repo ships a working
manifest), and the equivalent mechanisms in other agents.

[`PROMQL-CODY.md`](PROMQL-CODY.md) at the repo root is a short landing
page pointing at `skills/promql-cody/SKILL.md` — read it for an overview,
but install the `skills/promql-cody/` directory, not the root file.

## 1. Claude Code

### Quickest: copy the skill directory

No marketplace needed. Copy any `skills/<tool>/` directory into a skills
path Claude Code already watches:

```bash
# Personal — available in every project
cp -r skills/promql-fmt ~/.claude/skills/promql-fmt

# Project — checked into this repo, available to anyone who clones it
cp -r skills/promql-fmt .claude/skills/promql-fmt
```

Claude Code picks up new/edited skills in `~/.claude/skills/` and
`.claude/skills/` without a restart. Repeat per tool, or copy all six at
once (`cp -r skills/* ~/.claude/skills/`).

### Via the plugin marketplace

This repo ships a `.claude-plugin/marketplace.json` (validated with
`claude plugin validate .`) that lists each of the six tools as an
independently installable plugin. From inside Claude Code:

```
/plugin marketplace add conallob/o11y-analysis-tools
/plugin install promql-cody@o11y-analysis-tools
/plugin install promql-fmt@o11y-analysis-tools
/plugin install label-check@o11y-analysis-tools
/plugin install autogen-promql-tests@o11y-analysis-tools
/plugin install e2e-alertmanager-test@o11y-analysis-tools
/plugin install alert-hysteresis@o11y-analysis-tools
/plugin install stale-alerts-analyzer@o11y-analysis-tools
```

Or non-interactively (scripting/CI):

```bash
claude plugin marketplace add conallob/o11y-analysis-tools
claude plugin install promql-fmt@o11y-analysis-tools
```

Install only the tools you need — each plugin is independent. Refresh
after a repo update with `/plugin marketplace update o11y-analysis-tools`.

### Via a community marketplace listing

Third-party aggregators such as [claudemarketplaces.com](https://claudemarketplaces.com),
[skillsmp.com](https://skillsmp.com), and [aitmpl.com](https://aitmpl.com)
index public `marketplace.json` files (this repo's included). Check each
site's own submission page if you want a listing added explicitly rather
than waiting for indexing — submission mechanics differ per site and
change over time, so treat their current docs as authoritative rather than
this file.

### Via the official Anthropic marketplace

Anthropic's own curated marketplace lives at
[anthropics/claude-plugins-official](https://github.com/anthropics/claude-plugins-official)
and its example-skills repo at [anthropics/skills](https://github.com/anthropics/skills).
Both accept contributions by pull request; read each repo's own
`CONTRIBUTING` guidance before opening one, since inclusion criteria and
review process are theirs to define, not something this repo controls.

## 2. Other agents (the agentskills.io open standard)

Anthropic published Agent Skills as an open, cross-vendor standard
(spec at [agentskills.io](https://agentskills.io)). Because our
`skills/<tool>/SKILL.md` files only rely on the baseline of that spec —
YAML frontmatter with `name`/`description`, then plain-text instructions,
no Claude Code-only frontmatter fields — they're portable as-is to any
agent that implements the standard. As of mid-2026 that list includes
OpenAI Codex, GitHub Copilot, Cursor, Gemini CLI, Goose, and OpenCode,
among others; check agentskills.io's current showcase for the
full, up-to-date list rather than relying on a snapshot here.

### OpenAI Codex CLI

Codex reads skills from `~/.codex/skills/<name>/` (personal) or
`.codex/skills/<name>/` (project):

```bash
mkdir -p ~/.codex/skills
cp -r skills/promql-fmt ~/.codex/skills/promql-fmt
```

Codex auto-detects new skill directories. OpenAI also maintains a curated
catalog at [openai/skills](https://github.com/openai/skills), installable
from inside Codex with `$skill-installer <name>` — that only applies to
skills actually published in their catalog, so use the direct-copy method
above unless/until these are submitted there.

### Cursor

Cursor loads skills from `.cursor/skills/<name>/` in the project:

```bash
mkdir -p .cursor/skills
cp -r skills/promql-fmt .cursor/skills/promql-fmt
```

### Everything else that speaks the standard

For any other agentskills.io-compliant tool, the pattern is the same:
find that tool's skills directory (check its own docs — paths vary per
vendor) and copy the `skills/<tool>/` folder there unchanged. Since the
spec is the lowest common denominator, no file edits should be needed to
move a skill from one compliant agent to another.

## Which files are and aren't installable skills

- `skills/promql-cody/`, `skills/promql-fmt/`, `skills/label-check/`,
  `skills/autogen-promql-tests/`, `skills/e2e-alertmanager-test/`,
  `skills/alert-hysteresis/`, `skills/stale-alerts-analyzer/` — each is a
  real Agent Skill directory (`SKILL.md` + frontmatter) and can be
  copied/installed independently, as described above. Install
  `skills/promql-cody/` for the cross-tool orchestration skill, or any of
  the other six for a single tool's usage instructions.
- `PROMQL-CODY.md` at the repo root is a plain landing-page doc, not a
  `<dir>/SKILL.md` skill directory itself, so it isn't independently
  installable via the mechanisms above — it exists for humans browsing the
  repo and points at `skills/promql-cody/SKILL.md`, which is what you
  actually install.
