package topology

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/noPerfection/datatype"
	"github.com/noPerfection/log"
	"github.com/noPerfection/protocol/client"
	"github.com/noPerfection/protocol/client/sync_replier"
	"github.com/noPerfection/protocol/handler/control"
	"github.com/noPerfection/protocol/message"
	config "github.com/noPerfection/topology/config"

	"github.com/stretchr/testify/suite"
)

// Define the suite, and absorb the built-in basic suite
// functionality from testify - including a T() method which
// returns the current testing orchestra
type TestHandlerSuite struct {
	suite.Suite

	logger            *log.Logger
	depHandler        *Handler // the manager to test
	depHandlerManager *sync_replier.Client
	url               string // dependency source code
	id                string // the id of the dependency
	parent            string // the service name that dependency should connect back to

	client *client.Socket // imitating the service
}

// Make sure that Account is set to five
// before each test
func (test *TestHandlerSuite) SetupTest() {
	s := test.Suite.Require

	logger, _ := log.New("test", false)
	test.logger = logger

	var err error
	cfgPath := filepath.Join(test.T().TempDir(), "app.json")
	cfg, err := config.Load(cfgPath)
	s().NoError(err)
	test.depHandler, err = newHandler(&cfg)
	s().NoError(err)

	// Start the handler
	s().NoError(test.depHandler.Start())

	controlEndpoint := control.NewInternalControlEndpoint(HandlerEndpoint())
	test.depHandlerManager, err = sync_replier.NewClient(controlEndpoint.Id, controlEndpoint.Port)
	s().NoError(err)

	// wait a bit for closing
	time.Sleep(time.Millisecond * 100)

	// A valid source code that we want to download
	test.url = "github.com/noPerfection/test-manager"

	test.id = "test-manager"
	test.parent = "parent"

	handlerEndpoint := HandlerEndpoint()
	socket, err := client.New(handlerEndpoint.Id, handlerEndpoint.Port, client.ReplierType)
	s().NoError(err)

	test.client = socket
	test.client.Timeout(time.Second * 30)
	test.client.Attempt(1)
}

func (test *TestHandlerSuite) TearDownTest() {
	s := test.Suite.Require

	s().NoError(test.client.Close())

	_, err := test.depHandlerManager.Request(&message.Request{
		Command:    control.HandlerClose,
		Parameters: datatype.New(),
	})
	s().NoError(err)
	s().NoError(test.depHandlerManager.Close())

	// Wait a bit for the close of the handler thread.
	time.Sleep(time.Millisecond * 100)
}

