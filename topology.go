// Package topology manages dependency service lifecycle for noPerfection services.
package topology

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/noPerfection/log"
	"github.com/noPerfection/topology/config"
)

// ServiceLifecycle starts and stops dependency services through the topology.
//
// Pass a service name or dereference URL (*pkg:$?var=...). Topology must load
// the config record, not just resolve a path: Spore fetches the value and Fruit
// embeds nested links into a full Service (handlers, endpoints, start_command).
type ServiceLifecycle interface {
	// StopService stops the given dependency service.
	//
	// Symbol:
	//
	//	tp.StopService("worker")
	//
	// Dereference Mushroom URL:
	//
	//	tp.StopService("*pkg:$?var=services[name:worker]")
	StopService(mushroomURL string) error

	// StartService starts the dependency service and returns its topology id.
	//
	// Symbol:
	//
	//	id, err := tp.StartService("worker")
	//
	// Dereference Mushroom URL:
	//
	//	id, err := tp.StartService("*pkg:$?var=services[name:worker]")
	StartService(mushroomURL string) (string, error)
}

// NodeInterface is implemented by service managers that probe, start, and stop
// dependency services. Probes use the manager's own CURVE identity; they are not
// routed through the topology handler.
type NodeInterface interface {
	ServiceLifecycle

	// IsServiceRunning reports whether the dependency service is running.
	//
	// Symbol:
	//
	//	running, err := tp.IsServiceRunning("worker")
	//
	// Dereference Mushroom URL:
	//
	//	running, err := tp.IsServiceRunning("*pkg:$?var=services[name:worker]")
	// When attempts > 1 the config is reloaded before every probe so that
	// public keys written to disk by newly started services become visible.
	IsServiceRunning(mushroomURL string, attempts ...int) (bool, error)
}

