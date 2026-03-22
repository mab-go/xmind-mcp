package logging

// Logger provides methods for logging messages.
type Logger interface {
	// Debug emits a "DEBUG" level log message. If more than one argument is provided,
	// the first argument is used as a string format template and the remaining arguments
	// are used as string formatting parameters.
	Debug(args ...any)

	// Info emits an "INFO" level log message. If more than one argument is provided,
	// the first argument is used as a string format template and the remaining arguments
	// are used as string formatting parameters.
	Info(args ...any)

	// Warn emits a "WARN" level log message. If more than one argument is provided,
	// the first argument is used as a string format template and the remaining arguments
	// are used as string formatting parameters.
	Warn(args ...any)

	// Error emits an "ERROR" level log message. If more than one argument is provided,
	// the first argument is used as a string format template and the remaining arguments
	// are used as string formatting parameters.
	Error(args ...any)

	// Fatal emits a "FATAL" level log message. If more than one argument is provided,
	// the first argument is used as a string format template and the remaining arguments
	// are used as string formatting parameters.
	Fatal(args ...any)

	// Panic emits a "PANIC" level log message and then panics. If more than one argument
	// is provided, the first argument is used as a string format template and the remaining
	// arguments are used as string formatting parameters.
	Panic(args ...any)

	// WithField adds a field to the logger and returns a new Logger.
	WithField(key string, value any) Logger

	// WithFields adds multiple fields to the logger and returns a new Logger.
	WithFields(fields Fields) Logger

	// WithError adds a field called "error" to the logger and returns a new Logger.
	WithError(err error) Logger

	// Copy returns a copy of the logger. The copy will have the same fields as the
	// original logger.
	Copy() Logger
}
