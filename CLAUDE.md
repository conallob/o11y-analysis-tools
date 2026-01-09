# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is a **Go-based observability analysis tools repository** containing three CLI tools for PromQL-compatible monitoring systems:

1. **promql-fmt**: PromQL expression formatter and validator
2. **label-check**: Label standards enforcement tool
3. **alert-hysteresis**: Alert firing pattern analyzer

The repository uses standard Go project structure, GitHub Actions for CI/CD, and GoReleaser for multi-platform releases.

## Git Workflow

**Default workflow: Create feature branches off `main`.**

- Use descriptive branch names: `feat/description`, `fix/description`, `chore/description`
- GitHub Actions CI runs on all branches matching `claude/**` and on `main`
- Pull requests should target `main` branch
- Releases are automated via git tags matching `v*` pattern

## Key Architecture Patterns

### Go Module Structure

```
cmd/                    # CLI entry points (main packages)
  promql-fmt/          # PromQL formatter tool
  label-check/         # Label validation tool
  alert-hysteresis/    # Alert analysis tool
pkg/                   # Public libraries (importable by external projects)
  formatting/          # PromQL formatting logic
internal/              # Private libraries (not importable externally)
  promql/             # PromQL parsing utilities
  alertmanager/       # Prometheus/Alertmanager integration
```

### Testing Strategy

- Unit tests co-located with source: `*_test.go` files
- Test packages mirror source structure
- Use table-driven tests for multiple test cases
- Coverage tracked via Codecov on ubuntu-latest + Go 1.21

## CI/CD Auto-Detection and Local Execution

**To minimize Claude token usage and enable rapid iteration, always detect and run CI checks locally before committing.**

### Step 1: Detect CI Configuration

Read `.github/workflows/test.yml` to identify:
- Test commands (Go version matrix, test flags, build commands)
- Lint commands (golangci-lint version and configuration)
- Other checks (GoReleaser config validation)

### Step 2: Execute Checks Locally

**Run these commands in the exact same way as CI:**

```bash
# 1. Download dependencies (as CI does)
go mod download

# 2. Run tests with race detection and coverage (matches CI exactly)
go test -v -race -coverprofile=./coverage.txt ./...

# 3. Build all binaries (matches CI build step)
go build -o bin/ ./cmd/promql-fmt
go build -o bin/ ./cmd/label-check
go build -o bin/ ./cmd/alert-hysteresis

# 4. Run golangci-lint (matches CI lint job)
golangci-lint run

# 5. Validate GoReleaser config (matches CI goreleaser-check job)
# Note: Only if goreleaser is installed locally
goreleaser check 2>/dev/null || echo "goreleaser not installed, will validate in CI"
```

### Step 3: Automated Check Script

For convenience, you can create and use this script to run all CI checks:

```bash
#!/bin/bash
# scripts/ci-check.sh - Run all CI checks locally

set -e

echo "==> Running CI checks locally..."

echo "==> 1. Downloading dependencies..."
go mod download

echo "==> 2. Running tests with race detection..."
go test -v -race -coverprofile=./coverage.txt ./...

echo "==> 3. Building binaries..."
mkdir -p bin
go build -o bin/ ./cmd/promql-fmt
go build -o bin/ ./cmd/label-check
go build -o bin/ ./cmd/alert-hysteresis

echo "==> 4. Running golangci-lint..."
if command -v golangci-lint > /dev/null; then
    golangci-lint run
else
    echo "WARNING: golangci-lint not installed, skipping lint check"
    echo "Install from: https://golangci-lint.run/usage/install/"
fi

echo "==> 5. Validating GoReleaser config..."
if command -v goreleaser > /dev/null; then
    goreleaser check
else
    echo "WARNING: goreleaser not installed, skipping config validation"
fi

echo "==> All CI checks passed! ✓"
```

### Step 4: Before Every Commit

**ALWAYS run these checks before creating a commit:**