// TopologyInterface is implemented by the topology handler client.
//
// It doesn't have the `Stop` command.
// Because, stopping must be done by the remote call from other services.
// Use it if you want to implement your own topology.
type TopologyInterface interface {
	ServiceLifecycle

	// Service returns a service configuration resolved by symbol or dereference Mushroom URL.
	//
	// Symbol:
	//
	//	svc, err := tp.Service("auth_proxy")
	//
	// Dereference Mushroom URL:
	//
	//	svc, err := tp.Service("*pkg:$?var=services[name:auth_proxy]")
	Service(mushroomURL string) (config.Service, error)

	// Handler returns a handler configuration resolved by dereference Mushroom URL.
	//
	// Dereference Mushroom URL:
	//
	//	h, err := tp.Handler("*pkg:$?var=services[name:auth_proxy].handlers[category:main]")
	//
	// When the URL resolves to a service rather than a handler, DefaultCategory is used:
	//
	//	h, err := tp.Handler("*pkg:$?var=services[name:auth_proxy]")
	Handler(mushroomURL string) (config.Handler, error)

	// GetFacade returns a facade Mushroom link for a service resolved by dereference URL.
	//
	// Handler category comes from the mushroom URL additional property category
	// (defaults to DefaultCategory when omitted). command is an optional second
	// argument for the command route; resolution follows handler-deps and
	// command-deps to return the facade for a command handler and its dependency target.
	//
	// Dereference Mushroom URL:
	//
	//	link, err := tp.GetFacade("*pkg:$?var=services[name:main]&category=main", "authorize")
	GetFacade(mushroomURL string, command ...string) (string, error)

	// GetLink normalizes mushroomURL into a verified full Mushroom link.
	// Dereference URLs are converted to links; plain service names are expanded.
	// Resource paths and additional properties are preserved.
	//
	// Symbol:
	//
	//	link, err := tp.GetLink("auth_proxy")
	//	  → "pkg:json/…/app.json?var=services[name:auth_proxy]"
	//
	// Dereference Mushroom URL:
	//
	//	link, err := tp.GetLink("*pkg:$?var=services[name:auth_proxy]&category=main")
	//	  → "pkg:json/…/app.json?var=services[name:auth_proxy]&category=main"
	GetLink(mushroomURL string) (string, error)

	// Services returns the list of configured services.
	Services() ([]config.Service, error)

	// AddService registers a service in the topology configuration.
	//
	// Default parent (root services array):
	//
	//	err := tp.AddService(record)
	//
	// Explicit parent dereference Mushroom URL:
	//
	//	err := tp.AddService(record, "*pkg:$?var=services[name:proxy].handlers[category:main].outbounds")
	AddService(record config.Service, parent ...string) error

	// SetService updates an existing service in the topology configuration.
	//
	// Default parent:
	//
	//	err := tp.SetService(record)
	//
	// Explicit parent dereference Mushroom URL:
	//
	//	err := tp.SetService(record, "*pkg:$?var=services[name:proxy].handlers[category:main].outbounds")
	SetService(record config.Service, parent ...string) error

	// SetHandler updates an existing handler in the topology configuration.
	//
	//	mushroomURL is a dereference Mushroom URL of the handler or service with category:
	//
	//	err := tp.SetHandler(record, "*pkg:$?var=services[name:proxy].handlers[category:main]")
	//	err := tp.SetHandler(record, "*pkg:$?var=services[name:proxy]&category=main")
	SetHandler(record config.Handler, mushroomURL string) error

	// RemoveService removes a service from the topology configuration.
	//
	// Default parent:
	//
	//	err := tp.RemoveService("worker")
	//
	// Explicit parent dereference Mushroom URL:
	//
	//	err := tp.RemoveService("old_outbound", "*pkg:$?var=services[name:proxy].handlers[category:main].outbounds")
	RemoveService(name string, parent ...string) error

	// ValidateProtocolOrder checks protocol forwarding rules for a service and its
	// reachable dependency graph. Caller transport is the handler endpoint (tcp, ipc,
	// or inproc via inproc-handlers).
	//
	// Allowed forwarding:
	//
	//	Caller → TCP  → IPC  → inproc
	//	inproc   ✅     ✅     ✅
	//	ipc      ✅     ✅     ❌
	//	tcp      ✅     ❌     ❌
	//
	// Symbolic url:
	//
	//	err := tp.ValidateProtocolOrder("auth_proxy")
	//
	// Dereference Mushroom URL:
	//
	//	err := tp.ValidateProtocolOrder("*pkg:$?var=services[name:auth_proxy]")
	ValidateProtocolOrder(mushroomURL string) error

	// ValidateInprocServiceManagers checks every registered service: if inproc,
	// its manager must be inproc.
	ValidateInprocServiceManagers() error

	// InprocessDepNumber counts inproc dependency services reachable from the given
	// service through handler-deps and command-deps.
	//
	// Symbol:
	//
	//	count, err := tp.InprocessDepNumber("auth_proxy")
	//
	// Dereference Mushroom URL:
	//
	//	count, err := tp.InprocessDepNumber("*pkg:$?var=services[name:auth_proxy]")
	InprocessDepNumber(mushroomURL string) (int, error)

	// Snapshot returns the topology JSON document as a compact JSON string.
	Snapshot() (string, error)

	// Rollback restores the topology from a prior Snapshot by replacing the
	// entire module document.
	Rollback(snapshot string) error

	// Reload re-reads the topology JSON file from disk, replacing the in-memory
	// state.
	Reload() error
}

// DefaultTimeout is the default time to wait before considering the message is not delivered.
const DefaultTimeout = time.Second * 5

const rootServicesParent = "*pkg:$?var=services"

type Process struct {
	config *config.Service
	id     string
	cmd    *exec.Cmd
	done   chan error // signalizes when the service finished
}

// Topology runs spawned dependency service processes.
type Topology struct {
	sameServices     map[string]int
	runningProcesses map[string]*Process
	timeout          time.Duration
}

