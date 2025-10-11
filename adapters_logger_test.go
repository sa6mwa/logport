package logport_test

import (
	"bytes"
	"strings"
	"testing"

	logport "pkt.systems/logport"
)

func TestAdaptersLogLoggerClassification(t *testing.T) {
	cases := []struct {
		name          string
		input         string
		expect        string
		requireOutput bool
	}{
		{name: "info_prefix", input: "[INFO] service ready", expect: "service ready", requireOutput: true},
		{name: "no_match", input: "plain telemetry line", expect: "", requireOutput: false},
		{name: "error_substring", input: "operation failed with error code", expect: "code", requireOutput: true},
	}

	for _, factory := range adapterFactories() {
		factory := factory
		t.Run(factory.name, func(t *testing.T) {
			for _, tc := range cases {
				tc := tc
				t.Run(tc.name, func(t *testing.T) {
					var buf bytes.Buffer
					logger := factory.make(&buf)
					std := logport.LogLogger(logger)
					std.SetFlags(0)

					std.Println(tc.input)

					out := buf.String()
					if tc.requireOutput && out == "" {
						t.Fatalf("expected output for %s/%s", factory.name, tc.name)
					}
					if tc.expect != "" && !strings.Contains(out, tc.expect) {
						t.Fatalf("output %q does not contain %q", out, tc.expect)
					}
				})
			}
		})
	}
}
