package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/noPerfection/datatype"
	"github.com/noPerfection/protocol/message"
)

func testService() (*Service, IndependentHandler, IndependentHandler, IndependentHandler) {
	handlerOfType := IndependentHandler{
		Type:     ReplierType,
		Category: "public",
		Endpoint: message.NewEndpoint("handler_1", 4101),
	}
	handler2OfType := IndependentHandler{
		Type:     ReplierType,
		Category: "internal",
		Endpoint: message.NewEndpoint("handler_2", 4102),
	}
	handlerOfType2 := IndependentHandler{
		Type:     SyncReplierType,
		Category: "sync",
		Endpoint: message.NewEndpoint("handler_3", 4103),
	}

	return &Service{
		Type:     IndependentType,
		Name:     "service_id",
		Handlers: make([]Handler, 0),
	}, handlerOfType, handler2OfType, handlerOfType2
}

func TestServiceValidate(t *testing.T) {
	_, handlerOfType, _, _ := testService()

	invalidHandler := IndependentHandler{Type: HandlerType("invalid_handler_type")}

	generatedService := &Service{
		Name:     "generated",
		Type:     "the_invalid_type",
		Handlers: []Handler{handlerOfType},
	}

	if err := generatedService.Validate(); err == nil {
		t.Fatal("ValidateService with invalid service type returned nil error")
	}

	generatedService.Type = IndependentType
	if err := generatedService.Validate(); err != nil {
		t.Fatalf("ValidateService valid service: %v", err)
	}

	generatedService.Handlers = []Handler{IndependentHandler{Type: ReplierType}}
	if err := generatedService.Validate(); err == nil {
		t.Fatal("ValidateService with empty handler category returned nil error")
	}

	generatedService.Handlers = []Handler{invalidHandler}
	if err := generatedService.Validate(); err == nil {
		t.Fatal("ValidateService with invalid handler type returned nil error")
	}
}

func TestServiceValidateSocketBootstrap(t *testing.T) {
	service := Service{
		Type: ProxyType,
		Name: "inproc-service",
		Handlers: []Handler{
			ProxyHandler{
				IndependentHandler: IndependentHandler{
					Type:     ReplierType,
					Category: "inproc",
					Endpoint: message.NewEndpoint("inproc-handler", 0),
				},
			},
		},
	}
	if err := service.Validate(); err == nil {
		t.Fatal("ValidateService with inproc endpoint and no module-url returned nil error")
	}

	service.ModuleUrl = "github.com/noPerfection/inproc-service"
	if err := service.Validate(); err != nil {
		t.Fatalf("ValidateService with module-url: %v", err)
	}

	service = Service{
		Type: ProxyType,
		Name: "tmp-service",
		Handlers: []Handler{
			ProxyHandler{
				IndependentHandler: IndependentHandler{
					Type:     ReplierType,
					Category: "tmp",
					Endpoint: message.NewEndpoint("tmp/service.sock", 0),
				},
			},
		},
	}
	if err := service.Validate(); err == nil {
		t.Fatal("ValidateService with ipc endpoint and no start-command returned nil error")
	}

	service.StartCommand = "go run ./cmd/tmp-service"
	if err := service.Validate(); err != nil {
		t.Fatalf("ValidateService with start-command: %v", err)
	}

	service = Service{
		Type: ProxyType,
		Name: "tcp-service",
		Handlers: []Handler{
			ProxyHandler{
				IndependentHandler: IndependentHandler{
					Type:     ReplierType,
					Category: "tcp",
					Endpoint: message.NewEndpoint("tcp-service", 4101),
				},
			},
		},
	}
	if err := service.Validate(); err != nil {
		t.Fatalf("ValidateService with tcp endpoint and no bootstrap fields: %v", err)
	}
}

func TestServiceIsIpc(t *testing.T) {
	service := Service{
		Name: "proxy",
		Type: ProxyType,
		Handlers: []Handler{
			IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("tmp/proxy", 0),
			},
			IndependentHandler{
				Category: "manager",
				Endpoint: message.NewEndpoint("tmp/proxy_manager", 0),
			},
		},
	}
	if !service.IsIpc() {
		t.Fatal("Service.IsIpc with IPC handler returned false")
	}
}

