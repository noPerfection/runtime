package config

import (
	"encoding/json"
	"testing"

	"github.com/noPerfection/protocol/message"
)

func TestServiceJSONProxyHandlers(t *testing.T) {
	service := Service{
		Type: ProxyType,
		Name: "auth_proxy",
		Handlers: []Handler{
			ProxyHandler{
				IndependentHandler: IndependentHandler{
					Type:     ReplierType,
					Category: "auth",
					Endpoint: message.NewEndpoint("auth_1", 4301),
				},
				Outbounds: []string{
					"pkg:$?var=services[name:user_service]&category=main",
				},
			},
		},
	}

	data, err := json.Marshal(service)
	if err != nil {
		t.Fatalf("Marshal proxy service: %v", err)
	}

	var roundTrip Service
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("Unmarshal proxy service: %v", err)
	}
	if roundTrip.Name != "auth_proxy" {
		t.Fatalf("Name = %q, want auth_proxy", roundTrip.Name)
	}
	if len(roundTrip.Handlers) != 1 {
		t.Fatalf("Proxy handlers = %#v, want one outbound", roundTrip.Handlers)
	}
	proxyHandler, ok := roundTrip.Handlers[0].AsProxyHandler()
	if !ok {
		t.Fatal("handler is not a ProxyHandler")
	}
	if len(proxyHandler.Outbounds) != 1 {
		t.Fatalf("Proxy handler outbounds = %#v, want one outbound", proxyHandler.Outbounds)
	}
	if proxyHandler.Outbounds[0] != "pkg:$?var=services[name:user_service]&category=main" {
		t.Fatalf("Outbound url = %q, want user_service facade url", proxyHandler.Outbounds[0])
	}
}

func TestServiceJSONExtensionHandlers(t *testing.T) {
	service := Service{
		Type: ExtensionType,
		Name: "user_extension",
		Handlers: []Handler{
			ExtensionHandler{
				IndependentHandler: IndependentHandler{
					Type:     ReplierType,
					Category: "main",
					Endpoint: message.NewEndpoint("inproc/user_extension", 0),
				},
				Inbounds: []string{
					"pkg:$?var=services[name:api]",
				},
			},
		},
	}

	data, err := json.Marshal(service)
	if err != nil {
		t.Fatalf("Marshal extension service: %v", err)
	}

	var roundTrip Service
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("Unmarshal extension service: %v", err)
	}
	extensionHandler, ok := roundTrip.Handlers[0].AsExtensionHandler()
	if !ok {
		t.Fatal("handler is not an ExtensionHandler")
	}
	if len(extensionHandler.Inbounds) != 1 {
		t.Fatalf("Extension handler inbounds = %#v, want one inbound", extensionHandler.Inbounds)
	}
	if extensionHandler.Inbounds[0] != "pkg:$?var=services[name:api]" {
		t.Fatalf("Inbound url = %q, want api service link", extensionHandler.Inbounds[0])
	}
}

func TestExtensionHandlerNullInboundsUnmarshal(t *testing.T) {
	handler, err := handlerFromMap(map[string]any{
		"type":     ReplierType,
		"category": "main",
		"endpoint": map[string]any{
			"id":   "ai",
			"port": float64(0),
		},
		"inbounds": nil,
	})
	if err != nil {
		t.Fatalf("handlerFromMap: %v", err)
	}
	extensionHandler, ok := handler.AsExtensionHandler()
	if !ok {
		t.Fatal("handler is not an ExtensionHandler")
	}
	if extensionHandler.Inbounds == nil {
		t.Fatal("Inbounds = nil, want empty slice")
	}
}
