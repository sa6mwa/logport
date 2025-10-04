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
