# Feature: Local File Provider

## 1. Purpose

The Local File Provider manages plain files on the local filesystem. It is the simplest built-in provider and serves as the reference implementation for the `Provider` interface. Every team can use it without Docker or any external service — making it ideal for bootstrapping and testing.

## 2. Responsibilities

- `Create`: write a file to disk with mode `0644`; return path, content, size, and mode as attributes
- `Read`: stat + read the file; return `(nil, nil)` if the file does not exist
- `Update`: overwrite the file with new content; return refreshed attributes
- `Delete`: remove the file; return no error if already absent (idempotent)

## 3. Non-Responsibilities

- Does not manage directories (that is a future `local_dir` provider)
- Does not create parent directories — the path's parent must already exist
- Does not set file permissions other than `0644`
- Does not track file ownership

## 4. Architecture Design

```
pkg/provider/local/
└── file.go     FileProvider (Create, Read, Update, Delete)

ResourceState.ID = file path on disk
ResourceState.Attributes = {path, content, size, mode}
```

The provider-assigned ID is the file path itself (identical to the `path` property). This makes `Read`/`Update`/`Delete` trivial: the `resourceID` argument is a file path.

## 5. Core Data Structures (Go)

```go
package local

import (
    "GoIaC/pkg/config"
    "GoIaC/pkg/state"
    "context"
    "fmt"
    "os"
)

type FileProvider struct{}

func NewFileProvider() *FileProvider {
    return &FileProvider{}
}
```

### Schema (in `pkg/provider/validation.go`)
```go
"local_file": {
    Required: []string{"path", "content"},
    Optional: []string{},
}
```

### Attributes returned

| Attribute | Type | Source |
|---|---|---|
| `path` | string | input `path` property |
| `content` | string | input `content` property |
| `size` | int64 | `os.Stat.Size()` |
| `mode` | string | `os.Stat.Mode().String()` e.g. `-rw-r--r--` |

## 6. Public Interfaces

```go
func NewFileProvider() *FileProvider

func (p *FileProvider) Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error)
func (p *FileProvider) Read(ctx context.Context, resourceID string)         (*state.ResourceState, error)
func (p *FileProvider) Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error)
func (p *FileProvider) Delete(ctx context.Context, resourceID string)       error
```

## 7. Internal Algorithms

### Create
```
path    = desired.Properties["path"].(string)   // required, validated upstream
content = desired.Properties["content"].(string) // required, validated upstream
os.WriteFile(path, []byte(content), 0644)
info = os.Stat(path)
return ResourceState{
    ID:   path,
    Type: desired.Type,
    Attributes: {path, content, size=info.Size(), mode=info.Mode().String()},
}
```

### Read
```
info, err = os.Stat(resourceID)
if os.IsNotExist(err): return nil, nil   // not an error
content = os.ReadFile(resourceID)
return ResourceState{
    ID: resourceID,
    Type: "local_file",
    Attributes: {path, content, size, mode},
}
```

### Update
```
content = desired.Properties["content"].(string)
os.WriteFile(resourceID, []byte(content), 0644)
return p.Read(ctx, resourceID)
```

Update uses `resourceID` (the path in state) rather than `desired.Properties["path"]` to ensure the existing file is overwritten. If the path changed, the diff engine would have flagged this as an update but the new path would be written to the old location — this is a known limitation: path changes are not in-place renames.

### Delete
```
err = os.Remove(resourceID)
if os.IsNotExist(err): return nil  // idempotent
return err
```

## 8. Persistence Model

This provider's resources are the files themselves. The state file records the path, content, size, and mode so the diff engine can detect out-of-band changes (file edited externally).

## 9. Concurrency Model

`FileProvider` has no state. All methods are safe for concurrent use by the OS filesystem (concurrent writes to the same path are not protected — the reconciler serializes changes sequentially).

## 10. Configuration

No configuration. File mode is always `0644`. If configurable permissions are needed, add `mode` as an optional property in the schema.

## 11. Observability

No metrics. Errors wrap the underlying `os` error with context (operation and path).

## 12. Testing Strategy

**Integration tests** (in `pkg/provider/local/file_test.go`, use `t.TempDir()`):

- `TestFileProviderCreate`: create with valid properties → file exists on disk, attributes match
- `TestFileProviderRead`: create a file externally, call `Read` → attributes returned
- `TestFileProviderReadMissing`: `Read` on non-existent path → `(nil, nil)`
- `TestFileProviderUpdate`: create then update with new content → file content changed, attributes updated
- `TestFileProviderDelete`: create then delete → file no longer exists
- `TestFileProviderDeleteIdempotent`: delete a non-existent file → no error
- `TestFileProviderCreateMissingPath`: properties without `path` → error
- `TestFileProviderCreateMissingContent`: properties without `content` → error

No mocks. All tests use the real filesystem in a temp directory.

## 13. Open Questions

- Should `Update` detect if `path` changed and rename the file instead of overwriting the old location?
- Should file mode be configurable via an optional `mode` property?
