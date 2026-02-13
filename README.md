# GoIaC — A Minimal Infrastructure-as-Code Engine

A lightweight, declarative IaC framework written in Go. GoIaC lets you define infrastructure resources in YAML, automatically resolves dependencies between them, and applies changes idempotently — following the same core workflow as tools like Terraform.

---

## Features

- **Declarative YAML configuration** — describe your desired infrastructure state, not the steps to get there
- **State management** — tracks all managed resources in a local JSON state file with SHA-256 integrity verification
- **Dependency resolution (DAG)** — automatically detects inter-resource references and determines the correct execution order using topological sort
- **Idempotent operations** — only creates, updates, or deletes resources that have actually changed
- **Property interpolation** — reference outputs of one resource in another with `${type.resource_id.attribute}` syntax
- **Input validation** — validates resource properties against known schemas before any changes are made
- **File-based locking** — prevents concurrent modifications with stale-lock detection and exponential backoff
- **Structured logging** — built on Go's `slog` package with configurable log levels and optional JSON output
- **Graceful shutdown** — catches `SIGINT`/`SIGTERM` during apply and saves partial state before exiting
- **CI/CD friendly** — `--auto-approve` flag skips interactive prompts for automated pipelines
- **State versioning** — built-in migration framework for evolving the state format across releases
- **Error recovery** — partial state is persisted on failure so you never lose track of created resources

## Built-in Providers

| Resource Type | Description | Required Properties | Optional Properties |
|---|---|---|---|
| `local_file` | Manages a file on the local filesystem | `path`, `content` | — |
| `docker_container` | Manages a Docker container | `image` | `port`, `network_id` |
| `docker_network` | Manages a Docker network | `name` | `driver` |

---

## Installation

```bash
git clone https://github.com/Khanh-21522203/GoIaC.git
cd GoIaC
go build -o goiac ./cmd
```

## Quick Start

```bash
# 1. Initialize a new project (creates .goiac/ directory and example main.yaml)
./goiac init

# 2. Edit main.yaml to define your infrastructure

# 3. Preview what will change
./goiac plan

# 4. Apply changes
./goiac apply

# 5. Inspect current state
./goiac state show

# 6. Tear everything down
./goiac destroy
```

---

## Usage Guide

### Commands

| Command | Description |
|---|---|
| `goiac init` | Create `.goiac/` directory and a starter `main.yaml` |
| `goiac plan [config]` | Parse config (default: `main.yaml`), diff against state, and display an execution plan |
| `goiac apply [config] [--auto-approve]` | Apply the plan. Prompts for confirmation unless `--auto-approve` is set |
| `goiac destroy [--auto-approve]` | Delete all managed resources in reverse dependency order |
| `goiac state show [resource-id]` | Display the full state, or a single resource if an ID is given |
| `goiac help` | Print usage information |

### Global Flags

| Flag | Description |
|---|---|
| `--log-level <level>` | Set log verbosity: `debug`, `info` (default), `warn`, `error` |
| `--log-json` | Emit logs as JSON (useful for log aggregation) |

Global flags are placed **before** the command:

```bash
goiac --log-level debug apply
goiac --log-json --log-level warn plan
```

### Interactive Mode

Run `goiac` with no arguments to enter an interactive REPL:

```
$ ./goiac
GoIaC - A Minimal Infrastructure-as-Code Engine
Interactive mode - Type a command or 'exit' to quit

goiac> init
goiac> plan
goiac> apply
goiac> exit
```

---

## Configuration Format

Resources are defined in a YAML file (default: `main.yaml`):

```yaml
resources:
  - id: <unique-resource-id>
    type: <resource-type>
    properties:
      <key>: <value>
```

### Property Interpolation

Reference attributes of other resources using `${type.resource_id.attribute}`:

```yaml
resources:
  - id: app-network
    type: docker_network
    properties:
      name: my-network
      driver: bridge

  - id: web-server
    type: docker_container
    properties:
      image: nginx:latest
      port: 8080
      network_id: ${docker_network.app-network.network_id}
```

