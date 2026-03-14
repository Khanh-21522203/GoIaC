# Feature: CLI Commands

## 1. Purpose

The CLI Commands layer is the user-facing entry point for all GoIaC operations. It parses arguments and flags, wires together the subsystems (parser, reconciler, state manager, registry), and presents human-readable output. It also implements the interactive REPL for when no command is given.

## 2. Responsibilities

**`cmd/main.go`** (entry point):
- Parse global flags: `--log-level`, `--log-json`
- Configure the logger before dispatching
- Dispatch to `init`, `plan`, `apply`, `destroy`, `state`, or `help`
- If no command: start the interactive REPL

**`pkg/cli/cli.go`** (wiring):
- Construct `CLI` struct with stateManager, parser, registry, reconciler
- Register all built-in providers (`local_file`, `docker_container`, `docker_network`)

**`pkg/cli/init.go`**:
- Create `.goiac/` directory
- Create starter `main.yaml` if not already present

**`pkg/cli/plan.go`**:
- Parse config → reconciler.Plan → printPlan

**`pkg/cli/apply.go`**:
- Acquire lock → parse → plan → printPlan → prompt → apply with timeout + signal handling → release lock

**`pkg/cli/destroy.go`**:
- Acquire lock → load state → build graph → print resources → prompt → delete in topological order → save state → release lock

**`pkg/cli/state.go`**:
- Load state → print one resource (if ID given) or all resources

## 3. Non-Responsibilities

- Does not implement provider logic
- Does not implement diff or graph algorithms
- Does not format logs (that is the logger package)

## 4. Architecture Design

```
cmd/main.go
  os.Args parsing (manual, no flag library needed)
  logger.SetLevel / logger.SetJSON
  cli, _ = NewCLI()
  switch command:
    "init"         → cli.Init()
    "plan"         → cli.Plan(configPath)
    "apply"        → cli.Apply(configPath, autoApprove)
    "destroy"      → cli.Destroy(autoApprove)
    "state show"   → cli.StateShow(resourceID)
    "help"         → printHelp()
    ""             → startREPL(cli)

pkg/cli/cli.go
  NewCLI():
    stateManager = state.NewManager()
    parser       = config.NewParser()
    registry     = provider.NewRegistry()
    registerProviders(registry)   // registers local_file, docker_container, docker_network
    reconciler   = reconciler.NewReconciler(stateManager, registry)
```

## 5. Core Data Structures (Go)

```go
package cli

import (
    "GoIaC/pkg/config"
    "GoIaC/pkg/provider"
    "GoIaC/pkg/reconciler"
    "GoIaC/pkg/state"
)

type CLI struct {
    stateManager *state.Manager
    parser       *config.Parser
    registry     *provider.Registry
    reconciler   *reconciler.Reconciler
}
```

## 6. Public Interfaces

```go
// Construction
func NewCLI() (*CLI, error)

// Commands
func (c *CLI) Init() error
func (c *CLI) Plan(configPath string) error
func (c *CLI) Apply(configPath string, autoApprove bool) error
func (c *CLI) Destroy(autoApprove bool) error
func (c *CLI) StateShow(resourceID string) error
```

## 7. Internal Algorithms

### `Apply` lifecycle (most complex command)
```
1. stateManager.Lock()
   defer stateManager.Unlock()
2. parser.Parse(configPath) → desired
3. reconciler.Plan(desired) → changes
4. printPlan(changes)
5. if !autoApprove:
       prompt "Do you want to apply these changes? (yes/no):"
       fmt.Scanln(&response)
       if response != "yes": return nil (cancelled)
6. ctx, cancel = context.WithTimeout(background, 10*time.Minute)
   defer cancel()
7. sigChan = make(chan os.Signal, 1)
   signal.Notify(sigChan, SIGINT, SIGTERM)
   go func() { <-sigChan; cancel() }()
   defer signal.Stop(sigChan)
8. reconciler.Apply(ctx, desired)
9. print "Infrastructure updated successfully!"
```

