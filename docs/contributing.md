# Contributing

## Development Setup

```bash
git clone https://github.com/Khanh-21522203/GoIaC.git
cd GoIaC
go mod download
go build -o goiac ./cmd
```

Requirements:
- Go 1.21+
- Docker Engine (for `docker_container` and `docker_network` tests)

---

## Running Tests

```bash
# All packages
go test ./...

# Single package
go test ./pkg/reconciler/...

# With verbose output
go test -v ./pkg/graph/...

# With race detector
go test -race ./...
```

The Docker provider tests require a running Docker daemon. All other tests run without external dependencies.

---

## Project Layout

Each package has a single well-defined responsibility. See [architecture.md](architecture.md) for the full picture. The key rule: packages only depend inward — `cli` → `reconciler` → `provider`/`state`, never the reverse.

---

## Adding a New Command

1. Create `pkg/cli/<command>.go`.
2. Add a `func (c *CLI) Run<Command>(...)` method.
3. Register it in `cmd/main.go` (the argument dispatch switch).
4. If it modifies state, acquire the lock via `c.stateManager.WithLock(...)` before calling the reconciler.

---

## Adding a New Provider

See [providers.md](providers.md) for a complete walkthrough.

Short version:
1. Implement `provider.Provider` in `pkg/provider/<name>/`.
2. Register in `pkg/cli/cli.go` → `registerProviders`.
3. Add a schema entry in `pkg/provider/validation.go`.
4. Write an integration test following the pattern in `pkg/provider/local/file_test.go`.

---

## Code Style

- Standard `gofmt` formatting. No custom linter config.
- Error strings are lowercase and do not end with punctuation (Go convention).
- Wrap errors with `fmt.Errorf("context: %w", err)`.
- Use `logger.Get()` for all log output — never `fmt.Println` in library packages.
- Exported symbols get doc comments; unexported ones do not need them unless the logic is non-obvious.

---

## Commit Style

```
<area>: <short summary in imperative mood>

Optional longer explanation.
```

Examples:
```
provider/docker: add volume mount support
state: fix stale lock detection on macOS
graph: return deterministic sort order for nodes with same in-degree
```

---

## Checklist Before Opening a PR

- [ ] `go test ./...` passes
- [ ] `go vet ./...` reports no issues
- [ ] New provider includes schema entry and integration test
- [ ] New command acquires the state lock before touching state
- [ ] Error messages follow Go conventions (lowercase, no trailing period)
