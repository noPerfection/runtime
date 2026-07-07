// Package topology manages dependency service lifecycle for noPerfection services.
package topology

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/noPerfection/log"
	"github.com/noPerfection/topology/config"
)

// NodeInterface starts, stops, and probes dependency services.
//
// Pass a service name or dereference URL (*pkg:$?var=...). Topology must load
// the config record, not just resolve a path: Spore fetches the value and Fruit
// embeds nested links into a full Service (handlers, endpoints, start_command).
type NodeInterface interface {
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

	// IsServiceRunning reports whether the dependency service is running.
	//
	// Symbol:
	//
	//	running, err := tp.IsServiceRunning("worker")
	//
	// Dereference Mushroom URL:
	//
	//	running, err := tp.IsServiceRunning("*pkg:$?var=services[name:worker]")
	IsServiceRunning(mushroomURL string) (bool, error)
}

// TopologyInterface is implemented by the dependency topology.
//
// It doesn't have the `Stop` command.
// Because, stopping must be done by the remote call from other services.
// Use it if you want to implement your own topology.
type TopologyInterface interface {
	NodeInterface

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
}

// DefaultTimeout is the default time to wait before considering the message is not delivered.
// Topology.IsServiceRunning method uses this value before considering the endpoint as not running.
const DefaultTimeout = time.Second * 5

const rootServicesParent = "*pkg:$?var=services"

const DefaultCategory = config.DefaultCategory

const ipcManagerProbeTimeout = 100 * time.Millisecond

const ServiceManagerCategory = config.ServiceManagerCategory

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

// StopService stops the dependency service.
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

	node, err := tp.newServiceManagerClient(&service)
	if err != nil {
		return err
	}
	defer node.Close()

	node.Timeout(tp.managerProbeTimeout(service))
	node.Attempt(2)

	running, err := node.IsServiceRunning(serviceName)
	if err != nil {
		return fmt.Errorf("node.IsServiceRunning('%s'): %w", serviceName, err)
	}
	if !running {
		return nil
	}

	process := tp.processForService(serviceName)
	if err := node.StopService(serviceName); err != nil {
		if process != nil && tp.waitForProcess(process, tp.timeout*3) == nil {
			return nil
		}
		running, runningErr := tp.isServiceRunningWithTimeout(serviceName, service, tp.managerProbeTimeout(service))
		if runningErr == nil && !running {
			return nil
		}
		return fmt.Errorf("node.StopService('%s'): %w", serviceName, err)
	}

	if err := tp.waitForProcess(process, tp.timeout*3); err != nil {
		return fmt.Errorf("service('%s') is still running after stop", serviceName)
	}
	return nil
}

// IsServiceRunning checks whether the given service is running or not.
func (tp *Topology) IsServiceRunning(service config.Service) (bool, error) {
	if tp == nil {
		return false, fmt.Errorf("nil topology")
	}
	if service.Name == "" {
		return false, fmt.Errorf("service name is empty")
	}
	if service.Type == config.IndependentType {
		return true, nil
	}

	return tp.isServiceRunningWithTimeout(service.Name, service, tp.managerProbeTimeout(service))
}

func (tp *Topology) isServiceRunningWithTimeout(serviceName string, service config.Service, timeout time.Duration) (bool, error) {
	node, err := tp.newServiceManagerClient(&service)
	if err != nil {
		return false, err
	}
	defer node.Close()

	node.Attempt(1)
	node.Timeout(timeout)

	running, err := node.IsServiceRunning(serviceName)
	if err != nil {
		return false, nil
	}

	return running, nil
}

func (tp *Topology) managerProbeTimeout(service config.Service) time.Duration {
	managerHandler, err := service.HandlerByCategory(ServiceManagerCategory)
	if err != nil {
		return tp.timeout
	}
	handler, ok := managerHandler.AsIndependentHandler()
	if !ok {
		return tp.timeout
	}
	return tp.managerProbeTimeoutForHandler(handler)
}

func (tp *Topology) managerProbeTimeoutForHandler(handler config.IndependentHandler) time.Duration {
	if handler.Endpoint.IsIpc() {
		return ipcManagerProbeTimeout
	}
	return tp.timeout
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

func (tp *Topology) newServiceManagerClient(service *config.Service) (*NodeClient, error) {
	handler, err := service.HandlerByCategory(ServiceManagerCategory)
	if err != nil {
		return nil, fmt.Errorf("no manager found in the '%s' service, please set its config", service.Name)
	}

	independentHandler, ok := handler.AsIndependentHandler()
	if !ok {
		return nil, fmt.Errorf("manager handler in '%s' is invalid", service.Name)
	}
	node, err := newNodeClient(independentHandler.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("NewNode: %w", err)
	}

	return node, nil
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

	node, err := tp.newServiceManagerClient(&serviceConfig)
	if err != nil {
		return "", err
	}
	defer node.Close()

	node.Attempt(1)
	node.Timeout(tp.managerProbeTimeout(serviceConfig))

	running, err := node.IsServiceRunning(serviceConfig.Name)
	if err == nil && running {
		return "", fmt.Errorf("service('%s') is already running", serviceConfig.Name)
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
