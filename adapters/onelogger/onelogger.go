package onelogger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"time"

	onelogpkg "github.com/francoispqt/onelog"
	logport "pkt.systems/logport"
)

// Options controls how the onelog adapter formats and filters log output.
type Options struct {
	// Levels configures which onelog levels are enabled when constructing a fresh
	// logger. Defaults to onelog.ALL when zero.
	Levels uint8

	// ContextName, when set, constructs the underlying logger via onelog.NewContext
	// so every log entry nests fields under the provided key.
	ContextName string

	// Hook installs a function that runs for every log entry emitted by the
	// underlying logger.
	Hook func(onelogpkg.Entry)

	// ExitFunc overrides the logger's ExitFn; handy for testing fatal flows.
	ExitFunc onelogpkg.ExitFunc

	// Configure receives the newly created logger for additional tuning before it
	// is wrapped by the adapter.
	Configure func(*onelogpkg.Logger)

	// MinLevel optionally sets the minimum logport level the adapter should emit.
	// When nil, TraceLevel is used which keeps all messages enabled.
	MinLevel *logport.Level

	// TimeFormat controls the timestamp format applied via the adapter hook. When
	// empty, timestamps are disabled. Defaults to logport.DTGTimeFormat.
	TimeFormat string

	// DisableTimestamp disables the adapter-managed timestamp injection even if
	// TimeFormat is non-empty.
	DisableTimestamp bool
}

// New returns a ForLogging adapter backed by onelog with sensible defaults
// (structured JSON output with all levels enabled).
func New(w io.Writer) logport.ForLogging {
	return NewWithOptions(w, Options{TimeFormat: logport.DTGTimeFormat})
}

// NewWithOptions constructs an adapter using the provided writer and options.
func NewWithOptions(w io.Writer, opts Options) logport.ForLogging {
	levels := opts.Levels
	if levels == 0 {
		levels = onelogpkg.ALL
	}
	var logger *onelogpkg.Logger
	if opts.ContextName != "" {
		logger = onelogpkg.NewContext(w, levels, opts.ContextName)
	} else {
		logger = onelogpkg.New(w, levels)
	}
	if opts.ExitFunc != nil {
		logger.ExitFn = opts.ExitFunc
	}
	if opts.Configure != nil {
		opts.Configure(logger)
	}
	if hook := composeHook(opts); hook != nil {
		logger.Hook(hook)
	}
	minLevel := logport.TraceLevel
	if opts.MinLevel != nil {
		minLevel = *opts.MinLevel
	}
	return adapter{logger: logger, minLevel: minLevel}
}

// NewFromLogger wraps an existing onelog logger in the adapter.
func NewFromLogger(logger *onelogpkg.Logger) logport.ForLogging {
	if logger == nil {
		return adapter{}
	}
	return adapter{logger: logger, minLevel: logport.TraceLevel}
}

// NewWithLogger is an alias for NewFromLogger to mirror other adapters.
func NewWithLogger(logger *onelogpkg.Logger) logport.ForLogging {
	return NewFromLogger(logger)
}

// ContextWithLogger stores a new adapter constructed from the supplied options
// in the returned context.
func ContextWithLogger(ctx context.Context, w io.Writer, opts Options) context.Context {
	return logport.ContextWithLogger(ctx, NewWithOptions(w, opts))
}

type adapter struct {
	logger          *onelogpkg.Logger
	baseKeyvals     []any
	groups          []string
	forcedLevel     *logport.Level
	minLevel        logport.Level
	includeLogLevel bool
}

func (a adapter) LogLevelFromEnv(key string) logport.ForLogging {
	if level, ok := logport.LevelFromEnv(key); ok {
		return a.LogLevel(level)
	}
	return a
}

func (a adapter) LogLevel(level logport.Level) logport.ForLogging {
	switch level {
	case logport.NoLevel, logport.Disabled:
		lvl := level
		return adapter{logger: a.logger, baseKeyvals: a.baseKeyvals, groups: a.groups, forcedLevel: &lvl, minLevel: a.minLevel, includeLogLevel: a.includeLogLevel}
	default:
		return adapter{logger: a.logger, baseKeyvals: a.baseKeyvals, groups: a.groups, minLevel: level, includeLogLevel: a.includeLogLevel}
	}
}

func (a adapter) With(keyvals ...any) logport.ForLogging {
	if len(keyvals) == 0 {
		return a
	}
	addition := normalizeKeyvals(keyvals, nil)
	if len(addition) == 0 {
		return a
	}
	base := make([]any, 0, len(a.baseKeyvals)+len(addition))
	base = append(base, a.baseKeyvals...)
	base = append(base, addition...)
	return adapter{logger: a.logger, baseKeyvals: base, groups: a.groups, forcedLevel: a.forcedLevel, minLevel: a.minLevel, includeLogLevel: a.includeLogLevel}
}

