package psl_test

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
	"time"

	port "pkt.systems/logport"
	"pkt.systems/logport/adapters/psl"
)

func TestConsoleOutputMatchesFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := psl.NewWithOptions(&buf, psl.Options{Mode: psl.ModeConsole, DisableTimestamp: true, NoColor: true})
	logger.Info("ready", "foo", "bar", "greeting", "hello world")

	got := strings.TrimSpace(buf.String())
	expected := "INF ready foo=bar greeting=\"hello world\""
	if got != expected {
		t.Fatalf("unexpected output: got %q want %q", got, expected)
	}
}

func TestStructuredOutputJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := psl.NewWithOptions(&buf, psl.Options{Mode: psl.ModeStructured, DisableTimestamp: true})
	logger.Warn("boom", "count", 3)

	line := strings.TrimSpace(buf.String())
	if strings.Contains(line, "\x1b") {
		t.Fatalf("unexpected color codes in JSON: %q", line)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if payload["msg"] != "boom" {
		t.Fatalf("expected msg boom, got %v", payload["msg"])
	}
	lvl, ok := payload["lvl"]
	if !ok {
		t.Fatalf("expected lvl field, payload=%v", payload)
	}
	if lvl != "warn" {
		t.Fatalf("expected lvl warn, got %v", lvl)
	}
	if payload["count"] != float64(3) {
		t.Fatalf("expected count 3, got %v", payload["count"])
	}
}

func TestStructuredVerboseFields(t *testing.T) {
	var buf bytes.Buffer
	logger := psl.NewWithOptions(&buf, psl.Options{Mode: psl.ModeStructured, DisableTimestamp: true, VerboseFields: true})
	logger.Info("hello")

	line := strings.TrimSpace(buf.String())
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if payload["message"] != "hello" {
		t.Fatalf("expected message hello, got %v", payload["message"])
	}
	if payload["level"] != "info" {
		t.Fatalf("expected level info, got %v", payload["level"])
	}
	if _, ok := payload["msg"]; ok {
		t.Fatalf("unexpected short field present, payload=%v", payload)
	}
	if _, ok := payload["ts"]; ok {
		t.Fatalf("unexpected ts field when timestamps disabled, payload=%v", payload)
	}
}

func TestColorJSONDisabledOnNonTerminal(t *testing.T) {
	var buf bytes.Buffer
	logger := psl.NewWithOptions(&buf, psl.Options{Mode: psl.ModeStructured, DisableTimestamp: true, ColorJSON: true})
	logger.Info("msg")
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("expected no colors on non-terminal writer, got %q", buf.String())
	}
}

func TestWithAndMinimalSubset(t *testing.T) {
	var buf bytes.Buffer
	logger := psl.NewWithOptions(&buf, psl.Options{Mode: psl.ModeConsole, DisableTimestamp: true, NoColor: true}).With("app", "demo")
	logger.(port.ForLoggingMinimalSubset).Info("up")
	got := strings.TrimSpace(buf.String())
	if !strings.Contains(got, "app=demo") {
		t.Fatalf("expected base field in output, got %q", got)
	}
}

func TestLogLoggerBridgePSL(t *testing.T) {
	var buf bytes.Buffer
	std := log.New(psl.NewWithOptions(&buf, psl.Options{Mode: psl.ModeConsole, DisableTimestamp: true, NoColor: true}), "", 0)
	std.Println("[INFO] bridge")
	if !strings.Contains(buf.String(), "bridge") {
		t.Fatalf("bridge output missing message: %q", buf.String())
	}
}

func TestConsoleUTCOption(t *testing.T) {
	var buf bytes.Buffer
	logger := psl.NewWithOptions(&buf, psl.Options{
		Mode:       psl.ModeConsole,
		TimeFormat: time.RFC3339,
		NoColor:    true,
		UTC:        true,
	})
	logger.Info("utc-test")

	line := strings.TrimSpace(buf.String())
	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 0 {
		t.Fatalf("expected timestamp in output, got %q", line)
	}
	ts := parts[0]
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t.Fatalf("failed to parse timestamp %q: %v", ts, err)
	}
	if parsed.Location().String() != "UTC" {
		t.Fatalf("expected UTC timestamp, got %q (location=%s)", ts, parsed.Location())
	}
}
