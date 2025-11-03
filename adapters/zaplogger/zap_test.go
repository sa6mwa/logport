package zaplogger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	logport "pkt.systems/logport"
)

func testOptions() Options {
	cfg := zap.NewDevelopmentEncoderConfig()
	cfg.TimeKey = ""
	cfg.LevelKey = "level"
	cfg.NameKey = ""
	cfg.CallerKey = ""
	cfg.MessageKey = "msg"
	cfg.StacktraceKey = ""
	return Options{
		EncoderConfig: &cfg,
		Level:         zapcore.DebugLevel,
	}
}

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
	if !strings.Contains(got, "\"foo\":\"bar\"") {
		t.Fatalf("expected key/value to be present, got %q", got)
	}
}

func TestInfoSupportsSlogAttrArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewWithOptions(buf, testOptions())
	logger.Info("attr", slog.String("subject", "world"))

	got := buf.String()
	if !strings.Contains(got, "\"subject\":\"world\"") {
		t.Fatalf("expected slog attr field, got %q", got)
	}
}

func TestInfofFormatsMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewWithOptions(buf, testOptions())
	logger.Infof("hello %s %d", "world", 7)

	got := buf.String()
	if !strings.Contains(got, "\"msg\":\"hello world 7\"") {
		t.Fatalf("expected formatted message, got %q", got)
	}
}

func TestWithAddsPersistentFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewWithOptions(buf, testOptions())
	logger = logger.With("request_id", 123)
	logger.Info("processing")

	got := buf.String()
	if !strings.Contains(got, "processing") {
		t.Fatalf("expected log output to contain %q, got %q", "processing", got)
	}
	if !strings.Contains(got, "\"request_id\":123") {
		t.Fatalf("expected persistent field to be present, got %q", got)
	}
}

func TestContextWithLoggerInjectsAdapter(t *testing.T) {
	buf := &bytes.Buffer{}
	ctx := ContextWithLogger(context.Background(), buf, testOptions())
	logger := logport.LoggerFromContext(ctx)
	logger.Info("from context")

	got := buf.String()
	if got == "" {
		t.Fatalf("expected output when logging from context")
	}
	if !strings.Contains(got, "from context") {
		t.Fatalf("expected contextual log to contain message, got %q", got)
	}
}

func TestZapAdapterSupportsSlogHandler(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewWithOptions(buf, testOptions())
	slogLogger := slog.New(handler.(slog.Handler))
	slogLogger = slogLogger.WithGroup("request").With("id", 42)
	slogLogger.Info("handled", slog.String("status", "ok"))

	got := buf.String()
	if got == "" {
		t.Fatalf("expected slog handler to emit output")
	}
	if !strings.Contains(got, "handled") {
		t.Fatalf("expected message to appear, got %q", got)
	}
	if !strings.Contains(got, "\"request.id\":42") {
		t.Fatalf("expected grouped attribute, got %q", got)
	}
	if !strings.Contains(got, "\"request.status\":\"ok\"") {
		t.Fatalf("expected grouped attr from slog, got %q", got)
	}
}
