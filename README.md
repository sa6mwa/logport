# logport

`logport` defines a flexible logging port in Go and provides adapters for
popular logging back ends so that applications can depend on a single
capability-rich interface while remaining free to swap concrete loggers.

### Why logport?

Logport aims to be the “do everything” façade you reach for when the
application—not the logging backend—should dictate ergonomics. You won’t find
another Go project that combines a feature-rich interface *and* a native,
low-latency adapter in the same package; existing façades (go-kit/kit/log,
logr) keep their surface intentionally tiny, while high-performance libraries
(zerolog, onelog, phuslu/log, zap) expose their own APIs. Logport lets you keep
the richer façade and still swap between those back ends as needed.

- **Interface surface.** Lightweight emitters like onelog, zerolog, and
  phuslu/log are legendary for speed, but they expose narrowly scoped APIs.
  logport layers key/value helpers, printf fallbacks, environment-driven level
  promotion, slog handler compatibility, context propagation, minimal subsets,
  and a `log.Logger` bridge on top of any backend. Zap and charm/log offer rich
  features too, yet they expect you to program directly against their handler
  models. With logport you get the richer façade while retaining the freedom to
  swap concrete loggers per deployment or per integration.
- **Performance in practice.** The native PSL adapter keeps the façade honest:
  console output with timestamps lands around **40 ns/op**, and dropping
  timestamps pushes it to the low 30 ns band—close to onelog and
  phuslu/log despite PSL’s extra conveniences (colour-aware console output,
  json/console dual mode, UTC toggles, short vs. verbose JSON fields).
  Structured PSL output stays allocation-free and remains competitive even
  after we add RFC3339 timestamps—something many micro-loggers leave out by
  default.
- **Adapter breadth.** Need zerolog to feed structured ingestion, zap for
  ecosystem tooling, slog for standard-library integrations, or charm/log for a
  developer-friendly console? logport ships adapters for each, so the interface
  you code against stays unchanged. Benchmarks in this document use the same
  sink and timestamp policy across adapters precisely so you can compare them on
  equal footing.
- **Transparent trade-offs.** logport deliberately avoids areas already handled
  well by specialised libraries: log file rotation, sampling/rate limiting,
  metrics emission, dynamic configuration backplanes. You can still obtain those
  by selecting an adapter that supports them (e.g., zap’s sampling, phuslu’s
  rolling writers) or by composing logport with external sinks. The façade
  keeps you decoupled from those choices.

If you need a unified, feature-rich interface without giving up the ability to
leverage the fastest emitters in the Go ecosystem, logport plus its PSL adapter
is designed to sit in that sweet spot.

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
  - Go's standard [`log/slog`](https://pkg.go.dev/log/slog)
  - Native high-performance `psl` adapter for console and JSON output

## Quick Start

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
    // Native PSL: console output with cached timestamps and colour when terminal-aware.
    logger := psl.New(os.Stderr)
    logger.With("component", "worker").Info("ready", "addr", ":8080")

    // Switch PSL to structured JSON while keeping zero allocations.
    jsonLogger := psl.NewStructured(os.Stderr).With("component", "worker").WithLogLevel()
    jsonLogger.Info("ready", "addr", ":8080")

    // Propagate PSL via context and retrieve it down the call stack.
    ctx := port.ContextWithLogger(context.Background(), psl.New(os.Stderr))
    doWork(ctx)

    // Wrap PSL in a stdlib *log.Logger for legacy integrations.
    std := port.LogLogger(psl.New(os.Stderr))
    std.Println("legacy library ready")

    // Pin net/http's ErrorLog to ErrorLevel while still stripping prefixes.
    httpErr := port.LogLoggerWithLevel(psl.New(os.Stderr), port.ErrorLevel)
    srv := &http.Server{Addr: ":8080", ErrorLog: httpErr}
    go srv.ListenAndServe()

    // Drop in another backend (charmbracelet/log) without changing call sites.
    charm := charmlogger.New(os.Stderr)
    charm.With("component", "worker").Warn("slow response", "duration", "120ms")

    // Pick onelog when you want lean JSON with minimal allocation overhead.
    minimal := onelogger.New(os.Stderr)
    minimal.Info("ready", "component", "worker")

    // Derive a debug logger without mutating the original.
    debug := logger.LogLevel(port.DebugLevel)
    debug.Trace("debugging payload", "payload", map[string]any{"a": 1})

    // Elevate level from the environment and record it in-line.
    runtimeLogger := logger.LogLevelFromEnv("APP_LOG_LEVEL").WithLogLevel()
    runtimeLogger.Info("running", "pid", os.Getpid())
}