```bash
# Quick check (tests + lint)
go test ./... && golangci-lint run

# Full check (matches CI exactly)
./scripts/ci-check.sh

# Or use Makefile targets
make test && make lint
```

### Why This Matters

1. **Token Efficiency**: Catching errors locally avoids expensive back-and-forth with CI failures
2. **Faster Iteration**: Local checks run in seconds vs. minutes for CI
3. **Context Preservation**: Fixing issues immediately while context is fresh
4. **CI Confidence**: If local checks pass, CI will almost certainly pass

## Working with This Repository

### Development Workflow

1. **Before making changes:**
   ```bash
   # Ensure dependencies are current
   go mod download
   go mod tidy

   # Run existing tests to ensure baseline
   go test ./...
   ```

2. **While implementing features:**
   ```bash
   # Run tests for specific package
   go test ./pkg/formatting -v
   go test ./internal/promql -v

   # Quick format check
   go fmt ./...
   ```

3. **Before committing:**
   ```bash
   # Run full CI check suite
   go mod download
   go test -v -race -coverprofile=./coverage.txt ./...
   go build -o bin/ ./cmd/promql-fmt
   go build -o bin/ ./cmd/label-check
   go build -o bin/ ./cmd/alert-hysteresis
   golangci-lint run

   # Or use Makefile
   make test && make lint && make build
   ```

4. **Creating commits:**
   ```bash
   git add .
   git commit -m "feat: description

   Detailed explanation of changes.

   Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
   ```

### Testing Guidelines

**Always write tests when:**
- Adding new functions or methods
- Modifying existing logic
- Fixing bugs (add regression test)

**Test file naming:**
- Unit tests: `filename_test.go` in same package
- Test package name: same as source package or `package_test` for black-box tests

**Running tests:**
```bash
# All tests
go test ./...

# With verbose output
go test -v ./...

# Specific package
go test ./pkg/formatting

# With coverage
go test -coverprofile=coverage.txt ./...
go tool cover -html=coverage.txt

# With race detection (matches CI)
go test -race ./...
```

### Linting

The project uses golangci-lint v2.8 with custom configuration in `.golangci.yml`.

**Always run linter before committing:**
```bash
# Run with project config
golangci-lint run

# Auto-fix issues where possible
golangci-lint run --fix

# Check specific directory
golangci-lint run ./cmd/...
```

**Configuration notes:**
- Version must be v2.8+ (specified in `.golangci.yml` version field)
- Config uses v2 format (not compatible with golangci-lint < v2.8)
- Formatters (gofmt, goimports) are separate from linters in v2

### Building and Installing

```bash
# Build all tools to bin/ directory
make build

# Build specific tool
go build -o bin/promql-fmt ./cmd/promql-fmt

# Install to $GOPATH/bin
make install

# Clean build artifacts
make clean
```

### PromQL Best Practices (for promql-fmt development)

The `promql-fmt` tool validates against official Prometheus best practices from:
- https://prometheus.io/docs/practices/naming/
- https://prometheus.io/docs/practices/instrumentation/
- https://prometheus.io/docs/practices/alerting/

**Key validations implemented:**
1. **Naming conventions:**
   - snake_case for metric names (not camelCase)
   - Application prefix required (e.g., `app_metric_name`)
   - Base units (use `_seconds`, not `_milliseconds`)
   - Counter suffix `_total` for counters

2. **Instrumentation:**
   - `rate()` required on counters
   - Division protection (check denominator != 0)

3. **Aggregation consistency:**
   - All `by`/`without` clauses in same position (prefix or postfix)
   - Example: `sum(metric) by (label)` vs `sum by (label) (metric)`

4. **Line length:**
   - Configurable via `--disable-line-length` flag
   - Useful for long metric names in recording rules

### GoReleaser Configuration

The project uses GoReleaser v2 for releases:

