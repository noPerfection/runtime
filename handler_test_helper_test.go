package topology

import (
	"fmt"

	"github.com/noPerfection/log"
	"github.com/noPerfection/protocol/handler/replier"
	"github.com/noPerfection/topology/config"
)

func newHandler(appConfig *config.NoPerfection) (*Handler, error) {
	if appConfig == nil {
		return nil, fmt.Errorf("app config is nil, call config.Load() first")
	}

	handler := replier.New()

	logger, err := log.New(TopologyHandlerCategory, true)
	if err != nil {
		return nil, fmt.Errorf("log.New('%s'): %w", TopologyHandlerCategory, err)
	}

	handler.SetEndpoint(HandlerEndpoint())
	if err := handler.SetLogger(logger); err != nil {
		return nil, fmt.Errorf("handler.SetLogger: %w", err)
	}

	return &Handler{
		config:   appConfig,
		topology: New(),
		handler:  handler,
	}, nil
}
