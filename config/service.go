package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/ahmetson/mushroom"
	"github.com/ahmetson/mushroom/substrates/json_substrate"
	"github.com/noPerfection/datatype"
	"github.com/noPerfection/protocol/message"
)

const (
	// All services must have a manager handler. Identified by this category.
	ServiceManagerCategory = "manager"
	// DefaultCategory is the handler category used when a Mushroom URL resolves to a service.
	DefaultCategory = "main"
	// Use "inproc-handlers" to list handler categories that should be treated as inproc.
	// Its in the service parameters: parameters.inproc-handlers: [list of handler categories]
	InprocHandlersParameter = "inproc-handlers"
	// Use "ipc-handlers" to list handler categories that should be treated in ipc protocol but endpoint is tcp.
	// Its in the service parameters: parameters.ipc-handlers: [list of handler categories]
	IpcHandlersParameter = "ipc-handlers"
)

// Command Deps or Service deps per handler of service.
// Use it to pipe other services
type DepService struct {
	// For command deps its command, for handler deps its handler category
	Name       string   `json:"name"`
	Proxies    []string `json:"proxies,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
}

type IndependentHandler struct {
	Type        HandlerType      `json:"type"`
	Category    string           `json:"category"`
	Endpoint    message.Endpoint `json:"endpoint"`
	CommandDeps []DepService     `json:"command-deps,omitempty"`
}

type ProxyHandler struct {
	IndependentHandler
	Routes []string `json:"routes,omitempty"` // whitelist routes
	// Outbounds are facade handler Mushroom links the proxy may send traffic to
	// (e.g. pkg:$?var=services[name:worker]&category=main). Use links, not
	// dereferences, so Fruit leaves them as strings on load.
	Outbounds []string          `json:"outbounds"`
	Forward   map[string]string `json:"forward,omitempty"` // command route => outbound URL listed in outbounds
}

type ExtensionHandler struct {
	IndependentHandler
	// Inbounds are Mushroom links to services allowed to call this extension
	// (e.g. pkg:$?var=services[name:api]). Use links, not dereferences, so Fruit
	// leaves them as strings on load.
	Inbounds []string `json:"inbounds"`
}

type Handler interface {
	isHandler()
	AsIndependentHandler() (IndependentHandler, bool)
	AsProxyHandler() (ProxyHandler, bool)
	AsExtensionHandler() (ExtensionHandler, bool)
}

func (h IndependentHandler) isHandler() {}

func (h IndependentHandler) AsIndependentHandler() (IndependentHandler, bool) {
	return h, true
}

func (h IndependentHandler) AsProxyHandler() (ProxyHandler, bool) {
	return ProxyHandler{}, false
}

func (h IndependentHandler) AsExtensionHandler() (ExtensionHandler, bool) {
	return ExtensionHandler{}, false
}

func (h ProxyHandler) isHandler() {}

func (h ProxyHandler) AsIndependentHandler() (IndependentHandler, bool) {
	return h.IndependentHandler, true
}

func (h ProxyHandler) AsProxyHandler() (ProxyHandler, bool) {
	return h, true
}

// SetOutbound appends url when it is not already listed in outbounds.
// Returns false when url was already present.
func (p *ProxyHandler) SetOutbound(url string) bool {
	if slices.Contains(p.Outbounds, url) {
		return false
	}
	p.Outbounds = append(p.Outbounds, url)
	return true
}

func (h ProxyHandler) AsExtensionHandler() (ExtensionHandler, bool) {
	return ExtensionHandler{}, false
}

func (h ExtensionHandler) isHandler() {}

func (h ExtensionHandler) AsIndependentHandler() (IndependentHandler, bool) {
	return h.IndependentHandler, true
}

func (h ExtensionHandler) AsProxyHandler() (ProxyHandler, bool) {
	return ProxyHandler{}, false
}

func (h ExtensionHandler) AsExtensionHandler() (ExtensionHandler, bool) {
	return h, true
}

func UnmarshalHandler(data []byte) (Handler, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, fmt.Errorf("handler is empty")
	}

	var raw map[string]any
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, fmt.Errorf("handler object: %w", err)
	}
	return handlerFromMap(raw)
}

func decodeHandler(value any) (Handler, error) {
	switch handler := value.(type) {
	case IndependentHandler:
		return handler, nil
	case ProxyHandler:
		return handler, nil
	case ExtensionHandler:
		if handler.Inbounds == nil {
			handler.Inbounds = []string{}
		}
		return handler, nil
	case map[string]any:
		return handlerFromMap(handler)
	default:
		return nil, fmt.Errorf("value is not a handler: got %T", value)
	}
}

func handlerFromMap(raw map[string]any) (Handler, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("handler is empty")
	}
	if _, ok := raw["name"]; ok {
		return nil, fmt.Errorf("value is not a handler: service object")
	}
	if _, ok := raw["handlers"]; ok {
		return nil, fmt.Errorf("value is not a handler: service object")
	}
	if _, ok := raw["inbounds"]; ok {
		var handler ExtensionHandler
		if err := mapInto(raw, &handler); err != nil {
			return nil, fmt.Errorf("extension handler: %w", err)
		}
		if handler.Inbounds == nil {
			handler.Inbounds = []string{}
		}
		return handler, nil
	}
	if _, ok := raw["outbounds"]; ok {
		var handler ProxyHandler
		if err := mapInto(raw, &handler); err != nil {
			return nil, fmt.Errorf("proxy handler: %w", err)
		}
		return handler, nil
	}
	if _, ok := raw["routes"]; ok {
		var handler ProxyHandler
		if err := mapInto(raw, &handler); err != nil {
			return nil, fmt.Errorf("proxy handler: %w", err)
		}
		return handler, nil
	}
	if _, ok := raw["forward"]; ok {
		var handler ProxyHandler
		if err := mapInto(raw, &handler); err != nil {
			return nil, fmt.Errorf("proxy handler: %w", err)
		}
		return handler, nil
	}

	var handler IndependentHandler
	if err := mapInto(raw, &handler); err != nil {
		return nil, fmt.Errorf("handler: %w", err)
	}
	return handler, nil
}

func mapInto(src map[string]any, dst any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// Service type defined in the config.
//
// Fields
//   - Type is the type of service. For example, ProxyType, IndependentType or ExtensionType
//   - Name of the service
//   - Handlers that are listed in the service
//   - Parameters optional service-local metadata (not validated)
type Service struct {
	Type         Type              `json:"type"`
	Name         string            `json:"name"`
	ModuleUrl    string            `json:"module-url,omitempty"`
	StartCommand string            `json:"start-command,omitempty"`
	HandlerDeps  []DepService      `json:"handler-deps,omitempty"`
	Handlers     []Handler         `json:"handlers"`
	Parameters   datatype.KeyValue `json:"parameters,omitempty"`

	noPerf      *NoPerfection             `json:"-"`
	mycelium    **json_substrate.Mycelium `json:"-"`
	mushroomURL mushroom.Hypha            `json:"-"`
}

func (s *Service) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type         Type              `json:"type"`
		Name         string            `json:"name"`
		ModuleUrl    string            `json:"module-url,omitempty"`
		StartCommand string            `json:"start-command,omitempty"`
		HandlerDeps  []DepService      `json:"handler-deps,omitempty"`
		Handlers     []json.RawMessage `json:"handlers"`
		Parameters   datatype.KeyValue `json:"parameters,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	handlers := make([]Handler, len(raw.Handlers))
	for i, rawHandler := range raw.Handlers {
		handler, err := UnmarshalHandler(rawHandler)
		if err != nil {
			return fmt.Errorf("handlers[%d]: %w", i, err)
		}
		handlers[i] = handler
	}

	*s = Service{
		Type:         raw.Type,
		Name:         raw.Name,
		ModuleUrl:    raw.ModuleUrl,
		StartCommand: raw.StartCommand,
		HandlerDeps:  raw.HandlerDeps,
		Handlers:     handlers,
		Parameters:   raw.Parameters,
	}
	return nil
}

