# Contributing to TitanOps

Thanks for your interest in TitanOps. This document covers everything you need to get started contributing.

---

## Prerequisites

- **Go 1.22+** — all backend modules
- **Node.js 20+** — dashboard
- **golangci-lint** — install via `brew install golangci-lint` or [see docs](https://golangci-lint.run/welcome/install/)
- **goimports** — install via `go install golang.org/x/tools/cmd/goimports@latest`

---

## Quick Start

```bash
git clone https://github.com/mercadoalex/titanops.git
cd titanops

# Build everything
make build

# Run tests
make test

# Run linter
make lint

# Format code
make fmt
```

Run `make help` to see all available targets.

---

## Project Structure

```
titanops/
├── cmd/titanops/              # Platform entry point
├── cmd/titanops-mcp/          # MCP server for AI agents
├── correlation/               # Cross-module correlation engine
├── gateway/                   # REST API gateway
├── dashboard/                 # React UI (TypeScript)
├── integrations/              # External system receivers (AlertManager, etc.)
├── modules/
│   ├── earthworm/             # Health monitoring module
│   ├── ebeecontrol/           # Threat detection module
│   └── ollinai/               # Deployment intelligence module
├── shared/
│   ├── titanops-platform/     # Module/Kernel contract
│   ├── titanops-ai/           # ONNX + cloud AI interface
│   ├── titanops-k8s/          # K8s client helpers
│   ├── titanops-export/       # Multi-backend telemetry export
│   └── titanops-config/       # Config loading & validation
├── eval/                      # Evaluation framework & scenarios
├── docs/                      # Architecture & design docs
├── go.work                    # Go workspace (multi-module)
├── Makefile                   # Build automation
└── .golangci.yml              # Linter configuration
```

---

## Development Workflow

### 1. Pick an issue or area

- Check open issues for `good first issue` labels
- If working on something new, open an issue first to discuss the approach

### 2. Create a branch

```bash
git checkout -b feat/your-feature-name
# or
git checkout -b fix/what-you-are-fixing
```

### 3. Write code

Follow the standards below. Run `make fmt` before committing.

### 4. Verify locally

```bash
make          # runs lint + test + build
```

All three must pass before submitting a PR.

### 5. Submit a PR

- Keep PRs focused — one logical change per PR
- Write a clear title (under 70 characters)
- Describe what changed and why in the PR body
- Reference any related issues

---

## Code Standards

### Go Code

- **Format**: `goimports` with local prefix `github.com/mercadoalex/titanops`
- **Linter**: `golangci-lint` (config in `.golangci.yml`)
- **Imports order**: stdlib → external → internal (goimports handles this)
- **Error handling**: Always check errors. Wrap with context using `fmt.Errorf("operation: %w", err)`
- **Naming**: Follow standard Go conventions. No stuttering (`package.PackageThing`)
- **Comments**: Exported functions/types must have doc comments
- **Tests**: Required for all new functionality. Use table-driven tests where appropriate

### Testing

- **Unit tests**: Required for all logic. Run with `make test`
- **Race detector**: Always enabled in CI (`-race` flag)
- **Property-based tests**: Required for correctness-critical code (use `pgregory.net/rapid`)
- **Table-driven tests**: Preferred for functions with multiple input/output cases
- **Test file location**: Same package, `_test.go` suffix

```go
func TestFunctionName_Scenario(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"descriptive name", "input", "expected"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := FunctionName(tt.input)
            if got != tt.expected {
                t.Errorf("FunctionName(%q) = %q, want %q", tt.input, got, tt.expected)
            }
        })
    }
}
```

### Dashboard (TypeScript/React)

- **Format**: Prettier
- **Lint**: ESLint
- **Types**: Strict TypeScript — no `any` without justification
- **Components**: Functional components with hooks
- **Run**: `make dashboard-lint` to check

---

## Adding a New Module

If you're adding a new TitanOps module:

1. Create the directory under `modules/your-module/`
2. Initialize with `go mod init github.com/mercadoalex/titanops/modules/your-module`
3. Implement the `platform.Module` interface:
   ```go
   type Module interface {
       ID() string
       Version() string
       Start(ctx context.Context, kernel Kernel) error
       Stop(ctx context.Context) error
       HealthCheck(ctx context.Context) HealthStatus
   }
   ```
4. Add to `go.work`
5. Add tests (unit + property-based for decision logic)
6. Emit events via `titanops-export` for correlation

See `modules/earthworm/` as a reference implementation.

---

## Adding a New Integration

If you're adding a receiver for an external system (like AlertManager):

1. Create the directory under `integrations/your-system/`
2. Follow the pattern in `integrations/alertmanager/`:
   - `mapping.go` — pure function converting external format → `export.Event`
   - `receiver.go` — HTTP handler with lifecycle management
   - `receiver_test.go` — comprehensive tests
3. Add to `go.work`
4. The integration must be optional (enabled via environment variable)
5. Must degrade gracefully if disabled or misconfigured

---

## Commit Messages

Use conventional-style messages:

```
feat(correlation): add proximity bonus for events within 10s
fix(gateway): handle nil pointer in audit query
docs: update architecture diagram
test(earthworm): add property tests for scoring logic
ci: add golangci-lint to pipeline
```

Format: `type(scope): description`

Types: `feat`, `fix`, `docs`, `test`, `ci`, `refactor`, `chore`

---

## What We Value

- **Correctness over cleverness** — clear code that does the right thing
- **Tests prove behavior** — if it's not tested, it doesn't work
- **Small PRs** — easier to review, faster to merge
- **Documentation** — exported APIs need doc comments; complex logic needs inline explanation
- **Graceful degradation** — things will fail; handle it cleanly

---

## Getting Help

- Open an issue with your question
- Tag it with `question` label
- Be specific about what you're trying to do and what you've tried

---

## License

By contributing, you agree that your contributions will be licensed under the same license as the project.
