// Package logging provides a logging system for the application.
package logging

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Fields is a map of strings to any type. It is used to pass to Logger.WithFields.
type Fields map[string]any

// logrusLogger is the default implementation of Logger. It is backed by the logrus
// logging package.
type logrusLogger struct {
	entry *logrus.Entry
}

// NewLogger returns a new Logger.
//
// Parameters:
//   - config: The configuration for the logger.
//
// Returns:
//   - Logger: A new Logger configured with the given configuration.
func NewLogger(config Config) Logger {
	log := logrus.New()
	log.Level = config.Level.logrusLevel()

	output := config.Output
	if output == nil {
		output = os.Stderr
	}

	log.SetOutput(output)
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	return &logrusLogger{entry: logrus.NewEntry(log)}
}

// NewDefaultLogger returns a new logger that uses the package's default
// configuration.
//
// Returns:
//   - Logger: A new Logger configured with the default configuration.
func NewDefaultLogger() Logger {
	return NewLogger(defaultConfig)
}

// Ensure that *logrusLogger implements Logger.
var _ Logger = (*logrusLogger)(nil)

// Debug emits a "DEBUG" level log message. If more than one argument is provided,
// the first argument is used as a string format template and the remaining arguments
// are used as string formatting parameters.
func (log *logrusLogger) Debug(args ...any) {
	log.entry.Debug(args...)
}

// Info emits an "INFO" level log message. If more than one argument is provided,
// the first argument is used as a string format template and the remaining arguments
// are used as string formatting parameters.
func (log *logrusLogger) Info(args ...any) {
	log.entry.Info(args...)
}

// Warn emits a "WARN" level log message. If more than one argument is provided,
// the first argument is used as a string format template and the remaining arguments
// are used as string formatting parameters.
func (log *logrusLogger) Warn(args ...any) {
	log.entry.Warn(args...)
}

// Error emits an "ERROR" level log message. If more than one argument is provided,
// the first argument is used as a string format template and the remaining arguments
// are used as string formatting parameters.
func (log *logrusLogger) Error(args ...any) {
	log.entry.Error(args...)
}

// Fatal emits a "FATAL" level log message. If more than one argument is provided,
// the first argument is used as a string format template and the remaining arguments
// are used as string formatting parameters.
func (log *logrusLogger) Fatal(args ...any) {
	log.entry.Fatal(args...)
}

// Panic emits a "PANIC" level log message and then panics. If more than one argument
// is provided, the first argument is used as a string format template and the remaining
// arguments are used as string formatting parameters.
func (log *logrusLogger) Panic(args ...any) {
	log.entry.Panic(args...)
}

// WithField adds a field to the logger and returns a new Logger.
func (log *logrusLogger) WithField(key string, value any) Logger {
	return &logrusLogger{entry: log.entry.WithField(key, value)}
}

// WithFields adds multiple fields to the logger and returns a new Logger.
func (log *logrusLogger) WithFields(fields Fields) Logger {
	return &logrusLogger{entry: log.entry.WithFields(logrus.Fields(fields))}
}

// WithError adds a field called "error" to the logger and returns a new Logger.
func (log *logrusLogger) WithError(err error) Logger {
	return &logrusLogger{entry: log.entry.WithError(err)}
}

// Copy returns a copy of the logger.
func (log *logrusLogger) Copy() Logger {
	// Use Dup() to create a proper copy with all existing fields preserved
	return &logrusLogger{entry: log.entry.Dup()}
}