// If no name its wrong.
func (s Service) IsZero() bool {
	return s.Name == ""
}

// Equal reports whether s and other describe the same service identity in topology.
// Names must match. Manager handlers must both be absent or both present with the same endpoint.
func (s Service) Equal(other Service) bool {
	if s.Name != other.Name {
		return false
	}
	return managersEqual(s, other)
}

// EqualHandlers reports whether s and other have the same handlers, excluding ServiceManagerCategory.
func (s Service) EqualHandlers(other Service) bool {
	a := nonManagerHandlersByCategory(s.Handlers)
	b := nonManagerHandlersByCategory(other.Handlers)
	if len(a) != len(b) {
		return false
	}
	for category, indA := range a {
		indB, ok := b[category]
		if !ok {
			return false
		}
		if indA.Type != indB.Type || !endpointsEqual(indA.Endpoint, indB.Endpoint) {
			return false
		}
	}
	return true
}

func nonManagerHandlersByCategory(handlers []Handler) map[string]IndependentHandler {
	result := make(map[string]IndependentHandler)
	for _, handler := range handlers {
		ind, ok := handler.AsIndependentHandler()
		if !ok || ind.Category == "" || ind.Category == ServiceManagerCategory {
			continue
		}
		result[ind.Category] = ind
	}
	return result
}

