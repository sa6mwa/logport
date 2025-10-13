// logport defines a minimal logging port that can be satisfied by multiple back
// ends. The core ForLogging interface embeds slog.Handler so that adapters can
// be used both as structured logging receivers and as drop-in slog handlers and
// much more.
//
// Typical usage pairs the port with one of the provided adapters:
//
//	logger := zerologger.New(os.Stdout)
//	logger.Info("Ready", "addr", addr)
//
// or, when a context-based workflow is preferred:
//
//	ctx := port.ContextWithLogger(context.Background(), charmlogger.New(os.Stdout))
//	l := port.LoggerFromContext(ctx)
//	l.With("request_id", "abc-123").Warn("slow response", "duration", took)
//
// Because adapters implement slog.Handler, they can also be passed directly to
// slog.New:
//
//	slogLogger := slog.New(zerologger.New(os.Stdout))
//	slogLogger.Info("hello from slog", slog.String("component", "worker"))
package logport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
)

// Level defines log levels.
type Level int8

const (
	// DebugLevel defines debug log level.
	DebugLevel Level = iota
	// InfoLevel defines info log level.
	InfoLevel
	// WarnLevel defines warn log level.
	WarnLevel
	// ErrorLevel defines error log level.
	ErrorLevel
	// FatalLevel defines fatal log level.
	FatalLevel
	// PanicLevel defines panic log level.
	PanicLevel
	// NoLevel defines an absent log level.
	NoLevel
	// Disabled disables the logger.
	Disabled
	TraceLevel Level = -1
)

// ForLogging is a logger with superpowers wrapping several logging backends
// that plug in as adapters to this port. Prefer the structured helpers over the
// *f variants when you need maximum efficiency; the formatting versions allocate
// for convenience when ergonomics matter more than throughput.
type ForLogging interface {
	slog.Handler
	io.Writer

	// LogLevelFromEnv configures the logger's level using the value of key in the
	// environment. Recognised values are the same as ParseLevel. Missing or
	// invalid values leave the logger unchanged.
	LogLevelFromEnv(key string) ForLogging

	// LogLevel returns a logger derived from the receiver whose minimum level is
	// set to level. The receiver itself is not modified.
	LogLevel(Level) ForLogging

	// WithLogLevel returns a logger that carries a `loglevel` field describing
	// the logger's effective severity.
	WithLogLevel() ForLogging

	// With returns a logger that includes the supplied key/value pairs on every
	// subsequent log entry. The receiver remains untouched.
	With(keyvals ...any) ForLogging

	// WithTrace utilizes otel/trace to add trace_id and span_id keyvals to the
	// logger derived from an OpenTelemetry span context (set by
	// otel.Tracer("...").Start(origCtx, "...").
	//
	// Example:
	//
	//	tracer := otel.Tracer("worker")
	//	func handle(ctx context.Context, logger logport.ForLogging) {
	//		ctx, span := tracer.Start(ctx, "handle")
	//		defer span.End()
	//
	//		logger.WithTrace(ctx).With("component", "api").Info("processing request")
	//	}
	//
	// The returned logger includes "trace_id" and "span_id" keyvals when the
	// context carries a valid OpenTelemetry span.
	WithTrace(ctx context.Context) ForLogging

	// Log mirrors slog.Logger.Log and emits msg at the provided slog level using
	// the supplied context and key/value pairs.
	Log(ctx context.Context, level slog.Level, msg string, keyvals ...any)

	// Logp emits msg at the supplied logport level.
	Logp(level Level, msg string, keyvals ...any)

	// Logs emits msg using the level encoded in the string. Unknown or empty
	// values fall back to NoLevel semantics.
	Logs(level string, msg string, keyvals ...any)

	// Logf formats msg using fmt.Sprintf semantics and logs it at the supplied
	// logport level.
	Logf(level Level, format string, v ...any)

	ForLoggingMinimalSubset

	// Trace logs msg at TraceLevel (below DebugLevel).
	Trace(msg string, keyvals ...any)
	// Fatal logs msg at FatalLevel and terminates the process when the backend
	// supports it.
	Fatal(msg string, keyvals ...any)
	// Panic logs msg at PanicLevel and panics when the backend supports it.
	Panic(msg string, keyvals ...any)

	// Debugf logs a formatted message at DebugLevel.
	Debugf(format string, v ...any)
	// Infof logs a formatted message at InfoLevel.
	Infof(format string, v ...any)
	// Warnf logs a formatted message at WarnLevel.
	Warnf(format string, v ...any)
	// Errorf logs a formatted message at ErrorLevel.
	Errorf(format string, v ...any)
	// Fatalf logs a formatted message at FatalLevel.
	Fatalf(format string, v ...any)
	// Panicf logs a formatted message at PanicLevel.
	Panicf(format string, v ...any)
	// Tracef logs a formatted message at TraceLevel.
	Tracef(format string, v ...any)
}

