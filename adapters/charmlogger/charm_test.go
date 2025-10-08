package charmlogger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	port "pkt.systems/logport"
)

func TestNewLogsMessageWithFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	logger.Info("hello world", "foo", "bar")

	got := buf.String()
	if got == "" {
		t.Fatalf("expected log output, got empty string")
	}
	if !strings.Contains(got, "hello world") {
		t.Fatalf("expected message to contain %q, got %q", "hello world", got)
	}
	if !strings.Contains(got, "foo=bar") {
		t.Fatalf("expected key/value to be present, got %q", got)
	}
}

func TestInfoSupportsSlogAttrArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	logger.Info("attr", slog.String("subject", "world"))

	got := buf.String()
	if !strings.Contains(got, "subject=world") {
		t.Fatalf("expected slog attr field, got %q", got)
	}
}

func TestInfofFormatsMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	logger.Infof("hello %s %d", "world", 7)

	got := buf.String()
	if !strings.Contains(got, "hello world 7") {
		t.Fatalf("expected formatted message, got %q", got)
	}
}

func TestWithAddsPersistentFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	logger = logger.With("request_id", 123)
	logger.Info("processing")

	got := buf.String()
	if !strings.Contains(got, "processing") {
		t.Fatalf("expected log output to contain %q, got %q", "processing", got)
	}
	if !strings.Contains(got, "request_id=123") {
		t.Fatalf("expected persistent field to be present, got %q", got)
	}
}

func TestContextWithLoggerInjectsAdapter(t *testing.T) {
	buf := &bytes.Buffer{}
	ctx := ContextWithLogger(context.Background(), buf, log.Options{ReportTimestamp: false})

	logger := port.LoggerFromContext(ctx)
	logger.Info("from context")

	got := buf.String()
	if got == "" {
		t.Fatalf("expected output when logging from context")
	}
	if !strings.Contains(got, "from context") {
		t.Fatalf("expected contextual log to contain message, got %q", got)
	}
}

func TestCharmAdapterSupportsSlogHandler(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := New(buf)
	logger := slog.New(handler)
	logger = logger.WithGroup("request").With("id", 42)
	logger.Info("handled", slog.String("status", "ok"))

	got := buf.String()
	if got == "" {
		t.Fatalf("expected slog handler to emit output")
	}
	if !strings.Contains(got, "handled") {
		t.Fatalf("expected message to appear, got %q", got)
	}
	if !strings.Contains(got, "request.id=42") {
		t.Fatalf("expected grouped attribute, got %q", got)
	}
	if !strings.Contains(got, "request.status=ok") {
		t.Fatalf("expected grouped attr from slog, got %q", got)
	}
}

func TestCharmNoLevelRemovesSeverity(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewWithOptions(buf, log.Options{ReportTimestamp: false})
	noLevel := logger.LogLevel(port.NoLevel)

	noLevel.Info("no level", "foo", "bar")

	out := buf.String()
	if out == "" {
		t.Fatalf("expected output, got empty string")
	}
	if !strings.Contains(out, "no level") {
		t.Fatalf("expected message in output, got %q", out)
	}
	if !strings.Contains(out, "foo=bar") {
		t.Fatalf("expected fields in output, got %q", out)
	}
	for _, lvl := range []string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"} {
		if strings.Contains(out, lvl) {
			t.Fatalf("expected no explicit level, found %s in %q", lvl, out)
		}
	}

	buf.Reset()
	noLevel.With("request_id", 7).Warn("still no level")
	if strings.Contains(buf.String(), "WARN") {
		t.Fatalf("expected no level in chained logger, got %q", buf.String())
	}
}
