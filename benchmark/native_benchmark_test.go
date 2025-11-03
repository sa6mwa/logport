package benchmark

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"testing"
	"time"

	charm "github.com/charmbracelet/log"
	onelog "github.com/francoispqt/onelog"
	kitlog "github.com/go-kit/log"
	"github.com/go-logr/logr/funcr"
	plog "github.com/phuslu/log"
	"github.com/rs/zerolog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	logport "pkt.systems/logport"
	psl "pkt.systems/logport/adapters/psl"
)

type nativeBenchConfig struct {
	withTimestamp bool
}

type nativeLogger struct {
	name                  string
	supportsTimestampLess bool
	run                   func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry)
}

func BenchmarkNativeLoggerProductionDataset(b *testing.B) {
	runNativeLoggerBenchmarks(b, nativeBenchConfig{withTimestamp: true})
}

func BenchmarkNativeLoggerProductionDatasetNoTimestamp(b *testing.B) {
	runNativeLoggerBenchmarks(b, nativeBenchConfig{withTimestamp: false})
}

func runNativeLoggerBenchmarks(b *testing.B, cfg nativeBenchConfig) {
	entries := loadProductionEntries(b)
	if len(entries) == 0 {
		b.Fatal("production dataset empty")
	}
	staticWith, staticMap, staticKeys := productionStaticArgs(entries)
	dynamicEntries := productionEntriesWithoutStatic(entries, staticKeys)
	loggers := nativeLoggers(staticWith, staticMap)

	sink := newBenchmarkSink()
	for _, bench := range loggers {
		bench := bench
		b.Run(bench.name, func(b *testing.B) {
			if !cfg.withTimestamp && !bench.supportsTimestampLess {
				b.Skip("timestamp toggle not supported")
				return
			}
			sink.resetCount()
			bench.run(b, sink, cfg, dynamicEntries)
			reportBytesPerOp(b, sink)
		})
	}
}