func managersEqual(a, b Service) bool {
	mgrA, errA := a.HandlerByCategory(ServiceManagerCategory)
	mgrB, errB := b.HandlerByCategory(ServiceManagerCategory)
	if errA != nil && errB != nil {
		return true
	}
	if errA != nil || errB != nil {
		return false
	}
	indA, okA := mgrA.AsIndependentHandler()
	indB, okB := mgrB.AsIndependentHandler()
	if !okA || !okB {
		return false
	}
	return endpointsEqual(indA.Endpoint, indB.Endpoint)
}

func endpointsEqual(a, b message.Endpoint) bool {
	return a.Id == b.Id && a.Port == b.Port
}

// If service is not Inproc, and any handler is IPC except the ServiceManagerCategory,
// then the service is IPC.
// A handler category listed in parameters.ipc-handlers is also treated as ipc.
func (s Service) IsIpc() bool {
	if s.IsInproc() {
		return false
	}
	for _, variant := range s.Handlers {
		if variant == nil {
			continue
		}
		handler, ok := variant.AsIndependentHandler()
		if !ok {
			continue
		}
		if handler.Category == ServiceManagerCategory {
			continue
		}

		if serviceParameterHasIpcHandler(s, handler.Category) {
			return true
		}

		if handler.Endpoint.IsIpc() {
			return true
		}
	}
	return false
}

// If any handler except ServiceManagerCategory is inproc, then the service is inproc.
// A handler category listed in parameters.inproc-handlers is also treated as inproc.
func (s Service) IsInproc() bool {
	for _, variant := range s.Handlers {
		if variant == nil {
			continue
		}
		handler, ok := variant.AsIndependentHandler()
		if !ok {
			continue
		}
		if handler.Category == ServiceManagerCategory {
			continue
		}

		if serviceParameterHasInprocHandler(s, handler.Category) {
			return true
		}

		if handler.Endpoint.IsInproc() {
			return true
		}
	}
	return false
}

// ValidateInprocServiceManager reports an error when an inproc service has a non-inproc manager endpoint.
// When no manager handler is configured, validation is skipped.
func (s Service) ValidateInprocServiceManager() error {
	if !s.IsInproc() {
		return nil
	}
	managerHandler, err := s.HandlerByCategory(ServiceManagerCategory)
	if err != nil {
		return nil
	}
	handler, ok := managerHandler.AsIndependentHandler()
	if !ok {
		return nil
	}
	if !handler.Endpoint.IsInproc() {
		return fmt.Errorf("service %q is inproc but manager endpoint %q is not inproc", s.Name, handler.Endpoint.ClientUrl())
	}
	return nil
}

// IsInprocHandler reports whether the handler with the given category should be
// treated as in-process for the service. A handler listed in parameters.inproc-handlers
// is treated as inproc even when its endpoint is IPC or TCP. Otherwise the handler
// endpoint is used.
func (s Service) IsInprocHandler(category string) (bool, error) {
	handlerVariant, err := s.HandlerByCategory(category)
	if err != nil {
		return false, err
	}
	handler, ok := handlerVariant.AsIndependentHandler()
	if !ok {
		return false, fmt.Errorf("handler of '%s' category is not an independent handler", category)
	}
	if serviceParameterHasInprocHandler(s, category) {
		return true, nil
	}
	return handler.Endpoint.IsInproc(), nil
}