func (a adapter) WithTrace(ctx context.Context) logport.ForLogging {
	keyvals := logport.TraceKeyvalsFromContext(ctx)
	if len(keyvals) == 0 {
		return a
	}
	return a.With(keyvals...)
}

func (a adapter) WithLogLevel() logport.ForLogging {
	if a.includeLogLevel {
		return a
	}
	return adapter{
		logger:          a.logger,
		baseKeyvals:     a.baseKeyvals,
		groups:          a.groups,
		forcedLevel:     a.forcedLevel,
		minLevel:        a.minLevel,
		includeLogLevel: true,
	}
}

func (a adapter) Log(_ context.Context, level slog.Level, msg string, keyvals ...any) {
	a.Logp(logport.LevelFromSlog(level), msg, keyvals...)
}

func (a adapter) Logp(level logport.Level, msg string, keyvals ...any) {
	switch level {
	case logport.TraceLevel:
		a.Trace(msg, keyvals...)
	case logport.DebugLevel:
		a.Debug(msg, keyvals...)
	case logport.InfoLevel:
		a.Info(msg, keyvals...)
	case logport.WarnLevel:
		a.Warn(msg, keyvals...)
	case logport.ErrorLevel:
		a.Error(msg, keyvals...)
	case logport.FatalLevel:
		a.Fatal(msg, keyvals...)
	case logport.PanicLevel:
		a.Panic(msg, keyvals...)
	case logport.NoLevel:
		a.log(logport.InfoLevel, msg, keyvals)
	case logport.Disabled:
		return
	default:
		a.Info(msg, keyvals...)
	}
}

func (a adapter) Logs(level string, msg string, keyvals ...any) {
	if lvl, ok := logport.ParseLevel(level); ok {
		a.Logp(lvl, msg, keyvals...)
		return
	}
	a.Logp(logport.NoLevel, msg, keyvals...)
}

func (a adapter) Logf(level logport.Level, format string, args ...any) {
	a.Logp(level, formatMessage(format, args...))
}

func (a adapter) Debug(msg string, keyvals ...any) { a.log(logport.DebugLevel, msg, keyvals) }

func (a adapter) Debugf(format string, args ...any) { a.Debug(formatMessage(format, args...)) }

func (a adapter) Info(msg string, keyvals ...any) { a.log(logport.InfoLevel, msg, keyvals) }

func (a adapter) Infof(format string, args ...any) { a.Info(formatMessage(format, args...)) }

func (a adapter) Warn(msg string, keyvals ...any) { a.log(logport.WarnLevel, msg, keyvals) }

func (a adapter) Warnf(format string, args ...any) { a.Warn(formatMessage(format, args...)) }

func (a adapter) Error(msg string, keyvals ...any) { a.log(logport.ErrorLevel, msg, keyvals) }

func (a adapter) Errorf(format string, args ...any) { a.Error(formatMessage(format, args...)) }

func (a adapter) Fatal(msg string, keyvals ...any) { a.logFatal(msg, keyvals) }

func (a adapter) Fatalf(format string, args ...any) { a.logFatal(formatMessage(format, args...), nil) }

func (a adapter) Panic(msg string, keyvals ...any) {
	a.log(logport.PanicLevel, msg, keyvals)
	panic(msg)
}

func (a adapter) Panicf(format string, args ...any) {
	a.Panic(formatMessage(format, args...))
}

func (a adapter) Trace(msg string, keyvals ...any) { a.log(logport.TraceLevel, msg, keyvals) }

func (a adapter) Tracef(format string, args ...any) { a.Trace(formatMessage(format, args...)) }

func (a adapter) Write(p []byte) (int, error) {
	return logport.WriteToLogger(a, p)
}

func (a adapter) log(level logport.Level, msg string, keyvals []any) {
	if !a.shouldLog(level) {
		return
	}
	entry := a.newChainEntry(level, msg)
	entry = addKeyvals(entry, a.baseKeyvals)
	addition := normalizeKeyvals(keyvals, a.groups)
	entry = addKeyvals(entry, addition)
	entry = a.appendLogLevel(entry)
	entry.Write()
}

