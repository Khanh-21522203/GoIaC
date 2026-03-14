# Feature: State Manager

## 1. Purpose

The State Manager is responsible for persisting and loading the record of all infrastructure resources that GoIaC currently manages. It is the source of truth for "what exists" and provides the baseline against which the reconciler computes diffs.

Every `apply`, `destroy`, and `state show` command reads from or writes to state through this package. Without it, GoIaC would have no memory of what it created and could not detect drift, compute diffs, or destroy resources.

## 2. Responsibilities

- Load `*State` from `.goiac/state.json`; return an empty `*State` if the file does not exist
- Verify the SHA-256 checksum on load; return an error if the file has been externally modified
- Run state migrations before returning the loaded state
- Save `*State` to `.goiac/state.json` atomically (write to `.tmp`, rename)
- Compute and write a SHA-256 checksum to `.goiac/state.json.sha256` after every save
- Provide `Lock()` and `Unlock()` for exclusive access
- Provide `WithLock(fn func() error) error` as a convenience wrapper
- Provide helper methods `Update`, `DeleteResource`, `GetResource` for in-memory state mutation

## 3. Non-Responsibilities

- Does not perform the file lock internally during `Load`/`Save` — callers are responsible for locking before calling these (except commands that use `WithLock`)
- Does not know about resource types or provider semantics
- Does not parse YAML config

## 4. Architecture Design

```
pkg/state/
├── types.go      State, ResourceState, LockInfo structs
├── manager.go    Manager: Load, Save, Update, DeleteResource, GetResource
├── lock.go       Manager: Lock, Unlock, WithLock, checkStaleLock
└── migration.go  MigrateState, applyMigration, CurrentStateVersion
```

```
                    .goiac/
                    ├── state.json          (JSON, indented)
                    ├── state.json.sha256   (hex SHA-256 of state.json)
                    └── state.lock          (JSON LockInfo, present while locked)

Manager.Load()
  → ReadFile(state.json)
  → verify checksum
  → MigrateState(data) → *State

Manager.Save(*State)
  → json.MarshalIndent
  → WriteFile(state.json.tmp)
  → Rename(tmp → state.json)
  → WriteFile(state.json.sha256)
```

## 5. Core Data Structures (Go)

```go
package state

import "time"

type State struct {
    Version     int                       `json:"version"`
    LastUpdated string                    `json:"last_updated"` // RFC 3339
    Resources   map[string]*ResourceState `json:"resources"`    // key = config resource ID
}

type ResourceState struct {
    ID         string                 `json:"id"`         // provider-assigned ID
    Type       string                 `json:"type"`
    Attributes map[string]interface{} `json:"attributes"` // provider output attributes
}

type LockInfo struct {
    LockedAt  time.Time `json:"locked_at"`
    LockedBy  string    `json:"locked_by"`  // always "goiac"
    ProcessID int       `json:"process_id"`
}

func NewState() *State {
    return &State{
        Version:   1,
        Resources: make(map[string]*ResourceState),
    }
}

// Manager owns the state directory path and exposes all state operations.
type Manager struct {
    stateDir string // default: ".goiac"
}

func NewManager() *Manager {
    return &Manager{stateDir: StateDir}
}
```

### File constants

```go
const (
    StateDir          = ".goiac"
    StateFile         = "state.json"
    StateChecksumFile = "state.json.sha256"
    StateLockFile     = "state.lock"
)
```

## 6. Public Interfaces

```go
// Construction
func NewManager() *Manager
func NewState() *State

// I/O
func (m *Manager) Load() (*State, error)
func (m *Manager) Save(s *State) error

// In-memory helpers (no I/O)
func (m *Manager) Update(s *State, resourceID string, rs *ResourceState)
func (m *Manager) DeleteResource(s *State, resourceID string)
func (m *Manager) GetResource(s *State, resourceID string) (*ResourceState, bool)

// Locking
func (m *Manager) Lock() error
func (m *Manager) Unlock() error
func (m *Manager) WithLock(fn func() error) error
```

## 7. Internal Algorithms

### Load
```
1. ReadFile(".goiac/state.json")
   - if os.IsNotExist → return NewState(), nil
2. ReadFile(".goiac/state.json.sha256")
   - if file exists: compute sha256(data), compare → error if mismatch
3. MigrateState(data) → *State
4. return *State
```

### Save
```
1. state.LastUpdated = time.Now().Format(time.RFC3339)
2. data = json.MarshalIndent(state, "", "  ")
3. MkdirAll(".goiac", 0755)
4. WriteFile(".goiac/state.json.tmp", data, 0644)
5. Rename(".goiac/state.json.tmp", ".goiac/state.json")
   - if Rename fails: Remove(tmp), return error
6. checksum = sha256(data) → hex string
7. WriteFile(".goiac/state.json.sha256", checksum, 0644)
```

The `Rename` in step 5 is atomic on POSIX filesystems (Linux, macOS). On Windows it is not atomic but acceptable for a local-dev tool.

### Checksum
```go
func computeChecksum(data []byte) string {
    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:])
}
```

## 8. Persistence Model

| File | Format | Purpose |
|---|---|---|
| `state.json` | Indented JSON | Full infrastructure state |
| `state.json.sha256` | 64-char hex string | Integrity check |
| `state.lock` | JSON `LockInfo` | Mutual exclusion |

`state.json` is overwritten atomically. `.sha256` is overwritten in-place (non-atomic) because even if the checksum file is corrupted, the worst outcome is a spurious integrity error on next load, which is safe.

## 9. Concurrency Model

The file lock (`state.lock`) serializes concurrent GoIaC processes. Within a single process, state is single-threaded: commands hold the lock for the duration of their operation and do not share the `*State` across goroutines.

`Manager` itself has no mutexes. Thread safety is the caller's responsibility (the lock).

## 10. Configuration

`Manager` is constructed with a hardcoded `stateDir = ".goiac"`. There is no configuration to change the state directory in the current implementation. Tests set `stateDir` by creating a `Manager` with a custom `stateDir` field (if the field is exported or a constructor with options is added).

## 11. Observability

- `Load` and `Save` are called before and after every apply/destroy; no metrics emitted.
- Errors include full context (file path, reason) for easy debugging.
- The reconciler logs `"loading current state"` and `"failed to save partial state"` around state operations.

## 12. Testing Strategy

**Unit tests** (in `pkg/state/manager_test.go`):

- `TestSaveLoad`: save a state with resources, load it back, assert deep equality
- `TestLoadEmptyState`: no state file → `NewState()` returned
- `TestChecksumMismatch`: tamper with state.json after saving → load returns checksum error
- `TestWithLock`: call `WithLock(fn)`, assert fn runs and lock file is cleaned up after
- `TestLockContention`: acquire lock in one call, assert second `Lock()` fails with contention error
- `TestStaleLockRemoval`: create a lock file with `LockedAt` > 30 min ago, assert next `Lock()` succeeds
- `TestMigrationCurrentVersion`: save v1 state, load it → no migration runs, same data returned
- `TestMigrationV0ToV1`: inject raw JSON with `version: 0`, assert loaded state has `version: 1`
- `TestMigrationFutureVersion`: inject JSON with `version: 99`, assert error

All tests use `t.TempDir()` to isolate state files.

## 13. Open Questions

- Should the lock use PID-liveness checks (`os.FindProcess`) instead of age-based staleness? Currently a 30-minute-old lock from a running process would be removed.
- Should `stateDir` be configurable via an environment variable or flag for CI use cases?
