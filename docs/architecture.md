# Architecture

GoIaC follows the same conceptual model as Terraform: parse desired state from a config file, compare it against recorded actual state, compute a diff, then execute changes through provider plugins. This document explains how each layer fits together.

---

## High-Level Flow

```
main.yaml  →  Config Parser  →  Reconciler  →  Providers  →  Real Infrastructure
                                     ↕
                              State Manager  ↔  .goiac/state.json
```

Every command (`plan`, `apply`, `destroy`) runs through the same core pipeline. The difference is in what the Reconciler does at the end:

- `plan` — computes and prints the diff, stops before executing.
- `apply` — computes the diff, prompts for confirmation, then executes changes.
- `destroy` — treats all current state resources as targeted for deletion.

---

## Package Map

```
GoIaC/
├── cmd/              Entry point. Parses flags and dispatches to CLI commands.
└── pkg/
    ├── cli/          One file per command. Owns the lock lifecycle.
    ├── config/       YAML parser + validation. Produces []*Resource.
    ├── graph/        DAG builder and topological sort (Kahn's algorithm).
    ├── reconciler/   Diff engine, interpolation resolver, change executor.
    ├── provider/     Provider interface, thread-safe registry, schema validation.
    │   ├── docker/   Docker container and network implementations.
    │   └── local/    Local file implementation.
    ├── state/        State file I/O, atomic writes, file locking, migrations.
    └── logger/       slog wrapper with text/JSON output modes.
```

---

## Data Flow for `goiac apply`

```
1.  CLI acquires lock          pkg/state  → .goiac/state.lock
2.  Parser reads main.yaml     pkg/config → []*config.Resource
3.  Validator checks schemas   pkg/provider (validation.go)
4.  State Manager loads state  pkg/state  → *state.State
5.  Diff engine compares       pkg/reconciler/diff.go
      desired resources vs. state.Resources
      → []ChangeSet{action: Create|Update|Delete|Noop}
6.  Graph builder builds DAG   pkg/graph
      scans Properties for ${type.id.attr} references
      → directed edges: dependency → dependent
7.  Topological sort orders    pkg/graph/toposort.go (Kahn's algorithm)
      changes so dependencies execute first
8.  Executor runs changes      pkg/reconciler/executor.go
      for each change in order:
        a. Resolve ${...} expressions from state attributes
        b. Call Provider.Create / .Update / .Delete
        c. Update in-memory state immediately
        d. Save state to disk after each resource (partial-state safety)
9.  State Manager saves state  pkg/state  → .goiac/state.json + .sha256
10. CLI releases lock
```

If step 8 fails mid-way, the partial state written in 8d means the next `apply` knows which resources were already created and will not duplicate them.

---

## Key Interfaces

### Provider

```go
type Provider interface {
    Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error)
    Read(ctx context.Context, resourceID string)         (*state.ResourceState, error)
    Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error)
    Delete(ctx context.Context, resourceID string)       error
}
```

- `Read` returns `(nil, nil)` when the resource does not exist — this is not an error.
- `Delete` must be idempotent; calling it on an already-deleted resource must not error.
- Docker providers implement `Update` as delete + create (full replacement).

### State

```go
type State struct {
    Version     int
    LastUpdated string
    Resources   map[string]*ResourceState   // keyed by config resource ID
}

type ResourceState struct {
    ID         string                 // provider-assigned ID (e.g. Docker container ID)
    Type       string
    Attributes map[string]interface{} // outputs visible to ${...} interpolation
}
```

The `ID` inside `ResourceState` is the **provider-assigned** ID (e.g. the Docker container SHA), while the map key is the **user-defined** config ID (e.g. `"web-server"`). Providers receive the config resource ID when doing `Read`/`Update`/`Delete` so they can look up the underlying infrastructure object.

---

## Reconciler: Diff Engine

`ComputeDiff` produces a `[]*Change` slice. Each `Change` has:

```go
type ChangeType int
const (
    ChangeTypeCreate ChangeType = iota
    ChangeTypeUpdate
    ChangeTypeDelete
    ChangeTypeNoop
)

type Change struct {
    Type     ChangeType
    Resource *config.Resource     // nil for Delete changes
    OldState *state.ResourceState // nil for Create changes
    Reason   string               // human-readable diff description
}
```

**Diff algorithm:**
1. Iterate over desired resources. For each:
   - Not in state → `Create`
   - In state but properties differ → `Update` (with `Reason` listing changed fields)
   - In state and properties match → `Noop`
2. Iterate over state resources. Any not in desired config → `Delete`