func (a adapter) logFatal(msg string, keyvals []any) {
	if !a.shouldLog(logport.FatalLevel) {
		return
	}
	entry := a.newChainEntry(logport.FatalLevel, msg)
	entry = addKeyvals(entry, a.baseKeyvals)
	addition := normalizeKeyvals(keyvals, a.groups)
	entry = addKeyvals(entry, addition)
	entry = a.appendLogLevel(entry)
	entry.Write()
	if a.logger != nil && a.logger.ExitFn != nil {
		a.logger.ExitFn(1)
	}
}

func (a adapter) shouldLog(level logport.Level) bool {
	if a.logger == nil {
		return false
	}
	if a.forcedLevel != nil {
		switch *a.forcedLevel {
		case logport.Disabled:
			return false
		case logport.NoLevel:
			level = logport.InfoLevel
		default:
			level = *a.forcedLevel
		}
	}
	if level == logport.Disabled {
		return false
	}
	return level >= a.minLevel
}

func (a adapter) newChainEntry(level logport.Level, msg string) onelogpkg.ChainEntry {
	if a.logger == nil {
		return onelogpkg.ChainEntry{}
	}
	switch level {
	case logport.TraceLevel, logport.DebugLevel:
		return a.logger.DebugWith(msg)
	case logport.WarnLevel:
		return a.logger.WarnWith(msg)
	case logport.ErrorLevel:
		return a.logger.ErrorWith(msg)
	case logport.FatalLevel:
		return a.logger.FatalWith(msg)
	case logport.PanicLevel:
		return a.logger.ErrorWith(msg)
	case logport.NoLevel:
		return a.logger.InfoWith(msg)
	case logport.InfoLevel:
		fallthrough
	default:
		return a.logger.InfoWith(msg)
	}
}

func (a adapter) appendLogLevel(entry onelogpkg.ChainEntry) onelogpkg.ChainEntry {
	if !a.includeLogLevel {
		return entry
	}
	return entry.String("loglevel", logport.LevelString(a.currentLevel()))
}

func addKeyvals(entry onelogpkg.ChainEntry, keyvals []any) onelogpkg.ChainEntry {
	for i := 0; i+1 < len(keyvals); i += 2 {
		key := fmt.Sprint(keyvals[i])
		entry = addField(entry, key, keyvals[i+1])
	}
	return entry
}

func addField(entry onelogpkg.ChainEntry, key string, value any) onelogpkg.ChainEntry {
	switch v := value.(type) {
	case nil:
		return entry.Any(key, nil)
	case string:
		return entry.String(key, v)
	case fmt.Stringer:
		return entry.String(key, v.String())
	case error:
		return entry.Err(key, v)
	case bool:
		return entry.Bool(key, v)
	case int:
		return entry.Int(key, v)
	case int8:
		return entry.Int(key, int(v))
	case int16:
		return entry.Int(key, int(v))
	case int32:
		return entry.Int(key, int(v))
	case int64:
		return entry.Int64(key, v)
	case uint:
		if v <= math.MaxInt64 {
			return entry.Int64(key, int64(v))
		}
	case uint8:
		return entry.Int(key, int(v))
	case uint16:
		return entry.Int(key, int(v))
	case uint32:
		return entry.Int(key, int(v))
	case float32:
		return entry.Float(key, float64(v))
	case float64:
		return entry.Float(key, v)
	case []byte:
		return entry.String(key, string(v))
	}
	return entry.Any(key, value)
}

func (a adapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return a
	}
	addition := attrsToKeyvals(attrs, a.groups)
	if len(addition) == 0 {
		return a
	}
	base := make([]any, 0, len(a.baseKeyvals)+len(addition))
	base = append(base, a.baseKeyvals...)
	base = append(base, addition...)
	return adapter{
		logger:          a.logger,
		baseKeyvals:     base,
		groups:          a.groups,
		forcedLevel:     a.forcedLevel,
		minLevel:        a.minLevel,
		includeLogLevel: a.includeLogLevel,
	}
}

func (a adapter) WithGroup(name string) slog.Handler {
	if name == "" {
		return a
	}
	return adapter{
		logger:          a.logger,
		baseKeyvals:     a.baseKeyvals,
		groups:          appendGroup(a.groups, name),
		forcedLevel:     a.forcedLevel,
		minLevel:        a.minLevel,
		includeLogLevel: a.includeLogLevel,
	}
}

func (a adapter) Enabled(_ context.Context, level slog.Level) bool {
	if a.forcedLevel != nil && *a.forcedLevel == logport.Disabled {
		return false
	}
	return slogLevelToPort(level) >= a.minLevel
}