// New creates a dependency service runtime.
func New() *Topology {
	return &Topology{
		sameServices:     make(map[string]int),
		runningProcesses: make(map[string]*Process, 0),
		timeout:          DefaultTimeout,
	}
}

func (tp *Topology) forgetServiceCount(name string) {
	if tp != nil && tp.sameServices != nil {
		delete(tp.sameServices, name)
	}
}

func resolveParent(parent ...string) string {
	if len(parent) > 0 && parent[0] != "" {
		return parent[0]
	}
	return rootServicesParent
}

func serviceQueryURL(name, parent string) string {
	return fmt.Sprintf("%s[name:%s]", parent, name)
}

//---------------------------------------------------------------------
//
// Dependency service runtime
//
//---------------------------------------------------------------------

// StopService stops a locally spawned dependency process, waiting for it to exit.
func (tp *Topology) StopService(service config.Service) error {
	if tp == nil {
		return fmt.Errorf("nil topology")
	}
	serviceName := service.Name
	if serviceName == "" {
		return fmt.Errorf("service name is empty")
	}
	if service.Type == config.IndependentType {
		return fmt.Errorf("service('%s') is independent service, impossible to stop since you are now using it", serviceName)
	}

	process := tp.processForService(serviceName)
	if process == nil {
		return nil
	}

	if err := tp.stopLocalProcess(process); err != nil {
		return fmt.Errorf("service('%s') is still running after stop: %w", serviceName, err)
	}
	return nil
}

// StopAllSpawnedProcesses terminates every locally tracked dependency process.
func (tp *Topology) StopAllSpawnedProcesses() error {
	if tp == nil {
		return nil
	}

	processes := make([]*Process, 0, len(tp.runningProcesses))
	for _, process := range tp.runningProcesses {
		if process != nil {
			processes = append(processes, process)
		}
	}

	var stopErr error
	for _, process := range processes {
		if err := tp.stopLocalProcess(process); err != nil {
			stopErr = errors.Join(stopErr, err)
		}
	}
	return stopErr
}

func (tp *Topology) stopLocalProcess(process *Process) error {
	if process == nil {
		return nil
	}

	if process.cmd != nil && process.cmd.Process != nil {
		if err := process.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("signal SIGTERM: %w", err)
		}
	}

	waitTimeout := tp.timeout * 3
	if waitTimeout <= 0 {
		waitTimeout = DefaultTimeout * 3
	}
	if err := tp.waitForProcess(process, waitTimeout); err == nil {
		return nil
	}

	if process.cmd != nil && process.cmd.Process != nil {
		_ = process.cmd.Process.Kill()
		if killWait := tp.waitForProcess(process, 2*time.Second); killWait != nil {
			return killWait
		}
	}
	return nil
}

// OnStop returns a signal through the channel when the process spawned by the Topology stops.
// If the process is not existing, then it will simply return error.
func (tp *Topology) OnStop(id string) chan error {
	process, ok := tp.runningProcesses[id]
	if !ok {
		return nil
	}

	if process.cmd == nil {
		return nil
	}

	return process.done
}

// generateProcessId returns the next topology id for a service name.
func (tp *Topology) generateProcessId(serviceName string) (string, error) {
	if tp == nil {
		return "", fmt.Errorf("nil topology")
	}
	if len(serviceName) == 0 {
		return "", fmt.Errorf("service name is empty")
	}
	if tp.sameServices == nil {
		tp.sameServices = make(map[string]int)
	}

	count := tp.sameServices[serviceName]
	for {
		count++
		id := fmt.Sprintf("%s%d", serviceName, count)
		if _, exists := tp.runningProcesses[id]; !exists {
			tp.sameServices[serviceName]++
			return id, nil
		}
	}
}

func (tp *Topology) refreshServiceCount(serviceName string) {
	count := 0
	for _, process := range tp.runningProcesses {
		if process != nil && process.config != nil && process.config.Name == serviceName {
			count++
		}
	}
	if count == 0 {
		delete(tp.sameServices, serviceName)
		return
	}
	tp.sameServices[serviceName] = count
}

