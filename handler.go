// Package topology manages dependency service lifecycle for noPerfection services.
package topology

import (
	"fmt"
	"sync"
	"time"

	"github.com/ahmetson/mushroom"
	"github.com/noPerfection/datatype"
	"github.com/noPerfection/log"
	clientSyncReplier "github.com/noPerfection/protocol/client/sync_replier"
	"github.com/noPerfection/protocol/handler/base"
	"github.com/noPerfection/protocol/handler/control"
	"github.com/noPerfection/protocol/handler/replier"
	"github.com/noPerfection/protocol/message"
	"github.com/noPerfection/topology/config"
)

const (
	TopologyHandlerCategory       = "service_topology" // handler category
	TopologySocketType            = base.ReplierType
	IsRunning                     = "is-running"
	IsServiceRunning              = "is-service-running"
	StartService                  = "start-service"
	StopService                   = "stop-service"
	Service                       = "service"
	Services                      = "services"
	GetHandler                    = "get-handler"
	GetFacade                     = "get-facade"
	GetLink                       = "get-link"
	AddService                    = "add-service"
	SetService                    = "set-service"
	SetHandler                    = "set-handler"
	RemoveService                 = "remove-service"
	ValidateProtocolOrder         = "validate-protocol-order"
	ValidateInprocServiceManagers = "validate-inproc-service-managers"
	InprocessDepNumber            = "inprocess-dep-number"
	Snapshot                      = "snapshot"
	Rollback                      = "rollback"
)

// Handler acts as the router from other app processes to the topology.
type Handler struct {
	handler  base.Interface // Receive commands
	config   *config.NoPerfection
	topology *Topology // dependency service runtime
	started  bool
	mu       sync.Mutex
}

var _ TopologyInterface = (*Handler)(nil)

var topologyMutationMu sync.Mutex

// HandlerEndpoint returns the inproc endpoint for the topology handler.
func HandlerEndpoint() message.Endpoint {
	return message.NewEndpoint(TopologyHandlerCategory, 0)
}

// NewHandler loads app config, ensures the independent topology service entry exists,
// persists config when it changed, and returns a dependency topology handler.
//
// configMushroomURL is the mushroom URL of the service to load the config from.
// If its starts with "pkg:", then it is a mushroom URL and the config is loaded from the package.
// Otherwise, it is a file path and the config is loaded from the file that internally converted into mushroom url.
//
// Optional substrates are passed to config.Load for dereference resolution (e.g. pkg:os#env).
// Topology does not register built-in substrates; callers supply them.
func NewHandler(configMushroomURL string, substrates ...mushroom.Substrate) (*Handler, error) {
	appConfig, err := config.Load(configMushroomURL, substrates...)
	if err != nil {
		return nil, fmt.Errorf("config.Load('%s'): %w", configMushroomURL, err)
	}

	handler := replier.New()

	logger, err := log.New(TopologyHandlerCategory, true)
	if err != nil {
		return nil, fmt.Errorf("log.New('%s'): %w", TopologyHandlerCategory, err)
	}

	handler.SetEndpoint(HandlerEndpoint())
	err = handler.SetLogger(logger)
	if err != nil {
		return nil, fmt.Errorf("handler.SetLogger: %w", err)
	}

	return &Handler{
		config:   &appConfig,
		topology: New(),
		handler:  handler,
	}, nil
}

func (h *Handler) requireNotStarted() error {
	if h == nil {
		return fmt.Errorf("handler is nil")
	}
	if h.started {
		return fmt.Errorf("topology handler already started, use topology client")
	}
	if h.config == nil {
		return fmt.Errorf("config is nil")
	}
	if h.topology == nil {
		return fmt.Errorf("topology is nil")
	}
	return nil
}

func optionalParent(params datatype.KeyValue) []string {
	parent, err := params.StringValue("parent")
	if err != nil || parent == "" {
		return nil
	}
	return []string{parent}
}

// AddService registers a service in the topology configuration before the
// topology handler is started.
func (h *Handler) AddService(record config.Service, parent ...string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return err
	}

	return h.addService(record, parent...)
}

// SetService updates a service in the topology configuration before the topology
// handler is started.
func (h *Handler) SetService(record config.Service, parent ...string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return err
	}

	return h.setService(record, parent...)
}

// SetHandler updates a handler in the topology configuration before the topology
// handler is started.
func (h *Handler) SetHandler(record config.Handler, mushroomURL string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return err
	}

	return h.setHandler(record, mushroomURL)
}