**Key configuration points:**
- Config file: `.goreleaser.yml` (version: 2 format)
- Uses Docker buildx with deprecated `dockers`/`docker_manifests` (not `dockers_v2`)
- Reason: Podman support requires GoReleaser Pro; using buildx with Podman backend instead
- Homebrew formula published to `conallob/homebrew-tap`
- Container images pushed to `ghcr.io/conallob/*`

**Deprecation warnings (expected):**
- `dockers` and `docker_manifests` deprecated in favor of `dockers_v2`
- Keeping deprecated version because it supports `use: buildx` in free version
- Will migrate to `dockers_v2` when Podman support added (currently Pro-only)

**Testing releases locally:**
```bash
# Validate config
goreleaser check

# Build snapshot (no publish)
goreleaser release --snapshot --clean

# Test specific builds
goreleaser build --single-target --snapshot
```

## Important Files

### Configuration Files

- **go.mod/go.sum**: Go module dependencies
- **.golangci.yml**: golangci-lint v2 configuration
- **.goreleaser.yml**: Release automation config (GoReleaser v2)
- **Makefile**: Build automation targets

### CI/CD Files

- **.github/workflows/test.yml**: Test/lint/build workflow (runs on push/PR)
- **.github/workflows/release.yml**: Release workflow (runs on version tags)

### Source Files

- **cmd/*/main.go**: CLI entry points for each tool
- **pkg/formatting/promql.go**: Core PromQL formatting logic
- **internal/**: Private implementation packages

## Platform-Specific Notes

### Cross-Platform Support

The tools build for:
- **Linux**: amd64, arm64
- **macOS (Darwin)**: amd64, arm64
- **Windows**: amd64, arm64

### CI Matrix Testing

GitHub Actions tests on:
- **OS**: ubuntu-latest, macos-latest, windows-latest
- **Go versions**: 1.21, 1.22

**Windows-specific considerations:**
- Coverage file path must use `./` prefix: `./coverage.txt`
- Without prefix, Windows interprets `.txt` as package name

### Container Images

Built with Docker buildx using Podman backend:
- Multi-arch support (linux/amd64, linux/arm64)
- Separate images per tool
- Multi-arch manifests for version and latest tags

## Common Tasks for Claude

### Adding a New Feature

1. **Read existing tests to understand patterns:**
   ```bash
   # Example: adding validation to promql-fmt
   Read pkg/formatting/promql_test.go
   ```

2. **Implement feature with tests:**
   ```go
   // Add implementation in pkg/formatting/promql.go
   // Add tests in pkg/formatting/promql_test.go
   ```

3. **Run local CI checks:**
   ```bash
   go test -v -race ./pkg/formatting
   golangci-lint run ./pkg/formatting
   go test -v -race ./...  # Full suite
   ```

4. **Commit when all checks pass:**
   ```bash
   git add .
   git commit -m "feat(promql-fmt): add new validation"
   ```

### Fixing a Bug

1. **Write failing test that reproduces bug:**
   ```go
   // Add regression test in appropriate *_test.go
   func TestBugFix(t *testing.T) {
       // Test case that currently fails
   }
   ```

2. **Fix the bug:**
   ```go
   // Implement fix in source file
   ```

3. **Verify fix:**
   ```bash
   go test -v ./pkg/formatting  # Should now pass
   go test -race ./...            # Full test suite
   golangci-lint run             # No new issues
   ```

4. **Commit with reference to issue:**
   ```bash
   git commit -m "fix: resolve issue with metric name validation

   Fixes incorrect handling of metric names with underscores.
   Added regression test.

   Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
   ```

### Updating Dependencies

```bash
# Update all dependencies to latest compatible versions
go get -u ./...
go mod tidy

# Update specific dependency
go get github.com/example/pkg@latest
go mod tidy

# Verify everything still works
go test ./...
go build ./cmd/...
```

### Creating a Release

1. **Ensure all tests pass:**
   ```bash
   go test -v -race ./...
   golangci-lint run
   goreleaser check
   ```

2. **Create and push tag:**
   ```bash
   git tag -a v1.2.3 -m "Release v1.2.3: description"
   git push origin v1.2.3
   ```

3. **GitHub Actions will automatically:**
   - Build binaries for all platforms
   - Create deb/rpm packages
   - Build and push container images
   - Update Homebrew formula
   - Create GitHub release

## Error Patterns and Solutions

### "go.sum mismatch" errors
```bash
go mod tidy
go mod download
```

### "golangci-lint config invalid" errors
- Check `.golangci.yml` version field is `"2"` (string, not number)
- Ensure golangci-lint version is v2.8+
- Separate linters and formatters in v2 config

### "GoReleaser check failed" errors
- Validate config: `goreleaser check`
- Check deprecation warnings (some are expected)
- Ensure `version: 2` in `.goreleaser.yml`

### "race detector" errors in tests
- Indicates potential data race
- Fix by adding proper synchronization (mutexes, channels)
- Never disable race detector to hide issues

### Windows test failures with coverage
- Use `./coverage.txt` not `coverage.txt`
- Windows needs explicit relative path prefix

## Style Guidelines

### Code Style

- **Formatting**: Use `go fmt` (enforced by CI)
- **Comments**:
  - Package comments required (enforced by revive linter)
  - Public functions should have doc comments
  - Use `//` for single-line, `/* */` for multi-line
- **Naming**:
  - Use camelCase for variables (not snake_case)
  - Use PascalCase for exported identifiers
  - Avoid single-letter variables except in loops
- **Error handling**:
  - Always check errors
  - Don't capitalize error strings
  - Use `fmt.Errorf` with `%w` for wrapping

### Commit Messages

```
<type>: <short summary>

