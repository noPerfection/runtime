// Package config defines the noPerfection application configuration.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ahmetson/mushroom"
	"github.com/ahmetson/mushroom/substrates/json_substrate"
)

// ModuleRootMushroomPath addresses the entire JSON module document when mutating
// topology through Inoculate (pkg:$ with wildcards resolved from the colony).
const ModuleRootMushroomPath = "pkg:$#$"

// NoPerfection is the configuration of the entire application.
// Consists the supported services.
type NoPerfection struct {
	mycelium    *json_substrate.Mycelium
	mushroomURL string
	substrates  []mushroom.Substrate
}

// toHypha converts mushroomURL (link or dereference) into a full github.com/ahmetson/mushroom.Hypha
// wildcards are filled against the root mycelium.
// Plain symbols expand to *pkg:$?var=services[name:<symbol>].
func (a *NoPerfection) toHypha(mushroomURL string) (mushroom.Hypha, error) {
	if a == nil {
		return mushroom.Hypha{}, fmt.Errorf("app struct is nil")
	}
	if a.mycelium == nil {
		return mushroom.Hypha{}, fmt.Errorf("topology mycelium not set, call config.Load()")
	}
	if mushroomURL == "" {
		return mushroom.Hypha{}, fmt.Errorf("mushroom url is empty")
	}

	hypha, err := a.mycelium.Soil().Hypha(mushroomURL, a.mycelium.MyceliumURL())
	if err != nil {
		return mushroom.Hypha{}, fmt.Errorf("soil.Hypha(%q): %w", mushroomURL, err)
	}
	if !hypha.URL {
		hypha, err = a.mycelium.Soil().Hypha(
			fmt.Sprintf("*pkg:$?var=services[name:%s]", mushroomURL),
			a.mycelium.MyceliumURL(),
		)
		if err != nil {
			return mushroom.Hypha{}, fmt.Errorf("soil.Hypha(%q): %w", mushroomURL, err)
		}
	}

	return hypha, nil
}

func (a *NoPerfection) queryMycelium(mushroomURL string) (any, error) {
	if a == nil {
		return nil, fmt.Errorf("app struct is nil")
	}
	if a.mycelium == nil {
		return nil, fmt.Errorf("topology mycelium not set, call config.Load()")
	}

	spored, err := a.mycelium.Spore(mushroomURL)
	if err != nil {
		return nil, fmt.Errorf("mycelium.Spore(%q): %w", mushroomURL, err)
	}

	fruited, err := a.mycelium.Fruit(spored)
	if err != nil {
		return nil, fmt.Errorf("mycelium.Fruit: %w", err)
	}

	return fruited, nil
}

func (a *NoPerfection) decodeService(value any) (Service, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return Service{}, fmt.Errorf("value is not a service: %w", err)
	}

	var service Service
	if err := json.Unmarshal(data, &service); err != nil {
		return Service{}, fmt.Errorf("value is not a service: %w", err)
	}
	if service.Name == "" {
		return Service{}, fmt.Errorf("value is not a service: missing name")
	}

	if a != nil {
		service.noPerf = a
		service.mycelium = &a.mycelium
	}

	return service, nil
}

func (a *NoPerfection) decodeServices(value any) ([]Service, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("mushroom url is fetching wrong data: expected array, got %T", value)
	}

	services := make([]Service, 0, len(items))
	for _, item := range items {
		service, err := a.decodeService(item)
		if err != nil {
			return nil, fmt.Errorf("mushroom url is fetching wrong data: %w", err)
		}
		services = append(services, service)
	}

	return services, nil
}

func encodeServiceMap(record Service) (map[string]any, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal service: %w", err)
	}

	var serviceMap map[string]any
	if err := json.Unmarshal(data, &serviceMap); err != nil {
		return nil, fmt.Errorf("json.Unmarshal service map: %w", err)
	}

	return serviceMap, nil
}

func encodeHandlerMap(record Handler) (map[string]any, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal handler: %w", err)
	}

	var handlerMap map[string]any
	if err := json.Unmarshal(data, &handlerMap); err != nil {
		return nil, fmt.Errorf("json.Unmarshal handler map: %w", err)
	}

	return handlerMap, nil
}

// unwrapServiceValue normalizes a single mycelium result for decodeService.
// Filtered queries return an array; a direct path returns one object.
//
//	*pkg:$?var=services[name:auth_proxy]  →  [{...service...}]
//	unwrapServiceValue(...)                 →  {...service...}
func unwrapServiceValue(value any) (any, error) {
	items, ok := value.([]any)
	if !ok {
		return value, nil
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("service not found")
	}
	if len(items) > 1 {
		return nil, fmt.Errorf("multiple services matched")
	}
	return items[0], nil
}