func (tp *Topology) processForService(serviceName string) *Process {
	for _, process := range tp.runningProcesses {
		if process != nil && process.config != nil && process.config.Name == serviceName {
			return process
		}
	}
	return nil
}

func (tp *Topology) waitForProcess(process *Process, timeout time.Duration) error {
	if process == nil || process.done == nil {
		return nil
	}
	select {
	case <-process.done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("process did not stop")
	}
}

// StartService runs the service start command.
// If it fails to run, then it will return an error.
//
// Note that, services can crash during the initialization.
// In that case, you should use Topology.OnStop method.
func (tp *Topology) StartService(serviceConfig config.Service) (string, error) {
	if tp == nil {
		return "", fmt.Errorf("nil topology")
	}
	if serviceConfig.Name == "" {
		return "", fmt.Errorf("service name is empty")
	}
	if serviceConfig.Type == config.IndependentType {
		return "", fmt.Errorf("independent service can not be started")
	}
	if !serviceConfig.IsIpc() {
		return "", fmt.Errorf("service('%s') is not ipc service", serviceConfig.Name)
	}
	if len(serviceConfig.StartCommand) == 0 {
		return "", fmt.Errorf("service('%s') has no start command given", serviceConfig.Name)
	}

	return tp.startServiceConfig(serviceConfig)
}

func (tp *Topology) startServiceConfig(serviceConfig config.Service) (string, error) {
	process := &Process{config: &serviceConfig}

	if len(process.config.StartCommand) == 0 {
		return "", fmt.Errorf("no start command")
	}

	id, err := tp.generateProcessId(process.config.Name)
	if err != nil {
		return "", fmt.Errorf("tp.generateProcessId('%s'): %w", process.config.Name, err)
	}
	process.id = id

	idFlag := fmt.Sprintf("--id=%s", id)

	args := []string{idFlag}

	commandArgs := strings.Fields(process.config.StartCommand)
	if len(commandArgs) == 0 {
		tp.refreshServiceCount(process.config.Name)
		return "", fmt.Errorf("no start command")
	}

	instance := process.copy()

	tp.runningProcesses[id] = instance

	logger, err := log.New(id, false)
	if err != nil {
		delete(tp.runningProcesses, id)
		tp.refreshServiceCount(process.config.Name)
		return "", fmt.Errorf("log.New('%s'): %w", id, err)
	}
	errLogger, err := log.New(id+"Err", false)
	if err != nil {
		delete(tp.runningProcesses, id)
		tp.refreshServiceCount(process.config.Name)
		return "", fmt.Errorf("log.New('%sErr'): %w", id, err)
	}

	cmd := exec.Command(commandArgs[0], append(commandArgs[1:], args...)...)
	configureChildProcess(cmd)
	cmd.Stdout = logger
	cmd.Stderr = errLogger
	err = cmd.Start()
	if err != nil {
		delete(tp.runningProcesses, id)
		tp.refreshServiceCount(process.config.Name)
		return "", fmt.Errorf("cmd.Start: %w", err)
	}

	instance.cmd = cmd
	tp.wait(id)

	return id, nil
}

// The wait is invoked if the spawned dependency stops.
// The dependencies are running asynchronously.
// In order to call this function, you must use the Topology.StopService() method.
// If the Close signal was sent to the spawned child, then
// this method will be called automatically by the operating system.
func (tp *Topology) wait(id string) {
	go func() {
		process := tp.runningProcesses[id]
		err := process.cmd.Wait() // it can return an error
		process.done <- err
		close(process.done)
		delete(tp.runningProcesses, id)
		tp.refreshServiceCount(process.config.Name)
	}()
}

func (process *Process) copy() *Process {
	return &Process{
		config: process.config,
		id:     process.id,
		done:   make(chan error, 1),
	}
}
