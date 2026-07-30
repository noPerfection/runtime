# Topology

`topology` is a noPerfection module that provides noPerfection microservice to manage their dependencies.

The `config` directory is a sub module defining topology configuration. Now it's simply a shared `json` file.

> License? **Public Domain**

## Install

Requires zmq library C library. Go code running or building must be then done using C enabling.

```sh
go get github.com/noPerfection/topology@latest
```

## Tutorial

First we need to start the topology handler

```go
import "github.com/noPerfection/topology"

//.. rest of code
handler, _ := topology.NewHandler("service.json")

// Any handler's functions.

// Register any additional handler routes or setup before starting, if needed.
if err := handler.Start(); err != nil {
	panic(err)
}

```

That's it. Topology is running in-process.
Second, we need to interact with it from the code:

```go
	// Now interact with the topology manager through a topology client.
	topologyClient, _ := topology.NewClient()
	defer topologyClient.Close()

	running, err := topologyClient.IsServiceRunning("database")
	if err != nil {
		panic(err)
	}

	_ = running
}
```

## Topology Handler

`topology.NewHandler(configPath)` returns a handler that serves topology commands over an in-process noPerfection protocol socket. The handler loads `configPath` using `config.Load`, saves any topology bootstrap changes, and uses the predefined `inproc://<TopologyHandlerCategory>` command endpoint.

The handler exposes these commands internally:

- `add-service`
- `set-service`
- `remove-service`
- `start-service`
- `stop-service`
- `is-service-running`

Applications usually do not send these commands directly. Use `topology.NewClient()` instead.

Before `Start()` is called, the returned handler also implements `topology.TopologyInterface`. This lets setup code manipulate the topology configuration directly:

```go
handler, _ := topology.NewHandler("service.json")

if err := handler.AddService(service); err != nil {
	panic(err)
}

if err := handler.Start(); err != nil {
	panic(err)
}
```

After `Start()` succeeds, direct topology methods on the handler are unavailable and return an error. Use `topology.NewClient()` for `AddService`, `SetService`, `RemoveService`, `StartService`, `StopService`, and `IsServiceRunning` after launch.

## Topology Client API

`topology.NewClient()` returns a `*topology.Client`. It builds the same in-process endpoint internally with `message.NewEndpoint(TopologyHandlerCategory, 0)`, which resolves to `inproc://<TopologyHandlerCategory>`. Configure request behavior with:

```go
topologyClient.Timeout(5 * time.Second)
topologyClient.Attempt(1)
```

Available client methods:

```go
type NodeInterface interface {
	StopService(mushroomURL string) error
	StartService(mushroomURL string) (string, error)
	IsServiceRunning(mushroomURL string) (bool, error)
}

type TopologyInterface interface {
	NodeInterface

	Service(mushroomURL string) (config.Service, error)
	AddService(service config.Service, parent ...string) error
	SetService(service config.Service, parent ...string) error
	RemoveService(name string, parent ...string) error
}
```

Plain service names still work as shorthand for `*pkg:$?var=services[name:<name>]`. See [config/README.md](config/README.md) for Mushroom URL syntax.

### Add or Update Services

```go
service := config.Service{
	Type:         config.ExtensionType,
	Name:         "worker",
	StartCommand: "./worker",
	Handlers: config.NewHandlerVariants(
		config.Handler{
			Type:     config.ReplierType,
			Category: topology.ServiceManagerCategory,
			Endpoint: message.NewEndpoint("worker-manager", 6001),
		},
	),
}

if err := topologyClient.AddService(service); err != nil {
	panic(err)
}

// Optional parent dereference URL (defaults to *pkg:$?var=services when omitted)
if err := topologyClient.AddService(outbound, "*pkg:$?var=services[name:proxy].handlers[category:main].outbounds"); err != nil {
	panic(err)
}

service.StartCommand = "./worker --debug"
if err := topologyClient.SetService(service); err != nil {
	panic(err)
}
```

### Start, Check, and Stop Services

```go
id, err := topologyClient.StartService("worker")
if err != nil {
	panic(err)
}

// Or with an explicit dereference Mushroom URL
id, err = topologyClient.StartService("*pkg:$?var=services[name:worker]")

if running, err := topologyClient.IsServiceRunning("worker"); err != nil {
	panic(err)
} else if running {
	if err := topologyClient.StopService("worker"); err != nil {
		panic(err)
	}
}

_ = id
```

### Remove Services

```go
if err := topologyClient.RemoveService("worker"); err != nil {
	panic(err)
}
```

`RemoveService` refuses to remove a service that is currently running. `AddService` refuses to add independent services and refuses to overwrite an existing service. `SetService` updates an existing service.

## Service Requirements

Every managed service must have a handler that manages the service itself. By convention, this handler uses the `manager` category.

The topology uses the `manager` handler to:

- connect to the service
- send `heartbeat` requests for `IsServiceRunning`
- send the close command for `StopService`

Example service config:

```json
{
  "type": "Extension",
  "name": "worker",
  "start-command": "./worker",
  "handlers": [
    {
      "type": "Replier",
      "category": "manager",
      "endpoint": {
        "id": "worker-manager",
        "port": 6001
      }
    }
  ]
}
```

Independent services are special: there can be only one independent service in the config, and it represents the service currently running the topology handler. It cannot be added through `AddService` or stopped through `StopService`.

## Local development

For local testing, building, and running, prefix each command with `GOFLAGS=-modfile=go.local.mod`:

```sh
GOFLAGS=-modfile=go.local.mod go test ./...
GOFLAGS=-modfile=go.local.mod go build ./...
GOFLAGS=-modfile=go.local.mod go run ./cmd/your-app
cd config && GOFLAGS=-modfile=go.local.mod go test ./...
```

`make test` and `make build` already use `go.local.mod`. Use `make test-remote` or plain `go test ./...` without `GOFLAGS` for the remote module file.

## Tests

Run the tests locally:

```sh
GOFLAGS=-modfile=go.local.mod go test ./...
cd config && GOFLAGS=-modfile=go.local.mod go test ./...
```

Or:

```sh
make test
```

Topology tests compile on a fresh checkout. Tests that start sample binaries require local fixtures under `_test_services`.