// RemoveService removes a service from the topology configuration before the
// topology handler is started.
func (h *Handler) RemoveService(name string, parent ...string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return err
	}

	return h.removeService(name, parent...)
}

// StartService starts a dependency service before the topology handler is
// started.
func (h *Handler) StartService(mushroomURL string) (string, error) {
	if err := h.requireNotStarted(); err != nil {
		return "", err
	}

	service, err := h.service(mushroomURL)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.topology.StartService(service)
}

// IsServiceRunning checks a dependency service before the topology handler is
// started.
func (h *Handler) IsServiceRunning(mushroomURL string) (bool, error) {

	if err := h.requireNotStarted(); err != nil {
		return false, err
	}

	service, err := h.service(mushroomURL)
	if err != nil {
		return false, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.topology.IsServiceRunning(service)
}

// StopService stops a dependency service before the topology handler is started.
func (h *Handler) StopService(mushroomURL string) error {

	if err := h.requireNotStarted(); err != nil {
		return err
	}

	service, err := h.service(mushroomURL)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.topology.StopService(service)
}

// Service returns a service configuration before the topology handler is started.
func (h *Handler) Service(mushroomURL string) (config.Service, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return config.Service{}, err
	}

	return h.service(mushroomURL)
}

// Handler returns a handler configuration before the topology handler is started.
func (h *Handler) Handler(mushroomURL string) (config.Handler, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return nil, err
	}

	return h.handlerConfig(mushroomURL)
}

// GetFacade returns a facade Mushroom link before the topology handler is started.
// Handler category comes from the mushroom URL additional property category.
func (h *Handler) GetFacade(mushroomURL string, command ...string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return "", err
	}

	return h.getFacade(mushroomURL, command...)
}

// GetLink normalizes mushroomURL into a verified full Mushroom link before start.
func (h *Handler) GetLink(mushroomURL string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return "", err
	}

	return h.getLink(mushroomURL)
}

// Services returns service configurations before the topology handler is started.
func (h *Handler) Services() ([]config.Service, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return nil, err
	}

	return h.services()
}

// ValidateProtocolOrder checks protocol forwarding rules for a service and its
// reachable dependency graph before the topology handler is started.
//
// Allowed forwarding (caller handler transport → outbound):
//
//	Caller → TCP  → IPC  → inproc
//	inproc   ✅     ✅     ✅
//	ipc      ✅     ✅     ❌
//	tcp      ✅     ❌     ❌
func (h *Handler) ValidateProtocolOrder(mushroomURL string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return err
	}

	service, err := h.service(mushroomURL)
	if err != nil {
		return err
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	return h.config.ValidateProtocolOrdersFor(service)
}

// ValidateInprocServiceManagers checks if the service is inproc, then its manager must be inproc too.
// *pkg:golang/github.com/noPerfection/topology/config?var=NoPerfection.ValidateInprocServiceManagers&comment=true
func (h *Handler) ValidateInprocServiceManagers() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return err
	}

	return h.config.ValidateInprocServiceManagers()
}

// InprocessDepNumber counts inproc dependency services for the given service.
// *pkg:golang/github.com/noPerfection/topology/config?var=NoPerfection.InprocessDepNumber&comment=true
func (h *Handler) InprocessDepNumber(mushroomURL string) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.config == nil {
		return 0, fmt.Errorf("nil config")
	}
	service, err := h.service(mushroomURL)
	if err != nil {
		return 0, err
	}
	return h.config.InprocessDepNumber(service)
}

// Snapshot returns the topology JSON document as a compact JSON string.
func (h *Handler) Snapshot() (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return "", err
	}
	if h.config == nil {
		return "", fmt.Errorf("nil config")
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	return h.config.Snapshot()
}