func TestServiceIsIpcSkipsManager(t *testing.T) {
	service := Service{
		Name: "proxy",
		Handlers: []Handler{
			IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("localhost", 8000),
			},
			IndependentHandler{
				Category: ServiceManagerCategory,
				Endpoint: message.NewEndpoint("tmp/manager", 0),
			},
		},
	}
	if service.IsIpc() {
		t.Fatal("Service.IsIpc returned true when only manager handler is IPC")
	}
}

func TestServiceIsIpcMainIpcManagerInproc(t *testing.T) {
	service := Service{
		Name: "proxy",
		Handlers: []Handler{
			IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("tmp/proxy", 0),
			},
			IndependentHandler{
				Category: ServiceManagerCategory,
				Endpoint: message.NewEndpoint("inproc/proxy_manager", 0),
			},
		},
	}
	if service.IsInproc() {
		t.Fatal("Service.IsInproc returned true when only manager handler is inproc")
	}
	if !service.IsIpc() {
		t.Fatal("Service.IsIpc returned false for main IPC handler with inproc manager")
	}
}

func TestServiceIsIpcRemoteHandler(t *testing.T) {
	service := Service{
		Handlers: []Handler{IndependentHandler{
			Category: "main",
			Endpoint: message.NewEndpoint("localhost", 8000),
		}},
	}
	if service.IsIpc() {
		t.Fatal("Service.IsIpc returned true for remote handler")
	}
}

func TestServiceIsInproc(t *testing.T) {
	service := Service{
		Name: "extension",
		Handlers: []Handler{
			IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("inproc/extension", 0),
			},
		},
	}
	if !service.IsInproc() {
		t.Fatal("Service.IsInproc with inproc handler returned false")
	}
}

func TestServiceIsInprocSkipsManager(t *testing.T) {
	service := Service{
		Name: "extension",
		Handlers: []Handler{
			IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("localhost", 8000),
			},
			IndependentHandler{
				Category: ServiceManagerCategory,
				Endpoint: message.NewEndpoint("inproc/extension_manager", 0),
			},
		},
	}
	if service.IsInproc() {
		t.Fatal("Service.IsInproc returned true when only manager handler is inproc")
	}
}

func TestServiceIsIpcFalseWhenMainInproc(t *testing.T) {
	service := Service{
		Name: "extension",
		Handlers: []Handler{
			IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("inproc/extension", 0),
			},
			IndependentHandler{
				Category: ServiceManagerCategory,
				Endpoint: message.NewEndpoint("tmp/manager", 0),
			},
		},
	}
	if !service.IsInproc() {
		t.Fatal("Service.IsInproc returned false for main inproc handler")
	}
	if service.IsIpc() {
		t.Fatal("Service.IsIpc returned true when main handler is inproc")
	}
}

func TestServiceIsInprocRemoteHandler(t *testing.T) {
	service := Service{
		Handlers: []Handler{IndependentHandler{
			Category: "main",
			Endpoint: message.NewEndpoint("localhost", 8000),
		}},
	}
	if service.IsInproc() {
		t.Fatal("Service.IsInproc returned true for remote handler")
	}
}

