package logport_test

import (
	"log/slog"
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

	psl "pkt.systems/logport/adapters/psl"
)

type nativeBenchConfig struct {
	withTimestamp bool
}

type nativeLogger struct {
	name                  string
	supportsTimestampLess bool
	run                   func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig)
}

func BenchmarkNativeLoggerInfo(b *testing.B) {
	runNativeLoggerBenchmarks(b, nativeBenchConfig{withTimestamp: true})
}

func BenchmarkNativeLoggerInfoNoTimestamp(b *testing.B) {
	runNativeLoggerBenchmarks(b, nativeBenchConfig{withTimestamp: false})
}

func runNativeLoggerBenchmarks(b *testing.B, cfg nativeBenchConfig) {
	sink := newBenchmarkSink()
	for _, bench := range nativeLoggers() {
		bench := bench
		b.Run(bench.name, func(b *testing.B) {
			if !cfg.withTimestamp && !bench.supportsTimestampLess {
				b.Skip("timestamp toggle not supported")
				return
			}
			bench.run(b, sink, cfg)
		})
	}
}

func nativeLoggers() []nativeLogger {
	return []nativeLogger{
		{
			name:                  "psl/console",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				opts := psl.Options{Mode: psl.ModeConsole}
				if !cfg.withTimestamp {
					opts.DisableTimestamp = true
				}
				logger := psl.NewWithOptions(sink, opts)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info("bench")
				}
			},
		},
		{
			name:                  "psl/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				opts := psl.Options{Mode: psl.ModeStructured}
				if cfg.withTimestamp {
					opts.ColorJSON = true
				} else {
					opts.DisableTimestamp = true
				}
				logger := psl.NewWithOptions(sink, opts)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info("bench")
				}
			},
		},
		{
			name:                  "zerolog/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				builder := zerolog.New(sink)
				if cfg.withTimestamp {
					builder = builder.With().Timestamp().Logger()
				} else {
					builder = builder.With().Logger()
				}
				logger := builder
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info().Msg("bench")
				}
			},
		},
		{
			name:                  "zerolog/console",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				writer := zerolog.ConsoleWriter{
					Out:     sink,
					NoColor: true,
				}
				if cfg.withTimestamp {
					writer.TimeFormat = time.RFC3339
				} else {
					writer.TimeFormat = ""
				}
				logger := zerolog.New(writer)
				if cfg.withTimestamp {
					logger = logger.With().Timestamp().Logger()
				} else {
					logger = logger.With().Logger()
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info().Msg("bench")
				}
			},
		},
		{
			name:                  "charm/console",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				opts := charm.Options{
					ReportTimestamp: cfg.withTimestamp,
					TimeFormat:      time.RFC3339,
				}
				logger := charm.NewWithOptions(sink, opts)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info("bench")
				}
			},
		},
		{
			name:                  "charm/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				opts := charm.Options{
					Formatter:       charm.JSONFormatter,
					ReportTimestamp: cfg.withTimestamp,
					TimeFormat:      time.RFC3339,
				}
				logger := charm.NewWithOptions(sink, opts)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info("bench")
				}
			},
		},
		{
			name:                  "slog/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				var handler slog.Handler
				if cfg.withTimestamp {
					handler = slog.NewJSONHandler(sink, &slog.HandlerOptions{})
				} else {
					handler = slog.NewJSONHandler(sink, &slog.HandlerOptions{
						ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
							if a.Key == slog.TimeKey {
								return slog.Attr{}
							}
							return a
						},
					})
				}
				logger := slog.New(handler)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info("bench")
				}
			},
		},
		{
			name:                  "zap/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				encoderCfg := zap.NewProductionEncoderConfig()
				if !cfg.withTimestamp {
					encoderCfg.TimeKey = ""
				} else {
					encoderCfg.TimeKey = "ts"
				}
				encoder := zapcore.NewJSONEncoder(encoderCfg)
				core := zapcore.NewCore(encoder, zapcore.AddSync(sink), zapcore.InfoLevel)
				logger := zap.New(core, zap.WithCaller(false))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info("bench")
				}
			},
		},
		{
			name:                  "phuslu/json",
			supportsTimestampLess: false,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				logger := &plog.Logger{
					Level:  plog.InfoLevel,
					Writer: plog.IOWriter{Writer: sink},
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info().Msg("bench")
				}
			},
		},
		{
			name:                  "onelog/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				logger := onelog.New(sink, onelog.ALL)
				if cfg.withTimestamp {
					logger.Hook(func(e onelog.Entry) {
						e.String("ts", time.Now().UTC().Format(time.RFC3339))
					})
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info("bench")
				}
			},
		},
		{
			name:                  "kitlog/json",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				logger := kitlog.NewJSONLogger(sink)
				logger = kitlog.With(logger, "level", "info")
				if cfg.withTimestamp {
					logger = kitlog.With(logger, "ts", kitlog.DefaultTimestampUTC)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_ = logger.Log("msg", "bench")
				}
			},
		},
		{
			name:                  "logr/funcr",
			supportsTimestampLess: true,
			run: func(b *testing.B, sink *lockedDiscard, cfg nativeBenchConfig) {
				logger := funcr.NewJSON(func(obj string) {
					sink.Write([]byte(obj))
				}, funcr.Options{LogTimestamp: cfg.withTimestamp})
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info("bench")
				}
			},
		},
	}
}
