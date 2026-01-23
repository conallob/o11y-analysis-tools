# Contributing to o11y-analysis-tools

Thank you for your interest in contributing to o11y-analysis-tools! This document provides guidelines and information for contributors.

## Getting Started

### Prerequisites

- Go 1.21 or later
- Make (for build automation)
- golangci-lint v2.8+ (for linting)
- Docker or Podman (optional, for container builds)

### Setting Up Your Development Environment

1. **Fork and clone the repository:**
   ```bash
   git clone https://github.com/YOUR-USERNAME/o11y-analysis-tools.git
   cd o11y-analysis-tools
   ```

2. **Install dependencies:**
   ```bash
   go mod download
   ```

3. **Verify your setup:**
   ```bash
   make test
   make lint
   make build
   ```

## Development Workflow

### Making Changes

1. **Create a feature branch:**
   ```bash
   git checkout -b feat/your-feature-name
   # or
   git checkout -b fix/your-bug-fix
   ```

2. **Make your changes:**
   - Write clean, readable code
   - Follow Go best practices and idioms
   - Add tests for new functionality
   - Update documentation as needed

3. **Test your changes:**
   ```bash
   # Run tests
   make test

   # Run tests with coverage
   make test-coverage

   # Run linter
   make lint

   # Build all binaries
   make build
   ```

4. **Commit your changes:**
   ```bash
   git add .
   git commit -m "feat: add new feature

   Detailed description of changes.

   Fixes #123"
   ```

### Commit Message Format

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>: <short summary>

<optional body>

<optional footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Adding or updating tests
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `chore`: Maintenance tasks
- `ci`: CI/CD changes

**Examples:**
```bash
git commit -m "feat(promql-fmt): add support for nested aggregations"
git commit -m "fix(label-check): handle empty label values correctly"
git commit -m "docs: update installation instructions"
```

### Pull Request Process

1. **Push your branch:**
   ```bash
   git push origin feat/your-feature-name
   ```

2. **Create a Pull Request:**
   - Go to GitHub and create a PR from your fork
   - Provide a clear title and description
   - Reference any related issues
   - Ensure all CI checks pass

3. **Code Review:**
   - Address reviewer feedback
   - Make requested changes
   - Keep the PR updated with main branch

4. **Merge:**
   - Once approved, a maintainer will merge your PR
   - Your contribution will be included in the next release!

## Development Guidelines

### Code Style

- **Follow standard Go formatting:**
  ```bash
  go fmt ./...
  ```

- **Run the linter:**
  ```bash
  golangci-lint run
  ```

- **Write clear, self-documenting code:**
  - Use descriptive variable and function names
  - Add comments for complex logic
  - Keep functions focused and small

### Testing

**Always write tests for:**
- New functions and methods
- Bug fixes (regression tests)
- Edge cases and error conditions

**Test file conventions:**
- Place tests in `*_test.go` files
- Use table-driven tests for multiple test cases
- Test both success and failure scenarios

**Example test structure:**
```go
func TestFormatPromQL(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "simple expression",
            input:    "up",
            expected: "up",
            wantErr:  false,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := FormatPromQL(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("FormatPromQL() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if result != tt.expected {
                t.Errorf("FormatPromQL() = %v, want %v", result, tt.expected)
            }
        })
    }
}
```

### Running Tests

```bash
# Run all tests
go test ./...
make test

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -coverprofile=coverage.txt ./...
go tool cover -html=coverage.txt

# Run tests with race detection
go test -race ./...

# Run specific package tests
go test ./pkg/formatting
go test ./internal/promql
```

### Project Structure

```
.
├── cmd/                         # CLI entry points (main packages)
│   ├── promql-fmt/             # PromQL formatter tool
│   ├── label-check/            # Label validation tool
│   ├── alert-hysteresis/       # Alert analysis tool
│   ├── autogen-promql-tests/   # PromQL test generator
│   ├── e2e-alertmanager-test/  # Alertmanager E2E testing
│   └── stale-alerts-analyzer/  # Alert staleness analyzer
├── internal/                    # Private packages (not importable externally)
│   ├── promql/                 # PromQL parsing utilities
│   └── alertmanager/           # Prometheus/Alertmanager integration
├── pkg/                         # Public packages (importable by external projects)
│   └── formatting/             # PromQL formatting logic
├── examples/                    # Example configurations and rules
├── scripts/                     # Development and CI scripts
├── .github/workflows/          # GitHub Actions workflows
├── Makefile                    # Build automation
├── go.mod                      # Go module definition
└── README.md                   # Project documentation
```