### `Destroy` lifecycle
```
1. stateManager.Lock()
   defer stateManager.Unlock()
2. state = stateManager.Load()
3. if no resources: print "No resources to destroy." return nil
4. build []*config.Resource from state (ID=config id, Properties=state.Attributes)
5. graph.Build → TopologicalSort() → order (dependent-first)
6. print resources to destroy
7. if !autoApprove: prompt confirmation
8. ctx = context.Background()
9. for each id in order:
       prov = registry.Get(res.Type)
       prov.Delete(ctx, res.ID)
       print "Destroyed <id>"
       delete(state.Resources, id)
10. stateManager.Save(state)   // always save, even on partial failure
11. if destroyErrors: return combined error
```

### `printPlan` output format
```
=== Execution Plan ===
  + Create: <id> (<type>)
  ~ Update: <id>
      Reason: [field changed, ...]
  - Delete: <id>

Summary: N to create, N to update, N to delete, N unchanged
```

### Interactive REPL
```
print "GoIaC - A Minimal Infrastructure-as-Code Engine"
print "Interactive mode - Type a command or 'exit' to quit"
for:
    print "goiac> "
    read line from stdin
    switch trim(line):
        "init":    cli.Init()
        "plan":    cli.Plan("main.yaml")
        "apply":   cli.Apply("main.yaml", false)
        "destroy": cli.Destroy(false)
        "exit":    return
        "":        continue
        default:   print "Unknown command: <line>"
    print any error
```

### `init` command
```
os.MkdirAll(".goiac", 0755)
if "main.yaml" does not exist:
    write starter YAML:
        resources:
          - id: example
            type: local_file
            properties:
              path: ./example.txt
              content: "Hello from GoIaC!"
print instructions
```

### Global flags (`cmd/main.go`)
```
--log-level <level>  → logger.SetLevel(parseLevel(level))
--log-json           → flag to switch to JSON handler
```
Both flags are parsed before the command is dispatched. If `--log-json` is set, call `logger.SetJSON(level)` instead of `logger.SetLevel(level)`.

## 8. Persistence Model

The CLI layer does not directly persist anything. It delegates persistence to `stateManager.Save()` and `stateManager.Lock()/Unlock()`.

## 9. Concurrency Model

Each CLI command runs in a single goroutine. The signal handler in `Apply` is a separate goroutine that calls `cancel()` on the context — the only concurrent code in the CLI layer.

## 10. Configuration

| Global flag | Default | Description |
|---|---|---|
| `--log-level` | `info` | Log verbosity |
| `--log-json` | false | JSON log output |

| Command flag | Command | Default | Description |
|---|---|---|---|
| `[config]` | `plan`, `apply` | `main.yaml` | Config file path |
| `--auto-approve` | `apply`, `destroy` | false | Skip confirmation prompt |

## 11. Observability

The CLI prints user-facing output to `stdout` (`fmt.Println`/`fmt.Printf`) and routes structured logs to `stderr` via the logger.

Key user messages:
- `=== Execution Plan ===` with per-resource lines and summary
- `Applying changes...` / `Infrastructure updated successfully!`
- `=== Resources to Destroy ===` with per-resource lines
- `Destroyed <id>` for each resource
- `Apply cancelled.` / `Destroy cancelled.`
- Error messages from `fmt.Fprintf(os.Stderr, "Error: %v\n", err)`

## 12. Testing Strategy

CLI commands are primarily tested via integration tests or end-to-end tests because they combine I/O, prompts, and real providers.

Unit-testable parts:
- `printPlan`: assert output format for Create/Update/Delete/Noop changes
- `registerProviders`: assert all three provider types registered (mock registry)

End-to-end tests (require Docker + temp dir):
- `TestInitCreatesDirectory`: run `Init`, assert `.goiac/` and `main.yaml` exist
- `TestPlanNoChanges`: init + apply then plan again → 0 changes
- `TestApplyAutoApprove`: `Apply("fixtures/local.yaml", true)` → file created
- `TestDestroyAutoApprove`: apply then destroy → state empty, file deleted

## 13. Open Questions

- Should the REPL support `plan <file>` and `apply <file>` with custom config paths?
- Should `destroy` also support a 10-minute timeout + signal handling, like `apply`?
- Should errors from individual resource deletions in `destroy` stop the loop or collect all errors and continue?
