# logport

`logport` defines a flexible logging port for Go and ships adapters for the
popular structured logging back ends. Your code targets a single, rich façade
while you remain free to swap concrete loggers per deployment or integration.

### Why logport?

- **Broader interface, consistent ergonomics.** Unlike tiny façades, logport
  embraces the conveniences real applications depend on: key/value logging,
  printf helpers, context propagation, environment-driven level selection,
  `slog.Handler` compatibility, and bridges to the standard library. Every
  adapter implements the full surface so callers keep identical ergonomics
  regardless of backend.
- **Performance without ceremony.** The native PSL adapter builds on
  `pkt.systems/pslog v0.3.0`. It caches timestamp state, promotes trusted keys
  when you call `With`, and keeps the hot path allocation-free. Static fields
  live on the logger (mirroring how the benchmark suite pre-attaches dataset
  constants), dynamic payloads stay variadic, and `pslog.Keyvals` is available
  when you need to pre-promote runtime slices.
- **Adapter breadth.** Whether you need zerolog for ingestion, zap for its
  ecosystem, charm/log for developer-friendly consoles, slog for standard library
  integrations, or phuslu/log/onelog for lean pipelines, logport ships native
  adapters so the façade stays unchanged.
- **Transparent trade-offs.** Logport focuses on the call site. Rotation,
  sampling, metrics, and distributed configuration remain the domain of the
  adapters or downstream sinks. Pick the backend that provides the behaviour
  you need without refactoring every caller.

## Highlights

- Structured logging support via `With`, `WithGroup`, and full
  `slog.Handler` compliance (including direct consumption of `slog.Attr`).
- Convenience `*f` helpers (`Debugf`, `Infof`, …) for formatted messages, while
  key/value methods remain allocation-free.
- `WithTrace(ctx)` surfaces OpenTelemetry `trace_id` / `span_id` pairs across
  adapters.
- Level helpers (`Trace` through `Panic`) plus `LogLevel`, `LogLevelFromEnv`,
  and `WithLogLevel` for runtime control and auditability.
- Native adapters for charmbracelet/log, phuslu/log, rs/zerolog, onelog,
  go.uber.org/zap, Go’s `log/slog`, and the zero-allocation PSL adapter.

## Quick start

```go
package main

import (
    "context"
    "net/http"
    "os"

    port "pkt.systems/logport"
    psl "pkt.systems/logport/adapters/psl"
    charmlogger "pkt.systems/logport/adapters/charmlogger"
    onelogger "pkt.systems/logport/adapters/onelogger"
)

func main() {
    // Build a PSL console logger once and attach static fields up front.
    logger := psl.New(os.Stderr).With("component", "worker")
    logger.Info("ready", "addr", ":8080")

    // Zero-allocation structured output.
    jsonLogger := psl.NewStructured(os.Stderr).
        With("component", "worker").
        WithLogLevel()
    jsonLogger.Info("ready", "addr", ":8080")

    // Propagate PSL through a context and recover it downstream.
    ctx := port.ContextWithLogger(context.Background(), logger)
    doWork(ctx)

    // Wrap PSL in the stdlib logger for legacy integrations.
    std := port.LogLogger(psl.New(os.Stderr))
    std.Println("legacy library ready")

    // Pin net/http's ErrorLog to ErrorLevel while still stripping prefixes.
    httpErr := port.LogLoggerWithLevel(psl.New(os.Stderr), port.ErrorLevel)
    srv := &http.Server{Addr: ":8080", ErrorLog: httpErr}
    go srv.ListenAndServe()

    // Swap adapters without touching call sites.
    charm := charmlogger.New(os.Stderr).With("component", "worker")
    charm.Warn("slow response", "duration", "120ms")

    minimal := onelogger.New(os.Stderr).With("component", "worker")
    minimal.Info("ready", "addr", ":8080")

    // Derive a debug logger without mutating the original.
    debug := logger.LogLevel(port.DebugLevel)
    debug.Trace("payload", "payload", map[string]any{"a": 1})

    // Promote level from the environment and surface it in output.
    runtimeLogger := logger.LogLevelFromEnv("APP_LOG_LEVEL").WithLogLevel()
    runtimeLogger.Info("running", "pid", os.Getpid())
}

func doWork(ctx context.Context) {
    port.LoggerFromContext(ctx).WithTrace(ctx).Info("processing", "job", 42)
}
```

### Performance-minded usage

- **Static fields:** attach non-changing fields via `With` during construction.
  PSL promotes those key/value pairs and skips escape checks on subsequent logs.
- **Dynamic payloads:** pass per-call values directly. For reusable runtime
  slices, call `pslog.Keyvals(dynamic...)` once.
- **Timestamps:** PSL caches formatted timestamps by layout; disable them with
  `Options{DisableTimestamp: true}` when chasing every nanosecond.

### OpenTelemetry traces

```go
ctx, span := tracer.Start(ctx, "work")
defer span.End()

logger.WithTrace(ctx).Info("handled request")
```

### Stdlib compatibility

Every adapter implements `io.Writer`, making it trivial to share the logger with
packages that expect a writer, including `log.Logger`. `logport.LogLogger`
returns a prefix-free `*log.Logger`, and `logport.LogLoggerWithLevel` pins every
write to a specific severity while still stripping redundant prefixes.