// Rollback restores the topology from a prior Snapshot.
func (h *Handler) Rollback(snapshot string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.requireNotStarted(); err != nil {
		return err
	}
	if h.config == nil {
		return fmt.Errorf("nil config")
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	return h.config.Rollback(snapshot)
}

func (h *Handler) addService(record config.Service, parent ...string) error {
	if h.config == nil {
		return fmt.Errorf("nil config")
	}
	if record.IsZero() {
		return fmt.Errorf("service is empty")
	}
	if len(record.Name) == 0 {
		return fmt.Errorf("service name is empty")
	}
	if err := record.Validate(); err != nil {
		return fmt.Errorf("config.Validate('%s'): %w", record.Name, err)
	}

	parentURL := resolveParent(parent...)
	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	if err := h.config.AddService(record, parentURL); err != nil {
		return fmt.Errorf("config.AddService: %w", err)
	}
	return h.config.Save()
}

func (h *Handler) setService(record config.Service, parent ...string) error {
	if h.config == nil {
		return fmt.Errorf("nil config")
	}
	if len(record.Name) == 0 {
		return fmt.Errorf("service name is empty")
	}
	if err := record.Validate(); err != nil {
		return fmt.Errorf("config.Validate('%s'): %w", record.Name, err)
	}

	parentURL := resolveParent(parent...)

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	if err := h.config.SetService(record, parentURL); err != nil {
		return fmt.Errorf("config.SetService: %w", err)
	}
	return h.config.Save()
}

func (h *Handler) setHandler(record config.Handler, mushroomURL string) error {
	if h.config == nil {
		return fmt.Errorf("nil config")
	}
	if record == nil {
		return fmt.Errorf("handler is empty")
	}
	if _, ok := record.AsIndependentHandler(); !ok {
		return fmt.Errorf("handler is not an independent handler")
	}
	if mushroomURL == "" {
		return fmt.Errorf("mushroom url is empty")
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	if err := h.config.SetHandler(record, mushroomURL); err != nil {
		return fmt.Errorf("config.SetHandler: %w", err)
	}
	return h.config.Save()
}

func (h *Handler) removeService(name string, parent ...string) error {
	if h.config == nil {
		return fmt.Errorf("nil config")
	}
	if h.topology == nil {
		return fmt.Errorf("topology is nil")
	}
	if len(name) == 0 {
		return fmt.Errorf("service name is empty")
	}

	parentURL := resolveParent(parent...)
	service, err := h.config.GetService(serviceQueryURL(name, parentURL))
	if err != nil {
		return err
	}
	running, err := h.topology.IsServiceRunning(service)
	if err != nil {
		return err
	}
	if running {
		return fmt.Errorf("service('%s') is running, please stop it first", name)
	}

	// Removing from the configuration should be locking
	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	if err := h.config.RemoveService(name, parentURL); err != nil {
		return err
	}
	if err := h.config.Save(); err != nil {
		return fmt.Errorf("config.Save: %w", err)
	}

	h.topology.forgetServiceCount(name)
	return nil
}

func (h *Handler) service(mushroomURL string) (config.Service, error) {
	if h.config == nil {
		return config.Service{}, fmt.Errorf("nil config")
	}
	if len(mushroomURL) == 0 {
		return config.Service{}, fmt.Errorf("mushroom url is empty")
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	record, err := h.config.GetService(mushroomURL)
	if err != nil {
		return config.Service{}, fmt.Errorf("config.GetService(%q): %w", mushroomURL, err)
	}
	return record, nil
}

func (h *Handler) handlerConfig(mushroomURL string) (config.Handler, error) {
	if h.config == nil {
		return nil, fmt.Errorf("nil config")
	}
	if len(mushroomURL) == 0 {
		return nil, fmt.Errorf("mushroom url is empty")
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	handler, err := h.config.GetHandler(mushroomURL)
	if err != nil {
		return nil, fmt.Errorf("config.GetHandler(%q): %w", mushroomURL, err)
	}
	return handler, nil
}

func (h *Handler) getFacade(mushroomURL string, command ...string) (string, error) {
	if h.config == nil {
		return "", fmt.Errorf("nil config")
	}
	if len(mushroomURL) == 0 {
		return "", fmt.Errorf("mushroom url is empty")
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	link, err := h.config.GetFacade(mushroomURL, command...)
	if err != nil {
		return "", fmt.Errorf("config.GetFacade(%q): %w", mushroomURL, err)
	}
	return link.AsLink().String(), nil
}

func (h *Handler) getLink(mushroomURL string) (string, error) {
	if h.config == nil {
		return "", fmt.Errorf("nil config")
	}
	if len(mushroomURL) == 0 {
		return "", fmt.Errorf("mushroom url is empty")
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	link, err := h.config.GetServiceLink(mushroomURL)
	if err != nil {
		return "", fmt.Errorf("config.GetServiceLink(%q): %w", mushroomURL, err)
	}
	return link, nil
}

func (h *Handler) services() ([]config.Service, error) {
	if h.config == nil {
		return nil, fmt.Errorf("nil config")
	}
	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	services, err := h.config.GetServices(rootServicesParent)
	if err != nil {
		return nil, fmt.Errorf("config.GetServices(%q): %w", rootServicesParent, err)
	}
	copied := make([]config.Service, len(services))
	copy(copied, services)
	return copied, nil
}

func (h *Handler) onIsRunning(req message.RequestInterface) message.ReplyInterface {
	return req.Ok(datatype.New().Set("running", true))
}

// onIsServiceRunning checks whether the dependency is running or not.
// Requires 'service' — a service name or dereference Mushroom URL.
func (h *Handler) onIsServiceRunning(req message.RequestInterface) message.ReplyInterface {
	mushroomURL, err := req.RouteParameters().StringValue(Service)
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('service'): %v", err))
	}

	service, err := h.service(mushroomURL)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.service(%q): %v", mushroomURL, err))
	}

	running, err := h.topology.IsServiceRunning(service)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.topology.IsServiceRunning: %v", err))
	}

	params := datatype.New().Set("running", running)
	return req.Ok(params)
}

// onStartService starts the dependency service.
// Requires 'service' — a service name or dereference Mushroom URL.
func (h *Handler) onStartService(req message.RequestInterface) message.ReplyInterface {
	mushroomURL, err := req.RouteParameters().StringValue(Service)
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('service'): %v", err))
	}

	service, err := h.service(mushroomURL)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.service(%q): %v", mushroomURL, err))
	}

	id, err := h.topology.StartService(service)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.topology.StartService(%q): %v", mushroomURL, err))
	}

	return req.Ok(datatype.New().Set("id", id))
}

