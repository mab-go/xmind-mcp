package server

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/mab-go/logging"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// recoverToolPanics returns a tool-handler middleware that recovers a panic in a handler,
// logs it with a stack trace, and returns it as a Go error (an MCP protocol error), so one
// bad call cannot take down the stdio server.
func recoverToolPanics(log logging.Logger) mcpserver.ToolHandlerMiddleware {
	return func(next mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (res *mcp.CallToolResult, err error) {
			defer func() {
				if r := recover(); r != nil {
					log.WithFields(logging.Fields{
						"tool":  req.Params.Name,
						"panic": fmt.Sprint(r),
						"stack": string(debug.Stack()),
					}).Error(eventToolPanic)
					res = nil
					err = fmt.Errorf("panic in %s tool handler: %v", req.Params.Name, r)
				}
			}()
			return next(ctx, req)
		}
	}
}
