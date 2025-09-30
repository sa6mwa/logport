package phuslu

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	plog "github.com/phuslu/log"
	port "github.com/sa6mwa/logport"
)

func TestNewLogsMessageWithFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	logger.Info("hello", "foo", "bar")

	record := decodeLogLine(t, buf.Bytes())
	if record["message"] != "hello" {
		t.Fatalf("expected message 'hello', got %v", record["message"])
	}
	if record["foo"] != "bar" {
		t.Fatalf("expected foo=bar, got %v", record["foo"])
	}
	if record["level"] != "info" {
		t.Fatalf("expected level 'info', got %v", record["level"])
	}
}

func TestWithAddsPersistentFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf).With("request_id", 123)

	logger.Info("processing")

	record := decodeLogLine(t, buf.Bytes())
	if record["request_id"] != float64(123) {
		t.Fatalf("expected request_id=123, got %v", record["request_id"])
	}
}

func TestContextWithLoggerInjectsAdapter(t *testing.T) {
	buf := &bytes.Buffer{}
	ctx := ContextWithLogger(context.Background(), buf, Options{})

	logger := port.LoggerFromContext(ctx)
	logger.Info("from context")

	record := decodeLogLine(t, buf.Bytes())
	if record["message"] != "from context" {
		t.Fatalf("expected message 'from context', got %v", record["message"])
	}
}

func TestAdapterSupportsSlogHandler(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := New(buf)

	logger := slog.New(handler)
	logger = logger.With("status", 200).WithGroup("http")
	logger.Info("ok", slog.String("method", "GET"))

	record := decodeLogLine(t, buf.Bytes())
	if record["message"] != "ok" {
		t.Fatalf("expected message 'ok', got %v", record["message"])
	}
	if record["status"] != float64(200) {
		t.Fatalf("expected status=200, got %v", record["status"])
	}
	if record["http.method"] != "GET" {
		t.Fatalf("expected http.method='GET', got %v", record["http.method"])
	}
}

func TestEnabledRespectsLoggerLevel(t *testing.T) {
	handler := NewWithOptions(io.Discard, Options{
		Configure: func(l *plog.Logger) {
			l.Level = plog.WarnLevel
		},
	})

	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatalf("expected info logging to be disabled")
	}
	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Fatalf("expected error logging to be enabled")
	}
}

func TestNoLevelOmitsLevelField(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf).LogLevel(port.NoLevel)
	logger.Info("no level", "foo", "bar")

	record := decodeLogLine(t, buf.Bytes())
	if record["message"] != "no level" {
		t.Fatalf("expected message 'no level', got %v", record["message"])
	}
	if _, ok := record["level"]; ok {
		t.Fatalf("expected no level field, got %v", record["level"])
	}
	if record["foo"] != "bar" {
		t.Fatalf("expected foo=bar, got %v", record["foo"])
	}

	buf.Reset()
	logger.With("tenant", "acme").Warn("still no level")
	record = decodeLogLine(t, buf.Bytes())
	if _, ok := record["level"]; ok {
		t.Fatalf("expected no level field after With, got %v", record["level"])
	}
	if record["tenant"] != "acme" {
		t.Fatalf("expected tenant field to persist, got %v", record["tenant"])
	}
}

func decodeLogLine(t *testing.T, data []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	var record map[string]any
	if err := dec.Decode(&record); err != nil {
		t.Fatalf("failed to decode log output: %v", err)
	}
	return record
}