func serviceExists(services []Service, name string) bool {
	for _, service := range services {
		if service.Name == name {
			return true
		}
	}
	return false
}

// Load loads an app configuration from a symbolic file path or a Mushroom link URL.
//
// Symbolic paths are plain filesystem paths ending in .json (e.g. "noPerfection.json",
// "/etc/app/noPerfection.json"). They are turned into a json mycelium link:
//
//	noPerfection.json  →  pkg:json/.#noPerfection.json
//
// Mushroom URLs must be links (not dereferences), use substrate type json, refer to a
// module (no ?var= resource path), and end with a .json module id
// (e.g. pkg:json/tmp#app.json).
//
// If the backing file does not exist, Load seeds an empty services list. When the file
// already exists, Load validates the topology graph.
//
// Optional substrates are registered in the json mycelium soil before germination.
// Topology does not define built-in substrates; callers (e.g. service) pass them in.
func Load(mushroomURL string, substrates ...mushroom.Substrate) (NoPerfection, error) {
	linkURL, filePath, err := resolveLoadMyceliumURL(mushroomURL)
	if err != nil {
		return NoPerfection{}, err
	}

	loaded := true
	if _, err := os.Stat(filePath); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
			return NoPerfection{}, fmt.Errorf("os.MkdirAll(%q): %w", filepath.Dir(filePath), err)
		}
		if err := os.WriteFile(filePath, []byte("{\n  \"services\": []\n}\n"), 0600); err != nil {
			return NoPerfection{}, fmt.Errorf("os.WriteFile('%s'): %w", filePath, err)
		}
		loaded = false
	} else if err != nil {
		return NoPerfection{}, fmt.Errorf("os.Stat('%s'): %w", filePath, err)
	}

	mycelium, err := json_substrate.Root(linkURL, substrates...)
	if err != nil {
		return NoPerfection{}, fmt.Errorf("json_substrate.Root(%q): %w", linkURL, err)
	}

	appConfig := NoPerfection{
		mycelium:    mycelium,
		mushroomURL: mushroomURL,
		substrates:  substrates,
	}

	if loaded {
		if err := appConfig.validateTopology("*pkg:$?var=services"); err != nil {
			return NoPerfection{}, fmt.Errorf("validateTopology: %w", err)
		}
	}

	return appConfig, nil
}

// Reload re-reads the topology from the JSON file on disk, replacing the
// in-memory state.  Call this after all IPC dep services have started so
// that any public keys they wrote directly to the file (when they could not
// reach this service's topology handler) become visible in memory.
func (a *NoPerfection) Reload() error {
	fresh, err := Load(a.mushroomURL, a.substrates...)
	if err != nil {
		return fmt.Errorf("Reload: %w", err)
	}
	a.mycelium = fresh.mycelium
	return nil
}

// resolveLoadMyceliumURL maps a Load argument to a json mycelium link and filesystem path.
func resolveLoadMyceliumURL(arg string) (linkURL string, filePath string, err error) {
	if arg == "" {
		return "", "", fmt.Errorf("mushroom url is empty")
	}

	if strings.HasPrefix(arg, "*pkg:") {
		return "", "", fmt.Errorf("Load mushroom URL %q must be a link, not a dereference", arg)
	}

	if strings.HasPrefix(arg, "pkg:") {
		soil := &mushroom.Soil{}
		hypha, parseErr := soil.Hypha(arg)
		if parseErr != nil {
			return "", "", fmt.Errorf("soil.Hypha(%q): %w", arg, parseErr)
		}
		if hypha.Dereference {
			return "", "", fmt.Errorf("Load mushroom URL %q must be a link, not a dereference", arg)
		}
		if hypha.Type != "json" {
			return "", "", fmt.Errorf("Load mushroom URL %q type must be json, got %q", arg, hypha.Type)
		}
		if hypha.ModuleID == "" {
			return "", "", fmt.Errorf("Load mushroom URL %q must include a module", arg)
		}
		if !strings.HasSuffix(hypha.ModuleID, ".json") {
			return "", "", fmt.Errorf("Load mushroom URL %q module must end with .json", arg)
		}
		if hypha.PackageID == "" {
			return "", "", fmt.Errorf("Load mushroom URL %q must include a package", arg)
		}
		if hypha.ResourceKind != "" {
			return "", "", fmt.Errorf("Load mushroom URL %q must refer to a module, not a resource path", arg)
		}
		return hypha.String(), filepath.Join(hypha.PackageID, hypha.ModuleID), nil
	}

	if !strings.HasSuffix(arg, ".json") {
		return "", "", fmt.Errorf("config file %q must end with .json", arg)
	}

	link := fmt.Sprintf("pkg:json/%s#%s", filepath.Dir(arg), filepath.Base(arg))
	return link, arg, nil
}

