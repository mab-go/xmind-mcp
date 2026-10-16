package server

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mab-go/logging"
	"github.com/mab-go/xmind-mcp/internal/server/handler"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// handleJSON sends one JSON-RPC message to s and returns the response decoded as a generic map.
func handleJSON(t *testing.T, s *mcpserver.MCPServer, msg string) map[string]any {
	t.Helper()
	data, err := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(msg)))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

// TestServerDoesNotAdvertiseLogging checks the server does not declare the MCP logging
// capability: it never sends notifications/message, so logging/setLevel must not be offered.
func TestServerDoesNotAdvertiseLogging(t *testing.T) {
	s := newXMindServer(context.Background(), handler.NewXMindHandler())

	resp := handleJSON(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`)
	result, _ := resp["result"].(map[string]any)
	caps, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("initialize response has no result.capabilities: %v", resp)
	}
	if _, ok := caps["tools"]; !ok {
		t.Errorf("capabilities = %v, want a tools entry", caps)
	}
	if _, ok := caps["logging"]; ok {
		t.Errorf("capabilities = %v, want no logging entry", caps)
	}

	resp = handleJSON(t, s, `{"jsonrpc":"2.0","id":2,"method":"logging/setLevel","params":{"level":"debug"}}`)
	errObj, _ := resp["error"].(map[string]any)
	if code, _ := errObj["code"].(float64); code != float64(mcp.METHOD_NOT_FOUND) {
		t.Errorf("logging/setLevel response = %v, want error code %d", resp, mcp.METHOD_NOT_FOUND)
	}
}

// TestToolHandlerPanicIsRecovered checks a panic in a tool handler becomes an internal-error
// response and a tool.panic log entry with a stack trace, instead of crashing the server.
func TestToolHandlerPanicIsRecovered(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewLogger(logging.Config{Level: logging.DebugLevel, Output: &buf})
	s := newXMindServer(logging.NewContext(context.Background(), log), handler.NewXMindHandler())
	s.AddTool(mcp.NewTool("panic_probe"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		panic("boom")
	})

	handleJSON(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`)
	resp := handleJSON(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"panic_probe","arguments":{}}}`)

	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("tools/call response = %v, want an error", resp)
	}
	if code, _ := errObj["code"].(float64); code != float64(mcp.INTERNAL_ERROR) {
		t.Errorf("error code = %v, want %d", errObj["code"], mcp.INTERNAL_ERROR)
	}
	msg, _ := errObj["message"].(string)
	for _, want := range []string{"panic_probe", "boom"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message = %q, want it to contain %q", msg, want)
		}
	}

	logged := buf.String()
	for _, want := range []string{"level=ERROR", "event=tool.panic", "tool=panic_probe", "panic=boom", "goroutine "} {
		if !strings.Contains(logged, want) {
			t.Errorf("log output = %q, want it to contain %q", logged, want)
		}
	}
}
