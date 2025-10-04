package charmlogger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	port "github.com/sa6mwa/logport"
)

var (
	ErrLoggerRequired error = errors.New("logger is required")
)

// New constructs a logging adapter backed by charmbracelet/log.
func New(w io.Writer) port.ForLogging {
	return charmAdapter{logger: log.NewWithOptions(w, log.Options{
		TimeFormat:      time.RFC3339,
		ReportTimestamp: true,
	})}
}

func NewWithOptions(w io.Writer, o log.Options) port.ForLogging {
	return charmAdapter{logger: log.NewWithOptions(w, o)}
}

// Returns a new logger in a context, retrievable via
// port.LoggerFromContext.
func ContextWithLogger(ctx context.Context, w io.Writer, o log.Options) context.Context {
	return port.ContextWithLogger(ctx, charmAdapter{logger: log.NewWithOptions(w, o)})
}

type charmAdapter struct {
	logger      *log.Logger
	groups      []string
	forcedLevel *port.Level
}

func (c charmAdapter) LogLevel(level port.Level) port.ForLogging {
	if c.logger == nil {
		return c
	}
	if level == port.NoLevel {
		lvl := level
		return charmAdapter{logger: c.logger, groups: c.groups, forcedLevel: &lvl}
	}
	clone := c.logger.With()
	clone.SetLevel(portLevelToCharm(level))
	return charmAdapter{logger: clone, groups: c.groups}
}

func (c charmAdapter) With(keyvals ...any) port.ForLogging {
	if c.logger == nil || len(keyvals) == 0 {
		return charmAdapter{logger: c.logger, groups: c.groups, forcedLevel: c.forcedLevel}
	}
	normalized := normalizeCharmKeyvals(keyvals, nil)
	if len(normalized) == 0 {
		return charmAdapter{logger: c.logger, groups: c.groups, forcedLevel: c.forcedLevel}
	}
	return charmAdapter{logger: c.logger.With(normalized...), groups: c.groups, forcedLevel: c.forcedLevel}
}

func (c charmAdapter) Debug(msg string, keyvals ...any) {
	if c.forceNoLevel() {
		keyvals = normalizeCharmKeyvals(keyvals, c.groups)
		c.logger.Print(msg, keyvals...)
		return
	}
	keyvals = normalizeCharmKeyvals(keyvals, c.groups)
	c.logger.Debug(msg, keyvals...)
}

func (c charmAdapter) Info(msg string, keyvals ...any) {
	if c.forceNoLevel() {
		keyvals = normalizeCharmKeyvals(keyvals, c.groups)
		c.logger.Print(msg, keyvals...)
		return
	}
	keyvals = normalizeCharmKeyvals(keyvals, c.groups)
	c.logger.Info(msg, keyvals...)
}

func (c charmAdapter) Warn(msg string, keyvals ...any) {
	if c.forceNoLevel() {
		keyvals = normalizeCharmKeyvals(keyvals, c.groups)
		c.logger.Print(msg, keyvals...)
		return
	}
	keyvals = normalizeCharmKeyvals(keyvals, c.groups)
	c.logger.Warn(msg, keyvals...)
}

func (c charmAdapter) Error(msg string, keyvals ...any) {
	if c.forceNoLevel() {
		keyvals = normalizeCharmKeyvals(keyvals, c.groups)
		c.logger.Print(msg, keyvals...)
		return
	}
	keyvals = normalizeCharmKeyvals(keyvals, c.groups)
	c.logger.Error(msg, keyvals...)
}

func (c charmAdapter) Fatal(msg string, keyvals ...any) {
	keyvals = normalizeCharmKeyvals(keyvals, c.groups)
	c.logger.Fatal(msg, keyvals...)
}

func (c charmAdapter) Panic(msg string, keyvals ...any) {
	if c.logger != nil {
		keyvals = normalizeCharmKeyvals(keyvals, c.groups)
		c.logger.Error(msg, keyvals...)
	}
	panic(msg)
}

func (c charmAdapter) Trace(msg string, keyvals ...any) {
	if c.logger == nil {
		return
	}
	if c.forceNoLevel() {
		keyvals = normalizeCharmKeyvals(keyvals, c.groups)
		c.logger.Print(msg, keyvals...)
		return
	}
	keyvals = normalizeCharmKeyvals(keyvals, c.groups)
	c.logger.Debug(msg, keyvals...)
}

func (c charmAdapter) Enabled(_ context.Context, level slog.Level) bool {
	if c.logger == nil {
		return false
	}
	if c.forceNoLevel() {
		return true
	}
	return slogLevelToCharm(level) >= c.logger.GetLevel()
}

func (c charmAdapter) Handle(_ context.Context, record slog.Record) error {
	if c.logger == nil {
		return nil
	}
	keyvals := recordToKeyvals(record, c.groups)
	if c.forceNoLevel() {
		c.logger.Print(record.Message, keyvals...)
		return nil
	}
	switch {
	case record.Level <= slog.LevelDebug:
		c.logger.Debug(record.Message, keyvals...)
	case record.Level <= slog.LevelInfo:
		c.logger.Info(record.Message, keyvals...)
	case record.Level <= slog.LevelWarn:
		c.logger.Warn(record.Message, keyvals...)
	case record.Level <= slog.LevelError:
		c.logger.Error(record.Message, keyvals...)
	default:
		c.logger.Error(record.Message, append(keyvals, "slog_level", record.Level.String())...)
	}
	return nil
}

func (c charmAdapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 || c.logger == nil {
		return c
	}
	keyvals := attrsToKeyvals(attrs, c.groups)
	return charmAdapter{logger: c.logger.With(keyvals...), groups: c.groups, forcedLevel: c.forcedLevel}
}

func (c charmAdapter) WithGroup(name string) slog.Handler {
	if name == "" {
		return c
	}
	groups := appendGroup(c.groups, name)
	return charmAdapter{logger: c.logger, groups: groups, forcedLevel: c.forcedLevel}
}

func slogLevelToCharm(level slog.Level) log.Level {
	switch {
	case level < slog.LevelDebug:
		return log.DebugLevel
	case level <= slog.LevelDebug:
		return log.DebugLevel
	case level <= slog.LevelInfo:
		return log.InfoLevel
	case level <= slog.LevelWarn:
		return log.WarnLevel
	case level <= slog.LevelError:
		return log.ErrorLevel
	default:
		return log.FatalLevel
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

func portLevelToCharm(level port.Level) log.Level {
	switch level {
	case port.TraceLevel, port.DebugLevel, port.NoLevel:
		return log.DebugLevel
	case port.InfoLevel:
		return log.InfoLevel
	case port.WarnLevel:
		return log.WarnLevel
	case port.ErrorLevel:
		return log.ErrorLevel
	case port.FatalLevel, port.PanicLevel:
		return log.FatalLevel
	case port.Disabled:
		return log.FatalLevel + 1
	default:
		return log.InfoLevel
	}
}

func (c charmAdapter) forceNoLevel() bool {
	return c.logger != nil && c.forcedLevel != nil && *c.forcedLevel == port.NoLevel
}

var _ port.ForLogging = charmAdapter{}

func normalizeCharmKeyvals(keyvals []any, groups []string) []any {
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
