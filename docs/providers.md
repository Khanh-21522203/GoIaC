# Writing a Custom Provider

GoIaC's provider system is a small interface. Adding support for a new resource type means implementing four methods and registering the provider — nothing else.

---

## The Provider Interface

```go
// pkg/provider/interface.go
type Provider interface {
    Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error)
    Read(ctx context.Context, resourceID string)         (*state.ResourceState, error)
    Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error)
    Delete(ctx context.Context, resourceID string)       error
}
```

### Contract

| Method | Input | Must return | Notes |
|---|---|---|---|
| `Create` | Desired config resource | New `*ResourceState` | `ResourceState.ID` must be the provider-assigned ID. |
| `Read` | Provider-assigned resource ID | `*ResourceState` or `(nil, nil)` | Return `nil, nil` if the resource no longer exists — **not** an error. |
| `Update` | Desired config + provider ID | Updated `*ResourceState` | May be implemented as delete + create if in-place update is not possible. |
| `Delete` | Provider-assigned resource ID | `error` | Must be idempotent — safe to call on an already-deleted resource. |

The `resourceID` passed to `Read`, `Update`, and `Delete` is `ResourceState.ID` from the state file — the ID your `Create` (or a previous `Update`) returned, not the user-defined config ID.

---

## Step-by-Step Example

This example adds a minimal `local_dir` provider that manages directories.

### 1. Create the package

```
pkg/provider/local/dir.go
```

```go
package local

import (
    "GoIaC/pkg/config"
    "GoIaC/pkg/state"
    "context"
    "fmt"
    "os"
)

type DirProvider struct{}

func NewDirProvider() *DirProvider { return &DirProvider{} }

func (p *DirProvider) Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error) {
    path, ok := desired.Properties["path"].(string)
    if !ok {
        return nil, fmt.Errorf("path property required")
    }
    if err := os.MkdirAll(path, 0755); err != nil {
        return nil, fmt.Errorf("failed to create directory: %w", err)
    }
    return &state.ResourceState{
        ID:   path,
        Type: desired.Type,
        Attributes: map[string]interface{}{"path": path},
    }, nil
}

func (p *DirProvider) Read(ctx context.Context, resourceID string) (*state.ResourceState, error) {
    if _, err := os.Stat(resourceID); err != nil {
        if os.IsNotExist(err) {
            return nil, nil // not an error
        }
        return nil, err
    }
    return &state.ResourceState{
        ID:         resourceID,
        Type:       "local_dir",
        Attributes: map[string]interface{}{"path": resourceID},
    }, nil
}

func (p *DirProvider) Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error) {
    // path cannot change in-place; treat as delete + create
    if err := p.Delete(ctx, resourceID); err != nil {
        return nil, err
    }
    return p.Create(ctx, desired)
}

func (p *DirProvider) Delete(ctx context.Context, resourceID string) error {
    err := os.RemoveAll(resourceID)
    if err != nil && !os.IsNotExist(err) {
        return err
    }
    return nil
}
```

### 2. Register the provider

Open `pkg/cli/cli.go` and add one line inside `registerProviders`:

```go
registry.Register("local_dir", local.NewDirProvider())
```

### 3. Add a schema (required)

Open `pkg/provider/validation.go` and add your resource type to `knownSchemas`:

```go
// PropertySchema defines required and optional property names for a resource type.
// Any property not in either list is rejected at validation time.
type PropertySchema struct {
    Required []string
    Optional []string
}

var knownSchemas = map[string]PropertySchema{
    // existing entries ...
    "local_dir": {
        Required: []string{"path"},
        Optional: []string{},
    },
}
```

`ValidateResources` is called before any changes are applied. It iterates all resources, checks:
1. The resource type exists in `knownSchemas` (unknown type → error).
2. All `Required` keys are present in `resource.Properties` (missing → error).
3. No key in `resource.Properties` is outside `Required ∪ Optional` (unknown property → error).

If your schema entry is missing, the resource type will be rejected as "unknown" even though the provider is registered.

### 4. Use it in config

```yaml
resources:
  - id: uploads
    type: local_dir
    properties:
      path: ./storage/uploads
```

---

## ResourceState.Attributes

Whatever you put in `Attributes` becomes available to `${type.id.attr}` interpolation in other resources. Choose attribute names carefully — they are part of your provider's public API.

Convention used by the built-in providers:

- Use snake_case keys.
- Include the provider-assigned ID as an attribute (e.g. `container_id`, `network_id`) so other resources can reference it.
- Mirror the config input properties as attributes so `plan` output is self-contained.

---

## Error Handling Guidelines

- Wrap errors with `fmt.Errorf("context: %w", err)` so callers can add more context.
- If an API call returns a "not found" error, translate it to `return nil, nil` in `Read` and a no-op success in `Delete`.
- Do not return partial state on error — if `Create` fails, return `nil, err`. GoIaC will not update state for that resource.
- Use `context.Context` for cancellation. Check `ctx.Err()` around long-running API calls.

---

## Testing Your Provider

Follow the pattern in `pkg/provider/local/file_test.go`:

1. Create a real resource (no mocks).
2. Assert `Read` returns the expected attributes.
3. `Update` and assert attributes changed.
4. `Delete` and assert `Read` returns `(nil, nil)`.
5. Call `Delete` again and assert no error (idempotency).

Integration tests against real infrastructure are preferred over mocks to avoid masking provider-level bugs.
