package server

import (
	"context"

	"github.com/mab-go/logging"

	"github.com/mark3labs/mcp-go/mcp"
)

func hookAddBeforeInitialize(log logging.Logger) func(ctx context.Context, id any, message *mcp.InitializeRequest) {
	return func(_ context.Context, id any, _ *mcp.InitializeRequest) {
		log.WithField("id", id).Debug(eventServerStarting)
	}
}

func hookAddAfterInitialize(log logging.Logger) func(ctx context.Context, id any, _ *mcp.InitializeRequest, _ *mcp.InitializeResult) {
	return func(_ context.Context, id any, _ *mcp.InitializeRequest, _ *mcp.InitializeResult) {
		log.WithField("id", id).Debug(eventServerStarted)
	}
}