// IsIpcHandler reports whether the handler with the given category should be
// treated as IPC for the service. A handler listed in parameters.ipc-handlers is
// treated as ipc even when its endpoint is TCP. Otherwise the handler endpoint is used.
func (s Service) IsIpcHandler(category string) (bool, error) {
	handlerVariant, err := s.HandlerByCategory(category)
	if err != nil {
		return false, err
	}
	handler, ok := handlerVariant.AsIndependentHandler()
	if !ok {
		return false, fmt.Errorf("handler of '%s' category is not an independent handler", category)
	}
	if serviceParameterHasInprocHandler(s, category) {
		return false, nil
	}
	if serviceParameterHasIpcHandler(s, category) {
		return true, nil
	}
	return handler.Endpoint.IsIpc(), nil
}

func serviceParameterHasInprocHandler(service Service, category string) bool {
	if service.Parameters == nil || category == "" {
		return false
	}
	raw, exists := service.Parameters[InprocHandlersParameter]
	if !exists {
		return false
	}
	switch categories := raw.(type) {
	case []string:
		return slices.Contains(categories, category)
	case []interface{}:
		for _, item := range categories {
			if name, ok := item.(string); ok && name == category {
				return true
			}
		}
	}
	return false
}

func serviceParameterHasIpcHandler(service Service, category string) bool {
	if service.Parameters == nil || category == "" {
		return false
	}
	raw, exists := service.Parameters[IpcHandlersParameter]
	if !exists {
		return false
	}
	switch categories := raw.(type) {
	case []string:
		return slices.Contains(categories, category)
	case []interface{}:
		for _, item := range categories {
			if name, ok := item.(string); ok && name == category {
				return true
			}
		}
	}
	return false
}

// ValidateService validates the service metadata and endpoint bootstrap settings.
// Returns message.ErrInvalidArg if the service name is empty.
func (s *Service) Validate() error {
	if len(s.Name) == 0 {
		return fmt.Errorf("service name is empty: %w", message.ErrInvalidArg)
	}
	if err := ValidateServiceType(s.Type); err != nil {
		return fmt.Errorf("identity.ValidateServiceType: %v", err)
	}

	needsModuleURL := false
	needsStartCommand := false
	for i, dep := range s.HandlerDeps {
		if err := ValidateDepService(dep); err != nil {
			return fmt.Errorf("ValidateHandlerDep[%d]: %v", i, err)
		}
	}

	for i, h := range s.Handlers {
		if h == nil {
			return fmt.Errorf("handler[%d] is empty", i)
		}
		handler, ok := h.AsIndependentHandler()
		if !ok {
			return fmt.Errorf("handler[%d] is not an independent handler", i)
		}
		if err := ValidateHandlerType(handler.Type); err != nil {
			return fmt.Errorf("ValidateHandlerType[%d]: %v", i, err)
		}
		if len(handler.Category) == 0 {
			return fmt.Errorf("handler[%d] category is empty", i)
		}
		if len(handler.Endpoint.Id) == 0 && !handler.Endpoint.IsRemote() {
			return fmt.Errorf("handler[%d] '%s' endpoint id is empty", i, handler.Category)
		}

		if handler.Endpoint.IsInproc() {
			needsModuleURL = true
		}
		if handler.Endpoint.IsIpc() {
			needsStartCommand = true
		}
		if serviceParameterHasInprocHandler(*s, handler.Category) {
			needsModuleURL = true
		}
		if serviceParameterHasIpcHandler(*s, handler.Category) {
			needsStartCommand = true
		}

		for _, dep := range handler.CommandDeps {
			if err := ValidateDepService(dep); err != nil {
				return fmt.Errorf("ValidateCommandDepService[%d]: %v", i, err)
			}
		}
		if s.Type == ProxyType {
			if handler.Category == ServiceManagerCategory {
				continue
			}
			proxyHandler, ok := h.AsProxyHandler()
			if !ok {
				return fmt.Errorf("handler[%d] must be a proxy handler", i)
			}
			if err := ValidateProxyForwards(proxyHandler); err != nil {
				return fmt.Errorf("handler[%d] forward: %w", i, err)
			}
		}
		if s.Type == ExtensionType {
			if handler.Category == ServiceManagerCategory {
				continue
			}
			if _, ok := h.AsExtensionHandler(); !ok {
				return fmt.Errorf("handler[%d] must be an extension handler", i)
			}
		}
	}

	if needsModuleURL && len(s.ModuleUrl) == 0 {
		return fmt.Errorf("service('%s') has inproc endpoint and requires module-url", s.Name)
	}
	if needsStartCommand && len(s.StartCommand) == 0 {
		return fmt.Errorf("service('%s') has ipc endpoint and requires start-command", s.Name)
	}
	return nil
}

