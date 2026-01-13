# o11y-analysis-tools

[![Test](https://github.com/conallob/o11y-analysis-tools/workflows/Test/badge.svg)](https://github.com/conallob/o11y-analysis-tools/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/conallob/o11y-analysis-tools)](https://goreportcard.com/report/github.com/conallob/o11y-analysis-tools)
[![codecov](https://codecov.io/gh/conallob/o11y-analysis-tools/branch/main/graph/badge.svg)](https://codecov.io/gh/conallob/o11y-analysis-tools)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)](https://go.dev/)
[![License](https://img.shields.io/badge/License-BSD%203--Clause-blue.svg)](LICENSE)
[![Buy Me A Coffee](https://img.shields.io/badge/Buy%20Me%20A%20Coffee-support-yellow.svg?style=flat&logo=buy-me-a-coffee)](https://www.buymeacoffee.com/conallob)

A collection of static analysis and testing tools for PromQL-compatible monitoring systems. All tools are written in Go and designed for use in CI/CD workflows with `--check` functionality by default.

## Background

This collection of static analysis tools are intended to help:
* maintain PromQL rules, by keeping them readable, consistent, unit tested and most importantly, approachable by non PromQL experts
* identify common bugs regarding labels, time controls, etc
* maintain high quality, actionable alerts, including with the ability to preview how an alert would render
* identifying alerts in need of refinement and even deletion once they are no longer useful


Each of these tools is inspired from the mistakes I've made when writing PromQL rules, issues seen when peer reviewing PromQL PRs or providing PromQL
consultations, during my tenure as a member of [Google SRE](https://sre.google)

## Tools

### 1. promql-fmt - PromQL Expression Formatter

Statically analyzes and formats PromQL expressions for proper multiline formatting.

**Features:**
- Checks PromQL expressions for multiline formatting standards
- Automatically formats long or complex expressions for better readability
- Integrates with CI to enforce formatting standards

**Usage:**

```bash
# Check formatting (default mode, exits 1 if issues found)
promql-fmt --check ./alerts/

# Automatically fix formatting issues
promql-fmt --fix ./alerts/
promql-fmt --fmt ./alerts/  # alias for --fix

# Verbose output
promql-fmt --verbose --check ./prometheus/
```

**Example:**

Before:
```yaml
expr: sum(rate(http_requests_total{job="api",status=~"5.."}[5m])) by (instance) / sum(rate(http_requests_total{job="api"}[5m])) by (instance)
```

After:
```yaml
expr: |
  sum (
    rate(http_requests_total{job="api",status=~"5.."}[5m])
  )
    /
  sum by (instance) (
    rate(http_requests_total{job="api"}[5m])
  )
```

Note: The formatter automatically removes redundant aggregation clauses from the left operand when both operands share the same `by` clause, following PromQL best practices where only the final operand needs the aggregation.

### 2. label-check - Label Standards Enforcement

Enforces required labels in PromQL expressions to prevent collisions in multi-tenant observability platforms.

**Features:**
- Validates that all PromQL expressions include required labels
- Default: checks for `job` label to prevent tenant collisions
- Configurable for any set of required labels
- Detailed violation reporting with line numbers

**Usage:**

```bash
# Check for default 'job' label
label-check --check ./alerts/

# Check for multiple required labels
label-check --labels=job,namespace ./alerts/

# Check specific file
label-check --labels=job,cluster alerts.yml
```

**Example Output:**

```
./alerts/api-alerts.yml:
  Expression: rate(http_requests_total[5m])
    Missing required labels: job
    Line: 12

Found 1 expressions with missing required labels
Required labels: job
```

### 3. alert-hysteresis - Alert Hysteresis Analyzer

Analyzes historical alert firing patterns and recommends optimal `for` durations to reduce spurious, unactionable alerts.

**Features:**
- Queries Prometheus for historical alert firing data
- Compares actual firing durations with configured `for` values
- Recommends better hysteresis values based on statistical analysis
- Identifies spurious short-lived alerts
- Suggests optimal values to reduce alert fatigue

**Usage:**

```bash
# Analyze all alerts from last 7 days
alert-hysteresis --prometheus-url=http://localhost:9090

# Analyze specific alert over 24 hours
alert-hysteresis --prometheus-url=http://prometheus:9090 \
  --alert=HighErrorRate \
  --timeframe=24h

# Compare with configured values in rules file
alert-hysteresis --prometheus-url=http://prometheus:9090 \
  --rules=./alerts.yml \
  --timeframe=7d

# Adjust sensitivity threshold (default: 20% mismatch)
alert-hysteresis --prometheus-url=http://prometheus:9090 \
  --threshold=0.3 \
  --rules=./alerts.yml
```

**Example Output:**

```
Fetching alert history from http://prometheus:9090 (timeframe: 168h0m0s)...
Analyzing 156 alert firing events...

Alert: HighErrorRate
  Firing events: 45
  Average duration: 3m24s
  Median duration: 2m15s
  Min/Max duration: 45s / 25m30s
  Configured 'for': 30s
  ⚠ RECOMMENDATION: Change 'for' duration to 2m
     Reason: 33.3% of alerts (15/45) fire for less than 2m, suggesting spurious alerts
  Spurious alerts (< recommended): 15 (33.3%)

Alert: HighMemoryUsage
  Firing events: 12
  Average duration: 45m12s
  Median duration: 42m0s
  Min/Max duration: 15m / 2h15m
  Configured 'for': 30m
  Recommended 'for': 30m
  ✓ Current configuration is acceptable

Found 1 alerts that need hysteresis adjustment
```

## Installation

### Homebrew (macOS/Linux)

```bash
brew install conallob/tap/o11y-analysis-tools
```

### Container Images

Each tool is available as a container image:

```bash
# Pull specific tool
docker pull ghcr.io/conallob/promql-fmt:latest
docker pull ghcr.io/conallob/label-check:latest
docker pull ghcr.io/conallob/alert-hysteresis:latest

# Run in container
docker run -v $(pwd):/data ghcr.io/conallob/promql-fmt:latest --check /data
docker run -v $(pwd):/data ghcr.io/conallob/label-check:latest /data
```

### Package Managers

**Debian/Ubuntu:**
```bash
# Download .deb from releases page
wget https://github.com/conallob/o11y-analysis-tools/releases/download/vX.Y.Z/o11y-analysis-tools_X.Y.Z_linux_amd64.deb
sudo dpkg -i o11y-analysis-tools_X.Y.Z_linux_amd64.deb
```

**RHEL/Fedora/CentOS:**
```bash
# Download .rpm from releases page
wget https://github.com/conallob/o11y-analysis-tools/releases/download/vX.Y.Z/o11y-analysis-tools_X.Y.Z_linux_amd64.rpm
sudo rpm -i o11y-analysis-tools_X.Y.Z_linux_amd64.rpm
```

### Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/conallob/o11y-analysis-tools/releases).

Binaries are available for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

### Build from source

```bash
# Clone the repository
git clone https://github.com/conallob/o11y-analysis-tools.git
cd o11y-analysis-tools

# Build all tools
make build

# Or build individually
go build -o bin/promql-fmt ./cmd/promql-fmt
go build -o bin/label-check ./cmd/label-check
go build -o bin/alert-hysteresis ./cmd/alert-hysteresis

# Install to $GOPATH/bin
make install
```

## CI/CD Integration

All tools are designed to work in CI/CD pipelines with `--check` mode as the default behavior.

### GitHub Actions Example

```yaml
name: PromQL Validation

on: [pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Install tools
        run: |
          go install github.com/conallob/o11y-analysis-tools/cmd/promql-fmt@latest
          go install github.com/conallob/o11y-analysis-tools/cmd/label-check@latest

      - name: Check PromQL formatting
        run: promql-fmt --check ./prometheus/

      - name: Check required labels
        run: label-check --labels=job,namespace ./prometheus/
```

### GitLab CI Example

```yaml
promql-validation:
  image: golang:1.21
  script:
    - go install github.com/conallob/o11y-analysis-tools/cmd/promql-fmt@latest
    - go install github.com/conallob/o11y-analysis-tools/cmd/label-check@latest
    - promql-fmt --check ./alerts/
    - label-check --labels=job ./alerts/
  only:
    - merge_requests
```

### Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit

promql-fmt --check $(git diff --cached --name-only --diff-filter=ACM | grep -E '\.(yml|yaml)$')
if [ $? -ne 0 ]; then
    echo "PromQL formatting issues found. Run 'promql-fmt --fix' to fix."
    exit 1
fi

label-check --check $(git diff --cached --name-only --diff-filter=ACM | grep -E '\.(yml|yaml)$')
if [ $? -ne 0 ]; then
    echo "Missing required labels. Please add 'job' label to all PromQL expressions."
    exit 1
fi
```

## Development

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run specific package tests
go test ./pkg/formatting
go test ./internal/promql
go test ./internal/alertmanager
```

### Project Structure

```
.
├── cmd/
│   ├── promql-fmt/          # PromQL formatter CLI
│   ├── label-check/         # Label standards checker CLI
│   └── alert-hysteresis/    # Alert hysteresis analyzer CLI
├── internal/
│   ├── promql/              # PromQL parsing and analysis
│   └── alertmanager/        # Alertmanager/Prometheus integration
├── pkg/
│   └── formatting/          # PromQL formatting logic
├── examples/                # Example Prometheus rules and alerts
├── Makefile
└── README.md
```

## Configuration

### promql-fmt

No configuration file needed. All options are provided via CLI flags.

### label-check

Create a `.label-check.yml` in your repository root:

```yaml
required_labels:
  - job
  - namespace
  - cluster
```

Then run without flags:
```bash
label-check ./alerts/
```

### alert-hysteresis

Create a `.alert-hysteresis.yml`:

```yaml
prometheus_url: http://prometheus:9090
timeframe: 7d
threshold: 0.2
rules_file: ./prometheus/alerts.yml
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass: `make test`
5. Submit a pull request

## Releasing

Releases are automated using GitHub Actions and GoReleaser.

### Creating a Release

1. Ensure all tests pass: `make test`
2. Create and push a new tag:
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```
3. GitHub Actions will automatically:
   - Build binaries for all platforms
   - Create RPM and Deb packages
   - Build and push container images to ghcr.io
   - Publish Homebrew formula to conallob/homebrew-tap
   - Create a GitHub release with artifacts

### Prerequisites for Releases

Repository secrets required:
- `GITHUB_TOKEN` - Automatically provided by GitHub Actions
- `HOMEBREW_TAP_TOKEN` - Personal access token with write access to homebrew-tap repo
- `GPG_KEY_FILE` - (Optional) GPG key for signing packages

### Release Artifacts

Each release includes:
- Binary archives (tar.gz/zip) for Linux, macOS, Windows (amd64, arm64)
- Debian packages (.deb)
- RPM packages (.rpm)
- Container images (multi-arch) on GitHub Container Registry
- Homebrew formula in conallob/homebrew-tap

## License

BSD 3-Clause - See LICENSE file for details

## Roadmap

- [ ] Add support for Cortex and Thanos
- [ ] Web UI for alert hysteresis analysis
- [ ] Export analysis results to JSON/CSV
- [ ] Integration with Grafana for visualization
- [ ] Support for Mimir-specific PromQL extensions
- [ ] Alert simulation mode to test hysteresis changes
- [ ] Automatic PR creation for recommended changes

## FAQ

**Q: Does promql-fmt support all PromQL syntax?**
A: Currently supports most common PromQL patterns. Complex nested queries may need manual formatting.

**Q: Can alert-hysteresis work with Thanos or Cortex?**
A: Yes, as long as they expose a Prometheus-compatible API endpoint.

**Q: What if my alerts don't have a 'job' label?**
A: Use `--labels` to specify your required labels, or configure via `.label-check.yml`.

**Q: How does alert-hysteresis calculate recommendations?**
A: It uses statistical analysis (median, percentiles) of historical firing durations to recommend values that filter spurious short-lived alerts while preserving actionable ones.