func (a *NoPerfection) validateTopology(servicesURL string) error {
	if a == nil {
		return fmt.Errorf("app struct is nil")
	}

	services, err := a.GetServices(servicesURL)
	if err != nil {
		return err
	}

	visiting := make(map[string]bool)
	for i := range services {
		if err := services[i].Validate(); err != nil {
			return fmt.Errorf("service %q: %w", services[i].Name, err)
		}
		if err := a.validateServiceTopology(services[i], visiting); err != nil {
			return fmt.Errorf("service %q: %w", services[i].Name, err)
		}
	}
	return nil
}

func (a *NoPerfection) validateServiceTopology(service Service, visiting map[string]bool) error {
	if service.Name != "" {
		if visiting[service.Name] {
			return fmt.Errorf("cycle detected at service %q", service.Name)
		}
		visiting[service.Name] = true
		defer delete(visiting, service.Name)
	}

	for _, dep := range service.HandlerDeps {
		for _, link := range dep.Proxies {
			if err := a.validateDepLink(link); err != nil {
				return fmt.Errorf("handler-deps category %q: proxy: %w", dep.Name, err)
			}
		}
		for _, link := range dep.Extensions {
			if err := a.validateDepLink(link); err != nil {
				return fmt.Errorf("handler-deps category %q: extension: %w", dep.Name, err)
			}
		}
	}
	for _, handler := range service.Handlers {
		baseHandler, ok := handler.AsIndependentHandler()
		if !ok {
			continue
		}
		for _, dep := range baseHandler.CommandDeps {
			for _, link := range dep.Proxies {
				if err := a.validateDepLink(link); err != nil {
					return fmt.Errorf("command %q: proxy: %w", dep.Name, err)
				}
			}
			for _, link := range dep.Extensions {
				if err := a.validateDepLink(link); err != nil {
					return fmt.Errorf("command %q: extension: %w", dep.Name, err)
				}
			}
		}
		if service.Type == ProxyType {
			proxyHandler, ok := handler.AsProxyHandler()
			if !ok {
				continue
			}
			if err := a.validateProxyHandler(proxyHandler); err != nil {
				return fmt.Errorf("handler %q: %w", baseHandler.Category, err)
			}
		}
		if service.Type == ExtensionType {
			extensionHandler, ok := handler.AsExtensionHandler()
			if !ok {
				continue
			}
			if err := a.validateExtensionHandler(extensionHandler); err != nil {
				return fmt.Errorf("handler %q: %w", baseHandler.Category, err)
			}
		}
	}
	return nil
}

func (a *NoPerfection) validateProxyHandler(proxyHandler ProxyHandler) error {
	for _, outbound := range proxyHandler.Outbounds {
		if _, err := a.GetHandler("*" + outbound); err != nil {
			return fmt.Errorf("outbound %q: %w", outbound, err)
		}
	}
	for route, outboundRef := range proxyHandler.Forward {
		if !slices.Contains(proxyHandler.Outbounds, outboundRef) {
			return fmt.Errorf("forward route %q: outbound %q is not listed in outbounds", route, outboundRef)
		}
	}
	return nil
}

func (a *NoPerfection) validateExtensionHandler(extensionHandler ExtensionHandler) error {
	for _, inbound := range extensionHandler.Inbounds {
		if _, err := a.GetHandler("*" + inbound); err != nil {
			return fmt.Errorf("inbound %q: %w", inbound, err)
		}
	}
	return nil
}

// ValidateInprocServiceManagers checks every registered service: if inproc, its manager must be inproc.
func (a *NoPerfection) ValidateInprocServiceManagers() error {
	if a == nil {
		return fmt.Errorf("app struct is nil")
	}
	services, err := a.GetServices("*pkg:$?var=services")
	if err != nil {
		return err
	}
	for _, service := range services {
		if err := service.ValidateInprocServiceManager(); err != nil {
			return err
		}
	}
	return nil
}

