package psl_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	psl "pkt.systems/logport/adapters/psl"
)

var jsonEscapingSeeds = []struct {
	name  string
	msg   string
	key   string
	value string
}{
	{"plain", "hello", "key", "value"},
	{"quotes", `"quoted" message`, `quote"key`, `value"with"quotes`},
	{"braces", "ends } braces", "brace}", `{"evil":1}`},
	{"controls", "line\nfeed\tand\\slash", "new\nline", "tab\tvalue"},
	{"unicode", "emoji 😃", "control" + string(rune(0)), "snowman ☃"},
}

func TestStructuredJSONEscaping(t *testing.T) {
	for _, tc := range jsonEscapingSeeds {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := psl.NewWithOptions(&buf, psl.Options{Mode: psl.ModeStructured, DisableTimestamp: true, NoColor: true})
			logger = logger.With("seed", tc.name)
			logger.Info(tc.msg, tc.key, tc.value)

			line := strings.TrimSpace(buf.String())
			if line == "" {
				t.Fatalf("empty structured output")
			}
			if !strings.HasPrefix(line, "{") {
				t.Fatalf("expected json object, got %q", line)
			}

			var payload map[string]any
			if err := json.Unmarshal([]byte(line), &payload); err != nil {
				t.Fatalf("invalid json output: %v for line %q", err, line)
			}
			if payload["msg"] != tc.msg {
				t.Fatalf("message mismatch: got %q want %q", payload["msg"], tc.msg)
			}
		})
	}
}

func FuzzStructuredJSONEscaping(f *testing.F) {
	for _, tc := range jsonEscapingSeeds {
		f.Add(tc.msg, tc.key, tc.value)
	}

	f.Fuzz(func(t *testing.T, msg, key, value string) {
		var buf bytes.Buffer
		logger := psl.NewWithOptions(&buf, psl.Options{Mode: psl.ModeStructured, DisableTimestamp: true, NoColor: true})
		logger = logger.With("origin", "fuzz")

		logger.Info(msg, key, value, key+"_dup", msg)

		line := strings.TrimSpace(buf.String())
		if line == "" || line[0] != '{' {
			t.Fatalf("expected json object, got %q", line)
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("invalid json output: %v for line %q", err, line)
		}
	})
}