**Package organization guidelines:**
- `cmd/`: Only main packages, minimal logic
- `pkg/`: Reusable libraries, well-documented public APIs
- `internal/`: Implementation details, not exposed to external users

### Adding a New Tool

To add a new CLI tool to the repository:

1. **Create the command package:**
   ```bash
   mkdir -p cmd/your-tool
   ```

2. **Create main.go:**
   ```go
   package main

   import (
       "fmt"
       "os"
   )

   func main() {
       // Your tool implementation
   }
   ```

3. **Create a Dockerfile:**
   ```bash
   # Create Dockerfile.your-tool
   FROM alpine:latest
   COPY your-tool /usr/local/bin/your-tool
   ENTRYPOINT ["/usr/local/bin/your-tool"]
   ```

4. **Update .goreleaser.yml:**
   - Add build configuration for your tool
   - Add Docker configuration
   - Add to Homebrew formula install section
   - Run `./scripts/validate-goreleaser-consistency.sh` to verify

5. **Update documentation:**
   - Add tool description to README.md
   - Update CLAUDE.md with the new tool
   - Add examples and usage instructions

6. **Test the integration:**
   ```bash
   # Build your tool
   go build -o bin/your-tool ./cmd/your-tool

   # Test it
   ./bin/your-tool --help

   # Verify GoReleaser consistency
   ./scripts/validate-goreleaser-consistency.sh
   ```

### Building and Testing

```bash
# Build all binaries
make build

# Build specific tool
go build -o bin/promql-fmt ./cmd/promql-fmt

# Clean build artifacts
make clean

# Run full CI checks locally (before committing)
go mod download
go test -v -race -coverprofile=./coverage.txt ./...
go build -o bin/ ./cmd/promql-fmt
go build -o bin/ ./cmd/label-check
go build -o bin/ ./cmd/alert-hysteresis
go build -o bin/ ./cmd/autogen-promql-tests
go build -o bin/ ./cmd/e2e-alertmanager-test
go build -o bin/ ./cmd/stale-alerts-analyzer
golangci-lint run
```

## Documentation

### Updating Documentation

When making changes, update the relevant documentation:

- **README.md**: User-facing documentation, installation, usage examples
- **CLAUDE.md**: AI assistant guidance for working with the codebase
- **CONTRIBUTING.md**: This file, contribution guidelines
- **RELEASING.md**: Release process and versioning
- **Code comments**: GoDoc comments for public APIs

### Documentation Standards

- Use clear, concise language
- Include code examples for new features
- Update examples when APIs change
- Keep installation instructions current

## CI/CD Integration

### GitHub Actions Workflows

The project uses several GitHub Actions workflows:

1. **test.yml**: Runs on all pushes and PRs
   - Runs tests on multiple platforms (Ubuntu, macOS, Windows)
   - Tests with multiple Go versions (1.21, 1.22)
   - Runs linter
   - Validates GoReleaser configuration
   - Checks GoReleaser consistency

2. **release.yml**: Runs on version tags (v*)
   - Builds binaries for all platforms
   - Creates packages (deb, rpm)
   - Publishes container images
   - Updates Homebrew formula
   - Creates GitHub release

### Running CI Checks Locally

Before pushing, run the same checks that CI will run:

```bash
# Quick check
go test ./... && golangci-lint run

# Full CI simulation (recommended before PR)
./scripts/ci-check.sh  # if the script exists

# Or manually:
go mod download
go test -v -race -coverprofile=./coverage.txt ./...
golangci-lint run
./scripts/validate-goreleaser-consistency.sh
```

## Getting Help

- **Questions?** Open a [GitHub Discussion](https://github.com/conallob/o11y-analysis-tools/discussions)
- **Bug reports?** Open a [GitHub Issue](https://github.com/conallob/o11y-analysis-tools/issues)
- **Security issues?** See SECURITY.md (if exists) or contact maintainers privately

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). By participating, you are expected to uphold this code.

## License

By contributing to o11y-analysis-tools, you agree that your contributions will be licensed under the BSD 3-Clause License.

## Recognition

Contributors are recognized in:
- Git commit history
- GitHub contributors page
- Release notes (for significant contributions)

Thank you for contributing to o11y-analysis-tools! 🎉