// onAddService registers a service in the topology configuration.
// Requires 'service' as a service object.
func (h *Handler) onAddService(req message.RequestInterface) message.ReplyInterface {
	kv, err := req.RouteParameters().NestedValue("service")
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetKeyValue('service'): %v", err))
	}

	var record config.Service
	if err := kv.Interface(&record); err != nil {
		return req.Fail(fmt.Sprintf("kv.Interface('config.Service'): %v", err))
	}

	if err := h.addService(record, optionalParent(req.RouteParameters())...); err != nil {
		return req.Fail(fmt.Sprintf("h.addService('%s'): %v", record.Name, err))
	}

	return req.Ok(datatype.New())
}

// onSetService updates a service in the topology configuration.
// Requires 'service' of the config.Service type.
func (h *Handler) onSetService(req message.RequestInterface) message.ReplyInterface {
	kv, err := req.RouteParameters().NestedValue("service")
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetKeyValue('service'): %v", err))
	}

	var record config.Service
	err = kv.Interface(&record)
	if err != nil {
		return req.Fail(fmt.Sprintf("kv.Interface: %v", err))
	}

	err = h.setService(record, optionalParent(req.RouteParameters())...)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.setService('%s'): %v", record.Name, err))
	}

	return req.Ok(datatype.New())
}

// onSetHandler updates a handler in the topology configuration.
// Requires 'handler' record and 'mushroomURL' dereference Mushroom URL of the handler.
func (h *Handler) onSetHandler(req message.RequestInterface) message.ReplyInterface {
	kv, err := req.RouteParameters().NestedValue("handler")
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetKeyValue('handler'): %v", err))
	}

	raw, err := kv.Bytes()
	if err != nil {
		return req.Fail(fmt.Sprintf("kv.Bytes: %v", err))
	}

	record, err := config.UnmarshalHandler(raw)
	if err != nil {
		return req.Fail(fmt.Sprintf("config.UnmarshalHandler: %v", err))
	}

	mushroomURL, err := req.RouteParameters().StringValue("mushroomURL")
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('mushroomURL'): %v", err))
	}

	if err := h.setHandler(record, mushroomURL); err != nil {
		base, _ := record.AsIndependentHandler()
		return req.Fail(fmt.Sprintf("h.setHandler(%q): %v", base.Category, err))
	}

	return req.Ok(datatype.New())
}

// onRemoveService removes a service from the topology configuration.
// Requires 'service' string parameter with the service name.
func (h *Handler) onRemoveService(req message.RequestInterface) message.ReplyInterface {
	serviceName, err := req.RouteParameters().StringValue("service")
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('service'): %v", err))
	}

	err = h.removeService(serviceName, optionalParent(req.RouteParameters())...)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.removeService('%s'): %v", serviceName, err))
	}

	return req.Ok(datatype.New())
}

