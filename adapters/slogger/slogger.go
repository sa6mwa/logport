package slogger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	port "pkt.systems/logport"
)

type Options struct {
	Handler        slog.Handler
	HandlerOptions slog.HandlerOptions
	JSON           bool
	MinLevel       *port.Level
}

func New(w io.Writer) port.ForLogging {
	return NewWithOptions(w, Options{})
}

func NewJSON(w io.Writer) port.ForLogging {
	return NewWithOptions(w, Options{JSON: true})
}

func NewWithHandler(handler slog.Handler) port.ForLogging {
	return newAdapter(slog.New(handler), handler, port.TraceLevel)
}

func NewWithLogger(logger *slog.Logger) port.ForLogging {
	if logger == nil {
		return port.NoopLogger()
	}
	return newAdapter(logger, logger.Handler(), port.TraceLevel)
}

func NewWithOptions(w io.Writer, opts Options) port.ForLogging {
	handler := opts.Handler
	if handler == nil {
		if w == nil {
			w = io.Discard
		}
		handlerOpts := opts.HandlerOptions
		if opts.JSON {
			handler = slog.NewJSONHandler(w, &handlerOpts)
		} else {
			handler = slog.NewTextHandler(w, &handlerOpts)
		}
	}
	min := port.TraceLevel
	if opts.MinLevel != nil {
		min = *opts.MinLevel
	}
	return newAdapter(slog.New(handler), handler, min)
}

func ContextWithLogger(ctx context.Context, w io.Writer, opts Options) context.Context {
	return port.ContextWithLogger(ctx, NewWithOptions(w, opts))
}

type adapter struct {
	logger      *slog.Logger
	handler     slog.Handler
	forcedLevel *port.Level
	minLevel    port.Level
}

func newAdapter(logger *slog.Logger, handler slog.Handler, min port.Level) port.ForLogging {
	if logger == nil {
		return port.NoopLogger()
	}
	if handler == nil {
		handler = logger.Handler()
	}
	return adapter{logger: logger, handler: handler, minLevel: min}
}

func (a adapter) LogLevel(level port.Level) port.ForLogging {
	if level == port.NoLevel {
		lvl := level
		return adapter{logger: a.logger, handler: a.handler, forcedLevel: &lvl, minLevel: a.minLevel}
	}
	return adapter{logger: a.logger, handler: a.handler, minLevel: level}
}

func (a adapter) LogLevelFromEnv(key string) port.ForLogging {
	if level, ok := port.LevelFromEnv(key); ok {
		return a.LogLevel(level)
	}
	return a
}

func (a adapter) WithLogLevel() port.ForLogging {
	return a.With("loglevel", port.LevelString(a.currentLevel()))
}

func (a adapter) With(keyvals ...any) port.ForLogging {
	if len(keyvals) == 0 || a.logger == nil {
		return a
	}
	next := a.logger.With(keyvals...)
	return adapter{logger: next, handler: next.Handler(), forcedLevel: a.forcedLevel, minLevel: a.minLevel}
}

func (a adapter) Log(ctx context.Context, level slog.Level, msg string, keyvals ...any) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !a.shouldLog(port.LevelFromSlog(level)) {
		return
	}
	a.logger.Log(ctx, level, msg, keyvals...)
}

func (a adapter) Logp(level port.Level, msg string, keyvals ...any) {
	if !a.shouldLog(level) {
		return
	}
	a.logger.Log(context.Background(), portLevelToSlog(level), msg, keyvals...)
}

func (a adapter) Logs(level string, msg string, keyvals ...any) {
	if lvl, ok := port.ParseLevel(level); ok {
		a.Logp(lvl, msg, keyvals...)
		return
	}
	a.Logp(port.NoLevel, msg, keyvals...)
}

func (a adapter) Logf(level port.Level, format string, args ...any) {
	a.Logp(level, fmt.Sprintf(format, args...))
}

