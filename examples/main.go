package main

import (
	"os"

	"github.com/sa6mwa/logport/adapters/charmlogger"
	"github.com/sa6mwa/logport/adapters/phuslu"
	"github.com/sa6mwa/logport/adapters/zerologger"
)

func main() {
	logger := phuslu.New(os.Stdout).With("logAdapter", "phuslu")
	logger.Info("Hello, this is the phuslu log adapter", "hello", "world", "phuslu", true)
	logger.Warn("Warning", "phuslu", true)
	logger.Error("Error", "phuslu", true, "zerologger", false)
	logger.Debug("Debug message")

	logger = zerologger.New(os.Stdout).With("logAdapter", "zerologger", "zerologger", true)
	logger.Info("Hello, this is the zerologger log adapter, defaults to the colored console writer")
	logger.Warn("Warning message")
	logger.Error("Error message")
	logger.Debug("Debug message")

	logger = charmlogger.New(os.Stdout).With("logAdapter", "charmbracelet/log", "charmlogger", true)
	logger.Info("Hello, this is charmbracelet's log adapter")
	logger.Warn("Warning message")
	logger.Error("Error message")
	logger.Debug("Debug message")
}
