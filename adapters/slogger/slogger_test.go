package slogger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"strings"
	"testing"
	"time"

	port "pkt.systems/logport"
	"pkt.systems/logport/adapters/slogger"
)

func TestSloggerTextInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := slogger.New(&buf)
	logger.Info("hello", "foo", "bar")

	out := buf.String()
	if out == "" {
		t.Fatalf("expected output, got empty string")
	}
	if !strings.Contains(out, "level=INFO") {
		t.Fatalf("expected level marker in %q", out)
	}
	if !strings.Contains(out, "msg=hello") {
		t.Fatalf("expected msg field in %q", out)
	}
	if !strings.Contains(out, "foo=bar") {
		t.Fatalf("expected key/value in %q", out)
	}
}

func TestSloggerJSONOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := slogger.NewJSON(&buf).With("user", "alice")
	logger.Warn("something happened", "answer", 42)

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatalf("expected JSON output")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if payload["msg"] != "something happened" {
		t.Fatalf("expected msg field, got %v", payload["msg"])
	}
	if payload["user"] != "alice" {
		t.Fatalf("expected user field, got %v", payload["user"])
	}
	if v := payload["answer"]; v == nil || int(v.(float64)) != 42 {
		t.Fatalf("expected answer=42, got %v", payload["answer"])
	}
}

func TestSloggerRespectsMinLevel(t *testing.T) {
	min := port.WarnLevel
	var buf bytes.Buffer
	logger := slogger.NewWithOptions(&buf, slogger.Options{MinLevel: &min})

	logger.Info("suppressed")
	if buf.Len() != 0 {
		t.Fatalf("expected info to be suppressed, got %q", buf.String())
	}
	logger.Error("visible")
	if !strings.Contains(buf.String(), "visible") {
		t.Fatalf("expected error to be logged, got %q", buf.String())
	}
}

func TestSloggerHandlesRecords(t *testing.T) {
	var buf bytes.Buffer
	logger := slogger.New(&buf)
	handler := logger.(slog.Handler)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "handler", 0)
	record.Add("foo", "bar")
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("handle returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "handler") || !strings.Contains(out, "foo=bar") {
		t.Fatalf("unexpected handler output %q", out)
	}
}

func TestSloggerWriteIntegration(t *testing.T) {
	var buf bytes.Buffer
	std := log.New(slogger.New(&buf), "", 0)

	std.Println("[INFO] bridge works")
	out := buf.String()
	if !strings.Contains(out, "bridge works") {
		t.Fatalf("expected message, got %q", out)
	}
}