### Minimal subsets

`ForLogging` embeds `MinimalSubset` (Debug/Info/Warn/Error). Depend on the
minimal subset when you only need basic logging; all adapters implement it, and
bespoke loggers can satisfy just those methods to participate.

### Runtime level control

`LogLevelFromEnv` recognises `trace`, `debug`, `info`, `warn`, `warning`, `error`,
`fatal`, `panic`, `no`, `nolevel`, `disabled`, and `off`. Pair it with
`WithLogLevel()` to embed the selected level. Prefer `Logp` for programmatic
levels, `Logs` for textual severities, and `Logf`/`*f` helpers for formatted
output.

## Log levels

| Name        | Description                                                   |
|-------------|---------------------------------------------------------------|
| `TraceLevel`| Fine-grained diagnostics below debug.                         |
| `DebugLevel`| Development diagnostics.                                      |
| `InfoLevel` | Routine operational messages.                                 |
| `WarnLevel` | Non-fatal issues that require attention.                      |
| `ErrorLevel`| Failures handled within the process.                          |
| `FatalLevel`| Logs and exits (status 1).                                    |
| `PanicLevel`| Logs and panics.                                              |
| `NoLevel`   | Emits entries without severity when supported.               |
| `Disabled`  | Turns logging off.                                            |

## `NoLevel` behaviour per adapter

| Adapter        | Behaviour                                                  |
|----------------|------------------------------------------------------------|
| Charmbracelet  | Reuses `Print`; message/fields emitted without level.      |
| phuslu/log     | Uses `logger.Log()` to omit the `level` key.                |
| zerolog JSON   | Emits JSON without a `level` field.                         |
| zerolog Console| Displays zerolog’s placeholder.                             |
| zap            | Maps `NoLevel` to `Debug` (zap lacks a native level-less mode). |

## Adapter notes

- **onelogger** – timestamps default to `port.DTGTimeFormat`; customise with
  `Options{TimeFormat: ...}` or disable via `Options{DisableTimestamp: true}`.
- **zerologger** – use `NewStructured`/`Options{Structured: true}` for JSON,
  `Options{DisableTimestamp: true}` to drop timestamps.
- **slogger** – `New` for text, `NewJSON` for structured output, or wrap custom
  handlers via `NewWithHandler`/`NewWithLogger`.
- **charmlogger** – `New` for colourful console output or `NewStructured` for
  JSON (`log.JSONFormatter`).
- **psl** – the native adapter built on pslog v0.3.0. It caches timestamps,
  promotes trusted keys during `With`, supports colourful JSON via
  `Options{ColorJSON: true}`, expands JSON keys with `Options{VerboseFields: true}`
  (`ts/lvl/msg` → `time/level/message`), and forces UTC with `Options{UTC: true}`.
- **zaplogger** – tracks the configured level so `WithLogLevel()` reflects the
  underlying zap core after environment overrides or chained `With` calls.

## Panic and fatal helpers

`Fatal` logs and terminates (`os.Exit(1)`); `Panic` logs then panics, matching
each backend.

## Context integration

`ContextWithLogger` / `LoggerFromContext` stash and recover adapters inside a
`context.Context`, so per-request workflows can reuse the same logger without
plumbing it through every call.

## Benchmark suite

The repository includes a standalone module under `benchmark/`. It uses a
production log dataset to exercise each adapter/native logger in two modes:

- **ProductionDataset** – every logger attaches shared static fields via
  `.With(...)`, emits its native timestamp/level metadata, and logs the full
  dynamic payload at `TraceLevel`.
- **ProductionDatasetNoTimestamp** – the same workload but with every logger’s
  timestamp feature disabled to isolate the hot path without time formatting.

Run the suite with:

```
go test ./benchmark -bench=. -run=^$ -benchmem
```

On a 13th-gen Intel® i7 laptop (Go 1.25), the structured adapters land in the
same payload range once the static fields are pre-attached. Representative
numbers:

| Benchmark (timestamps on)   | ns/op | bytes/op | B/op | allocs/op |
|-----------------------------|------:|---------:|-----:|----------:|
| phuslu/json                 |  212  |   416.7  |   0  | 0 |
| psl/json                    |  244  |   408.7  |   0  | 0 |
| zerolog/json                |  328  |   416.7  |   4  | 0 |
| zap/json                    | 1095  |   413.7  | 548  | 1 |
| slog/json (Go 1.25 handler) | 1215  |   422.6  | 466  | 1 |

The `...NoTimestamp` benchmark drops each logger’s time field (PSL’s cached
layout, Zerolog’s `Timestamp()`, Zap’s ISO-8601 `ts`, slog’s `time`, etc.) so you
can compare the raw formatter overhead without time formatting.

> **Heads-up:** Some loggers (e.g. Charm v0.4) deliberately keep their JSON
> schema minimal, so their payloads will be shorter than pslog’s, which emits a
> compact `ts`/`lvl` alias alongside the main fields.

## Testing

```
go test ./...
```

Adapter tests cover `NoLevel` behaviour, level derivation, environment overrides,
standard library bridges, tracing helpers, and the zero-allocation paths to guard
against regressions.