// InprocessDepNumber counts inproc services reachable from serviceConfig through
// handler-deps and command-deps.
func (a *NoPerfection) InprocessDepNumber(serviceConfig Service) (int, error) {
	if a == nil {
		return 0, fmt.Errorf("app struct is nil")
	}

	seen := make(map[string]struct{})
	count := 0

	for _, dep := range serviceConfig.HandlerDeps {
		n, err := a.countInprocDepService(dep, seen)
		if err != nil {
			return 0, fmt.Errorf("handler dep %q: %w", dep, err)
		}
		count += n
	}

	for _, variant := range serviceConfig.Handlers {
		handler, ok := variant.AsIndependentHandler()
		if !ok {
			continue
		}
		for _, dep := range handler.CommandDeps {
			n, err := a.countInprocDepService(dep, seen)
			if err != nil {
				return 0, fmt.Errorf("handler %q command %q: %w", handler.Category, dep.Name, err)
			}
			count += n
		}
	}

	return count, nil
}

func (a *NoPerfection) countInprocDepService(dep DepService, seen map[string]struct{}) (int, error) {
	count := 0
	for _, link := range dep.Proxies {
		n, err := a.countInprocDepLink(link, seen)
		if err != nil {
			return 0, err
		}
		count += n
	}
	for _, link := range dep.Extensions {
		n, err := a.countInprocDepLink(link, seen)
		if err != nil {
			return 0, err
		}
		count += n
	}
	return count, nil
}

func (a *NoPerfection) countInprocDepLink(link string, seen map[string]struct{}) (int, error) {
	service, _, err := a.ResolveDep(link)
	if err != nil {
		return 0, fmt.Errorf("%q: %w", link, err)
	}
	if !service.IsInproc() {
		return 0, nil
	}
	if _, ok := seen[service.Name]; ok {
		return 0, nil
	}
	seen[service.Name] = struct{}{}
	return 1, nil
}

// ValidateProtocolOrdersFor checks protocol forwarding rules starting from serviceConfig.
func (a *NoPerfection) ValidateProtocolOrdersFor(serviceConfig Service) error {
	if a == nil {
		return fmt.Errorf("app struct is nil")
	}
	if serviceConfig.Type == ProxyType {
		for _, variant := range serviceConfig.Handlers {
			proxyHandler, _ := variant.AsProxyHandler()
			if len(proxyHandler.Outbounds) == 0 {
				continue
			}

			for _, outboundURL := range proxyHandler.Outbounds {
				outboundService, outboundHandler, err := a.serviceAndHandlerFromDep(outboundURL)
				if err != nil {
					return fmt.Errorf("proxy %q handler %q outbound %q: %w", serviceConfig.Name, proxyHandler.Category, outboundURL, err)
				}
				if err := validateProtocolOrder(serviceConfig, variant, outboundService, outboundHandler); err != nil {
					return fmt.Errorf("proxy %q handler %q outbound %q: %w", serviceConfig.Name, proxyHandler.Category, outboundURL, err)
				}
			}
		}
	}

	for _, dep := range serviceConfig.HandlerDeps {
		for _, proxyURL := range dep.Proxies {
			proxyService, _, err := a.serviceAndHandlerFromDep(proxyURL)
			if err != nil {
				return fmt.Errorf("handler dep %q proxy %q: %w", dep.Name, proxyURL, err)
			}
			if err := a.ValidateProtocolOrdersFor(proxyService); err != nil {
				return fmt.Errorf("handler dep %q proxy %q: %w", dep.Name, proxyURL, err)
			}
		}

		for _, extensionURL := range dep.Extensions {
			extensionService, _, err := a.serviceAndHandlerFromDep(extensionURL)
			if err != nil {
				return fmt.Errorf("handler dep %q extension %q: %w", dep.Name, extensionURL, err)
			}
			if err := a.ValidateProtocolOrdersFor(extensionService); err != nil {
				return fmt.Errorf("handler dep %q extension %q: %w", dep.Name, extensionURL, err)
			}
		}
	}

	for _, variant := range serviceConfig.Handlers {
		handler, ok := variant.AsIndependentHandler()
		if !ok {
			continue
		}
		if handler.Category == ServiceManagerCategory || len(handler.CommandDeps) == 0 {
			continue
		}

		for _, dep := range handler.CommandDeps {
			for _, proxyURL := range dep.Proxies {
				proxyService, _, err := a.serviceAndHandlerFromDep(proxyURL)
				if err != nil {
					return fmt.Errorf("handler %q command %q proxy %q: %w", handler.Category, dep.Name, proxyURL, err)
				}
				if err := a.ValidateProtocolOrdersFor(proxyService); err != nil {
					return fmt.Errorf("handler %q command %q proxy %q: %w", handler.Category, dep.Name, proxyURL, err)
				}
			}
			for _, extensionURL := range dep.Extensions {
				extensionService, _, err := a.serviceAndHandlerFromDep(extensionURL)
				if err != nil {
					return fmt.Errorf("handler %q command %q extension %q: %w", handler.Category, dep.Name, extensionURL, err)
				}
				if err := a.ValidateProtocolOrdersFor(extensionService); err != nil {
					return fmt.Errorf("handler %q command %q extension %q: %w", handler.Category, dep.Name, extensionURL, err)
				}
			}
		}
	}

	return nil
}

