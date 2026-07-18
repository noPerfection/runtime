package topology

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/noPerfection/log"
	"github.com/noPerfection/os/path"
	"github.com/noPerfection/protocol/message"
	config "github.com/noPerfection/topology/config"
	"github.com/stretchr/testify/suite"
)

// todo for public functions test with the nil values

// Define the suite, and absorb the built-in basic suite
// functionality from testify - including a T() method which
// returns the current testing orchestra
type TestDepManagerSuite struct {
	suite.Suite

	logger       *log.Logger
	appConfig    *config.NoPerfection
	topology     *Topology
	currentDir   string
	url          string
	id           string
	parent       string
	localTestDir string
}

func (test *TestDepManagerSuite) service(name string) config.Service {
	service, err := test.appConfig.GetService(name)
	test.Require().NoError(err)
	return service
}

func (test *TestDepManagerSuite) setServiceStartCommand(name string, startCommand string) {
	services, err := test.appConfig.GetServices(rootServicesParent)
	test.Require().NoError(err)
	for i := range services {
		if services[i].Name == name {
			services[i].StartCommand = startCommand
			test.Require().NoError(test.appConfig.SetService(services[i], rootServicesParent))
			return
		}
	}

	test.Require().NoError(test.appConfig.AddService(config.Service{
		Name:         name,
		StartCommand: startCommand,
	}, rootServicesParent))
}

func (test *TestDepManagerSuite) requireTestBinary(binary string) {
	_, err := os.Stat(binary)
	if os.IsNotExist(err) {
		test.T().Skipf("test service binary %q is missing; build the _test_services fixtures to run this test", binary)
	}
	test.Require().NoError(err)
}

// Make sure that Account is set to five
// before each test
func (test *TestDepManagerSuite) SetupTest() {
	s := test.Require

	logger, _ := log.New("TestDepManagerSuite", false)
	test.logger = logger

	currentDir, err := path.CurrentDir()
	s().NoError(err)
	test.currentDir = currentDir

	cfgPath := filepath.Join(test.T().TempDir(), "app.json")
	cfg, err := config.Load(cfgPath)
	s().NoError(err)
	s().NoError(cfg.AddService(config.Service{
		Type:         config.ProxyType,
		Name:         "test-manager",
		StartCommand: "test",
		Handlers: []config.Handler{
			config.ProxyHandler{
				IndependentHandler: config.IndependentHandler{
					Type:     config.ReplierType,
					Category: config.ServiceManagerCategory,
					Endpoint: message.NewEndpoint("test-manager", 6000),
				},
			},
		},
	}, rootServicesParent))

	test.appConfig = &cfg
	test.topology = New()

	// A valid source code that we want to download
	test.url = "github.com/noPerfection/test-manager"

	test.id = "test-manager"
	test.parent = "parent"

	test.localTestDir = filepath.Join("_test_services")
}

// Test_0_New tests the creation of the Topology.
func (test *TestDepManagerSuite) Test_0_New() {
	s := test.Require

	depTopology := New()
	s().NotNil(depTopology)
	s().NotNil(depTopology.sameServices)
	s().NotNil(depTopology.runningProcesses)
	s().Equal(DefaultTimeout, depTopology.timeout)

	test.topology = depTopology
}

func (test *TestDepManagerSuite) Test_10_GenerateId() {
	s := test.Require

	id, err := test.topology.generateProcessId(test.id)
	s().NoError(err)
	s().Equal("test-manager1", id)
	s().Equal(1, test.topology.sameServices[test.id])

	service := test.service("test-manager")
	test.topology.runningProcesses[id] = &Process{
		config: &service,
		id:     id,
	}

	id, err = test.topology.generateProcessId(test.id)
	s().NoError(err)
	s().Equal("test-manager2", id)
	s().Equal(2, test.topology.sameServices[test.id])

	delete(test.topology.runningProcesses, "test-manager1")
	test.topology.refreshServiceCount(test.id)
	s().Equal(0, test.topology.sameServices[test.id])
}

