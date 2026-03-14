# Feature: Reconciler

## 1. Purpose

The Reconciler is the core engine of GoIaC. It takes a desired configuration and a recorded state, computes what needs to change (diff), orders those changes correctly (topological sort), resolves cross-resource references (interpolation), and executes the changes through providers.

It is the only package that directly coordinates all other subsystems: config, graph, provider, state, and logger.

## 2. Responsibilities

**Diff engine** (`diff.go`):
- Compare `[]*config.Resource` against `*state.State` → `[]*Change`
- Classify each resource as Create, Update, Delete, or Noop
- Normalize numeric types before comparison to eliminate YAML/JSON type drift
- Produce a human-readable `Reason` for Update changes

**Interpolation resolver** (`interpolate.go`):
- Recursively walk resource properties and replace `${type.id.attr}` with actual values from state
- Return a new `*config.Resource`; never mutate the original

**Change executor** (`executor.go`):
- Execute a `[]*Change` slice, routing each to the correct provider method
- Check `ctx.Err()` before each change to support cancellation
- Return `[]ExecutionResult` with new state or error per change

**Reconciler orchestrator** (`reconciler.go`):
- `Plan`: validate → load state → diff → build/validate graph → return changes
- `Apply`: validate → load state → diff → build/validate graph → sort → execute in order → save state on success and on partial failure

## 3. Non-Responsibilities

- Does not acquire or release the state lock (that is the CLI command layer)
- Does not print output to the user (that is the CLI)
- Does not parse YAML
- Does not implement provider logic

## 4. Architecture Design

```
pkg/reconciler/
├── reconciler.go    Reconciler struct, Plan(), Apply()
├── diff.go          ComputeDiff(), Change, ChangeType, propertiesDiffer(), normalizeValue()
├── executor.go      ExecuteChanges(), ExecutionResult, executeCreate/Update/Delete()
└── interpolate.go   InterpolateReferences(), interpolateValue(), interpolateString()
```

```
Plan(desired):
  ValidateResources(desired)
  state = stateManager.Load()
  changes = ComputeDiff(desired, state)
  graph.Build(desired) → ValidateDAG → ValidateReferences
  return changes

Apply(ctx, desired):
  ValidateResources(desired)
  state = stateManager.Load()
  changes = ComputeDiff(desired, state)
  graph.Build(desired) → validate
  order = graph.TopologicalSortReverse()  // dependency-first
  for each resourceID in order:
      find changes for this resource
      results = ExecuteChanges(ctx, resourceChanges, state, registry)
      on error: Save(state), return error   // partial state safety
      on success: update state.Resources[id]
  handle deletions (resources in state not in desired)
  stateManager.Save(state)
```

## 5. Core Data Structures (Go)

```go
package reconciler

// ChangeType classifies what operation is needed for a resource.
type ChangeType int

const (
    ChangeTypeCreate ChangeType = iota
    ChangeTypeUpdate
    ChangeTypeDelete
    ChangeTypeNoop
)

// Change represents one planned operation on one resource.
type Change struct {
    Type     ChangeType
    Resource *config.Resource     // nil for Delete changes
    OldState *state.ResourceState // nil for Create changes
    Reason   string               // human-readable description of what changed
}

// ExecutionResult is the outcome of executing one Change.
type ExecutionResult struct {
    Change   *Change
    NewState *state.ResourceState // nil for Delete (resource removed)
    Err      error
}

// Reconciler coordinates diff, graph, interpolation, and execution.
type Reconciler struct {
    stateManager *state.Manager
    registry     *provider.Registry
}

func NewReconciler(stateManager *state.Manager, registry *provider.Registry) *Reconciler
func (r *Reconciler) Plan(desired []*config.Resource) ([]*Change, error)
func (r *Reconciler) Apply(ctx context.Context, desired []*config.Resource) error
```

## 6. Public Interfaces

```go
// Exported for use by CLI (printPlan)
func ComputeDiff(desired []*config.Resource, actual *state.State) []*Change

// Exported for use by executor
func InterpolateReferences(resource *config.Resource, currentState *state.State) *config.Resource

// Exported for use by reconciler.Apply
func ExecuteChanges(ctx context.Context, changes []*Change, currentState *state.State, registry *provider.Registry) []ExecutionResult
```

## 7. Internal Algorithms

### ComputeDiff
```
processed = {}
changes = []

for each desired resource:
    processed[resource.ID] = true
    if resource.ID not in state.Resources:
        changes += Change{Create, resource, nil, "resource does not exist"}
    else if propertiesDiffer(resource.Properties, state.Resources[id].Attributes):
        changes += Change{Update, resource, oldState, computeChangedFields(...)}
    else:
        changes += Change{Noop, resource, oldState, ""}

for each id in state.Resources:
    if id not in processed:
        changes += Change{Delete, nil, state.Resources[id], "resource no longer in configuration"}

return changes
```

### propertiesDiffer
```
for each key in desired:
    actualValue, exists = actual[key]
    if !exists || normalizeValue(desired[key]) != normalizeValue(actual[key]):
        return true
return false
```

Note: this is a **one-way diff** — it does not check for keys in `actual` that are absent from `desired`. Provider output attributes (e.g., `container_id`, `status`) are stored in state but not declared in config; they are intentionally ignored by the diff.

