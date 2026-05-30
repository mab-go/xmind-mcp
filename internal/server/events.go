package server

import "github.com/mab-go/logging"

const (
	eventServerError    logging.Event = "server.error"    // Error running MCP server
	eventServerReady    logging.Event = "server.ready"    // MCP server ready (serving over stdio)
	eventServerStarted  logging.Event = "server.started"  // Server initialized (after-initialize hook)
	eventServerStarting logging.Event = "server.starting" // Server initializing (before-initialize hook)
	eventServerStopped  logging.Event = "server.stopped"  // Server shutdown complete
	eventServerStopping logging.Event = "server.stopping" // Received shutdown signal
)
