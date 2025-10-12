package psl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	port "pkt.systems/logport"
)

// Mode controls how PSL renders log entries.
type Mode int

const (
	// ModeConsole emits zerolog-style console lines (colour aware, minimal allocations).
	ModeConsole Mode = iota
	// ModeStructured emits compact JSON (optionally colourful) suitable for ingestion.
	ModeStructured
)

// Options controls how the PSL adapter formats and filters output.
type Options struct {
	// Mode selects console (default) or structured JSON rendering.
	Mode Mode

	// TimeFormat overrides the timestamp layout. When empty, PSL uses
	// port.DTGTimeFormat for console output and time.RFC3339 for JSON.
	TimeFormat string

	// DisableTimestamp drops the timestamp entirely.
	DisableTimestamp bool

	// NoColor forces colour escape codes off regardless of terminal detection.
	NoColor bool

	// ColorJSON enables colourful JSON output when ModeStructured is set.
	ColorJSON bool

	// MinLevel sets the minimum level the adapter will emit. Defaults to Trace.
	MinLevel *port.Level

	// VerboseFields switches JSON keys from ts/lvl/msg to time/level/message.
	VerboseFields bool

	// UTC forces timestamps to be rendered in UTC.
	UTC bool
}

// New constructs a PSL adapter configured for console output. The console mode
// delivers colour-aware, zerolog-style lines when the destination is a TTY and
// keeps the hot path allocation free. Use NewStructured or NewWithOptions for
// JSON output or customised settings.
func New(w io.Writer) port.ForLogging {
	return NewWithOptions(w, Options{Mode: ModeConsole})
}

// NewStructured returns a PSL adapter in structured JSON mode. By default it
// emits compact JSON, short field names (ts/lvl/msg), and enables colourful
// output only when the destination appears to be a terminal. For strict JSON
// logs without colour, see NewStructuredNoColor or NewWithOptions.
func NewStructured(w io.Writer) port.ForLogging {
	return NewWithOptions(w, Options{Mode: ModeStructured, ColorJSON: true})
}

// NewStructuredNoColor returns a PSL adapter that always emits plain JSON
// (no colour escape codes), regardless of whether the output is a terminal.
func NewStructuredNoColor(w io.Writer) port.ForLogging {
	return NewWithOptions(w, Options{Mode: ModeStructured})
}

// NewWithOptions builds a PSL adapter with explicit settings. It is the entry
// point for toggling timestamp formats, colour handling, minimum levels, UTC
// enforcement, and JSON verbosity while reusing the PSL hot paths.
func NewWithOptions(w io.Writer, opts Options) port.ForLogging {
	if w == nil {
		w = io.Discard
	}
	mode := opts.Mode
	if mode != ModeStructured {
		mode = ModeConsole
	}
	minLevel := port.TraceLevel
	if opts.MinLevel != nil {
		minLevel = *opts.MinLevel
	}

	timeFormat := opts.TimeFormat
	if timeFormat == "" {
		if mode == ModeConsole {
			timeFormat = port.DTGTimeFormat
		} else {
			timeFormat = time.RFC3339
		}
	}

	noColor := opts.NoColor || !isTerminal(w) || os.Getenv("NO_COLOR") != ""
	colorJSON := opts.ColorJSON && !noColor
	useCache := !opts.DisableTimestamp && isCacheableLayout(timeFormat)
	discard := isDiscardWriter(w)
	var cache *timeCache
	if useCache {
		cache = &timeCache{layout: timeFormat, utc: opts.UTC}
	}

	return adapter{
		writer:           w,
		mode:             mode,
		timeFormat:       timeFormat,
		colorEnabled:     !noColor,
		colorJSONEnabled: colorJSON,
		disableTimestamp: opts.DisableTimestamp,
		minLevel:         minLevel,
		verbose:          opts.VerboseFields,
		timeCache:        cache,
		useTimeCache:     useCache,
		useUTC:           opts.UTC,
		discard:          discard,
	}
}

func isCacheableLayout(layout string) bool {
	_, ok := cacheableLayouts[layout]
	return ok
}

// ContextWithLogger stores a PSL-backed logger in a context using the supplied
// options, allowing downstream code to retrieve it with logport.LoggerFromContext.
func ContextWithLogger(ctx context.Context, w io.Writer, opts Options) context.Context {
	return port.ContextWithLogger(ctx, NewWithOptions(w, opts))
}

