package zaplogger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	logport "pkt.systems/logport"
)

type adapter struct {
	logger          *zap.Logger
	groups          []string
	minLevel        *zapcore.Level
	configuredLevel *logport.Level
	includeLogLevel bool
}

// Options controls zap-backed adapter configuration.
type Options struct {
	// Level controls the minimum level enabled by the adapter. Defaults to
	// zapcore.InfoLevel.
	Level zapcore.LevelEnabler

	// Encoder allows callers to supply a fully configured encoder. When nil, the
	// adapter builds a JSON encoder from EncoderConfig or, if that is nil, from
	// zap.NewProductionEncoderConfig().
	Encoder zapcore.Encoder

	// EncoderConfig customizes the encoder when Encoder is nil.
	EncoderConfig *zapcore.EncoderConfig

	// Fields are attached to the constructed logger.
	Fields []zap.Field

	// ZapOptions are forwarded to zap.New.
	ZapOptions []zap.Option

	// Configure, when non-nil, receives the constructed logger and returns the
	// logger the adapter should use. This makes it easy to apply tweaks such as
	// Named or WithOptions.
	Configure func(*zap.Logger) *zap.Logger
}

// New returns a zap-backed ForLogging implementation that writes JSON logs to
// the provided writer using sensible defaults.
func New(w io.Writer) logport.ForLogging {
	return NewWithOptions(w, Options{})
}

// NewWithOptions returns a zap-backed ForLogging implementation with custom
// configuration applied.
func NewWithOptions(w io.Writer, opts Options) logport.ForLogging {
	logger := buildLogger(w, opts)
	if logger == nil {
		return logport.NoopLogger()
	}
	return adapter{logger: logger}
}

// NewFromLogger wraps an existing zap.Logger so it satisfies logport.ForLogging.
func NewFromLogger(logger *zap.Logger) logport.ForLogging {
	if logger == nil {
		return logport.NoopLogger()
	}
	return adapter{logger: logger}
}

// ContextWithLogger installs a zap-backed logger into the supplied context.
func ContextWithLogger(ctx context.Context, w io.Writer, opts Options) context.Context {
	return logport.ContextWithLogger(ctx, NewWithOptions(w, opts))
}

func buildLogger(w io.Writer, opts Options) *zap.Logger {
	if w == nil {
		w = io.Discard
	}
	encoder := opts.Encoder
	if encoder == nil {
		cfg := opts.EncoderConfig
		if cfg == nil {
			defaultCfg := zap.NewProductionEncoderConfig()
			cfg = &defaultCfg
		}
		encoder = zapcore.NewJSONEncoder(*cfg)
	}
	level := opts.Level
	if level == nil {
		level = zapcore.InfoLevel
	}
	core := zapcore.NewCore(encoder, zapcore.AddSync(w), level)
	logger := zap.New(core, opts.ZapOptions...)
	if len(opts.Fields) > 0 {
		logger = logger.With(opts.Fields...)
	}
	if opts.Configure != nil {
		logger = opts.Configure(logger)
	}
	return logger
}

func (a adapter) With(keyvals ...any) logport.ForLogging {
	if a.logger == nil || len(keyvals) == 0 {
		return a
	}
	fields := keyvalsToFields(a.groups, keyvals)
	if len(fields) == 0 {
		return a
	}
	return adapter{logger: a.logger.With(fields...), groups: a.groups, minLevel: a.minLevel, configuredLevel: a.configuredLevel, includeLogLevel: a.includeLogLevel}
}

func (a adapter) WithTrace(ctx context.Context) logport.ForLogging {
	keyvals := logport.TraceKeyvalsFromContext(ctx)
	if len(keyvals) == 0 {
		return a
	}
	return a.With(keyvals...)
}

func (a adapter) LogLevelFromEnv(key string) logport.ForLogging {
	if level, ok := logport.LevelFromEnv(key); ok {
		return a.LogLevel(level)
	}
	return a
}

func (a adapter) LogLevel(level logport.Level) logport.ForLogging {
	if a.logger == nil {
		return a
	}
	if level == logport.NoLevel {
		lvl := zapcore.DebugLevel
		configured := level
		return adapter{logger: a.logger, groups: a.groups, minLevel: &lvl, configuredLevel: &configured, includeLogLevel: a.includeLogLevel}
	}
	zapLevel := portLevelToZap(level)
	configured := level
	return adapter{logger: a.logger, groups: a.groups, minLevel: &zapLevel, configuredLevel: &configured, includeLogLevel: a.includeLogLevel}
}

