package logport

import (
	"context"

	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	// TraceIDKey is the structured logging key used to store OpenTelemetry trace IDs.
	TraceIDKey = "trace_id"
	// SpanIDKey is the structured logging key used to store OpenTelemetry span IDs.
	SpanIDKey = "span_id"
)

// TraceKeyvalsFromContext extracts OpenTelemetry trace identifiers from ctx and
// returns them as alternating key/value pairs using TraceIDKey and SpanIDKey.
// When ctx does not contain a valid span context, the returned slice is nil.
func TraceKeyvalsFromContext(ctx context.Context) []any {
	if ctx == nil {
		return nil
	}
	spanCtx := oteltrace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return nil
	}

	traceID := spanCtx.TraceID()
	spanID := spanCtx.SpanID()
	if !traceID.IsValid() && !spanID.IsValid() {
		return nil
	}

	keyvals := make([]any, 0, 4)
	if traceID.IsValid() {
		keyvals = append(keyvals, TraceIDKey, traceID.String())
	}
	if spanID.IsValid() {
		keyvals = append(keyvals, SpanIDKey, spanID.String())
	}
	return keyvals
}

// AppendTraceKeyvals returns a slice that includes trace identifiers extracted
// from ctx (when present) followed by the supplied keyvals. When ctx does not
// carry a valid span context, keyvals is returned unchanged.
func AppendTraceKeyvals(ctx context.Context, keyvals []any) []any {
	traceKeyvals := TraceKeyvalsFromContext(ctx)
	if len(traceKeyvals) == 0 {
		return keyvals
	}
	if len(keyvals) == 0 {
		return traceKeyvals
	}
	combined := make([]any, 0, len(traceKeyvals)+len(keyvals))
	combined = append(combined, traceKeyvals...)
	combined = append(combined, keyvals...)
	return combined
}
