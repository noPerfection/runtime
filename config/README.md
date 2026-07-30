# noPerfection Configuration

This package defines the static JSON configuration model for a noPerfection topology. It contains Go types, JSON marshal/unmarshal helpers, `Load`, `NoPerfection.Save`, and validation. It does not run a topology server or client.

Based on [ahmetson/mushroom](https://github.com/ahmetson/mushroom) . To manage configuration is converted into mushroom URLs.

> License? **Public Domain**

## Topology Model

The config describes a topology as a graph.

- A `Service` is a node in the graph.
- A `Handler` is an entry point on that node.
- A `DepService` is an edge declaration: it says which other nodes a handler or command may call.
- A `ServicePointer` is the target of that edge, either by reference or as an inline nested service.

This separation keeps identity, transport, and routing independent:

- `Service` owns deployment metadata such as `module-url` and `start-command`.
- `Handler` owns endpoint metadata such as protocol type, category, and address.
- `DepService` owns dependency intent without embedding the target service into every caller.
- `ServicePointer` lets the same dependency point to a shared service or define an inline private service.

```
NoPerfection
└── services[]: Service
    ├── handler-deps[]: DepService          (service-wide routing by handler category)
    └── handlers[]: Handler
        ├── IndependentHandler              (single endpoint)
        ├── ProxyHandler                    (endpoint + outbound routing)
        └── ExtensionHandler                (endpoint + inbound services)
            └── command-deps[]: DepService  (per-command routing)
                ├── proxies[]: ServicePointer
                └── extensions[]: ServicePointer
```

See [examples/app-proxy-chain.json](examples/app-proxy-chain.json) for a complete graph.

## Root Config

`NoPerfection` is the JSON document root. It contains the services that can be resolved by name and validated as one topology.

```json
{
  "services": [
    {
      "type": "Independent",
      "name": "public_api",
      "handlers": []
    }
  ]
}
```

`Load` calls `json_substrate.Root`, which forages the JSON file from disk and germinates a mycelium. `Save` mineralizes in memory and persists with `Substrate.Sow`.

### `Load(mushroomURL)`

`Load` accepts either a **symbolic file path** or a **Mushroom link URL**.


| Form          | Example                                           | Mycelium link                       |
| ------------- | ------------------------------------------------- | ----------------------------------- |
| Symbolic path | `noPerfection.json`, `/etc/app/noPerfection.json` | `pkg:json/.#noPerfection.json`      |
| Mushroom link | `pkg:json/tmp#app.json`                           | used as-is (normalized via `Hypha`) |


Mushroom link requirements:

- must be a **link** (`pkg:…`), not a dereference (`*pkg:…`)
- substrate **type** must be `json`
- **module** id must end with `.json`
- must refer to a **module** only (no `?var=` resource path)

If the backing file does not exist, `Load` creates parent directories, seeds `{"services":[]}`, and skips topology validation. When the file already exists, `Load` validates the graph.

```go
// Symbolic path (same as before)
app, err := config.Load("noPerfection.json")

// Explicit Mushroom link
app, err := config.Load("pkg:json/tmp#app.json")
```

## Mycelium Storage

Topology data is stored as a [Mushroom](https://github.com/ahmetson/mushroom) JSON mycelium, not as a direct Go slice. `Load` uses `json_substrate.Root` (forage + germinate). Reads and writes go through **dereference** Mushroom URLs (`*pkg:$?var=`) via `Spore`/`Fruit`; mutations use `Graft`/`Inoculate`/`Prune`. `Save` uses `Mineralize` and `Sow`.

```text
*pkg:$?var=services
```

`$` is a wildcard that fills in type, package, and module from the loaded file URL. For a file loaded as `pkg:json/tmp#app.json`, `*pkg:$?var=services` resolves against that mycelium.

Link URLs (`?var=` without `*`) are not used by this package. Mushroom's `Link()` API resolves symbolic paths to absolute link strings; topology config does not call it for service queries or mutations.

Plain service names passed to `GetService` are shorthand for a dereference name filter on that array:

```text
auth_proxy  →  *pkg:$?var=services[name:auth_proxy]
```

See the [Mushroom README](https://github.com/ahmetson/mushroom) for URL syntax, filters, and built-in calls such as `first()` and `last()`.

## Querying Services

### `GetService(mushroomURL)`

Resolves a dereference Mushroom URL with `Spore`, embeds nested links with `Fruit`, and decodes the result into `Service`. If the value is not a service, it returns an error.

```go
// Shorthand by service name
service, err := app.GetService("auth_proxy")

// Explicit dereference Mushroom URL
service, err := app.GetService("*pkg:$?var=services[name:auth_proxy]")

// First Independent service
service, err := app.GetService("*pkg:$?var=services[type:Independent][$.first()]")
```

### `GetServices(mushroomURL)`

Same query flow as `GetService`, but expects an array and returns `[]Service`. Pass a dereference URL (`*var`). If the resolved value is not an array of services, the URL is fetching the wrong data.

```go
// All root services
services, err := app.GetServices("*pkg:$?var=services")

// Filter by type
services, err := app.GetServices("*pkg:$?var=services[type:Independent]")

// Proxy outbounds on a named handler
services, err := app.GetServices(
    "*pkg:$?var=services[name:auth_proxy].handlers[category:main].outbounds",
)
```

### `CountByType(mushroomURL)`

Calls `GetServices` and returns the length of the result.

```go
count, err := app.CountByType("*pkg:$?var=services[type:Proxy]")
```

### `Services()` (topology layer)

The topology handler exposes `Services()` as a convenience wrapper around `GetServices("*pkg:$?var=services")`.

## Mutating Services

`AddService`, `SetService`, and `RemoveService` take a **parent dereference URL** — the same `*pkg:$?var=` form used for reads. The parent identifies the array to read and mutate.

```go
err := app.AddService(newService, "*pkg:$?var=services")
```

Each method first loads the parent array with `GetServices(parent)`, checks name uniqueness or existence, then mutates the mycelium in memory. Call `Save` to persist changes.

### `AddService(record, parent)`

Appends a service with `Graft`. The name must not already exist in the parent array.

```go
err := app.AddService(newService, "*pkg:$?var=services")

// Append to a nested array
err := app.AddService(
    outbound,
    "*pkg:$?var=services[name:auth_proxy].handlers[category:main].outbounds",
)
```

### `SetService(record, parent)`

Replaces an existing service by name with `Inoculate`.

```go
err := app.SetService(updatedService, "*pkg:$?var=services")

// Update a service inside a nested parent
err := app.SetService(
    updatedOutbound,
    "*pkg:$?var=services[name:auth_proxy].handlers[category:main].outbounds",
)
```

### `RemoveService(name, parent)`

Removes a service by name with `Prune`.

```go
err := app.RemoveService("auth_proxy", "*pkg:$?var=services")

// Remove from a nested parent
err := app.RemoveService(
    "old_outbound",
    "*pkg:$?var=services[name:auth_proxy].handlers[category:main].outbounds",
)
```

Underlying mycelium operations use the same dereference paths:

```go
mycelium.Graft("*pkg:$?var=services", newServiceMap)
mycelium.Inoculate("*pkg:$?var=services[name:foo]", newServiceMap)
mycelium.Prune("*pkg:$?var=services[name:foo]")
```

## Services

A `Service` is the unit that gets registered, looked up, started, and referenced by dependencies. Its `name` is the stable identity used by refs such as `"auth_proxy"` or `"auth_proxy/main"`.

The `type` describes the role the service plays in routing:


| `type`        | Role                                                          |
| ------------- | ------------------------------------------------------------- |
| `Independent` | A normal service that handles its own traffic.                |
| `Proxy`       | A service that forwards commands to inline outbound services. |
| `Extension`   | A service used as an extension target in dependency routing.  |


Bootstrap fields live on the service, not on each handler, because starting or loading code is a property of the service process/module:

- `module-url` — how to load an inproc handler.
- `start-command` — how to start an IPC handler.

`handler-deps` belongs here when routing is tied to a handler category regardless of command. `parameters` is free-form service metadata and is intentionally not interpreted by this package.

## Handlers

A service may expose multiple handlers because one service can have several entry points: public API, internal API, manager endpoint, publisher, pair socket, and so on. The `category` is the stable label used to choose between those entry points.

In JSON, each `handlers[]` entry is an `IndependentHandler`, `ProxyHandler`, or `ExtensionHandler`. The unmarshaller chooses `ProxyHandler` when proxy routing fields such as `outbounds`, `routes`, or `forward` are present. It chooses `ExtensionHandler` when `inbounds` is present.

`IndependentHandler` is the base shape: protocol type, category, endpoint, and optional command-level dependencies. It represents an endpoint that receives or serves traffic.

`ProxyHandler` embeds the same base shape and adds outbound routing:

- `outbounds` — inline `Service` definitions where proxied traffic may be sent.
- `routes` — optional whitelist of command routes this handler accepts.
- `forward` — optional map from a route to a specific outbound.

Notice: `outbounds` are inline services, not refs. Referenced services may have their own proxies, so following a ref from an outbound would require traversing another topology path to find the final endpoint.

A `Proxy` service must use `ProxyHandler` for every handler in `handlers`.

`ExtensionHandler` embeds the same base shape and adds inbound services:

- `inbounds` — inline `Service` definitions that may call into this extension.

`Extension` services must use `ExtensionHandler` for every handler in `handlers`. `inbounds` follow the same validation rules as proxy `outbounds`.

## Dependencies

Dependencies are declared separately from services so callers describe intent, while targets remain reusable and independently validated.

`DepService` is used in two scopes. In `handler-deps`, `name` is a handler category on the service. In `command-deps`, `name` is a command handled by the parent handler.


| Declared in    | `name` means                               |
| -------------- | ------------------------------------------ |
| `handler-deps` | handler category on this service           |
| `command-deps` | command name handled by the parent handler |


Each dependency lists proxy targets, extension targets, or both. Proxies are for routed calls through another service. Extensions are for extension-style targets.

`ServicePointer` is the target value in `proxies` and `extensions`. It is either:

- a **ref** to an existing service in `services`, e.g. `"auth_proxy"` or `"auth_proxy/main"`.
- an **inline service** object defined directly in the JSON.

Refs make shared dependencies explicit. Inline services keep private or nested dependencies close to the caller without requiring a top-level service entry.

## Ref Format

Refs point to services declared in the root `services` list.

- `service_name`: references a service.
- `service_name/handler_category`: references one handler category on a service.

Invalid refs include empty strings, trailing slashes, empty service names, and empty path segments such as `"auth_proxy//main"`.

## Endpoint Bootstrap Rules

Endpoint `port` defaults to `0` when omitted.

- `port: 0` with an id starting with `tmp` is IPC and requires `start-command`.
- `port: 0` with any other id is inproc and requires `module-url`.
- non-zero ports do not require bootstrap metadata.
- manager handlers (`category: "manager"`) are ignored when deciding service bootstrap requirements.

## Validation

`Load` validates the graph via an internal `validateTopology` check when the JSON file already exists on disk.

Validation checks service names, service types, handler types, handler categories, endpoint ids, dependency target shape, inline service definitions, and ref existence. If a ref includes a handler category, the referenced service must contain that category.

For `ProxyHandler`, validation also checks outbound targets and verifies that `forward` entries refer to declared routes and outbounds.

## Local development

For local testing, building, and running, prefix each command with `GOFLAGS=-modfile=go.local.mod`:

```sh
GOFLAGS=-modfile=go.local.mod go test ./...
GOFLAGS=-modfile=go.local.mod go build ./...
GOFLAGS=-modfile=go.local.mod go run ./cmd/your-app
cd config && GOFLAGS=-modfile=go.local.mod go test ./...
```

`make test` and `make build` already use `go.local.mod`. Use `make test-remote` or plain `go test ./...` without `GOFLAGS` for the remote module file.

### Version

For versioning sub module use the semantics:

```bash
git tag -a config/v0.0.1 -m "Summary"
git push origin config/v0.0.1
```