func ValidateProxyForwards(proxyHandler ProxyHandler) error {
	for route := range proxyHandler.Forward {
		if !slices.Contains(proxyHandler.Routes, route) {
			return fmt.Errorf("route %q is not listed in routes", route)
		}
	}
	return nil
}

// Facade resolves the Mushroom link for this service.
//
// The name follows the facade pattern familiar in distributed systems: the caller
// doesn't have to know what topology this service holds. Facade is the endpoint that user need to access.
//
// Receives the handler category that user wants to access, and optionally a command route in that handler to see exact facade for it.
//
// Resolution order:
//  1. handler-deps on this service matching handler category
//  2. when command is given: command-deps on the handler matching command
//  3. otherwise return this service's link with additional property category=<category>
//
// Facade is possible to use in the topology/config.DepService.DepTarget so it includes category additional property.
//
// Examples (see config/examples/app-proxy-chain.json):
//
//	hypha, err := main.Facade("main", "authorize")
//	  → mushroom.Hypha link: pkg:…/services[name:audit_proxy]&category=audit-proxy
//
//	hypha, err := main.Facade("public-api", "authorize")
//	  → mushroom.Hypha link: pkg:…/services[name:audit_proxy]&category=audit-proxy
//
//	hypha, err := userService.Facade("user-service")
//	  → mushroom.Hypha link: pkg:…/services[name:user_service]&category=user-service
func (s Service) Facade(category string, command ...string) (mushroom.Hypha, error) {
	var cmd string
	if len(command) > 0 {
		cmd = command[0]
	} else {
		cmd = ""
	}

	if s.noPerf == nil {
		return mushroom.Hypha{}, fmt.Errorf("service was defined without topology, can't get its facade")
	}
	if s.mycelium == nil {
		return mushroom.Hypha{}, fmt.Errorf("service was defined without topology config, can't get its facade")
	}
	if category == "" {
		return mushroom.Hypha{}, fmt.Errorf("category argument is empty")
	}

	for _, dep := range s.HandlerDeps {
		if dep.Name != category {
			continue
		}
		for _, link := range dep.Proxies {
			next, nextCategory, err := s.noPerf.ResolveDep(link)
			if err != nil {
				return mushroom.Hypha{}, err
			}
			return next.Facade(nextCategory, cmd)
		}
	}

	handler, err := s.HandlerByCategory(category)
	if err != nil {
		return mushroom.Hypha{}, err
	}
	ind, ok := handler.AsIndependentHandler()
	if !ok {
		return mushroom.Hypha{}, fmt.Errorf("handler of %q category is not an independent handler", category)
	}

	if cmd != "" {
		for _, dep := range ind.CommandDeps {
			if dep.Name != cmd {
				continue
			}
			for _, link := range dep.Proxies {
				next, nextCategory, err := s.noPerf.ResolveDep(link)
				if err != nil {
					return mushroom.Hypha{}, err
				}
				return next.Facade(nextCategory, cmd)
			}
		}
	}

	link := s.mushroomURL.AsLink()
	if link.AdditionalProps == nil {
		link.AdditionalProps = map[string]string{}
	}
	link.AdditionalProps["category"] = category
	return link, nil
}

// HandlerByCategory returns the handler config by the handler category.
// If the handler doesn't exist, then it returns an error.
// Returns message.ErrNotFound if the handler doesn't exist.
// Returns message.ErrInvalidArg if the category is empty.
func (s *Service) HandlerByCategory(category string) (Handler, error) {
	if len(category) == 0 {
		return nil, fmt.Errorf("category argument is empty: %w", message.ErrInvalidArg)
	}

	i := slices.IndexFunc(s.Handlers, func(e Handler) bool {
		if e == nil {
			return false
		}
		handler, ok := e.AsIndependentHandler()
		return ok && handler.Category == category
	})
	if i == -1 {
		return nil, fmt.Errorf("handler of '%s' category not found: %w", category, message.ErrNotFound)
	}

	return s.Handlers[i], nil
}

