package topology

import (
	"encoding/json"
	"fmt"

	"github.com/noPerfection/datatype"
	"github.com/noPerfection/protocol/client"
	"github.com/noPerfection/protocol/message"
	"github.com/noPerfection/topology/config"
)

type Client struct {
	*client.Socket
}

var (
	_ TopologyInterface = (*Client)(nil)
)

// NewClient connects to the topology handler endpoint.
func NewClient() (*Client, error) {
	endpoint := message.NewEndpoint(TopologyHandlerCategory, 0)
	socket, err := client.New(endpoint.Id, endpoint.Port, client.HandlerType(TopologySocketType))
	if err != nil {
		return nil, fmt.Errorf("client.New: %w", err)
	}

	return &Client{Socket: socket}, nil
}

// IsRunning checks whether the topology handler endpoint is available.
func (c *Client) IsRunning() (bool, error) {
	req := message.Request{
		Command:    IsRunning,
		Parameters: datatype.New(),
	}

	fmt.Println("> is running request for is-running")
	reply, err := c.Request(&req)
	if err != nil {
		return false, fmt.Errorf("socket.Request('%s'): %w", IsRunning, err)
	}

	if !reply.IsOK() {
		return false, fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	res, err := reply.ReplyParameters().BoolValue("running")
	if err != nil {
		return false, fmt.Errorf("reply.Parameters.GetBoolean('running'): %w", err)
	}

	return res, nil
}

// Service returns a service configuration by name or dereference Mushroom URL.
//
// Symbol:
//
//	svc, err := client.Service("auth_proxy")
//
// Dereference Mushroom URL:
//
//	svc, err := client.Service("*pkg:$?var=services[name:auth_proxy]")
func (c *Client) Service(mushroomURL string) (config.Service, error) {
	req := message.Request{
		Command: Service,
		Parameters: datatype.New().
			Set("service", mushroomURL),
	}

	reply, err := c.Request(&req)
	if err != nil {
		return config.Service{}, fmt.Errorf("socket.Request('%s'): %w", Service, err)
	}

	if !reply.IsOK() {
		return config.Service{}, fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	raw, err := reply.ReplyParameters().NestedValue("service")
	if err != nil {
		return config.Service{}, fmt.Errorf("reply.ReplyParameters().NestedValue('service'): %w", err)
	}

	var record config.Service
	if err := raw.Interface(&record); err != nil {
		return config.Service{}, fmt.Errorf("raw.Interface('config.Service'): %w", err)
	}

	return record, nil
}

// Handler returns a handler configuration resolved by dereference Mushroom URL.
//
// Dereference Mushroom URL:
//
//	h, err := client.Handler("*pkg:$?var=services[name:auth_proxy].handlers[category:main]")
func (c *Client) Handler(mushroomURL string) (config.Handler, error) {
	req := message.Request{
		Command: GetHandler,
		Parameters: datatype.New().
			Set("handler", mushroomURL),
	}

	reply, err := c.Request(&req)
	if err != nil {
		return nil, fmt.Errorf("socket.Request('%s'): %w", GetHandler, err)
	}

	if !reply.IsOK() {
		return nil, fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	raw, err := reply.ReplyParameters().Bytes()
	if err != nil {
		return nil, fmt.Errorf("reply.ReplyParameters().Bytes(): %w", err)
	}

	record, err := config.UnmarshalHandler(raw)
	if err != nil {
		return nil, fmt.Errorf("config.UnmarshalHandler: %w", err)
	}

	return record, nil
}

// GetFacade returns a facade Mushroom link for a service resolved by dereference URL.
//
// Handler category comes from the mushroom URL additional property category
// (defaults to DefaultCategory when omitted). command is an optional second
// argument for the command route; resolution follows handler-deps and
// command-deps to return the facade for a command handler and its dependency target.
//
// Dereference Mushroom URL:
//
//	link, err := client.GetFacade("*pkg:$?var=services[name:main]&category=main", "authorize")
func (c *Client) GetFacade(mushroomURL string, command ...string) (string, error) {
	params := datatype.New().Set("service", mushroomURL)
	if len(command) > 0 && command[0] != "" {
		params.Set("command", command[0])
	}

	req := message.Request{
		Command:    GetFacade,
		Parameters: params,
	}

	reply, err := c.Request(&req)
	if err != nil {
		return "", fmt.Errorf("socket.Request('%s'): %w", GetFacade, err)
	}

	if !reply.IsOK() {
		return "", fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	facade, err := reply.ReplyParameters().StringValue("facade")
	if err != nil {
		return "", fmt.Errorf("reply.ReplyParameters().StringValue('facade'): %w", err)
	}

	return facade, nil
}

// GetLink normalizes mushroomURL into a verified full Mushroom link.
//
// Symbol:
//
//	link, err := client.GetLink("auth_proxy")
//	  → "pkg:json/.#app.json?var=services[name:auth_proxy]"
//
// Dereference Mushroom URL:
//
//	link, err := client.GetLink("*pkg:$?var=services[name:auth_proxy]&category=main")
//	  → "pkg:json/./#app.json?var=services[name:auth_proxy]&category=main"
func (c *Client) GetLink(mushroomURL string) (string, error) {
	req := message.Request{
		Command: GetLink,
		Parameters: datatype.New().
			Set("link", mushroomURL),
	}

	reply, err := c.Request(&req)
	if err != nil {
		return "", fmt.Errorf("socket.Request('%s'): %w", GetLink, err)
	}

	if !reply.IsOK() {
		return "", fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	link, err := reply.ReplyParameters().StringValue("link")
	if err != nil {
		return "", fmt.Errorf("reply.ReplyParameters().StringValue('link'): %w", err)
	}

	return link, nil
}

// Services returns all service configurations.
func (c *Client) Services() ([]config.Service, error) {
	req := message.Request{
		Command:    Services,
		Parameters: datatype.New(),
	}

	reply, err := c.Request(&req)
	if err != nil {
		return nil, fmt.Errorf("socket.Request('%s'): %w", Services, err)
	}

	if !reply.IsOK() {
		return nil, fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	rawServices, err := reply.ReplyParameters().NestedListValue("services")
	if err != nil {
		return nil, fmt.Errorf("reply.ReplyParameters().NestedListValue('services'): %w", err)
	}

	records := make([]config.Service, 0, len(rawServices))
	for i, rawService := range rawServices {
		var record config.Service
		if err := rawService.Interface(&record); err != nil {
			return nil, fmt.Errorf("rawServices[%d].Interface('config.Service'): %w", i, err)
		}
		records = append(records, record)
	}

	return records, nil
}

// AddService registers a service in the topology configuration.
//
// parent is the dereference Mushroom URL of the array to append to. When omitted,
// the parent defaults to *pkg:$?var=services (the root services array).
//
//	err := client.AddService(record)
//
//	err := client.AddService(outbound, "*pkg:$?var=services[name:proxy].handlers[category:main].outbounds")
func (c *Client) AddService(record config.Service, parent ...string) error {
	params := datatype.New().Set("service", record)
	if len(parent) > 0 && parent[0] != "" {
		params.Set("parent", parent[0])
	}
	req := message.Request{
		Command:    AddService,
		Parameters: params,
	}

	reply, err := c.Request(&req)
	if err != nil {
		return fmt.Errorf("socket.Submit('%s'): %w", AddService, err)
	}

	if !reply.IsOK() {
		return fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	return nil
}

// SetService updates an existing service in the topology configuration.
//
// parent is the dereference Mushroom URL of the array that contains the service.
// When omitted, the parent defaults to *pkg:$?var=services.
//
//	err := client.SetService(record)
//
//	err := client.SetService(updated, "*pkg:$?var=services[name:proxy].handlers[category:main].outbounds")
func (c *Client) SetService(record config.Service, parent ...string) error {
	params := datatype.New().Set("service", record)
	if len(parent) > 0 && parent[0] != "" {
		params.Set("parent", parent[0])
	}
	req := message.Request{
		Command:    SetService,
		Parameters: params,
	}

	reply, err := c.Request(&req)
	if err != nil {
		return fmt.Errorf("socket.Submit('%s'): %w", SetService, err)
	}

	if !reply.IsOK() {
		return fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	return nil
}

// SetHandler updates an existing handler in the topology configuration.
//
//	mushroomURL is a dereference Mushroom URL of the handler or service with category:
//
//	err := client.SetHandler(record, "*pkg:$?var=services[name:proxy].handlers[category:main]")
//	err := client.SetHandler(record, "*pkg:$?var=services[name:proxy]&category=main")
func (c *Client) SetHandler(record config.Handler, mushroomURL string) error {
	if mushroomURL == "" {
		return fmt.Errorf("mushroom url is empty")
	}

	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("json.Marshal handler: %w", err)
	}

	params := datatype.New().
		Set("handler", json.RawMessage(raw)).
		Set("mushroomURL", mushroomURL)

	req := message.Request{
		Command:    SetHandler,
		Parameters: params,
	}

	reply, err := c.Request(&req)
	if err != nil {
		return fmt.Errorf("socket.Submit('%s'): %w", SetHandler, err)
	}

	if !reply.IsOK() {
		return fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	return nil
}

// RemoveService removes a service from the topology configuration.
//
// parent is the dereference Mushroom URL of the array to remove from. When omitted,
// the parent defaults to *pkg:$?var=services.
//
//	err := client.RemoveService("worker")
//
//	err := client.RemoveService("old_outbound", "*pkg:$?var=services[name:proxy].handlers[category:main].outbounds")
func (c *Client) RemoveService(name string, parent ...string) error {
	params := datatype.New().Set("service", name)
	if len(parent) > 0 && parent[0] != "" {
		params.Set("parent", parent[0])
	}
	req := message.Request{
		Command:    RemoveService,
		Parameters: params,
	}

	reply, err := c.Request(&req)
	if err != nil {
		return fmt.Errorf("socket.Submit('%s'): %w", RemoveService, err)
	}

	if !reply.IsOK() {
		return fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	return nil
}

// StartService starts the dependency service and returns the generated topology id.
//
// Symbol:
//
//	id, err := client.StartService("worker")
//
// Dereference Mushroom URL:
//
//	id, err := client.StartService("*pkg:$?var=services[name:worker]")
func (c *Client) StartService(mushroomURL string) (string, error) {
	parameters := datatype.New().Set("service", mushroomURL)

	req := message.Request{
		Command:    StartService,
		Parameters: parameters,
	}

	reply, err := c.Request(&req)
	if err != nil {
		return "", fmt.Errorf("socket.Submit('%s'): %w", StartService, err)
	}

	if !reply.IsOK() {
		return "", fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	id, err := reply.ReplyParameters().StringValue("id")
	if err != nil {
		return "", fmt.Errorf("reply.Parameters.GetString('id'): %w", err)
	}

	return id, nil
}

// StopService stops the running dependency service.
//
// Symbol:
//
//	err := client.StopService("worker")
//
// Dereference Mushroom URL:
//
//	err := client.StopService("*pkg:$?var=services[name:worker]")
func (c *Client) StopService(mushroomURL string) error {
	req := message.Request{
		Command: StopService,
		Parameters: datatype.New().
			Set("service", mushroomURL),
	}

	if c == nil {
		return fmt.Errorf("dep manager not initialized")
	}

	if c.Socket == nil {
		return fmt.Errorf("dep manager socket was closed")
	}

	reply, err := c.Request(&req)
	if err != nil {
		return fmt.Errorf("socket.Submit('%s'): %w", StopService, err)
	}

	if !reply.IsOK() {
		return fmt.Errorf("c.socket.Requeset(request='%v'): reply failed with: %s", req, reply.ErrorMessage())
	}

	return nil
}

// ValidateProtocolOrder checks protocol forwarding rules for a service and its
// reachable dependency graph.
//
// Allowed forwarding (caller handler transport → outbound):
//
//	Caller → TCP  → IPC  → inproc
//	inproc   ✅     ✅     ✅
//	ipc      ✅     ✅     ❌
//	tcp      ✅     ❌     ❌
func (c *Client) ValidateProtocolOrder(mushroomURL string) error {
	req := message.Request{
		Command: ValidateProtocolOrder,
		Parameters: datatype.New().
			Set("service", mushroomURL),
	}

	reply, err := c.Request(&req)
	if err != nil {
		return fmt.Errorf("socket.Request('%s'): %w", ValidateProtocolOrder, err)
	}

	if !reply.IsOK() {
		return fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	return nil
}

// ValidateInprocServiceManagers checks every registered service: if inproc, its manager must be inproc.
func (c *Client) ValidateInprocServiceManagers() error {
	req := message.Request{
		Command:    ValidateInprocServiceManagers,
		Parameters: datatype.New(),
	}

	reply, err := c.Request(&req)
	if err != nil {
		return fmt.Errorf("socket.Request('%s'): %w", ValidateInprocServiceManagers, err)
	}

	if !reply.IsOK() {
		return fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	return nil
}

// InprocessDepNumber counts inproc dependency services for the given service.
func (c *Client) InprocessDepNumber(mushroomURL string) (int, error) {
	req := message.Request{
		Command: InprocessDepNumber,
		Parameters: datatype.New().
			Set("service", mushroomURL),
	}

	reply, err := c.Request(&req)
	if err != nil {
		return 0, fmt.Errorf("socket.Request('%s'): %w", InprocessDepNumber, err)
	}

	if !reply.IsOK() {
		return 0, fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	count, err := reply.ReplyParameters().Uint64Value("count")
	if err != nil {
		return 0, fmt.Errorf("reply.Parameters.Uint64Value('count'): %w", err)
	}

	return int(count), nil
}

// Snapshot returns the topology JSON document as a compact JSON string.
func (c *Client) Snapshot() (string, error) {
	req := message.Request{
		Command:    Snapshot,
		Parameters: datatype.New(),
	}

	reply, err := c.Request(&req)
	if err != nil {
		return "", fmt.Errorf("socket.Request('%s'): %w", Snapshot, err)
	}

	if !reply.IsOK() {
		return "", fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	snapshot, err := reply.ReplyParameters().StringValue("snapshot")
	if err != nil {
		return "", fmt.Errorf("reply.Parameters.StringValue('snapshot'): %w", err)
	}

	return snapshot, nil
}

// Rollback restores the topology from a prior Snapshot.
func (c *Client) Rollback(snapshot string) error {
	req := message.Request{
		Command: Rollback,
		Parameters: datatype.New().
			Set("snapshot", snapshot),
	}

	reply, err := c.Request(&req)
	if err != nil {
		return fmt.Errorf("socket.Request('%s'): %w", Rollback, err)
	}

	if !reply.IsOK() {
		return fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	return nil
}

// Reload re-reads the topology JSON file from disk.
func (c *Client) Reload() error {
	req := message.Request{
		Command:    Reload,
		Parameters: datatype.New(),
	}

	reply, err := c.Request(&req)
	if err != nil {
		return fmt.Errorf("socket.Request('%s'): %w", Reload, err)
	}

	if !reply.IsOK() {
		return fmt.Errorf("reply.Message: %s", reply.ErrorMessage())
	}

	return nil
}
