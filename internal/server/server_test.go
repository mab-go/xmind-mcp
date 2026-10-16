package server

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mab-go/logging"
	"github.com/mab-go/xmind-mcp/internal/server/handler"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// initializeRequest is a minimal initialize request for tests that need the handshake.
const initializeRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

// discoverRequest is a minimal server/discover request from a 2026-07-28 client, which skips
// initialize and sends its protocol version, client info, and capabilities in _meta.
const discoverRequest = `{"jsonrpc":"2.0","id":2,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"test","version":"0"},"io.modelcontextprotocol/clientCapabilities":{}}}}`

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

	resp := handleJSON(t, s, initializeRequest)
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

// TestInitializeReturnsInstructions checks the initialize result carries serverInstructions.
func TestInitializeReturnsInstructions(t *testing.T) {
	s := newXMindServer(context.Background(), handler.NewXMindHandler())

	resp := handleJSON(t, s, initializeRequest)
	result, _ := resp["result"].(map[string]any)
	if got, _ := result["instructions"].(string); got != serverInstructions {
		t.Errorf("initialize result.instructions = %q, want serverInstructions", got)
	}
}

// TestDiscoverAdvertisesServer checks a 2026-07-28 client, which skips initialize, gets the
// same server surface from server/discover: protocol versions, instructions, and capabilities.
func TestDiscoverAdvertisesServer(t *testing.T) {
	s := newXMindServer(context.Background(), handler.NewXMindHandler())

	resp := handleJSON(t, s, discoverRequest)
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("server/discover response = %v, want a result", resp)
	}
	versions, _ := result["supportedVersions"].([]any)
	for _, want := range []string{"2026-07-28", "2025-11-25"} {
		if !slices.Contains(versions, any(want)) {
			t.Errorf("supportedVersions = %v, want it to contain %q", versions, want)
		}
	}
	if got, _ := result["instructions"].(string); got != serverInstructions {
		t.Errorf("server/discover result.instructions = %q, want serverInstructions", got)
	}
	caps, _ := result["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Errorf("capabilities = %v, want a tools entry", caps)
	}
	if _, ok := caps["logging"]; ok {
		t.Errorf("capabilities = %v, want no logging entry", caps)
	}
}

// TestLifecycleHooksLogClient checks the initialize and server/discover hooks log the
// client and its protocol version at Debug, each on its own event's line.
func TestLifecycleHooksLogClient(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewLogger(logging.Config{Level: logging.DebugLevel, Output: &buf})
	s := newXMindServer(logging.NewContext(context.Background(), log), handler.NewXMindHandler())

	handleJSON(t, s, initializeRequest)
	handleJSON(t, s, discoverRequest)

	lines := strings.Split(buf.String(), "\n")
	for event, fields := range map[string][]string{
		"server.starting":    {"client=test", "protocol=2025-11-25"},
		"server.started":     {"protocol=2025-11-25"},
		"server.discovering": {"client=test", "protocol=2026-07-28"},
		"server.discovered":  nil,
	} {
		i := slices.IndexFunc(lines, func(line string) bool {
			return strings.Contains(line, "event="+event)
		})
		if i < 0 {
			t.Errorf("log output = %q, want an event=%s line", buf.String(), event)
			continue
		}
		for _, want := range fields {
			if !strings.Contains(lines[i], want) {
				t.Errorf("event=%s line = %q, want it to contain %q", event, lines[i], want)
			}
		}
	}
}

// TestInstructionsNameRegisteredTools checks every tool named in serverInstructions is
// registered, so renaming or removing a tool cannot leave stale guidance behind.
func TestInstructionsNameRegisteredTools(t *testing.T) {
	s := newXMindServer(context.Background(), handler.NewXMindHandler())

	names := regexp.MustCompile(`\bxmind_[A-Za-z0-9_-]*[A-Za-z0-9]`).FindAllString(serverInstructions, -1)
	if len(names) == 0 {
		t.Fatal("serverInstructions names no xmind_ tools; check the pattern")
	}
	for _, name := range names {
		if s.GetTool(name) == nil {
			t.Errorf("serverInstructions names %q, which is not a registered tool", name)
		}
	}
}

// TestInstructionsWordBudget keeps serverInstructions short: every connected session
// pays for it in context.
func TestInstructionsWordBudget(t *testing.T) {
	const maxWords = 150
	if n := len(strings.Fields(serverInstructions)); n > maxWords {
		t.Errorf("serverInstructions has %d words, want at most %d", n, maxWords)
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

	handleJSON(t, s, initializeRequest)
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
