package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/sa6mwa/logport"
	"github.com/sa6mwa/logport/adapters/charmlogger"
	"github.com/sa6mwa/logport/adapters/phuslu"
	"github.com/sa6mwa/logport/adapters/zaplogger"
	"github.com/sa6mwa/logport/adapters/zerologger"
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

	logger = zerologger.NewStructured(os.Stdout).With("adapter", "zerologger")
	logger.Warn("This is zerologger.NewStructured(os.Stdout)")

	zopts = zerologger.Options{
		DisableTimestamp: true,
		Structured:       true,
	}
	logger = zerologger.NewWithOptions(os.Stdout, zopts).With("adapter", "zerologger").With("options", zopts)
	logger.Error("This is zerologger.NewWithOptions with disabled timestamps")

	fmt.Println("")

	logger = charmlogger.New(os.Stdout).With("logAdapter", "charmbracelet/log", "charmlogger", true)
	logger.Info("Hello, this is charmbracelet's log adapter")
	logger.Warn("Warning message")
	logger.Error("Error message")
	logger.Debug("Debug message")
	logger.Info("This is using slog.Attr groups",
		slog.String("stringType", "string"),
		slog.Duration("duration", 5*time.Second),
		"normalKey", "normalValue",
	)
	logger.With(slog.String("via", "Warnf")).Warnf("This is a %q message", "convenient")
	fmt.Println("")

	logger = zaplogger.New(os.Stdout).With("logAdapter", "zap").LogLevel(logport.WarnLevel).With("loglevel", logport.WarnLevel)
	logger.Warn("Hello, this is Uber's zap log adapter")
	logger.Info("Info message, this should not show")
	logger.Error("Error message")
	logger.Debug("Debug message")
	logger.Warn("This is using slog.Attr groups",
		slog.String("stringType", "string"),
		slog.Duration("duration", 5*time.Second),
	)
	logger.With(slog.String("via", "Warnf")).Warnf("This is a %q message", "convenient")
}
