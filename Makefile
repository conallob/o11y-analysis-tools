.PHONY: build test clean install help

# Build variables
BINARY_DIR := bin
TOOLS := promql-fmt label-check alert-hysteresis

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOINSTALL := $(GOCMD) install

help:
	@echo "Available targets:"
	@echo "  build         - Build all tools"
	@echo "  test          - Run all tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  install       - Install tools to GOPATH/bin"
	@echo "  clean         - Remove built binaries"
	@echo "  deps          - Download dependencies"
	@echo "  fmt           - Format code"
	@echo "  lint          - Run linters"

build: deps
	@echo "Building tools..."
	@mkdir -p $(BINARY_DIR)
	@for tool in $(TOOLS); do \
		echo "Building $$tool..."; \
		$(GOBUILD) -o $(BINARY_DIR)/$$tool ./cmd/$$tool || exit 1; \
	done
	@echo "Build complete! Binaries in $(BINARY_DIR)/"

deps:
	@echo "Downloading dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy

install: deps
	@echo "Installing tools to GOPATH/bin..."
	@for tool in $(TOOLS); do \
		echo "Installing $$tool..."; \
		$(GOINSTALL) ./cmd/$$tool || exit 1; \
	done
	@echo "Install complete!"

test:
	@echo "Running tests..."
	@$(GOTEST) -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	@$(GOTEST) -v -coverprofile=coverage.out ./...
	@$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

fmt:
	@echo "Formatting code..."
	@$(GOCMD) fmt ./...

lint:
	@echo "Running linters..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install from https://golangci-lint.run/"; \
	fi

clean:
	@echo "Cleaning..."
	@rm -rf $(BINARY_DIR)
	@rm -f coverage.out coverage.html
	@echo "Clean complete!"

# Individual tool builds
promql-fmt: deps
	@echo "Building promql-fmt..."
	@mkdir -p $(BINARY_DIR)
	@$(GOBUILD) -o $(BINARY_DIR)/promql-fmt ./cmd/promql-fmt

label-check: deps
	@echo "Building label-check..."
	@mkdir -p $(BINARY_DIR)
	@$(GOBUILD) -o $(BINARY_DIR)/label-check ./cmd/label-check

alert-hysteresis: deps
	@echo "Building alert-hysteresis..."
	@mkdir -p $(BINARY_DIR)
	@$(GOBUILD) -o $(BINARY_DIR)/alert-hysteresis ./cmd/alert-hysteresis