func (a *NoPerfection) serviceAndHandlerFromDep(mushroomURL string) (Service, Handler, error) {
	service, category, err := a.ResolveDep(mushroomURL)
	if err != nil {
		return Service{}, nil, err
	}
	handler, err := service.HandlerByCategory(category)
	if err != nil {
		return Service{}, nil, err
	}
	return service, handler, nil
}

func validateProtocolOrder(callerService Service, caller Handler, outboundService Service, outbound Handler) error {
	callerHandler, ok := caller.AsIndependentHandler()
	if !ok {
		return fmt.Errorf("caller handler is not an independent handler")
	}
	outboundHandler, ok := outbound.AsIndependentHandler()
	if !ok {
		return fmt.Errorf("outbound handler is not an independent handler")
	}

	callerInproc, err := callerService.IsInprocHandler(callerHandler.Category)
	if err != nil {
		return err
	}
	if callerInproc {
		return nil
	}

	outboundInproc, err := outboundService.IsInprocHandler(outboundHandler.Category)
	if err != nil {
		return err
	}
	callerIpc, err := callerService.IsIpcHandler(callerHandler.Category)
	if err != nil {
		return err
	}
	callerProtocol := "tcp"
	if callerIpc {
		callerProtocol = "ipc"
	}
	outboundProtocol := "tcp"
	if outboundInproc {
		outboundProtocol = "inproc"
	} else {
		outboundIpc, err := outboundService.IsIpcHandler(outboundHandler.Category)
		if err != nil {
			return err
		}
		if outboundIpc {
			outboundProtocol = "ipc"
		}
	}

	if callerProtocol == "ipc" && !outboundInproc {
		return nil
	}
	if callerProtocol == "tcp" && outboundProtocol == "tcp" {
		return nil
	}
	return fmt.Errorf("can not access from %s to %s", callerProtocol, outboundProtocol)
}

func (a *NoPerfection) validateDepLink(link string) error {
	if _, _, err := a.ResolveDep(link); err != nil {
		return fmt.Errorf("dep link %q: %w", link, err)
	}
	return nil
}

// NoHandlerError reports that a resolved service does not contain the requested handler category.
type NoHandlerError struct {
	Service  string
	Category string
}

func (e *NoHandlerError) Error() string {
	return fmt.Sprintf("handler of %q category not found on service %q", e.Category, e.Service)
}

// ResolveDep resolves a dependency mushroom URL to a service and handler category.
// The URL may be a link or dereference pointing at a root service.
// When the URL carries an additional "category" property, that handler category
// is used; otherwise DefaultCategory is used.
func (a *NoPerfection) ResolveDep(mushroomURL string) (Service, string, error) {
	if a == nil {
		return Service{}, "", fmt.Errorf("app struct is nil")
	}
	if a.mycelium == nil {
		return Service{}, "", fmt.Errorf("topology mycelium not set, call config.Load()")
	}
	if mushroomURL == "" {
		return Service{}, "", fmt.Errorf("mushroom url is empty")
	}

	hypha, err := a.toHypha(mushroomURL)
	if err != nil {
		return Service{}, "", err
	}

	category := depCategory(hypha)
	hypha.AdditionalProps = nil

	service, err := a.GetService(hypha.AsDereference().String())
	if err != nil {
		return Service{}, "", err
	}

	// Make sure the category exists too.
	if _, err := service.HandlerByCategory(category); err != nil {
		return Service{}, "", &NoHandlerError{Service: service.Name, Category: category}
	}

	return service, category, nil
}

func depCategory(hypha mushroom.Hypha) string {
	if category, ok := hypha.AdditionalProps["category"]; ok && category != "" {
		return category
	}
	return DefaultCategory
}

func (a *NoPerfection) serviceNamedTargetURL(parent string, serviceName string) (string, error) {
	parentHypha, err := a.toHypha(parent)
	if err != nil {
		return "", err
	}
	targetHypha, err := parentHypha.ChildResource("[name:" + serviceName + "]")
	if err != nil {
		return "", fmt.Errorf("ChildResource(%q): %w", serviceName, err)
	}
	return targetHypha.AsDereference().String(), nil
}