// GetHandler returns a handler by its endpoint.
func (s *Service) GetHandler(endpoint message.Endpoint) (Handler, error) {
	if s == nil {
		return nil, fmt.Errorf("service struct is nil: %w", message.ErrNilStruct)
	}
	if len(endpoint.Id) == 0 && !endpoint.IsRemote() {
		return nil, fmt.Errorf("endpoint id argument is empty: %w", message.ErrInvalidArg)
	}

	i := slices.IndexFunc(s.Handlers, func(h Handler) bool {
		if h == nil {
			return false
		}
		handler, ok := h.AsIndependentHandler()
		return ok && handler.Endpoint.Id == endpoint.Id && handler.Endpoint.Port == endpoint.Port
	})
	if i == -1 {
		return nil, fmt.Errorf("handler with endpoint '%s:%d' not found: %w", endpoint.Id, endpoint.Port, message.ErrNotFound)
	}

	return s.Handlers[i], nil
}

// SetHandler adds a new handler.
// If the handler with the same endpoint is identical, it will over-write that handler.
// Otherwise, it will add a new handler.
func (s *Service) SetHandler(handler Handler, overwriteByCategory ...bool) {
	if s == nil {
		return
	}
	if handler == nil {
		return
	}
	baseHandler, ok := handler.AsIndependentHandler()
	if !ok {
		return
	}

	if len(s.Handlers) == 0 {
		s.Handlers = []Handler{handler}
		return
	}

	var i int
	if len(overwriteByCategory) > 0 && overwriteByCategory[0] {
		i = slices.IndexFunc(s.Handlers, func(h Handler) bool {
			if h == nil {
				return false
			}
			handler, ok := h.AsIndependentHandler()
			return ok && handler.Category == baseHandler.Category
		})
	} else {
		i = slices.IndexFunc(s.Handlers, func(h Handler) bool {
			if h == nil {
				return false
			}
			handler, ok := h.AsIndependentHandler()
			return ok && handler.Endpoint == baseHandler.Endpoint
		})
	}

	if i == -1 {
		s.Handlers = append(s.Handlers, handler)
		return
	}

	s.Handlers[i] = handler
}

// RemoveHandler removes a handler by its endpoint.
// Returns message.ErrNilStruct if the service struct is nil.
// Returns message.ErrInvalidArg if the endpoint id is empty.
// Returns message.ErrNotFound if the handler with the endpoint is not found.
func (s *Service) RemoveHandler(endpoint message.Endpoint) error {
	if s == nil {
		return fmt.Errorf("service struct is nil: %w", message.ErrNilStruct)
	}
	if len(endpoint.Id) == 0 && !endpoint.IsRemote() {
		return fmt.Errorf("endpoint id argument is empty: %w", message.ErrInvalidArg)
	}

	i := slices.IndexFunc(s.Handlers, func(h Handler) bool {
		if h == nil {
			return false
		}
		handler, ok := h.AsIndependentHandler()
		return ok && handler.Endpoint.Id == endpoint.Id && handler.Endpoint.Port == endpoint.Port
	})
	if i == -1 {
		return fmt.Errorf("handler with endpoint '%s:%d' not found: %w", endpoint.Id, endpoint.Port, message.ErrNotFound)
	}

	s.Handlers = slices.Delete(s.Handlers, i, i+1)
	return nil
}

// ValidateDepService checks that a dependency declares a name and routing targets.
// Returns message.ErrInvalidArg if the name is empty.
// Returns message.ErrInvalidArg if the proxies or extensions are empty.
// Returns message.ErrInvalidArg if a proxy or extension is values are empty.
func ValidateDepService(dep DepService) error {
	if len(dep.Name) == 0 {
		return fmt.Errorf("name argument is empty: %w", message.ErrInvalidArg)
	}
	if len(dep.Proxies) == 0 && len(dep.Extensions) == 0 {
		return fmt.Errorf("dep service('%s') must declare proxies or extensions: %w", dep.Name, message.ErrInvalidArg)
	}

	for i, link := range dep.Proxies {
		if link == "" {
			return fmt.Errorf("proxies[%d]: empty: %w", i, message.ErrInvalidArg)
		}
	}
	for i, link := range dep.Extensions {
		if link == "" {
			return fmt.Errorf("extensions[%d]: empty: %w", i, message.ErrInvalidArg)
		}
	}

	return nil
}