func nativeLoggers(staticWith []any, staticMap map[string]any) []nativeLogger {
	return []nativeLogger{
		{
			name:                  "psl/console",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				opts := psl.Options{Mode: psl.ModeConsole}
				if !cfg.withTimestamp {
					opts.DisableTimestamp = true
				}
				logger := withBenchmarkFields(psl.NewWithOptions(sink, opts), staticWith)
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					logger.Logp(entry.level, entry.message, entry.keyvals...)
				}
			},
		},
		{
			name:                  "psl/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				opts := psl.Options{Mode: psl.ModeStructured}
				if !cfg.withTimestamp {
					opts.DisableTimestamp = true
				}
				logger := withBenchmarkFields(psl.NewWithOptions(sink, opts), staticWith)
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					logger.Logp(entry.level, entry.message, entry.keyvals...)
				}
			},
		},
		{
			name:                  "zerolog/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				ctx := zerolog.New(sink).With()
				ctx = applyStaticToZerolog(ctx, staticMap)
				ctx = ctx.Str("component", "benchmark").Str("env", "test")
				if cfg.withTimestamp {
					ctx = ctx.Timestamp()
				}
				logger := ctx.Logger().Level(zerolog.TraceLevel)
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					if ev := zerologEventForLevel(logger, entry.level); ev != nil {
						entry.applyZerolog(ev).Msg(entry.message)
					}
				}
			},
		},
		{
			name:                  "zerolog/console",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				writer := zerolog.ConsoleWriter{
					Out:     sink,
					NoColor: true,
				}
				if cfg.withTimestamp {
					writer.TimeFormat = time.RFC3339
				} else {
					writer.TimeFormat = ""
				}
				ctx := zerolog.New(writer).With()
				ctx = applyStaticToZerolog(ctx, staticMap)
				ctx = ctx.Str("component", "benchmark").Str("env", "test")
				if cfg.withTimestamp {
					ctx = ctx.Timestamp()
				}
				logger := ctx.Logger().Level(zerolog.TraceLevel)
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					if ev := zerologEventForLevel(logger, entry.level); ev != nil {
						entry.applyZerolog(ev).Msg(entry.message)
					}
				}
			},
		},
		{
			name:                  "charm/console",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				opts := charm.Options{
					ReportTimestamp: cfg.withTimestamp,
					TimeFormat:      time.RFC3339,
				}
				logger := charm.NewWithOptions(sink, opts)
				if len(staticWith) > 0 {
					logger = logger.With(staticWith...)
				}
				logger = logger.With("component", "benchmark", "env", "test")
				logger.SetLevel(charm.DebugLevel)
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					logger.Log(charmLevelFromPort(entry.level), entry.message, entry.keyvals...)
				}
			},
		},
		{
			name:                  "charm/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				opts := charm.Options{
					Formatter:       charm.JSONFormatter,
					ReportTimestamp: cfg.withTimestamp,
					TimeFormat:      time.RFC3339,
				}
				logger := charm.NewWithOptions(sink, opts)
				if len(staticWith) > 0 {
					logger = logger.With(staticWith...)
				}
				logger = logger.With("component", "benchmark", "env", "test")
				logger.SetLevel(charm.DebugLevel)
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					logger.Log(charmLevelFromPort(entry.level), entry.message, entry.keyvals...)
				}
			},
		},
		{
			name:                  "slog/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				opts := &slog.HandlerOptions{Level: slog.LevelDebug}
				if !cfg.withTimestamp {
					opts = &slog.HandlerOptions{
						Level: slog.LevelDebug,
						ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
							if a.Key == slog.TimeKey {
								return slog.Attr{}
							}
							return a
						},
					}
				}
				handler := slog.NewJSONHandler(sink, opts)
				logger := slog.New(handler)
				if len(staticWith) > 0 {
					logger = logger.With(staticWith...)
				}
				logger = logger.With(slog.String("component", "benchmark"), slog.String("env", "test"))
				ctx := context.Background()
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					attrs := make([]slog.Attr, 0, len(entry.keyvals)/2)
					entry.forEachField(func(key string, value any) {
						attrs = append(attrs, slog.Any(key, value))
					})
					logger.LogAttrs(ctx, slogLevelFromPort(entry.level), entry.message, attrs...)
				}
			},
		},
		{
			name:                  "zap/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				encoderCfg := zap.NewProductionEncoderConfig()
				if cfg.withTimestamp {
					encoderCfg.TimeKey = "ts"
					encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
				} else {
					encoderCfg.TimeKey = ""
				}
				encoder := zapcore.NewJSONEncoder(encoderCfg)
				core := zapcore.NewCore(encoder, zapcore.AddSync(sink), zapcore.DebugLevel)
				logger := zap.New(core, zap.WithCaller(false)).With(zapStaticFields(staticMap)...).
					With(zap.String("component", "benchmark"), zap.String("env", "test"))
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					if ce := logger.Check(zapLevelFromPort(entry.level), entry.message); ce != nil {
						ce.Write(entry.zapFieldsSlice()...)
					}
				}
			},
		},
		{
			name:                  "phuslu/json",
			supportsTimestampLess: false,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				logger := &plog.Logger{Level: plog.TraceLevel, Writer: plog.IOWriter{Writer: sink}}
				if cfg.withTimestamp {
					logger.TimeFormat = time.RFC3339
					logger.TimeField = "time"
				} else {
					logger.TimeFormat = ""
					logger.TimeField = ""
				}
				logger.Context = buildPhusluContext(staticMap).
					Str("component", "benchmark").
					Str("env", "test").
					Value()
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					if ev := phusluEntryForLevel(logger, entry.level); ev != nil {
						entry.applyPhuslu(ev)
						ev.Msg(entry.message)
					}
				}
			},
		},
		{
			name:                  "onelog/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				logger := applyStaticToOnelog(onelog.New(sink, onelog.ALL), staticMap)
				if cfg.withTimestamp {
					logger.Hook(func(e onelog.Entry) {
						e.String("ts", time.Now().UTC().Format(time.RFC3339))
					})
				}
				logger = logger.With(func(e onelog.Entry) {
					e.String("component", "benchmark")
					e.String("env", "test")
				})
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					chain := logger.InfoWith(entry.message)
					entry.forEachField(func(key string, value any) {
						switch v := value.(type) {
						case string:
							chain = chain.String(key, v)
						case bool:
							chain = chain.Bool(key, v)
						case int:
							chain = chain.Int(key, v)
						case int64:
							chain = chain.Int64(key, v)
						case float64:
							chain = chain.Float(key, v)
						default:
							chain = chain.Any(key, v)
						}
					})
					chain.Write()
				}
			},
		},
		{
			name:                  "kitlog/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				logger := kitlog.NewJSONLogger(sink)
				if cfg.withTimestamp {
					logger = kitlog.With(logger, "ts", kitlog.DefaultTimestampUTC)
				}
				if len(staticWith) > 0 {
					logger = kitlog.With(logger, staticWith...)
				}
				logger = kitlog.With(logger, "component", "benchmark", "env", "test")
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					keyvals := make([]any, 0, len(entry.keyvals)+4)
					keyvals = append(keyvals, "level", levelString(entry.level), "msg", entry.message)
					keyvals = append(keyvals, entry.keyvals...)
					_ = logger.Log(keyvals...)
				}
			},
		},
		{
			name:                  "logr/funcr",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig, entries []productionEntry) {
				logger := funcr.NewJSON(func(obj string) {
					_, _ = sink.Write([]byte(obj))
				}, funcr.Options{LogTimestamp: cfg.withTimestamp})
				if len(staticWith) > 0 {
					logger = logger.WithValues(staticWith...)
				}
				logger = logger.WithValues("component", "benchmark", "env", "test")
				entryCount := len(entries)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					entry := entries[i%entryCount]
					logger.Info(entry.message, entry.keyvals...)
				}
			},
		},
	}
}

