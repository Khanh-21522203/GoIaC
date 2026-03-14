# Code Review Guide — GoIaC

This guide walks you through reviewing the GoIaC codebase as a junior engineer. It tells you **what order to read things**, **what questions to ask yourself in each file**, and **what specific things to verify**. Follow it top to bottom.

---

## Before You Start

### What is this project?

GoIaC is a minimal Infrastructure-as-Code engine — think Terraform, but smaller and written in Go. You write a YAML file describing what infrastructure you want (files, Docker containers, Docker networks), run `goiac apply`, and the tool creates it. Run `goiac destroy` and it tears everything down. It tracks what it created in a local state file so it only changes what actually needs changing.

### Reference documents

Before reading code, skim these to understand the intended design:

| Document | What it explains |
|---|---|
| `README.md` | Feature list, usage, architecture diagram |
| `docs/architecture.md` | Data flow, all key algorithms (diff, interpolation, graph) |
| `docs/configuration.md` | YAML format, `${...}` interpolation syntax |
| `docs/state.md` | State file format, locking, migrations |
| `docs/providers.md` | Provider interface contract, how to add a new provider |
| `plans/` | One plan file per feature — purpose, data structures, algorithms, test strategy |

You do **not** need to read every plan file upfront. Use them as a reference when you reach that package.

### Run everything first

```bash
go build -o goiac ./cmd     # should produce a binary with no errors
go test ./...               # should show ok for all packages with test files
```

If either fails, stop and fix it before reviewing. A codebase that does not build or test cleanly is not ready to review.

---

## How to Navigate the Codebase

```
GoIaC/
├── cmd/main.go                   ← entry point, read last
├── pkg/
│   ├── config/                   ← read 1st: defines Resource and Config structs
│   ├── logger/                   ← read 2nd: trivial, one function
│   ├── state/                    ← read 3rd: state file, locking, migrations
│   ├── graph/                    ← read 4th: DAG and topological sort
│   ├── provider/                 ← read 5th: interface, registry, validation
│   │   ├── local/                ← read 6th: simplest provider
│   │   └── docker/               ← read 7th: more complex provider
│   ├── reconciler/               ← read 8th: the core engine
│   └── cli/                      ← read 9th: wires everything together
└── cmd/main.go                   ← read 10th: flags and REPL
```

Read bottom-up: the packages at the top of the list have no dependencies; the ones at the bottom depend on all of them. Understanding the leaves first makes the higher layers easy to follow.

---

## Package-by-Package Review

---

### 1. `pkg/config/` — Types and Parser

**Files:** `types.go`, `parser.go`, `parser_test.go`

**Read `types.go` first.** It is 11 lines. Ask yourself:
- `Resource.Properties` is `map[string]interface{}` — why not a typed struct? (Hint: properties differ per resource type; they are validated later by the provider schema.)
- Both `Resource` and `Config` have YAML and JSON tags — why both? (Hint: YAML for reading config files; JSON for the state file format in `pkg/state`.)

**Read `parser.go`.** Ask yourself:
- What does `Parse("")` do — does it default to `main.yaml`?
- Does the validator reject an empty resource list? Should it? (An empty list is a valid desired state — it means "delete everything".)
- `parseString` is unexported — why? (Test helper that bypasses the filesystem.)

**Read `parser_test.go`.** Verify:
- [ ] There is a test for duplicate IDs
- [ ] There is a test for missing ID
- [ ] There is a test for missing type
- [ ] There is a test for empty config (should succeed, return empty slice)
- [ ] Tests use `t.TempDir()` — no hardcoded paths

**Cross-check with plan:** `plans/plan-config-parser.md` section 7 describes the parse flow. Does the code match it?

---

### 2. `pkg/logger/` — Logger

**File:** `logger.go`

This is the simplest package. Read it in under a minute.

Ask yourself:
- Where is the logger initialized? (package `init()` — runs before `main`)
- How does the CLI change the log level? (`logger.SetLevel` or `logger.SetJSON` in `cmd/main.go`)
- Why does every other package call `logger.Get()` at log time rather than storing the logger in a struct field? (So all packages immediately see configuration changes made at startup.)

