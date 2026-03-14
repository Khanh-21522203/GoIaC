# Feature: Docker Provider

## 1. Purpose

The Docker Provider manages Docker containers and networks on the local Docker Engine. It enables GoIaC to spin up and tear down containerized workloads declaratively — the primary demonstration use case for provider dependencies (a container depending on a network).

## 2. Responsibilities

**ContainerProvider**:
- `Create`: pull-if-absent is not done (Docker does it implicitly on `ContainerCreate`); create + start a container with optional port binding and network attachment; return `container_id`, `image`, `status`, and optionally `port`/`network_id`
- `Read`: inspect the container by ID; return `(nil, nil)` if not found
- `Update`: delete + re-create (full replacement, no in-place update)
- `Delete`: stop (10s timeout) + remove (force) the container; idempotent on not-found

**NetworkProvider**:
- `Create`: create a Docker network with optional driver; return `network_id`, `name`, `driver`
- `Read`: inspect the network by ID; return `(nil, nil)` if not found
- `Update`: delete + re-create
- `Delete`: remove the network; idempotent on not-found

## 3. Non-Responsibilities

- Does not manage Docker volumes
- Does not manage Docker images (no explicit pull; Docker daemon handles it)
- Does not configure environment variables, entrypoints, or resource limits
- Does not support multi-container networking beyond the `network_id` property
- Does not communicate with remote Docker daemons (uses `DOCKER_HOST` env via `client.FromEnv`)

## 4. Architecture Design

```
pkg/provider/docker/
├── container.go    ContainerProvider (Create, Read, Update, Delete)
└── network.go      NetworkProvider   (Create, Read, Update, Delete)

Both use: github.com/docker/docker/client
          github.com/docker/docker/api/types/container
          github.com/docker/go-connections/nat
```

```
NewContainerProvider()
  client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
  → connects to local Docker socket

ContainerProvider.Create(desired)
  image        = desired.Properties["image"]
  port         = desired.Properties["port"]   (float64 from YAML, cast to int)
  network_id   = desired.Properties["network_id"]
  ContainerCreate → ContainerStart
  return ResourceState{ID: containerID, Attributes: {...}}
```

### Port Handling

YAML decodes numeric literals as `float64`. The port property must be cast:
```go
switch v := portVal.(type) {
case float64: port = int(v)
case int:     port = v
}
```

Port binding binds `0.0.0.0:<port> → <port>/tcp` on the host.

## 5. Core Data Structures (Go)

```go
package docker

import "github.com/docker/docker/client"

type ContainerProvider struct {
    client *client.Client
}

func NewContainerProvider() (*ContainerProvider, error) {
    cli, err := client.NewClientWithOpts(
        client.FromEnv,
        client.WithAPIVersionNegotiation(),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create Docker client: %w", err)
    }
    return &ContainerProvider{client: cli}, nil
}

type NetworkProvider struct {
    client *client.Client
}

func NewNetworkProvider() (*NetworkProvider, error) {
    // same pattern as ContainerProvider
}
```

### Container Attributes

| Attribute | Type | Description |
|---|---|---|
| `container_id` | string | Full Docker container SHA |
| `image` | string | Image used at creation |
| `status` | string | `running` (Create), current status (Read) |
| `port` | int | Host port, if set |
| `network_id` | string | Network ID, if set |

### Network Attributes

| Attribute | Type | Description |
|---|---|---|
| `network_id` | string | Docker-assigned network ID |
| `name` | string | Network name |
| `driver` | string | Network driver |

## 6. Public Interfaces

```go
func NewContainerProvider() (*ContainerProvider, error)
func (p *ContainerProvider) Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error)
func (p *ContainerProvider) Read(ctx context.Context, resourceID string) (*state.ResourceState, error)
func (p *ContainerProvider) Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error)
func (p *ContainerProvider) Delete(ctx context.Context, resourceID string) error

func NewNetworkProvider() (*NetworkProvider, error)
func (p *NetworkProvider) Create(ctx context.Context, desired *config.Resource) (*state.ResourceState, error)
func (p *NetworkProvider) Read(ctx context.Context, resourceID string) (*state.ResourceState, error)
func (p *NetworkProvider) Update(ctx context.Context, desired *config.Resource, resourceID string) (*state.ResourceState, error)
func (p *NetworkProvider) Delete(ctx context.Context, resourceID string) error
```