// onStopService stops the dependency.
// Requires 'service' — a service name or dereference Mushroom URL.
func (h *Handler) onStopService(req message.RequestInterface) message.ReplyInterface {
	mushroomURL, err := req.RouteParameters().StringValue(Service)
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('service'): %v", err))
	}

	service, err := h.service(mushroomURL)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.service(%q): %v", mushroomURL, err))
	}

	err = h.topology.StopService(service)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.topology.StopService: %v", err))
	}

	return req.Ok(datatype.New())
}

// onService returns the configuration for a service.
// Requires 'service' — a service name or dereference Mushroom URL.
func (h *Handler) onService(req message.RequestInterface) message.ReplyInterface {
	mushroomURL, err := req.RouteParameters().StringValue(Service)
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('service'): %v", err))
	}

	record, err := h.service(mushroomURL)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.service(%q): %v", mushroomURL, err))
	}

	return req.Ok(datatype.New().Set("service", record))
}

// onGetHandler returns the configuration for a handler.
// Requires 'handler' — a dereference Mushroom URL.
func (h *Handler) onGetHandler(req message.RequestInterface) message.ReplyInterface {
	mushroomURL, err := req.RouteParameters().StringValue("handler")
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('handler'): %v", err))
	}

	record, err := h.handlerConfig(mushroomURL)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.handlerConfig(%q): %v", mushroomURL, err))
	}

	params, err := datatype.NewFromInterface(record)
	if err != nil {
		return req.Fail(fmt.Sprintf("datatype.NewFromInterface: %v", err))
	}
	return req.Ok(params)
}

// onGetFacade returns the facade Mushroom link for a service.
// Requires 'service' — a dereference Mushroom URL with optional category additional
// property. Optional 'command' selects a command route on that handler.
func (h *Handler) onGetFacade(req message.RequestInterface) message.ReplyInterface {
	mushroomURL, err := req.RouteParameters().StringValue(Service)
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('service'): %v", err))
	}

	command, _ := req.RouteParameters().StringValue("command")

	facade, err := h.getFacade(mushroomURL, optionalCommand(command)...)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.getFacade(%q): %v", mushroomURL, err))
	}

	return req.Ok(datatype.New().Set("facade", facade))
}

// onGetLink returns a verified full Mushroom link for mushroomURL.
// Requires 'link' — a symbol, link, or dereference Mushroom URL.
func (h *Handler) onGetLink(req message.RequestInterface) message.ReplyInterface {
	mushroomURL, err := req.RouteParameters().StringValue("link")
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('link'): %v", err))
	}

	link, err := h.getLink(mushroomURL)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.getLink(%q): %v", mushroomURL, err))
	}

	return req.Ok(datatype.New().Set("link", link))
}

func optionalCommand(command string) []string {
	if command == "" {
		return nil
	}
	return []string{command}
}

// onServices returns the configuration for all services.
func (h *Handler) onServices(req message.RequestInterface) message.ReplyInterface {
	records, err := h.services()
	if err != nil {
		return req.Fail(fmt.Sprintf("h.services: %v", err))
	}

	return req.Ok(datatype.New().Set("services", records))
}

// onValidateProtocolOrder checks protocol forwarding rules for a service.
// Requires 'service' — a service name or dereference Mushroom URL.
func (h *Handler) onValidateProtocolOrder(req message.RequestInterface) message.ReplyInterface {
	mushroomURL, err := req.RouteParameters().StringValue(Service)
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('service'): %v", err))
	}

	service, err := h.service(mushroomURL)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.service(%q): %v", mushroomURL, err))
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	if err := h.config.ValidateProtocolOrdersFor(service); err != nil {
		return req.Fail(fmt.Sprintf("h.config.ValidateProtocolOrdersFor(%q): %v", mushroomURL, err))
	}

	return req.Ok(datatype.New())
}

// onValidateInprocServiceManagers checks every registered service: if inproc, its manager must be inproc.
func (h *Handler) onValidateInprocServiceManagers(req message.RequestInterface) message.ReplyInterface {
	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	if err := h.config.ValidateInprocServiceManagers(); err != nil {
		return req.Fail(fmt.Sprintf("h.config.ValidateInprocServiceManagers: %v", err))
	}

	return req.Ok(datatype.New())
}

// onInprocessDepNumber counts inproc dependency services for a service.
// Requires 'service' — a service name or dereference Mushroom URL.
func (h *Handler) onInprocessDepNumber(req message.RequestInterface) message.ReplyInterface {
	mushroomURL, err := req.RouteParameters().StringValue(Service)
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.GetString('service'): %v", err))
	}

	service, err := h.service(mushroomURL)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.service(%q): %v", mushroomURL, err))
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	count, err := h.config.InprocessDepNumber(service)
	if err != nil {
		return req.Fail(fmt.Sprintf("h.config.InprocessDepNumber(%q): %v", mushroomURL, err))
	}

	return req.Ok(datatype.New().Set("count", count))
}

