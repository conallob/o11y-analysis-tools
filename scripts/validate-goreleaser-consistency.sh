#!/bin/bash
# Validate that .goreleaser.yml, cmd/ binaries, and Dockerfiles are all in sync
# This script ensures that every binary has a build config, Docker config, and Dockerfile

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "==> Validating GoReleaser consistency..."
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track if we found any errors
ERRORS=0

# Step 1: Find all binaries in cmd/
echo "Step 1: Finding all binaries in cmd/..."
CMD_BINARIES=()
for dir in "${PROJECT_ROOT}/cmd"/*/ ; do
    if [ -d "$dir" ]; then
        binary=$(basename "$dir")
        CMD_BINARIES+=("$binary")
        echo "  Found cmd binary: $binary"
    fi
done
echo ""

# Step 2: Find all Dockerfiles
echo "Step 2: Finding all Dockerfiles..."
DOCKERFILES=()
for dockerfile in "${PROJECT_ROOT}"/Dockerfile.* ; do
    if [ -f "$dockerfile" ]; then
        # Extract binary name from Dockerfile.{binary}
        binary=$(basename "$dockerfile" | sed 's/^Dockerfile\.//')
        DOCKERFILES+=("$binary")
        echo "  Found Dockerfile: Dockerfile.$binary"
    fi
done
echo ""

# Step 3: Parse .goreleaser.yml for build IDs
echo "Step 3: Parsing .goreleaser.yml for build configurations..."
GORELEASER_BUILDS=()
if [ -f "${PROJECT_ROOT}/.goreleaser.yml" ]; then
    # Extract build IDs (lines with "- id: <name>" under builds section)
    # We need to be careful to only get build IDs, not docker IDs
    while IFS= read -r line; do
        if [[ "$line" =~ ^[[:space:]]*-[[:space:]]+id:[[:space:]]+([a-zA-Z0-9_-]+)$ ]]; then
            build_id="${BASH_REMATCH[1]}"
            # Skip docker-specific IDs (they end with -amd64 or -arm64)
            if [[ ! "$build_id" =~ -amd64$ ]] && [[ ! "$build_id" =~ -arm64$ ]]; then
                GORELEASER_BUILDS+=("$build_id")
                echo "  Found build config: $build_id"
            fi
        fi
    done < <(sed -n '/^builds:/,/^[a-z]/p' "${PROJECT_ROOT}/.goreleaser.yml" | grep -E '^\s*-\s+id:')
else
    echo -e "${RED}ERROR: .goreleaser.yml not found${NC}"
    exit 1
fi
echo ""

# Step 4: Parse .goreleaser.yml for docker configurations
echo "Step 4: Parsing .goreleaser.yml for docker configurations..."
GORELEASER_DOCKERS=()
if [ -f "${PROJECT_ROOT}/.goreleaser.yml" ]; then
    # Extract unique docker binary names from dockerfile field
    while IFS= read -r line; do
        if [[ "$line" =~ dockerfile:[[:space:]]+Dockerfile\.([a-zA-Z0-9_-]+) ]]; then
            docker_binary="${BASH_REMATCH[1]}"
            # Add to array if not already present
            if [[ ! " ${GORELEASER_DOCKERS[@]} " =~ " ${docker_binary} " ]]; then
                GORELEASER_DOCKERS+=("$docker_binary")
                echo "  Found docker config for: $docker_binary"
            fi
        fi
    done < <(sed -n '/^dockers:/,/^[a-z]/p' "${PROJECT_ROOT}/.goreleaser.yml")
fi
echo ""

# Step 5: Parse .goreleaser.yml for Homebrew formula installs
echo "Step 5: Parsing .goreleaser.yml for Homebrew formula..."
HOMEBREW_BINARIES=()
if [ -f "${PROJECT_ROOT}/.goreleaser.yml" ]; then
    # Extract binaries from install section
    while IFS= read -r line; do
        if [[ "$line" =~ bin\.install[[:space:]]+\"([a-zA-Z0-9_-]+)\" ]]; then
            brew_binary="${BASH_REMATCH[1]}"
            HOMEBREW_BINARIES+=("$brew_binary")
            echo "  Found Homebrew install: $brew_binary"
        fi
    done < <(sed -n '/install: |/,/test: |/p' "${PROJECT_ROOT}/.goreleaser.yml")
fi
echo ""

# Step 6: Validate consistency
echo "==> Validating consistency..."
echo ""

# Check 1: Every cmd/ binary must have a build config in .goreleaser.yml
echo "Check 1: Every cmd/ binary has a build config..."
for binary in "${CMD_BINARIES[@]}"; do
    if [[ ! " ${GORELEASER_BUILDS[@]} " =~ " ${binary} " ]]; then
        echo -e "${RED}❌ MISSING: Build config for '${binary}' in .goreleaser.yml${NC}"
        echo "   Add this to the 'builds:' section:"
        echo ""
        echo "  - id: ${binary}"
        echo "    main: ./cmd/${binary}"
        echo "    binary: ${binary}"
        echo "    env:"
        echo "      - CGO_ENABLED=0"
        echo "    goos:"
        echo "      - linux"
        echo "      - darwin"
        echo "      - windows"
        echo "    goarch:"
        echo "      - amd64"
        echo "      - arm64"
        echo "    ldflags:"
        echo "      - -s -w -X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}"
        echo ""
        ERRORS=$((ERRORS + 1))
    else
        echo -e "${GREEN}✓${NC} Build config exists for: $binary"
    fi
done
echo ""

# Check 2: Every cmd/ binary must have a Dockerfile
echo "Check 2: Every cmd/ binary has a Dockerfile..."
for binary in "${CMD_BINARIES[@]}"; do
    if [[ ! " ${DOCKERFILES[@]} " =~ " ${binary} " ]]; then
        echo -e "${RED}❌ MISSING: Dockerfile.${binary}${NC}"
        ERRORS=$((ERRORS + 1))
    else
        echo -e "${GREEN}✓${NC} Dockerfile exists for: $binary"
    fi
done
echo ""

# Check 3: Every cmd/ binary must have Docker configs in .goreleaser.yml
echo "Check 3: Every cmd/ binary has Docker configs in .goreleaser.yml..."
for binary in "${CMD_BINARIES[@]}"; do
    if [[ ! " ${GORELEASER_DOCKERS[@]} " =~ " ${binary} " ]]; then
        echo -e "${RED}❌ MISSING: Docker config for '${binary}' in .goreleaser.yml${NC}"
        echo "   Add these to the 'dockers:' section:"
        echo ""
        echo "  - id: ${binary}-amd64"
        echo "    ids:"
        echo "      - ${binary}"
        echo "    image_templates:"
        echo "      - \"ghcr.io/conallob/${binary}:{{ .Version }}-amd64\""
        echo "      - \"ghcr.io/conallob/${binary}:latest-amd64\""
        echo "    dockerfile: Dockerfile.${binary}"
        echo "    use: buildx"
        echo "    build_flag_templates:"
        echo "      - \"--platform=linux/amd64\""
        echo "      - \"--label=org.opencontainers.image.created={{.Date}}\""
        echo "      - \"--label=org.opencontainers.image.title=${binary}\""
        echo "      - \"--label=org.opencontainers.image.revision={{.FullCommit}}\""
        echo "      - \"--label=org.opencontainers.image.version={{.Version}}\""
        echo ""
        echo "  - id: ${binary}-arm64"
        echo "    ids:"
        echo "      - ${binary}"
        echo "    image_templates:"
        echo "      - \"ghcr.io/conallob/${binary}:{{ .Version }}-arm64\""
        echo "      - \"ghcr.io/conallob/${binary}:latest-arm64\""
        echo "    dockerfile: Dockerfile.${binary}"
        echo "    use: buildx"
        echo "    goarch: arm64"
        echo "    build_flag_templates:"
        echo "      - \"--platform=linux/arm64\""
        echo "      - \"--label=org.opencontainers.image.created={{.Date}}\""
        echo "      - \"--label=org.opencontainers.image.title=${binary}\""
        echo "      - \"--label=org.opencontainers.image.revision={{.FullCommit}}\""
        echo "      - \"--label=org.opencontainers.image.version={{.Version}}\""
        echo ""
        echo "   And add this to the 'docker_manifests:' section:"
        echo ""
        echo "  - name_template: \"ghcr.io/conallob/${binary}:{{ .Version }}\""
        echo "    image_templates:"
        echo "      - \"ghcr.io/conallob/${binary}:{{ .Version }}-amd64\""
        echo "      - \"ghcr.io/conallob/${binary}:{{ .Version }}-arm64\""
        echo ""
        echo "  - name_template: \"ghcr.io/conallob/${binary}:latest\""
        echo "    image_templates:"
        echo "      - \"ghcr.io/conallob/${binary}:latest-amd64\""
        echo "      - \"ghcr.io/conallob/${binary}:latest-arm64\""
        echo ""
        ERRORS=$((ERRORS + 1))
    else
        echo -e "${GREEN}✓${NC} Docker config exists for: $binary"
    fi
done
echo ""

# Check 4: Every cmd/ binary must be in Homebrew formula
echo "Check 4: Every cmd/ binary is in Homebrew formula..."
for binary in "${CMD_BINARIES[@]}"; do
    if [[ ! " ${HOMEBREW_BINARIES[@]} " =~ " ${binary} " ]]; then
        echo -e "${RED}❌ MISSING: Homebrew install for '${binary}' in .goreleaser.yml${NC}"
        echo "   Add this to the 'brews:' > 'install:' section:"
        echo "      bin.install \"${binary}\""
        echo ""
        echo "   And add this to the 'brews:' > 'test:' section:"
        echo "      system \"#{bin}/${binary}\", \"--help\""
        echo ""
        ERRORS=$((ERRORS + 1))
    else
        echo -e "${GREEN}✓${NC} Homebrew install exists for: $binary"
    fi
done
echo ""

# Check 5: Every Dockerfile must have a corresponding cmd/ binary
echo "Check 5: Every Dockerfile has a corresponding cmd/ binary..."
for binary in "${DOCKERFILES[@]}"; do
    if [[ ! " ${CMD_BINARIES[@]} " =~ " ${binary} " ]]; then
        echo -e "${YELLOW}⚠ WARNING: Dockerfile.${binary} exists but no cmd/${binary} found${NC}"
        echo "   Consider removing Dockerfile.${binary} or creating cmd/${binary}"
        echo ""
        # This is a warning, not an error
    else
        echo -e "${GREEN}✓${NC} cmd/${binary} exists for Dockerfile.${binary}"
    fi
done
echo ""

# Check 6: Every build config must have a corresponding cmd/ binary
echo "Check 6: Every build config has a corresponding cmd/ binary..."
for binary in "${GORELEASER_BUILDS[@]}"; do
    if [[ ! " ${CMD_BINARIES[@]} " =~ " ${binary} " ]]; then
        echo -e "${YELLOW}⚠ WARNING: Build config for '${binary}' exists but no cmd/${binary} found${NC}"
        echo "   Consider removing the build config or creating cmd/${binary}"
        echo ""
        # This is a warning, not an error
    else
        echo -e "${GREEN}✓${NC} cmd/${binary} exists for build config '${binary}'"
    fi
done
echo ""

# Summary
echo "==> Summary"
echo ""
echo "  cmd/ binaries:        ${#CMD_BINARIES[@]}"
echo "  Dockerfiles:          ${#DOCKERFILES[@]}"
echo "  Build configs:        ${#GORELEASER_BUILDS[@]}"
echo "  Docker configs:       ${#GORELEASER_DOCKERS[@]}"
echo "  Homebrew installs:    ${#HOMEBREW_BINARIES[@]}"
echo ""

if [ $ERRORS -eq 0 ]; then
    echo -e "${GREEN}✓ All consistency checks passed!${NC}"
    exit 0
else
    echo -e "${RED}❌ Found ${ERRORS} error(s)${NC}"
    echo ""
    echo "Please update .goreleaser.yml to include all binaries from cmd/"
    exit 1
fi
