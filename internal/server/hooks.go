package server

import (
	"context"

	"github.com/mab-go/xmind-mcp/internal/logging"

	"github.com/mark3labs/mcp-go/mcp"
)

func hookAddBeforeInitialize(log logging.Logger) func(ctx context.Context, id any, message *mcp.InitializeRequest) {
	return func(_ context.Context, id any, _ *mcp.InitializeRequest) {
		log.WithField("id", id).Debug("Preparing to initialize server")
	}
}

func hookAddAfterInitialize(log logging.Logger) func(ctx context.Context, id any, _ *mcp.InitializeRequest, _ *mcp.InitializeResult) {
	return func(_ context.Context, id any, _ *mcp.InitializeRequest, _ *mcp.InitializeResult) {
		log.WithField("id", id).Debug("Initialized server")
	}
}
