package psl

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	logport "pkt.systems/logport"
)

func TestInfoAddsFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewWithOptions(buf, Options{Mode: ModeStructured, DisableTimestamp: true, NoColor: true})

	logger.Info("builder",
		"component", "worker",
		"attempt", int64(2),
		"cached", true,
	)

	record := decodeLastJSONLine(t, buf.Bytes())
	if got := record["lvl"]; got != "info" {
		t.Fatalf("expected lvl=info, got %v", got)
	}
	if got := record["msg"]; got != "builder" {
		t.Fatalf("expected msg=builder, got %v", got)
	}
	if got := record["component"]; got != "worker" {
		t.Fatalf("expected component=worker, got %v", got)
	}
	if got := record["attempt"]; got != float64(2) {
		t.Fatalf("expected attempt=2, got %v", got)
	}
	if got := record["cached"]; got != true {
		t.Fatalf("expected cached=true, got %v", got)
	}
}

func TestWithLogLevelIncludesField(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewWithOptions(buf, Options{Mode: ModeStructured, DisableTimestamp: true, NoColor: true}).
		LogLevel(logport.WarnLevel).
		WithLogLevel()

	logger.Warn("preflight")

	record := decodeLastJSONLine(t, buf.Bytes())
	if got := record["loglevel"]; got != "warn" {
		t.Fatalf("expected loglevel=warn, got %v", got)
	}
}

func TestLogLevelFilters(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewWithOptions(buf, Options{Mode: ModeStructured, DisableTimestamp: true, NoColor: true}).LogLevel(logport.ErrorLevel)

	logger.Info("ignored")
	if buf.Len() != 0 {
		t.Fatalf("expected info to be filtered, got output %q", buf.String())
	}

	logger.Error("emitted", "code", 500)

	record := decodeLastJSONLine(t, buf.Bytes())
	if got := record["msg"]; got != "emitted" {
		t.Fatalf("expected msg=emitted, got %v", got)
	}
	if got := record["code"]; got != float64(500) {
		t.Fatalf("expected code=500, got %v", got)
	}
}

func TestSlogWithGroup(t *testing.T) {
	buf := &bytes.Buffer{}
	raw := NewWithOptions(buf, Options{Mode: ModeStructured, DisableTimestamp: true, NoColor: true})

	handler, ok := any(raw).(slog.Handler)
	if !ok {
		t.Fatalf("expected logger to implement slog.Handler")
	}

	logger := handler.WithGroup("request").WithAttrs([]slog.Attr{
		slog.String("tenant", "enterprise"),
	})

	record := slog.NewRecord(time.Unix(0, 0), slog.LevelInfo, "handled", 0)
	record.Add("id", "abc123")

	if err := logger.Handle(context.Background(), record); err != nil {
		t.Fatalf("handle failed: %v", err)
	}

	decoded := decodeLastJSONLine(t, buf.Bytes())
	if got := decoded["request.id"]; got != "abc123" {
		t.Fatalf("expected request.id=abc123, got %v", got)
	}
	if got := decoded["request.tenant"]; got != "enterprise" {
		t.Fatalf("expected request.tenant=enterprise, got %v", got)
	}
}

func TestWithTraceAddsTraceIDs(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewWithOptions(buf, Options{Mode: ModeStructured, DisableTimestamp: true, NoColor: true})

	var tid trace.TraceID
	copy(tid[:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	var sid trace.SpanID
	copy(sid[:], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	logger.WithTrace(ctx).Info("traced")

	record := decodeLastJSONLine(t, buf.Bytes())
	if got := record["trace_id"]; got != tid.String() {
		t.Fatalf("expected trace_id %q, got %v", tid.String(), got)
	}
	if got := record["span_id"]; got != sid.String() {
		t.Fatalf("expected span_id %q, got %v", sid.String(), got)
	}
}

// --- helpers ---

func decodeLastJSONLine(t *testing.T, data []byte) map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) == 0 || len(lines[len(lines)-1]) == 0 {
		t.Fatalf("expected at least one JSON line in %q", data)
	}
	var record map[string]any
	if err := json.Unmarshal(lines[len(lines)-1], &record); err != nil {
		t.Fatalf("failed decoding json: %v", err)
	}
	return record
}