func doWork(ctx context.Context) {
    log := port.LoggerFromContext(ctx)
    log.Info("processing", "job", 42)
}
```

### Stdlib compatibility

`ForLogging` now satisfies `io.Writer`, so any adapter can plug into helpers
that expect a writer—including the standard library's `log.Logger`. Use
`logport.LogLogger(forLogging)` to obtain a prefix-free `*log.Logger` that
funnels calls back through the adapter, keeping legacy packages on the same
output pipeline as your structured logs. When the legacy integration represents
exactly one severity—`net/http.Server.ErrorLog` is a common example—wrap via
`logport.LogLoggerWithLevel(forLogging, logport.ErrorLevel)` to pin every write
to a specific level while still stripping redundant prefixes from the message.

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
- **slogger** – use `New` for text output, `NewJSON` for structured logs, or
  `NewWithHandler` / `NewWithLogger` when you already have a custom slog
  handler configured elsewhere.
- **charmlogger** – choose `New` for colourful console output or
  `NewStructured` to switch the adapter to JSON (`log.JSONFormatter`) while
  keeping the same timestamp defaults.
- **psl** – a native console/JSON adapter modelled on zerolog's appearance with
  terminal-aware colour, no allocations on the hot path, cached timestamps (now
  covering the built-in Go layouts, including `time.DateTime` and
  `port.DTGTimeFormat`), and optional colourful JSON output (`Options{ColorJSON: true}`)
  that only engages when the destination is a TTY. Toggle `Options{VerboseFields: true}`
  to expand JSON keys from `ts/lvl/msg` to `time/level/message`, set
  `Options{UTC: true}` to force UTC timestamps, and note that the adapter now
  mirrors charm’s optimisation by short-circuiting when pointed at `io.Discard`.
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
Go 1.25.1. Every run writes to a locked sink that mimics container stdout/stderr
instead of `io.Discard`, so the numbers account for both formatter work and the
small amount of synchronisation you get in typical deployment environments.
After adding structured JSON fuzz tests to PSL (to guarantee escaping under
arbitrary inputs) we reran the suite; the figures below remain within normal
variance of previous runs.

### Through the logport adapters (`logger.Info`)

`go test -run=^$ -bench BenchmarkAdapterInfo -benchmem -benchtime=50ms`

| Adapter         | Info ns/op | B/op | allocs/op |
|-----------------|------------|------|-----------|
| psl/console     | 39.9       | 0    | 0         |
| psl/json        | 59.8       | 0    | 0         |
| phuslu          | 80.0       | 0    | 0         |
| zerolog/json    | 138.0      | 0    | 0         |
| onelog          | 181.0      | 24   | 1         |
| zap             | 266.0      | 0    | 0         |
| slog/json       | 399.0      | 0    | 0         |
| slog/text       | 421.0      | 0    | 0         |
| charm/json      | 1100.0     | 419  | 16        |
| charm/console   | 2630.0     | 432  | 16        |
| zerolog/console | 3240.0     | 1658 | 31        |

**What drives the gaps?**

- **Timestamp policy:** PSL caches formatted timestamps when they’re enabled
  (`timeCache.Current()`), so the console hot path is just an atomic load plus a
  pre-baked DTG append. Disabling timestamps removes even that cost, trimming a
  few extra nanoseconds. Onelog ships with no timestamp, so we add one with a
  hook in this benchmark; the ~181 ns/op reflects that extra RFC3339 work. The
  no-timestamp suite below shows onelog back near 40 ns/op when the hook is
  removed.
- **Formatter complexity:** Console loggers spend real time producing human
  output. Charmbracelet/log renders styled strings and colour sequences; even
  with a mutex-bound sink it sits in the low microseconds. Zerolog’s console
  writer is similarly deliberate, so it remains multiple microseconds slower
  than the JSON emitters.
- **Allocations:** All of PSL’s paths stay allocation-free thanks to pooled
  buffers. Onelog’s adapter still allocates when it marshals via its hook, and
  charm/zerolog console allocate because they rebuild the formatted line for
  every write. The new kitlog/logr entries highlight how general-purpose
  façades pay for additional wrappers (hundreds of bytes per log event) even
  before formatting complexity kicks in.

### Native APIs (outside logport)

`go test -run=^$ -bench BenchmarkNativeLoggerInfo -benchmem -benchtime=50ms`

| Logger/API        | Info ns/op | B/op | allocs/op |
|-------------------|------------|------|-----------|
| psl console       | 35.4       | 0    | 0         |
| psl JSON          | 58.5       | 0    | 0         |
| phuslu/log JSON   | 76.8       | 0    | 0         |
| zerolog JSON      | 140.0      | 0    | 0         |
| onelog JSON       | 143.4      | 24   | 1         |
| zap JSON          | 268.0      | 0    | 0         |
| slog JSON         | 374.3      | 0    | 0         |
| kitlog/json       | 1327.0     | 816  | 13        |
| logr/funcr        | 1473.0     | 1360 | 9         |
| charm/log console | 2906.0     | 432  | 16        |
| charm/log JSON    | 1272.0     | 419  | 16        |
| zerolog console   | 2828.0     | 1658 | 31        |

These figures reflect each library’s preferred entry point (e.g.,
`logger.Info().Msg`, `zap.New(core).Info`). Once the discard fast-path is out of
the picture, PSL’s native console stack sits alongside onelog/phuslu in the
sub-100 ns bracket, while the console-focused libraries spend microseconds on
deliberate formatting work.

`BenchmarkNativeLoggerInfoNoTimestamp` disables timestamps wherever the library
allows it (PSL, zerolog, charm, zap, slog, onelog). phuslu/log currently lacks a
clean switch for removing the time field, so it is skipped in that variant.

| Logger/API (no ts) | Info ns/op | B/op | allocs/op |
|--------------------|------------|------|-----------|
| onelog JSON        | 42.1       | 0    | 0         |
| psl console        | 34.2       | 0    | 0         |
| psl JSON           | 50.2       | 0    | 0         |
| zerolog JSON       | 68.7       | 0    | 0         |
| zap JSON           | 193.9      | 0    | 0         |
| slog JSON          | 766.9      | 120  | 3         |
| kitlog/json        | 738.4      | 576  | 9         |
| logr/funcr         | 763.0      | 1248 | 6         |
| charm/log console  | 2038.0     | 264  | 13        |
| charm/log JSON     | 779.7      | 218  | 11        |
| zerolog console    | 2914.0     | 1514 | 26        |

These figures reflect each library’s preferred entry point (e.g.,
`logger.Info().Msg`, `zap.New(core).Info`). Once the discard fast-path is out of
the picture, PSL’s native console stack sits alongside onelog/phuslu in the
sub-100 ns bracket, while console-focused libraries spend microseconds on
deliberate formatting work. The kitlog/logr rows show what happens when you
layer additional abstraction on top of logging—useful for instrumentation, but
costly when every nanosecond matters. The no-timestamp variant underlines how
much of onelog’s cost is simply the added timestamp hook (42 ns vs. 143 ns),
and how PSL’s cached timestamps keep the penalty minimal (~60 ns with
timestamps, ~50 ns without).

## Testing

Run the full suite with:

```
go test ./...
```

Adapter-specific tests assert `NoLevel` behaviour, `LogLevel` chaining,
environment-driven level changes, compatibility with `log.Logger`, and the
additional helper methods to prevent regressions.