// Snapshot returns the topology JSON document as a compact JSON string.
func (a NoPerfection) Snapshot() (string, error) {
	if a.mycelium == nil {
		return "", fmt.Errorf("topology mycelium not set, call config.Load()")
	}

	raw, err := a.mycelium.Mineralize()
	if err != nil {
		return "", fmt.Errorf("mycelium.Mineralize: %w", err)
	}

	jsonText, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("mycelium.Mineralize returned %T, want string", raw)
	}

	return jsonText, nil
}

// Rollback restores the topology from a prior Snapshot.
func (a NoPerfection) Rollback(snapshot string) error {
	if a.mycelium == nil {
		return fmt.Errorf("topology mycelium not set, call config.Load()")
	}
	if snapshot == "" {
		return fmt.Errorf("snapshot is empty")
	}

	if err := a.mycelium.Inoculate(ModuleRootMushroomPath, snapshot); err != nil {
		return fmt.Errorf("mycelium.Inoculate(%q): %w", ModuleRootMushroomPath, err)
	}

	return a.Save()
}

// Save saves the app configuration as JSON into its file path.
func (a NoPerfection) Save() error {
	if a.mycelium == nil {
		return fmt.Errorf("topology mycelium not set, call config.Load()")
	}

	raw, err := a.mycelium.Mineralize()
	if err != nil {
		return fmt.Errorf("mycelium.Mineralize: %w", err)
	}

	jsonText, ok := raw.(string)
	if !ok {
		return fmt.Errorf("mycelium.Mineralize returned %T, want string", raw)
	}

	substrate, ok := (*a.mycelium.Substrate()).(*json_substrate.Substrate)
	if !ok {
		return fmt.Errorf("mycelium substrate is %T, want *json_substrate.Substrate", *a.mycelium.Substrate())
	}

	hypha := a.mycelium.MyceliumURL()

	var indented bytes.Buffer
	if err := json.Indent(&indented, []byte(jsonText), "", "  "); err != nil {
		return fmt.Errorf("json.Indent: %w", err)
	}
	indented.WriteByte('\n')

	if err := substrate.Sow(hypha, indented.String()); err != nil {
		return fmt.Errorf("substrate.Sow: %w", err)
	}

	filePath := filepath.Join(hypha.PackageID, hypha.ModuleID)
	if err := os.Chmod(filePath, 0600); err != nil {
		return fmt.Errorf("os.Chmod('%s'): %w", filePath, err)
	}

	return nil
}

// GetServiceLink normalizes mushroomURL into a full Mushroom link.
// Plain service names expand to services[name:<name>]. Dereference URLs are
// converted to links. Resource paths and additional properties (e.g. category)
// are preserved; wildcards ($) and lambdas are resolved against the loaded mycelium.
// The resource is not verified to exist in the substrate.
func (a *NoPerfection) GetServiceLink(mushroomURL string) (string, error) {
	if a == nil {
		return "", fmt.Errorf("app struct is nil")
	}
	if a.mycelium == nil {
		return "", fmt.Errorf("topology mycelium not set, call config.Load()")
	}

	hypha, err := a.toHypha(mushroomURL)
	if err != nil {
		return "", err
	}

	return hypha.AsLink().String(), nil
}

// GetService resolves a Mushroom URL and returns a single service.
// Plain service names are resolved as *pkg:$?var=services[name:<name>].
func (a *NoPerfection) GetService(mushroomURL string) (Service, error) {
	hypha, err := a.toHypha(mushroomURL)
	if err != nil {
		return Service{}, err
	}

	fruited, err := a.queryMycelium(hypha.String())
	if err != nil {
		return Service{}, err
	}

	value, err := unwrapServiceValue(fruited)
	if err != nil {
		return Service{}, fmt.Errorf("GetService(%q): %w", mushroomURL, err)
	}

	service, err := a.decodeService(value)
	if err != nil {
		return Service{}, fmt.Errorf("GetService(%q): %w", mushroomURL, err)
	}

	hypha.AdditionalProps = nil
	service.mushroomURL = hypha.AsLink()

	return service, nil
}

// GetHandler resolves a Mushroom URL and returns a single handler.
// The URL may point at a handler path (…handlers[category:main]) or at a service
// with category in additional props (…services[name:x]&category=main). The path is
// queried as-is; when the result is a service, category comes from the hypha.
func (a *NoPerfection) GetHandler(mushroomURL string) (Handler, error) {
	fruited, err := a.queryMycelium(mushroomURL)
	if err != nil {
		return nil, err
	}

	handler, err := decodeHandler(fruited)
	if err == nil {
		return handler, nil
	}

	value, err := unwrapServiceValue(fruited)
	if err != nil {
		return nil, fmt.Errorf("GetHandler(%q): handler not found", mushroomURL)
	}

	service, err := a.decodeService(value)
	if err != nil {
		return nil, fmt.Errorf("GetHandler(%q): handler not found", mushroomURL)
	}

	hypha, _ := a.toHypha(mushroomURL) // since query succeed, mushroom url is a valid link
	category := depCategory(hypha)
	handler, err = service.HandlerByCategory(category)
	if err != nil {
		return nil, fmt.Errorf("GetHandler(%q): %w", mushroomURL, err)
	}

	return handler, nil
}

