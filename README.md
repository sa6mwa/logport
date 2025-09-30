# logport

`logport` defines a small logging port in Go and provides adapters for popular
logging back ends so that applications can depend on a single interface while
remaining free to swap concrete loggers.

## Highlights

- Uniform interface with structured logging support via `With`, `WithGroup`, and
  full `slog.Handler` compliance.
- Level-aware helpers: `Trace`, `Debug`, `Info`, `Warn`, `Error`, `Fatal`, and
  `Panic` plus chaining through `LogLevel` to create derived loggers.
- New log levels include:
  - `TraceLevel` (below debug)
  - `NoLevel` (emit entries without a severity when the backend supports it)
  - `Disabled`
- Adapters for:
  - Charmbracelet [log](https://github.com/charmbracelet/log)
  - [phuslu/log](https://github.com/phuslu/log)
  - [rs/zerolog](https://github.com/rs/zerolog)
  - [go.uber.org/zap](https://github.com/uber-go/zap)

## Quick Start

```go
package main

import (
    "os"

    port "github.com/sa6mwa/logport"
    "github.com/sa6mwa/logport/adapters/zerologger"
)

func main() {
    logger := zerologger.New(os.Stdout)
    logger.With("component", "worker").Info("ready", "addr", ":8080")

    // Derive a debug logger without mutating the original.
    debug := logger.LogLevel(port.DebugLevel)
    debug.Trace("debugging payload", "payload", map[string]any{"a": 1})
}
```

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

Adapter-specific tests assert `NoLevel` behaviour, `LogLevel` chaining, and the
additional helper methods to prevent regressions.