func (test *TestDepManagerSuite) Test_12_AddRemoveService() {
	s := test.Require

	cfgPath := filepath.Join(test.T().TempDir(), "app.json")
	cfg, err := config.Load(cfgPath)
	s().NoError(err)
	handler, err := newHandler(&cfg)
	s().NoError(err)

	service := config.Service{
		Type:         config.ProxyType,
		Name:         "extra-service",
		StartCommand: "echo extra",
		Handlers: []config.Handler{
			config.ProxyHandler{
				IndependentHandler: config.IndependentHandler{
					Type:     config.ReplierType,
					Category: config.ServiceManagerCategory,
					Endpoint: message.NewEndpoint("extra-service-manager", 6001),
				},
			},
		},
	}
	err = handler.AddService(service)
	s().NoError(err)

	got, err := cfg.GetService("extra-service")
	s().NoError(err)
	s().Equal("echo extra", got.StartCommand)

	err = handler.RemoveService("extra-service")
	s().NoError(err)

	_, err = cfg.GetService("extra-service")
	s().Error(err)

	err = handler.RemoveService("missing")
	s().Error(err)

	err = handler.AddService(config.Service{
		Type:         config.ProxyType,
		Name:         "plain-service",
		StartCommand: "echo plain",
	})
	s().NoError(err)
	err = handler.RemoveService("plain-service")
	s().NoError(err)
}

func (test *TestDepManagerSuite) Test_13_AddServiceTargetValidation() {
	s := test.Require

	cfgPath := filepath.Join(test.T().TempDir(), "app.json")
	cfg, err := config.Load(cfgPath)
	s().NoError(err)
	service, err := test.appConfig.GetService("test-manager")
	s().NoError(err)
	s().NoError(cfg.AddService(service, rootServicesParent))
	test.appConfig = &cfg

	err = cfg.AddService(config.Service{}, rootServicesParent)
	s().Error(err)

	err = cfg.AddService(config.Service{
		Type: config.ProxyType,
		Name: "duplicate-socket",
		Handlers: []config.Handler{
			config.ProxyHandler{
				IndependentHandler: config.IndependentHandler{
					Type:     config.ReplierType,
					Category: config.ServiceManagerCategory,
					Endpoint: message.NewEndpoint("test-manager", 6000),
				},
			},
		},
	}, rootServicesParent)
	s().NoError(err)

	err = cfg.AddService(config.Service{
		Type: config.ProxyType,
		Name: "nested-parent",
		Handlers: []config.Handler{
			config.ProxyHandler{
				IndependentHandler: config.IndependentHandler{
					Type:     config.ReplierType,
					Category: config.ServiceManagerCategory,
					Endpoint: message.NewEndpoint("nested-parent-manager", 6100),
					CommandDeps: []config.DepService{
						{
							Name:    "proxy",
							Proxies: []string{"pkg:$?var=services[name:test-manager]"},
						},
					},
				},
			},
		},
	}, rootServicesParent)
	s().NoError(err)

	_, err = cfg.GetService("nested-parent")
	s().NoError(err)
	_, err = cfg.GetService("test-manager")
	s().NoError(err)

	err = cfg.AddService(config.Service{
		Type: config.ProxyType,
		Name: "service-level-parent",
		HandlerDeps: []config.DepService{
			{
				Name:    "manager",
				Proxies: []string{"pkg:$?var=services[name:test-manager]"},
			},
		},
		Handlers: []config.Handler{
			config.ProxyHandler{
				IndependentHandler: config.IndependentHandler{
					Type:     config.ReplierType,
					Category: config.ServiceManagerCategory,
					Endpoint: message.NewEndpoint("service-level-parent-manager", 6200),
				},
			},
		},
	}, rootServicesParent)
	s().NoError(err)

	_, err = cfg.GetService("service-level-parent")
	s().NoError(err)
	_, err = cfg.GetService("test-manager")
	s().NoError(err)

	err = cfg.AddService(config.Service{
		Type: config.ProxyType,
		Name: "proxy-outbound-child",
		Handlers: []config.Handler{
			config.ProxyHandler{
				IndependentHandler: config.IndependentHandler{
					Type:     config.ReplierType,
					Category: config.ServiceManagerCategory,
					Endpoint: message.NewEndpoint("proxy-outbound-child-manager", 6301),
				},
			},
		},
	}, rootServicesParent)
	s().NoError(err)

	err = cfg.AddService(config.Service{
		Type: config.ProxyType,
		Name: "proxy-parent",
		Handlers: []config.Handler{
			config.ProxyHandler{
				IndependentHandler: config.IndependentHandler{
					Type:     config.ReplierType,
					Category: config.ServiceManagerCategory,
					Endpoint: message.NewEndpoint("proxy-parent-manager", 6300),
				},
				Outbounds: []string{
					"pkg:$?var=services[name:proxy-outbound-child]&category=manager",
				},
			},
		},
	}, rootServicesParent)
	s().NoError(err)

	_, err = cfg.GetService("proxy-parent")
	s().NoError(err)
	_, err = cfg.GetService("proxy-outbound-child")
	s().NoError(err)
}