// GetFacade resolves a service by Mushroom URL and returns its facade link.
//
// mushroomURL may be a plain service name, a dereference Mushroom URL, or a link.
// Handler category is read from the URL additional property category (defaults to
// DefaultCategory when omitted). command is an optional second argument for the
// command route on that handler; resolution follows handler-deps and command-deps
// to return the facade for the dependency target (see Service.Facade).
//
// Examples (see config/examples/app-proxy-chain.json):
//
//	app.GetFacade("*pkg:$?var=services[name:main]&category=main", "authorize")
//	app.GetFacade("*pkg:$?var=services[name:user_service]&category=user-service")
func (a *NoPerfection) GetFacade(mushroomURL string, command ...string) (mushroom.Hypha, error) {
	service, err := a.GetService(mushroomURL)
	if err != nil {
		return mushroom.Hypha{}, fmt.Errorf("GetFacade(%q): %w", mushroomURL, err)
	}

	hypha, _ := a.toHypha(mushroomURL)
	category := depCategory(hypha)

	link, err := service.Facade(category, command...)
	if err != nil {
		return mushroom.Hypha{}, fmt.Errorf("GetFacade(%q): %w", mushroomURL, err)
	}

	return link, nil
}

// GetServices resolves a Mushroom URL and returns the services at that path.
func (a *NoPerfection) GetServices(mushroomURL string) ([]Service, error) {
	hypha, err := a.toHypha(mushroomURL)
	if err != nil {
		return nil, err
	}

	fruited, err := a.queryMycelium(hypha.String())
	if err != nil {
		return nil, err
	}

	services, err := a.decodeServices(fruited)
	if err != nil {
		return nil, fmt.Errorf("GetServices(%q): %w", mushroomURL, err)
	}

	listLink := hypha.AsLink()
	listLink.AdditionalProps = nil
	parent, _ := listLink.ParentResource()

	for i := range services {
		child, err := parent.ChildResource("[name:" + services[i].Name + "]")
		if err != nil {
			return nil, fmt.Errorf("GetServices(%q): service %q mushroom url: %w", mushroomURL, services[i].Name, err)
		}
		services[i].mushroomURL = child
	}

	return services, nil
}

// CountByType returns the number of services resolved by the Mushroom URL.
func (a *NoPerfection) CountByType(mushroomURL string) (int, error) {
	services, err := a.GetServices(mushroomURL)
	if err != nil {
		return 0, err
	}
	return len(services), nil
}

// AddService adds a new service into the services array at parent.
// parent is a dereference Mushroom URL, e.g. *pkg:$?var=services.
func (a *NoPerfection) AddService(record Service, parent string) error {
	if a == nil {
		return fmt.Errorf("app struct is nil")
	}
	if len(record.Name) == 0 {
		return fmt.Errorf("service name is empty")
	}
	if parent == "" {
		return fmt.Errorf("parent URL is empty")
	}

	services, err := a.GetServices(parent)
	if err != nil {
		return fmt.Errorf("GetServices(%q): %w", parent, err)
	}
	if serviceExists(services, record.Name) {
		return fmt.Errorf("service('%s') already exists", record.Name)
	}

	serviceMap, err := encodeServiceMap(record)
	if err != nil {
		return err
	}
	if err := a.mycelium.Graft(parent, serviceMap); err != nil {
		return fmt.Errorf("mycelium.Graft(%q): %w", parent, err)
	}

	return nil
}

// SetService updates an existing service in the services array at parent.
// parent is a dereference Mushroom URL, e.g. *pkg:$?var=services.
func (a *NoPerfection) SetService(record Service, parent string) error {
	if a == nil {
		return fmt.Errorf("app struct is nil")
	}
	if len(record.Name) == 0 {
		return fmt.Errorf("service name is empty")
	}
	if parent == "" {
		return fmt.Errorf("parent URL is empty")
	}

	services, err := a.GetServices(parent)
	if err != nil {
		return fmt.Errorf("GetServices(%q): %w", parent, err)
	}
	if !serviceExists(services, record.Name) {
		return fmt.Errorf("service('%s') not found", record.Name)
	}

	serviceMap, err := encodeServiceMap(record)
	if err != nil {
		return err
	}

	targetURL, err := a.serviceNamedTargetURL(parent, record.Name)
	if err != nil {
		return err
	}

	// Dereference restoration is handled by Inoculate itself (inoculateLocal reads
	// the current raw value, compares with Fruit-resolved values, and restores any
	// dereference strings the caller did not intentionally change).
	if err := a.mycelium.Inoculate(targetURL, serviceMap); err != nil {
		return fmt.Errorf("mycelium.Inoculate(%q): %w", targetURL, err)
	}

	return nil
}

