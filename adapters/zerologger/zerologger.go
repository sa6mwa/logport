package zerologger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/zerolog"
	port "github.com/sa6mwa/logport"
)

type adapter struct {
	logger      zerolog.Logger
	groups      []string
	forcedLevel *port.Level
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

	// Structured switches the adapter to structured (JSON) output instead of
	// zerolog's console writer. When true, ConfigureWriter is ignored and the
	// adapter emits zerolog JSON events directly.
	Structured bool
}

// optionsJSON mirrors Options for JSON serialization without the ConfigureWriter.
type optionsJSON struct {
	Level            *zerolog.Level `json:"level,omitempty"`
	NoColor          bool           `json:"noColor,omitempty"`
	TimeFormat       string         `json:"timeFormat,omitempty"`
	DisableTimestamp bool           `json:"disableTimestamp,omitempty"`
	Structured       bool           `json:"structured,omitempty"`
}

// MarshalJSON supports encoding Options while omitting ConfigureWriter.
func (o Options) MarshalJSON() ([]byte, error) {
	return json.Marshal(optionsJSON{
		Level:            o.Level,
		NoColor:          o.NoColor,
		TimeFormat:       o.TimeFormat,
		DisableTimestamp: o.DisableTimestamp,
		Structured:       o.Structured,
	})
}

// UnmarshalJSON restores Options from JSON while leaving ConfigureWriter unset.
func (o *Options) UnmarshalJSON(data []byte) error {
	var aux optionsJSON
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	o.Level = aux.Level
	o.NoColor = aux.NoColor
	o.TimeFormat = aux.TimeFormat
	o.DisableTimestamp = aux.DisableTimestamp
	o.Structured = aux.Structured
	o.ConfigureWriter = nil
	return nil
}

// New returns a zerolog-backed ForLogging implementation with sensible
// defaults that produce zerolog's familiar colored console output.
func New(w io.Writer) port.ForLogging {
	return NewWithOptions(w, Options{})
}

// NewStructured returns a zerolog-backed ForLogging implementation that writes
// structured JSON to the supplied writer.
func NewStructured(w io.Writer) port.ForLogging {
	return NewWithOptions(w, Options{Structured: true})
}

func NewFromLogger(logger zerolog.Logger) port.ForLogging {
	return adapter{logger: logger}
}

