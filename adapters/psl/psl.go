package psl

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	logport "pkt.systems/logport"
	pslog "pkt.systems/pslog"
)

// Mode aliases pslog.Mode so existing code can continue using psl.Mode.
type Mode = pslog.Mode

const (
	// ModeConsole emits console-friendly output.
	ModeConsole Mode = pslog.ModeConsole
	// ModeStructured emits JSON output.
	ModeStructured Mode = pslog.ModeStructured
)

// Options preserves the legacy PSL options while mapping to pslog underneath.
type Options struct {
	Mode             Mode
	TimeFormat       string
	DisableTimestamp bool
	NoColor          bool
	ColorJSON        bool
	MinLevel         *logport.Level
	VerboseFields    bool
	UTC              bool
}

// New constructs a console logger backed by pslog.
func New(w io.Writer) logport.ForLogging {
	return NewWithOptions(w, Options{Mode: ModeConsole})
}

// NewStructured returns a structured JSON logger. Colours are enabled when possible.
func NewStructured(w io.Writer) logport.ForLogging {
	return NewWithOptions(w, Options{Mode: ModeStructured, ColorJSON: true})
}

// NewStructuredNoColor returns a structured JSON logger without colours.
func NewStructuredNoColor(w io.Writer) logport.ForLogging {
	return NewWithOptions(w, Options{Mode: ModeStructured, NoColor: true})
}

// NewWithOptions builds a pslog-backed adapter using the supplied settings.
func NewWithOptions(w io.Writer, opts Options) logport.ForLogging {
	psOpts := pslog.Options{
		Mode:             pslog.ModeConsole,
		TimeFormat:       opts.TimeFormat,
		DisableTimestamp: opts.DisableTimestamp,
		NoColor:          opts.NoColor,
		VerboseFields:    opts.VerboseFields,
		UTC:              opts.UTC,
	}
	if opts.Mode == ModeStructured {
		psOpts.Mode = pslog.ModeStructured
	}
	if opts.ColorJSON && opts.Mode == ModeStructured && !opts.NoColor {
		psOpts.ForceColor = true
	}
	minLevel := logport.TraceLevel
	if opts.MinLevel != nil {
		minLevel = *opts.MinLevel
	}
	psOpts.MinLevel = toPslogLevel(minLevel)

	logger := pslog.NewWithOptions(w, psOpts)
	return adapter{
		logger:   logger,
		minLevel: minLevel,
	}
}

// ContextWithLogger stores a logger built from the supplied options inside ctx.
func ContextWithLogger(ctx context.Context, w io.Writer, opts Options) context.Context {
	return logport.ContextWithLogger(ctx, NewWithOptions(w, opts))
}

type adapter struct {
	logger      pslog.Logger
	minLevel    logport.Level
	forcedLevel *logport.Level
	groups      []string
}

func (a adapter) LogLevelFromEnv(key string) logport.ForLogging {
	if level, ok := logport.LevelFromEnv(key); ok {
		return a.LogLevel(level)
	}
	return a
}

func (a adapter) LogLevel(level logport.Level) logport.ForLogging {
	next := adapter{
		logger: a.logger.LogLevel(toPslogLevel(level)),
		groups: cloneGroups(a.groups),
	}
	switch level {
	case logport.Disabled, logport.NoLevel:
		lvl := level
		next.forcedLevel = &lvl
		next.minLevel = a.minLevel
	default:
		next.minLevel = level
	}
	return next
}

func (a adapter) WithLogLevel() logport.ForLogging {
	return adapter{
		logger:      a.logger.WithLogLevel(),
		minLevel:    a.minLevel,
		forcedLevel: cloneForced(a.forcedLevel),
		groups:      cloneGroups(a.groups),
	}
}

func (a adapter) With(keyvals ...any) logport.ForLogging {
	if len(keyvals) == 0 {
		return a
	}
	promoted := promoteStaticKeyvals(keyvals)
	if len(promoted) == 0 {
		return a
	}
	return adapter{
		logger:      a.logger.With(promoted...),
		minLevel:    a.minLevel,
		forcedLevel: cloneForced(a.forcedLevel),
		groups:      cloneGroups(a.groups),
	}
}

func (a adapter) WithTrace(ctx context.Context) logport.ForLogging {
	keyvals := logport.TraceKeyvalsFromContext(ctx)
	if len(keyvals) == 0 {
		return a
	}
	return a.With(keyvals...)
}

func (a adapter) Log(ctx context.Context, level slog.Level, msg string, keyvals ...any) {
	a.Logp(logport.LevelFromSlog(level), msg, keyvals...)
}