func applyStaticToZerolog(ctx zerolog.Context, static map[string]any) zerolog.Context {
	if len(static) == 0 {
		return ctx
	}
	for _, key := range sortedStaticKeys(static) {
		switch v := static[key].(type) {
		case string:
			ctx = ctx.Str(key, v)
		case bool:
			ctx = ctx.Bool(key, v)
		case int:
			ctx = ctx.Int(key, v)
		case int64:
			ctx = ctx.Int64(key, v)
		case uint64:
			ctx = ctx.Uint64(key, v)
		case float64:
			ctx = ctx.Float64(key, v)
		case []byte:
			ctx = ctx.Bytes(key, v)
		default:
			ctx = ctx.Interface(key, v)
		}
	}
	return ctx
}

func zapStaticFields(static map[string]any) []zap.Field {
	if len(static) == 0 {
		return nil
	}
	fields := make([]zap.Field, 0, len(static))
	for _, key := range sortedStaticKeys(static) {
		fields = append(fields, zapFieldFromValue(key, static[key]))
	}
	return fields
}

func buildPhusluContext(static map[string]any) *plog.Entry {
	ctx := plog.NewContext(nil)
	if len(static) == 0 {
		return ctx
	}
	for _, key := range sortedStaticKeys(static) {
		appendPhusluField(ctx, key, static[key])
	}
	return ctx
}

func appendPhusluField(entry *plog.Entry, key string, value any) {
	switch v := value.(type) {
	case string:
		entry.Str(key, v)
	case bool:
		entry.Bool(key, v)
	case int:
		entry.Int(key, v)
	case int64:
		entry.Int64(key, v)
	case uint64:
		entry.Uint64(key, v)
	case float64:
		entry.Float64(key, v)
	default:
		entry.Any(key, v)
	}
}

func applyStaticToOnelog(logger *onelog.Logger, static map[string]any) *onelog.Logger {
	if len(static) == 0 {
		return logger
	}
	keys := sortedStaticKeys(static)
	return logger.With(func(e onelog.Entry) {
		for _, key := range keys {
			switch v := static[key].(type) {
			case string:
				e.String(key, v)
			case bool:
				e.Bool(key, v)
			case int:
				e.Int(key, v)
			case int64:
				e.Int64(key, v)
			case uint64:
				e.Int64(key, int64(v))
			case float64:
				e.Float(key, v)
			default:
				e.String(key, fmt.Sprintf("%v", v))
			}
		}
	})
}

func sortedStaticKeys(static map[string]any) []string {
	keys := make([]string, 0, len(static))
	for key := range static {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func levelString(level logport.Level) string {
	switch level {
	case logport.TraceLevel:
		return "trace"
	case logport.DebugLevel:
		return "debug"
	case logport.InfoLevel:
		return "info"
	case logport.WarnLevel:
		return "warn"
	case logport.ErrorLevel:
		return "error"
	case logport.FatalLevel:
		return "fatal"
	case logport.PanicLevel:
		return "panic"
	default:
		return "info"
	}
}

func charmLevelFromPort(level logport.Level) charm.Level {
	switch level {
	case logport.TraceLevel:
		return charm.DebugLevel
	case logport.DebugLevel:
		return charm.DebugLevel
	case logport.InfoLevel:
		return charm.InfoLevel
	case logport.WarnLevel:
		return charm.WarnLevel
	case logport.ErrorLevel:
		return charm.ErrorLevel
	case logport.FatalLevel:
		return charm.FatalLevel
	case logport.PanicLevel:
		return charm.FatalLevel
	default:
		return charm.InfoLevel
	}
}

func zerologEventForLevel(logger zerolog.Logger, level logport.Level) *zerolog.Event {
	switch level {
	case logport.TraceLevel:
		return logger.Trace()
	case logport.DebugLevel:
		return logger.Debug()
	case logport.InfoLevel:
		return logger.Info()
	case logport.WarnLevel:
		return logger.Warn()
	case logport.ErrorLevel, logport.FatalLevel, logport.PanicLevel:
		return logger.Error()
	default:
		return logger.Info()
	}
}

func zapLevelFromPort(level logport.Level) zapcore.Level {
	switch level {
	case logport.TraceLevel:
		return zapcore.DebugLevel
	case logport.DebugLevel:
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
		return zapcore.DPanicLevel
	default:
		return zapcore.InfoLevel
	}
}

func phusluEntryForLevel(logger *plog.Logger, level logport.Level) *plog.Entry {
	switch level {
	case logport.TraceLevel:
		return logger.Trace()
	case logport.DebugLevel:
		return logger.Debug()
	case logport.InfoLevel:
		return logger.Info()
	case logport.WarnLevel:
		return logger.Warn()
	case logport.ErrorLevel:
		return logger.Error()
	case logport.FatalLevel:
		// avoid exiting the process; treat as Error
		return logger.Error()
	case logport.PanicLevel:
		return logger.Error()
	default:
		return logger.Info()
	}
}

func slogLevelFromPort(level logport.Level) slog.Level {
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
	case logport.FatalLevel:
		return slog.LevelError + 4
	case logport.PanicLevel:
		return slog.LevelError + 5
	default:
		return slog.LevelInfo
	}
}