// onSnapshot returns the topology JSON document.
func (h *Handler) onSnapshot(req message.RequestInterface) message.ReplyInterface {
	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	snapshot, err := h.config.Snapshot()
	if err != nil {
		return req.Fail(fmt.Sprintf("h.config.Snapshot: %v", err))
	}

	return req.Ok(datatype.New().Set("snapshot", snapshot))
}

// onRollback restores the topology from a prior Snapshot.
func (h *Handler) onRollback(req message.RequestInterface) message.ReplyInterface {
	snapshot, err := req.RouteParameters().StringValue("snapshot")
	if err != nil {
		return req.Fail(fmt.Sprintf("req.Parameters.StringValue('snapshot'): %v", err))
	}

	topologyMutationMu.Lock()
	defer topologyMutationMu.Unlock()

	if err := h.config.Rollback(snapshot); err != nil {
		return req.Fail(fmt.Sprintf("h.config.Rollback: %v", err))
	}

	return req.Ok(datatype.New())
}

// Start starts the dependency handler with the available operations.
func (h *Handler) Start() error {
	if h == nil {
		return fmt.Errorf("handler is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return nil
	}
	if h.isTopologyAlreadyRunning() {
		h.started = true
		return nil
	}
	if h.restartExistingTopologyHandler() {
		h.started = true
		return nil
	}

	if err := h.handler.Route(IsRunning, h.onIsRunning); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", IsRunning, err)
	}
	if err := h.handler.Route(IsServiceRunning, h.onIsServiceRunning); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", IsServiceRunning, err)
	}
	if err := h.handler.Route(StartService, h.onStartService); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", StartService, err)
	}
	if err := h.handler.Route(StopService, h.onStopService); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", StopService, err)
	}
	if err := h.handler.Route(Service, h.onService); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", Service, err)
	}
	if err := h.handler.Route(GetHandler, h.onGetHandler); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", GetHandler, err)
	}
	if err := h.handler.Route(GetFacade, h.onGetFacade); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", GetFacade, err)
	}
	if err := h.handler.Route(GetLink, h.onGetLink); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", GetLink, err)
	}
	if err := h.handler.Route(Services, h.onServices); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", Services, err)
	}
	if err := h.handler.Route(AddService, h.onAddService); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", AddService, err)
	}
	if err := h.handler.Route(SetService, h.onSetService); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", SetService, err)
	}
	if err := h.handler.Route(SetHandler, h.onSetHandler); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", SetHandler, err)
	}
	if err := h.handler.Route(RemoveService, h.onRemoveService); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", RemoveService, err)
	}
	if err := h.handler.Route(ValidateProtocolOrder, h.onValidateProtocolOrder); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", ValidateProtocolOrder, err)
	}
	if err := h.handler.Route(ValidateInprocServiceManagers, h.onValidateInprocServiceManagers); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", ValidateInprocServiceManagers, err)
	}
	if err := h.handler.Route(InprocessDepNumber, h.onInprocessDepNumber); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", InprocessDepNumber, err)
	}
	if err := h.handler.Route(Snapshot, h.onSnapshot); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", Snapshot, err)
	}
	if err := h.handler.Route(Rollback, h.onRollback); err != nil {
		return fmt.Errorf("h.handler.Route('%s'): %v", Rollback, err)
	}

	if err := h.handler.Start(); err != nil {
		return err
	}
	h.started = true
	return nil
}

func (h *Handler) isTopologyAlreadyRunning() bool {
	client, err := NewClient()
	if err != nil {
		return false
	}
	defer client.Close()

	client.Timeout(50 * time.Millisecond)
	client.Attempt(2)
	running, err := client.IsRunning()
	return err == nil && running
}

func (h *Handler) restartExistingTopologyHandler() bool {
	controlEndpoint := control.NewInternalControlEndpoint(HandlerEndpoint())
	client, err := clientSyncReplier.NewClient(controlEndpoint.Id, controlEndpoint.Port)
	if err != nil {
		return false
	}
	defer client.Close()

	client.Timeout(50 * time.Millisecond)
	client.Attempt(2)
	reply, err := client.Request(&message.Request{
		Command:    control.HandlerStart,
		Parameters: datatype.New(),
	})
	return err == nil && reply.IsOK()
}