func TestServiceIsInprocHandler(t *testing.T) {
	t.Run("missing handler", func(t *testing.T) {
		service := Service{Name: "proxy", Type: ProxyType}
		if _, err := service.IsInprocHandler("main"); err == nil {
			t.Fatal("IsInprocHandler with missing handler returned nil error")
		}
	})

	t.Run("inproc endpoint", func(t *testing.T) {
		service := Service{
			Name: "extension",
			Type: ExtensionType,
			Handlers: []Handler{IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("inproc/extension", 0),
			}},
		}
		inproc, err := service.IsInprocHandler("main")
		if err != nil {
			t.Fatalf("IsInprocHandler: %v", err)
		}
		if !inproc {
			t.Fatal("IsInprocHandler returned false for inproc endpoint")
		}
	})

	t.Run("proxy parameter override", func(t *testing.T) {
		service := Service{
			Name:       "proxy",
			Type:       ProxyType,
			Parameters: datatype.New().Set(InprocHandlersParameter, []string{"main"}),
			Handlers: []Handler{IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("tmp/proxy", 0),
			}},
		}
		inproc, err := service.IsInprocHandler("main")
		if err != nil {
			t.Fatalf("IsInprocHandler: %v", err)
		}
		if !inproc {
			t.Fatal("IsInprocHandler returned false for handler listed in inproc-handlers")
		}
	})

	t.Run("independent parameter override", func(t *testing.T) {
		service := Service{
			Name:       "service",
			Type:       IndependentType,
			Parameters: datatype.New().Set(InprocHandlersParameter, []string{"main"}),
			Handlers: []Handler{IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("tmp/service", 0),
			}},
		}
		inproc, err := service.IsInprocHandler("main")
		if err != nil {
			t.Fatalf("IsInprocHandler: %v", err)
		}
		if !inproc {
			t.Fatal("IsInprocHandler returned false for handler listed in inproc-handlers on Independent")
		}
	})
}

func TestServiceIsIpcHandler(t *testing.T) {
	t.Run("missing handler", func(t *testing.T) {
		service := Service{Name: "proxy", Type: ProxyType}
		if _, err := service.IsIpcHandler("main"); err == nil {
			t.Fatal("IsIpcHandler with missing handler returned nil error")
		}
	})

	t.Run("ipc endpoint", func(t *testing.T) {
		service := Service{
			Name: "proxy",
			Type: ProxyType,
			Handlers: []Handler{IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("tmp/proxy", 0),
			}},
		}
		ipc, err := service.IsIpcHandler("main")
		if err != nil {
			t.Fatalf("IsIpcHandler: %v", err)
		}
		if !ipc {
			t.Fatal("IsIpcHandler returned false for ipc endpoint")
		}
	})

	t.Run("proxy parameter override", func(t *testing.T) {
		service := Service{
			Name:       "proxy",
			Type:       ProxyType,
			Parameters: datatype.New().Set(IpcHandlersParameter, []string{"main"}),
			Handlers: []Handler{IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("proxy", 4101),
			}},
		}
		ipc, err := service.IsIpcHandler("main")
		if err != nil {
			t.Fatalf("IsIpcHandler: %v", err)
		}
		if !ipc {
			t.Fatal("IsIpcHandler returned false for handler listed in ipc-handlers")
		}
	})

	t.Run("independent parameter override", func(t *testing.T) {
		service := Service{
			Name:       "service",
			Type:       IndependentType,
			Parameters: datatype.New().Set(IpcHandlersParameter, []string{"main"}),
			Handlers: []Handler{IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("service", 4101),
			}},
		}
		ipc, err := service.IsIpcHandler("main")
		if err != nil {
			t.Fatalf("IsIpcHandler: %v", err)
		}
		if !ipc {
			t.Fatal("IsIpcHandler returned false for handler listed in ipc-handlers on Independent")
		}
	})

	t.Run("inproc parameter takes precedence", func(t *testing.T) {
		service := Service{
			Name: "proxy",
			Type: ProxyType,
			Parameters: datatype.New().
				Set(InprocHandlersParameter, []string{"main"}).
				Set(IpcHandlersParameter, []string{"main"}),
			Handlers: []Handler{IndependentHandler{
				Category: "main",
				Endpoint: message.NewEndpoint("proxy", 4101),
			}},
		}
		ipc, err := service.IsIpcHandler("main")
		if err != nil {
			t.Fatalf("IsIpcHandler: %v", err)
		}
		if ipc {
			t.Fatal("IsIpcHandler returned true when handler is listed in inproc-handlers")
		}
	})
}