func (a adapter) Handle(_ context.Context, record slog.Record) error {
	level := slogLevelToPort(record.Level)
	if !a.shouldLog(level) {
		return nil
	}
	entry := a.newChainEntry(level, record.Message)
	entry = addKeyvals(entry, a.baseKeyvals)
	keyvals := recordToKeyvals(record, a.groups)
	entry = addKeyvals(entry, keyvals)
	entry = a.appendLogLevel(entry)
	entry.Write()
	return nil
}

func slogLevelToPort(level slog.Level) logport.Level {
	switch {
	case level < slog.LevelDebug:
		return logport.TraceLevel
	case level <= slog.LevelDebug:
		return logport.DebugLevel
	case level <= slog.LevelInfo:
		return logport.InfoLevel
	case level <= slog.LevelWarn:
		return logport.WarnLevel
	case level <= slog.LevelError:
		return logport.ErrorLevel
	default:
		return logport.FatalLevel
	}
}

func recordToKeyvals(record slog.Record, groups []string) []any {
	if record.NumAttrs() == 0 {
		return nil
	}
	keyvals := make([]any, 0, record.NumAttrs()*2)
	record.Attrs(func(attr slog.Attr) bool {
		keyvals = appendAttrKeyvals(keyvals, attr, groups)
		return true
	})
	return keyvals
}

func attrsToKeyvals(attrs []slog.Attr, groups []string) []any {
	if len(attrs) == 0 {
		return nil
	}
	keyvals := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		keyvals = appendAttrKeyvals(keyvals, attr, groups)
	}
	return keyvals
}

func normalizeKeyvals(keyvals []any, groups []string) []any {
	if len(keyvals) == 0 {
		return nil
	}
	normalized := make([]any, 0, len(keyvals)+len(keyvals)%2)
	pairIndex := 0
	for i := 0; i < len(keyvals); {
		switch v := keyvals[i].(type) {
		case slog.Attr:
			normalized = appendAttrKeyvals(normalized, v, groups)
			i++
		case []slog.Attr:
			for _, attr := range v {
				normalized = appendAttrKeyvals(normalized, attr, groups)
			}
			i++
			continue
		default:
			if i+1 < len(keyvals) {
				key := fmt.Sprint(v)
				if len(groups) > 0 {
					key = joinAttrKey(groups, key)
				}
				normalized = append(normalized, key, keyvals[i+1])
				pairIndex++
				i += 2
			} else {
				key := fmt.Sprintf("arg%d", pairIndex)
				if len(groups) > 0 {
					key = joinAttrKey(groups, key)
				}
				normalized = append(normalized, key, v)
				pairIndex++
				i++
			}
			continue
		}
	}
	return normalized
}

func appendAttrKeyvals(dst []any, attr slog.Attr, groups []string) []any {
	attr.Value = attr.Value.Resolve()
	switch attr.Value.Kind() {
	case slog.KindGroup:
		subGroups := groups
		if attr.Key != "" {
			subGroups = appendGroup(groups, attr.Key)
		}
		for _, nested := range attr.Value.Group() {
			dst = appendAttrKeyvals(dst, nested, subGroups)
		}
		return dst
	default:
		key := joinAttrKey(groups, attr.Key)
		return append(dst, key, attr.Value.Any())
	}
}

func appendGroup(groups []string, name string) []string {
	if name == "" {
		return groups
	}
	newGroups := make([]string, len(groups)+1)
	copy(newGroups, groups)
	newGroups[len(groups)] = name
	return newGroups
}

func joinAttrKey(groups []string, key string) string {
	if len(groups) == 0 {
		return key
	}
	parts := make([]string, 0, len(groups)+1)
	parts = append(parts, groups...)
	if key != "" {
		parts = append(parts, key)
	}
	return strings.Join(parts, ".")
}

func formatMessage(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

func (a adapter) currentLevel() logport.Level {
	if a.forcedLevel != nil {
		return *a.forcedLevel
	}
	if a.minLevel != 0 {
		return a.minLevel
	}
	return logport.InfoLevel
}

func composeHook(opts Options) func(onelogpkg.Entry) {
	var hooks []func(onelogpkg.Entry)
	if !opts.DisableTimestamp {
		format := opts.TimeFormat
		if format != "" {
			hooks = append(hooks, func(e onelogpkg.Entry) {
				ts := time.Now().UTC().Format(format)
				e.String("ts", ts)
			})
		}
	}
	if opts.Hook != nil {
		hooks = append(hooks, opts.Hook)
	}
	if len(hooks) == 0 {
		return nil
	}
	return func(e onelogpkg.Entry) {
		for _, hook := range hooks {
			hook(e)
		}
	}
}

var _ logport.ForLogging = adapter{}
