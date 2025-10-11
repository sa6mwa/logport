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

    port "pkt.systems/logport"
    onelogger "pkt.systems/logport/adapters/onelogger"
    "pkt.systems/logport/adapters/zerologger"
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

### Stdlib compatibility

`ForLogging` now satisfies `io.Writer`, so any adapter can plug into helpers
that expect a writer—including the standard library's `log.Logger`. Use
`logport.LogLogger(forLogging)` to obtain a prefix-free `*log.Logger` that
funnels calls back through the adapter, keeping legacy packages on the same
output pipeline as your structured logs.

Writes are classified with inexpensive heuristics: leading tags such as
`[ERROR]`, `INFO:`, `warn -` and similar formats are stripped and remapped to
the appropriate severity, while lines that merely contain the word `error`
fall back to `ErrorLevel`. Messages that fail to match any hint are treated as
`NoLevel`, allowing adapters to decide whether to emit or drop them based on
their configured thresholds.

### Minimal subsets

The top-level `ForLogging` interface now embeds `ForLoggingMinimalSubset`, a
four-method contract (`Debug`, `Info`, `Warn`, `Error`) that mirrors what many
ecosystems—including Temporal's Go SDK—expect from a logger. When you only need
basic levelled logging for an integration, depend on the minimal subset: every
logport adapter (and `ForLogging` implementation) satisfies it automatically,
while third-party implementations can provide just those methods and still plug
into code that understands logport semantics. Additional subsets can grow over
time without forcing downstream consumers to depend on the full interface.

### Runtime level control

`LogLevelFromEnv` recognises the following (case insensitive) values: `trace`,
`debug`, `info`, `warn`, `warning`, `error`, `fatal`, `panic`, `no`, `nolevel`,
`disabled`, and `off`. Missing or invalid values are silently ignored. Pair
`LogLevelFromEnv` with `WithLogLevel()` to add a `loglevel` field to each entry
so downstream systems can confirm what severity is active without scanning
configuration. When you need to mirror `slog.Logger.Log`, use `Log(ctx, level,
msg, keyvals...)`. Prefer `Logp(port.DebugLevel, ...)` for the richer logport
levels, `Logs("warn", ...)` when the severity arrives as text, or
`Logf(port.InfoLevel, "ready in %v", took)` for formatted messages.

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
- **charmlogger** – choose `New` for colourful console output or
  `NewStructured` to switch the adapter to JSON (`log.JSONFormatter`) while
  keeping the same timestamp defaults.
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

## Benchmarks

Benchmarks were gathered on my machine (13th Gen Intel® Core™ i7-1355U) using
Go 1.25.1 with `go test -run=^$ -bench BenchmarkAdapter -benchmem -benchtime=50ms`.
Numbers are indicative rather than absolute—they include the adapters’ own
formatting costs.

### Direct adapter calls (`logger.Info`/`Error`)

| Adapter         | Info ns/op | B/op | allocs/op |
|-----------------|------------|------|-----------|
| charm/json      | 29.29      | 16   | 1         |
| charm/console   | 33.36      | 16   | 1         |
| phuslu          | 89.70      | 0    | 0         |
| zerolog/json    | 171.50     | 0    | 0         |
| onelog          | 214.60     | 8    | 1         |
| zap             | 274.40     | 0    | 0         |
| zerolog/console | 3240.00    | 1642 | 31        |

Error-level calls track the same ordering within a few nanoseconds of the
`Info` numbers. Charmbracelet/log now ships both text (`New`) and JSON
(`NewStructured`) helpers; the JSON formatter is the fastest path but still
allocates a small buffer per call. phuslu/log avoids allocations entirely while
remaining sub-100 ns. Zerolog’s structured (`NewStructured`) mode is competitive
with other JSON emitters, whereas the console formatter naturally takes longer
because it renders colorized human output.

### `log.Logger` compatibility path (`logport.LogLogger`)

| Adapter         | `[INFO]` ns/op (B/op, allocs) | no-match ns/op (B/op, allocs) | substring `error` ns/op (B/op, allocs) |
|-----------------|------------------------------|--------------------------------|----------------------------------------|
| charm/console   | 226.8 (96, 4)                | 310.1 (96, 4)                  | 334.4 (112, 4)                         |
| charm/json      | 301.8 (96, 4)                | 276.0 (96, 4)                  | 348.5 (112, 4)                         |
| phuslu          | 281.8 (96, 3)                | 320.0 (96, 3)                  | 403.0 (112, 3)                         |
| zerolog/json    | 483.6 (176, 3)               | 541.2 (176, 3)                 | 485.9 (192, 3)                         |
| zap             | 504.0 (80, 3)                | 231.3 (80, 3)                  | 594.4 (96, 3)                          |
| onelog          | 466.3 (120, 4)               | 466.6 (112, 4)                 | 620.5 (136, 4)                         |
| zerolog/console | 3646.0 (1787, 34)            | 3737.0 (1691, 26)              | 4092.0 (1820, 34)                      |

The stdlib bridge adds ~220–350 ns for the Go-based adapters (charmbracelet and
phuslu) and roughly half a microsecond for the structured JSON emitters.
Zerolog’s structured mode stays in the same band as zap and onelog, while the
console writer remains several microseconds because it renders human-friendly
output with formatting. Entries that fall back to `NoLevel` (e.g., “plain
telemetry line”) are subject to the adapter’s minimum level; zap keeps its
default `Info` threshold and drops them, explaining the faster no-match timing.

Choose the adapter that matches your output requirements: charmbracelet/log now
offers both console and JSON helpers, phuslu/log balances speed with zero
allocations, zerolog’s structured mode is competitive for JSON workflows while
its console mode prioritises aesthetics, and zap/onelog provide structured
output with mature ecosystems.

> **Note:** onelog’s raw benchmarks focus on its minimal API. The adapter adds
> port features (`With`, level coercion, timestamp hooks) so the figures above
> include that glue. Disable the timestamp hook via `Options{DisableTimestamp: true}`
> or avoid `With` for absolute minimalism.

## Testing

Run the full suite with:

```
go test ./...
```

Adapter-specific tests assert `NoLevel` behaviour, `LogLevel` chaining,
environment-driven level changes, compatibility with `log.Logger`, and the
additional helper methods to prevent regressions.
