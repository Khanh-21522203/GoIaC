# GoIaC - A Minimal Infrastructure-as-Code Engine

A lightweight IaC framework in Go demonstrating core concepts of declarative infrastructure management.

## Features

- ✅ Declarative YAML configuration
- ✅ State management with locking
- ✅ Dependency resolution (DAG)
- ✅ Parallel execution
- ✅ Idempotent operations
- ✅ Built-in providers (Docker, local files)

## Installation

```bash
# Clone repository
git clone https://github.com/Khanh-21522203/GoIaC.git
cd GoIaC

# Build
go build -o GoIaC ./cmd
```

## Quick Start

```bash
# Initialize project
GoIaC init

# Edit main.yaml to define your infrastructure

# Preview changes
GoIaC plan

# Apply changes
GoIaC apply

# View state
GoIaC state show

# Destroy all resources
GoIaC destroy
```

## Example Configuration

```yaml
resources:
  - id: app-network
    type: docker_network
    properties:
      name: app-network
      driver: bridge
  
  - id: web-server
    type: docker_container
    properties:
      image: nginx:latest
      port: 8080
      network_id: ${docker_network.app-network.id}
```

## Project Structure
```
GoIaC/
├── cmd/                # CLI entry point
├── pkg/
│   ├── config/         # Configuration parsing
│   ├── state/          # State management
│   ├── graph/          # Dependency resolution
│   ├── provider/       # Provider interface & implementations
│   ├── reconciler/     # Reconciliation engine
│   └── cli/            # CLI commands
├── examples/           # Example configurations
└── test/               # Integration tests
```