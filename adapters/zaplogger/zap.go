package zaplogger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	port "github.com/sa6mwa/logport"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type adapter struct {
	logger   *zap.Logger
	groups   []string
	minLevel *zapcore.Level
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
func New(w io.Writer) port.ForLogging {
	return NewWithOptions(w, Options{})
}

// NewWithOptions returns a zap-backed ForLogging implementation with custom
// configuration applied.
func NewWithOptions(w io.Writer, opts Options) port.ForLogging {
	logger := buildLogger(w, opts)
	if logger == nil {
		return port.NoopLogger()
	}
	return adapter{logger: logger}
}

// NewFromLogger wraps an existing zap.Logger so it satisfies port.ForLogging.
func NewFromLogger(logger *zap.Logger) port.ForLogging {
	if logger == nil {
		return port.NoopLogger()
	}
	return adapter{logger: logger}
}

// ContextWithLogger installs a zap-backed logger into the supplied context.
func ContextWithLogger(ctx context.Context, w io.Writer, opts Options) context.Context {
	return port.ContextWithLogger(ctx, NewWithOptions(w, opts))
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

func (a adapter) With(keyvals ...any) port.ForLogging {
	if a.logger == nil || len(keyvals) == 0 {
		return a
	}
	fields := keyvalsToFields(a.groups, keyvals)
	if len(fields) == 0 {
		return a
	}
	return adapter{logger: a.logger.With(fields...), groups: a.groups, minLevel: a.minLevel}
}

func (a adapter) LogLevel(level port.Level) port.ForLogging {
	if a.logger == nil {
		return a
	}
	if level == port.NoLevel {
		lvl := zapcore.DebugLevel
		return adapter{logger: a.logger, groups: a.groups, minLevel: &lvl}
	}
	lvl := portLevelToZap(level)
	return adapter{logger: a.logger, groups: a.groups, minLevel: &lvl}
}

func (a adapter) Debug(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	if !a.shouldLog(port.DebugLevel) {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	a.logger.Debug(msg, fields...)
}

func (a adapter) Info(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	if !a.shouldLog(port.InfoLevel) {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	a.logger.Info(msg, fields...)
}

func (a adapter) Warn(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	if !a.shouldLog(port.WarnLevel) {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	a.logger.Warn(msg, fields...)
}

func (a adapter) Error(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	if !a.shouldLog(port.ErrorLevel) {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	a.logger.Error(msg, fields...)
}

func (a adapter) Fatal(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	a.logger.Fatal(msg, fields...)
}

func (a adapter) Panic(msg string, keyvals ...any) {
	if a.logger == nil {
		panic(msg)
	}
	fields := keyvalsToFields(a.groups, keyvals)
	a.logger.Panic(msg, fields...)
}

func (a adapter) Trace(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	if !a.shouldLog(port.TraceLevel) {
		return
	}
	fields := keyvalsToFields(a.groups, keyvals)
	a.logger.Debug(msg, fields...)
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
	return adapter{logger: a.logger.With(fields...), groups: a.groups, minLevel: a.minLevel}
}

func (a adapter) WithGroup(name string) slog.Handler {
	if name == "" {
		return a
	}
	return adapter{logger: a.logger, groups: appendGroup(a.groups, name), minLevel: a.minLevel}
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

func (a adapter) shouldLog(level port.Level) bool {
	if a.logger == nil {
		return false
	}
	if a.minLevel == nil {
		return true
	}
	zapLevel := portLevelToZap(level)
	return zapLevel >= *a.minLevel
}

func portLevelToZap(level port.Level) zapcore.Level {
	switch level {
	case port.TraceLevel, port.DebugLevel, port.NoLevel:
		return zapcore.DebugLevel
	case port.InfoLevel:
		return zapcore.InfoLevel
	case port.WarnLevel:
		return zapcore.WarnLevel
	case port.ErrorLevel:
		return zapcore.ErrorLevel
	case port.FatalLevel:
		return zapcore.FatalLevel
	case port.PanicLevel:
		return zapcore.PanicLevel
	case port.Disabled:
		return zapcore.InvalidLevel
	default:
		return zapcore.InfoLevel
	}
}

var _ port.ForLogging = adapter{}