**Watch for:**
- [ ] Any package that uses `fmt.Println` for logging instead of `logger.Get()` — that is a violation of the convention.

---

### 3. `pkg/state/` — State Management

**Files:** `types.go`, `manager.go`, `lock.go`, `migration.go` and their test files

Read `types.go` first. Understand the difference between:
- `State.Resources` key (config resource ID, user-defined, e.g. `"web-server"`)
- `ResourceState.ID` (provider-assigned ID, e.g. Docker container SHA)

This distinction is critical. Many bugs in IaC tools come from confusing these two.

**Read `manager.go`.** Ask yourself:
- What happens if the state file does not exist? (Returns `NewState()` — empty state, not an error.)
- How is the write made atomic? (Write to `.tmp`, then `os.Rename` — atomic on POSIX.)
- What is the purpose of the `.sha256` checksum file?

**Read `lock.go`.** Ask yourself:
- What does `WithLock(fn func() error) error` do and why is it useful?
- How is a stale lock detected? (Age-based: if `LockedAt` is more than 30 minutes ago, the lock is removed. Note: this is **not** PID-liveness — a process that has been running for 30+ minutes and still holds the lock will have it stolen.)
- What is the retry strategy? (3 retries with exponential backoff: 100ms, 200ms, 400ms.)

**Read `migration.go`.** Ask yourself:
- What happens if the state file has `version: 0`? (Treated as version 1.)
- What happens if the state file has a version higher than `CurrentStateVersion`? (Error — do not silently misinterpret newer state.)
- How would you add a new migration? (Add a `case N:` to `applyMigration` and bump `CurrentStateVersion`.)

**Read the test files.** Verify:
- [ ] `TestStateManager_SaveAndLoad`: round-trip test
- [ ] `TestStateManager_LoadEmpty`: missing file returns empty state
- [ ] `TestStateManager_Lock`: second lock call fails
- [ ] `TestStateManager_WithLock`: lock is released even after the fn returns
- [ ] `TestStateManager_ChecksumMismatch`: tampered state file returns error
- [ ] `TestStateManager_StaleLock`: stale lock is removed on next lock attempt
- [ ] `TestMigrateState_ZeroVersion`: version 0 is treated as version 1
- [ ] `TestMigrateState_FutureVersion`: future version returns error

**Cross-check with docs:** `docs/state.md` describes the `WithLock` pattern and lock acquisition details. Verify the code matches.

---

### 4. `pkg/graph/` — Dependency Graph

**Files:** `graph.go`, `toposort.go`, `graph_test.go`

**Read `graph.go`.** The most important thing to understand is the **edge direction**:

```
edges[X] = [Y]  means "X depends on Y"
                i.e. edges point: dependent → dependency
```