type ForLoggingMinimalSubset interface {
	// Debug logs msg at DebugLevel.
	Debug(msg string, keyvals ...any)
	// Info logs msg at InfoLevel.
	Info(msg string, keyvals ...any)
	// Warn logs msg at WarnLevel.
	Warn(msg string, keyvals ...any)
	// Error logs msg at ErrorLevel.
	Error(msg string, keyvals ...any)
}

var DTGTimeFormat string = "021504"

type loggerContextKey struct{}

// ContextWithLogger returns a child context carrying the supplied
// logger implementation.
func ContextWithLogger(ctx context.Context, logger ForLogging) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

// LoggerFromContext extracts a logger implementation from context if
// present or returns a NoopLogger.
func LoggerFromContext(ctx context.Context) ForLogging {
	if ctx == nil {
		return noopLogger{}
	}
	if logger, ok := ctx.Value(loggerContextKey{}).(ForLogging); ok && logger != nil {
		return logger
	}
	return noopLogger{}
}

// LogLogger wraps a ForLogging implementation into a stdlib *log.Logger.
func LogLogger(logger ForLogging) *log.Logger {
	if logger == nil {
		logger = noopLogger{}
	}
	return log.New(logger, "", 0)
}

// LogLoggerWithLevel wraps a ForLogging implementation into a stdlib
// *log.Logger that pins every emitted entry to level. The wrapped logger still
// benefits from prefix classification to strip level tokens from msg, but the
// detected level is ignored in favour of the supplied one.
func LogLoggerWithLevel(logger ForLogging, level Level) *log.Logger {
	if logger == nil {
		logger = noopLogger{}
	}
	return log.New(levelPinnedWriter{logger: logger, level: level}, "", 0)
}

// WriteToLogger routes bytes to logger.Logp, splitting on newlines and
// detecting severity prefixes. It always reports len(p) to comply with
// io.Writer semantics.
func WriteToLogger(logger ForLogging, p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if logger == nil {
		return len(p), nil
	}
	writeBytesToLogger(logger, p, nil)
	return len(p), nil
}

type levelPinnedWriter struct {
	logger ForLogging
	level  Level
}

func (w levelPinnedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.logger == nil {
		return len(p), nil
	}
	level := w.level
	writeBytesToLogger(w.logger, p, &level)
	return len(p), nil
}

func writeBytesToLogger(logger ForLogging, p []byte, override *Level) {
	rest := p
	for len(rest) > 0 {
		line := rest
		if idx := bytes.IndexByte(line, '\n'); idx >= 0 {
			line = line[:idx]
			rest = rest[idx+1:]
		} else {
			rest = nil
		}
		raw := strings.TrimRight(string(line), "\r")
		if strings.TrimSpace(raw) == "" {
			continue
		}
		level, msg := classifyLogLine(raw)
		if override != nil {
			level = *override
		}
		if msg == "" {
			msg = strings.TrimSpace(raw)
		}
		if msg == "" {
			continue
		}
		logger.Logp(level, msg)
	}
}

// NoopLogger provides a logger implementation that discards all log messages.
func NoopLogger() ForLogging {
	return noopLogger{}
}

type noopLogger struct{}