// NewWithOptions returns a zerolog-backed ForLogging implementation using the
// supplied writer and options. The default behaviour is console-friendly output;
// set Options.Structured to true for JSON emission.
func NewWithOptions(w io.Writer, o Options) port.ForLogging {
	useConsole := !o.Structured
	if useConsole {
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
		if o.DisableTimestamp {
			writer.PartsExclude = appendUnique(writer.PartsExclude, zerolog.TimestampFieldName)
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

	logger := zerolog.New(w)
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
	if fields := fieldsFromKeyvals(keyvals, nil); len(fields) > 0 {
		ctx = ctx.Fields(fields)
	}
	return adapter{logger: ctx.Logger(), groups: a.groups, forcedLevel: a.forcedLevel}
}

func (a adapter) WithLogLevel() port.ForLogging {
	return a.With("loglevel", port.LevelString(a.currentLevel()))
}

func (a adapter) LogLevelFromEnv(key string) port.ForLogging {
	if level, ok := port.LevelFromEnv(key); ok {
		return a.LogLevel(level)
	}
	return a
}

func (a adapter) LogLevel(level port.Level) port.ForLogging {
	if level == port.NoLevel {
		lvl := level
		return adapter{logger: a.logger, groups: a.groups, forcedLevel: &lvl}
	}
	return adapter{logger: a.logger.Level(portLevelToZero(level)), groups: a.groups}
}

func (a adapter) Debug(msg string, keyvals ...any) {
	event := a.newEvent(zerolog.DebugLevel)
	addFields(event, keyvals, a.groups)
	event.Msg(msg)
}

func (a adapter) Debugf(format string, args ...any) {
	a.Debug(formatMessage(format, args...))
}

func (a adapter) Info(msg string, keyvals ...any) {
	event := a.newEvent(zerolog.InfoLevel)
	addFields(event, keyvals, a.groups)
	event.Msg(msg)
}

func (a adapter) Infof(format string, args ...any) {
	a.Info(formatMessage(format, args...))
}

func (a adapter) Warn(msg string, keyvals ...any) {
	event := a.newEvent(zerolog.WarnLevel)
	addFields(event, keyvals, a.groups)
	event.Msg(msg)
}

func (a adapter) Warnf(format string, args ...any) {
	a.Warn(formatMessage(format, args...))
}

func (a adapter) Error(msg string, keyvals ...any) {
	event := a.newEvent(zerolog.ErrorLevel)
	addFields(event, keyvals, a.groups)
	event.Msg(msg)
}

func (a adapter) Errorf(format string, args ...any) {
	a.Error(formatMessage(format, args...))
}

func (a adapter) Fatal(msg string, keyvals ...any) {
	event := a.logger.Fatal()
	addFields(event, keyvals, a.groups)
	event.Msg(msg)
}

func (a adapter) Fatalf(format string, args ...any) {
	a.Fatal(formatMessage(format, args...))
}

func (a adapter) Panic(msg string, keyvals ...any) {
	event := a.logger.Panic()
	if event == nil {
		panic(msg)
	}
	addFields(event, keyvals, a.groups)
	event.Msg(msg)
}

func (a adapter) Panicf(format string, args ...any) {
	a.Panic(formatMessage(format, args...))
}

func (a adapter) Trace(msg string, keyvals ...any) {
	event := a.newEvent(zerolog.TraceLevel)
	if event == nil {
		return
	}
	addFields(event, keyvals, a.groups)
	event.Msg(msg)
}

func (a adapter) Tracef(format string, args ...any) {
	a.Trace(formatMessage(format, args...))
}

func appendUnique(parts []string, part string) []string {
	for _, existing := range parts {
		if existing == part {
			return parts
		}
	}
	return append(parts, part)
}

func fieldsFromKeyvals(keyvals []any, groups []string) map[string]any {
	if len(keyvals) == 0 {
		return nil
	}
	fields := make(map[string]any, len(keyvals)/2+len(keyvals)%2)
	scratch := make([]any, 0, 4)
	pairIndex := 0
	for i := 0; i < len(keyvals); {
		switch v := keyvals[i].(type) {
		case slog.Attr:
			scratch = scratch[:0]
			scratch = appendAttrKeyvals(scratch, v, groups)
			for j := 0; j < len(scratch); j += 2 {
				key := fmt.Sprint(scratch[j])
				fields[key] = scratch[j+1]
			}
			i++
		case []slog.Attr:
			for _, attr := range v {
				scratch = scratch[:0]
				scratch = appendAttrKeyvals(scratch, attr, groups)
				for j := 0; j < len(scratch); j += 2 {
					key := fmt.Sprint(scratch[j])
					fields[key] = scratch[j+1]
				}
			}
			i++
			continue
		default:
			if i+1 < len(keyvals) {
				key := fmt.Sprint(v)
				if len(groups) > 0 {
					key = joinAttrKey(groups, key)
				}
				fields[key] = keyvals[i+1]
				pairIndex++
				i += 2
			} else {
				key := fmt.Sprintf("arg%d", pairIndex)
				if len(groups) > 0 {
					key = joinAttrKey(groups, key)
				}
				fields[key] = v
				pairIndex++
				i++
			}
			continue
		}
	}
	return fields
}

func addFields(event *zerolog.Event, keyvals []any, groups []string) {
	if event == nil || len(keyvals) == 0 {
		return
	}
	fields := fieldsFromKeyvals(keyvals, groups)
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
	if a.forceNoLevel() {
		return true
	}
	return slogLevelToZero(level) >= a.logger.GetLevel()
}

func (a adapter) Handle(_ context.Context, record slog.Record) error {
	var event *zerolog.Event
	if a.forceNoLevel() {
		event = a.logger.Log()
	} else {
		event = a.logger.WithLevel(slogLevelToZero(record.Level))
	}
	keyvals := recordToKeyvals(record, a.groups)
	addFields(event, keyvals, nil)
	event.Msg(record.Message)
	return nil
}

func (a adapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return a
	}
	ctx := a.logger.With()
	if fields := fieldsFromKeyvals(attrsToKeyvals(attrs, a.groups), nil); len(fields) > 0 {
		ctx = ctx.Fields(fields)
	}
	return adapter{logger: ctx.Logger(), groups: a.groups, forcedLevel: a.forcedLevel}
}

func (a adapter) WithGroup(name string) slog.Handler {
	if name == "" {
		return a
	}
	return adapter{logger: a.logger, groups: appendGroup(a.groups, name), forcedLevel: a.forcedLevel}
}

func slogLevelToZero(level slog.Level) zerolog.Level {
	switch {
	case level < slog.LevelDebug:
		return zerolog.TraceLevel
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

func (a adapter) currentLevel() port.Level {
	if a.forcedLevel != nil {
		return *a.forcedLevel
	}
	return zerologLevelToPort(a.logger.GetLevel())
}

func portLevelToZero(level port.Level) zerolog.Level {
	switch level {
	case port.TraceLevel:
		return zerolog.TraceLevel
	case port.NoLevel:
		return zerolog.NoLevel
	case port.DebugLevel:
		return zerolog.DebugLevel
	case port.InfoLevel:
		return zerolog.InfoLevel
	case port.WarnLevel:
		return zerolog.WarnLevel
	case port.ErrorLevel:
		return zerolog.ErrorLevel
	case port.FatalLevel:
		return zerolog.FatalLevel
	case port.PanicLevel:
		return zerolog.PanicLevel
	case port.Disabled:
		return zerolog.Disabled
	default:
		return zerolog.InfoLevel
	}
}

func zerologLevelToPort(level zerolog.Level) port.Level {
	switch level {
	case zerolog.TraceLevel:
		return port.TraceLevel
	case zerolog.DebugLevel:
		return port.DebugLevel
	case zerolog.InfoLevel:
		return port.InfoLevel
	case zerolog.WarnLevel:
		return port.WarnLevel
	case zerolog.ErrorLevel:
		return port.ErrorLevel
	case zerolog.FatalLevel:
		return port.FatalLevel
	case zerolog.PanicLevel:
		return port.PanicLevel
	case zerolog.NoLevel:
		return port.NoLevel
	case zerolog.Disabled:
		return port.Disabled
	default:
		return port.InfoLevel
	}
}

func (a adapter) newEvent(level zerolog.Level) *zerolog.Event {
	if a.forceNoLevel() {
		return a.logger.Log()
	}
	switch level {
	case zerolog.TraceLevel:
		return a.logger.Trace()
	case zerolog.DebugLevel:
		return a.logger.Debug()
	case zerolog.InfoLevel:
		return a.logger.Info()
	case zerolog.WarnLevel:
		return a.logger.Warn()
	case zerolog.ErrorLevel:
		return a.logger.Error()
	default:
		return a.logger.Log()
	}
}

func (a adapter) forceNoLevel() bool {
	return a.forcedLevel != nil && *a.forcedLevel == port.NoLevel
}

func formatMessage(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}