func TestServiceIsIpcWithIpcHandlersParameter(t *testing.T) {
	service := Service{
		Name:       "proxy",
		Type:       ProxyType,
		Parameters: datatype.New().Set(IpcHandlersParameter, []string{"main"}),
		Handlers: []Handler{IndependentHandler{
			Category: "main",
			Endpoint: message.NewEndpoint("proxy", 4101),
		}},
	}
	if !service.IsIpc() {
		t.Fatal("Service.IsIpc returned false for TCP handler listed in ipc-handlers")
	}
}

func TestServiceValidateIpcHandlersRequiresStartCommand(t *testing.T) {
	service := Service{
		Type: ProxyType,
		Name: "tcp-ipc-proxy",
		Parameters: datatype.New().Set(IpcHandlersParameter, []string{"main"}),
		Handlers: []Handler{
			ProxyHandler{
				IndependentHandler: IndependentHandler{
					Type:     ReplierType,
					Category: "main",
					Endpoint: message.NewEndpoint("tcp-ipc-proxy", 4101),
				},
			},
		},
	}
	if err := service.Validate(); err == nil {
		t.Fatal("ValidateService with ipc-handlers and no start-command returned nil error")
	}

	service.StartCommand = "go run ./cmd/tcp-ipc-proxy"
	if err := service.Validate(); err != nil {
		t.Fatalf("ValidateService with ipc-handlers and start-command: %v", err)
	}
}

func TestServiceValidateInprocHandlersRequiresModuleURL(t *testing.T) {
	service := Service{
		Type:      IndependentType,
		Name:      "tcp-inproc-service",
		Parameters: datatype.New().Set(InprocHandlersParameter, []string{"main"}),
		Handlers: []Handler{IndependentHandler{
			Type:     ReplierType,
			Category: "main",
			Endpoint: message.NewEndpoint("tcp-inproc-service", 4101),
		}},
	}
	if err := service.Validate(); err == nil {
		t.Fatal("ValidateService with inproc-handlers and no module-url returned nil error")
	}

	service.ModuleUrl = "github.com/noPerfection/tcp-inproc-service"
	if err := service.Validate(); err != nil {
		t.Fatalf("ValidateService with inproc-handlers and module-url: %v", err)
	}
}

func TestValidateDepService(t *testing.T) {
	if err := ValidateDepService(DepService{Name: "orphan"}); err == nil {
		t.Fatal("ValidateDepService without proxies or extensions returned nil error")
	}

	if err := ValidateDepService(DepService{
		Name:    "call-user-api",
		Proxies: []string{"pkg:$?var=services[name:auth_proxy]"},
	}); err != nil {
		t.Fatalf("ValidateDepService with proxies: %v", err)
	}

	if err := ValidateDepService(DepService{
		Name:       "get-user",
		Extensions: []string{"pkg:$?var=services[name:user_service]"},
	}); err != nil {
		t.Fatalf("ValidateDepService with extensions: %v", err)
	}
}

