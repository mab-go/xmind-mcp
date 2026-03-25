// Package server provides the MCP server for xmind-mcp.
package server

import (
	"context"
	"fmt"
	"io"

	"github.com/mab-go/xmind-mcp/internal/logging"
	"github.com/mab-go/xmind-mcp/internal/server/handler"
	"github.com/mab-go/xmind-mcp/internal/version"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

func newXMindServer(ctx context.Context, h *handler.XMindHandler) *mcpserver.MCPServer {
	log, _ := logging.FromContext(ctx)

	hooks := &mcpserver.Hooks{}
	hooks.AddBeforeInitialize(hookAddBeforeInitialize(log))
	hooks.AddAfterInitialize(hookAddAfterInitialize(log))

	s := mcpserver.NewMCPServer(
		"xmind-mcp",
		version.Version,
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithLogging(),
		mcpserver.WithHooks(hooks),
	)

	s.AddTool(toolOpenMap, h.OpenMap)
	s.AddTool(toolListSheets, h.ListSheets)
	s.AddTool(toolCreateMap, h.CreateMap)
	s.AddTool(toolAddSheet, h.AddSheet)
	s.AddTool(toolDeleteSheet, h.DeleteSheet)
	s.AddTool(toolGetSubtree, h.GetSubtree)
	s.AddTool(toolGetTopicProperties, h.GetTopicProperties)
	s.AddTool(toolSearchTopics, h.SearchTopics)
	s.AddTool(toolFindTopic, h.FindTopic)
	s.AddTool(toolAddTopic, h.AddTopic)
	s.AddTool(toolAddTopicsBulk, h.AddTopicsBulk)
	s.AddTool(toolDuplicateTopic, h.DuplicateTopic)
	s.AddTool(toolRenameTopic, h.RenameTopic)
	s.AddTool(toolDeleteTopic, h.DeleteTopic)
	s.AddTool(toolMoveTopic, h.MoveTopic)
	s.AddTool(toolReorderChildren, h.ReorderChildren)
	s.AddTool(toolSetTopicProperties, h.SetTopicProperties)
	s.AddTool(toolSetTopicPropertiesBulk, h.SetTopicPropertiesBulk)
	s.AddTool(toolAddFloatingTopic, h.AddFloatingTopic)
	s.AddTool(toolAddRelationship, h.AddRelationship)
	s.AddTool(toolListRelationships, h.ListRelationships)
	s.AddTool(toolDeleteRelationship, h.DeleteRelationship)
	s.AddTool(toolAddSummary, h.AddSummary)
	s.AddTool(toolAddBoundary, h.AddBoundary)
	s.AddTool(toolFlattenToOutline, h.FlattenToOutline)
	s.AddTool(toolImportFromOutline, h.ImportFromOutline)
	s.AddTool(toolFindAndReplace, h.FindAndReplace)

	return s
}

// RunStdioServer starts the MCP server and serves requests via stdio.
func RunStdioServer() error {
	log := logging.NewDefaultLogger()
	ctx := logging.NewContext(context.Background(), log)

	h := handler.NewXMindHandler()

	shutdown := func() {
		log.Info("Server shutdown complete")
	}
	defer shutdown()

	srv := newXMindServer(ctx, h)

	if err := mcpserver.ServeStdio(srv); err != nil {
		if err != context.Canceled && err != io.EOF {
			log.WithError(err).Error("Error running MCP server")
			return fmt.Errorf("failed to start stdio MCP server: %w", err)
		}
	}

	log.Info("Received shutdown signal; shutting down server...")

	return nil
}
