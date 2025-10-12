// Example of various logport adapters.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"pkt.systems/logport"
	"pkt.systems/logport/adapters/charmlogger"
	"pkt.systems/logport/adapters/onelogger"
	"pkt.systems/logport/adapters/phuslu"
	"pkt.systems/logport/adapters/psl"
	"pkt.systems/logport/adapters/zaplogger"
	"pkt.systems/logport/adapters/zerologger"
)

func main() {
	logger := phuslu.New(os.Stdout).With("logAdapter", "phuslu").LogLevel(logport.TraceLevel)
	logger.Info("Hello, this is the phuslu log adapter", "hello", "world", "phuslu", true)
	logger.Warn("Warning", "phuslu", true)
	logger.Error("Error", "phuslu", true, "zerologger", false)
	logger.Debug("Debug message")
	logger.Info("This is using slog.Attr groups",
		slog.String("stringType", "string"),
		slog.Duration("duration", 5*time.Second),
		"normalKey", "normalValue",
	)
	logger.With(slog.String("via", "Warnf")).Warnf("This is a %q message", "convenient")
	fmt.Println("")

	logger = zerologger.New(os.Stdout).With("logAdapter", "zerologger", "zerologger", true).LogLevel(logport.InfoLevel)
	logger.Info("Hello, this is the zerologger log adapter, defaults to the colored console writer")
	logger.Warn("Warning message")
	logger.Error("Error message")
	logger.Debug("Debug message") // not shown (LogLevel == Info)
	l := logger.LogLevel(logport.NoLevel).With("loglevel", logport.NoLevel)
	l.Info("Hello world, this is log level: NoLevel")
	logger.Info("This is using slog.Attr groups",
		slog.String("stringType", "string"),
		slog.Duration("duration", 5*time.Second),
		"normalKey", "normalValue",
	)
	logger.With(slog.String("via", "Warnf")).Warnf("This is a %q message", "convenient")
	zopts := zerologger.Options{
		DisableTimestamp: true,
	}
	logger = zerologger.NewWithOptions(os.Stdout, zerologger.Options{
		DisableTimestamp: true,
	}).With("adapter", "zerologger", "options", zopts)
	logger.Info("Disabled timestamp field")
	logger.Warn("Disabled timestamp field, warning")
	logger.Logp(logport.WarnLevel, "This is Logp at warning level")
	logger.WithLogLevel().Logs("info", "This is Logs at info level")

	fmt.Println("")

	logger = zerologger.NewStructured(os.Stdout).With("adapter", "zerologger")
	logger.Warn("This is zerologger.NewStructured(os.Stdout)")
	logger.WithLogLevel().Log(context.Background(), slog.LevelInfo, "This is Log, a replacement for slog.Logger.Log", "slog.LevelInfo", slog.LevelInfo)

	zopts = zerologger.Options{
		DisableTimestamp: true,
		Structured:       true,
	}
	logger = zerologger.NewWithOptions(os.Stdout, zopts).With("adapter", "zerologger").With("options", zopts)
	logger.Error("This is zerologger.NewWithOptions with disabled timestamps")

	os.Setenv("EXAMPLE_LOG_LEVEL", "WARNING")
	logger = logger.LogLevelFromEnv("EXAMPLE_LOG_LEVEL").WithLogLevel()
	logger.Info("This should not show as we set log level to WARNING from the environment")
	logger.With("LogLevelFromEnv", true, "os.Getenv(\"EXAMPLE_LOG_LEVEL\")", os.Getenv("EXAMPLE_LOG_LEVEL")).Warn("This should show")

	logger = zerologger.New(os.Stdout).LogLevelFromEnv("EXAMPLE_LOG_LEVEL").With("LogLevelFromEnv", os.Getenv("EXAMPLE_LOG_LEVEL")).WithLogLevel()
	logger.Debug("This should not show as we set LogLevelFromEnv to WARNING")
	logger.Error("This should show")
	logger.Warn("This should also show")

	fmt.Println("")

	logger = charmlogger.New(os.Stdout).With("logAdapter", "charmbracelet/log", "charmlogger", true)
	logger.Info("Hello, this is charmbracelet's log adapter")
	logger.Warn("Warning message")
	logger.Error("Error message")
	logger.WithLogLevel().Debug("Debug message")
	logger.Info("This is using slog.Attr groups",
		slog.String("stringType", "string"),
		slog.Duration("duration", 5*time.Second),
		"normalKey", "normalValue",
	)
	logger.With(slog.String("via", "Warnf")).Warnf("This is a %q message", "convenient")
	fmt.Println("")

	logger = zaplogger.New(os.Stdout).With("logAdapter", "zap").LogLevel(logport.WarnLevel).WithLogLevel()
	logger.Warn("Hello, this is Uber's zap log adapter")
	logger.Info("Info message, this should not show")
	logger.Error("Error message")
	logger.Debug("Debug message")
	logger.Warn("This is using slog.Attr groups",
		slog.String("stringType", "string"),
		slog.Duration("duration", 5*time.Second),
	)
	logger.With(slog.String("via", "Warnf")).Warnf("This is a %q message", "convenient")
	fmt.Println("")

	logger = onelogger.New(os.Stdout).With("adapter", "onelogger")
	logger.Info("This is github.com/francoispqt/onelog")
	logger.Warn("A warning message")
	logger.Debug("This is a debug message", "debugging", true)
	logger.Trace("Testing trace level with slog.Group", slog.Group("group", "key", "value", "key2", "value2"))
	oopts := onelogger.Options{
		ContextName: "testContext",
	}
	// options (oopts) will not be printed by onelog for some reason, despite
	// trying a custom JSON marshaller on the Options struct.
	logger = onelogger.NewWithOptions(os.Stdout, oopts).LogLevel(logport.WarnLevel).With("adapter", "onelogger", "options", oopts)
	logger.WithLogLevel().Info("This should not show")
	logger.Warn("This should show as loglevel is Warn")
	oopts = onelogger.Options{
		DisableTimestamp: true,
	}
	logger = onelogger.NewWithOptions(os.Stdout, oopts).With("adapter", "onelogger", "options", oopts)
	logger.Info("Timestamp field (ts) is disabled")
	logger.Warnf("A %s warning %s", "printf", "msg")
	logger = logger.LogLevelFromEnv("EXAMPLE_LOG_LEVEL").With("LogLevelFromEnv", os.Getenv("EXAMPLE_LOG_LEVEL")).WithLogLevel()
	logger.Info("This should not show")
	logger.Warn("This should show")

	logger = psl.New(os.Stdout).With("adapter", "psl").With("mode", "console")
	logger.Info("Hello, this is logport's native adapter in console mode")
	logger.Warn("This is a warning message")
	logger.Error("This is an error message")

	logger = psl.NewStructured(os.Stdout).With("adapter", "psl").With("mode", "structured").WithLogLevel().With("num", 123)
	logger.Info("Hello, this is logport's native structured logger")
	logger.Warn("This is a warning message")
	logger.Error("This is an error")

	popts := psl.Options{
		Mode:       psl.ModeStructured,
		TimeFormat: time.RFC3339,
		ColorJSON:  true,
		UTC:        true,
	}
	logger = psl.NewWithOptions(os.Stdout, popts).With("adapter", "psl")
	logger.Info("This is in UTC")

}