func TestValidateProxyForwards(t *testing.T) {
	defaultProxyOutbound := "pkg:$?var=services[name:default-name-proxy]&category=main"
	helloWorldOutbound := "pkg:$?var=services[name:hello-world]&category=main"
	proxyHandler := ProxyHandler{
		IndependentHandler: IndependentHandler{
			Type:     SyncReplierType,
			Category: "main",
			Endpoint: message.NewEndpoint("proxy", 4101),
		},
		Routes: []string{"hello", "age-verification"},
		Outbounds: []string{
			defaultProxyOutbound,
			helloWorldOutbound,
		},
		Forward: map[string]string{
			"hello":            defaultProxyOutbound,
			"age-verification": helloWorldOutbound,
		},
	}
	service := Service{
		Type:      ProxyType,
		Name:      "proxy",
		ModuleUrl: "github.com/noPerfection/proxy",
		Handlers:  []Handler{proxyHandler},
	}
	if err := service.Validate(); err != nil {
		t.Fatalf("ValidateService with forward mappings: %v", err)
	}

	proxyHandler.Forward = map[string]string{"missing-route": helloWorldOutbound}
	service.Handlers = []Handler{proxyHandler}
	if err := service.Validate(); err == nil {
		t.Fatal("ValidateService with forward route missing from routes returned nil error")
	}

	proxyHandler.Forward = map[string]string{"hello": "pkg:$?var=services[name:missing-service]&category=main"}
	service.Handlers = []Handler{proxyHandler}
	_, err := loadServices(t, []Service{
		{
			Type: ProxyType,
			Name: "default-name-proxy",
			Handlers: []Handler{ProxyHandler{
				IndependentHandler: IndependentHandler{
					Type:     SyncReplierType,
					Category: "main",
					Endpoint: message.NewEndpoint("tmp/default_name_proxy", 0),
				},
			}},
		},
		{
			Type:      IndependentType,
			Name:      "hello-world",
			ModuleUrl: "github.com/noPerfection/hello-world",
			Handlers: []Handler{IndependentHandler{
				Type:     ReplierType,
				Category: "main",
				Endpoint: message.NewEndpoint("hello-world", 4102),
			}},
		},
		service,
	})
	if err == nil {
		t.Fatal("Load with forward outbound missing from outbounds returned nil error")
	}
}

func TestProxyHandlerUnmarshalForwardOnly(t *testing.T) {
	data := []byte(`{
		"type": "SyncReplier",
		"category": "main",
		"endpoint": {"id": "proxy", "port": 4101},
		"forward": {"hello": "pkg:$?var=services[name:hello-world]&category=main"},
		"outbounds": ["pkg:$?var=services[name:hello-world]&category=main"]
	}`)

	handler, err := UnmarshalHandler(data)
	if err != nil {
		t.Fatalf("json.Unmarshal proxy handler with forward: %v", err)
	}
	proxyHandler, ok := handler.AsProxyHandler()
	if !ok {
		t.Fatal("handler is not a ProxyHandler")
	}
	if len(proxyHandler.Forward) != 1 || proxyHandler.Forward["hello"] != "pkg:$?var=services[name:hello-world]&category=main" {
		t.Fatalf("Forward = %#v, want hello mapping", proxyHandler.Forward)
	}
}

func TestServiceEqual(t *testing.T) {
	managerA := message.NewEndpoint("svc_manager", 0)
	managerB := message.NewEndpoint("other_manager", 0)

	base := Service{
		Type: IndependentType,
		Name: "worker",
		Handlers: []Handler{
			IndependentHandler{
				Type:     SyncReplierType,
				Category: ServiceManagerCategory,
				Endpoint: managerA,
			},
		},
	}
	sameManager := Service{
		Type: IndependentType,
		Name: "worker",
		Handlers: []Handler{
			IndependentHandler{
				Type:     ReplierType,
				Category: ServiceManagerCategory,
				Endpoint: managerA,
			},
			IndependentHandler{
				Type:     ReplierType,
				Category: "api",
				Endpoint: message.NewEndpoint("api", 4101),
			},
		},
	}

	if !base.Equal(sameManager) {
		t.Fatal("Equal returned false for same name and manager endpoint")
	}

	differentName := base
	differentName.Name = "other"
	if base.Equal(differentName) {
		t.Fatal("Equal returned true for different names")
	}

	differentManager := base
	differentManager.Handlers = []Handler{
		IndependentHandler{
			Type:     SyncReplierType,
			Category: ServiceManagerCategory,
			Endpoint: managerB,
		},
	}
	if base.Equal(differentManager) {
		t.Fatal("Equal returned true for different manager endpoints")
	}

	withoutManager := Service{Name: "worker"}
	if base.Equal(withoutManager) {
		t.Fatal("Equal returned true when only one service has a manager")
	}

	bothWithoutManager := Service{Name: "worker"}
	if !withoutManager.Equal(bothWithoutManager) {
		t.Fatal("Equal returned false when neither service has a manager")
	}
}