GoIaC automatically detects these references, builds a dependency graph, and ensures `app-network` is created before `web-server`. During destruction, the order is reversed.

### Example: Local Files

```yaml
resources:
  - id: hello
    type: local_file
    properties:
      path: ./hello.txt
      content: "Hello from GoIaC!"

  - id: config
    type: local_file
    properties:
      path: ./config.txt
      content: "app_name=GoIaC\nversion=1.0"
```

More examples are available in the [`example/`](example/) directory.

---

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                        CLI (cmd/)                        │
│  Parses commands & flags, runs interactive REPL          │
└──────────────┬───────────────────────────────────────────┘
               │
┌──────────────▼───────────────────────────────────────────┐
│                    CLI Commands (pkg/cli/)                │
│  init · plan · apply · destroy · state show              │
│  Acquires lock, coordinates parser → reconciler → state  │
└──────┬───────────┬──────────────┬────────────────────────┘
       │           │              │
┌──────▼──────┐ ┌──▼───────────┐ │
│ Config      │ │ Reconciler   │ │
│ (pkg/config)│ │(pkg/reconciler)│
│             │ │              │ │
│ YAML parser │ │ Plan:        │ │
│ + validation│ │  · Validate  │ │
│             │ │  · Diff      │ │
└─────────────┘ │  · Build DAG │ │
                │              │ │
                │ Apply:       │ │
                │  · Topo sort │ │
                │  · Execute   │ │
                │  · Interpolate│ │
                └──────┬───────┘ │
                       │         │
          ┌────────────▼──┐  ┌───▼──────────────┐
          │ Providers     │  │ State Manager     │
          │ (pkg/provider)│  │ (pkg/state)       │
          │               │  │                   │
          │ · Registry    │  │ · Load / Save     │
          │ · Validation  │  │ · Lock / Unlock   │
          │ · docker/     │  │ · SHA-256 checksum│
          │ · local/      │  │ · Migration       │
          └───────────────┘  └───────────────────┘
