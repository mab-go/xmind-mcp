package server

import (
	"context"

	"github.com/mab-go/logging"

	"github.com/mark3labs/mcp-go/mcp"
)

func hookAddBeforeInitialize(log logging.Logger) func(ctx context.Context, id any, message *mcp.InitializeRequest) {
	return func(_ context.Context, id any, message *mcp.InitializeRequest) {
		log.WithFields(logging.Fields{
			"id":             id,
			"client":         message.Params.ClientInfo.Name,
			"client_version": message.Params.ClientInfo.Version,
			"protocol":       message.Params.ProtocolVersion,
		}).Debug(eventServerStarting)
	}
}

func hookAddAfterInitialize(log logging.Logger) func(ctx context.Context, id any, _ *mcp.InitializeRequest, result *mcp.InitializeResult) {
	return func(_ context.Context, id any, _ *mcp.InitializeRequest, result *mcp.InitializeResult) {
		log.WithFields(logging.Fields{
			"id":       id,
			"protocol": result.ProtocolVersion,
		}).Debug(eventServerStarted)
	}
}

// hookAddBeforeDiscover logs clients on protocol 2026-07-28 and later, which skip
// initialize and carry their client info in each request's _meta.
func hookAddBeforeDiscover(log logging.Logger) func(ctx context.Context, id any, message *mcp.DiscoverRequest) {
	return func(_ context.Context, id any, message *mcp.DiscoverRequest) {
		fields := logging.Fields{
			"id":       id,
			"protocol": message.Params.Meta.ProtocolVersion(),
		}
		if client := message.Params.Meta.ClientInfo(); client != nil {
			fields["client"] = client.Name
			fields["client_version"] = client.Version
		}
		log.WithFields(fields).Debug(eventServerDiscovering)
	}
}

func hookAddAfterDiscover(log logging.Logger) func(ctx context.Context, id any, _ *mcp.DiscoverRequest, _ *mcp.DiscoverResult) {
	return func(_ context.Context, id any, _ *mcp.DiscoverRequest, _ *mcp.DiscoverResult) {
		log.WithField("id", id).Debug(eventServerDiscovered)
	}
}