func (noopLogger) LogLevel(Level) ForLogging                       { return noopLogger{} }
func (n noopLogger) LogLevelFromEnv(string) ForLogging             { return n }
func (noopLogger) WithLogLevel() ForLogging                        { return noopLogger{} }
func (noopLogger) Log(context.Context, slog.Level, string, ...any) {}
func (noopLogger) Logp(Level, string, ...any)                      {}
func (noopLogger) Logs(string, string, ...any)                     {}
func (noopLogger) Logf(Level, string, ...any)                      {}
func (noopLogger) With(keyvals ...any) ForLogging                  { return noopLogger{} }
func (noopLogger) WithTrace(context.Context) ForLogging            { return noopLogger{} }
func (noopLogger) Debug(msg string, keyvals ...any)                {}
func (noopLogger) Debugf(string, ...any)                           {}
func (noopLogger) Info(msg string, keyvals ...any)                 {}
func (noopLogger) Infof(string, ...any)                            {}
func (noopLogger) Warn(msg string, keyvals ...any)                 {}
func (noopLogger) Warnf(string, ...any)                            {}
func (noopLogger) Error(msg string, keyvals ...any)                {}
func (noopLogger) Errorf(string, ...any)                           {}
func (noopLogger) Fatal(msg string, keyvals ...any)                { os.Exit(1) }
func (noopLogger) Fatalf(string, ...any)                           { os.Exit(1) }
func (noopLogger) Panic(msg string, keyvals ...any)                { panic(msg) }
func (noopLogger) Panicf(format string, v ...any)                  { panic(fmt.Sprintf(format, v...)) }
func (noopLogger) Trace(msg string, keyvals ...any)                {}
func (noopLogger) Tracef(string, ...any)                           {}
func (noopLogger) Write(p []byte) (int, error)                     { return len(p), nil }

func (noopLogger) Enabled(context.Context, slog.Level) bool  { return false }
func (noopLogger) Handle(context.Context, slog.Record) error { return nil }
func (noopLogger) WithAttrs([]slog.Attr) slog.Handler        { return noopLogger{} }
func (noopLogger) WithGroup(string) slog.Handler             { return noopLogger{} }

// ParseLevel converts a textual level into a Level value. It accepts values
// such as "trace", "debug", "info", "warn", "warning", "error", "fatal",
// "panic", "no", "nolevel", "disabled", and "off" (case insensitive).
func ParseLevel(value string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "trace":
		return TraceLevel, true
	case "debug":
		return DebugLevel, true
	case "info":
		return InfoLevel, true
	case "warn", "warning":
		return WarnLevel, true
	case "error":
		return ErrorLevel, true
	case "fatal":
		return FatalLevel, true
	case "panic":
		return PanicLevel, true
	case "no", "nolevel", "none":
		return NoLevel, true
	case "disabled", "disable", "off":
		return Disabled, true
	default:
		return InfoLevel, false
	}
}

// LevelFromEnv looks up key in the environment and parses it into a Level.
func LevelFromEnv(key string) (Level, bool) {
	if key == "" {
		return InfoLevel, false
	}
	value, ok := os.LookupEnv(key)
	if !ok {
		return InfoLevel, false
	}
	return ParseLevel(value)
}

// LevelFromSlog translates a slog.Level into the closest logport Level.
func LevelFromSlog(level slog.Level) Level {
	switch {
	case level < slog.LevelDebug:
		return TraceLevel
	case level < slog.LevelInfo:
		return DebugLevel
	case level < slog.LevelWarn:
		return InfoLevel
	case level < slog.LevelError:
		return WarnLevel
	case level < slog.LevelError+4:
		return ErrorLevel
	default:
		return FatalLevel
	}
}

// LevelString returns the canonical string representation of a Level.
func LevelString(level Level) string {
	switch level {
	case TraceLevel:
		return "trace"
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case FatalLevel:
		return "fatal"
	case PanicLevel:
		return "panic"
	case NoLevel:
		return "nolevel"
	case Disabled:
		return "disabled"
	default:
		return "info"
	}
}
