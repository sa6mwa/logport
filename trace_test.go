package logport

import (
	"context"
	"testing"

	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestTraceKeyvalsFromContextExtractsIDs(t *testing.T) {
	traceID, _ := oteltrace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	spanID, _ := oteltrace.SpanIDFromHex("1111111111111111")

	spanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: oteltrace.FlagsSampled,
		Remote:     true,
	})
	ctx := oteltrace.ContextWithSpanContext(context.Background(), spanCtx)

	keyvals := TraceKeyvalsFromContext(ctx)
	if len(keyvals) != 4 {
		t.Fatalf("expected 4 keyvals, got %d (%v)", len(keyvals), keyvals)
	}
	if keyvals[0] != TraceIDKey || keyvals[1] != traceID.String() {
		t.Fatalf("expected trace key/value, got %v", keyvals[:2])
	}
	if keyvals[2] != SpanIDKey || keyvals[3] != spanID.String() {
		t.Fatalf("expected span key/value, got %v", keyvals[2:])
	}
}

func TestTraceKeyvalsFromContextNilOrInvalid(t *testing.T) {
	var nilCtx context.Context
	if got := TraceKeyvalsFromContext(nilCtx); len(got) != 0 {
		t.Fatalf("expected nil context to return no keyvals, got %v", got)
	}

	ctx := context.Background()
	if got := TraceKeyvalsFromContext(ctx); len(got) != 0 {
		t.Fatalf("expected empty context to return no keyvals, got %v", got)
	}

	invalidSpan := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{})
	ctx = oteltrace.ContextWithSpanContext(context.Background(), invalidSpan)
	if got := TraceKeyvalsFromContext(ctx); len(got) != 0 {
		t.Fatalf("expected invalid span context to return no keyvals, got %v", got)
	}
}