Ask yourself:
- How are `${...}` references extracted from properties? (`ExtractReferences` — recursive closure that scans strings, maps, and slices)
- What does `ValidateDAG` check? (Cycle detection via Kahn's algorithm)
- What does `ValidateReferences` check? (All referenced resource IDs exist as nodes)

**Read `toposort.go`.** This is the most counter-intuitive part of the codebase:

- `TopologicalSort()` returns **dependent-first** order (correct for **destroy**)
- `TopologicalSortReverse()` returns **dependency-first** order (correct for **create**)

Why? Because edges point `dependent → dependency`. Kahn's algorithm processes nodes with in-degree 0 first — those are nodes nothing depends on (leaves), which is the destroy order.

Trace through a concrete example to make sure you understand:
```
Resources: net (no deps), web (depends on net)
Edges: web → net
In-degree: web=0, net=1
Kahn's queue starts with: [web]
Result: [web, net]   ← TopologicalSort (destroy order: web before net)
Reversed: [net, web] ← TopologicalSortReverse (create order: net before web)
```

**Read `graph_test.go`.** Verify:
- [ ] `TestGraph_TopologicalSort`: comment asserts web (dependent) comes before net (dependency)
- [ ] `TestGraph_TopologicalSortReverse`: net comes before web
- [ ] `TestExtractReferences`: deduplication test (two references to same resource → only one entry)
- [ ] `TestGraph_ValidateDAG_WithCycle`: injected cycle → error
- [ ] `TestGraph_ValidateReferences_Invalid`: undefined resource reference → error

---

### 5. `pkg/provider/` — Provider System

**Files:** `interface.go`, `registry.go`, `validation.go`, `validation_test.go`

**Read `interface.go`.** Memorize the contract:
- `Read` returns `(nil, nil)` if the resource does not exist — **not** an error
- `Delete` must be idempotent — safe to call on an already-deleted resource
- `resourceID` in `Read`/`Update`/`Delete` is the **provider-assigned** ID (from `ResourceState.ID`), not the user's config ID

**Read `registry.go`.** Ask yourself:
- Why does the registry use `sync.RWMutex`? (It is registered at startup and read concurrently, though in practice registration is single-threaded.)
- What happens if you call `Get` for an unregistered type? (Returns an error — providers are registered in `pkg/cli/cli.go`.)

**Read `validation.go`.** Ask yourself:
- What are the three things `ValidateResource` checks? (1. Type exists in `knownSchemas`. 2. All required properties present. 3. No unknown properties.)
- What happens if you add a new provider but forget to add its schema here? (Its type is rejected as "unknown resource type" before any API calls are made.)

**Critical:** There are two places to update when adding a new provider:
1. `validation.go` — add schema to `knownSchemas`
2. `pkg/cli/cli.go` — register the provider in `registerProviders`

Both must be kept in sync. Forgetting either breaks the provider.

**Read `validation_test.go`.** Verify all five built-in test cases exist.

---

### 6. `pkg/provider/local/` — Local File Provider

**Files:** `file.go`, `file_test.go`

Read this before the Docker provider — it is simpler and establishes the pattern.

**Read `file.go`.** Ask yourself:
- What is `ResourceState.ID` for a `local_file`? (The file path — same as the `path` property. This makes `Read`/`Update`/`Delete` trivial since `resourceID` is the path.)
- What attributes does `Create` return? (`path`, `content`, `size`, `mode`)
- Is `Delete` idempotent? (Yes — `os.IsNotExist(err)` is ignored.)
- Does `Update` handle a changed `path`? (No — it overwrites the old path. Renaming requires delete + create, which the reconciler would do by treating it as an update of the same resource ID.)

**Read `file_test.go`.** This is the reference for how provider tests should be structured. Verify:
- [ ] Full CRUD lifecycle test (`TestFileProvider_CreateReadUpdateDelete`)
- [ ] `Read` on non-existent file returns `(nil, nil)` not an error
- [ ] `Delete` on non-existent file returns no error
- [ ] All tests use `t.TempDir()` — no hardcoded `/tmp` paths

---

### 7. `pkg/provider/docker/` — Docker Provider

**Files:** `container.go`, `network.go`

**Read `container.go`.** Pay special attention to port handling:
```go
switch v := portVal.(type) {
case float64: port = int(v)
case int:     port = v
}
```
Why both? YAML decodes integers as `int`; the state file (JSON) decodes them back as `float64`. Both cases must be handled.

Ask yourself:
- How is `Update` implemented? (Delete + Create — full replacement, no in-place update.)
- Is `Delete` idempotent? (Yes — `client.IsErrNotFound` is checked for both Stop and Remove.)
- What is `ResourceState.ID` for a container? (The Docker-assigned container SHA, e.g. `abc123def...`)

**Read `network.go`.** Same pattern — simpler because networks have no port bindings.

Ask yourself:
- What is the default driver if none is specified? (`"bridge"`)
- Does `Create` store the resolved network ID or the name in `ResourceState.ID`? (The Docker-assigned network ID — `resp.ID`. This is what gets passed to containers via interpolation.)

**No tests for Docker providers** — they require a running Docker daemon. If you want to verify them, run `go test ./pkg/provider/docker/... -v` with Docker running.

---

### 8. `pkg/reconciler/` — The Core Engine

**Files:** `diff.go`, `interpolate.go`, `executor.go`, `reconciler.go` and test files

This is the most important package. Read each file in order.

**Read `diff.go`.** Understand the `Change` struct:
```go
type Change struct {
    Type             ChangeType           // Create / Update / Delete / Noop
    Resource         *config.Resource     // nil for Delete
    OldState         *state.ResourceState // nil for Create
    Reason           string               // human-readable diff description
    ConfigResourceID string               // config-level ID (map key in state)
}
```

Ask yourself:
- Why does `propertiesDiffer` only check keys present in `desired`, not keys only in `actual`? (Provider output attributes like `container_id`, `status` are stored in state but not declared in config — intentionally ignored by the diff.)
- Why is `normalizeValue` needed? (YAML decodes `port: 8080` as `int(8080)`; JSON decodes it back as `float64(8080)`. Without normalization, every numeric property would show as changed on every plan.)

**Read `interpolate.go`.** Trace through the resolution of `${docker_network.app-network.network_id}`:
1. Regex captures: `type=docker_network`, `resource_id=app-network`, `attribute=network_id`
2. Look up `state.Resources["app-network"]`
3. Look up `.Attributes["network_id"]`
4. Replace with `fmt.Sprint(value)`

Ask yourself:
- What happens if `app-network` is not yet in state at resolve time? (The `${...}` expression is left verbatim — not an error. The topological ordering ensures dependencies are applied before their dependents.)
- Does `InterpolateReferences` mutate the original resource? (No — it creates and returns a new `*config.Resource`.)

**Read `executor.go`.** Ask yourself:
- What does `ExecuteChanges` do with a cancelled context? (Checks `ctx.Err()` before each change and short-circuits with an error — supports graceful shutdown.)
- For `executeCreate`, when is interpolation called? (Before calling the provider — so the provider receives fully resolved properties.)

**Read `reconciler.go`.** Trace through the full `Apply` flow:

```
1. ValidateResources   — fail fast before any I/O
2. stateManager.Load  — get current state
3. ComputeDiff        — desired vs actual → []Change
4. graph.Build        — extract ${...} references
5. ValidateDAG        — reject cycles
6. ValidateReferences — reject undefined references
7. TopologicalSortReverse — dependency-first order
8. ExecuteChanges     — per resource, in order
   → if error: Save(partial state), return error
   → if success: update in-memory state
9. Handle deletes     — topologically sorted, then executed
10. stateManager.Save — persist final state
```

**Read the test files.** Verify:
- [ ] `TestComputeDiff_*` covers Create, Update, Delete, Noop, Mixed
- [ ] `TestPropertiesDiffer_NumericTypes`: `int(8080)` == `float64(8080)` after normalization
- [ ] `TestInterpolateReferences_Simple`: basic replacement works
- [ ] `TestInterpolateReferences_NoMatch`: unresolved ref left verbatim
- [ ] `TestInterpolateReferences_DoesNotMutateOriginal`: original unchanged
- [ ] `TestReconciler_PlanDetectsCycle`: cyclic config returns error
- [ ] `TestReconciler_ApplyPartialStateOnFailure`: partial state is saved on error

---

### 9. `pkg/cli/` — CLI Commands

**Files:** `cli.go`, `init.go`, `plan.go`, `apply.go`, `destroy.go`, `state.go`

**Read `cli.go`.** This is the wiring file — it constructs all subsystems and registers providers. Ask yourself:
- What happens if Docker is not running? (The Docker SDK's `NewClientWithOpts` only tests connectivity on first API call, not at construction time — so `NewCLI` succeeds even without Docker.)

**Read `apply.go`.** Trace the lock lifecycle:
```
Parse config (no lock)
Plan — load state, compute diff (no lock)
Print plan (no lock)
Prompt user — may wait indefinitely (no lock)
Acquire lock
Apply with 10-min timeout + SIGINT handler
Release lock
```

Ask yourself:
- Why is the lock acquired **after** the prompt? (To avoid holding the lock while waiting for user input — a user who steps away could trigger the 30-minute stale lock threshold.)
- What happens on SIGINT during apply? (`cancel()` is called → `ctx.Err()` check in executor fires → partial state is saved → apply returns an error.)

**Read `destroy.go`.** Ask yourself:
- How does destroy determine deletion order without the original config? (Builds a graph from `state.Attributes` as properties, runs `TopologicalSort`. Note: attributes store resolved values, not `${...}` patterns, so edge detection is limited.)

---

### 10. `cmd/main.go` — Entry Point

**File:** `main.go`

Read this last — it is the glue. Ask yourself:
- How are global flags (`--log-level`, `--log-json`) parsed? (Extracted before command dispatch in `parseGlobalFlags`. Both flags are collected first, then applied together, so they combine correctly.)
- What happens when no command is given? (`runInteractiveMode` starts the REPL.)
- How does the REPL differ from normal mode? (Same `executeCommand` function — the REPL just calls it in a loop with stdin input.)

---

## Cross-Cutting Concerns

After reading all packages, verify these project-wide properties:

### Error wrapping
Every error should be wrapped with context. Look for:
```go
// Good
return nil, fmt.Errorf("failed to create container: %w", err)

// Bad — loses context
return nil, err
```
Spot-check 5 error return sites in different packages.

### Context propagation
Every provider method receives a `context.Context`. Check:
- [ ] `docker/container.go` passes `ctx` to every Docker API call
- [ ] `docker/network.go` passes `ctx` to every Docker API call
- [ ] `local/file.go` does not need to use `ctx` (filesystem operations are synchronous)
- [ ] `executor.go` checks `ctx.Err()` before each change

### No `fmt.Println` in library packages
`fmt.Println` should only appear in `cmd/main.go` and `pkg/cli/`. Library packages (`reconciler`, `state`, `graph`, `provider`) should use `logger.Get()` for all output.

### Test isolation
Every test that touches the filesystem should use `t.TempDir()`. Search for any test using `/tmp/` directly:
```bash
grep -r '"/tmp/' pkg/
```
Any hit is a test isolation problem.

---

## Known Limitations

These are documented design decisions — not bugs — but you should understand them:

1. **Delete ordering in `apply` is limited**: When resources are removed from config, the reconciler sorts the deletions via a graph built from state attributes. But state attributes store resolved values (e.g., `network_id: "abc123"`), not `${...}` patterns. So dependency edges are not detected, and deletion order is deterministic but not guaranteed to respect cross-resource dependencies.

2. **Stale lock is age-based**: A lock older than 30 minutes is removed regardless of whether the process that wrote it is still running. A long `apply` on a slow machine could have its lock stolen.

3. **`propertiesDiffer` is one-directional**: It checks keys in `desired` against `actual`, but not the reverse. Removing an optional property from your config is not detected as a change.

4. **Docker `Update` is destructive**: Both `docker_container` and `docker_network` implement update as delete + recreate. There is no in-place update.

---

## Review Checklist

After completing all of the above, answer each question:

**Correctness**
- [ ] Does `TopologicalSort` return dependent-first (destroy) order and `TopologicalSortReverse` return dependency-first (create) order?
- [ ] Does `normalizeValue` handle `int`, `int64`, `float32`, `int32`, `uint`, `uint32`, `uint64`?
- [ ] Does `InterpolateReferences` return a new resource without mutating the original?
- [ ] Does partial state get saved when `Apply` fails mid-way?
- [ ] Is `Provider.Delete` idempotent for both `local_file` and `docker_container`?

**Safety**
- [ ] Does `manager.Save` write atomically (tmp file + rename)?
- [ ] Is the state lock always released (even on panic or error)?
- [ ] Does `--log-level` and `--log-json` combine correctly when both are provided?

**Test coverage**
- [ ] `pkg/config` — parsing and validation covered
- [ ] `pkg/state` — save/load, locking, checksum, stale lock, migration covered
- [ ] `pkg/graph` — DAG, cycle, toposort, reference extraction covered
- [ ] `pkg/provider` — schema validation covered
- [ ] `pkg/provider/local` — full CRUD lifecycle covered
- [ ] `pkg/reconciler` — diff, interpolation, executor, reconciler orchestration covered

**Gaps (known)**
- [ ] `cmd/` has no tests
- [ ] `pkg/cli/` has no tests
- [ ] `pkg/provider/docker/` has no tests (requires Docker)
- [ ] No end-to-end test of the full apply → state → destroy cycle
