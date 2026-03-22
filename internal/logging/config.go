package logging

import (
	"io"
)

// Config is a logging configuration.
type Config struct {
	// Level is the lowest level of log message that should be emitted. Any log
	// messages logged at the specified level or any level more severe will be
	// emitted. The default level is DEBUG.
	Level Level

	// Output is the destination for log messages. By default, it is os.Stderr.
	Output io.Writer
}
