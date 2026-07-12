package topology

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/noPerfection/datatype"
	"github.com/noPerfection/log"
	protocolClient "github.com/noPerfection/protocol/client"
	protocolHandler "github.com/noPerfection/protocol/handler"
	"github.com/noPerfection/protocol/message"
	config "github.com/noPerfection/topology/config"

	"github.com/stretchr/testify/suite"
)

// Define the suite, and absorb the built-in basic suite
// functionality from testify - including a T() method which
// returns the current testing orchestra
type TestClientSuite struct {
	suite.Suite

	logger            *log.Logger
	depHandler        *Handler // the manager to test
	depHandlerManager *protocolClient.SyncReplierClient
	url               string // dependency source code
	id                string // the id of the dependency
	parent            string // the service name that dependency should connect back to

	client *Client
}

// Make sure that Account is set to five
// before each test
func (test *TestClientSuite) SetupTest() {
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

	controlEndpoint := protocolHandler.NewInternalControlEndpoint(HandlerEndpoint())
	test.depHandlerManager, err = protocolClient.NewSyncReplier(controlEndpoint.Id, controlEndpoint.Port)
	s().NoError(err)

	// wait a bit for closing
	time.Sleep(time.Millisecond * 100)

	// A valid source code that we want to download
	test.url = "github.com/noPerfection/test-manager"

	test.id = "test-manager"
	test.parent = "parent"

	socket, err := NewClient()
	s().NoError(err)

	test.client = socket
	test.client.Timeout(time.Second * 30)
	test.client.Attempt(1)
}

func (test *TestClientSuite) TearDownTest() {
	s := test.Suite.Require

	s().NoError(test.client.Close())

	_, err := test.depHandlerManager.Request(&message.Request{
		Command:    protocolHandler.HandlerClose,
		Parameters: datatype.New(),
	})
	s().NoError(err)
	s().NoError(test.depHandlerManager.Close())

	// Wait a bit for the close of the handler thread.
	time.Sleep(time.Millisecond * 100)
}

//// Test_13_Start tests IsServiceRunning, StartService and StopService commands.
//func (test *TestClientSuite) Test_13_Start() {
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
//	// Let's run the dependency
//	test.logger.Info("request to run the dependency", "srcUrl", src.Url, "id", test.id)
//	id, err := test.client.StartService(src.Url)
//	s().NoError(err)
//
//	// Just wait a bit for initialization of the service
//	time.Sleep(time.Millisecond * 100)
//
//	// check that service is running
//	test.logger.Info("check dependency status")
//	running, err := test.client.IsServiceRunning(depClient)
//	s().NoError(err)
//	s().True(running)
//	test.logger.Info("status returned from dependency manager", "running", running, "error", err)
//
//	// StopService the service
//	test.logger.Info("send a signal to close dependency")
//
//	err = test.client.StopService(depClient)
//	s().NoError(err)
//
//	// Wait a bit for closing the source process
//	time.Sleep(time.Millisecond * 100)
//
//	// Checking for a running source after it was closed must fail
//	test.logger.Info("check again the dependency status")
//	running, err = test.client.IsServiceRunning(depClient)
//	test.logger.Info("closed dependency status returned", "running", running, "error", err)
//	s().NoError(err)
//	s().False(running)
//
//}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestClient(t *testing.T) {
	suite.Run(t, new(TestClientSuite))
}
