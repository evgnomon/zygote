package util

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// NewLogger creates a new logger with format based on daemon mode
func NewLogger() Logger {
	var logger zerolog.Logger
	logLevelEnv := strings.ToLower(os.Getenv("Z_LOG_LEVEL"))

	// Default log level
	var level = zerolog.InfoLevel

	// Map environment variable to zerolog level
	switch logLevelEnv {
	case "debug":
		level = zerolog.DebugLevel
	case "info":
		level = zerolog.InfoLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	case "fatal":
		level = zerolog.FatalLevel
	}

	// Set global log level
	zerolog.SetGlobalLevel(level)

	// Check if running in daemon mode (e.g., via environment variable)
	isDaemon := os.Getenv("DAEMON_MODE") == "true"

	if isDaemon {
		// JSON output for daemon mode
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		// Text output for non-daemon mode (human-readable)
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	}

	return &zerologLogger{logger: logger}
} // M represents key-value pairs for structured logging with any value type.
type M map[string]any

// Logger defines the logging interface with a single Args parameter.
type Logger interface {
	Debug(msg string, args ...M)
	Info(msg string, args ...M)
	Warning(msg string, args ...M)
	Error(msg string, err error, args ...M)
	Fatal(msg string, args ...M)
	FatalIfErr(msg string, err error, args ...M)
}

// zerologLogger wraps a Zerolog logger to implement the Logger interface.
type zerologLogger struct {
	logger zerolog.Logger
}

// addFields adds key-value pairs from Args to a Zerolog event in sorted order.
func (z *zerologLogger) addFields(event *zerolog.Event, args M) *zerolog.Event {
	if args == nil {
		return event
	}

	// Collect and sort keys
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Process fields in sorted order
	for _, key := range keys {
		value := args[key]
		if key == "error" {
			if err, ok := value.(error); ok && err != nil {
				// Log the error message
				event = event.Str("error", err.Error())

				// Check for stack trace (using pkg/errors)
				type stackTracer interface {
					StackTrace() errors.StackTrace
				}
				if stackErr, ok := err.(stackTracer); ok {
					// Format stack trace as a slice of strings
					stack := stackErr.StackTrace()
					stackLines := make([]string, 0, len(stack))
					for _, frame := range stack {
						stackLines = append(stackLines, fmt.Sprintf("%+s:%d", frame, frame))
					}
					event = event.Strs("stack", stackLines)
				}
				continue
			}
		}

		// Handle other types
		switch v := value.(type) {
		case string:
			event = event.Str(key, v)
		case int:
			event = event.Int(key, v)
		case int64:
			event = event.Int64(key, v)
		case float64:
			event = event.Float64(key, v)
		case bool:
			event = event.Bool(key, v)
		case time.Time:
			event = event.Time(key, v)
		case error:
			event = event.Str(key, v.Error())
		case nil:
			event = event.Interface(key, nil)
		default:
			event = event.Interface(key, v)
		}
	}

	return event
}

// Debug logs a message at Debug level with an optional Args map.
func (z *zerologLogger) Debug(msg string, args ...M) {
	event := z.logger.Debug()
	for _, arg := range args {
		if arg == nil {
			continue
		}
		event = z.addFields(event, arg)
	}
	event.Msg(msg)
}

// Info logs a message at Info level with an optional Args map.
func (z *zerologLogger) Info(msg string, args ...M) {
	event := z.logger.Info()
	for _, arg := range args {
		if arg == nil {
			continue
		}
		event = z.addFields(event, arg)
	}
	event.Msg(msg)
}

// Warning logs a message at Warn level with an optional Args map.
func (z *zerologLogger) Warning(msg string, args ...M) {
	event := z.logger.Warn()
	for _, arg := range args {
		if arg == nil {
			continue
		}
		event = z.addFields(event, arg)
	}
	event.Msg(msg)
}

// Error logs a message at Error level with an optional Args map.
func (z *zerologLogger) Error(msg string, err error, args ...M) {
	event := z.logger.Error()
	event = event.Err(err)
	for _, arg := range args {
		if arg == nil {
			continue
		}
		event = z.addFields(event, arg)
	}
	event.Msg(msg)
}

// Fatal logs a message at Fatal level with an optional Args map and exits.
func (z *zerologLogger) Fatal(msg string, args ...M) {
	event := z.logger.Fatal()
	for _, arg := range args {
		if arg == nil {
			continue
		}
		event = z.addFields(event, arg)
	}
	event.Msg(msg)
}

func (z *zerologLogger) FatalIfErr(msg string, err error, args ...M) {
	if err != nil {
		event := z.logger.Fatal()
		event = event.Err(err)
		for _, arg := range args {
			if arg == nil {
				continue
			}
			event = z.addFields(event, arg)
		}
		event.Msg(msg)
	}
	z.Debug(msg, args...)
}

func WrapError(err error) M {
	if err == nil {
		return nil
	}
	return M{"error": err}
}