func TestServiceEqualHandlers(t *testing.T) {
	manager := IndependentHandler{
		Type:     SyncReplierType,
		Category: ServiceManagerCategory,
		Endpoint: message.NewEndpoint("manager", 0),
	}
	api := IndependentHandler{
		Type:     ReplierType,
		Category: "api",
		Endpoint: message.NewEndpoint("api", 4101),
	}
	web := IndependentHandler{
		Type:     ReplierType,
		Category: "web",
		Endpoint: message.NewEndpoint("web", 4102),
	}

	base := Service{
		Name:     "worker",
		Handlers: []Handler{manager, api},
	}
	sameHandlers := Service{
		Name:     "worker",
		Handlers: []Handler{manager, IndependentHandler(api)},
	}
	if !base.EqualHandlers(sameHandlers) {
		t.Fatal("EqualHandlers returned false for same non-manager handlers")
	}

	differentEndpoint := Service{
		Name:     "worker",
		Handlers: []Handler{manager, IndependentHandler{Type: api.Type, Category: api.Category, Endpoint: message.NewEndpoint("api", 9999)}},
	}
	if base.EqualHandlers(differentEndpoint) {
		t.Fatal("EqualHandlers returned true for different handler endpoints")
	}

	differentCategory := Service{
		Name:     "worker",
		Handlers: []Handler{manager, web},
	}
	if base.EqualHandlers(differentCategory) {
		t.Fatal("EqualHandlers returned true for different handler categories")
	}

	ignoresManagerOnly := Service{
		Name:     "worker",
		Handlers: []Handler{IndependentHandler{
			Type:     SyncReplierType,
			Category: ServiceManagerCategory,
			Endpoint: message.NewEndpoint("other-manager", 1),
		}, api},
	}
	if !base.EqualHandlers(ignoresManagerOnly) {
		t.Fatal("EqualHandlers should ignore ServiceManagerCategory handlers")
	}
}

func TestProxyHandlerSetOutbound(t *testing.T) {
	existing := "pkg:$?var=services[name:custom-service]&category=api"
	proxy := ProxyHandler{
		Outbounds: []string{existing},
	}
	updated := "pkg:$?var=services[name:custom-service]&category=web"

	if proxy.SetOutbound(existing) {
		t.Fatal("SetOutbound returned true, want false when already set")
	}

	if !proxy.SetOutbound(updated) {
		t.Fatal("SetOutbound returned false, want true for append")
	}
	if len(proxy.Outbounds) != 2 {
		t.Fatalf("len(Outbounds) = %d, want 2", len(proxy.Outbounds))
	}
	if proxy.Outbounds[1] != updated {
		t.Fatalf("Outbounds[1] = %q, want %q", proxy.Outbounds[1], updated)
	}
}

func TestServiceParametersNotValidated(t *testing.T) {
	serviceConfig, handlerOfType, _, _ := testService()
	serviceConfig.Handlers = []Handler{handlerOfType}
	serviceConfig.Parameters = datatype.New().Set("region", "eu-west")

	if err := serviceConfig.Validate(); err != nil {
		t.Fatalf("ValidateService with parameters: %v", err)
	}
}

func TestServiceParametersJSONRoundTrip(t *testing.T) {
	data := []byte(`{
		"type": "Proxy",
		"name": "worker",
		"parameters": {
			"region": "eu-west",
			"replicas": 3
		},
		"handlers": [{
			"type": "Replier",
			"category": "manager",
			"endpoint": {"id": "worker_1", "port": 6001}
		}]
	}`)

	var service Service
	if err := json.Unmarshal(data, &service); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	region, err := service.Parameters.StringValue("region")
	if err != nil {
		t.Fatalf("Parameters.StringValue('region'): %v", err)
	}
	if region != "eu-west" {
		t.Fatalf("region = %q, want eu-west", region)
	}

	replicas, err := service.Parameters.Uint64Value("replicas")
	if err != nil {
		t.Fatalf("Parameters.Uint64Value('replicas'): %v", err)
	}
	if replicas != 3 {
		t.Fatalf("replicas = %d, want 3", replicas)
	}

	out, err := json.Marshal(service)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var roundTrip Service
	if err := json.Unmarshal(out, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal round trip: %v", err)
	}
	if roundTrip.Parameters == nil {
		t.Fatal("Parameters is nil after round trip")
	}
	if got, err := roundTrip.Parameters.StringValue("region"); err != nil || got != "eu-west" {
		t.Fatalf("round trip region = %q, err = %v", got, err)
	}
}

