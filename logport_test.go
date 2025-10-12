package logport

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestContextWithLoggerStoresAndRetrieves(t *testing.T) {
	base := context.Background()
	logger := noopLogger{}
	ctx := ContextWithLogger(base, logger)

	// Ensure a new context is returned when a logger is supplied.
	if ctx == base {
		t.Fatalf("expected child context when logger is non-nil")
	}

	got := LoggerFromContext(ctx)
	if _, ok := got.(noopLogger); !ok {
		t.Fatalf("expected stored logger, got %T", got)
	}
}

func TestContextWithLoggerNilLoggerReturnsOriginal(t *testing.T) {
	base := context.WithValue(context.Background(), "key", 1)
	ctx := ContextWithLogger(base, nil)

	if ctx != base {
		// Context should be unchanged when logger is nil.
		t.Fatalf("expected original context when logger is nil")
	}
}

func TestLoggerFromContextFallbacks(t *testing.T) {
	if _, ok := LoggerFromContext(nil).(noopLogger); !ok {
		t.Fatalf("expected noop logger when context is nil")
	}

	if _, ok := LoggerFromContext(context.Background()).(noopLogger); !ok {
		t.Fatalf("expected noop logger when none stored in context")
	}
}

func TestNoopLoggerBehaviour(t *testing.T) {
	logger := NoopLogger()

	// Verify helper methods do not panic and return noop implementations.
	if _, ok := logger.With("foo", "bar").(noopLogger); !ok {
		t.Fatalf("expected With to return noop logger")
	}
	if _, ok := logger.LogLevel(DebugLevel).(noopLogger); !ok {
		t.Fatalf("expected LogLevel to return noop logger")
	}

	// Trace/Debug/etc should be no-ops.
	logger.Debug("debug", "foo", "bar")
	logger.Debugf("debug %d", 1)
	logger.Info("info")
	logger.Infof("info %s", "fmt")
	logger.Warn("warn")
	logger.Warnf("warn %s", "fmt")
	logger.Error("error")
	logger.Errorf("error %s", "fmt")
	logger.Trace("trace")
	logger.Tracef("trace %s", "fmt")

	// Enabled should always be false and Handle should return nil.
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatalf("expected noop logger to be disabled for all levels")
	}
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	if err := logger.Handle(context.Background(), record); err != nil {
		t.Fatalf("expected Handle to return nil, got %v", err)
	}

	if handler := logger.WithAttrs(nil); handler == nil {
		t.Fatalf("expected WithAttrs to return handler")
	} else if _, ok := handler.(noopLogger); !ok {
		t.Fatalf("expected WithAttrs to return noop logger, got %T", handler)
	}
	if handler := logger.WithGroup("group"); handler == nil {
		t.Fatalf("expected WithGroup to return handler")
	} else if _, ok := handler.(noopLogger); !ok {
		t.Fatalf("expected WithGroup to return noop logger, got %T", handler)
	}
}

func TestWriteToLoggerDetectsLevels(t *testing.T) {
	rec := &recordingLogger{}
	lines := []struct {
		input string
		level Level
		msg   string
	}{
		{"INFO: ready", InfoLevel, "ready"},
		{"warn - slow path", WarnLevel, "slow path"},
		{"ERROR critical failure", ErrorLevel, "critical failure"},
		{"[DEBUG] details", DebugLevel, "details"},
		{"http: TLS handshake error from 1.2.3.4: remote error: tls: bad certificate", ErrorLevel, "from 1.2.3.4: remote error: tls: bad certificate"},
		{"request failed with error code 42", ErrorLevel, "code 42"},
		{"plain message", NoLevel, "plain message"},
	}

	for _, tc := range lines {
		payload := tc.input + "\n"
		if n, err := rec.Write([]byte(payload)); err != nil {
			t.Fatalf("Write(%q) returned error %v", tc.input, err)
		} else if n != len(payload) {
			t.Fatalf("Write(%q) returned %d, want %d", tc.input, n, len(payload))
		}
	}

	if len(rec.entries) != len(lines) {
		t.Fatalf("expected %d entries, got %d", len(lines), len(rec.entries))
	}

	for i, tc := range lines {
		entry := rec.entries[i]
		if entry.level != tc.level {
			t.Fatalf("entry %d level = %v, want %v", i, entry.level, tc.level)
		}
		if entry.msg != tc.msg {
			t.Fatalf("entry %d msg = %q, want %q", i, entry.msg, tc.msg)
		}
	}
}

func TestLogLoggerUsesForLoggingWriter(t *testing.T) {
	rec := &recordingLogger{}
	std := LogLogger(rec)

	std.Println("ERROR: boom")

	if len(rec.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(rec.entries))
	}
	if rec.entries[0].level != ErrorLevel {
		t.Fatalf("expected ErrorLevel, got %v", rec.entries[0].level)
	}
	if rec.entries[0].msg != "boom" {
		t.Fatalf("expected message %q, got %q", "boom", rec.entries[0].msg)
	}
}

func TestLogLoggerWithLevelPinsSeverity(t *testing.T) {
	t.Helper()

	rec := &recordingLogger{}
	std := LogLoggerWithLevel(rec, ErrorLevel)

	std.Println("INFO something noisy")
	std.Println("[WARN] be careful")

	if len(rec.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(rec.entries))
	}
	for i, entry := range rec.entries {
		if entry.level != ErrorLevel {
			t.Fatalf("entry %d level = %v, want %v", i, entry.level, ErrorLevel)
		}
	}
	if rec.entries[0].msg != "something noisy" {
		t.Fatalf("first message = %q, want %q", rec.entries[0].msg, "something noisy")
	}
	if rec.entries[1].msg != "be careful" {
		t.Fatalf("second message = %q, want %q", rec.entries[1].msg, "be careful")
	}
}

type logEntry struct {
	level Level
	msg   string
}

type recordingLogger struct {
	noopLogger
	entries []logEntry
}

func (r *recordingLogger) Logp(level Level, msg string, _ ...any) {
	r.entries = append(r.entries, logEntry{level: level, msg: msg})
}

func (r *recordingLogger) Write(p []byte) (int, error) {
	return WriteToLogger(r, p)
}
