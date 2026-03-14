# Feature: Config Parser

## 1. Purpose

The Config Parser reads a YAML file from disk and produces a validated `[]*config.Resource` slice. It is the first stage in every GoIaC command — before any diff, plan, or execution can happen, the desired state must be parsed from a file.

It also performs structural validation (unique IDs, non-empty types) that is independent of any provider schema. Provider-level property validation is handled separately by `pkg/provider/validation.go`.

## 2. Responsibilities

- Open and decode a YAML file into a `*config.Config` struct
- Return a meaningful error if the file does not exist or is malformed YAML
- Validate that every resource has a non-empty `id`
- Validate that every resource has a non-empty `type`
- Validate that no two resources share the same `id`
- Return a non-nil empty slice (not nil) if the config has zero resources
- Default the config file path to `main.yaml` if none is provided

## 3. Non-Responsibilities

- Does not validate provider-specific properties (that is `pkg/provider/validation.go`)
- Does not resolve `${...}` interpolation expressions (that is `pkg/reconciler/interpolate.go`)
- Does not touch the state file or lock

## 4. Architecture Design

```
disk: main.yaml
       |
       v
config.Parser.Parse(path string) ([]*config.Resource, error)
       |
       +-- os.ReadFile
       +-- yaml.Unmarshal → *config.Config
       +-- structuralValidate(config)
              |-- each resource has ID
              |-- each resource has Type
              +-- no duplicate IDs
       |
       v
[]*config.Resource   →   passed to reconciler.Plan / reconciler.Apply
```

## 5. Core Data Structures (Go)

```go
package config

import (
    "fmt"
    "os"
    "gopkg.in/yaml.v3"
)

type Parser struct{}

func NewParser() *Parser {
    return &Parser{}
}

// Parse reads the YAML file at path and returns the validated resource list.
// If path is empty, defaults to "main.yaml".
func (p *Parser) Parse(path string) ([]*Resource, error) {
    if path == "" {
        path = "main.yaml"
    }

    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("failed to parse YAML: %w", err)
    }

    if err := validateConfig(&cfg); err != nil {
        return nil, err
    }

    if cfg.Resources == nil {
        return []*Resource{}, nil
    }
    return cfg.Resources, nil
}

func validateConfig(cfg *Config) error {
    seen := make(map[string]bool)
    for _, r := range cfg.Resources {
        if r.ID == "" {
            return fmt.Errorf("resource missing required field 'id'")
        }
        if r.Type == "" {
            return fmt.Errorf("resource %q missing required field 'type'", r.ID)
        }
        if seen[r.ID] {
            return fmt.Errorf("duplicate resource id: %q", r.ID)
        }
        seen[r.ID] = true
    }
    return nil
}
```

## 6. Public Interfaces

```go
func NewParser() *Parser
func (p *Parser) Parse(path string) ([]*config.Resource, error)
```

## 7. Internal Algorithms

### Parse Flow
```
1. Resolve path (default "main.yaml" if empty)
2. os.ReadFile(path) → []byte
3. yaml.Unmarshal(data, &cfg) → *Config
4. validateConfig:
     a. for each resource: assert ID != ""
     b. for each resource: assert Type != ""
     c. for each resource: assert ID not already seen
5. Return cfg.Resources (or empty slice if nil)
```

Time complexity: O(n) where n = number of resources.

### YAML Type Mapping
The YAML library maps:
- String values → `string`
- Integer literals → `int` (not `float64` — unlike JSON)
- Float literals → `float64`
- Boolean literals → `bool`
- Nested mappings → `map[string]interface{}`
- Sequences → `[]interface{}`

This is important: the diff engine's `normalizeValue` normalizes `int` → `float64` to match JSON-decoded state attributes. See `docs/architecture.md`.

## 8. Persistence Model

Read-only. No writes to disk.

## 9. Concurrency Model

`Parser` has no state. All methods are safe for concurrent use.

## 10. Configuration

The only configuration is the config file path, passed as an argument to `Parse`. The CLI defaults it to `"main.yaml"`.

## 11. Observability

No metrics. The reconciler logs `"loading config"` before calling `Parse`.

## 12. Testing Strategy

**Unit tests** (table-driven, in `pkg/config/parser_test.go`):

- `TestParseValidConfig`: valid YAML with multiple resources → assert correct slice
- `TestParseMissingID`: resource without `id` field → assert error contains `"missing"` and `"id"`
- `TestParseMissingType`: resource without `type` field → assert error
- `TestParseDuplicateID`: two resources with same `id` → assert error contains `"duplicate"`
- `TestParseEmptyConfig`: YAML with no resources → assert empty (non-nil) slice
- `TestParseFileNotFound`: non-existent path → assert error wraps `os.ErrNotExist`
- `TestParseInvalidYAML`: malformed YAML → assert parse error
- `TestParseDefaultPath`: empty path argument → attempts `"main.yaml"`

All tests use `t.TempDir()` and write temporary YAML files; no test depends on real files.

## 13. Open Questions

None.