// Test_20_Run runs the given binary.
func (test *TestDepManagerSuite) Test_20_Run() {
	s := test.Require

	localBin := path.BinPath(filepath.Join(test.localTestDir, "test-manager", "bin"), "test")
	invalidBin := path.BinPath(filepath.Join(test.localTestDir, "test-manager", "bin"), "non_existing")
	test.requireTestBinary(localBin)
	test.setServiceStartCommand(test.id, localBin)

	_, ok := test.topology.runningProcesses[test.id+"1"]
	s().False(ok)

	// running nil values must exist
	var depTopology *Topology
	_, err := depTopology.StartService(test.service(test.id))
	s().Error(err)

	_, err = test.topology.StartService(config.Service{})
	s().Error(err) // missing service name

	test.setServiceStartCommand("no-command", "")
	_, err = test.topology.StartService(test.service("no-command"))
	s().Error(err) // no start command

	// the binary doesn't exist
	test.setServiceStartCommand(test.id, invalidBin)
	_, err = test.topology.StartService(test.service(test.id))
	s().Error(err) // no binary

	// Let's run it, it should exit immediately
	test.setServiceStartCommand(test.id, localBin)
	id, err := test.topology.StartService(test.service(test.id))
	s().NoError(err)

	_, ok = test.topology.runningProcesses[id]
	s().True(ok)

	// clean out
	_, ok = test.topology.runningProcesses[id]
	if ok {
		onStop := test.topology.OnStop(id)
		err = <-onStop
		s().NoError(err)

		_, running := test.topology.runningProcesses[id]
		s().False(running)
	}
}

// Test_21_RunError runs the binary that exits with error.
// If it exists with an error, it must catch it.
func (test *TestDepManagerSuite) Test_21_RunError() {
	s := test.Require

	localBin := path.BinPath(filepath.Join(test.localTestDir, "with-error", "bin"), "test")
	test.requireTestBinary(localBin)
	test.setServiceStartCommand(test.id, localBin)

	// Let's run it
	id, err := test.topology.StartService(test.service(test.id))
	s().NoError(err)

	// make sure that it exists
	_, ok := test.topology.runningProcesses[id]
	s().True(ok)

	stopChan := test.topology.OnStop(id)
	s().NotNil(stopChan)

	err = <-stopChan
	s().Error(err)

	// the closed service is removed from Topology
	_, ok = test.topology.runningProcesses[id]
	s().False(ok)

}

// Test_22_Running checks that service is running
func (test *TestDepManagerSuite) Test_22_Running() {
	s := test.Require

	localBin := path.BinPath(filepath.Join(test.localTestDir, "server", "bin"), "test")
	test.requireTestBinary(localBin)
	test.setServiceStartCommand(test.id, localBin)

	// First, install the manager
	// Let's run it
	id, err := test.topology.StartService(test.service(test.id))
	s().NoError(err)
	s().NotNil(test.topology.runningProcesses[id]) // cmd == nil indicates that the program was closed

	// Check that the local process is tracked while running.
	s().NotNil(test.topology.runningProcesses[id])

	// service is running two seconds. after that running should return false
	onStop := test.topology.OnStop(id)
	s().NotNil(onStop)
	err = <-onStop
	s().NoError(err)

	s().Nil(test.topology.runningProcesses[id]) // cmd == nil indicates that the program was closed
}

func TestStartServiceProceedsWhenManagerUnreachable(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "app.json")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.AddService(config.Service{
		Type:         config.ProxyType,
		Name:         "ipc-proxy",
		StartCommand: "true",
		Handlers: []config.Handler{
			config.ProxyHandler{
				IndependentHandler: config.IndependentHandler{
					Type:     config.SyncReplierType,
					Category: "main",
					Endpoint: message.NewEndpoint("tmp/unreachable_proxy", 0),
				},
			},
			config.ProxyHandler{
				IndependentHandler: config.IndependentHandler{
					Type:     config.SyncReplierType,
					Category: config.ServiceManagerCategory,
					Endpoint: message.NewEndpoint("tmp/unreachable_proxy_manager", 0),
				},
			},
		},
	}, rootServicesParent); err != nil {
		t.Fatalf("AddService: %v", err)
	}

	service, err := cfg.GetService("ipc-proxy")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}

	tp := New()
	tp.timeout = time.Millisecond * 100

	startedAt := time.Now()
	id, err := tp.StartService(service)
	if err != nil {
		t.Fatalf("StartService: %v", err)
	}
	if id == "" {
		t.Fatal("expected generated process id")
	}
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("StartService took %s, expected fast local start", elapsed)
	}
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestDepManager(t *testing.T) {
	suite.Run(t, new(TestDepManagerSuite))
}
