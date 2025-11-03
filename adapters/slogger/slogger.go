package slogger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	logport "pkt.systems/logport"
)

// Options configures the slog adapter when constructing a logger.
type Options struct {
	Handler        slog.Handler
	HandlerOptions slog.HandlerOptions
	JSON           bool
	MinLevel       *logport.Level
}

// New returns a slog adapter that emits text output to w.
func New(w io.Writer) logport.ForLogging {
	return NewWithOptions(w, Options{})
}

// NewJSON returns a slog adapter that emits JSON output to w.
func NewJSON(w io.Writer) logport.ForLogging {
	return NewWithOptions(w, Options{JSON: true})
}

// NewWithHandler wraps an existing slog handler in a logport adapter.
func NewWithHandler(handler slog.Handler) logport.ForLogging {
	return newAdapter(slog.New(handler), handler, logport.TraceLevel)
}

// NewWithLogger wraps an existing slog.Logger in a logport adapter.
func NewWithLogger(logger *slog.Logger) logport.ForLogging {
	if logger == nil {
		return logport.NoopLogger()
	}
	return newAdapter(logger, logger.Handler(), logport.TraceLevel)
}

// NewWithOptions builds a slog adapter using the provided writer and options.
func NewWithOptions(w io.Writer, opts Options) logport.ForLogging {
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
	min := logport.TraceLevel
	if opts.MinLevel != nil {
		min = *opts.MinLevel
	}
	return newAdapter(slog.New(handler), handler, min)
}

// ContextWithLogger stores a configured slog adapter inside the context.
func ContextWithLogger(ctx context.Context, w io.Writer, opts Options) context.Context {
	return logport.ContextWithLogger(ctx, NewWithOptions(w, opts))
}

type adapter struct {
	logger          *slog.Logger
	handler         slog.Handler
	forcedLevel     *logport.Level
	minLevel        logport.Level
	includeLogLevel bool
}

func newAdapter(logger *slog.Logger, handler slog.Handler, min logport.Level) logport.ForLogging {
	if logger == nil {
		return logport.NoopLogger()
	}
	if handler == nil {
		handler = logger.Handler()
	}
	return adapter{logger: logger, handler: handler, minLevel: min}
}

func (a adapter) LogLevel(level logport.Level) logport.ForLogging {
	if level == logport.NoLevel {
		lvl := level
		return adapter{logger: a.logger, handler: a.handler, forcedLevel: &lvl, minLevel: a.minLevel, includeLogLevel: a.includeLogLevel}
	}
	return adapter{logger: a.logger, handler: a.handler, minLevel: level, includeLogLevel: a.includeLogLevel}
}

func (a adapter) LogLevelFromEnv(key string) logport.ForLogging {
	if level, ok := logport.LevelFromEnv(key); ok {
		return a.LogLevel(level)
	}
	return a
}

func (a adapter) WithLogLevel() logport.ForLogging {
	if a.includeLogLevel {
		return a
	}
	return adapter{logger: a.logger, handler: a.handler, forcedLevel: a.forcedLevel, minLevel: a.minLevel, includeLogLevel: true}
}

func (a adapter) With(keyvals ...any) logport.ForLogging {
	if len(keyvals) == 0 || a.logger == nil {
		return a
	}
	next := a.logger.With(keyvals...)
	return adapter{logger: next, handler: next.Handler(), forcedLevel: a.forcedLevel, minLevel: a.minLevel, includeLogLevel: a.includeLogLevel}
}

func (a adapter) WithTrace(ctx context.Context) logport.ForLogging {
	keyvals := logport.TraceKeyvalsFromContext(ctx)
	if len(keyvals) == 0 {
		return a
	}
	return a.With(keyvals...)
}

func (a adapter) Log(ctx context.Context, level slog.Level, msg string, keyvals ...any) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !a.shouldLog(logport.LevelFromSlog(level)) {
		return
	}
	keyvals = a.appendLogLevelKeyvals(keyvals)
	a.logger.Log(ctx, level, msg, keyvals...)
}

func (a adapter) Logp(level logport.Level, msg string, keyvals ...any) {
	if !a.shouldLog(level) {
		return
	}
	keyvals = a.appendLogLevelKeyvals(keyvals)
	a.logger.Log(context.Background(), portLevelToSlog(level), msg, keyvals...)
}

func (a adapter) Logs(level string, msg string, keyvals ...any) {
	if lvl, ok := logport.ParseLevel(level); ok {
		a.Logp(lvl, msg, keyvals...)
		return
	}
	a.Logp(logport.NoLevel, msg, keyvals...)
}

func (a adapter) Logf(level logport.Level, format string, args ...any) {
	a.Logp(level, fmt.Sprintf(format, args...))
}