func TestHandlerTopologyInterfaceBeforeStart(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "app.json")
	appConfig, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	handler, err := newHandler(&appConfig)
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}

	err = handler.AddService(config.Service{
		Type: config.ProxyType,
		Name: "pre-start-service",
		Handlers: []config.Handler{
			config.ProxyHandler{
				IndependentHandler: config.IndependentHandler{
					Type:     config.ReplierType,
					Category: ServiceManagerCategory,
					Endpoint: message.NewEndpoint("pre-start-manager", 6100),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("handler.AddService before start: %v", err)
	}

	if _, err := appConfig.GetService("pre-start-service"); err != nil {
		t.Fatalf("appConfig.GetService: %v", err)
	}
}

func TestHandlerStartSkipsWhenTopologyAlreadyRunning(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "app.json")
	appConfig, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	first, err := newHandler(&appConfig)
	if err != nil {
		t.Fatalf("newHandler first: %v", err)
	}
	second, err := newHandler(&appConfig)
	if err != nil {
		t.Fatalf("newHandler second: %v", err)
	}
	third, err := newHandler(&appConfig)
	if err != nil {
		t.Fatalf("newHandler third: %v", err)
	}

	if err := first.Start(); err != nil {
		t.Fatalf("first.Start: %v", err)
	}
	defer closeTopologyHandler(t)

	if err := second.Start(); err != nil {
		t.Fatalf("second.Start: %v", err)
	}
	if err := third.Start(); err != nil {
		t.Fatalf("third.Start: %v", err)
	}

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	client.Timeout(50 * time.Millisecond)
	client.Attempt(2)

	running, err := client.IsRunning()
	if err != nil {
		t.Fatalf("client.IsRunning: %v", err)
	}
	if !running {
		t.Fatal("client.IsRunning returned false")
	}
}

func TestHandlerConcurrentPreStartCallsAcrossInstances(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "app.json")
	appConfig, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	handlers := make([]*Handler, 5)
	for i := range handlers {
		handlers[i], err = newHandler(&appConfig)
		if err != nil {
			t.Fatalf("newHandler[%d]: %v", i, err)
		}
	}

	for i := range handlers {
		if err := handlers[0].AddService(testHandlerService("set-service-" + string(rune('a'+i)))); err != nil {
			t.Fatalf("seed service %d: %v", i, err)
		}
	}

	start := make(chan struct{})
	errs := make(chan error, len(handlers)*4)
	var wg sync.WaitGroup
	for i, handler := range handlers {
		wg.Add(1)
		go func(index int, handler *Handler) {
			defer wg.Done()
			<-start

			errs <- handler.SetService(testHandlerService("set-service-" + string(rune('a'+index))))
			errs <- handler.AddService(testHandlerService("add-service-" + string(rune('a'+index))))
			errs <- handler.RemoveService("set-service-" + string(rune('a'+index)))
			_, err := handler.StartService("set-service-" + string(rune('a'+index)))
			errs <- err
		}(i, handler)
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		_ = err // Some calls are expected to fail; this test verifies concurrent access is safe.
	}

	for i, handler := range handlers {
		if err := handler.Start(); err != nil {
			t.Fatalf("handler[%d].Start: %v", i, err)
		}
	}
	defer closeTopologyHandler(t)
}

func testHandlerService(name string) config.Service {
	return config.Service{
		Type: config.IndependentType,
		Name: name,
		Handlers: []config.Handler{config.IndependentHandler{
			Type:     config.ReplierType,
			Category: "main",
			Endpoint: message.NewEndpoint("localhost", 9000),
		}},
	}
}

func closeTopologyHandler(t *testing.T) {
	t.Helper()

	controlEndpoint := control.NewInternalControlEndpoint(HandlerEndpoint())
	manager, err := sync_replier.NewClient(controlEndpoint.Id, controlEndpoint.Port)
	if err != nil {
		t.Fatalf("sync_replier.NewClient: %v", err)
	}
	defer manager.Close()

	_, err = manager.Request(&message.Request{
		Command:    control.HandlerClose,
		Parameters: datatype.New(),
	})
	if err != nil {
		t.Fatalf("control.HandlerClose: %v", err)
	}

	time.Sleep(250 * time.Millisecond)
}

func (test *TestHandlerSuite) TestTopologyInterfaceAfterStartBlocked() {
	s := test.Suite.Require

	s().Error(test.depHandler.AddService(config.Service{Name: "blocked", Type: config.ProxyType}))
	s().Error(test.depHandler.SetService(config.Service{Name: "blocked", Type: config.ProxyType}))
	s().Error(test.depHandler.RemoveService("blocked"))
	_, err := test.depHandler.StartService("blocked")
	s().Error(err)
	_, err = test.depHandler.IsServiceRunning("blocked")
	s().Error(err)
	s().Error(test.depHandler.StopService("blocked"))
}

//
//// Test_13_Start tests IsServiceRunning, StartService and StopService commands.
//func (test *TestHandlerSuite) Test_13_Start() {
//	s := test.Suite.Require
//
//	depClient := &clientConfig.Client{
//		ServiceUrl: test.url,
//		Id:         test.id,
//		Port:       6000,
//		TargetType: handlerConfig.SocketType(handlerConfig.ReplierType),
//	}
//
//	src, err := source.New(test.url)
//	s().NoError(err)
//	src.SetBranch("server") // the sample server is written in this branch.
//
//	// Let's run it
//	runReq := message.Request{
//		Command: StartService,
//		Parameters: key_value.New().
//			Set("parent", test.parent).
//			Set("url", src.Url).
//			Set("id", test.id),
//	}
//	rep, err = test.client.Request(&runReq)
//	s().NoError(err)
//	s().True(rep.IsOK())
//
//	// Just wait a bit for initialization of the service
//	time.Sleep(time.Millisecond * 100)
//
//	// check that service is running
//	runningReq := message.Request{
//		Command: IsServiceRunning,
//		Parameters: key_value.New().
//			Set("dep", depClient),
//	}
//	running, err := test.client.Request(&runningReq)
//	s().NoError(err)
//	s().True(running.IsOK())
//	result, err := running.ReplyParameters().BoolValue("running")
//	s().NoError(err)
//	s().True(result)
//
//	// Close the service
//	closeReq := message.Request{
//		Command: StopService,
//		Parameters: key_value.New().
//			Set("dep", depClient),
//	}
//	running, err = test.client.Request(&closeReq)
//	s().NoError(err)
//	s().True(running.IsOK())
//
//	// Wait a bit for closing the source process
//	time.Sleep(time.Millisecond * 100)
//
//	// Checking for a running source after it was closed must fail
//	running, err = test.client.Request(&runningReq)
//	s().NoError(err)
//	s().True(running.IsOK())
//	result, err = running.ReplyParameters().BoolValue("running")
//	s().NoError(err)
//	s().False(result)
//
//}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestHandler(t *testing.T) {
	suite.Run(t, new(TestHandlerSuite))
}
