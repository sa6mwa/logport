package phuslu

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"

	plog "github.com/phuslu/log"
	port "github.com/sa6mwa/logport"
)

type Options struct {
	Configure func(*plog.Logger)
}

func New(w io.Writer) port.ForLogging {
	return NewWithOptions(w, Options{})
}

func NewWithOptions(w io.Writer, opts Options) port.ForLogging {
	logger := &plog.Logger{Level: plog.InfoLevel}
	if w != nil {
		logger.Writer = plog.IOWriter{Writer: w}
	}
	if opts.Configure != nil {
		opts.Configure(logger)
	}
	return adapter{logger: logger}
}

func NewFromLogger(logger *plog.Logger) port.ForLogging {
	return adapter{logger: logger}
}

func ContextWithLogger(ctx context.Context, w io.Writer, opts Options) context.Context {
	return port.ContextWithLogger(ctx, NewWithOptions(w, opts))
}

type adapter struct {
	logger      *plog.Logger
	baseKeyvals []any
	groups      []string
}

func (a adapter) With(keyvals ...any) port.ForLogging {
	if len(keyvals) == 0 {
		return a
	}
	if a.logger == nil {
		return a
	}
	addition := normalizeKeyvals(keyvals)
	if len(addition) == 0 {
		return a
	}
	base := make([]any, 0, len(a.baseKeyvals)+len(addition))
	base = append(base, a.baseKeyvals...)
	base = append(base, addition...)
	return adapter{logger: a.logger, baseKeyvals: base, groups: a.groups}
}

func (a adapter) Debug(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	a.logEntry(a.logger.Debug(), msg, keyvals)
}

func (a adapter) Info(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	a.logEntry(a.logger.Info(), msg, keyvals)
}

func (a adapter) Warn(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	a.logEntry(a.logger.Warn(), msg, keyvals)
}

func (a adapter) Error(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	a.logEntry(a.logger.Error(), msg, keyvals)
}

func (a adapter) Fatal(msg string, keyvals ...any) {
	if a.logger == nil {
		return
	}
	a.logEntry(a.logger.Fatal(), msg, keyvals)
}

func (a adapter) logEntry(entry *plog.Entry, msg string, keyvals []any) {
	if entry == nil {
		return
	}
	if len(a.baseKeyvals) > 0 {
		entry.KeysAndValues(a.baseKeyvals...)
	}
	if len(keyvals) > 0 {
		addition := normalizeKeyvals(keyvals)
		if len(addition) > 0 {
			entry.KeysAndValues(addition...)
		}
	}
	entry.Msg(msg)
}

func (a adapter) Enabled(_ context.Context, level slog.Level) bool {
	if a.logger == nil {
		return false
	}
	current := plog.Level(atomic.LoadUint32((*uint32)(&a.logger.Level)))
	target := slogLevelToPhuslu(level)
	return target >= current
}

func (a adapter) Handle(_ context.Context, record slog.Record) error {
	if a.logger == nil {
		return nil
	}
	entry := a.logger.WithLevel(slogLevelToPhuslu(record.Level))
	if entry == nil {
		return nil
	}
	if len(a.baseKeyvals) > 0 {
		entry.KeysAndValues(a.baseKeyvals...)
	}
	if kvs := recordToKeyvals(record, a.groups); len(kvs) > 0 {
		entry.KeysAndValues(kvs...)
	}
	entry.Msg(record.Message)
	return nil
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
	return adapter{logger: a.logger, baseKeyvals: base, groups: a.groups}
}

func (a adapter) WithGroup(name string) slog.Handler {
	if name == "" {
		return a
	}
	groups := appendGroup(a.groups, name)
	return adapter{logger: a.logger, baseKeyvals: a.baseKeyvals, groups: groups}
}

func slogLevelToPhuslu(level slog.Level) plog.Level {
	switch {
	case level < slog.LevelDebug:
		return plog.TraceLevel
	case level < slog.LevelInfo:
		return plog.DebugLevel
	case level < slog.LevelWarn:
		return plog.InfoLevel
	case level < slog.LevelError:
		return plog.WarnLevel
	case level < slog.LevelError+4:
		return plog.ErrorLevel
	default:
		return plog.FatalLevel
	}
}

func normalizeKeyvals(keyvals []any) []any {
	if len(keyvals) == 0 {
		return nil
	}
	pairs := len(keyvals) / 2
	normalized := make([]any, 0, len(keyvals)+len(keyvals)%2)
	for i := range pairs {
		key := fmt.Sprint(keyvals[2*i])
		normalized = append(normalized, key, keyvals[2*i+1])
	}
	if len(keyvals)%2 != 0 {
		normalized = append(normalized, fmt.Sprintf("arg%d", pairs), keyvals[len(keyvals)-1])
	}
	return normalized
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

var _ port.ForLogging = adapter{}
