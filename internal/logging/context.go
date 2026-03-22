package logging

import (
	"context"
)

// The key type is unexported to prevent collisions with context keys defined in
// other packages.
type key int

// loggerKey is the context key for the logger instance. Its value of zero is
// arbitrary. If this package defined other context keys, they would have
// different integer values.
const loggerKey key = 0

// FromContext extracts the Logger from ctx, if present. If no Logger is present
// within the context, a new default Logger is created.
func FromContext(ctx context.Context) (logger Logger, created bool) {
	// ctx.Value returns nil if ctx has no value for the key; the Logger
	// type assertion returns ok=false for nil.
	logger, ok := ctx.Value(loggerKey).(Logger)

	if !ok {
		logger = NewDefaultLogger()
		created = true
	}

	return
}

// NewContext returns a new Context carrying logger.
func NewContext(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}