func TestServiceValidateHandlerDeps(t *testing.T) {
	serviceConfig, handlerOfType, _, _ := testService()
	serviceConfig.Handlers = []Handler{handlerOfType}
	serviceConfig.HandlerDeps = []DepService{{Name: "orphan"}}

	if err := serviceConfig.Validate(); err == nil {
		t.Fatal("ValidateService with invalid handler-deps returned nil error")
	}

	serviceConfig.HandlerDeps = []DepService{
		{
			Name: "public",
			Proxies: []string{
				"pkg:$?var=services[name:auth_proxy]",
			},
		},
	}
	if err := serviceConfig.Validate(); err != nil {
		t.Fatalf("ValidateService with handler-deps: %v", err)
	}
}

func TestServiceHandlerByCategory(t *testing.T) {
	serviceConfig, handlerOfType, handler2OfType, handlerOfType2 := testService()
	serviceConfig.Handlers = []Handler{handlerOfType, handler2OfType, handlerOfType2}

	if _, err := serviceConfig.HandlerByCategory(""); err == nil {
		t.Fatal("HandlerByCategory with empty category returned nil error")
	} else if !errors.Is(err, message.ErrInvalidArg) {
		t.Fatalf("HandlerByCategory with empty category: want ErrInvalidArg, got %v", err)
	}
	if _, err := serviceConfig.HandlerByCategory("missing"); err == nil {
		t.Fatal("HandlerByCategory with missing category returned nil error")
	} else if !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("HandlerByCategory with missing category: want ErrNotFound, got %v", err)
	}

	foundHandler, err := serviceConfig.HandlerByCategory("public")
	if err != nil {
		t.Fatalf("HandlerByCategory public: %v", err)
	}
	handler, ok := foundHandler.AsIndependentHandler()
	if !ok {
		t.Fatal("found handler is not an independent handler")
	}
	if handler.Endpoint.Id != handlerOfType.Endpoint.Id {
		t.Fatalf("handler id = %q, want %q", handler.Endpoint.Id, handlerOfType.Endpoint.Id)
	}
	if handler.Category != handlerOfType.Category {
		t.Fatalf("handler category = %q, want %q", handler.Category, handlerOfType.Category)
	}
}

func TestServiceGetHandler(t *testing.T) {
	serviceConfig, handlerOfType, _, handlerOfType2 := testService()
	serviceConfig.Handlers = []Handler{
		handlerOfType,
		IndependentHandler{
			Type:     PairType,
			Category: "pair",
			Endpoint: message.NewEndpoint(handlerOfType.Endpoint.Id, 9999),
		},
		handlerOfType2,
	}

	if _, err := serviceConfig.GetHandler(message.Endpoint{}); err == nil {
		t.Fatal("GetHandler with empty id returned nil error")
	}
	if _, err := serviceConfig.GetHandler(message.NewEndpoint(handlerOfType.Endpoint.Id, 1234)); err == nil {
		t.Fatal("GetHandler with missing endpoint returned nil error")
	}

	foundHandler, err := serviceConfig.GetHandler(handlerOfType.Endpoint)
	if err != nil {
		t.Fatalf("GetHandler: %v", err)
	}
	handler, ok := foundHandler.AsIndependentHandler()
	if !ok {
		t.Fatal("found handler is not an independent handler")
	}
	if handler.Type != handlerOfType.Type {
		t.Fatalf("handler type = %q, want %q", handler.Type, handlerOfType.Type)
	}
}