<detailed description>

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

**Types:** feat, fix, docs, test, refactor, chore, perf

## Dependencies

**Standard library focused** - minimize external dependencies:
- No web frameworks required
- YAML parsing: use standard library or minimal deps
- HTTP clients: use `net/http` from stdlib

**Current key dependencies** (check go.mod for versions):
- Prometheus client/API libraries (for alert-hysteresis)
- YAML parsing (for reading Prometheus rules)

## Troubleshooting

### Tests pass locally but fail in CI

1. **Check Go version mismatch:**
   ```bash
   go version  # Should match CI matrix (1.21 or 1.22)
   ```

2. **Check OS-specific issues:**
   - Test on all platforms if possible
   - Watch for path separator differences (/ vs \)
   - Windows line endings (CRLF vs LF)

3. **Check race detector:**
   ```bash
   go test -race ./...  # CI always uses -race
   ```

### golangci-lint version mismatch

CI uses v2.8. Install locally:
```bash
# macOS
brew install golangci-lint

# Linux/WSL
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.8.0

# Verify version
golangci-lint --version  # Should be v2.8.x
```

## Resources

- **Prometheus Best Practices**: https://prometheus.io/docs/practices/
- **Go Testing**: https://go.dev/doc/tutorial/add-a-test
- **golangci-lint**: https://golangci-lint.run/
- **GoReleaser**: https://goreleaser.com/
- **GitHub Actions**: https://docs.github.com/en/actions

## Critical Reminders

1. ✅ **ALWAYS run local CI checks before committing** (saves tokens!)
2. ✅ **ALWAYS write tests for new code** (required by CI)
3. ✅ **ALWAYS run golangci-lint** (CI will fail otherwise)
4. ✅ **ALWAYS use go fmt** (enforced automatically)
5. ✅ **ALWAYS check coverage** (aim for > 80% on new code)
6. ⚠️ **NEVER skip race detector** (`-race` flag)
7. ⚠️ **NEVER commit without testing** (even for "simple" changes)
8. ⚠️ **NEVER ignore linter warnings** (fix or explicitly disable with comment)