// SetHandler updates an existing handler at mushroomURL.
// mushroomURL is a dereference Mushroom URL of the handler or of the service
// with category in additional props, e.g.
// *pkg:$?var=services[name:proxy].handlers[category:main] or
// *pkg:$?var=services[name:proxy]&category=main.
func (a *NoPerfection) SetHandler(record Handler, mushroomURL string) error {
	if a == nil {
		return fmt.Errorf("app struct is nil")
	}
	if record == nil {
		return fmt.Errorf("handler is empty")
	}
	base, ok := record.AsIndependentHandler()
	if !ok {
		return fmt.Errorf("handler is not an independent handler")
	}
	if base.Category == "" {
		return fmt.Errorf("handler category is empty")
	}
	if mushroomURL == "" {
		return fmt.Errorf("mushroom url is empty")
	}

	hypha, err := a.toHypha(mushroomURL)
	if err != nil {
		return fmt.Errorf("toHypha(%q): %w", mushroomURL, err)
	}
	if !hypha.URL {
		return fmt.Errorf("%q is not a mushroom URL", mushroomURL)
	}
	if !hypha.Dereference {
		return fmt.Errorf("%q must be a dereference mushroom URL", mushroomURL)
	}

	fruited, err := a.queryMycelium(mushroomURL)
	if err != nil {
		return fmt.Errorf("queryMycelium(%q): %w", mushroomURL, err)
	}

	var existing Handler
	var targetURL string

	if handler, err := decodeHandler(fruited); err == nil {
		existing = handler
		targetURL = hypha.String()
	} else {
		if _, err := unwrapServiceValue(fruited); err != nil {
			return fmt.Errorf("mushroomURL %q is neither a handler nor a service", mushroomURL)
		}
		service, err := a.GetService(mushroomURL)
		if err != nil {
			return fmt.Errorf("GetService(%q): %w", mushroomURL, err)
		}
		category := depCategory(hypha)
		existing, err = service.HandlerByCategory(category)
		if err != nil {
			return fmt.Errorf("handler of %q category not found on service %q: %w", category, service.Name, err)
		}
		handlersHypha, err := hypha.ChildResource("handlers")
		if err != nil {
			return fmt.Errorf("ChildResource(handlers): %w", err)
		}
		target, err := handlersHypha.ChildResource("[category:" + category + "]")
		if err != nil {
			return fmt.Errorf("ChildResource(%q): %w", category, err)
		}
		targetURL = target.AsDereference().String()
	}

	existingBase, ok := existing.AsIndependentHandler()
	if !ok {
		return fmt.Errorf("handler at %q is not an independent handler", mushroomURL)
	}
	if existingBase.Category != base.Category {
		return fmt.Errorf("handler category %q does not match record category %q", existingBase.Category, base.Category)
	}

	handlerMap, err := encodeHandlerMap(record)
	if err != nil {
		return err
	}

	if err := a.mycelium.Inoculate(targetURL, handlerMap); err != nil {
		return fmt.Errorf("mycelium.Inoculate(%q): %w", targetURL, err)
	}

	return nil
}

// RemoveService removes a service by name from the services array at parent.
// parent is a dereference Mushroom URL, e.g. *pkg:$?var=services.
func (a *NoPerfection) RemoveService(name, parent string) error {
	if a == nil {
		return fmt.Errorf("app struct is nil")
	}
	if len(name) == 0 {
		return fmt.Errorf("service name argument is empty")
	}
	if parent == "" {
		return fmt.Errorf("parent URL is empty")
	}

	services, err := a.GetServices(parent)
	if err != nil {
		return fmt.Errorf("GetServices(%q): %w", parent, err)
	}
	if !serviceExists(services, name) {
		return fmt.Errorf("service('%s') not found", name)
	}

	targetURL, err := a.serviceNamedTargetURL(parent, name)
	if err != nil {
		return err
	}
	if err := a.mycelium.Prune(targetURL); err != nil {
		return fmt.Errorf("mycelium.Prune(%q): %w", targetURL, err)
	}

	return nil
}
