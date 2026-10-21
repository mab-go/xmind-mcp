package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mab-go/logging"
	"github.com/mab-go/xmind-mcp/internal/server/handler"
)

// stdioTimeout bounds each wait on serveStdio, so a hang fails the test instead of stalling it
// until go test's own timeout.
const stdioTimeout = 5 * time.Second

// stdioClient drives serveStdio over in-memory pipes, as an MCP client drives the server
// process over its stdin and stdout.
type stdioClient struct {
	t      *testing.T
	cancel context.CancelFunc // cancels serveStdio's context
	in     *io.PipeWriter     // the server's stdin; Close sends EOF
	out    *bufio.Reader      // the server's stdout
	logs   *bytes.Buffer      // Debug-level log; read only after wait returns
	done   chan struct{}      // closed when serveStdio returns
	err    error              // serveStdio's result, set before done is closed
}

// startStdio runs serveStdio on a goroutine and returns a client for it. Cleanup cancels the
// context, closes both pipes and waits for serveStdio to return, so no Listen outlives its test:
// mcp-go's stdio session is shared by the whole process.
func startStdio(t *testing.T) *stdioClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &stdioClient{
		t:      t,
		cancel: cancel,
		in:     inW,
		out:    bufio.NewReader(outR),
		logs:   &bytes.Buffer{},
		done:   make(chan struct{}),
	}
	log := logging.NewLogger(logging.Config{Level: logging.DebugLevel, Output: c.logs})
	go func() {
		defer close(c.done)
		c.err = serveStdio(logging.NewContext(ctx, log), handler.NewXMindHandler(), inR, outW)
	}()
	t.Cleanup(func() {
		cancel()
		_ = inW.Close()
		_ = outR.Close()
		select {
		case <-c.done:
		case <-time.After(stdioTimeout):
			t.Errorf("serveStdio did not return within %v of cleanup", stdioTimeout)
		}
	})
	return c
}

// within runs f on a goroutine and returns its result, failing the test if f fails or takes
// longer than stdioTimeout. A call that times out stays blocked until cleanup closes its pipe;
// the buffered channel then lets its goroutine exit.
func within[T any](t *testing.T, what string, f func() (T, error)) T {
	t.Helper()
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := f()
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("%s: %v", what, r.err)
		}
		return r.v
	case <-time.After(stdioTimeout):
		t.Fatalf("%s: no result within %v", what, stdioTimeout)
	}
	var zero T
	return zero
}

// send writes msg to the server's stdin as one line. Listen drops a last line that has no
// newline.
func (c *stdioClient) send(msg string) {
	c.t.Helper()
	within(c.t, "write the server's stdin", func() (int, error) {
		return io.WriteString(c.in, msg+"\n")
	})
}

// recv reads the server's next stdout line and decodes it as a JSON-RPC message.
func (c *stdioClient) recv() map[string]any {
	c.t.Helper()
	line := within(c.t, "read the server's stdout", func() (string, error) {
		return c.out.ReadString('\n')
	})
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		c.t.Fatalf("decode %q: %v", line, err)
	}
	return msg
}

// wait returns serveStdio's result, failing the test if it has not returned within
// stdioTimeout.
func (c *stdioClient) wait() error {
	c.t.Helper()
	select {
	case <-c.done:
	case <-time.After(stdioTimeout):
		c.t.Fatalf("serveStdio did not return within %v", stdioTimeout)
	}
	return c.err
}

// assertEventsInOrder fails unless logs has a line for each event, in the given order.
func assertEventsInOrder(t *testing.T, logs string, events ...string) {
	t.Helper()
	lines := strings.Split(logs, "\n")
	from := 0
	for _, event := range events {
		i := slices.IndexFunc(lines[from:], func(line string) bool {
			return strings.Contains(line, "event="+event)
		})
		if i < 0 {
			t.Fatalf("log output = %q, want an event=%s line after the first %d lines", logs, event, from)
		}
		from += i + 1
	}
}

// TestServeStdioRoundTrip checks serveStdio answers initialize and a tool call over stdio,
// passes its logger to the lifecycle hooks, and stops cleanly at stdin EOF.
func TestServeStdioRoundTrip(t *testing.T) {
	c := startStdio(t)

	c.send(initializeRequest)
	result, _ := c.recv()["result"].(map[string]any)
	if got := result["protocolVersion"]; got != "2025-11-25" {
		t.Errorf("initialize protocolVersion = %v, want 2025-11-25", got)
	}
	c.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	// Marshal the request so a Windows path's backslashes are escaped.
	missing := filepath.Join(t.TempDir(), "missing.xmind")
	call, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params":  map[string]any{"name": "xmind_open_map", "arguments": map[string]any{"path": missing}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.send(string(call))
	assertToolError(t, c.recv(), "file not found: "+missing)

	if err := c.in.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.wait(); err != nil {
		t.Fatalf("serveStdio = %v, want nil at stdin EOF", err)
	}
	assertEventsInOrder(t, c.logs.String(),
		"server.ready", "server.starting", "server.started", "server.stopping", "server.stopped")
}

// TestServeStdioStopsOnCancel checks serveStdio stops cleanly when its context is cancelled
// while it waits for input, as it is on an interrupt or SIGTERM.
func TestServeStdioStopsOnCancel(t *testing.T) {
	c := startStdio(t)

	// The reply shows Listen is in its read loop, so the cancel below lands while it waits.
	c.send(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if reply := c.recv(); reply["id"] != float64(1) || reply["result"] == nil {
		t.Fatalf("ping reply = %v, want a result for id 1", reply)
	}

	c.cancel()
	if err := c.wait(); err != nil {
		t.Fatalf("serveStdio = %v, want nil after cancel", err)
	}
	assertEventsInOrder(t, c.logs.String(), "server.ready", "server.stopping", "server.stopped")
}

// TestServeStdioReportsReadError checks a failed stdin read ends serveStdio with that error,
// logged as server.error instead of a clean stop.
func TestServeStdioReportsReadError(t *testing.T) {
	c := startStdio(t)

	errRead := errors.New("stdin read failed")
	if err := c.in.CloseWithError(errRead); err != nil {
		t.Fatal(err)
	}
	err := c.wait()
	if !errors.Is(err, errRead) || !strings.HasPrefix(err.Error(), "failed to start stdio MCP server: ") {
		t.Fatalf("serveStdio = %v, want a failed-to-start error wrapping %q", err, errRead)
	}
	logs := c.logs.String()
	assertEventsInOrder(t, logs, "server.ready", "server.error", "server.stopped")
	if strings.Contains(logs, "event=server.stopping") {
		t.Errorf("log output = %q, want no event=server.stopping line", logs)
	}
}
