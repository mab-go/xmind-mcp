package logging

var defaultConfig Config
var defaultLogger Logger

func init() {
	defaultConfig = Config{
		Level: DebugLevel,
	}

	defaultLogger = NewDefaultLogger()
}

// SetDefaultConfig sets the package's default logging configuration.
//
// Parameters:
//   - config: The configuration to set as the default.
func SetDefaultConfig(config Config) {
	defaultConfig = config
	defaultLogger = NewDefaultLogger()
}

// --- Default Logger Functions ------------------------------------------------

// Debug emits a "DEBUG" level log message using the default logger. If more than
// one argument is provided, the first argument is used as a string format template
// and the remaining arguments are used as string formatting parameters.
func Debug(args ...any) {
	defaultLogger.Debug(args...)
}

// Info emits an "INFO" level log message using the default logger. If more than
// one argument is provided, the first argument is used as a string format template
// and the remaining arguments are used as string formatting parameters.
func Info(args ...any) {
	defaultLogger.Info(args...)
}

// Warn emits a "WARN" level log message using the default logger. If more than
// one argument is provided, the first argument is used as a string format template
// and the remaining arguments are used as string formatting parameters.
func Warn(args ...any) {
	defaultLogger.Warn(args...)
}

// Error emits an "ERROR" level log message using the default logger. If more than
// one argument is provided, the first argument is used as a string format template
// and the remaining arguments are used as string formatting parameters.
func Error(args ...any) {
	defaultLogger.Error(args...)
}

// Fatal emits a "FATAL" level log message using the default logger. If more than
// one argument is provided, the first argument is used as a string format template
// and the remaining arguments are used as string formatting parameters.
func Fatal(args ...any) {
	defaultLogger.Fatal(args...)
}

// Panic emits a "PANIC" level log message using the default and then panics. If
// more than one argument is provided, the first argument is used as a string
// format template and the remaining arguments are used as string formatting
// parameters.
func Panic(args ...any) {
	defaultLogger.Panic(args...)
}

// WithField adds a field to the default logger and returns a new Logger.
func WithField(key string, value any) Logger {
	return defaultLogger.WithField(key, value)
}

// WithFields adds multiple fields to the default logger and returns a new Logger.
func WithFields(fields Fields) Logger {
	return defaultLogger.WithFields(fields)
}

// WithError adds a field called "error" to the default logger and returns a new
// Logger.
func WithError(err error) Logger {
	return defaultLogger.WithError(err)
}