func isDiscardWriter(w io.Writer) bool {
	return w == io.Discard
}

type adapter struct {
	writer           io.Writer
	mode             Mode
	timeFormat       string
	colorEnabled     bool
	colorJSONEnabled bool
	disableTimestamp bool
	minLevel         port.Level
	forcedLevel      *port.Level
	baseFields       []kv
	groups           []string
	verbose          bool
	timeCache        *timeCache
	useTimeCache     bool
	useUTC           bool
	discard          bool
}

type kv struct {
	key   string
	value any
}

type scratch struct {
	buf []byte
}

var scratchPool = sync.Pool{New: func() any { return &scratch{buf: make([]byte, 0, 256)} }}

var cacheableLayouts = map[string]struct{}{
	port.DTGTimeFormat: {},
	time.ANSIC:         {},
	time.UnixDate:      {},
	time.RubyDate:      {},
	time.RFC822:        {},
	time.RFC822Z:       {},
	time.RFC850:        {},
	time.RFC1123:       {},
	time.RFC1123Z:      {},
	time.RFC3339:       {},
	time.RFC3339Nano:   {},
	time.Kitchen:       {},
	time.Stamp:         {},
	time.StampMilli:    {},
	time.StampMicro:    {},
	time.StampNano:     {},
	time.DateTime:      {},
	time.DateOnly:      {},
	time.TimeOnly:      {},
}

type timeCache struct {
	layout string
	utc    bool
	once   sync.Once
	value  atomic.Value
}

func (c *timeCache) Current() string {
	c.once.Do(func() {
		now := time.Now()
		if c.utc {
			now = now.UTC()
		}
		c.value.Store(now.Format(c.layout))
		go c.refresh()
	})
	if v := c.value.Load(); v != nil {
		return v.(string)
	}
	now := time.Now()
	if c.utc {
		now = now.UTC()
	}
	return now.Format(c.layout)
}

func (c *timeCache) refresh() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for now := range ticker.C {
		if c.utc {
			now = now.UTC()
		}
		c.value.Store(now.Format(c.layout))
	}
}

func (a adapter) LogLevel(level port.Level) port.ForLogging {
	if level == port.NoLevel {
		lvl := level
		return adapter{
			writer:           a.writer,
			mode:             a.mode,
			timeFormat:       a.timeFormat,
			colorEnabled:     a.colorEnabled,
			colorJSONEnabled: a.colorJSONEnabled,
			disableTimestamp: a.disableTimestamp,
			minLevel:         a.minLevel,
			forcedLevel:      &lvl,
			baseFields:       cloneFields(a.baseFields),
			groups:           cloneStrings(a.groups),
			verbose:          a.verbose,
			timeCache:        a.timeCache,
			useTimeCache:     a.useTimeCache,
			useUTC:           a.useUTC,
			discard:          a.discard,
		}
	}
	return adapter{
		writer:           a.writer,
		mode:             a.mode,
		timeFormat:       a.timeFormat,
		colorEnabled:     a.colorEnabled,
		colorJSONEnabled: a.colorJSONEnabled,
		disableTimestamp: a.disableTimestamp,
		minLevel:         level,
		baseFields:       cloneFields(a.baseFields),
		groups:           cloneStrings(a.groups),
		verbose:          a.verbose,
		timeCache:        a.timeCache,
		useTimeCache:     a.useTimeCache,
		useUTC:           a.useUTC,
		discard:          a.discard,
	}
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
	if len(keyvals) == 0 {
		return a
	}
	fields := collectKeyvals(nil, keyvals, nil)
	if len(fields) == 0 {
		return a
	}
	combined := append(cloneFields(a.baseFields), fields...)
	return adapter{
		writer:           a.writer,
		mode:             a.mode,
		timeFormat:       a.timeFormat,
		colorEnabled:     a.colorEnabled,
		colorJSONEnabled: a.colorJSONEnabled,
		disableTimestamp: a.disableTimestamp,
		minLevel:         a.minLevel,
		forcedLevel:      cloneLevel(a.forcedLevel),
		baseFields:       combined,
		groups:           cloneStrings(a.groups),
		verbose:          a.verbose,
		timeCache:        a.timeCache,
		useTimeCache:     a.useTimeCache,
		useUTC:           a.useUTC,
		discard:          a.discard,
	}
}

