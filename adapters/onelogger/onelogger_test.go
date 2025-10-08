package onelogger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	onelogpkg "github.com/francoispqt/onelog"
	port "pkt.systems/logport"
)

func TestInfoProducesStructuredOutput(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)
	logger.Info("hello", "foo", "bar", "answer", 42)

	record := decodeJSON(t, buf.Bytes())
	if record["message"] != "hello" {
		t.Fatalf("expected message 'hello', got %v", record["message"])
	}
	if record["foo"] != "bar" {
		t.Fatalf("expected foo=bar, got %v", record["foo"])
	}
	if got := record["answer"]; got != float64(42) {
		t.Fatalf("expected answer=42, got %v", got)
	}
	ts, ok := record["ts"].(string)
	if !ok || ts == "" {
		t.Fatalf("expected timestamp field, got %v", record["ts"])
	}
	if _, err := time.Parse(port.DTGTimeFormat, ts); err != nil {
		t.Fatalf("expected timestamp to follow %q, got %q: %v", port.DTGTimeFormat, ts, err)
	}
}

func TestWithBaseFieldsAndGroups(t *testing.T) {
	buf := &bytes.Buffer{}
	base := New(buf).With("app", "logport")
	handler, ok := any(base).(slog.Handler)
	if !ok {
		t.Fatalf("expected adapter to satisfy slog.Handler")
	}
	grouped := handler.WithGroup("request")
	slogger := slog.New(grouped)
	slogger.Info("grouped", slog.String("id", "abc123"))

	record := decodeJSON(t, buf.Bytes())
	if record["app"] != "logport" {
		t.Fatalf("expected base field, got %v", record["app"])
	}
	if record["request.id"] != "abc123" {
		t.Fatalf("expected grouped field request.id=abc123, got %v", record["request.id"])
	}
}

func TestLogLevelFiltersMessages(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf).LogLevel(port.WarnLevel)

	logger.Info("skip me")
	if buf.Len() != 0 {
		t.Fatalf("expected info log to be skipped when min level is warn, output: %s", buf.String())
	}

	logger.Warn("warned", "active", true)
	record := decodeJSON(t, buf.Bytes())
	if record["message"] != "warned" {
		t.Fatalf("expected warn message, got %v", record["message"])
	}
	if record["active"] != true {
		t.Fatalf("expected active=true, got %v", record["active"])
	}
}

func TestSlogHandlerIntegration(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := New(buf)
	slogger := slog.New(handler)
	slogger = slogger.With("base", "value").WithGroup("http")
	slogger.Info("request", slog.Int("status", 201))

	record := decodeJSON(t, buf.Bytes())
	if record["message"] != "request" {
		t.Fatalf("expected message 'request', got %v", record["message"])
	}
	if record["base"] != "value" {
		t.Fatalf("expected base=value, got %v", record["base"])
	}
	if record["http.status"] != float64(201) {
		t.Fatalf("expected http.status=201, got %v", record["http.status"])
	}
}

func TestOptionsMinLevelAndHook(t *testing.T) {
	buf := &bytes.Buffer{}
	min := port.ErrorLevel
	logger := NewWithOptions(buf, Options{
		MinLevel: &min,
		Hook: func(e onelogpkg.Entry) {
			e.String("hook", "invoked")
		},
	})

	logger.Warn("ignored")
	if buf.Len() != 0 {
		t.Fatalf("expected warn below MinLevel to be skipped, got %s", buf.String())
	}

	logger.Error("error", "id", 99)
	record := decodeJSON(t, buf.Bytes())
	if record["hook"] != "invoked" {
		t.Fatalf("expected hook to add field, got %v", record["hook"])
	}
	if record["id"] != float64(99) {
		t.Fatalf("expected id=99, got %v", record["id"])
	}
}

func TestDisableTimestampOption(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewWithOptions(buf, Options{DisableTimestamp: true})
	logger.Info("no ts")
	record := decodeJSON(t, buf.Bytes())
	if _, ok := record["ts"]; ok {
		t.Fatalf("expected timestamp to be omitted, got %v", record["ts"])
	}
}

func TestEmptyTimeFormatDisablesTimestamp(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewWithOptions(buf, Options{TimeFormat: ""})
	logger.Info("no ts")
	record := decodeJSON(t, buf.Bytes())
	if _, ok := record["ts"]; ok {
		t.Fatalf("expected timestamp to be omitted when TimeFormat is empty, got %v", record["ts"])
	}
}

func TestCustomTimeFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	format := time.RFC3339
	logger := NewWithOptions(buf, Options{TimeFormat: format})
	logger.Info("custom")
	record := decodeJSON(t, buf.Bytes())
	ts, ok := record["ts"].(string)
	if !ok {
		t.Fatalf("expected timestamp field, got %v", record["ts"])
	}
	if _, err := time.Parse(format, ts); err != nil {
		t.Fatalf("expected timestamp to follow %q, got %q: %v", format, ts, err)
	}
}

func TestNewFromLoggerAndContext(t *testing.T) {
	buf := &bytes.Buffer{}
	underlying := onelogpkg.New(buf, onelogpkg.ALL)
	logger := NewFromLogger(underlying)

	ctx := ContextWithLogger(context.Background(), buf, Options{})
	port.LoggerFromContext(ctx).Info("from ctx")

	logger.Info("direct")

	records := decodeLines(t, buf.Bytes())
	if len(records) != 2 {
		t.Fatalf("expected two log lines, got %d", len(records))
	}
}

func TestFatalUsesExitFunc(t *testing.T) {
	buf := &bytes.Buffer{}
	var exitCode int
	logger := NewWithOptions(buf, Options{
		ExitFunc: func(code int) { exitCode = code },
	})

	logger.Fatal("fatal", "foo", "bar")

	if exitCode != 1 {
		t.Fatalf("expected ExitFunc to receive code 1, got %d", exitCode)
	}
	record := decodeJSON(t, buf.Bytes())
	if record["message"] != "fatal" {
		t.Fatalf("expected fatal message, got %v", record["message"])
	}
	if record["foo"] != "bar" {
		t.Fatalf("expected foo=bar, got %v", record["foo"])
	}
}

func TestPanicLogsBeforePanicking(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(buf)

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic to occur")
		}
		record := decodeJSON(t, buf.Bytes())
		if record["message"] != "boom" {
			t.Fatalf("expected panic message logged, got %v", record["message"])
		}
		if record["panic"] != true {
			t.Fatalf("expected panic flag true, got %v", record["panic"])
		}
	}()

	logger.Panic("boom", "panic", true)
}

func decodeJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	lines := decodeLines(t, data)
	if len(lines) == 0 {
		t.Fatalf("expected at least one log line")
	}
	return lines[len(lines)-1]
}

func decodeLines(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	rawLines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	records := make([]map[string]any, 0, len(rawLines))
	for _, line := range rawLines {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("failed to decode JSON %q: %v", string(line), err)
		}
		records = append(records, record)
	}
	return records
}
