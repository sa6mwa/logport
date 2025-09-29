// logport defines a minimal logging port that can be satisfied by multiple back
// ends. The core ForLogging interface embeds slog.Handler so that adapters can
// be used both as structured logging receivers and as drop-in slog handlers.
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
	"context"
	"log/slog"
)

// ForLogging mirrors the capabilities of structured loggers such as
// charmbracelet/log.
type ForLogging interface {
	slog.Handler
	With(keyvals ...any) ForLogging
	Debug(msg string, keyvals ...any)
	Info(msg string, keyvals ...any)
	Warn(msg string, keyvals ...any)
	Error(msg string, keyvals ...any)
	Fatal(msg string, keyvals ...any)
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

// NoopLogger provides a logger implementation that discards all log messages.
func NoopLogger() ForLogging {
	return noopLogger{}
}

type noopLogger struct{}

func (noopLogger) With(keyvals ...any) ForLogging   { return noopLogger{} }
func (noopLogger) Debug(msg string, keyvals ...any) {}
func (noopLogger) Info(msg string, keyvals ...any)  {}
func (noopLogger) Warn(msg string, keyvals ...any)  {}
func (noopLogger) Error(msg string, keyvals ...any) {}
func (noopLogger) Fatal(msg string, keyvals ...any) {}

func (noopLogger) Enabled(context.Context, slog.Level) bool  { return false }
func (noopLogger) Handle(context.Context, slog.Record) error { return nil }
func (noopLogger) WithAttrs([]slog.Attr) slog.Handler        { return noopLogger{} }
func (noopLogger) WithGroup(string) slog.Handler             { return noopLogger{} }
