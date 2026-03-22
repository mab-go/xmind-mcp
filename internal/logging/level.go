package logging

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// A Level is a level of severity for a log message.
type Level uint8

const (
	// DebugLevel causes a logger to emit messages logged at "DEBUG" level or more
	// severe. It is typically only enabled when debugging or during development,
	// and usually results in very verbose logging output.
	DebugLevel Level = iota

	// InfoLevel causes a logger to emit messages logged at "INFO" level or more
	// severe. It is typically used for general operational entries about what's
	// going on inside an application.
	InfoLevel

	// WarnLevel causes a logger to emit messages logged at "WARN" level or more
	// severe. It is typically used for non-critical entries that deserve attention.
	WarnLevel

	// ErrorLevel causes a logger to emit messages logged at "ERROR" level or more
	// severe. It is typically used for errors that should definitely be noted, and
	// is commonly used for hooks to send errors to an error tracking service.
	ErrorLevel

	// FatalLevel causes a logger to emit messages logged at "FATAL" level or more
	// severe. Messages logged at this level cause a logger to log the message and
	// then call os.Exit(1). It will exit even if the logging level is set to PanicLevel.
	FatalLevel

	// PanicLevel causes a logger to only emit messages logged at "PANIC" level.
	// It is the highest level of severity.
	PanicLevel
)

// String implements fmt.Stringer for Level.
func (l Level) String() string {
	switch l {
	case PanicLevel:
		return "PANIC"
	case FatalLevel:
		return "FATAL"
	case ErrorLevel:
		return "ERROR"
	case WarnLevel:
		return "WARN"
	case InfoLevel:
		return "INFO"
	case DebugLevel:
		return "DEBUG"
	default:
		panic(fmt.Sprintf("invalid log level: %[1]d", l))
	}
}

func (l Level) logrusLevel() logrus.Level {
	switch l {
	case PanicLevel:
		return logrus.PanicLevel
	case FatalLevel:
		return logrus.FatalLevel
	case ErrorLevel:
		return logrus.ErrorLevel
	case WarnLevel:
		return logrus.WarnLevel
	case InfoLevel:
		return logrus.InfoLevel
	case DebugLevel:
		return logrus.DebugLevel
	default:
		panic(fmt.Sprintf("invalid log level: %[1]d", l))
	}
}
