# Feature: Logger

## 1. Purpose

The Logger package provides a single shared `*slog.Logger` instance that every other package obtains via `logger.Get()`. It is configured once at program startup by the CLI flag handler (`--log-level`, `--log-json`), and all packages inherit that configuration automatically without needing to pass a logger around.

## 2. Responsibilities

- Initialize a default `*slog.Logger` (text format, `INFO` level) at package init time
- Expose `Get()` to return the current logger
- Expose `SetLevel(level slog.Level)` to reconfigure to text format with a different level
- Expose `SetJSON(level slog.Level)` to switch to JSON output with a given level
- Write all log output to `os.Stderr`

## 3. Non-Responsibilities

- Does not add structured fields automatically (callers add their own)
- Does not support log rotation or file output
- Does not support per-package log levels
- Does not provide a mock logger for tests

## 4. Architecture Design

```
cmd/main.go
  parse --log-level, --log-json flags
  call logger.SetLevel() or logger.SetJSON()
         |
         v
   logger.defaultLogger (package-level var)
         |
    logger.Get()
         |
   +-----------+-----------+-----------+
   |           |           |           |
pkg/cli  pkg/reconciler pkg/state  pkg/graph
  log.Info(...)
```

The package-level `defaultLogger` variable is overwritten by `SetLevel`/`SetJSON`. Since `Get()` is called at log time (not stored in structs), all packages immediately see the new configuration.

## 5. Core Data Structures (Go)

```go
package logger

import (
    "log/slog"
    "os"
)

var defaultLogger *slog.Logger

func init() {
    defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
}

func Get() *slog.Logger {
    return defaultLogger
}

func SetLevel(level slog.Level) {
    defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
        Level: level,
    }))
}

func SetJSON(level slog.Level) {
    defaultLogger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
        Level: level,
    }))
}
```

## 6. Public Interfaces

```go
func Get() *slog.Logger
func SetLevel(level slog.Level)
func SetJSON(level slog.Level)
```

## 7. Internal Algorithms

Not applicable. Configuration replaces the package-level pointer atomically (single assignment is safe in Go for pointer-sized values, though a race detector test would be needed for true concurrent safety — see Concurrency Model).

## 8. Persistence Model

Not applicable. Logger configuration is in-memory only, re-established each run from CLI flags.

## 9. Concurrency Model

`defaultLogger` is a package-level pointer. `SetLevel`/`SetJSON` are called once at startup before any goroutines are spawned, so there is no concurrent write. `Get()` is read-only after initialization.

If future code calls `SetLevel` concurrently, an `atomic.Pointer[slog.Logger]` should be used. For now, the single-init pattern is sufficient.

## 10. Configuration

Configured by CLI flags in `cmd/main.go`:

| Flag | Effect |
|---|---|
| `--log-level debug` | `logger.SetLevel(slog.LevelDebug)` |
| `--log-level info` | default; no call needed |
| `--log-level warn` | `logger.SetLevel(slog.LevelWarn)` |
| `--log-level error` | `logger.SetLevel(slog.LevelError)` |
| `--log-json` | `logger.SetJSON(parsedLevel)` instead of `SetLevel` |

`--log-json` and `--log-level` are parsed together; if both are set, `SetJSON` is called with the parsed level.

## 11. Observability

The logger is the observability primitive. It does not observe itself.

## 12. Testing Strategy

No dedicated tests for this package. The behavior is trivial (assign a new `slog.Logger`).

If tests are added:
- `TestGetReturnsNonNil`: assert `logger.Get() != nil` after `init()`
- `TestSetLevelChangesLogger`: call `SetLevel(slog.LevelDebug)`, call `Get()`, assert the new logger is different from the old
- `TestSetJSONChangesLogger`: same for `SetJSON`

## 13. Open Questions

None.