**Property comparison** uses `reflect.DeepEqual` after running both sides through `normalizeValue`. This is required because YAML decodes all numbers as `float64`, while the state file (JSON) may round-trip integers as `int` or `float64` depending on the value. `normalizeValue` converts `int`, `int64`, and `float32` → `float64`, and recurses into nested maps and slices.

Without normalization, a property `port: 8080` would always show as changed because the YAML-decoded `float64(8080)` would not match the JSON-decoded `float64(8080)` from state (they would actually match, but `int(8080)` vs `float64(8080)` would not).

---

## Interpolation Resolver

Pattern: `\$\{(\w+)\.(\w+)\.(\w+)\}` — captures `(type, resource_id, attribute)`.

`InterpolateReferences` returns a new `*config.Resource` with all `${...}` expressions replaced by resolved values. The original resource is never mutated.

**Walk algorithm** (`interpolateValue`):
- `string` → scan with regex, replace each match by looking up `state.Resources[resource_id].Attributes[attribute]`
- `map[string]interface{}` → recurse over every value
- `[]interface{}` → recurse over every element
- anything else → return unchanged

**Type** field in `${type.resource_id.attribute}` is captured by the regex but **ignored** at resolution time. Only `resource_id` and `attribute` matter. The type is only used by the graph builder to construct edge labels.

**Unresolved references** (attribute not in state yet): `fmt.Sprint(attrValue)` is called when the attribute exists; if either the `resource_id` or `attribute` key lookup fails, the original `${...}` expression is returned verbatim. This means an unresolved reference silently passes through — callers must ensure all dependencies are resolved before interpolation by using the topological order.

---

## Dependency Graph

**Edge direction**: `edges[dependent] = [dependency1, dependency2, ...]`

Reading: "resource X depends on resource Y" is stored as `edges["X"] = ["Y"]`.

**Reference extraction** (`ExtractReferences`): recursive closure `scan(value)`:
- `string` → `refPattern.FindAllStringSubmatch` → collect `match[2]` (resource_id) deduplicated
- `map[string]interface{}` → recurse over all values
- `[]interface{}` → recurse over all elements

**Build**: first pass registers all nodes; second pass calls `ExtractReferences` on each resource's `Properties` and adds edges.

**`ValidateDAG`**: Kahn's algorithm on the graph. If `processed != len(nodes)`, at least one cycle exists. Reports stuck node IDs in the error.

**`ValidateReferences`**: checks every `edges[node]` target exists in `nodes`. Reports which resource references which undefined resource.

**`TopologicalSort`**: Kahn's algorithm, returns dependency-first order (nodes with in-degree 0 first). Since edges go `dependent → dependency`, in-degree 0 means "no dependents" — i.e., leaves of the dependency tree. This gives **dependent-first** order, which is correct for **destroy**.

**`TopologicalSortReverse`**: reverses the Kahn output → **dependency-first** order, correct for **create/update**.

The reconciler calls `TopologicalSortReverse` for apply and `TopologicalSort` for destroy.

---

## Apply: Signal Handling and Timeout

```
Apply():
  Lock()
  defer Unlock()
  Parse YAML
  Plan (validate + diff + build graph)
  Print plan, prompt for confirmation (skipped with --auto-approve)
  ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
  defer cancel()
  signal.Notify(sigChan, SIGINT, SIGTERM)
  go func() { <-sigChan; cancel() }()
  reconciler.Apply(ctx, desired)
    → for each change: check ctx.Err() first; if cancelled, return early
    → on provider error: save partial state, return error
  Save final state
  Unlock
```

The 10-minute timeout is the outer bound. `SIGINT`/`SIGTERM` triggers `cancel()`, which propagates through `ctx` to the executor's `ctx.Err()` check at the top of each change iteration. Partial state is always saved before the error is returned.

---

## State Safety

| Mechanism | Location | Purpose |
|---|---|---|
| Atomic write | `state/manager.go` | Write to tmp file, then rename — prevents corrupt state on crash |
| SHA-256 checksum | `.goiac/state.json.sha256` | Detects external modification or disk corruption |
| File lock | `.goiac/state.lock` | Prevents concurrent `apply` runs |
| Stale lock detection | `state/lock.go` | Removes locks from dead processes (PID check) |
| Exponential backoff | `state/lock.go` | Retries lock acquisition up to a configured timeout |
| Version migration | `state/migration.go` | Upgrades state from older schema versions on load |

---

## Structured Logging

All packages use `logger.Get()` to obtain the shared `slog.Logger`. The CLI sets the level and format once at startup via `--log-level` and `--log-json` flags; every package inherits that configuration automatically.
