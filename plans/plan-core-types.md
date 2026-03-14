# Feature: Core Types

## 1. Purpose

Core Types defines the two foundational structs that flow through every layer of GoIaC: `Resource` (a single desired infrastructure object) and `Config` (the parsed YAML file). All other packages import from here — the parser produces `[]*Resource`, the reconciler diffs it against state, the providers receive it, and the graph builder scans it for references.

Without a shared types package, every subsystem would define its own resource struct, leading to implicit conversions and tight coupling between parser and consumer.

## 2. Responsibilities

- Define `Resource`: holds `id`, `type`, and `properties` for one infrastructure object
- Define `Config`: top-level wrapper that holds a slice of `*Resource`
- Provide YAML and JSON struct tags so the same types round-trip through both formats
- Keep the package dependency-free (no imports beyond the standard library)

## 3. Non-Responsibilities

- No parsing or validation logic (that belongs in `pkg/config/parser.go`)
- No I/O
- No business logic

## 4. Architecture Design

```
main.yaml
    |
    v
pkg/config/parser.go  →  *Config{ Resources: []*Resource }
                                        |
                    +-------------------+-------------------+
                    |                   |                   |
            pkg/graph           pkg/reconciler      pkg/provider
            (scan refs)         (diff, interpolate) (create/update/delete)
```

`Resource` and `Config` sit at the bottom of the import graph. No other package they import should import them back.

## 5. Core Data Structures (Go)

```go
package config

// Resource represents a single desired infrastructure object.
// It maps directly to one YAML block under `resources:`.
type Resource struct {
    ID         string                 `yaml:"id"         json:"id"`
    Type       string                 `yaml:"type"       json:"type"`
    Properties map[string]interface{} `yaml:"properties" json:"properties"`
}

// Config is the top-level structure parsed from a YAML configuration file.
type Config struct {
    Resources []*Resource `yaml:"resources" json:"resources"`
}
```

`Properties` is `map[string]interface{}` because property shapes vary by resource type and are validated separately by the provider schema system. Using `interface{}` allows the YAML decoder to produce its native types (`string`, `float64`, `bool`, `map`, `[]interface{}`), which the diff engine and interpolation resolver handle explicitly.

## 6. Public Interfaces

No functions — only type definitions. The package surface is:

```go
type Resource struct { ... }
type Config   struct { ... }
```

## 7. Internal Algorithms

Not applicable. No logic.

## 8. Persistence Model

`Resource` and `Config` are in-memory only. The parser produces them from YAML; the state manager stores `ResourceState` (a separate type in `pkg/state`) to disk. These types are never written to disk directly.

## 9. Concurrency Model

Both types are passed by pointer but treated as read-only after construction. No locks needed. The reconciler creates a new `*Resource` when resolving interpolation references rather than mutating the original.

## 10. Configuration

Not applicable.

## 11. Observability

Not applicable.

## 12. Testing Strategy

No dedicated unit tests for this package — the types are implicitly tested by `pkg/config/parser_test.go`.

If standalone tests are added later:
- `TestResourceYAMLRoundTrip`: marshal to YAML, unmarshal back, assert equality
- `TestResourceJSONRoundTrip`: same for JSON

## 13. Open Questions

None.