## 7. Internal Algorithms

### ContainerProvider.Create
```
image    = desired.Properties["image"].(string)
port     = cast Properties["port"] to int (float64 or int)
networkID = desired.Properties["network_id"].(string)  // optional

containerCfg = &container.Config{Image: image}
hostCfg      = &container.HostConfig{}

if port > 0:
    portStr = fmt.Sprintf("%d/tcp", port)
    containerCfg.ExposedPorts = {portStr: {}}
    hostCfg.PortBindings = {portStr: [{HostIP:"0.0.0.0", HostPort: strconv.Itoa(port)}]}

if networkID != "":
    hostCfg.NetworkMode = NetworkMode(networkID)

resp = client.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, desired.ID)
client.ContainerStart(ctx, resp.ID, StartOptions{})

return ResourceState{
    ID: resp.ID,
    Attributes: {container_id, image, status:"running", port?, network_id?}
}
```

### ContainerProvider.Delete (idempotent)
```
client.ContainerStop(ctx, resourceID, {Timeout: &10})
  // ignore IsErrNotFound
client.ContainerRemove(ctx, resourceID, {Force: true})
  // ignore IsErrNotFound
```

### ContainerProvider.Update (replace)
```
p.Delete(ctx, resourceID)
return p.Create(ctx, desired)
```

### NetworkProvider.Create
```
name   = desired.Properties["name"].(string)
driver = desired.Properties["driver"].(string)  // optional

resp = client.NetworkCreate(ctx, name, NetworkCreate{Driver: driver})
return ResourceState{
    ID: resp.ID,
    Attributes: {network_id: resp.ID, name, driver}
}
```

## 8. Persistence Model

No files. All state is tracked in GoIaC's `state.json`. The Docker daemon itself maintains the actual container/network state.

## 9. Concurrency Model

Both providers use a `*client.Client` which is thread-safe (the Docker SDK explicitly documents this). Providers themselves have no mutable state after construction.

## 10. Configuration

Docker connection is configured via environment variables read by `client.FromEnv`:

| Env var | Default | Description |
|---|---|---|
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker daemon socket or TCP address |
| `DOCKER_TLS_VERIFY` | — | Enable TLS verification |
| `DOCKER_CERT_PATH` | — | Path to TLS certificates |

No GoIaC-specific configuration.

## 11. Observability

No metrics. Errors wrap Docker API errors with operation context (e.g., `"failed to create container: ..."`, `"failed to start container: ..."`).

## 12. Testing Strategy

Docker provider tests require a running Docker daemon. They are integration tests.

- `TestContainerProviderCreate`: create an `alpine` container → inspect via Docker API, assert running
- `TestContainerProviderRead`: create then `Read` → attributes match
- `TestContainerProviderReadMissing`: `Read("nonexistent-id")` → `(nil, nil)`
- `TestContainerProviderDelete`: create then delete → container no longer exists
- `TestContainerProviderDeleteIdempotent`: delete non-existent ID → no error
- `TestContainerProviderUpdate`: create then update with new image → new container running
- `TestContainerWithPort`: create with `port: 8080` → Docker API confirms port binding
- `TestNetworkProviderCRUD`: create network, read it, delete it, verify gone
- `TestContainerWithNetwork`: create network, create container referencing network ID → container attached

All tests must `t.Cleanup(func() { provider.Delete(ctx, id) })` to avoid leaving containers behind.

## 13. Open Questions

- Should `Create` explicitly pull the image if not present, rather than relying on Docker's implicit pull (which may fail silently on air-gapped systems)?
- Should `Update` preserve the container name (`desired.ID`) rather than letting Docker auto-name the replacement?
