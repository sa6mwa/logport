package zerologger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	port "github.com/sa6mwa/logport"
)

type customStringer struct{ value string }

func (c customStringer) String() string { return c.value }

func TestFieldsFromKeyvals(t *testing.T) {
	keyvals := []any{"foo", "bar", 99, "answer"}

	fields := fieldsFromKeyvals(keyvals)

	if fields["foo"] != "bar" {
		t.Fatalf("expected foo=bar, got %v", fields["foo"])
	}
	if fields["99"] != "answer" {
		t.Fatalf("expected numeric key to be stringified, got %v", fields["99"])
	}
	if len(fields) != 2 {
		t.Fatalf("expected two fields, got %d", len(fields))
	}
}

func TestFieldsFromKeyvalsOddCount(t *testing.T) {
	keyvals := []any{"foo", "bar", "trailing"}

	fields := fieldsFromKeyvals(keyvals)

	if fields["foo"] != "bar" {
		t.Fatalf("expected foo=bar, got %v", fields["foo"])
	}
	if fields["arg1"] != "trailing" {
		t.Fatalf("expected trailing value under arg1, got %v", fields["arg1"])
	}
}

func TestAddFieldsCoversSupportedTypes(t *testing.T) {
	now := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)
	errVal := errors.New("boom")
	data := []byte("abc")
	stringerVal := customStringer{value: "stringer"}

	buf := &bytes.Buffer{}
	logger := zerolog.New(buf)

	event := logger.Info()
	addFields(event, []any{
		"string", "value",
		"err", errVal,
		"stringer", stringerVal,
		"bool", true,
		"int", int(-1),
		"int8", int8(-8),
		"int16", int16(-16),
		"int32", int32(-32),
		"int64", int64(-64),
		"uint", uint(1),
		"uint8", uint8(8),
		"uint16", uint16(16),
		"uint32", uint32(32),
		"uint64", uint64(64),
		"float32", float32(1.5),
		"float64", float64(2.5),
		"time", now,
		"duration", time.Second,
		"bytes", data,
	})
	event.Msg("done")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode log output: %v", err)
	}

	if record["string"] != "value" {
		t.Fatalf("expected string field, got %v", record["string"])
	}
	if record["err"] != errVal.Error() {
		t.Fatalf("expected err field, got %v", record["err"])
	}
	if record["stringer"] != stringerVal.value {
		t.Fatalf("expected stringer field, got %v", record["stringer"])
	}
	if record["bool"] != true {
		t.Fatalf("expected bool field true, got %v", record["bool"])
	}

	numericFields := map[string]float64{
		"int":     -1,
		"int8":    -8,
		"int16":   -16,
		"int32":   -32,
		"int64":   -64,
		"uint":    1,
		"uint8":   8,
		"uint16":  16,
		"uint32":  32,
		"uint64":  64,
		"float32": 1.5,
		"float64": 2.5,
	}
	for key, expected := range numericFields {
		value, ok := record[key]
		if !ok {
			t.Fatalf("missing numeric field %s", key)
		}
		number, ok := value.(float64)
		if !ok {
			t.Fatalf("expected %s to decode as float64, got %T", key, value)
		}
		if number != expected {
			t.Fatalf("expected %s=%v, got %v", key, expected, number)
		}
	}

	timeField, ok := record["time"].(string)
	if !ok || timeField == "" {
		t.Fatalf("expected time field as string, got %v", record["time"])
	}

	if record["duration"] == nil {
		t.Fatalf("expected duration field to be present")
	}

	if record["bytes"] != string(data) {
		t.Fatalf("expected bytes field %q, got %v", data, record["bytes"])
	}

	if record["message"] != "done" {
		t.Fatalf("expected message 'done', got %v", record["message"])
	}
}

func TestAdapterWithAddsFieldsToEvents(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewFromLogger(zerolog.New(buf))

	logger.With("foo", "bar").Info("hello")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode log output: %v", err)
	}

	if record["foo"] != "bar" {
		t.Fatalf("expected foo=bar, got %v", record["foo"])
	}
	if record["message"] != "hello" {
		t.Fatalf("expected message 'hello', got %v", record["message"])
	}
}

func TestContextWithLoggerInjectsAdapter(t *testing.T) {
	buf := &bytes.Buffer{}
	ctx := ContextWithLogger(context.Background(), buf, Options{NoColor: true, DisableTimestamp: true})

	logger := port.LoggerFromContext(ctx)
	logger.Info("ctx message", "foo", "bar")

	got := buf.String()
	if got == "" {
		t.Fatalf("expected log output from context logger")
	}
	if !strings.Contains(got, "ctx message") {
		t.Fatalf("expected message to appear, got %q", got)
	}
	if !strings.Contains(got, "foo=bar") {
		t.Fatalf("expected structured fields to appear, got %q", got)
	}
}

func TestZerologAdapterSupportsSlogHandler(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewFromLogger(zerolog.New(buf))
	logger := slog.New(handler)
	logger = logger.With("trace_id", "abc123").WithGroup("outer")
	logger.Warn("warned", slog.Int("code", 7))

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode slog output: %v", err)
	}

	if record["message"] != "warned" {
		t.Fatalf("expected message 'warned', got %v", record["message"])
	}
	if record["trace_id"] != "abc123" {
		t.Fatalf("expected trace_id to be preserved, got %v", record["trace_id"])
	}
	outerCode, ok := record["outer.code"].(float64)
	if !ok || outerCode != 7 {
		t.Fatalf("expected grouped attr outer.code=7, got %v", record["outer.code"])
	}
}
