package logport_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestAdaptersWithTraceAddsSpanContext(t *testing.T) {
	traceID, _ := oteltrace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	spanID, _ := oteltrace.SpanIDFromHex("fedcba9876543210")
	spanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: oteltrace.FlagsSampled,
	})
	ctx := oteltrace.ContextWithSpanContext(context.Background(), spanCtx)

	for _, factory := range adapterFactories() {
		factory := factory
		t.Run(factory.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := factory.make(&buf)
			logger.WithTrace(ctx).Info("with trace")

			got := buf.String()
			if got == "" {
				t.Fatalf("expected output for %s", factory.name)
			}
			if !strings.Contains(got, traceID.String()) {
				t.Fatalf("expected trace id %q in output %q", traceID.String(), got)
			}
			if !strings.Contains(got, spanID.String()) {
				t.Fatalf("expected span id %q in output %q", spanID.String(), got)
			}
		})
	}
}
