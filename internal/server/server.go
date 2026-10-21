// Package server provides the MCP server for xmind-mcp.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/mab-go/logging"
	"github.com/mab-go/xmind-mcp/internal/server/handler"
	"github.com/mab-go/xmind-mcp/internal/version"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Server metadata sent alongside the name and version in the initialize result's serverInfo
// and in the server info that 2026-07-28 results carry in _meta. Clients show it in server lists.
const (
	// serverTitle is the human-readable display name.
	serverTitle = "XMind MCP"
	// serverDescription is a one-sentence summary of what the server does.
	serverDescription = "An unofficial MCP server for reading and writing local XMind mind map files."
	// serverWebsiteURL is the project's home page.
	serverWebsiteURL = "https://github.com/mab-go/xmind-mcp"
)

func newXMindServer(ctx context.Context, h *handler.XMindHandler) *mcpserver.MCPServer {
	log, _ := logging.FromContext(ctx)

	hooks := &mcpserver.Hooks{}
	hooks.AddBeforeInitialize(hookAddBeforeInitialize(log))
	hooks.AddAfterInitialize(hookAddAfterInitialize(log))
	hooks.AddBeforeDiscover(hookAddBeforeDiscover(log))
	hooks.AddAfterDiscover(hookAddAfterDiscover(log))

	s := mcpserver.NewMCPServer(
		"xmind-mcp",
		version.Version,
		mcpserver.WithTitle(serverTitle),
		mcpserver.WithDescription(serverDescription),
		mcpserver.WithWebsiteURL(serverWebsiteURL),
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithInstructions(serverInstructions),
		mcpserver.WithToolHandlerMiddleware(recoverToolPanics(log)),
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

// RunStdioServer serves MCP over the process's stdin and stdout. It returns nil when stdin
// closes or the process gets an interrupt or SIGTERM.
func RunStdioServer() error {
	log := logging.NewDefaultLogger()
	ctx := logging.NewContext(context.Background(), log)
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	return serveStdio(ctx, handler.NewXMindHandler(), os.Stdin, os.Stdout)
}

// serveStdio serves MCP for h, reading requests from in and writing responses to out, until in
// reaches EOF or ctx is cancelled; both are a clean stop and return nil. newXMindServer and
// every request get ctx, so hooks and tool handlers log through its logger.
func serveStdio(ctx context.Context, h *handler.XMindHandler, in io.Reader, out io.Writer) error {
	log, _ := logging.FromContext(ctx)
	defer log.Info(eventServerStopped)

	srv := newXMindServer(ctx, h)

	log.WithField("transport", "stdio").Info(eventServerReady)

	// Listen returns nil at EOF and context.Canceled once ctx is cancelled.
	if err := mcpserver.NewStdioServer(srv).Listen(ctx, in, out); err != nil && !errors.Is(err, context.Canceled) {
		log.WithError(err).Error(eventServerError)
		return fmt.Errorf("failed to start stdio MCP server: %w", err)
	}

	log.Info(eventServerStopping)

	return nil
}