```

### Core Packages

| Package | Responsibility |
|---|---|
| **`cmd/`** | Entry point. Parses CLI arguments and global flags, dispatches to commands or starts the interactive REPL. |
| **`pkg/cli/`** | Implements each command (`init`, `plan`, `apply`, `destroy`, `state show`). Manages the lock lifecycle and coordinates between parser, reconciler, and state manager. |
| **`pkg/config/`** | Reads and validates YAML configuration files. Ensures every resource has a unique ID and a type. |
| **`pkg/graph/`** | Builds a directed acyclic graph (DAG) from resource references. Provides topological sort (for creation order) and reverse topological sort (for destruction order). Detects circular dependencies and undefined references. |
| **`pkg/reconciler/`** | The core engine. Computes a diff between desired config and current state, resolves `${...}` interpolation expressions, and executes create/update/delete operations in dependency order. Saves partial state on failure. |
| **`pkg/provider/`** | Defines the `Provider` interface (CRUD) and a thread-safe registry. Includes schema-based property validation. |
| **`pkg/provider/docker/`** | Docker container and network providers using the Docker Engine API. |
| **`pkg/provider/local/`** | Local file provider for managing files on disk. |
| **`pkg/logger/`** | Thin wrapper around Go's `slog` package. Supports text/JSON output and runtime level changes. |
| **`pkg/state/`** | Manages the state file (`.goiac/state.json`). Handles atomic writes (write-to-tmp + rename), SHA-256 checksums, file-based locking with stale-lock detection, and state version migration. |

### Lifecycle of `goiac apply`

```
1. Acquire lock            (.goiac/state.lock)
2. Parse YAML              (main.yaml → []*Resource)
3. Validate properties     (check required/optional per type)
4. Load current state      (.goiac/state.json)
5. Compute diff            (desired vs actual → Create/Update/Delete/Noop)
6. Build dependency graph  (extract ${...} references → DAG)
7. Validate DAG            (no cycles, no undefined references)
8. Topological sort        (dependency-first order)
9. Execute changes         (call Provider.Create/Update/Delete in order)
10. Save state             (atomic write + checksum)
11. Release lock
```

If step 9 fails partway through, the state saved so far is persisted (step 10 runs in the error path) so that the next `apply` can pick up where it left off.

### State File

State is stored in `.goiac/state.json` alongside a `.goiac/state.json.sha256` checksum file:

```json
{
  "version": 1,
  "last_updated": "2026-02-13T16:00:00+07:00",
  "resources": {
    "web-server": {
      "id": "abc123...",
      "type": "docker_container",
      "attributes": {
        "container_id": "abc123...",
        "image": "nginx:latest",
        "status": "running",
        "port": 8080
      }
    }
  }
}
```

### Provider Interface

Every provider implements four methods:

```go
type Provider interface {
    Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error)
    Read(ctx context.Context, resourceID string) (*state.ResourceState, error)
    Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error)
    Delete(ctx context.Context, resourceID string) error
}
```

- **Create** returns the new resource state (including the provider-assigned ID).
- **Read** returns `(nil, nil)` if the resource no longer exists — this is not an error.
- **Update** is implemented as delete + create for Docker resources.
- **Delete** must be idempotent (safe to call on an already-deleted resource).

---

## Running Tests

```bash
go test ./...
```

Test coverage includes:
- **`pkg/config/`** — YAML parsing, validation (missing ID, missing type, duplicates, empty config)
- **`pkg/graph/`** — DAG building, cycle detection, reference validation, topological sort, reference extraction
- **`pkg/reconciler/`** — Diff computation (create/update/delete/noop/mixed), numeric type normalization, interpolation (simple, nested, lists, unresolved, immutability)
- **`pkg/provider/`** — Schema validation (required, optional, unknown properties, unknown types)
- **`pkg/provider/local/`** — Full CRUD lifecycle, error cases, idempotent delete
- **`pkg/state/`** — Save/load round-trip, empty state, locking, `WithLock`, state migration (current version, zero version, future version, invalid JSON)

---

## Project Structure

```
GoIaC/
├── cmd/
│   └── main.go                  # Entry point, CLI argument parsing
├── pkg/
│   ├── cli/
│   │   ├── cli.go               # CLI struct, provider registration
│   │   ├── init.go              # goiac init
│   │   ├── plan.go              # goiac plan
│   │   ├── apply.go             # goiac apply (with timeout & signal handling)
│   │   ├── destroy.go           # goiac destroy (reverse dependency order)
│   │   └── state.go             # goiac state show
│   ├── config/
│   │   ├── types.go             # Resource, Config structs
│   │   ├── parser.go            # YAML parser + validation
│   │   └── parser_test.go
│   ├── graph/
│   │   ├── graph.go             # DAG builder, cycle & reference validation
│   │   ├── toposort.go          # Kahn's algorithm (normal + reverse)
│   │   └── graph_test.go
│   ├── logger/
│   │   └── logger.go            # slog wrapper (text/JSON, level config)
│   ├── provider/
│   │   ├── interface.go         # Provider CRUD interface
│   │   ├── registry.go          # Thread-safe provider registry
│   │   ├── validation.go        # Property schema validation
│   │   ├── validation_test.go
│   │   ├── docker/
│   │   │   ├── container.go     # Docker container provider
│   │   │   └── network.go       # Docker network provider
│   │   └── local/
│   │       ├── file.go          # Local file provider
│   │       └── file_test.go
│   ├── reconciler/
│   │   ├── reconciler.go        # Plan & Apply orchestration
│   │   ├── diff.go              # Desired vs actual diff engine
│   │   ├── diff_test.go
│   │   ├── executor.go          # Change execution with context support
│   │   ├── interpolate.go       # ${...} reference resolution
│   │   └── interpolate_test.go
│   └── state/
│       ├── types.go             # State, ResourceState, LockInfo structs
│       ├── manager.go           # Load/Save with atomic writes & checksums
│       ├── manager_test.go
│       ├── lock.go              # File locking with backoff & stale detection
│       ├── migration.go         # State version migration framework
│       └── migration_test.go
├── example/
│   ├── local-file/main.yaml     # Local file example
│   └── docker-app/main.yaml     # Docker container + network example
├── go.mod
└── go.sum
```