func (a adapter) WithGroup(name string) slog.Handler {
	if name == "" {
		return a
	}
	groups := append(cloneStrings(a.groups), name)
	return adapter{
		writer:           a.writer,
		mode:             a.mode,
		timeFormat:       a.timeFormat,
		colorEnabled:     a.colorEnabled,
		colorJSONEnabled: a.colorJSONEnabled,
		disableTimestamp: a.disableTimestamp,
		minLevel:         a.minLevel,
		forcedLevel:      cloneLevel(a.forcedLevel),
		baseFields:       cloneFields(a.baseFields),
		groups:           groups,
		verbose:          a.verbose,
		timeCache:        a.timeCache,
		useTimeCache:     a.useTimeCache,
		useUTC:           a.useUTC,
		discard:          a.discard,
	}
}

func (a adapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return a
	}
	additional := collectAttrs(nil, attrs, a.groups)
	combined := append(cloneFields(a.baseFields), additional...)
	return adapter{
		writer:           a.writer,
		mode:             a.mode,
		timeFormat:       a.timeFormat,
		colorEnabled:     a.colorEnabled,
		colorJSONEnabled: a.colorJSONEnabled,
		disableTimestamp: a.disableTimestamp,
		minLevel:         a.minLevel,
		forcedLevel:      cloneLevel(a.forcedLevel),
		baseFields:       combined,
		groups:           cloneStrings(a.groups),
		verbose:          a.verbose,
		timeCache:        a.timeCache,
		useTimeCache:     a.useTimeCache,
		useUTC:           a.useUTC,
		discard:          a.discard,
	}
}

func (a adapter) Enabled(_ context.Context, level slog.Level) bool {
	return a.shouldLog(port.LevelFromSlog(level))
}

func (a adapter) Handle(ctx context.Context, record slog.Record) error {
	if !a.shouldLog(port.LevelFromSlog(record.Level)) {
		return nil
	}
	keyvals := make([]any, 0, record.NumAttrs())
	record.Attrs(func(attr slog.Attr) bool {
		keyvals = append(keyvals, attr)
		return true
	})
	a.logInternal(port.LevelFromSlog(record.Level), record.Message, ctx, keyvals...)
	return nil
}

func (a adapter) Log(ctx context.Context, level slog.Level, msg string, keyvals ...any) {
	a.logInternal(port.LevelFromSlog(level), msg, ctx, keyvals...)
}

func (a adapter) Logp(level port.Level, msg string, keyvals ...any) {
	a.logInternal(level, msg, context.Background(), keyvals...)
}

func (a adapter) Logs(level string, msg string, keyvals ...any) {
	if lvl, ok := port.ParseLevel(level); ok {
		a.logInternal(lvl, msg, context.Background(), keyvals...)
		return
	}
	a.logInternal(port.NoLevel, msg, context.Background(), keyvals...)
}

func (a adapter) Logf(level port.Level, format string, args ...any) {
	a.logInternal(level, fmt.Sprintf(format, args...), context.Background())
}

func (a adapter) Debug(msg string, keyvals ...any) {
	a.logInternal(port.DebugLevel, msg, context.Background(), keyvals...)
}
func (a adapter) Info(msg string, keyvals ...any) {
	a.logInternal(port.InfoLevel, msg, context.Background(), keyvals...)
}
func (a adapter) Warn(msg string, keyvals ...any) {
	a.logInternal(port.WarnLevel, msg, context.Background(), keyvals...)
}
func (a adapter) Error(msg string, keyvals ...any) {
	a.logInternal(port.ErrorLevel, msg, context.Background(), keyvals...)
}

func (a adapter) Fatal(msg string, keyvals ...any) {
	a.logInternal(port.FatalLevel, msg, context.Background(), keyvals...)
	os.Exit(1)
}

func (a adapter) Panic(msg string, keyvals ...any) {
	a.logInternal(port.PanicLevel, msg, context.Background(), keyvals...)
	panic(msg)
}

func (a adapter) Trace(msg string, keyvals ...any) {
	a.logInternal(port.TraceLevel, msg, context.Background(), keyvals...)
}

