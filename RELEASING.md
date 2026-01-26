# Releasing o11y-analysis-tools

This document describes the release process for o11y-analysis-tools.

## Overview

Releases are automated using GitHub Actions and GoReleaser. When a new version tag is pushed, the CI/CD pipeline automatically builds binaries, packages, container images, and publishes the Homebrew formula.

## Creating a Release

### Prerequisites

Ensure all tests and checks pass before creating a release:

```bash
# Run full test suite
make test

# Run linter
make lint

# Validate GoReleaser configuration
goreleaser check
```

### Release Process

1. **Update version and changelog** (if applicable)
   - Update version references in documentation
   - Ensure CHANGELOG.md is up to date (or let GoReleaser generate it)

2. **Create and push a version tag:**
   ```bash
   # Create an annotated tag
   git tag -a v1.0.0 -m "Release v1.0.0: Description of changes"

   # Push the tag to trigger release workflow
   git push origin v1.0.0
   ```

3. **Monitor the release workflow:**
   - Go to [GitHub Actions](https://github.com/conallob/o11y-analysis-tools/actions)
   - Watch the "Release" workflow for the new tag
   - The workflow will take several minutes to complete

4. **Verify the release:**
   - Check the [releases page](https://github.com/conallob/o11y-analysis-tools/releases)
   - Verify all artifacts are present
   - Test the Homebrew formula: `brew install conallob/tap/o11y-analysis-tools`
   - Test a container image: `docker pull ghcr.io/conallob/promql-fmt:v1.0.0`

## What Gets Released

The GitHub Actions release workflow automatically:

1. **Builds binaries** for all platforms:
   - Linux (amd64, arm64)
   - macOS/Darwin (amd64, arm64)
   - Windows (amd64, arm64)

2. **Creates packages:**
   - Debian packages (.deb) for Linux
   - RPM packages (.rpm) for RedHat-based systems

3. **Builds and pushes container images:**
   - Multi-architecture images (amd64, arm64) for all 6 tools:
     - `ghcr.io/conallob/promql-fmt`
     - `ghcr.io/conallob/label-check`
     - `ghcr.io/conallob/alert-hysteresis`
     - `ghcr.io/conallob/autogen-promql-tests`
     - `ghcr.io/conallob/e2e-alertmanager-test`
     - `ghcr.io/conallob/stale-alerts-analyzer`
   - Tags: `latest`, `v1.0.0`, `v1.0.0-amd64`, `v1.0.0-arm64`

4. **Publishes Homebrew formula:**
   - Updates formula in `conallob/homebrew-tap`
   - Installs all 6 binaries

5. **Creates GitHub release:**
   - Generates changelog from commits
   - Attaches all binary archives
   - Attaches checksums file
   - Publishes release notes

## Release Artifacts

Each release includes:

- **Binary archives** (`.tar.gz` for Unix, `.zip` for Windows):
  - `o11y-analysis-tools_1.0.0_Darwin_x86_64.tar.gz`
  - `o11y-analysis-tools_1.0.0_Darwin_arm64.tar.gz`
  - `o11y-analysis-tools_1.0.0_Linux_x86_64.tar.gz`
  - `o11y-analysis-tools_1.0.0_Linux_arm64.tar.gz`
  - `o11y-analysis-tools_1.0.0_Windows_x86_64.zip`
  - `o11y-analysis-tools_1.0.0_Windows_arm64.zip`

- **Package files:**
  - `o11y-analysis-tools_1.0.0_linux_amd64.deb`
  - `o11y-analysis-tools_1.0.0_linux_arm64.deb`
  - `o11y-analysis-tools_1.0.0_linux_amd64.rpm`
  - `o11y-analysis-tools_1.0.0_linux_arm64.rpm`

- **Checksums:**
  - `checksums.txt` - SHA256 checksums for all artifacts

- **Container images** - Published to GitHub Container Registry (ghcr.io)

## Repository Secrets Required

The release workflow requires the following GitHub repository secrets:

- **`GITHUB_TOKEN`** - Automatically provided by GitHub Actions
  - Used for creating releases and pushing to ghcr.io
  - No manual configuration needed

- **`HOMEBREW_TAP_GITHUB_TOKEN`** - Personal Access Token with write access
  - Required to push formula updates to `conallob/homebrew-tap`
  - Must have `repo` scope for the homebrew-tap repository
  - Create at: https://github.com/settings/tokens

- **`GPG_KEY_FILE`** - (Optional) GPG key for signing packages
  - Used to sign Debian and RPM packages
  - Not required but recommended for production releases

### Setting up HOMEBREW_TAP_GITHUB_TOKEN

1. Go to GitHub Settings → Developer settings → Personal access tokens
2. Create a new token with `repo` scope
3. Add as repository secret in o11y-analysis-tools repository
4. Name it exactly: `HOMEBREW_TAP_GITHUB_TOKEN`

## Versioning

This project follows [Semantic Versioning](https://semver.org/):

- **MAJOR** version for incompatible API changes
- **MINOR** version for new functionality in a backwards compatible manner
- **PATCH** version for backwards compatible bug fixes

Examples:
- `v1.0.0` - Initial stable release
- `v1.1.0` - New feature added (backwards compatible)
- `v1.1.1` - Bug fix (backwards compatible)
- `v2.0.0` - Breaking change

## Pre-releases

For pre-release versions, use semantic versioning with pre-release identifiers:

```bash
# Alpha release
git tag -a v1.0.0-alpha.1 -m "Release v1.0.0-alpha.1"

# Beta release
git tag -a v1.0.0-beta.1 -m "Release v1.0.0-beta.1"

# Release candidate
git tag -a v1.0.0-rc.1 -m "Release v1.0.0-rc.1"
```

Pre-releases will be marked as "Pre-release" on GitHub and won't update the Homebrew formula's stable version.

## Troubleshooting

### Release workflow fails

1. **Check the workflow logs** in GitHub Actions
2. **Common issues:**
   - GoReleaser configuration error: Run `goreleaser check` locally
   - Missing secret: Verify all required secrets are set
   - Build failure: Ensure `make test` and `make build` pass locally
   - Docker build failure: Check Dockerfiles are present for all binaries

### Homebrew formula not updated

1. Verify `HOMEBREW_TAP_GITHUB_TOKEN` secret is set correctly
2. Check the token has write access to `conallob/homebrew-tap`
3. Look for errors in the GoReleaser logs related to brew publishing

### Container images not published

1. Check Docker login step in release workflow
2. Verify `GITHUB_TOKEN` has packages:write permission
3. Ensure all Dockerfiles exist (one per binary)

## Rolling Back a Release

If a release has critical issues:

1. **Delete the GitHub release** (but keep the tag for history)
2. **Create a new patch release** with the fix:
   ```bash
   git tag -a v1.0.1 -m "Release v1.0.1: Fix critical issue in v1.0.0"
   git push origin v1.0.1
   ```

3. **Update Homebrew formula manually** if needed:
   ```bash
   # Clone homebrew-tap repo
   git clone https://github.com/conallob/homebrew-tap
   cd homebrew-tap

   # Update Formula/o11y-analysis-tools.rb
   # Change version and SHA256

   git commit -am "Rollback to v1.0.1"
   git push
   ```

## Testing Releases Locally

Test the release process locally without publishing:

```bash
# Build a snapshot release (no git tag required)
goreleaser release --snapshot --clean

# Check the dist/ directory for artifacts
ls -la dist/

# Test a specific binary
./dist/promql-fmt_darwin_arm64/promql-fmt --help
```

This helps verify the release configuration before creating an actual release.

## Release Checklist

Before creating a release:

- [ ] All tests pass (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] GoReleaser config valid (`goreleaser check`)
- [ ] Consistency validation passes (`./scripts/validate-goreleaser-consistency.sh`)
- [ ] CHANGELOG updated (or commits follow conventional format)
- [ ] Version number follows semantic versioning
- [ ] All 6 binaries build successfully
- [ ] Documentation updated for new features

After creating a release:

- [ ] GitHub release created successfully
- [ ] All binary artifacts present
- [ ] Container images published to ghcr.io
- [ ] Homebrew formula updated
- [ ] Installation instructions tested (`brew install`, `docker pull`)
- [ ] Release announcement posted (if applicable)
