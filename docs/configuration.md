# Configuration Reference

GoIaC configuration is a single YAML file (default: `main.yaml`) that declares the desired state of your infrastructure.

---

## Top-Level Structure

```yaml
resources:
  - id: <unique-string>
    type: <resource-type>
    properties:
      <key>: <value>
```

| Field | Required | Description |
|---|---|---|
| `id` | yes | Unique identifier for this resource within the config. Used as the state map key and in `${...}` references. |
| `type` | yes | The resource type, which determines which provider handles it. |
| `properties` | yes | A map of key-value pairs specific to the resource type. |

Rules:
- Every resource must have a non-empty `id` and `type`.
- No two resources may share the same `id`.
- Unknown types are caught at validation time (before any changes are made).
- Unknown properties for a known type are also rejected at validation time.

---

## Property Interpolation

You can reference an attribute from another resource using:

```
${type.resource_id.attribute}
```

- `type` — the `type` field of the referenced resource (e.g. `docker_network`)
- `resource_id` — the `id` field of the referenced resource (e.g. `app-network`)
- `attribute` — the output attribute name from the provider (e.g. `network_id`)

Example:

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

GoIaC extracts the reference, adds a dependency edge from `app-network` to `web-server`, and resolves `${docker_network.app-network.network_id}` to the actual `network_id` attribute that Docker assigns — only after `app-network` has been created.

Interpolation is resolved at **apply time**, not plan time, because the attribute value may not exist yet.

---

## Built-In Resource Types

### `local_file`

Manages a file on the local filesystem.

| Property | Required | Type | Description |
|---|---|---|---|
| `path` | yes | string | Absolute or relative path to the file. |
| `content` | yes | string | Full text content of the file. |

**Output attributes** (available via `${local_file.<id>.<attr>}`):

| Attribute | Type | Description |
|---|---|---|
| `path` | string | Same as the input `path`. |
| `content` | string | Content written to the file. |
| `size` | int | File size in bytes. |
| `mode` | string | File permission bits (e.g. `-rw-r--r--`). |

Example:

```yaml
resources:
  - id: app-config
    type: local_file
    properties:
      path: ./config/app.conf
      content: |
        host=localhost
        port=5432
```

---

### `docker_container`

Manages a Docker container. Requires Docker Engine to be running and accessible.

| Property | Required | Type | Description |
|---|---|---|---|
| `image` | yes | string | Docker image name and tag (e.g. `nginx:latest`). |
| `port` | no | int | Host port to expose. Binds `0.0.0.0:<port>` → `<port>/tcp`. |
| `network_id` | no | string | Docker network ID or name to attach the container to. |

**Output attributes**:

| Attribute | Type | Description |
|---|---|---|
| `container_id` | string | Full Docker container SHA. |
| `image` | string | Image used. |
| `status` | string | Container status (e.g. `running`). |
| `port` | int | Exposed port (if set). |
| `network_id` | string | Attached network ID (if set). |

**Update behavior**: Docker containers are replaced on update (stop + remove + recreate). There is no in-place update.

Example:

```yaml
resources:
  - id: web
    type: docker_container
    properties:
      image: nginx:1.25
      port: 8080
```

---

### `docker_network`

Manages a Docker network.

| Property | Required | Type | Description |
|---|---|---|---|
| `name` | yes | string | Network name. |
| `driver` | no | string | Network driver (e.g. `bridge`, `overlay`). Defaults to Docker's default (`bridge`). |

**Output attributes**:

| Attribute | Type | Description |
|---|---|---|
| `network_id` | string | Docker-assigned network ID. |
| `name` | string | Network name. |
| `driver` | string | Driver used. |

Example:

```yaml
resources:
  - id: backend-net
    type: docker_network
    properties:
      name: backend
      driver: bridge
```

---

## Full Example

```yaml
resources:
  - id: backend-net
    type: docker_network
    properties:
      name: backend
      driver: bridge

  - id: db
    type: docker_container
    properties:
      image: postgres:15
      port: 5432
      network_id: ${docker_network.backend-net.network_id}

  - id: api
    type: docker_container
    properties:
      image: myapp:latest
      port: 8080
      network_id: ${docker_network.backend-net.network_id}

  - id: api-config
    type: local_file
    properties:
      path: ./api.env
      content: "DATABASE_URL=postgres://localhost:5432/app"
```

GoIaC will create `backend-net` first, then `db` and `api` (which both depend on `backend-net`), then `api-config` (no dependencies, may run at any point after its own resources are ready). On destroy, the order is reversed.