func TestServiceSetHandler(t *testing.T) {
	serviceConfig, handlerOfType, _, handlerOfType2 := testService()

	if len(serviceConfig.Handlers) != 0 {
		t.Fatalf("initial len(Handlers) = %d, want 0", len(serviceConfig.Handlers))
	}

	var nilService *Service
	nilService.SetHandler(handlerOfType)

	serviceConfig.SetHandler(handlerOfType)
	if len(serviceConfig.Handlers) != 1 {
		t.Fatalf("len(Handlers) = %d, want 1", len(serviceConfig.Handlers))
	}
	firstHandler, ok := serviceConfig.Handlers[0].AsIndependentHandler()
	if !ok || firstHandler.Type != ReplierType {
		t.Fatalf("handler type = %q, want %q", firstHandler.Type, ReplierType)
	}

	serviceConfig.SetHandler(handlerOfType2)
	if len(serviceConfig.Handlers) != 2 {
		t.Fatalf("len(Handlers) = %d, want 2", len(serviceConfig.Handlers))
	}
	firstHandler, ok = serviceConfig.Handlers[0].AsIndependentHandler()
	if !ok || firstHandler.Type != ReplierType {
		t.Fatalf("first handler type = %q, want %q", firstHandler.Type, ReplierType)
	}
	secondHandler, ok := serviceConfig.Handlers[1].AsIndependentHandler()
	if !ok || secondHandler.Type != SyncReplierType {
		t.Fatalf("second handler type = %q, want %q", secondHandler.Type, SyncReplierType)
	}

	updatedHandler := IndependentHandler{
		Type:     PairType,
		Category: "pair",
		Endpoint: handlerOfType.Endpoint,
	}
	serviceConfig.SetHandler(updatedHandler)
	if len(serviceConfig.Handlers) != 2 {
		t.Fatalf("len(Handlers) after update = %d, want 2", len(serviceConfig.Handlers))
	}
	firstHandler, ok = serviceConfig.Handlers[0].AsIndependentHandler()
	if !ok || firstHandler.Type != PairType {
		t.Fatalf("first handler type = %q, want %q", firstHandler.Type, PairType)
	}
	secondHandler, ok = serviceConfig.Handlers[1].AsIndependentHandler()
	if !ok || secondHandler.Type != SyncReplierType {
		t.Fatalf("second handler type = %q, want %q", secondHandler.Type, SyncReplierType)
	}
}

func TestServiceRemoveHandler(t *testing.T) {
	serviceConfig, handlerOfType, handler2OfType, handlerOfType2 := testService()
	serviceConfig.Handlers = []Handler{handlerOfType, handler2OfType, handlerOfType2}

	if err := serviceConfig.RemoveHandler(message.Endpoint{}); err == nil {
		t.Fatal("RemoveHandler with empty endpoint returned nil error")
	}
	if err := serviceConfig.RemoveHandler(message.NewEndpoint(handlerOfType.Endpoint.Id, 9999)); err == nil {
		t.Fatal("RemoveHandler with missing endpoint returned nil error")
	}

	if err := serviceConfig.RemoveHandler(handler2OfType.Endpoint); err != nil {
		t.Fatalf("RemoveHandler: %v", err)
	}
	if len(serviceConfig.Handlers) != 2 {
		t.Fatalf("len(Handlers) = %d, want 2", len(serviceConfig.Handlers))
	}
	firstHandler, ok := serviceConfig.Handlers[0].AsIndependentHandler()
	if !ok || firstHandler.Endpoint.Id != handlerOfType.Endpoint.Id {
		t.Fatalf("first handler id = %q, want %q", firstHandler.Endpoint.Id, handlerOfType.Endpoint.Id)
	}
	secondHandler, ok := serviceConfig.Handlers[1].AsIndependentHandler()
	if !ok || secondHandler.Endpoint.Id != handlerOfType2.Endpoint.Id {
		t.Fatalf("second handler id = %q, want %q", secondHandler.Endpoint.Id, handlerOfType2.Endpoint.Id)
	}
}
