.PHONY: help lint test build build-all clean install deps coverage integration

# Default target
.DEFAULT_GOAL := help

# Version information
VERSION_FILE := VERSION
VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "dev")
LDFLAGS := -X 'github.com/kris-hansen/comanda/cmd.version=v$(VERSION)'

# Build configuration
BINARY_NAME := comanda
BUILD_DIR := dist
MAIN_FILE := main.go

# Colors for output
COLOR_RESET := \033[0m
COLOR_BOLD := \033[1m
COLOR_GREEN := \033[32m
COLOR_YELLOW := \033[33m
COLOR_BLUE := \033[34m

help: ## Display this help message
	@echo "$(COLOR_BOLD)COMandA - Makefile Commands$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Available commands:$(COLOR_RESET)"
	@awk 'BEGIN {FS = ":.*##"; printf "\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  $(COLOR_GREEN)%-15s$(COLOR_RESET) %s\n", $$1, $$2 } /^##@/ { printf "\n$(COLOR_BOLD)%s$(COLOR_RESET)\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
	@echo ""

deps: ## Install/update dependencies
	@echo "$(COLOR_BLUE)Installing dependencies...$(COLOR_RESET)"
	@go mod download
	@go mod tidy
	@echo "$(COLOR_GREEN)✓ Dependencies installed$(COLOR_RESET)"

lint: ## Run golangci-lint with auto-fix
	@echo "$(COLOR_BLUE)Running linters...$(COLOR_RESET)"
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "$(COLOR_YELLOW)golangci-lint not found. Installing...$(COLOR_RESET)"; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	@golangci-lint run --fix
	@echo "$(COLOR_GREEN)✓ Linting complete$(COLOR_RESET)"

lint-check: ## Run golangci-lint without auto-fix (for CI)
	@echo "$(COLOR_BLUE)Running linters (check mode)...$(COLOR_RESET)"
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "$(COLOR_YELLOW)golangci-lint not found. Installing...$(COLOR_RESET)"; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	@golangci-lint run
	@echo "$(COLOR_GREEN)✓ Linting complete$(COLOR_RESET)"

test: ## Run all unit tests with coverage
	@echo "$(COLOR_BLUE)Running tests...$(COLOR_RESET)"
	@go test -v -coverprofile=coverage.out -covermode=atomic ./...
	@echo "$(COLOR_GREEN)✓ Tests complete$(COLOR_RESET)"

test-race: ## Run tests with race detector
	@echo "$(COLOR_BLUE)Running tests with race detector...$(COLOR_RESET)"
	@go test -v -race ./...
	@echo "$(COLOR_GREEN)✓ Race detection complete$(COLOR_RESET)"

coverage: test ## Run tests and show coverage report
	@echo "$(COLOR_BLUE)Generating coverage report...$(COLOR_RESET)"
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(COLOR_GREEN)✓ Coverage report generated: coverage.html$(COLOR_RESET)"

integration: build ## Run integration tests
	@echo "$(COLOR_BLUE)Running integration tests...$(COLOR_RESET)"
	@if [ ! -f .env ]; then \
		echo "$(COLOR_YELLOW)Warning: .env file not found. Integration tests may fail.$(COLOR_RESET)"; \
	fi
	@cd tests/integration && go test -v -tags=integration ./...
	@echo "$(COLOR_GREEN)✓ Integration tests complete$(COLOR_RESET)"

build: ## Build binary for current platform
	@echo "$(COLOR_BLUE)Building $(BINARY_NAME) v$(VERSION)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "$(COLOR_GREEN)✓ Build complete: $(BUILD_DIR)/$(BINARY_NAME)$(COLOR_RESET)"

build-all: ## Build binaries for all platforms
	@echo "$(COLOR_BLUE)Building for all platforms...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	@echo "Building for windows/amd64..."
	@GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_FILE)
	@echo "Building for windows/386..."
	@GOOS=windows GOARCH=386 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-386.exe $(MAIN_FILE)
	@echo "Building for darwin/amd64..."
	@GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_FILE)
	@echo "Building for darwin/arm64..."
	@GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_FILE)
	@echo "Building for linux/amd64..."
	@GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_FILE)
	@echo "Building for linux/386..."
	@GOOS=linux GOARCH=386 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-386 $(MAIN_FILE)
	@echo "Building for linux/arm64..."
	@GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_FILE)
	@echo "$(COLOR_GREEN)✓ All builds complete$(COLOR_RESET)"

install: build ## Install binary to $GOPATH/bin
	@echo "$(COLOR_BLUE)Installing $(BINARY_NAME)...$(COLOR_RESET)"
	@go install -ldflags="$(LDFLAGS)" .
	@echo "$(COLOR_GREEN)✓ Installed to $$(go env GOPATH)/bin/$(BINARY_NAME)$(COLOR_RESET)"

clean: ## Remove build artifacts
	@echo "$(COLOR_BLUE)Cleaning build artifacts...$(COLOR_RESET)"
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "$(COLOR_GREEN)✓ Clean complete$(COLOR_RESET)"

fmt: ## Format code with gofmt
	@echo "$(COLOR_BLUE)Formatting code...$(COLOR_RESET)"
	@gofmt -s -w .
	@echo "$(COLOR_GREEN)✓ Formatting complete$(COLOR_RESET)"

vet: ## Run go vet
	@echo "$(COLOR_BLUE)Running go vet...$(COLOR_RESET)"
	@go vet ./...
	@echo "$(COLOR_GREEN)✓ Vet complete$(COLOR_RESET)"

check: lint-check vet test ## Run all checks (lint, vet, test) - suitable for CI

ci: check ## Alias for 'check' - run all CI checks

dev: deps lint test build ## Full development cycle: deps, lint, test, build
	@echo "$(COLOR_GREEN)✓ Development build complete$(COLOR_RESET)"

release: check build-all ## Prepare release: run checks and build all platforms
	@echo "$(COLOR_GREEN)✓ Release build complete for version v$(VERSION)$(COLOR_RESET)"
	@echo "Binaries available in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/