func (a adapter) Debugf(format string, args ...any) {
	a.logInternal(port.DebugLevel, fmt.Sprintf(format, args...), context.Background())
}
func (a adapter) Infof(format string, args ...any) {
	a.logInternal(port.InfoLevel, fmt.Sprintf(format, args...), context.Background())
}
func (a adapter) Warnf(format string, args ...any) {
	a.logInternal(port.WarnLevel, fmt.Sprintf(format, args...), context.Background())
}
func (a adapter) Errorf(format string, args ...any) {
	a.logInternal(port.ErrorLevel, fmt.Sprintf(format, args...), context.Background())
}
func (a adapter) Fatalf(format string, args ...any) { a.Fatal(fmt.Sprintf(format, args...)) }
func (a adapter) Panicf(format string, args ...any) { a.Panic(fmt.Sprintf(format, args...)) }
func (a adapter) Tracef(format string, args ...any) {
	a.logInternal(port.TraceLevel, fmt.Sprintf(format, args...), context.Background())
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
	if a.writer == nil {
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

func (a adapter) logInternal(level port.Level, msg string, _ context.Context, keyvals ...any) {
	if level == port.Disabled || !a.shouldLog(level) || a.discard {
		return
	}
	var fields []kv
	if len(a.baseFields) > 0 {
		fields = append(fields, a.baseFields...)
	}
	if len(keyvals) > 0 {
		fields = collectKeyvals(fields, keyvals, a.groups)
	}

	includeTime := !a.disableTimestamp
	var timestamp string
	if includeTime {
		if a.useTimeCache && a.timeCache != nil {
			timestamp = a.timeCache.Current()
		} else {
			now := time.Now()
			if a.useUTC {
				now = now.UTC()
			}
			timestamp = now.Format(a.timeFormat)
		}
	}

	sc := scratchPool.Get().(*scratch)
	buf := sc.buf[:0]
	if a.mode == ModeConsole {
		buf = writeConsole(buf, level, msg, fields, timestamp, includeTime, a.colorEnabled)
	} else {
		names := shortFieldNames
		if a.verbose {
			names = verboseFieldNames
		}
		buf = writeStructured(buf, level, msg, fields, timestamp, includeTime, names, a.colorJSONEnabled, a.colorEnabled)
	}
	buf = append(buf, '\n')
	_, _ = a.writer.Write(buf)
	if cap(buf) > 8192 {
		sc.buf = make([]byte, 0, 256)
	} else {
		sc.buf = buf[:0]
	}
	scratchPool.Put(sc)
}

func cloneFields(src []kv) []kv {
	if len(src) == 0 {
		return nil
	}
	dst := make([]kv, len(src))
	copy(dst, src)
	return dst
}

func cloneStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func cloneLevel(lvl *port.Level) *port.Level {
	if lvl == nil {
		return nil
	}
	value := *lvl
	return &value
}

func collectKeyvals(dst []kv, keyvals []any, groups []string) []kv {
	pair := 0
	for i := 0; i < len(keyvals); {
		switch v := keyvals[i].(type) {
		case slog.Attr:
			dst = collectAttr(dst, v, groups)
			i++
		case []slog.Attr:
			for _, attr := range v {
				dst = collectAttr(dst, attr, groups)
			}
			i++
		default:
			var key string
			if i+1 < len(keyvals) {
				key = fmt.Sprint(v)
				if len(groups) > 0 {
					key = joinAttrKey(groups, key)
				}
				dst = append(dst, kv{key: key, value: keyvals[i+1]})
				pair++
				i += 2
			} else {
				key = fmt.Sprintf("arg%d", pair)
				if len(groups) > 0 {
					key = joinAttrKey(groups, key)
				}
				dst = append(dst, kv{key: key, value: v})
				pair++
				i++
			}
		}
	}
	return dst
}

func collectAttrs(dst []kv, attrs []slog.Attr, groups []string) []kv {
	for _, attr := range attrs {
		dst = collectAttr(dst, attr, groups)
	}
	return dst
}

func collectAttr(dst []kv, attr slog.Attr, groups []string) []kv {
	attr.Value = attr.Value.Resolve()
	switch attr.Value.Kind() {
	case slog.KindGroup:
		parent := groups
		if attr.Key != "" {
			parent = appendGroup(parent, attr.Key)
		}
		for _, nested := range attr.Value.Group() {
			dst = collectAttr(dst, nested, parent)
		}
	default:
		key := joinAttrKey(groups, attr.Key)
		dst = append(dst, kv{key: key, value: attr.Value.Any()})
	}
	return dst
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

const (
	ansiReset        = "\x1b[0m"
	ansiBold         = "\x1b[1m"
	ansiFaint        = "\x1b[90m"
	ansiRed          = "\x1b[31m"
	ansiGreen        = "\x1b[32m"
	ansiYellow       = "\x1b[33m"
	ansiBlue         = "\x1b[34m"
	ansiMagenta      = "\x1b[35m"
	ansiCyan         = "\x1b[36m"
	ansiBrightRed    = "\x1b[1;31m"
	ansiBrightGreen  = "\x1b[1;32m"
	ansiBrightYellow = "\x1b[1;33m"
)

type fieldNames struct {
	time          string
	level         string
	message       string
	timePrefix    []byte
	levelPrefix   []byte
	messagePrefix []byte
}

var (
	timeKeyPrefix    = []byte(`"time":"`)
	tsKeyPrefix      = []byte(`"ts":"`)
	levelKeyPrefix   = []byte(`"level":"`)
	lvlKeyPrefix     = []byte(`"lvl":"`)
	messageKeyPrefix = []byte(`"message":"`)
	msgKeyPrefix     = []byte(`"msg":"`)
)

const hexDigits = "0123456789abcdef"

var (
	shortFieldNames = fieldNames{
		time:          "ts",
		level:         "lvl",
		message:       "msg",
		timePrefix:    tsKeyPrefix,
		levelPrefix:   lvlKeyPrefix,
		messagePrefix: msgKeyPrefix,
	}
	verboseFieldNames = fieldNames{
		time:          "time",
		level:         "level",
		message:       "message",
		timePrefix:    timeKeyPrefix,
		levelPrefix:   levelKeyPrefix,
		messagePrefix: messageKeyPrefix,
	}
)

func writeConsole(buf []byte, level port.Level, msg string, fields []kv, timestamp string, includeTime bool, color bool) []byte {
	if includeTime {
		if color {
			buf = append(buf, ansiFaint...)
			buf = append(buf, timestamp...)
			buf = append(buf, ansiReset...)
		} else {
			buf = append(buf, timestamp...)
		}
		buf = append(buf, ' ')
	}
	levelStr, levelColor := consoleLevel(level)
	if color && levelColor != "" {
		buf = append(buf, levelColor...)
		buf = append(buf, levelStr...)
		buf = append(buf, ansiReset...)
	} else {
		buf = append(buf, levelStr...)
	}
	if msg != "" {
		buf = append(buf, ' ')
		if color && shouldHighlight(level) {
			buf = append(buf, ansiBold...)
			buf = append(buf, msg...)
			buf = append(buf, ansiReset...)
		} else {
			buf = append(buf, msg...)
		}
	}
	for _, f := range fields {
		buf = append(buf, ' ')
		if color {
			buf = append(buf, ansiCyan...)
			buf = append(buf, f.key...)
			buf = append(buf, '=')
			buf = append(buf, ansiReset...)
		} else {
			buf = append(buf, f.key...)
			buf = append(buf, '=')
		}
		buf = append(buf, formatConsoleValue(f.value)...)
	}
	return buf
}

func shouldHighlight(level port.Level) bool {
	switch level {
	case port.InfoLevel, port.WarnLevel, port.ErrorLevel, port.FatalLevel, port.PanicLevel:
		return true
	default:
		return false
	}
}

func consoleLevel(level port.Level) (string, string) {
	switch level {
	case port.TraceLevel:
		return "TRC", ansiBlue
	case port.DebugLevel:
		return "DBG", ""
	case port.InfoLevel:
		return "INF", ansiGreen
	case port.WarnLevel:
		return "WRN", ansiYellow
	case port.ErrorLevel:
		return "ERR", ansiRed
	case port.FatalLevel:
		return "FTL", ansiRed
	case port.PanicLevel:
		return "PNC", ansiRed
	case port.NoLevel:
		return "---", ""
	default:
		return "INF", ansiGreen
	}
}

func structuredLevel(level port.Level) string {
	switch level {
	case port.TraceLevel:
		return "trace"
	case port.DebugLevel:
		return "debug"
	case port.InfoLevel:
		return "info"
	case port.WarnLevel:
		return "warn"
	case port.ErrorLevel:
		return "error"
	case port.FatalLevel:
		return "fatal"
	case port.PanicLevel:
		return "panic"
	case port.NoLevel:
		return "nolevel"
	case port.Disabled:
		return "disabled"
	default:
		return "info"
	}
}

func formatConsoleValue(v any) string {
	switch val := v.(type) {
	case string:
		if needsQuote(val) {
			return strconv.Quote(val)
		}
		return val
	case fmt.Stringer:
		str := val.String()
		if needsQuote(str) {
			return strconv.Quote(str)
		}
		return str
	case error:
		str := val.Error()
		if needsQuote(str) {
			return strconv.Quote(str)
		}
		return str
	case time.Time:
		return val.Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(v)
	}
}

func needsQuote(s string) bool {
	for i := range len(s) {
		c := s[i]
		if c < 0x20 || c > 0x7e || c == ' ' || c == '\\' || c == '"' {
			return true
		}
	}
	return false
}

func writeStructured(buf []byte, level port.Level, msg string, fields []kv, timestamp string, includeTime bool, names fieldNames, colorJSON bool, colorEnabled bool) []byte {
	colored := colorJSON && colorEnabled
	if !colored && len(fields) == 0 {
		buf = append(buf, '{')
		first := true
		if includeTime {
			buf = appendFastStringField(buf, names.time, names.timePrefix, timestamp, &first)
		}
		buf = appendFastStringField(buf, names.level, names.levelPrefix, structuredLevel(level), &first)
		if msg != "" {
			if needsQuote(msg) {
				buf = appendQuotedField(buf, names.message, msg, &first)
			} else {
				buf = appendFastStringField(buf, names.message, names.messagePrefix, msg, &first)
			}
		}
		buf = append(buf, '}')
		return buf
	}
	buf = append(buf, '{')
	first := true
	writePair := func(key string, value any, kind jsonValueKind) {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = writeJSONKey(buf, key, colored)
		buf = append(buf, ':')
		switch kind {
		case jsonValueLevel:
			buf = writeJSONLevelValue(buf, level, colored)
		case jsonValueMessage:
			switch v := value.(type) {
			case string:
				buf = writeJSONMessageValue(buf, v, colored)
			default:
				buf = writeJSONMessageValue(buf, fmt.Sprint(v), colored)
			}
		default:
			buf = writeJSONValue(buf, value, colored)
		}
	}
	if includeTime {
		writePair(names.time, timestamp, jsonValueDefault)
	}
	writePair(names.level, structuredLevel(level), jsonValueLevel)
	if msg != "" {
		writePair(names.message, msg, jsonValueMessage)
	}
	for _, f := range fields {
		writePair(f.key, f.value, jsonValueDefault)
	}
	buf = append(buf, '}')
	return buf
}

type jsonValueKind int

const (
	jsonValueDefault jsonValueKind = iota
	jsonValueLevel
	jsonValueMessage
)

func appendSafeStringField(buf []byte, key, value string, first *bool) []byte {
	if !*first {
		buf = append(buf, ',')
	}
	*first = false
	switch key {
	case "time":
		buf = append(buf, timeKeyPrefix...)
	case "ts":
		buf = append(buf, tsKeyPrefix...)
	case "level":
		buf = append(buf, levelKeyPrefix...)
	case "lvl":
		buf = append(buf, lvlKeyPrefix...)
	case "msg":
		buf = append(buf, msgKeyPrefix...)
	case "message":
		buf = append(buf, messageKeyPrefix...)
	default:
		buf = appendJSONString(buf, key)
		buf = append(buf, ':')
		buf = appendJSONString(buf, value)
		return buf
	}
	buf = append(buf, value...)
	buf = append(buf, '"')
	return buf
}

func appendFastStringField(buf []byte, key string, prefix []byte, value string, first *bool) []byte {
	if prefix == nil {
		return appendSafeStringField(buf, key, value, first)
	}
	if !*first {
		buf = append(buf, ',')
	}
	*first = false
	buf = append(buf, prefix...)
	buf = append(buf, value...)
	buf = append(buf, '"')
	return buf
}

func appendQuotedField(buf []byte, key, value string, first *bool) []byte {
	if !*first {
		buf = append(buf, ',')
	}
	*first = false
	buf = appendJSONString(buf, key)
	buf = append(buf, ':')
	buf = appendJSONString(buf, value)
	return buf
}

func writeJSONKey(buf []byte, key string, color bool) []byte {
	if color {
		buf = append(buf, ansiCyan...)
		buf = appendJSONString(buf, key)
		buf = append(buf, ansiReset...)
		return buf
	}
	return appendJSONString(buf, key)
}

func writeJSONValue(buf []byte, value any, color bool) []byte {
	if !color {
		return writeJSONValuePlain(buf, value)
	}
	switch v := value.(type) {
	case string:
		return writeJSONString(buf, v, true, true)
	case fmt.Stringer:
		return writeJSONString(buf, v.String(), true, true)
	case error:
		return writeJSONString(buf, v.Error(), true, true)
	case bool:
		buf = append(buf, ansiYellow...)
		buf = strconv.AppendBool(buf, v)
		buf = append(buf, ansiReset...)
		return buf
	case time.Time:
		return writeJSONString(buf, v.Format(time.RFC3339Nano), true, true)
	case json.Marshaler:
		bytes, err := v.MarshalJSON()
		if err != nil {
			return writeJSONString(buf, err.Error(), true, true)
		}
		return writeJSONRaw(buf, bytes, true)
	case nil:
		buf = append(buf, ansiFaint...)
		buf = append(buf, "null"...)
		buf = append(buf, ansiReset...)
		return buf
	default:
		switch vv := v.(type) {
		case int:
			return writeJSONNumber(buf, int64(vv), true)
		case int8:
			return writeJSONNumber(buf, int64(vv), true)
		case int16:
			return writeJSONNumber(buf, int64(vv), true)
		case int32:
			return writeJSONNumber(buf, int64(vv), true)
		case int64:
			return writeJSONNumber(buf, vv, true)
		case uint:
			return writeJSONUint(buf, uint64(vv), true)
		case uint8:
			return writeJSONUint(buf, uint64(vv), true)
		case uint16:
			return writeJSONUint(buf, uint64(vv), true)
		case uint32:
			return writeJSONUint(buf, uint64(vv), true)
		case uint64:
			return writeJSONUint(buf, vv, true)
		case float32:
			return writeJSONFloat(buf, float64(vv), true)
		case float64:
			return writeJSONFloat(buf, vv, true)
		case json.Number:
			return writeJSONRaw(buf, []byte(vv.String()), true)
		case []byte:
			return writeJSONString(buf, string(vv), true, true)
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return writeJSONString(buf, err.Error(), true, true)
			}
			return writeJSONRaw(buf, b, true)
		}
	}
}

func writeJSONValuePlain(buf []byte, value any) []byte {
	switch v := value.(type) {
	case string:
		return writeJSONStringPlain(buf, v)
	case fmt.Stringer:
		return writeJSONStringPlain(buf, v.String())
	case error:
		return writeJSONStringPlain(buf, v.Error())
	case bool:
		return strconv.AppendBool(buf, v)
	case time.Time:
		return writeJSONStringPlain(buf, v.Format(time.RFC3339Nano))
	case json.Marshaler:
		bytes, err := v.MarshalJSON()
		if err != nil {
			return writeJSONStringPlain(buf, err.Error())
		}
		return writeJSONRawPlain(buf, bytes)
	case nil:
		return append(buf, "null"...)
	default:
		switch vv := v.(type) {
		case int:
			return writeJSONNumberPlain(buf, int64(vv))
		case int8:
			return writeJSONNumberPlain(buf, int64(vv))
		case int16:
			return writeJSONNumberPlain(buf, int64(vv))
		case int32:
			return writeJSONNumberPlain(buf, int64(vv))
		case int64:
			return writeJSONNumberPlain(buf, vv)
		case uint:
			return writeJSONUintPlain(buf, uint64(vv))
		case uint8:
			return writeJSONUintPlain(buf, uint64(vv))
		case uint16:
			return writeJSONUintPlain(buf, uint64(vv))
		case uint32:
			return writeJSONUintPlain(buf, uint64(vv))
		case uint64:
			return writeJSONUintPlain(buf, vv)
		case float32:
			return writeJSONFloatPlain(buf, float64(vv))
		case float64:
			return writeJSONFloatPlain(buf, vv)
		case json.Number:
			return writeJSONRawPlain(buf, []byte(vv.String()))
		case []byte:
			return writeJSONStringPlain(buf, string(vv))
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return writeJSONStringPlain(buf, err.Error())
			}
			return writeJSONRawPlain(buf, b)
		}
	}
}

func writeJSONStringPlain(buf []byte, s string) []byte {
	return appendJSONString(buf, s)
}

func writeJSONNumberPlain(buf []byte, n int64) []byte {
	return strconv.AppendInt(buf, n, 10)
}

func writeJSONUintPlain(buf []byte, n uint64) []byte {
	return strconv.AppendUint(buf, n, 10)
}

func writeJSONFloatPlain(buf []byte, f float64) []byte {
	return strconv.AppendFloat(buf, f, 'f', -1, 64)
}

func writeJSONRawPlain(buf []byte, raw []byte) []byte {
	return append(buf, raw...)
}

func writeJSONLevelValue(buf []byte, level port.Level, color bool) []byte {
	val := structuredLevel(level)
	if color {
		switch level {
		case port.InfoLevel:
			buf = append(buf, ansiBrightGreen...)
		case port.WarnLevel:
			buf = append(buf, ansiBrightYellow...)
		case port.ErrorLevel, port.FatalLevel, port.PanicLevel:
			buf = append(buf, ansiBrightRed...)
		case port.TraceLevel:
			buf = append(buf, ansiBlue...)
		default:
			buf = append(buf, ansiFaint...)
		}
		buf = strconv.AppendQuote(buf, val)
		buf = append(buf, ansiReset...)
		return buf
	}
	return strconv.AppendQuote(buf, val)
}

func writeJSONMessageValue(buf []byte, value string, color bool) []byte {
	if color {
		buf = append(buf, ansiBold...)
		buf = appendJSONString(buf, value)
		buf = append(buf, ansiReset...)
		return buf
	}
	return appendJSONString(buf, value)
}

func writeJSONString(buf []byte, s string, color bool, dim bool) []byte {
	if color {
		if !dim {
			buf = append(buf, ansiGreen...)
		}
		buf = appendJSONString(buf, s)
		buf = append(buf, ansiReset...)
		return buf
	}
	return appendJSONString(buf, s)
}

func writeJSONNumber(buf []byte, n int64, color bool) []byte {
	if color {
		buf = append(buf, ansiMagenta...)
		buf = strconv.AppendInt(buf, n, 10)
		buf = append(buf, ansiReset...)
		return buf
	}
	return strconv.AppendInt(buf, n, 10)
}

func writeJSONUint(buf []byte, n uint64, color bool) []byte {
	if color {
		buf = append(buf, ansiMagenta...)
		buf = strconv.AppendUint(buf, n, 10)
		buf = append(buf, ansiReset...)
		return buf
	}
	return strconv.AppendUint(buf, n, 10)
}

func writeJSONFloat(buf []byte, f float64, color bool) []byte {
	if color {
		buf = append(buf, ansiMagenta...)
		buf = strconv.AppendFloat(buf, f, 'f', -1, 64)
		buf = append(buf, ansiReset...)
		return buf
	}
	return strconv.AppendFloat(buf, f, 'f', -1, 64)
}

func writeJSONRaw(buf []byte, raw []byte, color bool) []byte {
	if color {
		buf = append(buf, ansiMagenta...)
		buf = append(buf, raw...)
		buf = append(buf, ansiReset...)
		return buf
	}
	return append(buf, raw...)
}

func appendJSONString(buf []byte, s string) []byte {
	buf = append(buf, '"')
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r >= 0x20 && r != '\\' && r != '"' {
			i += size
			continue
		}
		if start < i {
			buf = append(buf, s[start:i]...)
		}
		switch r {
		case '"', '\\':
			buf = append(buf, '\\', byte(r))
		case '\b':
			buf = append(buf, '\\', 'b')
		case '\f':
			buf = append(buf, '\\', 'f')
		case '\n':
			buf = append(buf, '\\', 'n')
		case '\r':
			buf = append(buf, '\\', 'r')
		case '\t':
			buf = append(buf, '\\', 't')
		default:
			if r < 0x20 {
				buf = append(buf, '\\', 'u', '0', '0', hexDigits[r>>4], hexDigits[r&0xF])
			} else if r == utf8.RuneError && size == 1 {
				buf = append(buf, '\\', 'u', 'f', 'f', 'f', 'd')
			} else {
				buf = append(buf, s[i:i+size]...)
			}
		}
		i += size
		start = i
	}
	if start < len(s) {
		buf = append(buf, s[start:]...)
	}
	buf = append(buf, '"')
	return buf
}

// These are not really necessary...
//
// var _ port.ForLoggingMinimalSubset = adapter{}
// var _ port.ForLogging = adapter{}
// var _ slog.Handler = adapter{}
// var _ io.Writer = adapter{}