func (a adapter) WithLogLevel() logport.ForLogging {
	if a.includeLogLevel {
		return a
	}
	return adapter{logger: a.logger, groups: a.groups, minLevel: a.minLevel, configuredLevel: a.configuredLevel, includeLogLevel: true}
}

func (a adapter) Log(ctx context.Context, level slog.Level, msg string, keyvals ...any) {
	a.Logp(logport.LevelFromSlog(level), msg, keyvals...)
}

func (a adapter) Logp(level logport.Level, msg string, keyvals ...any) {
	switch level {
	case logport.TraceLevel, logport.DebugLevel:
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
		a.Debug(msg, keyvals...)
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

func (a adapter) Debug(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	if !a.shouldLog(logport.DebugLevel) {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	fields = a.appendLogLevelField(fields)
	a.logger.Debug(msg, fields...)
}

func (a adapter) Debugf(format string, args ...any) {
	a.Debug(formatMessage(format, args...))
}

func (a adapter) Info(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	if !a.shouldLog(logport.InfoLevel) {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	fields = a.appendLogLevelField(fields)
	a.logger.Info(msg, fields...)
}

func (a adapter) Infof(format string, args ...any) {
	a.Info(formatMessage(format, args...))
}

func (a adapter) Warn(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	if !a.shouldLog(logport.WarnLevel) {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	fields = a.appendLogLevelField(fields)
	a.logger.Warn(msg, fields...)
}

func (a adapter) Warnf(format string, args ...any) {
	a.Warn(formatMessage(format, args...))
}

func (a adapter) Error(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	if !a.shouldLog(logport.ErrorLevel) {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	fields = a.appendLogLevelField(fields)
	a.logger.Error(msg, fields...)
}

func (a adapter) Errorf(format string, args ...any) {
	a.Error(formatMessage(format, args...))
}

func (a adapter) Fatal(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	fields = a.appendLogLevelField(fields)
	a.logger.Fatal(msg, fields...)
}

func (a adapter) Fatalf(format string, args ...any) {
	a.Fatal(formatMessage(format, args...))
}

func (a adapter) Panic(msg string, keyvals ...any) {
	if a.logger == nil {
		panic(msg)
	}
	fields := keyvalsToFields(a.groups, keyvals)
	fields = a.appendLogLevelField(fields)
	a.logger.Panic(msg, fields...)
}

func (a adapter) Panicf(format string, args ...any) {
	a.Panic(formatMessage(format, args...))
}

func (a adapter) Trace(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	if !a.shouldLog(logport.TraceLevel) {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	fields = a.appendLogLevelField(fields)
	a.logger.Debug(msg, fields...)
}

func (a adapter) Tracef(format string, args ...any) {
	a.Trace(formatMessage(format, args...))
}

func (a adapter) Write(p []byte) (int, error) {
	return logport.WriteToLogger(a, p)
}

func (a adapter) Enabled(_ context.Context, level slog.Level) bool {
	if a.logger == nil {
		return false
	}
	zapLevel := slogLevelToZap(level)
	if a.minLevel != nil && zapLevel < *a.minLevel {
		return false
	}
	return a.logger.Core().Enabled(zapLevel)
}

func (a adapter) Handle(_ context.Context, record slog.Record) error {
	if a.logger == nil {
		return nil
	}
	zapLevel := slogLevelToZap(record.Level)
	if a.minLevel != nil && zapLevel < *a.minLevel {
		return nil
	}
	if ce := a.logger.Check(zapLevel, record.Message); ce != nil {
		fields := recordToFields(record, a.groups)
		fields = a.appendLogLevelField(fields)
		ce.Write(fields...)
	}
	return nil
}

func (a adapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	if a.logger == nil || len(attrs) == 0 {
		return a
	}
	fields := attrsToFields(attrs, a.groups)
	if len(fields) == 0 {
		return a
	}
	return adapter{logger: a.logger.With(fields...), groups: a.groups, minLevel: a.minLevel, configuredLevel: a.configuredLevel, includeLogLevel: a.includeLogLevel}
}

func (a adapter) WithGroup(name string) slog.Handler {
	if name == "" {
		return a
	}
	return adapter{logger: a.logger, groups: appendGroup(a.groups, name), minLevel: a.minLevel, configuredLevel: a.configuredLevel, includeLogLevel: a.includeLogLevel}
}

func slogLevelToZap(level slog.Level) zapcore.Level {
	switch {
	case level < slog.LevelDebug:
		return zapcore.DebugLevel
	case level < slog.LevelInfo:
		return zapcore.DebugLevel
	case level < slog.LevelWarn:
		return zapcore.InfoLevel
	case level < slog.LevelError:
		return zapcore.WarnLevel
	case level < slog.LevelError+4:
		return zapcore.ErrorLevel
	default:
		return zapcore.FatalLevel
	}
}

func (a adapter) currentLevel() logport.Level {
	if a.configuredLevel != nil {
		return *a.configuredLevel
	}
	if a.minLevel != nil {
		return zapLevelToPort(*a.minLevel)
	}
	return logport.InfoLevel
}

func (a adapter) appendLogLevelField(fields []zap.Field) []zap.Field {
	if !a.includeLogLevel {
		return fields
	}
	return append(fields, zap.String("loglevel", logport.LevelString(a.currentLevel())))
}

func keyvalsToFields(groups []string, keyvals []any) []zap.Field {
	if len(keyvals) == 0 {
		return nil
	}
	fields := make([]zap.Field, 0, len(keyvals)/2+len(keyvals)%2)
	pairIndex := 0
	for i := 0; i < len(keyvals); {
		switch v := keyvals[i].(type) {
		case slog.Attr:
			fields = appendAttrField(fields, v, groups)
			i++
		case []slog.Attr:
			for _, attr := range v {
				fields = appendAttrField(fields, attr, groups)
			}
			i++
			continue
		default:
			if i+1 < len(keyvals) {
				key := fmt.Sprint(v)
				fields = append(fields, zap.Any(joinKey(groups, key), keyvals[i+1]))
				pairIndex++
				i += 2
			} else {
				key := fmt.Sprintf("arg%d", pairIndex)
				fields = append(fields, zap.Any(joinKey(groups, key), v))
				pairIndex++
				i++
			}
			continue
		}
	}
	return fields
}

func zapLevelToPort(level zapcore.Level) logport.Level {
	switch level {
	case zapcore.DebugLevel:
		return logport.DebugLevel
	case zapcore.InfoLevel:
		return logport.InfoLevel
	case zapcore.WarnLevel:
		return logport.WarnLevel
	case zapcore.ErrorLevel:
		return logport.ErrorLevel
	case zapcore.DPanicLevel, zapcore.FatalLevel:
		return logport.FatalLevel
	case zapcore.PanicLevel:
		return logport.PanicLevel
	case zapcore.InvalidLevel:
		return logport.Disabled
	default:
		return logport.InfoLevel
	}
}

func attrsToFields(attrs []slog.Attr, groups []string) []zap.Field {
	if len(attrs) == 0 {
		return nil
	}
	fields := make([]zap.Field, 0, len(attrs))
	for _, attr := range attrs {
		fields = appendAttrField(fields, attr, groups)
	}
	return fields
}

func recordToFields(record slog.Record, groups []string) []zap.Field {
	if record.NumAttrs() == 0 {
		return nil
	}
	fields := make([]zap.Field, 0, record.NumAttrs())
	record.Attrs(func(attr slog.Attr) bool {
		fields = appendAttrField(fields, attr, groups)
		return true
	})
	return fields
}

func appendAttrField(dst []zap.Field, attr slog.Attr, groups []string) []zap.Field {
	attr.Value = attr.Value.Resolve()
	switch attr.Value.Kind() {
	case slog.KindGroup:
		subGroups := groups
		if attr.Key != "" {
			subGroups = appendGroup(groups, attr.Key)
		}
		for _, nested := range attr.Value.Group() {
			dst = appendAttrField(dst, nested, subGroups)
		}
		return dst
	default:
		key := joinKey(groups, attr.Key)
		return append(dst, zap.Any(key, attr.Value.Any()))
	}
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

func joinKey(groups []string, key string) string {
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

func (a adapter) shouldLog(level logport.Level) bool {
	if a.logger == nil {
		return false
	}
	if a.minLevel == nil {
		return true
	}
	zapLevel := portLevelToZap(level)
	return zapLevel >= *a.minLevel
}

func formatMessage(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

func portLevelToZap(level logport.Level) zapcore.Level {
	switch level {
	case logport.TraceLevel, logport.DebugLevel, logport.NoLevel:
		return zapcore.DebugLevel
	case logport.InfoLevel:
		return zapcore.InfoLevel
	case logport.WarnLevel:
		return zapcore.WarnLevel
	case logport.ErrorLevel:
		return zapcore.ErrorLevel
	case logport.FatalLevel:
		return zapcore.FatalLevel
	case logport.PanicLevel:
		return zapcore.PanicLevel
	case logport.Disabled:
		return zapcore.InvalidLevel
	default:
		return zapcore.InfoLevel
	}
}

var _ logport.ForLogging = adapter{}
