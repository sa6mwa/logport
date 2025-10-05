# logport

`logport` defines a flexible logging port in Go and provides adapters for
popular logging back ends so that applications can depend on a single
capability-rich interface while remaining free to swap concrete loggers.

## Highlights

- Uniform interface with structured logging support via `With`, `WithGroup`, and
  full `slog.Handler` compliance (including direct acceptance of `slog.Attr`
  arguments like `logger.Info("msg", slog.String("key", "value"))`).
- Convenience `*f` helpers (`Debugf`, `Infof`, etc.) when you want printf-style
  messages—handy for quick logs, though key/value logging remains the
  allocation-free option for hot paths.
- Level-aware helpers: `Trace`, `Debug`, `Info`, `Warn`, `Error`, `Fatal`, and
  `Panic` plus chaining through `LogLevel`. `LogLevelFromEnv("APP_LOG_LEVEL")`
  lets you promote severity at runtime, and `WithLogLevel()` stamps the current
  level into log output for auditability.
- New log levels include:
  - `TraceLevel` (below debug)
  - `NoLevel` (emit entries without a severity when the backend supports it)
  - `Disabled`
- Flexible output selection per adapter: console-style previews by default, or
  fully structured JSON with `zerologger.NewStructured` / `Options{Structured:true}`.
- Adapters for:
  - Charmbracelet [log](https://github.com/charmbracelet/log)
  - [phuslu/log](https://github.com/phuslu/log)
  - [rs/zerolog](https://github.com/rs/zerolog)
  - [francoispqt/onelog](https://github.com/francoispqt/onelog)
  - [go.uber.org/zap](https://github.com/uber-go/zap)

## Quick Start

```go
package main

import (
    "os"

    port "github.com/sa6mwa/logport"
    onelogger "github.com/sa6mwa/logport/adapters/onelogger"
    "github.com/sa6mwa/logport/adapters/zerologger"
)

func main() {
    logger := zerologger.New(os.Stdout)
    logger.With("component", "worker").Info("ready", "addr", ":8080")

    // Switch to JSON output if you'd rather stream structured logs.
    jsonLogger := zerologger.NewStructured(os.Stdout)
    jsonLogger.Info("ready", "component", "worker", "addr", ":8080")

    // Pick onelog when you want lean JSON with minimal allocation overhead.
    minimal := onelogger.New(os.Stdout)
    minimal.Info("ready", "component", "worker")

    // Derive a debug logger without mutating the original.
    debug := logger.LogLevel(port.DebugLevel)
    debug.Trace("debugging payload", "payload", map[string]any{"a": 1})

    // Elevate level from the environment and record it in-line.
    runtimeLogger := logger.LogLevelFromEnv("APP_LOG_LEVEL").WithLogLevel()
    runtimeLogger.Info("running", "pid", os.Getpid())
}
```

### Runtime level control

`LogLevelFromEnv` recognises the following (case insensitive) values: `trace`,
`debug`, `info`, `warn`, `warning`, `error`, `fatal`, `panic`, `no`, `nolevel`,
`disabled`, and `off`. Missing or invalid values are silently ignored. Pair
`LogLevelFromEnv` with `WithLogLevel()` to add a `loglevel` field to each entry
so downstream systems can confirm what severity is active without scanning
configuration.

## Log Levels

| Name        | Description                                                   |
|-------------|---------------------------------------------------------------|
| `TraceLevel`| Very fine-grained diagnostics below debug.                    |
| `DebugLevel`| Development diagnostics.                                      |
| `InfoLevel` | Routine operational messages.                                 |
| `WarnLevel` | Non-fatal issues that require attention.                      |
| `ErrorLevel`| Failures handled within the process.                          |
| `FatalLevel`| Logs and terminates the process via `os.Exit(1)`.             |
| `PanicLevel`| Logs and panics after emitting the message.                   |
| `NoLevel`   | Emits entries without a severity tag if the backend supports it.|
| `Disabled`  | Turns off logging.                                            |

Use `LogLevel(port.Level)` to derive a logger constrained to a minimum
threshold (or to drop the level for `NoLevel`). The original logger is left
untouched so you can fan out per-component loggers easily.

## NoLevel Behaviour per Adapter

| Adapter        | Behaviour                                                  |
|----------------|------------------------------------------------------------|
| Charmbracelet  | Reuses `Print` so output contains message/fields but no level. |
| phuslu/log     | Uses `logger.Log()` to omit the `level` key from JSON output. |
| zerolog JSON   | Emits raw JSON without a `level` field.                     |
| zerolog Console| Displays the built-in `???` placeholder (zerolog default).  |
| zap            | Maps `NoLevel` to `Debug` (zap lacks a native level-less mode). |

## Adapter Notes

- **onelogger** – timestamps are injected by default using `port.DTGTimeFormat`
  (`HHMMSS`). Supply `Options{TimeFormat: time.RFC3339}` to change the layout or
  `Options{DisableTimestamp: true}` to suppress the field entirely.
- **zerologger** – pass `Options{Structured: true}` or call `NewStructured` for
  raw JSON logs, and `Options{DisableTimestamp: true}` to leave the timestamp
  out altogether.
- **zaplogger** – the adapter tracks the configured level so
  `WithLogLevel()` reflects the active zap core even after calling
  `LogLevelFromEnv` or chaining additional `With(...)` calls.

## Panic and Fatal Helpers

`Fatal` exits the process with status code 1 after logging. `Panic` logs at the
highest level available for the backend and then panics, matching each
underlying logger’s expectations.

## Context Integration

`ContextWithLogger` and `LoggerFromContext` let you stash adapter instances in a
`context.Context`, making it easy to thread loggers through request-scoped
workflows. Adapters remain `slog.Handler`s, letting you bridge between the port
and Go’s structured logging ecosystem.

## Testing

Run the full suite with:

```
go test ./...
```

Adapter-specific tests assert `NoLevel` behaviour, `LogLevel` chaining,
environment-driven level changes, and the additional helper methods to prevent
regressions.
