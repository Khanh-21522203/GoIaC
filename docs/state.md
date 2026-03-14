# State Management

GoIaC tracks all managed resources in a local state file. This document explains the state format, file layout, locking protocol, and migration system.

---

## State Directory

Running `goiac init` creates a `.goiac/` directory in your project root:

```
.goiac/
├── state.json         Current state of all managed resources
├── state.json.sha256  SHA-256 checksum of state.json
└── state.lock         Present only while an operation is in progress
```

Add `.goiac/` to `.gitignore` unless you intentionally want to share state across machines (shared remote state is not yet supported).

---

## State File Format

`state.json` is a JSON file with the following structure:

```json
{
  "version": 1,
  "last_updated": "2026-02-13T16:00:00+07:00",
  "resources": {
    "<config-resource-id>": {
      "id": "<provider-assigned-id>",
      "type": "<resource-type>",
      "attributes": {
        "<key>": "<value>"
      }
    }
  }
}
```

| Field | Description |
|---|---|
| `version` | Schema version. Used by the migration framework. Currently `1`. |
| `last_updated` | RFC 3339 timestamp of the last successful write. |
| `resources` | Map from user-defined config `id` → `ResourceState`. |

### ResourceState

| Field | Description |
|---|---|
| `id` | Provider-assigned identifier (e.g. Docker container SHA, file path). |
| `type` | Resource type string (e.g. `docker_container`). |
| `attributes` | Provider-reported output attributes. These are what `${...}` interpolation reads from. |

---

## Write Safety

State writes are atomic:

1. The new state is serialized to a temporary file (`state.json.tmp`).
2. A SHA-256 checksum of the new content is computed.
3. The tmp file is renamed to `state.json` (atomic on POSIX filesystems).
4. The checksum is written to `state.json.sha256`.

On load, if the checksum file exists, GoIaC verifies the state file has not been externally modified or corrupted before proceeding.

---

## Partial State on Failure

During `apply`, GoIaC writes state after each successful resource operation (not just at the end). If the process is killed or a provider returns an error mid-way, the state file reflects the resources that were already created.

The next `apply` reads this partial state, detects which resources still need to be created, and continues from where it left off — without re-creating already-existing resources.

---

## Locking

### WithLock Pattern

Commands that modify state acquire the lock via `Manager.WithLock`:

```go
func (m *Manager) WithLock(fn func() error) error {
    if err := m.Lock(); err != nil {
        return err
    }
    defer m.Unlock()
    return fn()
}
```

Commands that read state only (`state show`, `plan`) do **not** acquire the lock — they read state optimistically. Only `apply` and `destroy` call `Lock()` / `defer Unlock()` directly (they manage the lock lifecycle manually because the lock must be held across the entire operation, including the interactive confirmation prompt).

### Lock Acquisition Details

GoIaC uses a file-based lock (`.goiac/state.lock`) to prevent two concurrent operations from modifying state simultaneously.

The lock file is a JSON object:

```json
{
  "locked_at": "2026-02-13T16:00:00+07:00",
  "locked_by": "hostname",
  "process_id": 12345
}
```

**Stale lock detection**: If a lock file exists but `time.Since(lockInfo.LockedAt) > 30 minutes`, GoIaC considers the lock stale and removes it automatically, then retries. Note: the current implementation uses age-based staleness, not PID-liveness. A lock held by a running-but-stuck process for more than 30 minutes will be removed.

**Acquisition**: GoIaC retries up to `LockMaxRetries = 3` times with exponential backoff (100ms, 200ms, 400ms). If all retries fail, it exits with a "failed to acquire lock due to contention, please retry" error.

The lock is always released in a `defer` so it is cleaned up even if a command panics.

---

## State Commands

```bash
# Show all resources in state
goiac state show

# Show a single resource
goiac state show web-server
```

Output format:

```
Resource: web-server
  Type: docker_container
  Provider ID: abc123def456...
  Attributes:
    container_id: abc123def456...
    image:        nginx:latest
    status:       running
    port:         8080
```

---

## State Versioning and Migration

The `version` field in `state.json` tracks the schema version. `MigrateState` is called on every `Load()`.

**Algorithm:**
1. Unmarshal only the `version` field from the raw JSON bytes.
2. If `version == 0` (pre-versioned state), treat it as version 1.
3. If `version > CurrentStateVersion` (1), return an error telling the user to upgrade GoIaC.
4. Loop: while `version < CurrentStateVersion`, call `applyMigration(version, data)` which transforms the raw JSON bytes, increment version.
5. Unmarshal the final bytes into `*State`, force `state.Version = CurrentStateVersion`.

**Adding a migration:** add a `case N:` to `applyMigration` in `pkg/state/migration.go` that transforms raw JSON from version N to N+1, and bump `CurrentStateVersion`.

Migrations operate on raw `[]byte` (not a typed struct) so they can handle the old schema shape freely.

If GoIaC encounters a state version newer than it knows about, it exits with an error rather than silently misinterpreting the state.

---

## Manual State Operations

GoIaC does not yet support `state import` or `state rm`. If you need to manipulate state manually:

1. Stop any running GoIaC process.
2. Edit `.goiac/state.json` directly.
3. Delete `.goiac/state.json.sha256` so the checksum is regenerated on next load, **or** recompute it: `sha256sum .goiac/state.json | awk '{print $1}' > .goiac/state.json.sha256`

Incorrect manual edits can cause GoIaC to attempt to create/update/delete the wrong resources. Always back up state before editing.