func (a adapter) Debug(msg string, keyvals ...any) { a.Logp(port.DebugLevel, msg, keyvals...) }
func (a adapter) Info(msg string, keyvals ...any)  { a.Logp(port.InfoLevel, msg, keyvals...) }
func (a adapter) Warn(msg string, keyvals ...any)  { a.Logp(port.WarnLevel, msg, keyvals...) }
func (a adapter) Error(msg string, keyvals ...any) { a.Logp(port.ErrorLevel, msg, keyvals...) }

func (a adapter) Fatal(msg string, keyvals ...any) {
	a.Logp(port.FatalLevel, msg, keyvals...)
	os.Exit(1)
}

func (a adapter) Panic(msg string, keyvals ...any) {
	a.Logp(port.PanicLevel, msg, keyvals...)
	panic(msg)
}

func (a adapter) Trace(msg string, keyvals ...any) { a.Logp(port.TraceLevel, msg, keyvals...) }

func (a adapter) Debugf(format string, args ...any) {
	a.Logp(port.DebugLevel, fmt.Sprintf(format, args...))
}
func (a adapter) Infof(format string, args ...any) {
	a.Logp(port.InfoLevel, fmt.Sprintf(format, args...))
}
func (a adapter) Warnf(format string, args ...any) {
	a.Logp(port.WarnLevel, fmt.Sprintf(format, args...))
}
func (a adapter) Errorf(format string, args ...any) {
	a.Logp(port.ErrorLevel, fmt.Sprintf(format, args...))
}
func (a adapter) Fatalf(format string, args ...any) { a.Fatal(fmt.Sprintf(format, args...)) }
func (a adapter) Panicf(format string, args ...any) { a.Panic(fmt.Sprintf(format, args...)) }
func (a adapter) Tracef(format string, args ...any) {
	a.Logp(port.TraceLevel, fmt.Sprintf(format, args...))
}

func (a adapter) Write(p []byte) (int, error) {
	return port.WriteToLogger(a, p)
}

func (a adapter) currentLevel() port.Level {
	if a.forcedLevel != nil {
		return *a.forcedLevel
	}
	return a.minLevel
}

func (a adapter) shouldLog(level port.Level) bool {
	if a.logger == nil {
		return false
	}
	effective := level
	if a.forcedLevel != nil {
		switch *a.forcedLevel {
		case port.Disabled:
			return false
		case port.NoLevel:
			effective = port.InfoLevel
		default:
			effective = *a.forcedLevel
		}
	}
	if effective == port.Disabled {
		return false
	}
	return effective >= a.minLevel
}

func (a adapter) Enabled(ctx context.Context, level slog.Level) bool {
	return a.shouldLog(port.LevelFromSlog(level))
}

func (a adapter) Handle(ctx context.Context, record slog.Record) error {
	if !a.shouldLog(port.LevelFromSlog(record.Level)) {
		return nil
	}
	if a.handler == nil {
		return nil
	}
	return a.handler.Handle(ctx, record)
}

func (a adapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	if a.handler == nil {
		return a
	}
	next := a.handler.WithAttrs(attrs)
	return adapter{logger: slog.New(next), handler: next, forcedLevel: a.forcedLevel, minLevel: a.minLevel}
}

func (a adapter) WithGroup(name string) slog.Handler {
	if a.handler == nil {
		return a
	}
	next := a.handler.WithGroup(name)
	return adapter{logger: slog.New(next), handler: next, forcedLevel: a.forcedLevel, minLevel: a.minLevel}
}

func portLevelToSlog(level port.Level) slog.Level {
	switch level {
	case port.TraceLevel:
		return slog.LevelDebug - 4
	case port.DebugLevel:
		return slog.LevelDebug
	case port.InfoLevel:
		return slog.LevelInfo
	case port.WarnLevel:
		return slog.LevelWarn
	case port.ErrorLevel:
		return slog.LevelError
	case port.FatalLevel, port.PanicLevel:
		return slog.LevelError + 4
	case port.NoLevel:
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}

var _ port.ForLogging = adapter{}
var _ slog.Handler = adapter{}