### normalizeValue
Converts all numeric types to `float64` for consistent comparison:
```
int    → float64
int64  → float64
float32 → float64
map[string]interface{} → recurse over values
[]interface{}          → recurse over elements
everything else        → unchanged
```

This is necessary because YAML decodes `port: 8080` as `int(8080)` while JSON (state file) decodes `"port": 8080` as `float64(8080)`.

### InterpolateReferences
```
refPattern = regexp.MustCompile(`\$\{(\w+)\.(\w+)\.(\w+)\}`)

InterpolateReferences(resource, state):
    return new Resource{
        ID:   resource.ID,
        Type: resource.Type,
        Properties: interpolateProperties(resource.Properties, state),
    }

interpolateValue(value, state):
    string:                → interpolateString(v, state)
    map[string]interface{}: → recurse over values
    []interface{}:          → recurse over elements
    other:                  → unchanged

interpolateString(s, state):
    refPattern.ReplaceAllStringFunc(s, func(match):
        type       = match[1]  // ignored
        resourceID = match[2]
        attribute  = match[3]
        if state.Resources[resourceID].Attributes[attribute] exists:
            return fmt.Sprint(value)
        else:
            return match  // leave verbatim if not yet in state
    )
```

Unresolved references are left verbatim — not an error. The topological ordering ensures that at the time `InterpolateReferences` is called for resource X, all dependencies of X have already been executed and their attributes are in `currentState`.

### ExecuteChanges (context-aware)
```
for each change:
    if ctx.Err() != nil:
        append ExecutionResult{change, nil, "context cancelled: ..."}
        break
    result = executeChange(ctx, change, state, registry)
    append result
```

### executeCreate
```
resolved = InterpolateReferences(change.Resource, currentState)
prov = registry.Get(change.Resource.Type)
newState = prov.Create(ctx, resolved)
return ExecutionResult{change, newState, nil}
```

### executeDelete
```
prov = registry.Get(change.OldState.Type)
prov.Delete(ctx, change.OldState.ID)
return ExecutionResult{change, nil, nil}
```

### Apply: Partial State Safety
```
for each resource in topological order:
    results = ExecuteChanges(ctx, resourceChanges, currentState, registry)
    for each result:
        if result.Err != nil:
            stateManager.Save(currentState)  // save what we have so far
            return error
        currentState.Resources[id] = result.NewState
// handle deletions after all creates/updates
stateManager.Save(currentState)
```

The in-memory `currentState` is mutated as each resource succeeds. On failure, the current (partially-updated) state is persisted, so the next `apply` can continue from that point.

## 8. Persistence Model

The reconciler reads and writes state through `stateManager.Load()` / `stateManager.Save()`. It does not directly touch any files.

## 9. Concurrency Model

The reconciler is single-threaded. `ExecuteChanges` is sequential (one change at a time). Resources at the same DAG depth could theoretically be parallelised in the future, but this is not implemented.

The `ctx` passed to `Apply` propagates cancellation to provider calls. Each provider is responsible for honouring context cancellation.

## 10. Configuration

No configuration. The 10-minute apply timeout is set by the CLI (`apply.go`), not the reconciler.

## 11. Observability

Uses `logger.Get()`:
- `INFO "loading current state"`
- `INFO "computing diff" desired_count=N current_count=M`
- `INFO "plan complete" changes=N`
- `INFO "executing changes" resource=X count=N`
- `ERROR "change failed" resource=X error=...`
- `ERROR "failed to save partial state" error=...`

## 12. Testing Strategy

**Unit tests** (table-driven, no real providers needed):

`diff_test.go`:
- `TestComputeDiffCreate`: desired has resource not in state → Create
- `TestComputeDiffUpdate`: desired has resource in state with different properties → Update, Reason includes changed field
- `TestComputeDiffDelete`: state has resource not in desired → Delete
- `TestComputeDiffNoop`: desired matches state exactly → Noop
- `TestComputeDiffMixed`: combination of all four cases
- `TestNormalizeValue`: int, int64, float32, map, slice, string → expected float64/unchanged
- `TestPropertiesDifferNumeric`: YAML int vs JSON float64 of same value → not different

`interpolate_test.go`:
- `TestInterpolateSimple`: single `${type.id.attr}` → replaced with state value
- `TestInterpolateNested`: `${...}` inside a map value → resolved
- `TestInterpolateList`: `${...}` inside a list element → resolved
- `TestInterpolateUnresolved`: reference to non-existent resource → left verbatim
- `TestInterpolateImmutability`: original resource not mutated

`reconciler_test.go` (with mock providers):
- `TestPlanValidates`: invalid resource type → error before state load
- `TestPlanCycleDetected`: config with cycle → error
- `TestApplyCreatesResource`: mock provider Create called with correct resolved properties
- `TestApplyPartialStateOnFailure`: first resource succeeds, second fails → state contains first resource

## 13. Open Questions

- Deletion of resources does not currently respect topological order from the original config. It iterates the `changes` slice in compute order. If deleted resources have dependencies on each other, deletions may fail. Should deletions be topologically sorted in reverse as well?
- Should `propertiesDiffer` also detect extra keys in `actual` that were removed from `desired` (e.g., if an optional property was removed)?