func (a adapter) Debug(msg string, keyvals ...any) { a.Logp(logport.DebugLevel, msg, keyvals...) }
func (a adapter) Info(msg string, keyvals ...any)  { a.Logp(logport.InfoLevel, msg, keyvals...) }
func (a adapter) Warn(msg string, keyvals ...any)  { a.Logp(logport.WarnLevel, msg, keyvals...) }
func (a adapter) Error(msg string, keyvals ...any) { a.Logp(logport.ErrorLevel, msg, keyvals...) }

func (a adapter) Fatal(msg string, keyvals ...any) {
	a.Logp(logport.FatalLevel, msg, keyvals...)
	os.Exit(1)
}

func (a adapter) Panic(msg string, keyvals ...any) {
	a.Logp(logport.PanicLevel, msg, keyvals...)
	panic(msg)
}

func (a adapter) Trace(msg string, keyvals ...any) { a.Logp(logport.TraceLevel, msg, keyvals...) }

func (a adapter) Debugf(format string, args ...any) {
	a.Logp(logport.DebugLevel, fmt.Sprintf(format, args...))
}
func (a adapter) Infof(format string, args ...any) {
	a.Logp(logport.InfoLevel, fmt.Sprintf(format, args...))
}
func (a adapter) Warnf(format string, args ...any) {
	a.Logp(logport.WarnLevel, fmt.Sprintf(format, args...))
}
func (a adapter) Errorf(format string, args ...any) {
	a.Logp(logport.ErrorLevel, fmt.Sprintf(format, args...))
}
func (a adapter) Fatalf(format string, args ...any) { a.Fatal(fmt.Sprintf(format, args...)) }
func (a adapter) Panicf(format string, args ...any) { a.Panic(fmt.Sprintf(format, args...)) }
func (a adapter) Tracef(format string, args ...any) {
	a.Logp(logport.TraceLevel, fmt.Sprintf(format, args...))
}

func (a adapter) Write(p []byte) (int, error) {
	return logport.WriteToLogger(a, p)
}

func (a adapter) currentLevel() logport.Level {
	if a.forcedLevel != nil {
		return *a.forcedLevel
	}
	return a.minLevel
}

func (a adapter) appendLogLevelKeyvals(keyvals []any) []any {
	if !a.includeLogLevel {
		return keyvals
	}
	return append(keyvals, "loglevel", logport.LevelString(a.currentLevel()))
}

func (a adapter) shouldLog(level logport.Level) bool {
	if a.logger == nil {
		return false
	}
	effective := level
	if a.forcedLevel != nil {
		switch *a.forcedLevel {
		case logport.Disabled:
			return false
		case logport.NoLevel:
			effective = logport.InfoLevel
		default:
			effective = *a.forcedLevel
		}
	}
	if effective == logport.Disabled {
		return false
	}
	return effective >= a.minLevel
}

func (a adapter) Enabled(ctx context.Context, level slog.Level) bool {
	return a.shouldLog(logport.LevelFromSlog(level))
}

func (a adapter) Handle(ctx context.Context, record slog.Record) error {
	if !a.shouldLog(logport.LevelFromSlog(record.Level)) {
		return nil
	}
	if a.handler == nil {
		return nil
	}
	if a.includeLogLevel {
		record.AddAttrs(slog.String("loglevel", logport.LevelString(a.currentLevel())))
	}
	return a.handler.Handle(ctx, record)
}

func (a adapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	if a.handler == nil {
		return a
	}
	next := a.handler.WithAttrs(attrs)
	return adapter{logger: slog.New(next), handler: next, forcedLevel: a.forcedLevel, minLevel: a.minLevel, includeLogLevel: a.includeLogLevel}
}

func (a adapter) WithGroup(name string) slog.Handler {
	if a.handler == nil {
		return a
	}
	next := a.handler.WithGroup(name)
	return adapter{logger: slog.New(next), handler: next, forcedLevel: a.forcedLevel, minLevel: a.minLevel, includeLogLevel: a.includeLogLevel}
}

func portLevelToSlog(level logport.Level) slog.Level {
	switch level {
	case logport.TraceLevel:
		return slog.LevelDebug - 4
	case logport.DebugLevel:
		return slog.LevelDebug
	case logport.InfoLevel:
		return slog.LevelInfo
	case logport.WarnLevel:
		return slog.LevelWarn
	case logport.ErrorLevel:
		return slog.LevelError
	case logport.FatalLevel, logport.PanicLevel:
		return slog.LevelError + 4
	case logport.NoLevel:
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}

var _ logport.ForLogging = adapter{}
var _ slog.Handler = adapter{}
