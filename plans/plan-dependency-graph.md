# Feature: Dependency Graph

## 1. Purpose

The Dependency Graph automatically determines the correct execution order for infrastructure operations. When resource B references an attribute of resource A, GoIaC must create A before B and destroy B before A — without the user having to specify this order manually.

The graph package builds a directed acyclic graph (DAG) from the `${type.resource_id.attribute}` references found in resource properties, validates it for cycles and undefined references, and provides a topological sort for both creation and destruction order.

## 2. Responsibilities

- Build a DAG from a `[]*config.Resource` slice by scanning every property value for `${...}` references
- Register all resource IDs as nodes
- Add edges: `edges[dependent] = [...dependencies]`
- Validate that no cycles exist (reject configs that would deadlock)
- Validate that all referenced resource IDs exist in the config
- Provide `TopologicalSort()` → dependent-first order (for destroy)
- Provide `TopologicalSortReverse()` → dependency-first order (for create/update)
- Expose `GetDependencies(nodeID)` and `GetNodes()` for inspection

## 3. Non-Responsibilities

- Does not resolve `${...}` expressions to actual values (that is `pkg/reconciler/interpolate.go`)
- Does not know about resource types or provider semantics
- Does not persist the graph

## 4. Architecture Design

```
[]*config.Resource
        |
        v
graph.Build(resources)
   pass 1: register all nodes
   pass 2: ExtractReferences(resource.Properties) → edges
        |
        +-- ValidateDAG()       (cycle check via Kahn's algorithm)
        +-- ValidateReferences() (undefined node check)
        |
        v
TopologicalSort()        → dependent-first (destroy order)
TopologicalSortReverse() → dependency-first (create order)
```

### Edge Direction Convention

`edges[X] = [Y, Z]` means "X depends on Y and Z". Edges point **from dependent to dependency**.

Kahn's algorithm counts in-degrees from this convention. Nodes with in-degree 0 are those that nothing depends on — i.e., the leaves of the dependency tree — which are destroyed first. This is why `TopologicalSort()` (plain Kahn) gives destroy order, and `TopologicalSortReverse()` gives create order.

## 5. Core Data Structures (Go)

```go
package graph

import (
    "GoIaC/pkg/config"
    "fmt"
    "regexp"
)

// refPattern matches ${type.resource_id.attribute}
var refPattern = regexp.MustCompile(`\$\{(\w+)\.(\w+)\.(\w+)\}`)

type Graph struct {
    nodes map[string]*Node   // key = resource ID
    edges map[string][]string // key = dependent ID, value = dependency IDs
}

type Node struct {
    ID       string
    Type     string
    Resource *config.Resource
}

func NewGraph() *Graph {
    return &Graph{
        nodes: make(map[string]*Node),
        edges: make(map[string][]string),
    }
}
```

## 6. Public Interfaces

```go
func NewGraph() *Graph
func (g *Graph) Build(resources []*config.Resource) error
func (g *Graph) ValidateDAG() error
func (g *Graph) ValidateReferences() error
func (g *Graph) TopologicalSort() ([]string, error)
func (g *Graph) TopologicalSortReverse() ([]string, error)
func (g *Graph) GetDependencies(nodeID string) []string
func (g *Graph) GetNodes() map[string]*Node

// Package-level helper used by both graph and reconciler/interpolate
func ExtractReferences(properties map[string]interface{}) []string
```

## 7. Internal Algorithms

### Build
```
1. for each resource: nodes[resource.ID] = &Node{...}
2. for each resource:
     refs = ExtractReferences(resource.Properties)
     for each ref: edges[resource.ID] = append(edges[resource.ID], ref)
```

### ExtractReferences
Recursive closure `scan(value interface{})`:
```
switch value:
case string:
    refPattern.FindAllStringSubmatch → collect match[2] (resource_id), deduplicate
case map[string]interface{}:
    for _, v := range val { scan(v) }
case []interface{}:
    for _, item := range val { scan(item) }
```
Returns deduplicated list of resource IDs referenced by the given properties map.

### ValidateDAG (Kahn's algorithm for cycle detection)
```
inDegree[id] = 0 for all nodes
for from, tos in edges:
    for each to: inDegree[to]++

queue = nodes with inDegree == 0
processed = 0

for head in queue:
    processed++
    for each to in edges[head]:
        inDegree[to]--
        if inDegree[to] == 0: enqueue(to)

if processed != len(nodes):
    stuckNodes = nodes where inDegree > 0
    return error("circular dependency: stuck nodes: ...")
```

### ValidateReferences
```
for node, deps in edges:
    for dep in deps:
        if dep not in nodes:
            return error("resource X references undefined resource Y")
```

### TopologicalSort (Kahn's, returns dependent-first for destroy)
Same Kahn traversal as ValidateDAG, but collects the output order:
```
order = []
for head in queue:
    order = append(order, head)
    for each to: decrement inDegree, enqueue if 0
return order   // dependent-first (leaves first)
```

### TopologicalSortReverse (dependency-first for create)
```
order, _ = TopologicalSort()
reverse(order)
return order
```

## 8. Persistence Model

The graph is built in-memory for each command invocation and discarded. It is never written to disk.

## 9. Concurrency Model

`Graph` is not thread-safe. It is built and used within a single goroutine (the CLI command handler). No locking needed.

## 10. Configuration

No configuration. The `${...}` regex pattern is a compile-time constant.

## 11. Observability

No metrics or logging. Errors are returned with descriptive messages including resource IDs.

## 12. Testing Strategy

**Unit tests** (in `pkg/graph/graph_test.go`, table-driven):

- `TestBuildSimpleGraph`: two resources, one references the other → correct edge
- `TestExtractReferences`: string with `${type.id.attr}`, map with nested refs, list with refs, no refs → correct deduplication
- `TestValidateDAGNoCycle`: linear chain A→B→C → no error
- `TestValidateDAGCycle`: A→B, B→A → error contains stuck node IDs
- `TestValidateReferencesValid`: all referenced IDs exist → no error
- `TestValidateReferencesUndefined`: resource references non-existent ID → error naming both resources
- `TestTopologicalSortLinear`: A depends on B → sort gives [A, B] (A is destroyed first)
- `TestTopologicalSortReverse`: same → reverse gives [B, A] (B is created first)
- `TestTopologicalSortIndependent`: no dependencies → valid sort (any order)
- `TestTopologicalSortCycle`: cycle → error

## 13. Open Questions

- `ExtractReferences` has a `TODO: remove recursion` comment. The recursive closure could be replaced with an explicit stack for very deep nested properties, but this is unlikely to be a practical concern.
- Should the graph distinguish between "same-type" and "cross-type" references for future parallel execution at the same DAG depth?
