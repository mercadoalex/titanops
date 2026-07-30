# TitanOps Makefile
# Run `make help` to see available targets.

.PHONY: all build test lint vet fmt clean dashboard dashboard-build dashboard-lint help

# Default target
all: lint test build

# --- Go Targets ---

## Build all Go modules in the workspace
build:
	@echo "==> Building all modules..."
	@for mod in $$(grep '\./' go.work | tr -d '\t'); do \
		echo "  $$mod"; \
		(cd "$$mod" && go build ./...) || exit 1; \
	done
	@echo "==> Build OK"

## Run tests with race detector across all modules
test:
	@echo "==> Testing all modules (race detector enabled)..."
	@for mod in $$(grep '\./' go.work | tr -d '\t'); do \
		echo "  $$mod"; \
		(cd "$$mod" && go test -race -timeout 120s ./...) || exit 1; \
	done
	@echo "==> Tests OK"

## Run tests with coverage report
test-cover:
	@echo "==> Testing with coverage..."
	@for mod in $$(grep '\./' go.work | tr -d '\t'); do \
		echo "  $$mod"; \
		(cd "$$mod" && go test -race -timeout 120s -coverprofile=coverage.out ./...) || exit 1; \
	done
	@echo "==> Coverage files written (coverage.out per module)"

## Run golangci-lint
lint:
	@echo "==> Linting Go code..."
	@golangci-lint run --timeout=5m ./...
	@echo "==> Lint OK"

## Run go vet across all modules
vet:
	@echo "==> Vetting all modules..."
	@for mod in $$(grep '\./' go.work | tr -d '\t'); do \
		(cd "$$mod" && go vet ./...) || exit 1; \
	done
	@echo "==> Vet OK"

## Format all Go code
fmt:
	@echo "==> Formatting Go code..."
	@goimports -w -local github.com/mercadoalex/titanops .
	@echo "==> Format OK"

## Tidy all module dependencies
tidy:
	@echo "==> Tidying modules..."
	@for mod in $$(grep '\./' go.work | tr -d '\t'); do \
		(cd "$$mod" && go mod tidy) || exit 1; \
	done
	@echo "==> Tidy OK"

# --- Dashboard Targets ---

## Install dashboard dependencies
dashboard:
	@echo "==> Installing dashboard dependencies..."
	@cd dashboard && npm ci
	@echo "==> Dashboard ready"

## Build the dashboard
dashboard-build: dashboard
	@echo "==> Building dashboard..."
	@cd dashboard && npm run build
	@echo "==> Dashboard build OK"

## Lint the dashboard
dashboard-lint: dashboard
	@echo "==> Linting dashboard..."
	@cd dashboard && npm run lint
	@echo "==> Dashboard lint OK"

# --- Utility Targets ---

## Remove build artifacts
clean:
	@echo "==> Cleaning..."
	@rm -f cmd/titanops/titanops
	@rm -f eval/runner/runner
	@for mod in $$(grep '\./' go.work | tr -d '\t'); do \
		rm -f "$$mod/coverage.out"; \
	done
	@rm -rf dashboard/dist
	@echo "==> Clean"

## Run the platform locally
run:
	@go run ./cmd/titanops

## Run the MCP server locally
run-mcp:
	@go run ./cmd/titanops-mcp

## Show this help
help:
	@echo "TitanOps Development Targets"
	@echo "============================"
	@echo ""
	@echo "  make            Run lint + test + build (default)"
	@echo "  make build      Build all Go modules"
	@echo "  make test       Run tests with race detector"
	@echo "  make test-cover Run tests with coverage output"
	@echo "  make lint       Run golangci-lint"
	@echo "  make vet        Run go vet"
	@echo "  make fmt        Format code with goimports"
	@echo "  make tidy       Tidy all module dependencies"
	@echo ""
	@echo "  make dashboard       Install dashboard deps"
	@echo "  make dashboard-build Build the React dashboard"
	@echo "  make dashboard-lint  Lint the dashboard"
	@echo ""
	@echo "  make run         Run the platform locally"
	@echo "  make run-mcp     Run the MCP server locally"
	@echo "  make clean       Remove build artifacts"
	@echo "  make help        Show this help"
