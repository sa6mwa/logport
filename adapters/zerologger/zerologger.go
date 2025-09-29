package zerologger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/zerolog"
	port "github.com/sa6mwa/logport"
)

type adapter struct {
	logger zerolog.Logger
	groups []string
}

// Options controls how the zerolog adapter formats log output.
type Options struct {
	// ConfigureWriter allows callers to modify the zerolog.ConsoleWriter that is
	// created for the adapter. The supplied writer already has Out set to the
	// io.Writer provided to NewWithOptions and uses the Options' TimeFormat (which
	// defaults to port.DTGTimeFormat).
	ConfigureWriter func(*zerolog.ConsoleWriter)

	// Level, when non-nil, sets the minimum level for the adapter's logger.
	Level *zerolog.Level

	// NoColor disables the colorized output. When false (the default), colors are
	// automatically disabled if the provided writer does not appear to be a
	// terminal.
	NoColor bool

	// TimeFormat according to time.Time, default is port.DTGTimeFormat.
	TimeFormat string

	// DisableTimestamp skips attaching timestamp to every log entry.
	DisableTimestamp bool
}

// New returns a zerolog-backed ForLogging implementation with sensible
// defaults that produce the familiar, colored console output.
func New(w io.Writer) port.ForLogging {
	return NewWithOptions(w, Options{})
}

func NewFromLogger(logger zerolog.Logger) port.ForLogging {
	return adapter{logger: logger}
}

// NewWithOptions returns a zerolog-backed ForLogging implementation with the
// supplied writer and options applied.
func NewWithOptions(w io.Writer, o Options) port.ForLogging {
	if o.TimeFormat == "" {
		o.TimeFormat = port.DTGTimeFormat
	}
	noColor := o.NoColor || !isTerminal(w)
	writer := zerolog.ConsoleWriter{
		Out:        w,
		NoColor:    noColor,
		TimeFormat: o.TimeFormat,
	}
	if o.ConfigureWriter != nil {
		o.ConfigureWriter(&writer)
	}
	logger := zerolog.New(writer)
	if !o.DisableTimestamp {
		logger = logger.With().Timestamp().Logger()
	}
	if o.Level != nil {
		logger = logger.Level(*o.Level)
	}
	return adapter{logger: logger}
}

// ContextWithLogger returns a new context carrying a zerolog-backed logger.
func ContextWithLogger(ctx context.Context, w io.Writer, opts Options) context.Context {
	return port.ContextWithLogger(ctx, NewWithOptions(w, opts))
}

func (a adapter) With(keyvals ...any) port.ForLogging {
	if len(keyvals) == 0 {
		return a
	}
	ctx := a.logger.With()
	if fields := fieldsFromKeyvals(keyvals); len(fields) > 0 {
		ctx = ctx.Fields(fields)
	}
	return adapter{logger: ctx.Logger(), groups: a.groups}
}

func (a adapter) Debug(msg string, keyvals ...any) {
	event := a.logger.Debug()
	addFields(event, keyvals)
	event.Msg(msg)
}

func (a adapter) Info(msg string, keyvals ...any) {
	event := a.logger.Info()
	addFields(event, keyvals)
	event.Msg(msg)
}

func (a adapter) Warn(msg string, keyvals ...any) {
	event := a.logger.Warn()
	addFields(event, keyvals)
	event.Msg(msg)
}

func (a adapter) Error(msg string, keyvals ...any) {
	event := a.logger.Error()
	addFields(event, keyvals)
	event.Msg(msg)
}

func (a adapter) Fatal(msg string, keyvals ...any) {
	event := a.logger.Fatal()
	addFields(event, keyvals)
	event.Msg(msg)
}

func fieldsFromKeyvals(keyvals []any) map[string]any {
	if len(keyvals) == 0 {
		return nil
	}
	fields := make(map[string]any, len(keyvals)/2+len(keyvals)%2)
	pairs := len(keyvals) / 2
	for i := range pairs {
		key := fmt.Sprint(keyvals[2*i])
		fields[key] = keyvals[2*i+1]
	}
	if len(keyvals)%2 != 0 {
		fields[fmt.Sprintf("arg%d", pairs)] = keyvals[len(keyvals)-1]
	}
	return fields
}

func addFields(event *zerolog.Event, keyvals []any) {
	if event == nil || len(keyvals) == 0 {
		return
	}
	fields := fieldsFromKeyvals(keyvals)
	for key, value := range fields {
		switch v := value.(type) {
		case error:
			event.AnErr(key, v)
		case fmt.Stringer:
			event.Stringer(key, v)
		case bool:
			event.Bool(key, v)
		case int:
			event.Int(key, v)
		case int8:
			event.Int8(key, v)
		case int16:
			event.Int16(key, v)
		case int32:
			event.Int32(key, v)
		case int64:
			event.Int64(key, v)
		case uint:
			event.Uint(key, v)
		case uint8:
			event.Uint8(key, v)
		case uint16:
			event.Uint16(key, v)
		case uint32:
			event.Uint32(key, v)
		case uint64:
			event.Uint64(key, v)
		case float32:
			event.Float32(key, v)
		case float64:
			event.Float64(key, v)
		case time.Time:
			event.Time(key, v)
		case time.Duration:
			event.Dur(key, v)
		case string:
			event.Str(key, v)
		case []byte:
			event.Bytes(key, v)
		case zerolog.LogObjectMarshaler:
			event.Object(key, v)
		case zerolog.LogArrayMarshaler:
			event.Array(key, v)
		default:
			event.Interface(key, v)
		}
	}
}

var _ port.ForLogging = adapter{}

func (a adapter) Enabled(_ context.Context, level slog.Level) bool {
	return slogLevelToZero(level) >= a.logger.GetLevel()
}

func (a adapter) Handle(_ context.Context, record slog.Record) error {
	event := a.logger.WithLevel(slogLevelToZero(record.Level))
	keyvals := recordToKeyvals(record, a.groups)
	addFields(event, keyvals)
	event.Msg(record.Message)
	return nil
}

func (a adapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return a
	}
	ctx := a.logger.With()
	if fields := fieldsFromKeyvals(attrsToKeyvals(attrs, a.groups)); len(fields) > 0 {
		ctx = ctx.Fields(fields)
	}
	return adapter{logger: ctx.Logger(), groups: a.groups}
}

func (a adapter) WithGroup(name string) slog.Handler {
	if name == "" {
		return a
	}
	return adapter{logger: a.logger, groups: appendGroup(a.groups, name)}
}

func slogLevelToZero(level slog.Level) zerolog.Level {
	switch {
	case level <= slog.LevelDebug:
		return zerolog.DebugLevel
	case level <= slog.LevelInfo:
		return zerolog.InfoLevel
	case level <= slog.LevelWarn:
		return zerolog.WarnLevel
	case level <= slog.LevelError:
		return zerolog.ErrorLevel
	default:
		return zerolog.FatalLevel
	}
}

func recordToKeyvals(record slog.Record, groups []string) []any {
	keyvals := make([]any, 0, record.NumAttrs()*2)
	record.Attrs(func(attr slog.Attr) bool {
		keyvals = appendAttrKeyvals(keyvals, attr, groups)
		return true
	})
	return keyvals
}

func attrsToKeyvals(attrs []slog.Attr, groups []string) []any {
	keyvals := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		keyvals = appendAttrKeyvals(keyvals, attr, groups)
	}
	return keyvals
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
