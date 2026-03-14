# Feature: Provider System

## 1. Purpose

The Provider System defines the contract between the GoIaC engine and external infrastructure APIs. It provides the `Provider` interface that all resource implementations must satisfy, a thread-safe registry for looking up providers by resource type, and a schema-based validation layer that rejects unknown resource types and invalid properties before any real infrastructure is touched.

## 2. Responsibilities

- Define the `Provider` CRUD interface
- Implement a thread-safe `Registry` for registering and looking up providers by type string
- Define `PropertySchema` (required/optional property lists) for each built-in resource type
- Validate a `[]*config.Resource` slice against the schema before apply (fail fast, no side effects)
- Report clear errors: unknown type, missing required property, unknown property

## 3. Non-Responsibilities

- Does not implement any specific resource (that is `pkg/provider/docker/` and `pkg/provider/local/`)
- Does not manage the execution order of provider calls (that is the reconciler)
- Does not persist anything

## 4. Architecture Design

```
pkg/provider/
├── interface.go    Provider interface (Create, Read, Update, Delete)
├── registry.go     Registry (Register, Get, List)
└── validation.go   PropertySchema, knownSchemas, ValidateResource, ValidateResources
```

```
CLI.registerProviders()
  registry.Register("local_file",       local.NewFileProvider())
  registry.Register("docker_container", docker.NewContainerProvider())
  registry.Register("docker_network",   docker.NewNetworkProvider())

reconciler.Plan / reconciler.Apply
  provider.ValidateResources(desired)   ← fails fast before any I/O
        |
        v
  registry.Get(resource.Type) → Provider
  provider.Create / Update / Delete
```

The `knownSchemas` map in `validation.go` and the `registry` in `cli.go` are **two separate registrations** that must be kept in sync: when adding a new provider, both must be updated.

## 5. Core Data Structures (Go)

```go
package provider

import (
    "GoIaC/pkg/config"
    "GoIaC/pkg/state"
    "context"
    "fmt"
    "sync"
)

// Provider is the interface every resource implementation must satisfy.
type Provider interface {
    Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error)
    Read(ctx context.Context, resourceID string)         (*state.ResourceState, error)
    Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error)
    Delete(ctx context.Context, resourceID string)       error
}

// Registry is a thread-safe map from resource type string to Provider.
type Registry struct {
    providers map[string]Provider
    mu        sync.RWMutex
}

func NewRegistry() *Registry {
    return &Registry{providers: make(map[string]Provider)}
}

// PropertySchema defines the expected property names for a resource type.
// Properties not in Required or Optional are rejected at validation time.
type PropertySchema struct {
    Required []string
    Optional []string
}

// knownSchemas is the authoritative list of built-in resource types.
// It must be updated whenever a new provider is added.
var knownSchemas = map[string]PropertySchema{
    "local_file": {
        Required: []string{"path", "content"},
        Optional: []string{},
    },
    "docker_container": {
        Required: []string{"image"},
        Optional: []string{"port", "network_id"},
    },
    "docker_network": {
        Required: []string{"name"},
        Optional: []string{"driver"},
    },
}
```

## 6. Public Interfaces

```go
// Provider interface
type Provider interface {
    Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error)
    Read(ctx context.Context, resourceID string)         (*state.ResourceState, error)
    Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error)
    Delete(ctx context.Context, resourceID string)       error
}

// Registry
func NewRegistry() *Registry
func (r *Registry) Register(resourceType string, provider Provider)
func (r *Registry) Get(resourceType string) (Provider, error)
func (r *Registry) List() []string

// Validation
func ValidateResource(resource *config.Resource) error
func ValidateResources(resources []*config.Resource) error
```

## 7. Internal Algorithms

### Registry.Get
```
r.mu.RLock()
defer r.mu.RUnlock()
prov, ok = r.providers[resourceType]
if !ok: return nil, error("provider not found for resource type: <type>")
return prov, nil
```

### ValidateResource
```
schema, ok = knownSchemas[resource.Type]
if !ok: return error("unknown resource type: <type>")

for each required in schema.Required:
    if _, exists = resource.Properties[required]; !exists:
        return error("resource <id> (<type>): missing required property <required>")

allowed = set(schema.Required) ∪ set(schema.Optional)
for each key in resource.Properties:
    if key not in allowed:
        return error("resource <id> (<type>): unknown property <key>")
```

### Provider Interface Contract

| Method | `resourceID` arg | Must return on "not found" | Idempotency requirement |
|---|---|---|---|
| `Create` | — | N/A | N/A |
| `Read` | Provider-assigned ID | `(nil, nil)` — not an error | Safe to call any time |
| `Update` | Provider-assigned ID | Return updated state | N/A |
| `Delete` | Provider-assigned ID | `nil` error | Safe if already deleted |

The `resourceID` passed to `Read`, `Update`, `Delete` is `ResourceState.ID` from state — the provider-assigned identifier (e.g. Docker container SHA, file path), not the user's config `id`.

## 8. Persistence Model

No persistence in this package. The registry is rebuilt from `registerProviders()` on every program start.

## 9. Concurrency Model

`Registry` uses `sync.RWMutex`: read lock for `Get`/`List`, write lock for `Register`. In practice, `Register` is called once at startup before any goroutines use the registry, so the lock is mostly for correctness guarantees.

`PropertySchema` and `knownSchemas` are read-only after program init.

## 10. Configuration

No configuration. All schemas are hardcoded in `validation.go`. Provider registration is hardcoded in `pkg/cli/cli.go → registerProviders()`.

## 11. Observability

No metrics or logging. Validation errors include resource ID and type for easy user diagnosis.

## 12. Testing Strategy

**Unit tests** (in `pkg/provider/validation_test.go`):

- `TestValidateUnknownType`: resource with `type: "nonexistent"` → error "unknown resource type"
- `TestValidateMissingRequired`: `local_file` missing `path` → error "missing required property"
- `TestValidateUnknownProperty`: `docker_container` with extra key → error "unknown property"
- `TestValidateValid`: fully valid resource for each known type → no error
- `TestValidateResources`: mixed list with one invalid → returns first error

**Registry tests** (can be added to `validation_test.go` or separate):
- `TestRegistryGetMissing`: `Get("nonexistent")` → error
- `TestRegistryRegisterGet`: register a mock provider, `Get` returns it
- `TestRegistryList`: register two providers, `List` returns both types

## 13. Open Questions

- Should `knownSchemas` be populated dynamically (e.g., providers register their own schemas via `RegisterSchema`) to keep validation and implementation co-located? Currently they must be updated in two separate files.