func (a adapter) Logp(level logport.Level, msg string, keyvals ...any) {
	switch level {
	case logport.Disabled:
		return
	case logport.TraceLevel:
		a.logger.Trace(msg, keyvals...)
	case logport.DebugLevel:
		a.logger.Debug(msg, keyvals...)
	case logport.InfoLevel:
		a.logger.Info(msg, keyvals...)
	case logport.WarnLevel:
		a.logger.Warn(msg, keyvals...)
	case logport.ErrorLevel:
		a.logger.Error(msg, keyvals...)
	case logport.FatalLevel:
		a.logger.Fatal(msg, keyvals...)
	case logport.PanicLevel:
		a.logger.Panic(msg, keyvals...)
	case logport.NoLevel:
		a.logger.Log(pslog.NoLevel, msg, keyvals...)
	default:
		a.logger.Log(toPslogLevel(level), msg, keyvals...)
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

func (a adapter) Debug(msg string, keyvals ...any) { a.logger.Debug(msg, keyvals...) }
func (a adapter) Info(msg string, keyvals ...any)  { a.logger.Info(msg, keyvals...) }
func (a adapter) Warn(msg string, keyvals ...any)  { a.logger.Warn(msg, keyvals...) }
func (a adapter) Error(msg string, keyvals ...any) { a.logger.Error(msg, keyvals...) }

func (a adapter) Trace(msg string, keyvals ...any) { a.logger.Trace(msg, keyvals...) }

func (a adapter) Debugf(format string, args ...any) { a.logger.Debug(formatMessage(format, args...)) }
func (a adapter) Infof(format string, args ...any)  { a.logger.Info(formatMessage(format, args...)) }
func (a adapter) Warnf(format string, args ...any)  { a.logger.Warn(formatMessage(format, args...)) }
func (a adapter) Errorf(format string, args ...any) { a.logger.Error(formatMessage(format, args...)) }
func (a adapter) Tracef(format string, args ...any) { a.logger.Trace(formatMessage(format, args...)) }

func (a adapter) Fatal(msg string, keyvals ...any) {
	a.logger.Fatal(msg, keyvals...)
}

func (a adapter) Fatalf(format string, args ...any) {
	a.logger.Fatal(formatMessage(format, args...))
}

func (a adapter) Panic(msg string, keyvals ...any) {
	a.logger.Panic(msg, keyvals...)
}

func (a adapter) Panicf(format string, args ...any) {
	a.logger.Panic(formatMessage(format, args...))
}

func (a adapter) Write(p []byte) (int, error) {
	return logport.WriteToLogger(a, p)
}

func (a adapter) Enabled(_ context.Context, level slog.Level) bool {
	return a.shouldLog(logport.LevelFromSlog(level))
}

func (a adapter) Handle(_ context.Context, record slog.Record) error {
	level := logport.LevelFromSlog(record.Level)
	keyvals := recordToKeyvals(record, a.groups)
	a.logger.Log(toPslogLevel(level), record.Message, keyvals...)
	return nil
}

func (a adapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return a
	}
	keyvals := attrsToKeyvals(attrs, a.groups)
	if len(keyvals) == 0 {
		return a
	}
	return adapter{
		logger:      a.logger.With(promoteStaticKeyvals(keyvals)...),
		minLevel:    a.minLevel,
		forcedLevel: cloneForced(a.forcedLevel),
		groups:      cloneGroups(a.groups),
	}
}

func (a adapter) WithGroup(name string) slog.Handler {
	if name == "" {
		return a
	}
	return adapter{
		logger:      a.logger,
		minLevel:    a.minLevel,
		forcedLevel: cloneForced(a.forcedLevel),
		groups:      appendGroup(a.groups, name),
	}
}

func (a adapter) shouldLog(level logport.Level) bool {
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

func formatMessage(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

func cloneForced(level *logport.Level) *logport.Level {
	if level == nil {
		return nil
	}
	value := *level
	return &value
}

func cloneGroups(groups []string) []string {
	if len(groups) == 0 {
		return nil
	}
	clone := make([]string, len(groups))
	copy(clone, groups)
	return clone
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

func appendAttrKeyvals(dst []any, attr slog.Attr, groups []string) []any {
	attr.Value = attr.Value.Resolve()
	if attr.Value.Kind() == slog.KindGroup {
		subGroups := groups
		if attr.Key != "" {
			subGroups = appendGroup(groups, attr.Key)
		}
		for _, nested := range attr.Value.Group() {
			dst = appendAttrKeyvals(dst, nested, subGroups)
		}
		return dst
	}
	key := joinAttrKey(groups, attr.Key)
	return append(dst, key, attr.Value.Any())
}

func appendGroup(groups []string, name string) []string {
	if name == "" {
		return groups
	}
	next := make([]string, len(groups)+1)
	copy(next, groups)
	next[len(groups)] = name
	return next
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

func promoteStaticKeyvals(keyvals []any) []any {
	if len(keyvals) == 0 {
		return nil
	}
	return pslog.Keyvals(keyvals...)
}

func toPslogLevel(level logport.Level) pslog.Level {
	switch level {
	case logport.TraceLevel:
		return pslog.TraceLevel
	case logport.DebugLevel:
		return pslog.DebugLevel
	case logport.InfoLevel:
		return pslog.InfoLevel
	case logport.WarnLevel:
		return pslog.WarnLevel
	case logport.ErrorLevel:
		return pslog.ErrorLevel
	case logport.FatalLevel:
		return pslog.FatalLevel
	case logport.PanicLevel:
		return pslog.PanicLevel
	case logport.NoLevel:
		return pslog.NoLevel
	case logport.Disabled:
		return pslog.Disabled
	default:
		return pslog.InfoLevel
	}
}
